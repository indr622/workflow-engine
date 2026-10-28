package domain

import (
	"context"
	"encoding/json"
	"time"

	"github.com/indr622/workflow-engine/libs/go/bpmn"
)

// KeyGenerator memberi key UUIDv7 baru (ADR-0011). Sengaja tanpa context dan error: key dibuat di
// dalam proses engine tanpa I/O. Determinisme replay tidak bergantung padanya karena key tercatat
// di record.
type KeyGenerator interface {
	Next() string
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

// Repository membuka transaksi untuk satu command dan membaca referensi yang tidak pernah berubah
// (job → instance, instance → definition) di luar transaksi, agar model proses bisa dimuat
// sebelum row lock diambil.
type Repository interface {
	WithTx(ctx context.Context, fn func(ctx context.Context, tx TxRepository) error) error
	// JobRef mengembalikan process instance pemilik job beserta process definition-nya;
	// ErrJobNotFound bila job tidak ada.
	JobRef(ctx context.Context, jobKey string) (instanceKey, definitionKey string, err error)
	// DefinitionKeyOf mengembalikan process definition sebuah instance; ErrInstanceNotFound bila tidak ada.
	DefinitionKeyOf(ctx context.Context, instanceKey string) (string, error)
}

// TxRepository adalah operasi di dalam transaksi satu command.
type TxRepository interface {
	// LockAndLoad mengunci baris process instance (FOR UPDATE) dan memuat state runtime-nya dalam
	// satu round trip; ErrInstanceNotFound bila tidak ada.
	LockAndLoad(ctx context.Context, key string) (*InstanceSnapshot, error)
	// AppendAndProject menambahkan record ke log dan memperbarui proyeksi, dengan state akhir
	// sebagai sumber nilai turunan (counter join, has_incident).
	AppendAndProject(ctx context.Context, records []Record, state *InstanceState) error
}
