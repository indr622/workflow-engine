package application

import (
	"context"

	executionapi "github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/metrics"
)

// SweepLeases mengembalikan job yang lease-nya lewat deadline ke antrean aktivasi lalu
// membangunkan long polling tipe job tersebut. Memenuhi sweeper.Task.
type SweepLeases struct {
	jobs     executionapi.Jobs
	notifier Notifier
}

// NewSweepLeases merakit task sweeper.
func NewSweepLeases(jobs executionapi.Jobs, notifier Notifier) *SweepLeases {
	return &SweepLeases{jobs: jobs, notifier: notifier}
}

// Name memenuhi sweeper.Task.
func (s *SweepLeases) Name() string { return "job_lease_timeouts" }

// Sweep memenuhi sweeper.Task.
func (s *SweepLeases) Sweep(ctx context.Context, batch int) (int, error) {
	released, err := s.jobs.ReleaseExpiredLeases(ctx, batch)
	if err != nil {
		return 0, err
	}
	types := map[string]bool{}
	for _, j := range released {
		types[j.Type] = true
	}
	for t := range types {
		s.notifier.Notify(t)
	}
	if len(released) > 0 {
		metrics.Get().JobTimeouts.Add(ctx, int64(len(released)))
	}
	return len(released), nil
}
