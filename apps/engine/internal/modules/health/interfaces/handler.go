// Package interfaces berisi adapter masuk modul health: handler Connect dan HTTP.
package interfaces

import (
	"context"
	"encoding/json"
	"net/http"

	"connectrpc.com/connect"

	healthv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/health/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/health/v1/healthv1connect"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/health/application"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/health/domain"
)

// Handler mengekspos use case CheckHealth lewat Connect RPC dan endpoint HTTP /healthz.
type Handler struct {
	checkHealth *application.CheckHealth
}

var _ healthv1connect.HealthServiceHandler = (*Handler)(nil)

// NewHandler membuat handler untuk use case yang diberikan.
func NewHandler(checkHealth *application.CheckHealth) *Handler {
	return &Handler{checkHealth: checkHealth}
}

// Register memasang route Connect dan /healthz ke mux.
func (h *Handler) Register(mux *http.ServeMux, opts ...connect.HandlerOption) {
	path, connectHandler := healthv1connect.NewHealthServiceHandler(h, opts...)
	mux.Handle(path, connectHandler)
	mux.HandleFunc("GET /healthz", h.serveHTTP)
}

// Check mengimplementasikan RPC HealthService.Check.
func (h *Handler) Check(ctx context.Context, _ *connect.Request[healthv1.CheckRequest]) (*connect.Response[healthv1.CheckResponse], error) {
	report := h.checkHealth.Execute(ctx)
	return connect.NewResponse(toProto(report)), nil
}

type httpDependency struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type httpReport struct {
	Status       string           `json:"status"`
	Version      string           `json:"version"`
	Dependencies []httpDependency `json:"dependencies"`
}

func (h *Handler) serveHTTP(w http.ResponseWriter, r *http.Request) {
	report := h.checkHealth.Execute(r.Context())
	body := httpReport{Status: string(report.Status), Version: report.Version, Dependencies: []httpDependency{}}
	for _, d := range report.Dependencies {
		body.Dependencies = append(body.Dependencies, httpDependency{Name: d.Name, Status: string(d.Status), Message: d.Message})
	}

	w.Header().Set("Content-Type", "application/json")
	if !report.IsServing() {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	_ = json.NewEncoder(w).Encode(body)
}

func toProto(r domain.Report) *healthv1.CheckResponse {
	resp := &healthv1.CheckResponse{Status: toProtoStatus(r.Status), Version: r.Version}
	for _, d := range r.Dependencies {
		resp.Dependencies = append(resp.Dependencies, &healthv1.DependencyStatus{
			Name:    d.Name,
			Status:  toProtoStatus(d.Status),
			Message: d.Message,
		})
	}
	return resp
}

func toProtoStatus(s domain.Status) healthv1.ServingStatus {
	switch s {
	case domain.StatusServing:
		return healthv1.ServingStatus_SERVING_STATUS_SERVING
	case domain.StatusNotServing:
		return healthv1.ServingStatus_SERVING_STATUS_NOT_SERVING
	default:
		return healthv1.ServingStatus_SERVING_STATUS_UNSPECIFIED
	}
}
