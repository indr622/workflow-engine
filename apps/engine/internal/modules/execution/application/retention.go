package application

import (
	"context"
	"log/slog"
	"time"
)

// RetentionStore menghapus instance final yang lebih tua dari batas (ADR-0019).
type RetentionStore interface {
	// PruneExpired mengunci (SKIP LOCKED) paling banyak limit instance final dengan end_time < before,
	// lalu menghapus semua tabel state, record, dan snapshot-nya; dryRun hanya mengembalikan key.
	PruneExpired(ctx context.Context, before time.Time, limit int, dryRun bool) ([]string, error)
}

// Retention adalah janitor riwayat.
type Retention struct {
	store RetentionStore
	log   *slog.Logger
	batch int
	pause time.Duration
}

// NewRetention membuat janitor retensi; batch default 500, pause default 200 ms.
func NewRetention(store RetentionStore, log *slog.Logger, batch int, pause time.Duration) *Retention {
	if batch <= 0 {
		batch = 500
	}
	if pause < 0 {
		pause = 0
	}
	if log == nil {
		log = slog.Default()
	}
	return &Retention{store: store, log: log, batch: batch, pause: pause}
}

// Prune menghapus instance final dengan end_time lebih tua dari olderThan, per batch dengan jeda
// agar tidak mengganggu command paralel. dryRun hanya menghitung batch pertama.
func (r *Retention) Prune(ctx context.Context, olderThan time.Duration, dryRun bool) (int, error) {
	before := time.Now().Add(-olderThan)
	total := 0
	started := time.Now()
	for {
		keys, err := r.store.PruneExpired(ctx, before, r.batch, dryRun)
		if err != nil {
			return total, err
		}
		total += len(keys)
		if dryRun || len(keys) < r.batch {
			break
		}
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		case <-time.After(r.pause):
		}
	}
	if total > 0 && !dryRun {
		r.log.Info("history pruned", slog.Int("instances", total), slog.Int64("duration_ms", time.Since(started).Milliseconds()))
	}
	return total, nil
}
