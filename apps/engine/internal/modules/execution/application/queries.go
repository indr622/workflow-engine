package application

import (
	"context"
	"encoding/json"
	"time"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/api"
)

// Model baca atas proyeksi. Berbeda dari state runtime domain: memuat juga riwayat (elemen
// final, job selesai, incident yang sudah diselesaikan).

// InstanceSummary adalah satu baris daftar process instance.
type InstanceSummary struct {
	Key                  string
	ProcessDefinitionKey string
	BpmnProcessID        string
	Version              int32
	State                string
	HasIncident          bool
	StartTime            time.Time
	EndTime              *time.Time
}

// ElementInstanceView adalah element instance beserta waktu.
type ElementInstanceView struct {
	Key             string
	FlowScopeKey    string
	ElementID       string
	BpmnElementType string
	State           string
	IncidentKey     string
	StartTime       time.Time
	EndTime         *time.Time
}

// VariableView adalah variabel dengan nilai JSON.
type VariableView struct {
	ScopeKey string
	Name     string
	Value    json.RawMessage
}

// IncidentView adalah incident terbuka atau yang sudah diselesaikan.
type IncidentView struct {
	Key                string
	ElementInstanceKey string
	ElementID          string
	ErrorType          string
	ErrorMessage       string
	Resolved           bool
	CreatedAt          time.Time
}

// Header adalah satu custom header job.
type Header = api.Header

// JobView adalah job beserta konteks, lease, dan kegagalan terakhirnya.
type JobView = api.JobInfo

// InstanceDetail adalah satu instance lengkap.
type InstanceDetail struct {
	Instance InstanceSummary
	// Elements dibatasi MaxElementsInDetail; ElementsTruncated true bila ada lebih banyak.
	Elements          []ElementInstanceView
	ElementsTruncated bool
	Variables         []VariableView
	Incidents         []IncidentView
	Jobs              []JobView
}

// InstanceFilter memfilter daftar instance; AfterKey adalah key terakhir halaman sebelumnya.
type InstanceFilter struct {
	BpmnProcessID        string
	ProcessDefinitionKey string
	State                string
	OnlyWithIncident     bool
	AfterKey             string
	Limit                int32
}

// JobFilter memfilter daftar job.
type JobFilter = api.JobFilter

// IncidentFilter memfilter daftar incident lintas instance.
type IncidentFilter struct {
	ProcessInstanceKey string
	ErrorType          string
	OnlyOpen           bool
	AfterKey           string
	Limit              int32
}

// IncidentInfo adalah satu incident beserta konteks instance-nya.
type IncidentInfo struct {
	Key                  string
	ProcessInstanceKey   string
	ElementInstanceKey   string
	ElementID            string
	BpmnProcessID        string
	ProcessDefinitionKey string
	ErrorType            string
	ErrorMessage         string
	JobKey               string
	State                string
	CreatedAt            time.Time
	ResolvedAt           *time.Time
}

// ReadModel adalah port baca proyeksi, diimplementasikan infrastructure.
type ReadModel interface {
	GetInstance(ctx context.Context, key string) (InstanceDetail, error)
	ListInstances(ctx context.Context, f InstanceFilter) ([]InstanceSummary, error)
	ListJobs(ctx context.Context, f JobFilter) ([]JobView, error)
	ListIncidents(ctx context.Context, f IncidentFilter) ([]IncidentInfo, error)
}

// Batas halaman.
const (
	DefaultPageSize = 50
	MaxPageSize     = 500
)

// Queries membaca proyeksi untuk handler.
type Queries struct {
	read ReadModel
}

// NewQueries membuat use case query.
func NewQueries(read ReadModel) *Queries { return &Queries{read: read} }

// GetInstance mengembalikan instance lengkap; domain.ErrInstanceNotFound bila tidak ada.
func (q *Queries) GetInstance(ctx context.Context, key string) (InstanceDetail, error) {
	return q.read.GetInstance(ctx, key)
}

// ListInstances mengembalikan satu halaman instance terbaru dahulu dan key untuk halaman berikutnya.
func (q *Queries) ListInstances(ctx context.Context, f InstanceFilter) ([]InstanceSummary, string, error) {
	f.Limit = pageSize(f.Limit)
	page, err := q.read.ListInstances(ctx, InstanceFilter{
		BpmnProcessID: f.BpmnProcessID, ProcessDefinitionKey: f.ProcessDefinitionKey, State: f.State,
		OnlyWithIncident: f.OnlyWithIncident, AfterKey: f.AfterKey, Limit: f.Limit + 1,
	})
	if err != nil {
		return nil, "", err
	}
	next := ""
	if int32(len(page)) > f.Limit { //nolint:gosec // panjang halaman dibatasi MaxPageSize
		page = page[:f.Limit]
		next = page[len(page)-1].Key
	}
	return page, next, nil
}

// ListJobs mengembalikan satu halaman job terbaru dahulu dan key untuk halaman berikutnya.
func (q *Queries) ListJobs(ctx context.Context, f JobFilter) ([]JobView, string, error) {
	f.Limit = pageSize(f.Limit)
	limit := f.Limit
	f.Limit++
	page, err := q.read.ListJobs(ctx, f)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if int32(len(page)) > limit { //nolint:gosec // panjang halaman dibatasi MaxPageSize
		page = page[:limit]
		next = page[len(page)-1].Key
	}
	return page, next, nil
}

// ListIncidents mengembalikan satu halaman incident terbaru dahulu dan key untuk halaman berikutnya.
func (q *Queries) ListIncidents(ctx context.Context, f IncidentFilter) ([]IncidentInfo, string, error) {
	f.Limit = pageSize(f.Limit)
	limit := f.Limit
	f.Limit++
	page, err := q.read.ListIncidents(ctx, f)
	if err != nil {
		return nil, "", err
	}
	next := ""
	if int32(len(page)) > limit { //nolint:gosec // panjang halaman dibatasi MaxPageSize
		page = page[:limit]
		next = page[len(page)-1].Key
	}
	return page, next, nil
}

func pageSize(n int32) int32 {
	switch {
	case n <= 0:
		return DefaultPageSize
	case n > MaxPageSize:
		return MaxPageSize
	default:
		return n
	}
}
