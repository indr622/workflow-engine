// Command engine adalah composition root workflow engine: memuat konfigurasi, merakit semua modul,
// menjalankan migrasi dan pemulihan, lalu melayani HTTP/Connect. Subperintah admin (verify,
// rebuild, prune, outbox) memakai perakitan yang sama tanpa server HTTP.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/indr622/workflow-engine/apps/engine/internal/platform/config"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/httpserver"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/idempotency"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/logger"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/outbox"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/recovery"
)

// version diisi saat build lewat -ldflags "-X main.version=...".
var version = "dev"

const usage = `usage: engine [command]

commands:
  serve (default)   run migrations and recovery, then serve HTTP/Connect
  healthcheck       exit 0 when the local engine is ready (Docker HEALTHCHECK)
  verify            check projections against execution records (--all, --instance KEY, --json)
  rebuild           rebuild projections from execution records (--instance KEY | --all)
  prune             delete finished instances older than the retention (--older-than, --dry-run)
  outbox            outbox status, or requeue a dead letter (status | requeue --consumer C --event-id E)
`

func main() {
	cmd, args := "serve", os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	var code int
	switch cmd {
	case "serve":
		code = exitCode(serve())
	case "healthcheck":
		code = healthcheck()
	case "verify", "rebuild", "prune", "outbox":
		code = admin(cmd, args)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "engine: unknown command %q\n\n%s", cmd, usage)
		code = 2
	}
	os.Exit(code)
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	fmt.Fprintf(os.Stderr, "engine: %v\n", err)
	return 1
}

// healthcheck memanggil /readyz pada engine lokal; exit 0 hanya bila engine sudah ready.
func healthcheck() int {
	addr := os.Getenv("ENGINE_HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get( //nolint:gosec // alamat dari konfigurasi lokal engine, bukan input pengguna
		"http://127.0.0.1" + addr[strings.LastIndex(addr, ":"):] + "/readyz")
	if err != nil {
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

// serve menjalankan urutan start Fase 3 (5.2): migrasi → server HTTP mendengarkan (liveness saja)
// → pemulihan → relay outbox dan janitor → ready.
func serve() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logger.New(cfg.LogLevel)
	log.Info("starting engine", slog.String("version", version))
	onMismatch, err := recovery.ParseOnMismatch(cfg.Recovery.OnMismatch)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := assemble(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer app.pool.Close()

	gate := recovery.NewGate()
	app.mux.Handle("/readyz", gate.ReadyzHandler())
	handler := gate.Middleware(httpserver.CORS(cfg.CORSOrigins, app.mux), "/healthz", "/readyz")
	srv := httpserver.New(cfg.HTTPAddr, handler, log)
	serverErr := make(chan error, 1)
	go func() { serverErr <- srv.Run(ctx, cfg.ShutdownTimeout) }()

	runner := recovery.Runner{
		Verifiers: []recovery.Verifier{app.recovery}, OnMismatch: onMismatch,
		Timeout: cfg.Recovery.Timeout, Log: log, Gate: gate,
	}
	if _, err := runner.Run(ctx); err != nil {
		stop()
		if serr := <-serverErr; serr != nil {
			log.Warn("http server stopped", slog.Any("error", serr))
		}
		return err
	}

	go func() {
		if err := app.relay.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("outbox relay stopped", slog.Any("error", err))
		}
	}()
	go janitor(ctx, app, cfg, log)

	gate.SetReady()
	log.Info("engine ready", slog.String("addr", cfg.HTTPAddr))
	return <-serverErr
}

// janitor menjalankan pembersihan berkala: outbox yang sudah dilewati semua consumer, instance
// final di luar retensi (bila diaktifkan), dan klaim request_id yang kedaluwarsa.
func janitor(ctx context.Context, app *application, cfg config.Config, log *slog.Logger) {
	t := time.NewTicker(cfg.JanitorInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if n, err := outbox.Prune(ctx, app.pool, cfg.Outbox.Retention); err != nil {
			log.Warn("outbox prune failed", slog.Any("error", err))
		} else if n > 0 {
			log.Info("outbox pruned", slog.Int64("events", n))
		}
		if cfg.HistoryRetentionDays > 0 {
			if _, err := app.retention.Prune(ctx, time.Duration(cfg.HistoryRetentionDays)*24*time.Hour, false); err != nil {
				log.Warn("history prune failed", slog.Any("error", err))
			}
		}
		if n, err := idempotency.Prune(ctx, app.pool, 1000); err != nil {
			log.Warn("idempotency prune failed", slog.Any("error", err))
		} else if n > 0 {
			log.Info("idempotency keys pruned", slog.Int64("keys", n))
		}
	}
}
