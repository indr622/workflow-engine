// Package sweeper menjalankan tugas pembersihan berkala berbasis batch FOR UPDATE SKIP LOCKED
// (lease job kedaluwarsa, message kedaluwarsa; timer di Fase 5). Aman dijalankan di banyak replika.
package sweeper

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/indr622/workflow-engine/apps/engine/internal/platform/metrics"
)

// Task adalah satu jenis pembersihan. Sweep memproses paling banyak batch baris dan mengembalikan
// jumlah yang diproses; Runner mengulang selama hasilnya sama dengan batch.
type Task interface {
	Name() string
	Sweep(ctx context.Context, batch int) (int, error)
}

// Defaults.
const (
	DefaultInterval = time.Second
	DefaultBatch    = 500
	// jitterFraction mencegah sweeper di beberapa replika berjalan serentak.
	jitterFraction = 0.2
	// maxRoundsPerTick membatasi satu tick agar satu task tidak memonopoli runner.
	maxRoundsPerTick = 100
)

// Runner menjalankan semua task setiap Interval (± 20 % jitter).
type Runner struct {
	Tasks    []Task
	Interval time.Duration
	Batch    int
	Log      *slog.Logger
}

// Run berjalan sampai ctx dibatalkan.
func (r *Runner) Run(ctx context.Context) {
	interval, batch, log := r.Interval, r.Batch, r.Log
	if interval <= 0 {
		interval = DefaultInterval
	}
	if batch <= 0 {
		batch = DefaultBatch
	}
	if log == nil {
		log = slog.Default()
	}
	timer := time.NewTimer(jitter(interval))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		for _, t := range r.Tasks {
			r.runTask(ctx, t, batch, log)
		}
		timer.Reset(jitter(interval))
	}
}

// RunOnce menjalankan setiap task sampai habis satu kali; dipakai test dan subperintah admin.
func (r *Runner) RunOnce(ctx context.Context) {
	batch := r.Batch
	if batch <= 0 {
		batch = DefaultBatch
	}
	log := r.Log
	if log == nil {
		log = slog.Default()
	}
	for _, t := range r.Tasks {
		r.runTask(ctx, t, batch, log)
	}
}

func (r *Runner) runTask(ctx context.Context, t Task, batch int, log *slog.Logger) {
	for round := 0; round < maxRoundsPerTick; round++ {
		n, err := t.Sweep(ctx, batch)
		if n > 0 {
			metrics.Get().SweeperSwept.Add(ctx, int64(n), metrics.Attr("task", t.Name()))
		}
		if err != nil {
			if ctx.Err() == nil {
				log.Warn("sweeper task failed", slog.String("task", t.Name()), slog.Any("error", err))
			}
			return
		}
		if n < batch {
			return
		}
	}
}

func jitter(d time.Duration) time.Duration {
	delta := float64(d) * jitterFraction
	return d + time.Duration((rand.Float64()*2-1)*delta) //nolint:gosec // jitter, bukan kriptografi
}
