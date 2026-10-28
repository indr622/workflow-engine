// Package infrastructure berisi repository PostgreSQL buffer message.
package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/message/domain"
)

// PostgresRepository mengimplementasikan domain.Repository.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

var _ domain.Repository = (*PostgresRepository)(nil)

// NewPostgresRepository membuat repository.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// Insert memenuhi domain.Repository. Duplikat yang sudah kedaluwarsa tetapi belum disapu sweeper
// dihapus dulu, sehingga dedup hanya berlaku selama message masih di buffer.
func (r *PostgresRepository) Insert(ctx context.Context, m domain.Message) (time.Time, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return time.Time{}, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var messageID *string
	if m.MessageID != "" {
		messageID = &m.MessageID
		if _, err := tx.Exec(ctx, `DELETE FROM message
WHERE tenant_id = $1 AND name = $2 AND correlation_key = $3 AND message_id = $4 AND expires_at <= now()`,
			m.TenantID, m.Name, m.CorrelationKey, m.MessageID); err != nil {
			return time.Time{}, fmt.Errorf("drop expired duplicate: %w", err)
		}
	}
	var expiresAt time.Time
	err = tx.QueryRow(ctx, `INSERT INTO message
    (key, tenant_id, name, correlation_key, message_id, variables, published_at, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, now(), now() + make_interval(secs => $7))
ON CONFLICT (tenant_id, name, correlation_key, message_id) WHERE message_id IS NOT NULL DO NOTHING
RETURNING expires_at`, m.Key, m.TenantID, m.Name, m.CorrelationKey, messageID, string(m.Variables), m.TTL.Seconds()).Scan(&expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		var existing string
		if err := tx.QueryRow(ctx, `SELECT key::text FROM message
WHERE tenant_id = $1 AND name = $2 AND correlation_key = $3 AND message_id = $4`,
			m.TenantID, m.Name, m.CorrelationKey, m.MessageID).Scan(&existing); err != nil {
			return time.Time{}, fmt.Errorf("read duplicate message: %w", err)
		}
		return time.Time{}, domain.DuplicateOf(existing)
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("insert message: %w", err)
	}
	return expiresAt, tx.Commit(ctx)
}

// DeleteExpired memenuhi domain.Repository.
func (r *PostgresRepository) DeleteExpired(ctx context.Context, batch int) (int, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM message WHERE key IN (
    SELECT key FROM message WHERE expires_at <= now() ORDER BY expires_at LIMIT $1 FOR UPDATE SKIP LOCKED)`, batch)
	if err != nil {
		return 0, fmt.Errorf("delete expired messages: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
