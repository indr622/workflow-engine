// Package interfaces berisi adapter masuk modul execution: handler Connect untuk
// ProcessInstanceService dan JobService.
package interfaces

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	deploymentapi "github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/application"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/domain"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/idempotency"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/keygen"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/paging"
	commonv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/common/v1"
	jobv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1/jobv1connect"
	processv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/process/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/process/v1/processv1connect"
)

// MaxRequestBytes adalah batas ukuran request; dokumen variabel dibatasi terpisah oleh application.
const MaxRequestBytes = 2 << 20

// Handler mengekspos use case execution lewat Connect RPC.
type Handler struct {
	// RPC Fase 4 yang belum diimplementasikan menjawab UNIMPLEMENTED sampai use case-nya ada.
	processv1connect.UnimplementedProcessInstanceServiceHandler
	jobv1connect.UnimplementedJobServiceHandler

	instances api.ProcessInstances
	jobs      api.Jobs
	queries   *application.Queries
	history   *application.History
}

var (
	_ processv1connect.ProcessInstanceServiceHandler = (*Handler)(nil)
	_ jobv1connect.JobServiceHandler                 = (*Handler)(nil)
)

// NewHandler membuat handler.
func NewHandler(instances api.ProcessInstances, jobs api.Jobs, queries *application.Queries, history *application.History) *Handler {
	return &Handler{instances: instances, jobs: jobs, queries: queries, history: history}
}

// Register memasang kedua service ke mux.
func (h *Handler) Register(mux *http.ServeMux) {
	path, handler := processv1connect.NewProcessInstanceServiceHandler(h, connect.WithReadMaxBytes(MaxRequestBytes))
	mux.Handle(path, handler)
	path, handler = jobv1connect.NewJobServiceHandler(h, connect.WithReadMaxBytes(MaxRequestBytes))
	mux.Handle(path, handler)
}

// CreateProcessInstance mengimplementasikan RPC CreateProcessInstance.
func (h *Handler) CreateProcessInstance(ctx context.Context, req *connect.Request[processv1.CreateProcessInstanceRequest]) (*connect.Response[processv1.CreateProcessInstanceResponse], error) {
	in := api.CreateInstanceRequest{Variables: []byte(req.Msg.GetVariables())}
	switch d := req.Msg.GetDefinition().(type) {
	case *processv1.CreateProcessInstanceRequest_ProcessDefinitionKey:
		if err := requireKey("process_definition_key", d.ProcessDefinitionKey); err != nil {
			return nil, err
		}
		in.DefinitionKey = d.ProcessDefinitionKey
	case *processv1.CreateProcessInstanceRequest_BpmnProcess:
		if d.BpmnProcess.GetBpmnProcessId() == "" || d.BpmnProcess.GetVersion() < 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("bpmn_process needs a bpmn_process_id and a version >= 0"))
		}
		in.BpmnProcessID, in.Version = d.BpmnProcess.GetBpmnProcessId(), d.BpmnProcess.GetVersion()
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, application.ErrDefinitionRequired)
	}
	idem, err := idempotencyOf(processv1connect.ProcessInstanceServiceCreateProcessInstanceProcedure, req.Msg, req.Msg.GetRequestId(),
		func(m proto.Message) { m.(*processv1.CreateProcessInstanceRequest).RequestId = "" })
	if err != nil {
		return nil, err
	}
	in.Idempotency = idem
	res, err := h.instances.CreateProcessInstance(ctx, in)
	if err != nil {
		return nil, toConnectError(err)
	}
	resp := connect.NewResponse(&processv1.CreateProcessInstanceResponse{
		ProcessInstanceKey: res.ProcessInstanceKey, ProcessDefinitionKey: res.ProcessDefinitionKey,
		BpmnProcessId: res.BpmnProcessID, Version: res.Version, State: instanceState(res.State),
	})
	markReplayed(resp.Header(), res.Replayed)
	return resp, nil
}

// GetProcessInstance mengimplementasikan RPC GetProcessInstance.
func (h *Handler) GetProcessInstance(ctx context.Context, req *connect.Request[processv1.GetProcessInstanceRequest]) (*connect.Response[processv1.GetProcessInstanceResponse], error) {
	if err := requireKey("process_instance_key", req.Msg.GetProcessInstanceKey()); err != nil {
		return nil, err
	}
	d, err := h.queries.GetInstance(ctx, req.Msg.GetProcessInstanceKey())
	if err != nil {
		return nil, toConnectError(err)
	}
	out := &processv1.GetProcessInstanceResponse{ProcessInstance: toProtoInstance(d.Instance), ElementInstancesTruncated: d.ElementsTruncated}
	for _, e := range d.Elements {
		out.ElementInstances = append(out.ElementInstances, toProtoElement(e))
	}
	for _, v := range d.Variables {
		out.Variables = append(out.Variables, &processv1.Variable{ScopeKey: v.ScopeKey, Name: v.Name, Value: string(v.Value)})
	}
	for _, inc := range d.Incidents {
		out.Incidents = append(out.Incidents, &processv1.Incident{
			IncidentKey: inc.Key, ElementInstanceKey: inc.ElementInstanceKey, ElementId: inc.ElementID, ErrorType: inc.ErrorType,
			ErrorMessage: inc.ErrorMessage, Resolved: inc.Resolved, CreatedAt: timestamppb.New(inc.CreatedAt),
		})
	}
	for _, j := range d.Jobs {
		out.Jobs = append(out.Jobs, &processv1.Job{
			JobKey: j.Key, Type: j.Type, ElementId: j.ElementID, ElementInstanceKey: j.ElementInstanceKey, Retries: j.Retries, State: j.State,
		})
	}
	return connect.NewResponse(out), nil
}

// ListProcessInstances mengimplementasikan RPC ListProcessInstances.
func (h *Handler) ListProcessInstances(ctx context.Context, req *connect.Request[processv1.ListProcessInstancesRequest]) (*connect.Response[processv1.ListProcessInstancesResponse], error) {
	m := req.Msg
	if err := optionalKey("process_definition_key", m.GetProcessDefinitionKey()); err != nil {
		return nil, err
	}
	if err := optionalKey("page_token", m.GetPageToken()); err != nil {
		return nil, err
	}
	items, next, err := h.queries.ListInstances(ctx, application.InstanceFilter{
		BpmnProcessID: m.GetBpmnProcessId(), ProcessDefinitionKey: m.GetProcessDefinitionKey(),
		State: stateFilter(m.GetState()), OnlyWithIncident: m.GetOnlyWithIncident(),
		AfterKey: m.GetPageToken(), Limit: m.GetPageSize(),
	})
	if err != nil {
		return nil, toConnectError(err)
	}
	out := &processv1.ListProcessInstancesResponse{NextPageToken: next}
	for _, it := range items {
		out.ProcessInstances = append(out.ProcessInstances, toProtoInstance(it))
	}
	return connect.NewResponse(out), nil
}

// CancelProcessInstance mengimplementasikan RPC CancelProcessInstance.
func (h *Handler) CancelProcessInstance(ctx context.Context, req *connect.Request[processv1.CancelProcessInstanceRequest]) (*connect.Response[processv1.CancelProcessInstanceResponse], error) {
	if err := requireKey("process_instance_key", req.Msg.GetProcessInstanceKey()); err != nil {
		return nil, err
	}
	idem, err := idempotencyOf(processv1connect.ProcessInstanceServiceCancelProcessInstanceProcedure, req.Msg, req.Msg.GetRequestId(),
		func(m proto.Message) { m.(*processv1.CancelProcessInstanceRequest).RequestId = "" })
	if err != nil {
		return nil, err
	}
	replayed, err := h.instances.CancelProcessInstance(ctx, req.Msg.GetProcessInstanceKey(), idem)
	if err != nil {
		return nil, toConnectError(err)
	}
	resp := connect.NewResponse(&processv1.CancelProcessInstanceResponse{})
	markReplayed(resp.Header(), replayed)
	return resp, nil
}

// ListProcessInstanceRecords mengimplementasikan RPC ListProcessInstanceRecords.
func (h *Handler) ListProcessInstanceRecords(ctx context.Context, req *connect.Request[processv1.ListProcessInstanceRecordsRequest]) (*connect.Response[processv1.ListProcessInstanceRecordsResponse], error) {
	m := req.Msg
	if err := requireKey("process_instance_key", m.GetProcessInstanceKey()); err != nil {
		return nil, err
	}
	tok, err := paging.Decode(m.GetPageToken())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	records, next, err := h.history.ListRecords(ctx, m.GetProcessInstanceKey(), tok.Sequence, m.GetPageSize())
	if err != nil {
		return nil, toConnectError(err)
	}
	out := &processv1.ListProcessInstanceRecordsResponse{NextPageToken: paging.Encode(paging.Token{Sequence: next})}
	for _, r := range records {
		out.Records = append(out.Records, &processv1.ExecutionRecord{
			Sequence: r.Sequence, RecordKey: r.RecordKey, ValueType: r.ValueType, Intent: r.Intent,
			ElementId: r.ElementID, Payload: string(r.Payload), Timestamp: timestamppb.New(r.Timestamp),
		})
	}
	return connect.NewResponse(out), nil
}

// ListElementInstances mengimplementasikan RPC ListElementInstances.
func (h *Handler) ListElementInstances(ctx context.Context, req *connect.Request[processv1.ListElementInstancesRequest]) (*connect.Response[processv1.ListElementInstancesResponse], error) {
	m := req.Msg
	if err := requireKey("process_instance_key", m.GetProcessInstanceKey()); err != nil {
		return nil, err
	}
	tok, err := paging.Decode(m.GetPageToken())
	if err != nil || (tok.Key != "" && !keygen.Valid(tok.Key)) {
		return nil, connect.NewError(connect.CodeInvalidArgument, paging.ErrInvalidToken)
	}
	elements, next, err := h.history.ListElementInstances(ctx, m.GetProcessInstanceKey(), m.GetOnlyActive(), tok.Key, m.GetPageSize())
	if err != nil {
		return nil, toConnectError(err)
	}
	out := &processv1.ListElementInstancesResponse{NextPageToken: paging.Encode(paging.Token{Key: next})}
	for _, e := range elements {
		out.ElementInstances = append(out.ElementInstances, toProtoElement(e))
	}
	return connect.NewResponse(out), nil
}

func toProtoElement(e application.ElementInstanceView) *processv1.ElementInstance {
	return &processv1.ElementInstance{
		ElementInstanceKey: e.Key, FlowScopeKey: e.FlowScopeKey, ElementId: e.ElementID, BpmnElementType: e.BpmnElementType,
		State: elementState(e.State), IncidentKey: e.IncidentKey, StartTime: timestamppb.New(e.StartTime), EndTime: optTimestamp(e.EndTime),
	}
}

// ListJobs mengimplementasikan RPC ListJobs.
func (h *Handler) ListJobs(ctx context.Context, req *connect.Request[jobv1.ListJobsRequest]) (*connect.Response[jobv1.ListJobsResponse], error) {
	m := req.Msg
	if err := optionalKey("process_instance_key", m.GetProcessInstanceKey()); err != nil {
		return nil, err
	}
	if err := optionalKey("page_token", m.GetPageToken()); err != nil {
		return nil, err
	}
	items, next, err := h.queries.ListJobs(ctx, application.JobFilter{
		Type: m.GetType(), ProcessInstanceKey: m.GetProcessInstanceKey(), OnlyOpen: m.GetOnlyOpen(),
		AfterKey: m.GetPageToken(), Limit: m.GetPageSize(),
	})
	if err != nil {
		return nil, toConnectError(err)
	}
	out := &jobv1.ListJobsResponse{NextPageToken: next}
	for _, j := range items {
		headers := make(map[string]string, len(j.CustomHeaders))
		for _, hd := range j.CustomHeaders {
			headers[hd.Key] = hd.Value
		}
		out.Jobs = append(out.Jobs, &jobv1.JobInfo{
			JobKey: j.Key, Type: j.Type, ProcessInstanceKey: j.ProcessInstanceKey, ElementInstanceKey: j.ElementInstanceKey,
			ElementId: j.ElementID, BpmnProcessId: j.BpmnProcessID, Retries: j.Retries, CustomHeaders: headers,
			State: j.State, ProcessDefinitionKey: j.ProcessDefinitionKey,
		})
	}
	return connect.NewResponse(out), nil
}

// CompleteJob mengimplementasikan RPC CompleteJob.
func (h *Handler) CompleteJob(ctx context.Context, req *connect.Request[jobv1.CompleteJobRequest]) (*connect.Response[jobv1.CompleteJobResponse], error) {
	if err := requireKey("job_key", req.Msg.GetJobKey()); err != nil {
		return nil, err
	}
	idem, err := idempotencyOf(jobv1connect.JobServiceCompleteJobProcedure, req.Msg, req.Msg.GetRequestId(),
		func(m proto.Message) { m.(*jobv1.CompleteJobRequest).RequestId = "" })
	if err != nil {
		return nil, err
	}
	replayed, err := h.jobs.CompleteJob(ctx, req.Msg.GetJobKey(), []byte(req.Msg.GetVariables()), idem)
	if err != nil {
		return nil, toConnectError(err)
	}
	resp := connect.NewResponse(&jobv1.CompleteJobResponse{})
	markReplayed(resp.Header(), replayed)
	return resp, nil
}

// ReplayedHeader menandai response yang diambil dari klaim request_id sebelumnya.
const ReplayedHeader = "Idempotent-Replayed"

// defaultTenant sama dengan tenant default modul lain; multi-tenant menyusul di Fase 8.
const defaultTenant = "<default>"

// idempotencyOf membangun klaim dari request_id; nil bila request_id kosong.
func idempotencyOf(procedure string, msg proto.Message, requestID string, clear func(proto.Message)) (*api.Idempotency, error) {
	if requestID == "" {
		return nil, nil
	}
	if err := idempotency.ValidateKey(requestID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	hash, err := idempotency.Hash(msg, clear)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return &api.Idempotency{Scope: defaultTenant + procedure, Key: requestID, RequestHash: hash}, nil
}

func markReplayed(h http.Header, replayed bool) {
	if replayed {
		h.Set(ReplayedHeader, "true")
	}
}

func requireKey(field, key string) error {
	if !keygen.Valid(key) {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("%s must be a canonical uuid", field))
	}
	return nil
}

func optionalKey(field, key string) error {
	if key == "" {
		return nil
	}
	return requireKey(field, key)
}

func toProtoInstance(s application.InstanceSummary) *processv1.ProcessInstance {
	return &processv1.ProcessInstance{
		ProcessInstanceKey: s.Key, ProcessDefinitionKey: s.ProcessDefinitionKey, BpmnProcessId: s.BpmnProcessID,
		Version: s.Version, State: instanceState(s.State), HasIncident: s.HasIncident,
		StartTime: timestamppb.New(s.StartTime), EndTime: optTimestamp(s.EndTime),
	}
}

func instanceState(s string) processv1.ProcessInstanceState {
	switch domain.InstanceStatus(s) {
	case domain.InstanceActive:
		return processv1.ProcessInstanceState_PROCESS_INSTANCE_STATE_ACTIVE
	case domain.InstanceCompleted:
		return processv1.ProcessInstanceState_PROCESS_INSTANCE_STATE_COMPLETED
	case domain.InstanceTerminated:
		return processv1.ProcessInstanceState_PROCESS_INSTANCE_STATE_TERMINATED
	default:
		return processv1.ProcessInstanceState_PROCESS_INSTANCE_STATE_UNSPECIFIED
	}
}

func stateFilter(s processv1.ProcessInstanceState) string {
	switch s {
	case processv1.ProcessInstanceState_PROCESS_INSTANCE_STATE_ACTIVE:
		return string(domain.InstanceActive)
	case processv1.ProcessInstanceState_PROCESS_INSTANCE_STATE_COMPLETED:
		return string(domain.InstanceCompleted)
	case processv1.ProcessInstanceState_PROCESS_INSTANCE_STATE_TERMINATED:
		return string(domain.InstanceTerminated)
	default:
		return ""
	}
}

var elementStates = map[domain.ElementState]processv1.ElementInstanceState{
	domain.ElementActivating:  processv1.ElementInstanceState_ELEMENT_INSTANCE_STATE_ACTIVATING,
	domain.ElementActivated:   processv1.ElementInstanceState_ELEMENT_INSTANCE_STATE_ACTIVATED,
	domain.ElementCompleting:  processv1.ElementInstanceState_ELEMENT_INSTANCE_STATE_COMPLETING,
	domain.ElementCompleted:   processv1.ElementInstanceState_ELEMENT_INSTANCE_STATE_COMPLETED,
	domain.ElementTerminating: processv1.ElementInstanceState_ELEMENT_INSTANCE_STATE_TERMINATING,
	domain.ElementTerminated:  processv1.ElementInstanceState_ELEMENT_INSTANCE_STATE_TERMINATED,
}

func elementState(s string) processv1.ElementInstanceState {
	return elementStates[domain.ElementState(s)]
}

func optTimestamp(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}

// toConnectError memetakan error domain dan application ke kode Connect.
func toConnectError(err error) error {
	switch {
	case errors.Is(err, domain.ErrInstanceNotFound), errors.Is(err, domain.ErrJobNotFound), errors.Is(err, deploymentapi.ErrNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, idempotency.ErrKeyReused):
		cerr := connect.NewError(connect.CodeInvalidArgument, err)
		if detail, derr := connect.NewErrorDetail(&commonv1.IdempotencyKeyReused{}); derr == nil {
			cerr.AddDetail(detail)
		}
		return cerr
	case errors.Is(err, domain.ErrInvalidVariables), errors.Is(err, application.ErrDefinitionRequired):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, domain.ErrInstanceNotActive), errors.Is(err, domain.ErrJobNotActive), errors.Is(err, domain.ErrNoNoneStartEvent):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}
