package interfaces

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/application"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/keygen"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/paging"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/rpc"
	incidentv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/incident/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/incident/v1/incidentv1connect"
)

// IncidentHandler mengimplementasikan IncidentService.
type IncidentHandler struct {
	incidents api.Incidents
	queries   *application.Queries
}

var _ incidentv1connect.IncidentServiceHandler = (*IncidentHandler)(nil)

// NewIncidentHandler membuat handler.
func NewIncidentHandler(incidents api.Incidents, queries *application.Queries) *IncidentHandler {
	return &IncidentHandler{incidents: incidents, queries: queries}
}

// Register memasang IncidentService ke mux dengan opsi rantai interceptor bersama.
func (h *IncidentHandler) Register(mux *http.ServeMux, opts ...connect.HandlerOption) {
	mux.Handle(incidentv1connect.NewIncidentServiceHandler(h, opts...))
}

// ResolveIncident mengimplementasikan RPC ResolveIncident.
func (h *IncidentHandler) ResolveIncident(ctx context.Context, req *connect.Request[incidentv1.ResolveIncidentRequest]) (*connect.Response[incidentv1.ResolveIncidentResponse], error) {
	m := req.Msg
	idem, err := idempotencyOf(incidentv1connect.IncidentServiceResolveIncidentProcedure, m, m.GetRequestId(),
		func(m proto.Message) { m.(*incidentv1.ResolveIncidentRequest).RequestId = "" })
	if err != nil {
		return nil, err
	}
	replayed, err := h.incidents.ResolveIncident(ctx, m.GetIncidentKey(), idem)
	if err != nil {
		return nil, toConnectError(err)
	}
	resp := connect.NewResponse(&incidentv1.ResolveIncidentResponse{})
	markReplayed(resp.Header(), replayed)
	return resp, nil
}

// ListIncidents mengimplementasikan RPC ListIncidents.
func (h *IncidentHandler) ListIncidents(ctx context.Context, req *connect.Request[incidentv1.ListIncidentsRequest]) (*connect.Response[incidentv1.ListIncidentsResponse], error) {
	m := req.Msg
	if tok := m.GetPageToken(); tok != "" && !keygen.Valid(tok) {
		return nil, rpc.Reject(connect.CodeInvalidArgument, rpc.ReasonPageTokenInvalid, paging.ErrInvalidToken, nil)
	}
	items, next, err := h.queries.ListIncidents(ctx, application.IncidentFilter{
		ProcessInstanceKey: m.GetProcessInstanceKey(), ErrorType: m.GetErrorType(), OnlyOpen: m.GetOnlyOpen(),
		AfterKey: m.GetPageToken(), Limit: m.GetPageSize(),
	})
	if err != nil {
		return nil, toConnectError(err)
	}
	out := &incidentv1.ListIncidentsResponse{NextPageToken: next, Incidents: make([]*incidentv1.Incident, 0, len(items))}
	for _, i := range items {
		state := incidentv1.IncidentState_INCIDENT_STATE_CREATED
		if i.State == "RESOLVED" {
			state = incidentv1.IncidentState_INCIDENT_STATE_RESOLVED
		}
		out.Incidents = append(out.Incidents, &incidentv1.Incident{
			IncidentKey: i.Key, ProcessInstanceKey: i.ProcessInstanceKey, ElementInstanceKey: i.ElementInstanceKey,
			ElementId: i.ElementID, BpmnProcessId: i.BpmnProcessID, ProcessDefinitionKey: i.ProcessDefinitionKey,
			ErrorType: i.ErrorType, ErrorMessage: i.ErrorMessage, JobKey: i.JobKey, State: state,
			CreatedAt: timestamppb.New(i.CreatedAt), ResolvedAt: optTimestamp(i.ResolvedAt),
		})
	}
	return connect.NewResponse(out), nil
}
