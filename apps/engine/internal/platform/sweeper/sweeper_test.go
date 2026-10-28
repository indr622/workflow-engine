package sweeper_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/indr622/workflow-engine/apps/engine/internal/platform/sweeper"
)

// countdown mengembalikan batch penuh sampai sisa habis.
type countdown struct {
	left  int
	calls int
	err   error
}

func (c *countdown) Name() string { return "countdown" }

func (c *countdown) Sweep(_ context.Context, batch int) (int, error) {
	c.calls++
	if c.err != nil {
		return 0, c.err
	}
	n := min(batch, c.left)
	c.left -= n
	return n, nil
}

func TestRunOnceRepeatsUntilBatchIsNotFull(t *testing.T) {
	task := &countdown{left: 1250}
	(&sweeper.Runner{Tasks: []sweeper.Task{task}, Batch: 500}).RunOnce(context.Background())
	if task.left != 0 || task.calls != 3 {
		t.Fatalf("left=%d calls=%d, want 0 and 3", task.left, task.calls)
	}
}

func TestRunOnceStopsOnError(t *testing.T) {
	task := &countdown{left: 10, err: errors.New("db down")}
	(&sweeper.Runner{Tasks: []sweeper.Task{task}}).RunOnce(context.Background())
	if task.calls != 1 {
		t.Fatalf("calls=%d, want 1", task.calls)
	}
}

type ticks struct{ n atomic.Int32 }

func (t *ticks) Name() string { return "ticks" }

func (t *ticks) Sweep(context.Context, int) (int, error) {
	t.n.Add(1)
	return 0, nil
}

func TestRunTicksUntilCanceled(t *testing.T) {
	task := &ticks{}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	(&sweeper.Runner{Tasks: []sweeper.Task{task}, Interval: 20 * time.Millisecond}).Run(ctx)
	if n := task.n.Load(); n < 5 {
		t.Fatalf("ran %d times in 300ms with a 20ms interval", n)
	}
}
