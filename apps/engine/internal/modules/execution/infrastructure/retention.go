package infrastructure

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/application"
)

var _ application.RetentionStore = (*PostgresRepository)(nil)

// PruneExpired memenuhi application.RetentionStore. Instance yang sedang dikunci command dilewati
// (SKIP LOCKED); instance final tidak pernah menerima command, jadi persaingan hanya dengan admin.
func (r *PostgresRepository) PruneExpired(ctx context.Context, before time.Time, limit int, dryRun bool) ([]string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `SELECT key::text FROM process_instance
WHERE state <> 'ACTIVE' AND end_time < $1
ORDER BY end_time LIMIT $2 FOR UPDATE SKIP LOCKED`, before, limit)
	if err != nil {
		return nil, fmt.Errorf("pick expired instances: %w", err)
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil || len(keys) == 0 || dryRun {
		return keys, err
	}
	b := &pgx.Batch{}
	for _, table := range []string{"execution_record", "incident", "job", "variable", "taken_sequence_flow", "element_instance"} {
		b.Queue("DELETE FROM "+table+" WHERE process_instance_key = ANY($1::uuid[])", keys)
	}
	// execution_snapshot ikut terhapus lewat ON DELETE CASCADE.
	b.Queue("DELETE FROM process_instance WHERE key = ANY($1::uuid[])", keys)
	if err := tx.SendBatch(ctx, b).Close(); err != nil {
		return nil, fmt.Errorf("delete expired instances: %w", err)
	}
	return keys, tx.Commit(ctx)
}
