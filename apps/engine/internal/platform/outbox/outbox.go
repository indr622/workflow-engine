// Package outbox adalah outbox transaksional engine (ADR-0015): modul menulis event dalam transaksi
// command yang sama dengan state, lalu Relay mengirimnya ke consumer durable at-least-once, urut per
// aggregate, dengan kursor per consumer di database.
//
// Pembacaan bebas celah: relay hanya membaca baris dengan tx_id < xmin snapshot saat ini. Semua
// transaksi dengan xid di bawah xmin sudah selesai, jadi kursor tidak pernah melompati transaksi yang
// commit belakangan. Urutan per aggregate mengikuti single writer modul (xid diambil setelah row lock).
package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Event adalah satu event domain di outbox.
type Event struct {
	// ID adalah UUIDv7; diisi Writer bila kosong.
	ID            string
	Topic         string
	AggregateType string
	AggregateKey  string
	// AggregateSeq adalah sequence record pemicu; 0 bila tidak relevan.
	AggregateSeq int32
	Payload      json.RawMessage
	// OccurredAt terisi saat event diterima consumer.
	OccurredAt time.Time
}

// KeySource memberi UUIDv7 untuk event tanpa ID.
type KeySource interface {
	NextKeys(ctx context.Context, n int) ([]string, error)
}

// Writer mengantrekan event ke dalam pgx.Batch milik transaksi command, sehingga event ikut terkirim
// dalam round trip yang sama dengan proyeksi. NOTIFY ikut transaksional: baru sampai saat commit.
type Writer struct {
	keys KeySource
}

// NewWriter membuat writer.
func NewWriter(keys KeySource) *Writer { return &Writer{keys: keys} }

// WakeupChannel adalah kanal LISTEN/NOTIFY untuk membangunkan relay.
const WakeupChannel = "outbox_wakeup"

const insertSQL = `INSERT INTO outbox_event (event_id, topic, aggregate_type, aggregate_key, aggregate_seq, payload)
SELECT unnest($1::uuid[]), unnest($2::text[]), unnest($3::text[]), unnest($4::uuid[]), unnest($5::int[]), unnest($6::jsonb[])`

// Queue menambahkan insert event dan NOTIFY ke batch. ID kosong diisi UUIDv7 baru.
func (w *Writer) Queue(ctx context.Context, b *pgx.Batch, events ...Event) error {
	if len(events) == 0 {
		return nil
	}
	missing := 0
	for i := range events {
		if events[i].ID == "" {
			missing++
		}
	}
	var fresh []string
	if missing > 0 {
		var err error
		if fresh, err = w.keys.NextKeys(ctx, missing); err != nil {
			return fmt.Errorf("outbox event ids: %w", err)
		}
	}
	n := len(events)
	ids, topics, types, keys, seqs, payloads := make([]string, n), make([]string, n), make([]string, n), make([]string, n), make([]int32, n), make([]string, n)
	for i := range events {
		e := &events[i]
		if e.ID == "" {
			e.ID, fresh = fresh[0], fresh[1:]
		}
		if e.Topic == "" || e.AggregateType == "" || e.AggregateKey == "" {
			return errors.New("outbox event needs a topic, aggregate type and aggregate key")
		}
		payload := e.Payload
		if len(payload) == 0 {
			payload = json.RawMessage("{}")
		}
		ids[i], topics[i], types[i], keys[i], seqs[i], payloads[i] = e.ID, e.Topic, e.AggregateType, e.AggregateKey, e.AggregateSeq, string(payload)
	}
	b.Queue(insertSQL, ids, topics, types, keys, seqs, payloads)
	b.Queue(`SELECT pg_notify($1, '')`, WakeupChannel)
	return nil
}

// Enqueue menulis event langsung dalam transaksi (satu round trip sendiri).
func (w *Writer) Enqueue(ctx context.Context, tx pgx.Tx, events ...Event) error {
	b := &pgx.Batch{}
	if err := w.Queue(ctx, b, events...); err != nil {
		return err
	}
	if b.Len() == 0 {
		return nil
	}
	if err := tx.SendBatch(ctx, b).Close(); err != nil {
		return fmt.Errorf("enqueue outbox events: %w", err)
	}
	return nil
}
