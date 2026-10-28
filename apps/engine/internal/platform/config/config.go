// Package config memuat konfigurasi engine dari environment variable.
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"

	"github.com/indr622/workflow-engine/apps/engine/internal/platform/db"
)

// Config adalah seluruh konfigurasi runtime engine.
type Config struct {
	HTTPAddr        string        `env:"ENGINE_HTTP_ADDR" envDefault:":8080"`
	DatabaseURL     string        `env:"DATABASE_URL" envDefault:"postgres://workflow:workflow@localhost:5433/workflow?sslmode=disable"`
	Migrate         bool          `env:"ENGINE_MIGRATE" envDefault:"true"`
	LogLevel        string        `env:"ENGINE_LOG_LEVEL" envDefault:"info"`
	ShutdownTimeout time.Duration `env:"ENGINE_SHUTDOWN_TIMEOUT" envDefault:"15s"`
	CORSOrigins     []string      `env:"ENGINE_CORS_ORIGINS" envSeparator:"," envDefault:"http://localhost:4200"`
	// MaxRecordsPerCommand membatasi record satu command; loop tanpa wait state menjadi incident.
	MaxRecordsPerCommand int `env:"ENGINE_MAX_RECORDS_PER_COMMAND" envDefault:"10000"`
	// MaxVariablesBytes membatasi dokumen variabel per request (1 MiB).
	MaxVariablesBytes int `env:"ENGINE_MAX_VARIABLES_BYTES" envDefault:"1048576"`
	// DB adalah grup ENGINE_DB_*: ukuran pool dan timeout sesi Postgres (rencana Fase 3, 9.1).
	DB DB
	// Recovery mengatur pemulihan saat start (Fase 3, 5.2).
	Recovery Recovery
	// Outbox mengatur relay outbox (Fase 3, 4.3).
	Outbox Outbox
	// SnapshotIntervalRecords: snapshot state runtime instance aktif setiap N record (0 = mati).
	SnapshotIntervalRecords int `env:"ENGINE_SNAPSHOT_INTERVAL_RECORDS" envDefault:"5000"`
	// IdempotencyTTL adalah masa berlaku klaim request_id.
	IdempotencyTTL time.Duration `env:"ENGINE_IDEMPOTENCY_TTL" envDefault:"24h"`
	// HistoryRetentionDays: instance final lebih tua dari N hari dihapus janitor (0 = simpan selamanya).
	HistoryRetentionDays int `env:"ENGINE_HISTORY_RETENTION_DAYS" envDefault:"0"`
	// HistoryPrunePause adalah jeda antar batch penghapusan retensi.
	HistoryPrunePause time.Duration `env:"ENGINE_HISTORY_PRUNE_PAUSE" envDefault:"200ms"`
	// JanitorInterval adalah jeda antar putaran janitor (outbox, idempotensi, retensi).
	JanitorInterval time.Duration `env:"ENGINE_JANITOR_INTERVAL" envDefault:"1m"`
	// RPCDefaultTimeout membatasi RPC biasa; long polling memakai request_timeout + 5 s (Fase 4).
	RPCDefaultTimeout time.Duration `env:"ENGINE_RPC_DEFAULT_TIMEOUT" envDefault:"30s"`
	// Job mengatur protokol job worker (Fase 4, bagian 4).
	Job Job
	// MessageMaxTTL membatasi time_to_live PublishMessage.
	MessageMaxTTL time.Duration `env:"ENGINE_MESSAGE_MAX_TTL" envDefault:"720h"`
	// SweepInterval adalah jeda sweeper lease job dan message kedaluwarsa.
	SweepInterval time.Duration `env:"ENGINE_SWEEP_INTERVAL" envDefault:"1s"`
}

// Job mengatur klem parameter ActivateJobs dan long polling. Nilai di luar batas dipotong, bukan
// ditolak, agar worker lama tetap jalan setelah batas diturunkan.
type Job struct {
	MaxActivate       int32         `env:"ENGINE_JOB_MAX_ACTIVATE" envDefault:"1000"`
	MinTimeout        time.Duration `env:"ENGINE_JOB_MIN_TIMEOUT" envDefault:"1s"`
	MaxTimeout        time.Duration `env:"ENGINE_JOB_MAX_TIMEOUT" envDefault:"24h"`
	MaxRequestTimeout time.Duration `env:"ENGINE_JOB_MAX_REQUEST_TIMEOUT" envDefault:"55s"`
	MaxBackoff        time.Duration `env:"ENGINE_JOB_MAX_BACKOFF" envDefault:"24h"`
	LongPollInterval  time.Duration `env:"ENGINE_JOB_LONG_POLL_INTERVAL" envDefault:"1s"`
	MaxLongPolling    int           `env:"ENGINE_JOB_MAX_LONG_POLLING" envDefault:"1024"`
}

// Recovery mengatur verifikasi proyeksi saat start.
type Recovery struct {
	// OnMismatch: fail (default), rebuild, atau warn.
	OnMismatch string        `env:"ENGINE_RECOVERY_ON_MISMATCH" envDefault:"fail"`
	DeepWindow time.Duration `env:"ENGINE_RECOVERY_DEEP_WINDOW" envDefault:"10m"`
	DeepSample int           `env:"ENGINE_RECOVERY_DEEP_SAMPLE" envDefault:"100"`
	Timeout    time.Duration `env:"ENGINE_RECOVERY_TIMEOUT" envDefault:"10m"`
}

// Outbox mengatur relay dan retensi outbox.
type Outbox struct {
	PollInterval time.Duration `env:"ENGINE_OUTBOX_POLL_INTERVAL" envDefault:"200ms"`
	BatchSize    int           `env:"ENGINE_OUTBOX_BATCH_SIZE" envDefault:"100"`
	Retention    time.Duration `env:"ENGINE_OUTBOX_RETENTION" envDefault:"24h"`
	MaxAttempts  int           `env:"ENGINE_OUTBOX_MAX_ATTEMPTS" envDefault:"10"`
}

// DB mengatur pool pgx dan timeout sesi. MaxConns 0 berarti max(4, 2*NumCPU).
type DB struct {
	MaxConns         int32         `env:"ENGINE_DB_MAX_CONNS" envDefault:"0"`
	MinConns         int32         `env:"ENGINE_DB_MIN_CONNS" envDefault:"2"`
	MaxConnLifetime  time.Duration `env:"ENGINE_DB_MAX_CONN_LIFETIME" envDefault:"30m"`
	MaxConnIdleTime  time.Duration `env:"ENGINE_DB_MAX_CONN_IDLE_TIME" envDefault:"5m"`
	StatementTimeout time.Duration `env:"ENGINE_DB_STATEMENT_TIMEOUT" envDefault:"10s"`
	LockTimeout      time.Duration `env:"ENGINE_DB_LOCK_TIMEOUT" envDefault:"5s"`
	IdleInTxTimeout  time.Duration `env:"ENGINE_DB_IDLE_IN_TX_TIMEOUT" envDefault:"15s"`
}

// DBConfig menggabungkan DATABASE_URL dan grup ENGINE_DB_* menjadi konfigurasi db.Connect.
func (c Config) DBConfig() db.Config {
	return db.Config{
		URL:              c.DatabaseURL,
		MaxConns:         c.DB.MaxConns,
		MinConns:         c.DB.MinConns,
		MaxConnLifetime:  c.DB.MaxConnLifetime,
		MaxConnIdleTime:  c.DB.MaxConnIdleTime,
		StatementTimeout: c.DB.StatementTimeout,
		LockTimeout:      c.DB.LockTimeout,
		IdleInTxTimeout:  c.DB.IdleInTxTimeout,
	}
}

// Load membaca konfigurasi dari environment.
func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}
