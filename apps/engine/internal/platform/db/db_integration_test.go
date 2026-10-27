//go:build integration

package db

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"
)

// TestConnectAndMigrate memastikan engine bisa terhubung ke Postgres sungguhan dan migrasi idempoten.
// Jalankan dengan: make test-integration (butuh DATABASE_URL yang menunjuk ke Postgres).
func TestConnectAndMigrate(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
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
