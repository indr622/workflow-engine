package application

import (
	"context"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/domain"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/outbox"
)

// RecordEvent adalah event outbox yang dipicu record ke-RecordIndex (0-based) dari satu command.
// Infrastructure mengisi AggregateSeq dengan sequence record tersebut.
type RecordEvent struct {
	RecordIndex int
	Event       outbox.Event
}

// TxScope adalah semua port transaksional satu command. domain.TxRepository tidak tahu outbox;
// outbox hanya terlihat di application (ADR-0015).
type TxScope interface {
	domain.TxRepository
	// EnqueueForRecords mengantrekan event outbox; event dikirim dalam batch yang sama dengan
	// AppendAndProject (atau saat commit bila tidak ada record).
	EnqueueForRecords(events []RecordEvent)
}

// UnitOfWork membuka transaksi command dan membaca referensi yang tidak pernah berubah
// (job → instance, instance → definition) di luar transaksi, agar model proses dimuat sebelum row lock.
type UnitOfWork interface {
	Do(ctx context.Context, fn func(ctx context.Context, tx TxScope) error) error
	// JobRef mengembalikan process instance pemilik job beserta process definition-nya;
	// domain.ErrJobNotFound bila job tidak ada.
	JobRef(ctx context.Context, jobKey string) (instanceKey, definitionKey string, err error)
	// DefinitionKeyOf mengembalikan process definition sebuah instance; domain.ErrInstanceNotFound bila tidak ada.
	DefinitionKeyOf(ctx context.Context, instanceKey string) (string, error)
}
