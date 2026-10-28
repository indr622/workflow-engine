// Package application berisi use case modul job: long polling ActivateJobs dan sweeper lease di
// atas execution/api.Jobs. Modul job tidak punya tabel sendiri (ADR-0020).
package application

import (
	"context"
	"sync/atomic"
	"time"

	executionapi "github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/job/domain"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/metrics"
)

// Notifier membangunkan long polling saat job bertipe tertentu mungkin dapat diaktifkan.
type Notifier interface {
	// Subscribe mendaftar untuk tipe job; channel berkapasitas 1 (sinyal digabung). cancel wajib
	// dipanggil.
	Subscribe(jobType string) (signal <-chan struct{}, cancel func())
	Notify(jobType string)
}

// ActivateJobs menjalankan long polling: coba aktifkan → tunggu notifikasi/polling cadangan →
// coba lagi, sampai ada job, request_timeout habis, klien putus, atau engine berhenti.
type ActivateJobs struct {
	jobs     executionapi.Jobs
	notifier Notifier
	limits   domain.Limits
	active   atomic.Int64
	// stopping ditutup saat engine shutdown agar semua long polling dijawab kosong segera.
	stopping <-chan struct{}
}

// NewActivateJobs merakit use case. stopping boleh nil.
func NewActivateJobs(jobs executionapi.Jobs, notifier Notifier, limits domain.Limits, stopping <-chan struct{}) *ActivateJobs {
	return &ActivateJobs{jobs: jobs, notifier: notifier, limits: limits.WithDefaults(), stopping: stopping}
}

// Limits mengembalikan batas yang dipakai (untuk klem RPC lain di handler).
func (a *ActivateJobs) Limits() domain.Limits { return a.limits }

// Execute mengaktifkan satu batch job. Respons kosong bukan error.
func (a *ActivateJobs) Execute(ctx context.Context, req domain.ActivationRequest) ([]executionapi.ActivatedJob, error) {
	req = req.Clamp(a.limits)
	q := executionapi.ActivationQuery{
		Type: req.Type, Worker: req.Worker, MaxJobs: req.MaxJobs, Timeout: req.Timeout, FetchVariables: req.FetchVariables,
	}
	if req.RequestTimeout <= 0 || a.active.Load() >= int64(a.limits.MaxLongPolls) {
		return a.activate(ctx, q)
	}
	// Daftar SEBELUM kueri pertama: job yang dibuat di antara kueri dan tunggu tetap membangunkan.
	signal, cancel := a.notifier.Subscribe(req.Type)
	defer cancel()
	a.active.Add(1)
	metrics.Get().LongPollsActive.Add(ctx, 1)
	defer func() {
		a.active.Add(-1)
		metrics.Get().LongPollsActive.Add(context.WithoutCancel(ctx), -1)
	}()

	deadline := time.NewTimer(req.RequestTimeout)
	defer deadline.Stop()
	poll := time.NewTicker(a.limits.PollInterval)
	defer poll.Stop()
	for {
		jobs, err := a.activate(ctx, q)
		if err != nil || len(jobs) > 0 {
			return jobs, err
		}
		select {
		case <-ctx.Done():
			// Klien putus atau batas RPC habis: berhenti tanpa kueri lagi.
			return nil, nil
		case <-a.stopping:
			return nil, nil
		case <-deadline.C:
			return nil, nil
		case <-signal:
		case <-poll.C:
		}
	}
}

func (a *ActivateJobs) activate(ctx context.Context, q executionapi.ActivationQuery) ([]executionapi.ActivatedJob, error) {
	jobs, err := a.jobs.ActivateJobs(ctx, q)
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil
		}
		return nil, err
	}
	if len(jobs) > 0 {
		metrics.Get().JobsActivated.Add(ctx, int64(len(jobs)), metrics.Attr("job_type", q.Type))
	}
	return jobs, nil
}

// LongPollsActive mengembalikan jumlah long polling yang sedang menunggu.
func (a *ActivateJobs) LongPollsActive() int64 { return a.active.Load() }
