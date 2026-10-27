// Package keygen menghasilkan key int64 unik global untuk semua entity engine
// (deployment, process definition, instance, job) dari sequence PostgreSQL.
package keygen

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Generator menghasilkan key baru.
type Generator interface {
	// NextKeys mengembalikan n key berurutan naik dalam satu round trip.
	NextKeys(ctx context.Context, n int) ([]int64, error)
}

// Querier adalah bagian pgx yang dibutuhkan: pool maupun transaksi memenuhinya.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// Postgres mengambil key dari sequence engine_key_seq.
type Postgres struct {
	q Querier
}

// NewPostgres membuat generator di atas pool atau transaksi.
func NewPostgres(q Querier) *Postgres { return &Postgres{q: q} }

// NextKeys memanggil nextval sebanyak n kali dalam satu query.
func (p *Postgres) NextKeys(ctx context.Context, n int) ([]int64, error) {
	if n <= 0 {
		return nil, nil
	}
	rows, err := p.q.Query(ctx, "SELECT nextval('engine_key_seq') FROM generate_series(1, $1)", n)
	if err != nil {
		return nil, fmt.Errorf("next keys: %w", err)
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return nil, fmt.Errorf("collect keys: %w", err)
	}
	return keys, nil
}
