// Package interfaces berisi adapter masuk modul deployment: handler Connect.
package interfaces

import (
	"context"
	"errors"
	"math"
	"net/http"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	deploymentv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/deployment/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/deployment/v1/deploymentv1connect"
	"github.com/indr622/workflow-engine/libs/go/bpmn"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/application"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/domain"
)

// MaxRequestBytes adalah batas ukuran request Connect untuk service ini.
const MaxRequestBytes = 16 << 20

// Handler mengekspos use case deployment lewat Connect RPC.
type Handler struct {
	deploy   *application.DeployResource
	validate application.ValidateResource
	queries  *application.Queries
}

var _ deploymentv1connect.DeploymentServiceHandler = (*Handler)(nil)

// NewHandler membuat handler untuk use case yang diberikan.
func NewHandler(deploy *application.DeployResource, queries *application.Queries) *Handler {
	return &Handler{deploy: deploy, queries: queries}
}

// Register memasang route Connect ke mux.
func (h *Handler) Register(mux *http.ServeMux) {
	path, handler := deploymentv1connect.NewDeploymentServiceHandler(h, connect.WithReadMaxBytes(MaxRequestBytes))
	mux.Handle(path, handler)
}

// DeployResource mengimplementasikan RPC DeployResource.
func (h *Handler) DeployResource(ctx context.Context, req *connect.Request[deploymentv1.DeployResourceRequest]) (*connect.Response[deploymentv1.DeployResourceResponse], error) {
	res, err := h.deploy.Execute(ctx, domain.DefaultTenant, toInputs(req.Msg.GetResources()))
	if err != nil {
		return nil, toConnectError(err)
	}
	out := &deploymentv1.DeployResourceResponse{DeploymentKey: res.DeploymentKey, Warnings: toProtoIssues(res.Warnings)}
	for _, d := range res.Definitions {
		pd := toProtoDefinition(d.ProcessDefinition)
		pd.Duplicate = d.Duplicate
		out.ProcessDefinitions = append(out.ProcessDefinitions, pd)
	}
	return connect.NewResponse(out), nil
}

// ValidateResource mengimplementasikan RPC ValidateResource.
func (h *Handler) ValidateResource(_ context.Context, req *connect.Request[deploymentv1.ValidateResourceRequest]) (*connect.Response[deploymentv1.ValidateResourceResponse], error) {
	profile := bpmn.ProfileExecutable
	if req.Msg.GetProfile() == deploymentv1.Profile_PROFILE_FULL {
		profile = bpmn.ProfileFull
	}
	res := h.validate.Execute(toInputs(req.Msg.GetResources()), profile)
	out := &deploymentv1.ValidateResourceResponse{Valid: res.Valid, Issues: toProtoIssues(res.Violations)}
	for _, p := range res.Processes {
		out.Processes = append(out.Processes, &deploymentv1.ProcessSummary{
			ResourceName: p.ResourceName, BpmnProcessId: p.BpmnProcessID, Name: p.Name,
			VersionTag: p.VersionTag, ElementCount: int32Of(p.ElementCount),
		})
	}
	return connect.NewResponse(out), nil
}

// GetProcessDefinition mengimplementasikan RPC GetProcessDefinition.
func (h *Handler) GetProcessDefinition(ctx context.Context, req *connect.Request[deploymentv1.GetProcessDefinitionRequest]) (*connect.Response[deploymentv1.GetProcessDefinitionResponse], error) {
	pd, content, err := h.queries.GetProcessDefinition(ctx, req.Msg.GetProcessDefinitionKey())
	if err != nil {
		return nil, toConnectError(err)
	}
	return connect.NewResponse(&deploymentv1.GetProcessDefinitionResponse{
		ProcessDefinition: toProtoDefinition(pd), BpmnXml: string(content),
	}), nil
}

// ListProcessDefinitions mengimplementasikan RPC ListProcessDefinitions.
func (h *Handler) ListProcessDefinitions(ctx context.Context, req *connect.Request[deploymentv1.ListProcessDefinitionsRequest]) (*connect.Response[deploymentv1.ListProcessDefinitionsResponse], error) {
	items, next, err := h.queries.ListProcessDefinitions(ctx, application.ListInput{
		BpmnProcessID: req.Msg.GetBpmnProcessId(), LatestOnly: req.Msg.GetLatestOnly(),
		PageSize: req.Msg.GetPageSize(), PageToken: req.Msg.GetPageToken(),
	})
	if err != nil {
		return nil, toConnectError(err)
	}
	out := &deploymentv1.ListProcessDefinitionsResponse{NextPageToken: next}
	for _, pd := range items {
		out.ProcessDefinitions = append(out.ProcessDefinitions, toProtoDefinition(pd))
	}
	return connect.NewResponse(out), nil
}

func toInputs(resources []*deploymentv1.Resource) []domain.ResourceInput {
	out := make([]domain.ResourceInput, 0, len(resources))
	for _, r := range resources {
		out = append(out, domain.ResourceInput{Name: r.GetName(), Content: r.GetContent()})
	}
	return out
}

func toProtoDefinition(pd domain.ProcessDefinition) *deploymentv1.ProcessDefinition {
	return &deploymentv1.ProcessDefinition{
		ProcessDefinitionKey: pd.Key, BpmnProcessId: pd.BpmnProcessID, Version: pd.Version,
		VersionTag: pd.VersionTag, Name: pd.Name, ResourceName: pd.ResourceName,
		DeploymentKey: pd.DeploymentKey, CreatedAt: timestamppb.New(pd.CreatedAt),
	}
}

func toProtoIssues(vs []domain.Violation) []*deploymentv1.ValidationIssue {
	out := make([]*deploymentv1.ValidationIssue, 0, len(vs))
	for _, v := range vs {
		sev := deploymentv1.Severity_SEVERITY_ERROR
		if v.Severity == domain.SeverityWarning {
			sev = deploymentv1.Severity_SEVERITY_WARNING
		}
		out = append(out, &deploymentv1.ValidationIssue{
			Code: v.Code, Severity: sev, Message: v.Message, ResourceName: v.ResourceName,
			BpmnProcessId: v.ProcessID, ElementId: v.ElementID, Line: int32Of(v.Line), Column: int32Of(v.Column),
		})
	}
	return out
}

// int32Of memotong nilai ke rentang int32 agar konversi ke proto aman.
func int32Of(n int) int32 {
	switch {
	case n > math.MaxInt32:
		return math.MaxInt32
	case n < math.MinInt32:
		return math.MinInt32
	default:
		return int32(n)
	}
}

// toConnectError memetakan error domain ke kode Connect beserta detail.
func toConnectError(err error) error {
	var rejected *domain.RejectedError
	switch {
	case errors.As(err, &rejected):
		cerr := connect.NewError(connect.CodeInvalidArgument, rejected)
		if detail, derr := connect.NewErrorDetail(&deploymentv1.DeploymentRejection{Issues: toProtoIssues(rejected.Violations)}); derr == nil {
			cerr.AddDetail(detail)
		}
		return cerr
	case errors.Is(err, domain.ErrNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, application.ErrInvalidPageToken):
		return connect.NewError(connect.CodeInvalidArgument, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}
