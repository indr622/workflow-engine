// Package infrastructure berisi adapter keluar modul health.
package infrastructure

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresProbe memeriksa koneksi ke PostgreSQL lewat ping pada pool.
type PostgresProbe struct {
	pool *pgxpool.Pool
}

// NewPostgresProbe membuat probe untuk pool yang diberikan.
func NewPostgresProbe(pool *pgxpool.Pool) *PostgresProbe {
	return &PostgresProbe{pool: pool}
}

// Name mengembalikan nama dependensi.
func (p *PostgresProbe) Name() string { return "postgres" }

// Probe melakukan ping ke database.
func (p *PostgresProbe) Probe(ctx context.Context) error {
	return p.pool.Ping(ctx)
}
