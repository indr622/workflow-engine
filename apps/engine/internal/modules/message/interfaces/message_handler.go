// Package interfaces berisi handler Connect MessageService.
package interfaces

import (
	"context"
	"net/http"

	"connectrpc.com/connect"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/message/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/rpc"
	messagev1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/message/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/message/v1/messagev1connect"
)

// Handler mengimplementasikan MessageService.
type Handler struct {
	messages api.Messages
}

var _ messagev1connect.MessageServiceHandler = (*Handler)(nil)

// NewHandler membuat handler.
func NewHandler(messages api.Messages) *Handler { return &Handler{messages: messages} }

// Register memasang MessageService ke mux dengan opsi rantai interceptor bersama.
func (h *Handler) Register(mux *http.ServeMux, opts ...connect.HandlerOption) {
	mux.Handle(messagev1connect.NewMessageServiceHandler(h, opts...))
}

// PublishMessage mengimplementasikan RPC PublishMessage.
func (h *Handler) PublishMessage(ctx context.Context, req *connect.Request[messagev1.PublishMessageRequest]) (*connect.Response[messagev1.PublishMessageResponse], error) {
	m := req.Msg
	key, err := h.messages.Publish(ctx, api.PublishRequest{
		Name: m.GetName(), CorrelationKey: m.GetCorrelationKey(), TTL: m.GetTimeToLive().AsDuration(),
		MessageID: m.GetMessageId(), Variables: []byte(m.GetVariables()),
	})
	if err != nil {
		return nil, rpc.ToConnect(err)
	}
	return connect.NewResponse(&messagev1.PublishMessageResponse{MessageKey: key, TenantId: rpc.FromContext(ctx).Tenant}), nil
}
