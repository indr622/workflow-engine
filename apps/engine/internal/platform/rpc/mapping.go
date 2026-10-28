package rpc

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	"github.com/indr622/workflow-engine/apps/engine/internal/platform/idempotency"
	commonv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/common/v1"
)

// Reasoner dipenuhi error domain yang membawa reason stabil. Modul memetakan error modul lain
// lewat interface ini tanpa meng-import paket domain-nya.
type Reasoner interface {
	Reason() string
}

// MetadataCarrier dipenuhi error yang menyertakan metadata ErrorInfo (mis. process_instance_key).
type MetadataCarrier interface {
	ErrorMetadata() map[string]string
}

// reasonCodes memetakan reason ke kode Connect (tabel 8.4 rencana Fase 4).
var reasonCodes = map[string]connect.Code{
	ReasonRequestInvalid:             connect.CodeInvalidArgument,
	ReasonVariablesInvalid:           connect.CodeInvalidArgument,
	ReasonPageTokenInvalid:           connect.CodeInvalidArgument,
	ReasonDeploymentRejected:         connect.CodeInvalidArgument,
	ReasonIdempotencyKeyReused:       connect.CodeInvalidArgument,
	ReasonJobNotFound:                connect.CodeNotFound,
	ReasonProcessInstanceNotFound:    connect.CodeNotFound,
	ReasonIncidentNotFound:           connect.CodeNotFound,
	ReasonProcessDefinitionNotFound:  connect.CodeNotFound,
	ReasonScopeNotFound:              connect.CodeNotFound,
	ReasonJobNotActive:               connect.CodeFailedPrecondition,
	ReasonJobNotActivated:            connect.CodeFailedPrecondition,
	ReasonProcessInstanceNotActive:   connect.CodeFailedPrecondition,
	ReasonIncidentAlreadyResolved:    connect.CodeFailedPrecondition,
	ReasonProcessHasNoNoneStartEvent: connect.CodeFailedPrecondition,
	ReasonMessageDuplicate:           connect.CodeAlreadyExists,
	ReasonLocalScopeNotSupported:     connect.CodeUnimplemented,
	ReasonResultTimeout:              connect.CodeDeadlineExceeded,
	ReasonEngineShuttingDown:         connect.CodeUnavailable,
}

// CodeForReason mengembalikan kode Connect untuk reason; reason tak dikenal → FailedPrecondition.
func CodeForReason(reason string) connect.Code {
	if c, ok := reasonCodes[reason]; ok {
		return c
	}
	return connect.CodeFailedPrecondition
}

// ToConnect memetakan error use case ke error Connect dengan ErrorInfo. Error tanpa reason menjadi
// INTERNAL tanpa detail.
func ToConnect(err error) error {
	if err == nil {
		return nil
	}
	var cerr *connect.Error
	if errors.As(err, &cerr) {
		return cerr
	}
	if errors.Is(err, idempotency.ErrKeyReused) {
		out := Reject(connect.CodeInvalidArgument, ReasonIdempotencyKeyReused, err, nil)
		if detail, derr := connect.NewErrorDetail(&commonv1.IdempotencyKeyReused{}); derr == nil {
			out.AddDetail(detail)
		}
		return out
	}
	var r Reasoner
	if errors.As(err, &r) {
		var meta map[string]string
		var mc MetadataCarrier
		if errors.As(err, &mc) {
			meta = mc.ErrorMetadata()
		}
		return Reject(CodeForReason(r.Reason()), r.Reason(), err, meta)
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return connect.NewError(connect.CodeDeadlineExceeded, err)
	case errors.Is(err, context.Canceled):
		return connect.NewError(connect.CodeCanceled, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}

// ReasonedError adalah error dengan reason stabil, untuk lapisan yang tidak punya tipe error sendiri.
type ReasonedError struct {
	reason, msg string
}

// NewReasoned membuat ReasonedError.
func NewReasoned(reason, msg string) *ReasonedError { return &ReasonedError{reason: reason, msg: msg} }

func (e *ReasonedError) Error() string { return e.msg }

// Reason memenuhi Reasoner.
func (e *ReasonedError) Reason() string { return e.reason }
