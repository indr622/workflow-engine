// Package interfaces berisi handler Connect JobService: protokol job worker (ADR-0020). Handler ini
// dipindah dari modul execution di Fase 4 karena ActivateJobs membutuhkan notifier dan klem milik
// modul job; operasi lain adalah panggilan tipis ke execution/api.
package interfaces

import (
	"context"
	"errors"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	executionapi "github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/job/application"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/job/domain"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/idempotency"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/keygen"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/rpc"
	jobv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1/jobv1connect"
)

// ReplayedHeader menandai response yang diambil dari klaim request_id sebelumnya (ADR-0017).
const ReplayedHeader = "Idempotent-Replayed"

// Handler mengimplementasikan JobService.
type Handler struct {
	activate *application.ActivateJobs
	jobs     executionapi.Jobs
	queries  executionapi.JobQueries
}

var _ jobv1connect.JobServiceHandler = (*Handler)(nil)

// NewHandler membuat handler.
func NewHandler(activate *application.ActivateJobs, jobs executionapi.Jobs, queries executionapi.JobQueries) *Handler {
	return &Handler{activate: activate, jobs: jobs, queries: queries}
}

// Register memasang JobService ke mux dengan opsi rantai interceptor bersama.
func (h *Handler) Register(mux *http.ServeMux, opts ...connect.HandlerOption) {
	mux.Handle(jobv1connect.NewJobServiceHandler(h, opts...))
}

// ActivateJobs mengimplementasikan RPC ActivateJobs.
func (h *Handler) ActivateJobs(ctx context.Context, req *connect.Request[jobv1.ActivateJobsRequest]) (*connect.Response[jobv1.ActivateJobsResponse], error) {
	m := req.Msg
	jobs, err := h.activate.Execute(ctx, domain.ActivationRequest{
		Type: m.GetType(), Worker: m.GetWorker(), MaxJobs: m.GetMaxJobsToActivate(), Timeout: m.GetTimeout().AsDuration(),
		FetchVariables: m.GetFetchVariable(), RequestTimeout: m.GetRequestTimeout().AsDuration(),
	})
	if err != nil {
		return nil, rpc.ToConnect(err)
	}
	out := &jobv1.ActivateJobsResponse{Jobs: make([]*jobv1.ActivatedJob, 0, len(jobs))}
	for _, j := range jobs {
		out.Jobs = append(out.Jobs, &jobv1.ActivatedJob{
			JobKey: j.Key, Type: j.Type, ProcessInstanceKey: j.ProcessInstanceKey, BpmnProcessId: j.BpmnProcessID,
			ProcessDefinitionVersion: j.ProcessDefinitionVersion, ProcessDefinitionKey: j.ProcessDefinitionKey,
			ElementId: j.ElementID, ElementInstanceKey: j.ElementInstanceKey, CustomHeaders: headerMap(j.CustomHeaders),
			Worker: j.Worker, Retries: j.Retries, Deadline: timestamppb.New(j.Deadline), Variables: string(j.Variables),
			TenantId: rpc.FromContext(ctx).Tenant,
		})
	}
	return connect.NewResponse(out), nil
}

// CompleteJob mengimplementasikan RPC CompleteJob.
func (h *Handler) CompleteJob(ctx context.Context, req *connect.Request[jobv1.CompleteJobRequest]) (*connect.Response[jobv1.CompleteJobResponse], error) {
	idem, err := idempotencyOf(ctx, jobv1connect.JobServiceCompleteJobProcedure, req.Msg, req.Msg.GetRequestId(),
		func(m proto.Message) { m.(*jobv1.CompleteJobRequest).RequestId = "" })
	if err != nil {
		return nil, err
	}
	replayed, err := h.jobs.CompleteJob(ctx, req.Msg.GetJobKey(), []byte(req.Msg.GetVariables()), idem)
	return replayedResponse(&jobv1.CompleteJobResponse{}, replayed, err)
}

// FailJob mengimplementasikan RPC FailJob.
func (h *Handler) FailJob(ctx context.Context, req *connect.Request[jobv1.FailJobRequest]) (*connect.Response[jobv1.FailJobResponse], error) {
	m := req.Msg
	idem, err := idempotencyOf(ctx, jobv1connect.JobServiceFailJobProcedure, m, m.GetRequestId(),
		func(m proto.Message) { m.(*jobv1.FailJobRequest).RequestId = "" })
	if err != nil {
		return nil, err
	}
	replayed, err := h.jobs.FailJob(ctx, executionapi.JobFailure{
		JobKey: m.GetJobKey(), Retries: m.GetRetries(), ErrorMessage: m.GetErrorMessage(),
		RetryBackoff: h.activate.Limits().ClampBackoff(m.GetRetryBackOff().AsDuration()),
		Variables:    []byte(m.GetVariables()), Idempotency: idem,
	})
	return replayedResponse(&jobv1.FailJobResponse{}, replayed, err)
}

// ThrowError mengimplementasikan RPC ThrowError.
func (h *Handler) ThrowError(ctx context.Context, req *connect.Request[jobv1.ThrowErrorRequest]) (*connect.Response[jobv1.ThrowErrorResponse], error) {
	m := req.Msg
	idem, err := idempotencyOf(ctx, jobv1connect.JobServiceThrowErrorProcedure, m, m.GetRequestId(),
		func(m proto.Message) { m.(*jobv1.ThrowErrorRequest).RequestId = "" })
	if err != nil {
		return nil, err
	}
	replayed, err := h.jobs.ThrowError(ctx, executionapi.JobError{
		JobKey: m.GetJobKey(), ErrorCode: m.GetErrorCode(), ErrorMessage: m.GetErrorMessage(),
		Variables: []byte(m.GetVariables()), Idempotency: idem,
	})
	return replayedResponse(&jobv1.ThrowErrorResponse{}, replayed, err)
}

// UpdateJobRetries mengimplementasikan RPC UpdateJobRetries.
func (h *Handler) UpdateJobRetries(ctx context.Context, req *connect.Request[jobv1.UpdateJobRetriesRequest]) (*connect.Response[jobv1.UpdateJobRetriesResponse], error) {
	m := req.Msg
	idem, err := idempotencyOf(ctx, jobv1connect.JobServiceUpdateJobRetriesProcedure, m, m.GetRequestId(),
		func(m proto.Message) { m.(*jobv1.UpdateJobRetriesRequest).RequestId = "" })
	if err != nil {
		return nil, err
	}
	replayed, err := h.jobs.UpdateJobRetries(ctx, m.GetJobKey(), m.GetRetries(), idem)
	return replayedResponse(&jobv1.UpdateJobRetriesResponse{}, replayed, err)
}

// UpdateJobTimeout mengimplementasikan RPC UpdateJobTimeout.
func (h *Handler) UpdateJobTimeout(ctx context.Context, req *connect.Request[jobv1.UpdateJobTimeoutRequest]) (*connect.Response[jobv1.UpdateJobTimeoutResponse], error) {
	timeout := h.activate.Limits().ClampTimeout(req.Msg.GetTimeout().AsDuration())
	if err := h.jobs.UpdateJobTimeout(ctx, req.Msg.GetJobKey(), timeout); err != nil {
		return nil, rpc.ToConnect(err)
	}
	return connect.NewResponse(&jobv1.UpdateJobTimeoutResponse{}), nil
}

// ListJobs mengimplementasikan RPC ListJobs.
func (h *Handler) ListJobs(ctx context.Context, req *connect.Request[jobv1.ListJobsRequest]) (*connect.Response[jobv1.ListJobsResponse], error) {
	m := req.Msg
	if tok := m.GetPageToken(); tok != "" && !validKey(tok) {
		return nil, rpc.Reject(connect.CodeInvalidArgument, rpc.ReasonPageTokenInvalid, errInvalidPageToken, nil)
	}
	items, next, err := h.queries.ListJobs(ctx, executionapi.JobFilter{
		Type: m.GetType(), ProcessInstanceKey: m.GetProcessInstanceKey(), OnlyOpen: m.GetOnlyOpen(),
		State: stateName(m.GetState()), AfterKey: m.GetPageToken(), Limit: m.GetPageSize(),
	})
	if err != nil {
		return nil, rpc.ToConnect(err)
	}
	out := &jobv1.ListJobsResponse{NextPageToken: next, Jobs: make([]*jobv1.JobInfo, 0, len(items))}
	for _, j := range items {
		out.Jobs = append(out.Jobs, &jobv1.JobInfo{
			JobKey: j.Key, Type: j.Type, ProcessInstanceKey: j.ProcessInstanceKey, ElementInstanceKey: j.ElementInstanceKey,
			ElementId: j.ElementID, BpmnProcessId: j.BpmnProcessID, Retries: j.Retries, CustomHeaders: headerMap(j.CustomHeaders),
			State: j.State, ProcessDefinitionKey: j.ProcessDefinitionKey, StateEnum: stateEnum(j.State), Worker: j.Worker,
			Deadline: optTimestamp(j.Deadline), ActivatableAt: optTimestamp(j.ActivatableAt), ErrorMessage: j.ErrorMessage,
			ErrorCode: j.ErrorCode, ActivationCount: j.ActivationCount,
		})
	}
	return connect.NewResponse(out), nil
}

func replayedResponse[T any](msg *T, replayed bool, err error) (*connect.Response[T], error) {
	if err != nil {
		return nil, rpc.ToConnect(err)
	}
	resp := connect.NewResponse(msg)
	if replayed {
		resp.Header().Set(ReplayedHeader, "true")
	}
	return resp, nil
}

// idempotencyOf membangun klaim dari request_id; nil bila kosong. Scope = tenant + prosedur.
func idempotencyOf(ctx context.Context, procedure string, msg proto.Message, requestID string,
	clear func(proto.Message)) (*executionapi.Idempotency, error) {
	if requestID == "" {
		return nil, nil
	}
	if err := idempotency.ValidateKey(requestID); err != nil {
		return nil, rpc.Reject(connect.CodeInvalidArgument, rpc.ReasonRequestInvalid, err, nil)
	}
	hash, err := idempotency.Hash(msg, clear)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return &executionapi.Idempotency{Scope: rpc.FromContext(ctx).Tenant + procedure, Key: requestID, RequestHash: hash}, nil
}

func headerMap(hs []executionapi.Header) map[string]string {
	out := make(map[string]string, len(hs))
	for _, h := range hs {
		out[h.Key] = h.Value
	}
	return out
}

var jobStates = map[string]jobv1.JobState{
	"CREATED":      jobv1.JobState_JOB_STATE_CREATED,
	"ACTIVATED":    jobv1.JobState_JOB_STATE_ACTIVATED,
	"COMPLETED":    jobv1.JobState_JOB_STATE_COMPLETED,
	"FAILED":       jobv1.JobState_JOB_STATE_FAILED,
	"ERROR_THROWN": jobv1.JobState_JOB_STATE_ERROR_THROWN,
	"CANCELED":     jobv1.JobState_JOB_STATE_CANCELED,
}

func stateEnum(s string) jobv1.JobState { return jobStates[s] }

func stateName(s jobv1.JobState) string {
	for name, v := range jobStates {
		if v == s {
			return name
		}
	}
	return ""
}

func optTimestamp(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}

var errInvalidPageToken = errors.New("page_token is not a token returned by this RPC")

func validKey(k string) bool { return keygen.Valid(k) }
