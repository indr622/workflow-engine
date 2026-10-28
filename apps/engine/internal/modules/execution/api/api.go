// Package api adalah kontrak publik modul execution untuk modul lain: membuat dan membatalkan
// process instance, protokol job worker, incident, variabel, dan event yang dipublikasikan setelah
// commit.
package api

import (
	"context"
	"time"
)

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
	// CreateProcessInstanceWithResult membuat instance lalu menunggu sampai selesai atau timeout
	// (ErrResultTimeout, instance tetap berjalan). fetchVariables kosong berarti semua variabel.
	CreateProcessInstanceWithResult(ctx context.Context, req CreateInstanceRequest, fetchVariables []string,
		timeout time.Duration) (InstanceResult, error)
}

// ResultTimeoutError dikembalikan CreateProcessInstanceWithResult bila instance belum selesai saat
// timeout habis.
type ResultTimeoutError struct {
	ProcessInstanceKey string
}

func (e *ResultTimeoutError) Error() string {
	return "process instance " + e.ProcessInstanceKey + " did not complete before the request timeout"
}

// Reason memberi ErrorInfo.reason.
func (e *ResultTimeoutError) Reason() string { return "RESULT_TIMEOUT" }

// ErrorMetadata memberi ErrorInfo.metadata.
func (e *ResultTimeoutError) ErrorMetadata() map[string]string {
	return map[string]string{"process_instance_key": e.ProcessInstanceKey}
}
