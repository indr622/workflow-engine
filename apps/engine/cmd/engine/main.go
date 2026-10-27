// Command engine adalah composition root workflow engine: memuat konfigurasi,
// merakit semua modul, menjalankan migrasi, lalu melayani HTTP/Connect.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	healthapi "github.com/indr622/workflow-engine/apps/engine/internal/modules/health/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/health/application"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/health/infrastructure"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/health/interfaces"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/config"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/db"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/eventbus"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/httpserver"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/logger"
)

// Pastikan use case health memenuhi kontrak publik modulnya.
var _ healthapi.Checker = (*application.CheckHealth)(nil)

// version diisi saat build lewat -ldflags "-X main.version=...".
var version = "dev"

func main() {
	// Subperintah "healthcheck" dipakai HEALTHCHECK Docker karena image distroless tidak punya curl.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "engine: %v\n", err)
		os.Exit(1)
	}
}

// healthcheck memanggil /healthz pada engine lokal dan mengembalikan exit code 0 jika serving.
func healthcheck() int {
	addr := os.Getenv("ENGINE_HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get( //nolint:gosec // alamat dari konfigurasi lokal engine, bukan input pengguna
		"http://127.0.0.1" + addr[strings.LastIndex(addr, ":"):] + "/healthz")
	if err != nil {
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logger.New(cfg.LogLevel)
	log.Info("starting engine", slog.String("version", version))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if cfg.Migrate {
		if err := db.Migrate(ctx, pool, log); err != nil {
			return err
		}
	}

	// Event bus dibagi semua modul; belum ada subscriber di Fase 0.
	_ = eventbus.NewInMemory()

	mux := http.NewServeMux()

	// Modul health.
	checkHealth := application.NewCheckHealth(version, 3*time.Second, infrastructure.NewPostgresProbe(pool))
	interfaces.NewHandler(checkHealth).Register(mux)

	srv := httpserver.New(cfg.HTTPAddr, httpserver.CORS(cfg.CORSOrigins, mux), log)
	return srv.Run(ctx, cfg.ShutdownTimeout)
}
