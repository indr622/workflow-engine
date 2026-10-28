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
}

// CreateInstanceResult adalah hasil command pertama sebuah instance.
type CreateInstanceResult struct {
	ProcessInstanceKey   string
	ProcessDefinitionKey string
	BpmnProcessID        string
	Version              int32
	// State adalah ACTIVE, COMPLETED, atau TERMINATED setelah command pertama selesai.
	State string
}

// ProcessInstances membuat dan membatalkan process instance.
type ProcessInstances interface {
	CreateProcessInstance(ctx context.Context, req CreateInstanceRequest) (CreateInstanceResult, error)
	CancelProcessInstance(ctx context.Context, processInstanceKey string) error
}

// Jobs menyelesaikan job service task. Modul job Fase 4 membangun protokol worker di atasnya.
type Jobs interface {
	CompleteJob(ctx context.Context, jobKey string, variables []byte) error
}
