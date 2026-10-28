package domain

import "errors"

// IncidentType adalah kategori incident; nama mengikuti ErrorType Zeebe bila ada padanannya.
type IncidentType string

// Tipe incident Fase 2.
const (
	// IncidentConditionError: tidak ada flow yang cocok tanpa default, kondisi bukan boolean,
	// atau evaluasi kondisi gagal di exclusive/inclusive gateway.
	IncidentConditionError IncidentType = "CONDITION_ERROR"
	// IncidentExtractValueError: type/retries task definition gagal dievaluasi atau tipenya salah.
	IncidentExtractValueError IncidentType = "EXTRACT_VALUE_ERROR"
	// IncidentExecutionLimitExceeded: batas record per command terlampaui (loop tanpa wait state).
	IncidentExecutionLimitExceeded IncidentType = "EXECUTION_LIMIT_EXCEEDED"
	// IncidentJobNoRetries: worker menggagalkan job dengan retries 0.
	IncidentJobNoRetries IncidentType = "JOB_NO_RETRIES"
	// IncidentUnhandledErrorEvent: worker melempar BPMN error yang tidak ditangkap catch event.
	IncidentUnhandledErrorEvent IncidentType = "UNHANDLED_ERROR_EVENT"
)

// Error adalah error aturan domain dengan reason stabil (ErrorInfo.reason, ADR-0023). Modul lain
// memetakannya lewat method Reason tanpa meng-import paket ini.
type Error struct {
	reason, msg string
}

func (e *Error) Error() string { return e.msg }

// Reason mengembalikan kode stabil SCREAMING_SNAKE_CASE.
func (e *Error) Reason() string { return e.reason }

// Error domain. Semuanya membatalkan command tanpa menulis record.
var (
	ErrInstanceNotFound  = &Error{"PROCESS_INSTANCE_NOT_FOUND", "process instance not found"}
	ErrInstanceNotActive = &Error{"PROCESS_INSTANCE_NOT_ACTIVE", "process instance is not active"}
	ErrJobNotFound       = &Error{"JOB_NOT_FOUND", "job not found"}
	ErrJobNotActive      = &Error{"JOB_NOT_ACTIVE", "job is not active"}
	// ErrJobNotActivated: UpdateJobTimeout pada job yang tidak sedang dipegang worker.
	ErrJobNotActivated = &Error{"JOB_NOT_ACTIVATED", "job is not activated"}
	// ErrJobNotOpen: UpdateJobRetries pada job yang sudah final.
	ErrJobNotOpen              = &Error{"JOB_NOT_ACTIVE", "job is already completed or canceled"}
	ErrIncidentNotFound        = &Error{"INCIDENT_NOT_FOUND", "incident not found"}
	ErrIncidentAlreadyResolved = &Error{"INCIDENT_ALREADY_RESOLVED", "incident is already resolved"}
	ErrScopeNotFound           = &Error{"SCOPE_NOT_FOUND", "variable scope not found"}
	// ErrLocalScopeNotSupported: scope variabel lokal baru ada di Fase 7.
	ErrLocalScopeNotSupported = &Error{"LOCAL_SCOPE_NOT_SUPPORTED", "local variables on an element instance scope are not supported yet"}
	ErrInvalidVariables       = &Error{"VARIABLES_INVALID", "invalid variables"}
	ErrNoNoneStartEvent       = &Error{"PROCESS_HAS_NO_NONE_START_EVENT", "process has no none start event"}
	// ErrUnsupportedElement dan ErrInvalidRecord sengaja tanpa reason: keduanya bug, dipetakan INTERNAL.
	ErrUnsupportedElement = errors.New("element is not supported by the engine")
	// ErrInvalidRecord menandai record yang melanggar state saat Apply: selalu bug.
	ErrInvalidRecord = errors.New("invalid record for current state")
)
