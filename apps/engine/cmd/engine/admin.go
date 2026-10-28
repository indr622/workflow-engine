package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	executionapp "github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/application"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/config"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/keygen"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/logger"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/outbox"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/recovery"
)

// Exit code subperintah admin.
const (
	exitOK       = 0
	exitError    = 1
	exitUsage    = 2
	exitMismatch = 3
)

// admin menjalankan subperintah operator tanpa server HTTP. Migrasi tidak dijalankan kecuali
// ENGINE_MIGRATE diset eksplisit, agar perintah baca tidak mengubah skema database restore.
func admin(cmd string, args []string) int {
	cfg, err := config.Load()
	if err != nil {
		return exitCode(err)
	}
	if _, set := os.LookupEnv("ENGINE_MIGRATE"); !set {
		cfg.Migrate = false
	}
	log := logger.New(cfg.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch cmd {
	case "verify":
		return runVerify(ctx, cfg, log, args)
	case "rebuild":
		return runRebuild(ctx, cfg, log, args)
	case "prune":
		return runPrune(ctx, cfg, log, args)
	case "outbox":
		return runOutbox(ctx, cfg, log, args)
	}
	return exitUsage
}

func runVerify(ctx context.Context, cfg config.Config, log *slog.Logger, args []string) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	all := fs.Bool("all", false, "deep-verify every instance (default: changed recently + sample)")
	instance := fs.String("instance", "", "deep-verify one process instance key")
	asJSON := fs.Bool("json", false, "print the report as JSON")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	app, err := assemble(ctx, cfg, log)
	if err != nil {
		return exitCode(err)
	}
	defer app.pool.Close()

	var rep recovery.Report
	switch {
	case *instance != "":
		if !keygen.Valid(*instance) {
			fmt.Fprintln(os.Stderr, "engine verify: --instance must be a canonical uuid")
			return exitUsage
		}
		m, err := app.recovery.DeepVerify(ctx, *instance)
		if err != nil {
			return exitCode(err)
		}
		rep = recovery.Report{Checked: 1, Mismatches: m}
	case *all:
		rep, err = app.recovery.VerifyAll(ctx)
	default:
		rep, err = app.recovery.VerifyOnStart(ctx)
	}
	if err != nil {
		return exitCode(err)
	}
	printReport(os.Stdout, rep, *asJSON)
	if len(rep.Mismatches) > 0 {
		return exitMismatch
	}
	return exitOK
}

func printReport(w io.Writer, rep recovery.Report, asJSON bool) {
	if asJSON {
		_ = json.NewEncoder(w).Encode(rep)
		return
	}
	_, _ = fmt.Fprintf(w, "checked %d instances in %s, %d mismatches\n", rep.Checked, rep.Duration.Round(1e6), len(rep.Mismatches))
	for _, m := range rep.Mismatches {
		_, _ = fmt.Fprintf(w, "  %s %s: %s\n", m.Subject, m.Kind, m.Detail)
	}
}

func runRebuild(ctx context.Context, cfg config.Config, log *slog.Logger, args []string) int {
	fs := flag.NewFlagSet("rebuild", flag.ContinueOnError)
	all := fs.Bool("all", false, "rebuild every instance")
	instance := fs.String("instance", "", "rebuild one process instance key")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if (*instance == "") == !*all {
		fmt.Fprintln(os.Stderr, "engine rebuild: pass exactly one of --instance KEY or --all")
		return exitUsage
	}
	app, err := assemble(ctx, cfg, log)
	if err != nil {
		return exitCode(err)
	}
	defer app.pool.Close()
	if *instance != "" {
		if !keygen.Valid(*instance) {
			fmt.Fprintln(os.Stderr, "engine rebuild: --instance must be a canonical uuid")
			return exitUsage
		}
		res, err := app.recovery.RebuildInstance(ctx, *instance)
		if err != nil {
			return exitCode(err)
		}
		fmt.Printf("rebuilt %s from %d records\n", res.ProcessInstanceKey, res.Records)
		return exitOK
	}
	n, err := app.recovery.RebuildAll(ctx, func(res executionapp.RebuildResult) {
		fmt.Printf("rebuilt %s from %d records\n", res.ProcessInstanceKey, res.Records)
	})
	fmt.Printf("rebuilt %d instances\n", n)
	return exitCode(err)
}

func runPrune(_ context.Context, _ config.Config, _ *slog.Logger, _ []string) int {
	fmt.Fprintln(os.Stderr, "engine prune: retention is not enabled yet")
	return exitError
}

func runOutbox(ctx context.Context, cfg config.Config, log *slog.Logger, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "engine outbox: expected status or requeue")
		return exitUsage
	}
	app, err := assemble(ctx, cfg, log)
	if err != nil {
		return exitCode(err)
	}
	defer app.pool.Close()
	switch args[0] {
	case "status":
		statuses, err := outbox.Status(ctx, app.pool)
		if err != nil {
			return exitCode(err)
		}
		fmt.Printf("%-40s %10s %8s %12s  %s\n", "CONSUMER", "PENDING", "ATTEMPTS", "DEAD LETTERS", "LAST ERROR")
		for _, s := range statuses {
			fmt.Printf("%-40s %10d %8d %12d  %s\n", s.Consumer, s.Pending, s.Attempts, s.DeadLetters, s.LastError)
		}
		return exitOK
	case "requeue":
		fs := flag.NewFlagSet("outbox requeue", flag.ContinueOnError)
		consumer := fs.String("consumer", "", "consumer name")
		eventID := fs.String("event-id", "", "dead-lettered event id")
		if err := fs.Parse(args[1:]); err != nil || *consumer == "" || !keygen.Valid(*eventID) {
			fmt.Fprintln(os.Stderr, "engine outbox requeue: --consumer and a canonical --event-id are required")
			return exitUsage
		}
		if err := outbox.Requeue(ctx, app.pool, app.writer, *consumer, *eventID); err != nil {
			return exitCode(err)
		}
		fmt.Printf("requeued %s for %s\n", *eventID, *consumer)
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "engine outbox: unknown subcommand %q\n", args[0])
		return exitUsage
	}
}
