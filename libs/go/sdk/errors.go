package sdk

import (
	"errors"

	"connectrpc.com/connect"

	commonv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/common/v1"
)

// Error adalah error dari engine dengan reason stabil (workflow.common.v1.ErrorInfo). Gunakan
// errors.Is dengan sentinel di bawah, misalnya errors.Is(err, sdk.ErrJobNotActive).
type Error struct {
	Code     connect.Code
	Reason   string
	Message  string
	Metadata map[string]string
	cause    error
}

func (e *Error) Error() string {
	if e.Reason == "" {
		return e.Code.String() + ": " + e.Message
	}
	return e.Code.String() + " " + e.Reason + ": " + e.Message
}

// Unwrap mengembalikan *connect.Error asli.
func (e *Error) Unwrap() error { return e.cause }

// Is mencocokkan berdasarkan reason, sehingga errors.Is(err, ErrJobNotFound) bekerja.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Reason != "" && t.Reason == e.Reason
}

// Sentinel per reason (tabel 8.4 rencana Fase 4).
var (
	ErrRequestInvalid            = &Error{Reason: "REQUEST_INVALID"}
	ErrVariablesInvalid          = &Error{Reason: "VARIABLES_INVALID"}
	ErrJobNotFound               = &Error{Reason: "JOB_NOT_FOUND"}
	ErrJobNotActive              = &Error{Reason: "JOB_NOT_ACTIVE"}
	ErrJobNotActivated           = &Error{Reason: "JOB_NOT_ACTIVATED"}
	ErrProcessInstanceNotFound   = &Error{Reason: "PROCESS_INSTANCE_NOT_FOUND"}
	ErrProcessInstanceNotActive  = &Error{Reason: "PROCESS_INSTANCE_NOT_ACTIVE"}
	ErrProcessDefinitionNotFound = &Error{Reason: "PROCESS_DEFINITION_NOT_FOUND"}
	ErrIncidentNotFound          = &Error{Reason: "INCIDENT_NOT_FOUND"}
	ErrIncidentAlreadyResolved   = &Error{Reason: "INCIDENT_ALREADY_RESOLVED"}
	ErrScopeNotFound             = &Error{Reason: "SCOPE_NOT_FOUND"}
	ErrMessageDuplicate          = &Error{Reason: "MESSAGE_DUPLICATE"}
	ErrResultTimeout             = &Error{Reason: "RESULT_TIMEOUT"}
	ErrDeploymentRejected        = &Error{Reason: "DEPLOYMENT_REJECTED"}
	ErrIdempotencyKeyReused      = &Error{Reason: "IDEMPOTENCY_KEY_REUSED"}
)

// ErrInvalidVariables dikembalikan SDK sendiri bila nilai variabel tidak bisa dijadikan JSON object.
var ErrInvalidVariables = errors.New("sdk: variables must be a map, struct, json.RawMessage, []byte or JSON string object")

// AsErrorInfo mengembalikan detail ErrorInfo dari error engine, atau nil.
func AsErrorInfo(err error) *commonv1.ErrorInfo {
	var cerr *connect.Error
	if !errors.As(err, &cerr) {
		return nil
	}
	for _, d := range cerr.Details() {
		if v, derr := d.Value(); derr == nil {
			if info, ok := v.(*commonv1.ErrorInfo); ok {
				return info
			}
		}
	}
	return nil
}

// wrap mengubah *connect.Error menjadi *Error; error lain dikembalikan apa adanya.
func wrap(err error) error {
	if err == nil {
		return nil
	}
	var cerr *connect.Error
	if !errors.As(err, &cerr) {
		return err
	}
	out := &Error{Code: cerr.Code(), Message: cerr.Message(), cause: err}
	if info := AsErrorInfo(err); info != nil {
		out.Reason, out.Metadata = info.GetReason(), info.GetMetadata()
	}
	return out
}
