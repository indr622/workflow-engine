//go:build integration

package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func databaseURL(t *testing.T) string {
	t.Helper()
	u := os.Getenv("DATABASE_URL")
	if u == "" {
		t.Skip("DATABASE_URL not set")
	}
	return u
}

// TestConnectAndMigrate memastikan engine bisa terhubung ke Postgres sungguhan dan migrasi idempoten.
// Jalankan dengan: make test-integration (butuh DATABASE_URL yang menunjuk ke Postgres).
func TestConnectAndMigrate(t *testing.T) {
	url := databaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := Connect(ctx, ConfigFromURL(url))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	log := testLogger()
	for i := range 2 {
		if err := Migrate(ctx, pool, log); err != nil {
			t.Fatalf("migrate run %d: %v", i+1, err)
		}
	}

	var owner string
	if err := pool.QueryRow(ctx, "SELECT value FROM engine_metadata WHERE key = 'schema_owner'").Scan(&owner); err != nil {
		t.Fatalf("query metadata: %v", err)
	}
	if owner != "workflow-engine" {
		t.Fatalf("schema_owner = %q", owner)
	}
}

// TestRuntimeParamsApplied memastikan parameter sesi dari Config terpasang di setiap koneksi pool.
func TestRuntimeParamsApplied(t *testing.T) {
	cfg := ConfigFromURL(databaseURL(t))
	cfg.MaxConns = 3
	cfg.StatementTimeout = 1500 * time.Millisecond
	cfg.LockTimeout = 750 * time.Millisecond
	cfg.IdleInTxTimeout = 20 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := Connect(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	if got := pool.Config().MaxConns; got != 3 {
		t.Fatalf("MaxConns = %d, want 3", got)
	}
	want := map[string]string{
		"application_name":                    "engine",
		"statement_timeout":                   "1500ms",
		"lock_timeout":                        "750ms",
		"idle_in_transaction_session_timeout": "20s",
	}
	for name, value := range want {
		var got string
		if err := pool.QueryRow(ctx, "SHOW "+name).Scan(&got); err != nil {
			t.Fatalf("show %s: %v", name, err)
		}
		if got != value {
			t.Errorf("%s = %q, want %q", name, got, value)
		}
	}
}

// TestStatementTimeoutCancelsQuery membuktikan query yang melebihi statement_timeout dibatalkan
// server dengan SQLSTATE 57014 (query_canceled).
func TestStatementTimeoutCancelsQuery(t *testing.T) {
	cfg := ConfigFromURL(databaseURL(t))
	cfg.MaxConns = 2
	cfg.StatementTimeout = 200 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := Connect(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	start := time.Now()
	_, err = pool.Exec(ctx, "SELECT pg_sleep(5)")
	elapsed := time.Since(start)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("expected PgError, got %v", err)
	}
	if pgErr.Code != "57014" {
		t.Fatalf("SQLSTATE = %s (%s), want 57014", pgErr.Code, pgErr.Message)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("query ran %s, statement_timeout not enforced", elapsed)
	}
	// Pool tetap sehat setelah pembatalan.
	var one int
	if err := pool.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil || one != 1 {
		t.Fatalf("pool unusable after timeout: %v", err)
	}
}

// TestConcurrentMigrateOnEmptyDatabase mensimulasikan dua replika engine yang start bersamaan pada
// database kosong: keduanya harus selesai tanpa error dan berakhir di versi migrasi yang sama.
func TestConcurrentMigrateOnEmptyDatabase(t *testing.T) {
	adminURL := databaseURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	suffix := make([]byte, 6)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	dbName := "wf_migrate_test_" + hex.EncodeToString(suffix)

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{dbName}.Sanitize()); err != nil {
		t.Fatalf("create database: %v", err)
	}
	t.Cleanup(func() {
		dropCtx, dropCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer dropCancel()
		defer func() { _ = admin.Close(dropCtx) }()
		if _, err := admin.Exec(dropCtx, "DROP DATABASE IF EXISTS "+pgx.Identifier{dbName}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Errorf("drop database %s: %v", dbName, err)
		}
	})

	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + dbName
	testURL := u.String()

	const replicas = 2
	versions := make([]int64, replicas)
	errs := make([]error, replicas)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range replicas {
		wg.Go(func() {
			cfg := ConfigFromURL(testURL)
			cfg.MaxConns = 4
			pool, err := Connect(ctx, cfg)
			if err != nil {
				errs[i] = err
				return
			}
			defer pool.Close()
			<-start
			if err := Migrate(ctx, pool, testLogger()); err != nil {
				errs[i] = err
				return
			}
			errs[i] = pool.QueryRow(ctx, "SELECT max(version_id) FROM goose_db_version").Scan(&versions[i])
		})
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("replica %d: %v", i, err)
		}
	}
	if versions[0] == 0 || versions[0] != versions[1] {
		t.Fatalf("versions = %v, want equal and non-zero", versions)
	}
	// Setiap migrasi tercatat tepat sekali: tidak ada replika yang menjalankan ulang.
	var dup int
	check, err := Connect(ctx, ConfigFromURL(testURL))
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	if err := check.QueryRow(ctx,
		"SELECT count(*) FROM (SELECT version_id FROM goose_db_version WHERE is_applied GROUP BY version_id HAVING count(*) > 1) d",
	).Scan(&dup); err != nil {
		t.Fatal(err)
	}
	if dup != 0 {
		t.Fatalf("%d migration versions applied more than once", dup)
	}
}
