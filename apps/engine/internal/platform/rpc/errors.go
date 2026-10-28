// Package rpc berisi bagian Connect yang dipakai semua modul (ADR-0023): rantai interceptor
// (recover, logging, timeout, authn placeholder, protovalidate), batas ukuran request, dan helper
// error dengan detail workflow.common.v1.ErrorInfo.
package rpc

import (
	"errors"

	"connectrpc.com/connect"

	commonv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/common/v1"
)

// Domain adalah nilai ErrorInfo.domain untuk semua error engine.
const Domain = "workflow-engine"

// Reason stabil di ErrorInfo.reason (tabel 8.4 rencana Fase 4). Klien bercabang berdasarkan reason.
const (
	ReasonRequestInvalid             = "REQUEST_INVALID"
	ReasonVariablesInvalid           = "VARIABLES_INVALID"
	ReasonPageTokenInvalid           = "PAGE_TOKEN_INVALID"
	ReasonJobNotFound                = "JOB_NOT_FOUND"
	ReasonProcessInstanceNotFound    = "PROCESS_INSTANCE_NOT_FOUND"
	ReasonIncidentNotFound           = "INCIDENT_NOT_FOUND"
	ReasonProcessDefinitionNotFound  = "PROCESS_DEFINITION_NOT_FOUND"
	ReasonScopeNotFound              = "SCOPE_NOT_FOUND"
	ReasonJobNotActive               = "JOB_NOT_ACTIVE"
	ReasonJobNotActivated            = "JOB_NOT_ACTIVATED"
	ReasonProcessInstanceNotActive   = "PROCESS_INSTANCE_NOT_ACTIVE"
	ReasonIncidentAlreadyResolved    = "INCIDENT_ALREADY_RESOLVED"
	ReasonMessageDuplicate           = "MESSAGE_DUPLICATE"
	ReasonLocalScopeNotSupported     = "LOCAL_SCOPE_NOT_SUPPORTED"
	ReasonResultTimeout              = "RESULT_TIMEOUT"
	ReasonDeploymentRejected         = "DEPLOYMENT_REJECTED"
	ReasonIdempotencyKeyReused       = "IDEMPOTENCY_KEY_REUSED"
	ReasonProcessHasNoNoneStartEvent = "PROCESS_HAS_NO_NONE_START_EVENT"
	ReasonEngineShuttingDown         = "ENGINE_SHUTTING_DOWN"
)

// Reject membuat error Connect dengan detail ErrorInfo. metadata boleh nil.
func Reject(code connect.Code, reason string, err error, metadata map[string]string) *connect.Error {
	cerr := connect.NewError(code, err)
	if detail, derr := connect.NewErrorDetail(&commonv1.ErrorInfo{Reason: reason, Domain: Domain, Metadata: metadata}); derr == nil {
		cerr.AddDetail(detail)
	}
	return cerr
}

// ErrorInfoOf mengembalikan detail ErrorInfo pertama dari error Connect, atau nil.
func ErrorInfoOf(err error) *commonv1.ErrorInfo {
	var cerr *connect.Error
	if !errors.As(err, &cerr) {
		return nil
	}
	for _, d := range cerr.Details() {
		v, derr := d.Value()
		if derr != nil {
			continue
		}
		if info, ok := v.(*commonv1.ErrorInfo); ok {
			return info
		}
	}
	return nil
}
