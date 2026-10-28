// Package api adalah kontrak publik modul execution untuk modul lain: membuat dan membatalkan
// process instance, menyelesaikan job, dan event yang dipublikasikan setelah commit.
package api

import "context"

// CreateInstanceRequest memilih versi proses lewat DefinitionKey, atau BpmnProcessID dengan
// Version (0 = terbaru). Variables adalah dokumen JSON object; kosong berarti tanpa variabel.
type CreateInstanceRequest struct {
	DefinitionKey string
	BpmnProcessID string
	Version       int32
	Variables     []byte
	// Idempotency opsional: request dengan klaim yang sama dijalankan sekali (ADR-0017).
	Idempotency *Idempotency
}

// Idempotency adalah klaim request_id klien.
type Idempotency struct {
	// Scope memisahkan RPC: tenant + "/" + nama RPC lengkap.
	Scope string
	Key   string
	// RequestHash adalah SHA-256 request deterministik tanpa request_id.
	RequestHash [32]byte
}

// CreateInstanceResult adalah hasil command pertama sebuah instance.
type CreateInstanceResult struct {
	ProcessInstanceKey   string
	ProcessDefinitionKey string
	BpmnProcessID        string
	Version              int32
	// State adalah ACTIVE, COMPLETED, atau TERMINATED setelah command pertama selesai.
	State string
	// Replayed bernilai true bila hasil diambil dari klaim request_id sebelumnya.
	Replayed bool `json:"-"`
}

// ProcessInstances membuat dan membatalkan process instance.
type ProcessInstances interface {
	CreateProcessInstance(ctx context.Context, req CreateInstanceRequest) (CreateInstanceResult, error)
	// CancelProcessInstance mengembalikan replayed = true bila klaim request_id sudah pernah dijalankan.
	CancelProcessInstance(ctx context.Context, processInstanceKey string, idem *Idempotency) (replayed bool, err error)
}

// Jobs menyelesaikan job service task. Modul job Fase 4 membangun protokol worker di atasnya.
type Jobs interface {
	// CompleteJob mengembalikan replayed = true bila klaim request_id sudah pernah dijalankan.
	CompleteJob(ctx context.Context, jobKey string, variables []byte, idem *Idempotency) (replayed bool, err error)
}
