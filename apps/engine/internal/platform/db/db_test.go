package db

import (
	"testing"
	"time"
)

func TestPoolConfigDefaults(t *testing.T) {
	pc, err := poolConfig(ConfigFromURL("postgres://u:p@localhost:5432/db?sslmode=disable"))
	if err != nil {
		t.Fatal(err)
	}
	if pc.MaxConns != DefaultMaxConns() || pc.MaxConns < 4 {
		t.Errorf("MaxConns = %d, want %d (>= 4)", pc.MaxConns, DefaultMaxConns())
	}
	if pc.MinConns != DefaultMinConns {
		t.Errorf("MinConns = %d, want %d", pc.MinConns, DefaultMinConns)
	}
	if pc.MaxConnLifetime != 30*time.Minute || pc.MaxConnIdleTime != 5*time.Minute {
		t.Errorf("lifetime/idle = %s/%s", pc.MaxConnLifetime, pc.MaxConnIdleTime)
	}
	want := map[string]string{
		"application_name":                    "engine",
		"statement_timeout":                   "10000",
		"lock_timeout":                        "5000",
		"idle_in_transaction_session_timeout": "15000",
	}
	for k, v := range want {
		if got := pc.ConnConfig.RuntimeParams[k]; got != v {
			t.Errorf("RuntimeParams[%s] = %q, want %q", k, got, v)
		}
	}
}

func TestPoolConfigOverridesAndClamp(t *testing.T) {
	cfg := ConfigFromURL("postgres://u:p@localhost:5432/db")
	cfg.MaxConns = 3
	cfg.MinConns = 10 // lebih besar dari MaxConns: dipangkas
	cfg.StatementTimeout = 0
	pc, err := poolConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if pc.MaxConns != 3 || pc.MinConns != 3 {
		t.Errorf("Max/MinConns = %d/%d, want 3/3", pc.MaxConns, pc.MinConns)
	}
	if got := pc.ConnConfig.RuntimeParams["statement_timeout"]; got != "0" {
		t.Errorf("statement_timeout = %q, want 0 (disabled)", got)
	}
}

func TestPoolConfigInvalidURL(t *testing.T) {
	if _, err := poolConfig(Config{URL: "::not a url"}); err == nil {
		t.Fatal("expected error for invalid url")
	}
}
