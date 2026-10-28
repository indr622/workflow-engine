package application

import (
	"context"
	"encoding/json"
	"time"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/domain"
)

// MaxElementsInDetail membatasi element instance di GetInstance; sisanya lewat ListElementInstances.
const MaxElementsInDetail = 1000

// Batas halaman riwayat.
const (
	DefaultHistoryPageSize = 100
	MaxHistoryPageSize     = 500
)

// RecordView adalah satu record append-only untuk riwayat.
type RecordView struct {
	Sequence  int32
	RecordKey string
	ValueType string
	Intent    string
	ElementID string
	Payload   json.RawMessage
	Timestamp time.Time
}

// HistoryReader adalah port baca riwayat berhalaman (keyset).
type HistoryReader interface {
	ListRecords(ctx context.Context, key string, afterSequence int32, limit int32) ([]RecordView, error)
	ListElementInstances(ctx context.Context, key string, onlyActive bool, afterKey string, limit int32) ([]ElementInstanceView, error)
	InstanceExists(ctx context.Context, key string) (bool, error)
}

// History membaca riwayat instance berhalaman.
type History struct {
	read HistoryReader
}

// NewHistory membuat use case riwayat.
func NewHistory(read HistoryReader) *History { return &History{read: read} }

func historyPageSize(n int32) int32 {
	switch {
	case n <= 0:
		return DefaultHistoryPageSize
	case n > MaxHistoryPageSize:
		return MaxHistoryPageSize
	default:
		return n
	}
}

// ListRecords mengembalikan satu halaman record urut sequence; next = sequence terakhir bila masih ada.
func (h *History) ListRecords(ctx context.Context, key string, afterSequence, pageSize int32) ([]RecordView, int32, error) {
	if err := h.mustExist(ctx, key); err != nil {
		return nil, 0, err
	}
	limit := historyPageSize(pageSize)
	page, err := h.read.ListRecords(ctx, key, afterSequence, limit+1)
	if err != nil {
		return nil, 0, err
	}
	if int32(len(page)) > limit { //nolint:gosec // dibatasi MaxHistoryPageSize
		page = page[:limit]
		return page, page[len(page)-1].Sequence, nil
	}
	return page, 0, nil
}

// ListElementInstances mengembalikan satu halaman element instance urut key.
func (h *History) ListElementInstances(ctx context.Context, key string, onlyActive bool, afterKey string, pageSize int32) ([]ElementInstanceView, string, error) {
	if err := h.mustExist(ctx, key); err != nil {
		return nil, "", err
	}
	limit := historyPageSize(pageSize)
	page, err := h.read.ListElementInstances(ctx, key, onlyActive, afterKey, limit+1)
	if err != nil {
		return nil, "", err
	}
	if int32(len(page)) > limit { //nolint:gosec // dibatasi MaxHistoryPageSize
		page = page[:limit]
		return page, page[len(page)-1].Key, nil
	}
	return page, "", nil
}

func (h *History) mustExist(ctx context.Context, key string) error {
	ok, err := h.read.InstanceExists(ctx, key)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrInstanceNotFound
	}
	return nil
}
