// Package db menyediakan connection pool PostgreSQL dan runner migrasi.
package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"runtime"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Nilai default pool dan timeout sesi (rencana Fase 3, 9.1). MaxConns default dihitung dari CPU.
const (
	DefaultMinConns         int32 = 2
	DefaultMaxConnLifetime        = 30 * time.Minute
	DefaultMaxConnIdleTime        = 5 * time.Minute
	DefaultStatementTimeout       = 10 * time.Second
	DefaultLockTimeout            = 5 * time.Second
	DefaultIdleInTxTimeout        = 15 * time.Second

	applicationName = "engine"
)

// Config mengatur pool pgx dan timeout sesi Postgres. Nilai nol pada MaxConns berarti
// max(4, 2*NumCPU); nilai nol pada timeout berarti timeout dinonaktifkan (perilaku Postgres).
type Config struct {
	URL              string
	MaxConns         int32
	MinConns         int32
	MaxConnLifetime  time.Duration
	MaxConnIdleTime  time.Duration
	StatementTimeout time.Duration
	LockTimeout      time.Duration
	IdleInTxTimeout  time.Duration
}

// ConfigFromURL mengembalikan Config dengan seluruh default untuk pemanggil yang hanya punya URL
// (test integrasi, tool).
func ConfigFromURL(url string) Config {
	return Config{
		URL:              url,
		MinConns:         DefaultMinConns,
		MaxConnLifetime:  DefaultMaxConnLifetime,
		MaxConnIdleTime:  DefaultMaxConnIdleTime,
		StatementTimeout: DefaultStatementTimeout,
		LockTimeout:      DefaultLockTimeout,
		IdleInTxTimeout:  DefaultIdleInTxTimeout,
	}
}

// DefaultMaxConns adalah ukuran pool bila ENGINE_DB_MAX_CONNS tidak diisi: max(4, 2*NumCPU).
func DefaultMaxConns() int32 {
	return int32(max(4, 2*runtime.NumCPU())) //nolint:gosec // jumlah CPU jauh di bawah batas int32
}

// poolConfig menerjemahkan Config menjadi konfigurasi pgxpool tanpa membuka koneksi.
func poolConfig(cfg Config) (*pgxpool.Config, error) {
	pc, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	maxConns := cfg.MaxConns
	if maxConns <= 0 {
		maxConns = DefaultMaxConns()
	}
	minConns := min(max(cfg.MinConns, 0), maxConns)
	pc.MaxConns, pc.MinConns = maxConns, minConns
	if cfg.MaxConnLifetime > 0 {
		pc.MaxConnLifetime = cfg.MaxConnLifetime
	}
	if cfg.MaxConnIdleTime > 0 {
		pc.MaxConnIdleTime = cfg.MaxConnIdleTime
	}
	params := pc.ConnConfig.RuntimeParams
	params["application_name"] = applicationName
	params["statement_timeout"] = ms(cfg.StatementTimeout)
	params["lock_timeout"] = ms(cfg.LockTimeout)
	params["idle_in_transaction_session_timeout"] = ms(cfg.IdleInTxTimeout)
	return pc, nil
}

// ms memformat durasi sebagai milidetik untuk parameter sesi Postgres ("0" = nonaktif).
func ms(d time.Duration) string {
	return strconv.FormatInt(max(d.Milliseconds(), 0), 10)
}

// Connect membuat pool pgx dengan pengaturan pool dan timeout sesi, lalu memastikan database
// bisa dijangkau.
func Connect(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	pc, err := poolConfig(cfg)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// Migrate menjalankan seluruh migrasi goose yang di-embed ke binary. Migrasi berjalan di bawah
// session lock Postgres (pg_try_advisory_lock dengan retry) sehingga replika yang start bersamaan
// menunggu, lalu melihat versi sudah terbaru.
//
// Koneksi migrasi dibuka terpisah dari pool dengan statement_timeout dan
// idle_in_transaction_session_timeout dimatikan: DDL seperti CREATE INDEX CONCURRENTLY boleh
// berjalan lebih lama dari batas query OLTP. lock_timeout tetap berlaku agar DDL tidak menahan
// antrean lock tabel tanpa batas.
func Migrate(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
	connCfg := pool.Config().ConnConfig.Copy()
	connCfg.RuntimeParams["statement_timeout"] = "0"
	connCfg.RuntimeParams["idle_in_transaction_session_timeout"] = "0"
	connCfg.RuntimeParams["application_name"] = applicationName + "-migrate"
	sqlDB := stdlib.OpenDB(*connCfg)
	defer func() { _ = sqlDB.Close() }()

	fsys, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("open embedded migrations: %w", err)
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("create migration locker: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, fsys, goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("create migration provider: %w", err)
	}
	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	version, err := provider.GetDBVersion(ctx)
	if err != nil {
		return fmt.Errorf("read migration version: %w", err)
	}
	log.Info("database migrated", slog.Int64("version", version), slog.Int("applied", len(results)))
	return nil
}
