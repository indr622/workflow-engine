package api

import (
	"context"
	"time"
)

// Kontrak protokol job worker (ADR-0020). Modul job membangun long polling, notifier, dan sweeper
// di atas interface ini; execution memiliki tabel job demi atomisitas dengan token.

// Header adalah satu custom header job.
type Header struct {
	Key   string
	Value string
}

// ActivationQuery adalah parameter aktivasi yang sudah dipotong ke batas server oleh modul job.
type ActivationQuery struct {
	Type    string
	Worker  string
	MaxJobs int32
	Timeout time.Duration
	// FetchVariables kosong berarti semua variabel scope process instance.
	FetchVariables []string
}

// ActivatedJob adalah job yang baru saja di-lease ke worker.
type ActivatedJob struct {
	Key                      string
	Type                     string
	ProcessInstanceKey       string
	ElementInstanceKey       string
	ElementID                string
	BpmnProcessID            string
	ProcessDefinitionKey     string
	ProcessDefinitionVersion int32
	Retries                  int32
	CustomHeaders            []Header
	Worker                   string
	Deadline                 time.Time
	// Variables adalah dokumen JSON object.
	Variables []byte
}

// JobFailure adalah hasil FailJob dari worker.
type JobFailure struct {
	JobKey       string
	Retries      int32
	RetryBackoff time.Duration
	ErrorMessage string
	Variables    []byte
	Idempotency  *Idempotency
}

// JobError adalah BPMN error yang dilempar worker.
type JobError struct {
	JobKey       string
	ErrorCode    string
	ErrorMessage string
	Variables    []byte
	Idempotency  *Idempotency
}

// ReleasedJob adalah job yang lease-nya kedaluwarsa dan kembali dapat diaktifkan.
type ReleasedJob struct {
	Key  string
	Type string
}

// Jobs adalah operasi job untuk worker dan operator.
type Jobs interface {
	// CompleteJob mengembalikan replayed = true bila klaim request_id sudah pernah dijalankan.
	CompleteJob(ctx context.Context, jobKey string, variables []byte, idem *Idempotency) (replayed bool, err error)
	// ActivateJobs me-lease sampai MaxJobs job tanpa mengunci instance (SKIP LOCKED).
	ActivateJobs(ctx context.Context, q ActivationQuery) ([]ActivatedJob, error)
	FailJob(ctx context.Context, f JobFailure) (replayed bool, err error)
	ThrowError(ctx context.Context, e JobError) (replayed bool, err error)
	UpdateJobRetries(ctx context.Context, jobKey string, retries int32, idem *Idempotency) (replayed bool, err error)
	// UpdateJobTimeout mengubah deadline lease job ACTIVATED menjadi sekarang + timeout.
	UpdateJobTimeout(ctx context.Context, jobKey string, timeout time.Duration) error
	// ReleaseExpiredLeases mengembalikan sampai limit job ACTIVATED yang deadline-nya lewat ke CREATED.
	ReleaseExpiredLeases(ctx context.Context, limit int) ([]ReleasedJob, error)
}

// JobFilter memfilter daftar job; AfterKey adalah key terakhir halaman sebelumnya.
type JobFilter struct {
	Type               string
	ProcessInstanceKey string
	// OnlyOpen: hanya job yang belum final.
	OnlyOpen bool
	// State memfilter satu state; kosong berarti semua.
	State    string
	AfterKey string
	Limit    int32
}

// JobInfo adalah job beserta konteks, lease, dan kegagalan terakhirnya.
type JobInfo struct {
	Key                  string
	Type                 string
	ProcessInstanceKey   string
	ElementInstanceKey   string
	ElementID            string
	BpmnProcessID        string
	ProcessDefinitionKey string
	Retries              int32
	CustomHeaders        []Header
	State                string
	CreatedAt            time.Time
	Worker               string
	Deadline             *time.Time
	ActivatableAt        *time.Time
	ErrorMessage         string
	ErrorCode            string
	ActivationCount      int32
}

// JobQueries membaca daftar job dari proyeksi.
type JobQueries interface {
	// ListJobs mengembalikan satu halaman job terbaru dahulu dan key untuk halaman berikutnya.
	ListJobs(ctx context.Context, f JobFilter) ([]JobInfo, string, error)
}

// Incidents menyelesaikan incident.
type Incidents interface {
	ResolveIncident(ctx context.Context, incidentKey string, idem *Idempotency) (replayed bool, err error)
}

// Variables menulis variabel process instance.
type Variables interface {
	SetVariables(ctx context.Context, scopeKey string, variables []byte, local bool, idem *Idempotency) (replayed bool, err error)
}

// InstanceResult adalah hasil CreateProcessInstanceWithResult.
type InstanceResult struct {
	CreateInstanceResult
	// Variables adalah dokumen JSON object berisi variabel akhir scope process instance.
	Variables []byte
}
