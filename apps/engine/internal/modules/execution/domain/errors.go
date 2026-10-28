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
)

// Error domain. Semuanya membatalkan command tanpa menulis record.
var (
	ErrInstanceNotFound   = errors.New("process instance not found")
	ErrInstanceNotActive  = errors.New("process instance is not active")
	ErrJobNotFound        = errors.New("job not found")
	ErrJobNotActive       = errors.New("job is not active")
	ErrInvalidVariables   = errors.New("invalid variables")
	ErrNoNoneStartEvent   = errors.New("process has no none start event")
	ErrUnsupportedElement = errors.New("element is not supported by the engine")
	// ErrInvalidRecord menandai record yang melanggar state saat Apply: selalu bug.
	ErrInvalidRecord = errors.New("invalid record for current state")
)
