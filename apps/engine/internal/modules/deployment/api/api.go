// Package api adalah kontrak publik modul deployment untuk modul lain.
package api

import (
	"context"
	"errors"
	"time"

	"github.com/indr622/workflow-engine/libs/go/bpmn"
)

// ErrNotFound dikembalikan bila process definition tidak ada.
var ErrNotFound = errors.New("process definition not found")

// ProcessDefinition adalah metadata satu versi proses.
type ProcessDefinition struct {
	Key           string
	TenantID      string
	BpmnProcessID string
	Version       int32
	VersionTag    string
	Name          string
	ResourceName  string
	DeploymentKey string
	CreatedAt     time.Time
}

// ProcessDefinitions memberi modul lain akses ke model BPMN ter-parse (di-cache).
type ProcessDefinitions interface {
	GetByKey(ctx context.Context, key string) (ProcessDefinition, *bpmn.Process, error)
	GetLatest(ctx context.Context, bpmnProcessID string) (ProcessDefinition, *bpmn.Process, error)
	GetByVersion(ctx context.Context, bpmnProcessID string, version int32) (ProcessDefinition, *bpmn.Process, error)
}
