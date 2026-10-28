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

// Error domain. Semuanya membatalkan command tanpa menulis record.
var (
	ErrInstanceNotFound  = errors.New("process instance not found")
	ErrInstanceNotActive = errors.New("process instance is not active")
	ErrJobNotFound       = errors.New("job not found")
	ErrJobNotActive      = errors.New("job is not active")
	// ErrJobNotActivated: UpdateJobTimeout pada job yang tidak sedang dipegang worker.
	ErrJobNotActivated = errors.New("job is not activated")
	// ErrJobNotOpen: UpdateJobRetries pada job yang sudah final.
	ErrJobNotOpen              = errors.New("job is already completed or canceled")
	ErrIncidentNotFound        = errors.New("incident not found")
	ErrIncidentAlreadyResolved = errors.New("incident is already resolved")
	ErrScopeNotFound           = errors.New("variable scope not found")
	// ErrLocalScopeNotSupported: scope variabel lokal baru ada di Fase 7.
	ErrLocalScopeNotSupported = errors.New("local variables on an element instance scope are not supported yet")
	ErrInvalidVariables       = errors.New("invalid variables")
	ErrNoNoneStartEvent       = errors.New("process has no none start event")
	ErrUnsupportedElement     = errors.New("element is not supported by the engine")
	// ErrInvalidRecord menandai record yang melanggar state saat Apply: selalu bug.
	ErrInvalidRecord = errors.New("invalid record for current state")
)
