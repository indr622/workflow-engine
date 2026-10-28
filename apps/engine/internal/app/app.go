// Package app merakit seluruh modul engine (composition root). cmd/engine dan test e2e memakai
// perakitan yang sama sehingga e2e menguji engine yang persis sama dengan produksi.
package app

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
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
	jobapp "github.com/indr622/workflow-engine/apps/engine/internal/modules/job/application"
	jobdomain "github.com/indr622/workflow-engine/apps/engine/internal/modules/job/domain"
	jobinfra "github.com/indr622/workflow-engine/apps/engine/internal/modules/job/infrastructure"
	jobifc "github.com/indr622/workflow-engine/apps/engine/internal/modules/job/interfaces"
	messageapp "github.com/indr622/workflow-engine/apps/engine/internal/modules/message/application"
	messagedomain "github.com/indr622/workflow-engine/apps/engine/internal/modules/message/domain"
	messageinfra "github.com/indr622/workflow-engine/apps/engine/internal/modules/message/infrastructure"
	messageifc "github.com/indr622/workflow-engine/apps/engine/internal/modules/message/interfaces"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/config"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/db"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/eventbus"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/keygen"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/outbox"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/rpc"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/sweeper"
)

// Pastikan use case health memenuhi kontrak publik modulnya.
var _ healthapi.Checker = (*healthapp.CheckHealth)(nil)

// systemClock memenuhi port Clock domain execution dengan jam sistem (UTC).
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// App adalah hasil perakitan semua modul, dipakai server, subperintah admin, dan test e2e.
type App struct {
	Pool      *pgxpool.Pool
	Mux       *http.ServeMux
	Keys      *keygen.UUIDv7
	Writer    *outbox.Writer
	Relay     *outbox.Relay
	Recovery  *executionapp.Recovery
	Commands  *executionapp.Commands
	Retention *executionapp.Retention
	// Sweeper menjalankan lease job dan TTL message yang kedaluwarsa.
	Sweeper *sweeper.Runner

	stopping chan struct{}
	stopOnce sync.Once
}

// Build membuka pool, menjalankan migrasi (bila diaktifkan), lalu merakit modul.
func Build(ctx context.Context, cfg config.Config, log *slog.Logger, version string) (*App, error) {
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
	a, err := assemble(pool, cfg, log, version)
	if err != nil {
		pool.Close()
		return nil, err
	}
	return a, nil
}

func assemble(pool *pgxpool.Pool, cfg config.Config, log *slog.Logger, version string) (*App, error) {
	// Rantai interceptor yang sama untuk setiap handler Connect (ADR-0023).
	opts, err := rpc.HandlerOptions(rpc.Config{Log: log, DefaultTimeout: cfg.RPCDefaultTimeout})
	if err != nil {
		return nil, err
	}

	// Event bus in-process (sinyal best-effort), generator key UUIDv7, dan outbox dibagi semua modul.
	bus := eventbus.NewInMemory()
	keys := keygen.New()
	writer := outbox.NewWriter(keys)
	relay := outbox.NewRelay(pool, log, outbox.Options{PollInterval: cfg.Outbox.PollInterval, BatchSize: cfg.Outbox.BatchSize})
	mux := http.NewServeMux()
	stopping := make(chan struct{})

	// Modul health.
	checkHealth := healthapp.NewCheckHealth(version, 3*time.Second, healthinfra.NewPostgresProbe(pool))
	healthifc.NewHandler(checkHealth).Register(mux, opts...)

	// Modul deployment.
	deploymentRepo := deploymentinfra.NewPostgresRepository(pool, keys).WithIdempotencyTTL(cfg.IdempotencyTTL)
	processCache := deploymentapp.NewProcessCache(deploymentRepo, 256)
	deployResource := deploymentapp.NewDeployResource(deploymentRepo, bus, processCache, nil, log)
	deploymentifc.NewHandler(deployResource, deploymentapp.NewQueries(deploymentRepo)).Register(mux, opts...)
	var _ deploymentapi.ProcessDefinitions = processCache

	// Modul execution: hanya bergantung pada deployment lewat api.ProcessDefinitions.
	executionRepo := executioninfra.NewPostgresRepository(pool, writer).
		WithSnapshotInterval(cfg.SnapshotIntervalRecords).WithIdempotencyTTL(cfg.IdempotencyTTL)
	processor := executiondomain.NewProcessor(executiondomain.NewFeelEvaluator(), systemClock{},
		executiondomain.Limits{MaxRecordsPerCommand: cfg.MaxRecordsPerCommand})
	commands := executionapp.NewCommands(executionRepo, processCache, processor,
		executioninfra.NewKeyGenerator(keys), bus, log, executionapp.Config{MaxVariablesBytes: cfg.MaxVariablesBytes})
	queries := executionapp.NewQueries(executionRepo)
	executionifc.NewHandler(commands, commands, queries, executionapp.NewHistory(executionRepo)).Register(mux, opts...)
	executionifc.NewIncidentHandler(commands, queries).Register(mux, opts...)
	rec := executionapp.NewRecovery(executionRepo, processCache, log, executionapp.RecoveryConfig{
		DeepWindow: cfg.Recovery.DeepWindow, DeepSample: cfg.Recovery.DeepSample,
	})
	retention := executionapp.NewRetention(executionRepo, log, 500, cfg.HistoryPrunePause)

	// Modul job: protokol worker di atas execution/api.
	notifier := jobinfra.NewNotifier(bus)
	activate := jobapp.NewActivateJobs(commands, notifier, jobdomain.Limits{
		MaxJobs: cfg.Job.MaxActivate, MinTimeout: cfg.Job.MinTimeout, MaxTimeout: cfg.Job.MaxTimeout,
		MaxRequestTimeout: cfg.Job.MaxRequestTimeout, MaxBackoff: cfg.Job.MaxBackoff,
		PollInterval: cfg.Job.LongPollInterval, MaxLongPolls: cfg.Job.MaxLongPolling,
	}, stopping)
	jobifc.NewHandler(activate, commands, queries).Register(mux, opts...)

	// Modul message: buffer dengan TTL; korelasi menyusul di Fase 5.
	messageRepo := messageinfra.NewPostgresRepository(pool)
	publish := messageapp.NewPublishMessage(messageRepo, keys, bus,
		messagedomain.Policy{MaxTTL: cfg.MessageMaxTTL, MaxVariablesBytes: cfg.MaxVariablesBytes}, log)
	messageifc.NewHandler(publish).Register(mux, opts...)

	sw := &sweeper.Runner{
		Tasks:    []sweeper.Task{jobapp.NewSweepLeases(commands, notifier), messageapp.NewSweepExpired(messageRepo)},
		Interval: cfg.SweepInterval, Log: log,
	}

	return &App{
		Pool: pool, Mux: mux, Keys: keys, Writer: writer, Relay: relay, Recovery: rec, Commands: commands,
		Retention: retention, Sweeper: sw, stopping: stopping,
	}, nil
}

// BeginShutdown menjawab semua long polling dengan respons kosong agar server bisa berhenti tanpa
// menunggu request_timeout worker. Aman dipanggil berkali-kali.
func (a *App) BeginShutdown() {
	a.stopOnce.Do(func() { close(a.stopping) })
}
