package domain

import (
	"context"
	"encoding/json"
	"time"

	"github.com/indr622/workflow-engine/libs/go/bpmn"
)

// KeyGenerator memberi key dari blok yang sudah disiapkan application. Sengaja tanpa context dan
// I/O: bila blok habis, Processor mengembalikan ErrKeysExhausted dan application mengulang command.
type KeyGenerator interface {
	Next() (int64, bool)
}

// VariableLookup membaca variabel yang terlihat oleh ekspresi.
type VariableLookup interface {
	Lookup(name string) (json.RawMessage, bool)
}

// ExpressionEvaluator mengevaluasi atribut BPMN yang bisa berupa literal atau ekspresi FEEL.
// Pesan error dipakai apa adanya sebagai pesan incident.
type ExpressionEvaluator interface {
	EvaluateBool(expr bpmn.Expression, vars VariableLookup) (bool, error)
	EvaluateString(expr bpmn.Expression, vars VariableLookup) (string, error)
	EvaluateInt(expr bpmn.Expression, vars VariableLookup) (int64, error)
}

// Clock memberi waktu sekarang; di-inject agar Processor deterministik.
type Clock interface {
	Now() time.Time
}

// Repository membuka transaksi untuk satu command.
type Repository interface {
	WithTx(ctx context.Context, fn func(ctx context.Context, tx TxRepository) error) error
}

// TxRepository adalah operasi di dalam transaksi satu command.
type TxRepository interface {
	// LockInstance mengunci baris process instance (FOR UPDATE); ErrInstanceNotFound bila tidak ada.
	LockInstance(ctx context.Context, key int64) error
	// LoadRuntimeState memuat state runtime instance yang sudah dikunci.
	LoadRuntimeState(ctx context.Context, key int64) (*InstanceSnapshot, error)
	// InstanceKeyOfJob mengembalikan process instance pemilik job; ErrJobNotFound bila tidak ada.
	InstanceKeyOfJob(ctx context.Context, jobKey int64) (int64, error)
	// NextKeys mengambil n key baru dari generator global.
	NextKeys(ctx context.Context, n int) ([]int64, error)
	// AppendAndProject menambahkan record ke log dan memperbarui proyeksi, dengan state akhir
	// sebagai sumber nilai turunan (counter join, has_incident).
	AppendAndProject(ctx context.Context, records []Record, state *InstanceState) error
}

// SliceKeys adalah KeyGenerator di atas blok key yang sudah diambil.
type SliceKeys struct {
	keys []int64
	next int
}

// NewSliceKeys membuat generator dari blok key.
func NewSliceKeys(keys []int64) *SliceKeys { return &SliceKeys{keys: keys} }

// Next memenuhi KeyGenerator.
func (s *SliceKeys) Next() (int64, bool) {
	if s.next >= len(s.keys) {
		return 0, false
	}
	k := s.keys[s.next]
	s.next++
	return k, true
}

// Used mengembalikan jumlah key yang sudah dipakai.
func (s *SliceKeys) Used() int { return s.next }
