package application

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/domain"
)

// Batas ukuran halaman daftar definition.
const (
	DefaultPageSize int32 = 50
	MaxPageSize     int32 = 500
)

// Queries adalah use case baca modul deployment.
type Queries struct {
	repo domain.Repository
}

// NewQueries membuat use case baca.
func NewQueries(repo domain.Repository) *Queries { return &Queries{repo: repo} }

// GetProcessDefinition mengembalikan metadata dan XML asli.
func (q *Queries) GetProcessDefinition(ctx context.Context, key string) (domain.ProcessDefinition, []byte, error) {
	return q.repo.GetByKey(ctx, key)
}

// ListInput adalah parameter daftar definition.
type ListInput struct {
	TenantID      string
	BpmnProcessID string
	LatestOnly    bool
	PageSize      int32
	PageToken     string
}

// ListProcessDefinitions mengembalikan satu halaman dan token halaman berikutnya (kosong bila habis).
func (q *Queries) ListProcessDefinitions(ctx context.Context, in ListInput) ([]domain.ProcessDefinition, string, error) {
	if in.TenantID == "" {
		in.TenantID = domain.DefaultTenant
	}
	size := in.PageSize
	if size <= 0 {
		size = DefaultPageSize
	}
	if size > MaxPageSize {
		size = MaxPageSize
	}
	afterID, afterVersion, err := decodePageToken(in.PageToken)
	if err != nil {
		return nil, "", err
	}
	items, err := q.repo.List(ctx, domain.ListQuery{
		TenantID: in.TenantID, BpmnProcessID: in.BpmnProcessID, LatestOnly: in.LatestOnly,
		AfterProcessID: afterID, AfterVersion: afterVersion, Limit: size + 1,
	})
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(items) > int(size) {
		items = items[:size]
		last := items[len(items)-1]
		next = encodePageToken(last.BpmnProcessID, last.Version)
	}
	return items, next, nil
}

// ErrInvalidPageToken menandai page token yang tidak bisa dibaca.
var ErrInvalidPageToken = fmt.Errorf("invalid page token")

func encodePageToken(processID string, version int32) string {
	return base64.RawURLEncoding.EncodeToString([]byte(processID + "\x00" + strconv.Itoa(int(version))))
}

func decodePageToken(token string) (string, int32, error) {
	if token == "" {
		return "", 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return "", 0, ErrInvalidPageToken
	}
	id, ver, ok := strings.Cut(string(raw), "\x00")
	if !ok {
		return "", 0, ErrInvalidPageToken
	}
	v, err := strconv.ParseInt(ver, 10, 32)
	if err != nil {
		return "", 0, ErrInvalidPageToken
	}
	return id, int32(v), nil
}
