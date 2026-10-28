package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/application"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/domain"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/infrastructure/db"
)

// defaultTenant sama dengan tenant default modul deployment; multi-tenant menyusul di Fase 8.
const defaultTenant = "<default>"

var _ application.ReadModel = (*PostgresRepository)(nil)

// GetInstance memenuhi application.ReadModel.
func (r *PostgresRepository) GetInstance(ctx context.Context, key string) (application.InstanceDetail, error) {
	pi, err := r.q.GetProcessInstanceRow(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.InstanceDetail{}, domain.ErrInstanceNotFound
	}
	if err != nil {
		return application.InstanceDetail{}, fmt.Errorf("get instance %s: %w", key, err)
	}
	d := application.InstanceDetail{Instance: summary(pi.Key, pi.ProcessDefinitionKey, pi.BpmnProcessID,
		pi.ProcessDefinitionVersion, pi.State, pi.HasIncident, pi.StartTime, pi.EndTime)}

	elements, err := r.q.ListElementInstances(ctx, key)
	if err != nil {
		return d, fmt.Errorf("list elements: %w", err)
	}
	if len(elements) > application.MaxElementsInDetail {
		elements, d.ElementsTruncated = elements[:application.MaxElementsInDetail], true
	}
	for _, e := range elements {
		d.Elements = append(d.Elements, application.ElementInstanceView{
			Key: e.Key, FlowScopeKey: deref(e.FlowScopeKey), ElementID: e.ElementID, BpmnElementType: e.BpmnElementType,
			State: e.State, IncidentKey: deref(e.IncidentKey), StartTime: e.StartTime.Time, EndTime: optTime(e.EndTime),
		})
	}
	vars, err := r.q.ListVariables(ctx, key)
	if err != nil {
		return d, fmt.Errorf("list variables: %w", err)
	}
	for _, v := range vars {
		d.Variables = append(d.Variables, application.VariableView{ScopeKey: v.ScopeKey, Name: v.Name, Value: json.RawMessage(v.Value)})
	}
	incidents, err := r.q.ListIncidents(ctx, key)
	if err != nil {
		return d, fmt.Errorf("list incidents: %w", err)
	}
	for _, inc := range incidents {
		d.Incidents = append(d.Incidents, application.IncidentView{
			Key: inc.Key, ElementInstanceKey: inc.ElementInstanceKey, ElementID: inc.ElementID, ErrorType: inc.ErrorType,
			ErrorMessage: inc.ErrorMessage, Resolved: inc.State == "RESOLVED", CreatedAt: inc.CreatedAt.Time,
		})
	}
	jobs, err := r.q.ListJobsByInstance(ctx, key)
	if err != nil {
		return d, fmt.Errorf("list jobs: %w", err)
	}
	for _, j := range jobs {
		view, err := jobView(j.Key, j.Type, j.ProcessInstanceKey, j.ElementInstanceKey, j.ElementID, j.BpmnProcessID,
			j.ProcessDefinitionKey, j.Retries, j.CustomHeaders, j.State, j.CreatedAt)
		if err != nil {
			return d, err
		}
		d.Jobs = append(d.Jobs, view)
	}
	return d, nil
}

// ListInstances memenuhi application.ReadModel.
func (r *PostgresRepository) ListInstances(ctx context.Context, f application.InstanceFilter) ([]application.InstanceSummary, error) {
	rows, err := r.q.ListProcessInstances(ctx, db.ListProcessInstancesParams{
		TenantID: defaultTenant, BpmnProcessID: f.BpmnProcessID, ProcessDefinitionKey: f.ProcessDefinitionKey,
		State: f.State, OnlyWithIncident: f.OnlyWithIncident, AfterKey: f.AfterKey, PageLimit: f.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list instances: %w", err)
	}
	out := make([]application.InstanceSummary, 0, len(rows))
	for _, pi := range rows {
		out = append(out, summary(pi.Key, pi.ProcessDefinitionKey, pi.BpmnProcessID,
			pi.ProcessDefinitionVersion, pi.State, pi.HasIncident, pi.StartTime, pi.EndTime))
	}
	return out, nil
}

// ListJobs memenuhi application.ReadModel.
func (r *PostgresRepository) ListJobs(ctx context.Context, f application.JobFilter) ([]application.JobView, error) {
	rows, err := r.q.ListJobs(ctx, db.ListJobsParams{
		JobType: f.Type, ProcessInstanceKey: f.ProcessInstanceKey, OnlyOpen: f.OnlyOpen, JobState: f.State,
		AfterKey: f.AfterKey, PageLimit: f.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	out := make([]application.JobView, 0, len(rows))
	for _, j := range rows {
		view, err := jobView(j.Key, j.Type, j.ProcessInstanceKey, j.ElementInstanceKey, j.ElementID, j.BpmnProcessID,
			j.ProcessDefinitionKey, j.Retries, j.CustomHeaders, j.State, j.CreatedAt)
		if err != nil {
			return nil, err
		}
		view.Worker, view.ErrorMessage, view.ErrorCode = deref(j.Worker), deref(j.ErrorMessage), deref(j.ErrorCode)
		view.Deadline, view.ActivatableAt, view.ActivationCount = optTime(j.Deadline), optTime(j.ActivatableAt), j.ActivationCount
		out = append(out, view)
	}
	return out, nil
}

// ListIncidents memenuhi application.ReadModel.
func (r *PostgresRepository) ListIncidents(ctx context.Context, f application.IncidentFilter) ([]application.IncidentInfo, error) {
	rows, err := r.q.ListIncidentsPage(ctx, db.ListIncidentsPageParams{
		ProcessInstanceKey: f.ProcessInstanceKey, ErrorType: f.ErrorType, OnlyOpen: f.OnlyOpen,
		AfterKey: f.AfterKey, PageLimit: f.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list incidents: %w", err)
	}
	out := make([]application.IncidentInfo, 0, len(rows))
	for _, i := range rows {
		out = append(out, application.IncidentInfo{
			Key: i.Key, ProcessInstanceKey: i.ProcessInstanceKey, ElementInstanceKey: i.ElementInstanceKey,
			ElementID: i.ElementID, BpmnProcessID: i.BpmnProcessID, ProcessDefinitionKey: i.ProcessDefinitionKey,
			ErrorType: i.ErrorType, ErrorMessage: i.ErrorMessage, JobKey: deref(i.JobKey), State: i.State,
			CreatedAt: i.CreatedAt.Time, ResolvedAt: optTime(i.ResolvedAt),
		})
	}
	return out, nil
}

func summary(key, defKey, processID string, version int32, state string, hasIncident bool,
	start, end pgtype.Timestamptz) application.InstanceSummary {
	return application.InstanceSummary{
		Key: key, ProcessDefinitionKey: defKey, BpmnProcessID: processID, Version: version, State: state,
		HasIncident: hasIncident, StartTime: start.Time, EndTime: optTime(end),
	}
}

func jobView(key, typ, piKey, elemKey, elemID, processID, defKey string, retries int32, headers []byte,
	state string, created pgtype.Timestamptz) (application.JobView, error) {
	var hs []application.Header
	if err := json.Unmarshal(headers, &hs); err != nil {
		return application.JobView{}, fmt.Errorf("job %s headers: %w", key, err)
	}
	return application.JobView{
		Key: key, Type: typ, ProcessInstanceKey: piKey, ElementInstanceKey: elemKey, ElementID: elemID,
		BpmnProcessID: processID, ProcessDefinitionKey: defKey, Retries: retries, CustomHeaders: hs,
		State: state, CreatedAt: created.Time,
	}, nil
}
