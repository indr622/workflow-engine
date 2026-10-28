package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	deploymentapi "github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/api"
	deploymentapp "github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/application"
	deploymentinfra "github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/infrastructure"
	deploymentifc "github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/interfaces"
	executionapp "github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/application"
	executiondomain "github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/domain"
	executioninfra "github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/infrastructure"
	executionifc "github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/interfaces"
	healthapi "github.com/indr622/workflow-engine/apps/engine/internal/modules/health/api"
	healthapp "github.com/indr622/workflow-engine/apps/engine/internal/modules/health/application"
	healthinfra "github.com/indr622/workflow-engine/apps/engine/internal/modules/health/infrastructure"
	healthifc "github.com/indr622/workflow-engine/apps/engine/internal/modules/health/interfaces"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/config"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/db"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/eventbus"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/keygen"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/outbox"
)

// Pastikan use case health memenuhi kontrak publik modulnya.
var _ healthapi.Checker = (*healthapp.CheckHealth)(nil)

// systemClock memenuhi port Clock domain execution dengan jam sistem (UTC).
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// application adalah hasil perakitan semua modul, dipakai server dan subperintah admin.
type application struct {
	pool     *pgxpool.Pool
	mux      *http.ServeMux
	keys     *keygen.UUIDv7
	writer   *outbox.Writer
	relay    *outbox.Relay
	recovery *executionapp.Recovery
	commands *executionapp.Commands
}

// assemble membuka pool, menjalankan migrasi (bila diaktifkan), lalu merakit modul.
func assemble(ctx context.Context, cfg config.Config, log *slog.Logger) (*application, error) {
	pool, err := db.Connect(ctx, cfg.DBConfig())
	if err != nil {
		return nil, err
	}
	if cfg.Migrate {
		if err := db.Migrate(ctx, pool, log); err != nil {
			pool.Close()
			return nil, err
		}
	}

	// Event bus in-process (sinyal best-effort), generator key UUIDv7, dan outbox dibagi semua modul.
	bus := eventbus.NewInMemory()
	keys := keygen.New()
	writer := outbox.NewWriter(keys)
	relay := outbox.NewRelay(pool, log, outbox.Options{PollInterval: cfg.Outbox.PollInterval, BatchSize: cfg.Outbox.BatchSize})
	mux := http.NewServeMux()

	// Modul health.
	checkHealth := healthapp.NewCheckHealth(version, 3*time.Second, healthinfra.NewPostgresProbe(pool))
	healthifc.NewHandler(checkHealth).Register(mux)

	// Modul deployment.
	deploymentRepo := deploymentinfra.NewPostgresRepository(pool, keys)
	processCache := deploymentapp.NewProcessCache(deploymentRepo, 256)
	deployResource := deploymentapp.NewDeployResource(deploymentRepo, bus, processCache, nil, log)
	deploymentifc.NewHandler(deployResource, deploymentapp.NewQueries(deploymentRepo)).Register(mux)
	var _ deploymentapi.ProcessDefinitions = processCache

	// Modul execution: hanya bergantung pada deployment lewat api.ProcessDefinitions.
	executionRepo := executioninfra.NewPostgresRepository(pool, writer).WithSnapshotInterval(cfg.SnapshotIntervalRecords)
	processor := executiondomain.NewProcessor(executiondomain.NewFeelEvaluator(), systemClock{},
		executiondomain.Limits{MaxRecordsPerCommand: cfg.MaxRecordsPerCommand})
	commands := executionapp.NewCommands(executionRepo, processCache, processor,
		executioninfra.NewKeyGenerator(keys), bus, log, executionapp.Config{MaxVariablesBytes: cfg.MaxVariablesBytes})
	executionifc.NewHandler(commands, commands, executionapp.NewQueries(executionRepo)).Register(mux)
	rec := executionapp.NewRecovery(executionRepo, processCache, log, executionapp.RecoveryConfig{
		DeepWindow: cfg.Recovery.DeepWindow, DeepSample: cfg.Recovery.DeepSample,
	})

	return &application{
		pool: pool, mux: mux, keys: keys, writer: writer, relay: relay, recovery: rec, commands: commands,
	}, nil
}
