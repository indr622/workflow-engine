package application

import (
	"context"
	"fmt"
	"time"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/domain"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/metrics"
)

// Use case protokol job worker dan operator (Fase 4). Command yang menulis record berjalan lewat
// runOnInstance (lock instance, klaim request_id, Processor); lease berjalan lewat LeaseStore tanpa
// lock instance.

var (
	_ api.Incidents = (*Commands)(nil)
	_ api.Variables = (*Commands)(nil)
)

// ActivateJobs me-lease job bertipe q.Type. Parameter sudah dipotong ke batas server oleh modul job.
func (c *Commands) ActivateJobs(ctx context.Context, q api.ActivationQuery) ([]api.ActivatedJob, error) {
	defer metrics.Since(ctx, metrics.Get().CommandDuration, time.Now(), metrics.Attr("command", "activate jobs"))
	jobs, err := c.uow.ActivateJobs(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("activate jobs: %w", err)
	}
	return jobs, nil
}

// FailJob menetapkan retries tersisa job; retries 0 membuat incident JOB_NO_RETRIES.
func (c *Commands) FailJob(ctx context.Context, f api.JobFailure) (bool, error) {
	vars, err := domain.ParseVariables(f.Variables, c.cfg.MaxVariablesBytes)
	if err != nil {
		return false, err
	}
	return c.runOnJob(ctx, "fail job", f.JobKey, f.Idempotency, domain.FailJob{
		JobKey: f.JobKey, Retries: f.Retries, RetryBackoff: f.RetryBackoff, ErrorMessage: f.ErrorMessage, Variables: vars,
	})
}

// ThrowError melempar BPMN error dari job.
func (c *Commands) ThrowError(ctx context.Context, e api.JobError) (bool, error) {
	vars, err := domain.ParseVariables(e.Variables, c.cfg.MaxVariablesBytes)
	if err != nil {
		return false, err
	}
	return c.runOnJob(ctx, "throw error", e.JobKey, e.Idempotency, domain.ThrowError{
		JobKey: e.JobKey, ErrorCode: e.ErrorCode, ErrorMessage: e.ErrorMessage, Variables: vars,
	})
}

// UpdateJobRetries mengubah retries job yang belum final.
func (c *Commands) UpdateJobRetries(ctx context.Context, jobKey string, retries int32, idem *api.Idempotency) (bool, error) {
	return c.runOnJob(ctx, "update job retries", jobKey, idem, domain.UpdateJobRetries{JobKey: jobKey, Retries: retries})
}

// UpdateJobTimeout mengubah deadline lease job ACTIVATED.
func (c *Commands) UpdateJobTimeout(ctx context.Context, jobKey string, timeout time.Duration) error {
	if err := c.uow.ExtendLease(ctx, jobKey, timeout); err != nil {
		return fmt.Errorf("update job timeout: %w", err)
	}
	return nil
}

// ReleaseExpiredLeases mengembalikan job yang lease-nya kedaluwarsa ke antrean aktivasi.
func (c *Commands) ReleaseExpiredLeases(ctx context.Context, limit int) ([]api.ReleasedJob, error) {
	released, err := c.uow.ReleaseExpiredLeases(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("release expired leases: %w", err)
	}
	return released, nil
}

func (c *Commands) runOnJob(ctx context.Context, op, jobKey string, idem *api.Idempotency, cmd domain.Command) (bool, error) {
	piKey, defKey, err := c.uow.JobRef(ctx, jobKey)
	if err != nil {
		return false, fmt.Errorf("%s: %w", op, err)
	}
	proc, err := c.process(ctx, defKey)
	if err != nil {
		return false, fmt.Errorf("%s: %w", op, err)
	}
	return c.runOnInstance(ctx, op, piKey, proc, idem, cmd)
}

// ResolveIncident menyelesaikan incident lalu menjalankan ulang langkah yang gagal. Incident yang
// sudah diselesaikan ditolak domain.ErrIncidentAlreadyResolved setelah klaim request_id diperiksa,
// sehingga ulangan request yang sudah sukses tetap mendapat replay.
func (c *Commands) ResolveIncident(ctx context.Context, incidentKey string, idem *api.Idempotency) (bool, error) {
	piKey, defKey, err := c.uow.IncidentRef(ctx, incidentKey)
	if err != nil {
		return false, fmt.Errorf("resolve incident: %w", err)
	}
	proc, err := c.process(ctx, defKey)
	if err != nil {
		return false, fmt.Errorf("resolve incident: %w", err)
	}
	return c.runOnInstance(ctx, "resolve incident", piKey, proc, idem, domain.ResolveIncident{IncidentKey: incidentKey})
}

// SetVariables menulis variabel ke scope process instance pemilik scopeKey.
func (c *Commands) SetVariables(ctx context.Context, scopeKey string, variables []byte, local bool, idem *api.Idempotency) (bool, error) {
	vars, err := domain.ParseVariables(variables, c.cfg.MaxVariablesBytes)
	if err != nil {
		return false, err
	}
	if len(vars) == 0 {
		return false, fmt.Errorf("%w: at least one variable is required", domain.ErrInvalidVariables)
	}
	piKey, defKey, err := c.uow.ScopeRef(ctx, scopeKey)
	if err != nil {
		return false, fmt.Errorf("set variables: %w", err)
	}
	proc, err := c.process(ctx, defKey)
	if err != nil {
		return false, fmt.Errorf("set variables: %w", err)
	}
	return c.runOnInstance(ctx, "set variables", piKey, proc, idem, domain.SetVariables{ScopeKey: scopeKey, Variables: vars, Local: local})
}
