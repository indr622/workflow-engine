package config

import (
	"testing"
	"time"
)

func TestLoadDBDefaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := DB{
		MaxConns:         0,
		MinConns:         2,
		MaxConnLifetime:  30 * time.Minute,
		MaxConnIdleTime:  5 * time.Minute,
		StatementTimeout: 10 * time.Second,
		LockTimeout:      5 * time.Second,
		IdleInTxTimeout:  15 * time.Second,
	}
	if cfg.DB != want {
		t.Fatalf("DB = %+v, want %+v", cfg.DB, want)
	}
}

func TestLoadDBOverrides(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://x@db:5432/x")
	t.Setenv("ENGINE_DB_MAX_CONNS", "8")
	t.Setenv("ENGINE_DB_STATEMENT_TIMEOUT", "2s")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	dc := cfg.DBConfig()
	if dc.URL != "postgres://x@db:5432/x" || dc.MaxConns != 8 || dc.StatementTimeout != 2*time.Second {
		t.Fatalf("DBConfig = %+v", dc)
	}
}
