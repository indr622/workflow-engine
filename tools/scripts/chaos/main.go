// Command chaos menjalankan suite chaos berbasis Compose (rencana Fase 3, 11.2–11.3, langkah
// 12.10): load generator lewat API Connect, injeksi kegagalan di level proses/container lewat
// Docker CLI (kill -9, restart, partisi jaringan, replika ganda), lalu verifikasi invarian
// durabilitas (3.1) terhadap database dan `engine verify --all`.
//
//	go run ./tools/scripts/chaos -scenario all -duration 30s
//	go run ./tools/scripts/chaos -scenario kill-engine,replicas -duration 15s
//	go run ./tools/scripts/chaos -list
//
// Setiap skenario memakai project compose yang segar (`down -v` lalu `up`). Biasanya dipanggil
// lewat `make chaos`, yang juga membangun image engine dan selalu membersihkan project di akhir.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"
)

type config struct {
	scenario      string
	duration      time.Duration
	composeFile   string
	image         string
	engineURL     string
	engine2URL    string
	dbURL         string
	workers       int
	readyTimeout  time.Duration
	retryTimeout  time.Duration
	logDir        string
	migrationsDir string
	keep          bool
	verbose       bool
}

type outcome struct {
	sc       scenario
	err      error
	took     time.Duration
	logPath  string
	acked    int64
	errors   int64
	replayed int64
}

func main() {
	os.Exit(run())
}

func run() int {
	var cfg config
	list := flag.Bool("list", false, "list scenarios and exit")
	flag.StringVar(&cfg.scenario, "scenario", "all", "all, or a comma-separated list of scenario names or numbers")
	flag.DurationVar(&cfg.duration, "duration", 30*time.Second, "load duration per scenario")
	flag.StringVar(&cfg.composeFile, "compose", "infra/docker/docker-compose.chaos.yml", "chaos compose file")
	flag.StringVar(&cfg.image, "image", os.Getenv("CHAOS_ENGINE_IMAGE"), "engine image (CHAOS_ENGINE_IMAGE)")
	flag.StringVar(&cfg.engineURL, "engine", "http://localhost:8180", "engine base URL")
	flag.StringVar(&cfg.engine2URL, "engine2", "http://localhost:8181", "second replica base URL (profile replicas)")
	flag.StringVar(&cfg.dbURL, "db", "postgres://workflow:chaos@localhost:5544/workflow?sslmode=disable", "chaos postgres URL for invariant checks")
	flag.IntVar(&cfg.workers, "workers", 8, "load generator goroutines")
	flag.DurationVar(&cfg.readyTimeout, "ready-timeout", 120*time.Second, "max wait for /readyz after an injection")
	flag.DurationVar(&cfg.retryTimeout, "retry-timeout", 90*time.Second, "max time to resolve un-acked commands")
	flag.StringVar(&cfg.logDir, "logs", "tmp/chaos-logs", "directory for logs of failed scenarios")
	flag.StringVar(&cfg.migrationsDir, "migrations", "apps/engine/internal/platform/db/migrations", "engine migrations (scenario concurrent-start)")
	flag.BoolVar(&cfg.keep, "keep", false, "leave the last compose project running")
	flag.BoolVar(&cfg.verbose, "v", false, "print the runner log while scenarios run")
	flag.Parse()

	if *list {
		printScenarioList()
		return 0
	}
	selected, err := selectScenarios(cfg.scenario)
	if err != nil {
		fmt.Fprintln(os.Stderr, "chaos:", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dc := &compose{file: cfg.composeFile, image: cfg.image}

	fmt.Printf("chaos suite: %d scenarios, load %s each, compose %s, image %s\n", len(selected), cfg.duration, cfg.composeFile, orDefault(cfg.image, "(compose default)"))
	var results []outcome
	for _, sc := range selected {
		fmt.Printf("\n[%d] %s: %s\n", sc.number, sc.name, sc.title)
		s := newSession(cfg, dc)
		start := time.Now()
		runErr := sc.run(ctx, s)
		o := outcome{sc: sc, err: runErr, took: time.Since(start), acked: s.ledger.acked.Load(), errors: s.ledger.errors.Load(), replayed: s.ledger.replayed.Load()}
		if runErr != nil {
			// Log dikumpulkan dengan context baru: Ctrl-C tidak boleh menghilangkan bukti.
			path, werr := writeFailureLog(context.Background(), cfg.logDir, s, sc.name, runErr)
			if werr != nil {
				fmt.Fprintf(os.Stderr, "  could not write logs: %v\n", werr)
			}
			o.logPath = path
			fmt.Printf("  FAIL after %s: %v\n", o.took.Round(time.Second), runErr)
		} else {
			fmt.Printf("  PASS in %s\n", o.took.Round(time.Second))
		}
		if !cfg.verbose {
			fmt.Print(indent(s.log.String(), "    "))
		}
		results = append(results, o)
		if ctx.Err() != nil {
			break
		}
	}
	if !cfg.keep {
		if err := dc.down(context.Background()); err != nil {
			fmt.Fprintln(os.Stderr, "chaos: teardown:", err)
		}
	}
	if !printSummary(results) {
		return 1
	}
	return 0
}

func selectScenarios(spec string) ([]scenario, error) {
	if spec == "" || spec == "all" {
		return scenarios, nil
	}
	var out []scenario
	for _, name := range strings.Split(spec, ",") {
		name = strings.TrimSpace(name)
		found := false
		for _, sc := range scenarios {
			if sc.name == name || fmt.Sprint(sc.number) == name {
				out, found = append(out, sc), true
				break
			}
		}
		if !found {
			for _, sk := range skippedScenarios {
				if sk.name == name || fmt.Sprint(sk.number) == name {
					return nil, fmt.Errorf("scenario %s (#%d) is not run by this suite: %s", sk.name, sk.number, sk.reason)
				}
			}
			return nil, fmt.Errorf("unknown scenario %q (see -list)", name)
		}
	}
	return out, nil
}

func printScenarioList() {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "#\tNAME\tINJECTION")
	for _, sc := range scenarios {
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\n", sc.number, sc.name, sc.title)
	}
	_, _ = fmt.Fprintln(w, "\n#\tSKIPPED\tREASON")
	for _, sk := range skippedScenarios {
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\n", sk.number, sk.name, sk.reason)
	}
	_ = w.Flush()
}

// printSummary mencetak tabel PASS/FAIL dan mengembalikan true bila semua lulus.
func printSummary(results []outcome) bool {
	ok := len(results) > 0
	fmt.Println()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "#\tSCENARIO\tRESULT\tDURATION\tACKED\tERRORS\tREPLAYED\tDETAIL")
	for _, r := range results {
		res, detail := "PASS", ""
		if r.err != nil {
			ok = false
			res, detail = "FAIL", tail(strings.ReplaceAll(r.err.Error(), "\n", " "), 160)
			if r.logPath != "" {
				detail += " (logs: " + r.logPath + ")"
			}
		}
		_, _ = fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%d\t%d\t%d\t%s\n", r.sc.number, r.sc.name, res, r.took.Round(time.Second), r.acked, r.errors, r.replayed, detail)
	}
	for _, sk := range skippedScenarios {
		_, _ = fmt.Fprintf(w, "%d\t%s\tSKIP\t\t\t\t\t%s\n", sk.number, sk.name, sk.reason)
	}
	_ = w.Flush()
	if ok {
		fmt.Println("\nchaos: all scenarios passed")
	} else {
		fmt.Println("\nchaos: FAILED")
	}
	return ok
}

func indent(s, prefix string) string {
	if s == "" {
		return ""
	}
	return prefix + strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", "\n"+prefix) + "\n"
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
