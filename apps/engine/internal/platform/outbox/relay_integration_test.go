//go:build integration

package outbox_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	platformdb "github.com/indr622/workflow-engine/apps/engine/internal/platform/db"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/keygen"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/outbox"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func setup(t *testing.T) (*pgxpool.Pool, *outbox.Writer, string) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := platformdb.Migrate(ctx, pool, quiet); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS outbox_test_received (
		consumer TEXT NOT NULL, event_id UUID NOT NULL, handler TEXT NOT NULL, aggregate_seq INT NOT NULL,
		received_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(), PRIMARY KEY (consumer, event_id))`); err != nil {
		t.Fatal(err)
	}
	prefix := fmt.Sprintf("test.%d", time.Now().UnixNano())
	// Kursor consumer uji dihapus agar tidak menahan prune outbox di database bersama.
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM outbox_cursor WHERE consumer LIKE $1`, prefix+"%")
		_, _ = pool.Exec(context.Background(), `DELETE FROM outbox_dead_letter WHERE consumer LIKE $1`, prefix+"%")
	})
	return pool, outbox.NewWriter(keygen.New()), prefix
}

// recorder mencatat event ke tabel uji di dalam transaksi relay; duplikat dihitung.
type recorder struct {
	consumer   string
	handler    string
	duplicates atomic.Int64
}

func (r *recorder) handle(ctx context.Context, tx pgx.Tx, ev outbox.Event) error {
	tag, err := tx.Exec(ctx, `INSERT INTO outbox_test_received (consumer, event_id, handler, aggregate_seq)
		VALUES ($1, $2::uuid, $3, $4) ON CONFLICT DO NOTHING`, r.consumer, ev.ID, r.handler, ev.AggregateSeq)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		r.duplicates.Add(1)
	}
	return nil
}

func received(t *testing.T, pool *pgxpool.Pool, consumer string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM outbox_test_received WHERE consumer = $1`, consumer).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func enqueue(t *testing.T, pool *pgxpool.Pool, w *outbox.Writer, topic, aggregate string, seq int32, delay time.Duration) string {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Error(err)
		return ""
	}
	defer func() { _ = tx.Rollback(ctx) }()
	ev := outbox.Event{Topic: topic, AggregateType: "test", AggregateKey: aggregate, AggregateSeq: seq, Payload: []byte(`{"n":1}`)}
	if err := w.Enqueue(ctx, tx, ev); err != nil {
		t.Error(err)
		return ""
	}
	time.Sleep(delay)
	if err := tx.Commit(ctx); err != nil {
		t.Error(err)
	}
	return ""
}

func newKey(t *testing.T) string {
	t.Helper()
	k, err := keygen.New().NewKey()
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// TestNoEventMissedWithOutOfOrderCommits: 200 transaksi paralel commit dengan jeda acak (urutan
// commit berbeda dari urutan xid) sementara relay berjalan; tidak ada event yang terlewat.
func TestNoEventMissedWithOutOfOrderCommits(t *testing.T) {
	pool, w, topic := setup(t)
	consumer := topic + ".collector"
	rec := &recorder{consumer: consumer, handler: "relay"}
	relay := outbox.NewRelay(pool, quiet, outbox.Options{PollInterval: 20 * time.Millisecond, BatchSize: 25})
	if err := relay.Register(outbox.Consumer{Name: consumer, Topics: []string{topic}, Handle: rec.handle, StartFrom: outbox.StartBeginning}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = relay.Run(ctx) }()

	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			delay := time.Duration(rand.IntN(80)) * time.Millisecond //nolint:gosec // jeda uji
			enqueue(t, pool, w, topic, newKey(t), int32(i), delay)
		}(i)
	}
	wg.Wait()
	waitFor(t, 30*time.Second, func() bool { return received(t, pool, consumer) == 200 })
	if d := rec.duplicates.Load(); d != 0 {
		t.Fatalf("%d duplicate deliveries inside the relay transaction", d)
	}
}

// TestFailingEventIsRetriedThenDeadLettered: event beracun masuk dead letter setelah MaxAttempts,
// event lain tetap terkirim, kursor maju.
func TestFailingEventIsRetriedThenDeadLettered(t *testing.T) {
	pool, w, topic := setup(t)
	consumer := topic + ".poison"
	rec := &recorder{consumer: consumer, handler: "drain"}
	for i := int32(1); i <= 5; i++ {
		enqueue(t, pool, w, topic, newKey(t), i, 0)
	}
	attempts := atomic.Int64{}
	relay := outbox.NewRelay(pool, quiet, outbox.Options{})
	err := relay.Register(outbox.Consumer{
		Name: consumer, Topics: []string{topic}, MaxAttempts: 3, StartFrom: outbox.StartBeginning,
		Handle: func(ctx context.Context, tx pgx.Tx, ev outbox.Event) error {
			if ev.AggregateSeq == 3 {
				attempts.Add(1)
				return errors.New("poison event")
			}
			return rec.handle(ctx, tx, ev)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := relay.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := received(t, pool, consumer); got != 4 {
		t.Fatalf("delivered %d events, want 4", got)
	}
	if attempts.Load() != 3 {
		t.Fatalf("poison event attempted %d times, want 3", attempts.Load())
	}
	var dead int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM outbox_dead_letter WHERE consumer = $1`, consumer).Scan(&dead); err != nil || dead != 1 {
		t.Fatalf("dead letters = %d, %v", dead, err)
	}
	statuses, err := outbox.Status(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range statuses {
		// Pending menghitung semua topik di belakang kursor; paket test lain bisa menulis event
		// bersamaan, jadi di sini hanya dead letter yang diperiksa dari status.
		if s.Consumer == consumer && s.DeadLetters != 1 {
			t.Fatalf("status = %+v", s)
		}
	}
	var behind int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM outbox_event e JOIN outbox_cursor c ON c.consumer = $1
		WHERE e.topic = $2 AND (e.tx_id, e.event_id) > (c.tx_id, c.event_id)`, consumer, topic).Scan(&behind); err != nil || behind != 0 {
		t.Fatalf("%d events of the topic still behind the cursor, %v", behind, err)
	}
}

// TestRedeliveryIsAtLeastOnce: handler yang gagal setelah efek di luar DB membuat event dikirim
// ulang; efek luar terjadi dua kali, efek DB (atomik dengan kursor) hanya sekali.
func TestRedeliveryIsAtLeastOnce(t *testing.T) {
	pool, w, topic := setup(t)
	consumer := topic + ".redeliver"
	rec := &recorder{consumer: consumer, handler: "drain"}
	enqueue(t, pool, w, topic, newKey(t), 1, 0)
	var external atomic.Int64
	relay := outbox.NewRelay(pool, quiet, outbox.Options{})
	err := relay.Register(outbox.Consumer{
		Name: consumer, Topics: []string{topic}, StartFrom: outbox.StartBeginning,
		Handle: func(ctx context.Context, tx pgx.Tx, ev outbox.Event) error {
			if err := rec.handle(ctx, tx, ev); err != nil {
				return err
			}
			if external.Add(1) == 1 {
				return errors.New("crash after the external effect")
			}
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := relay.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if external.Load() != 2 || received(t, pool, consumer) != 1 {
		t.Fatalf("external effects = %d, db effects = %d; want 2 and 1", external.Load(), received(t, pool, consumer))
	}
}

// TestTwoRelaysOneHolder: dua relay (dua replika) untuk consumer yang sama; advisory lock membuat
// hanya satu yang memproses, dan setiap event diproses sekali.
func TestTwoRelaysOneHolder(t *testing.T) {
	pool, w, topic := setup(t)
	consumer := topic + ".replicas"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	recs := []*recorder{{consumer: consumer, handler: "replica-a"}, {consumer: consumer, handler: "replica-b"}}
	for _, rec := range recs {
		relay := outbox.NewRelay(pool, quiet, outbox.Options{PollInterval: 20 * time.Millisecond, LockRetry: 100 * time.Millisecond})
		if err := relay.Register(outbox.Consumer{Name: consumer, Topics: []string{topic}, Handle: rec.handle, StartFrom: outbox.StartBeginning}); err != nil {
			t.Fatal(err)
		}
		go func() { _ = relay.Run(ctx) }()
	}
	for i := int32(1); i <= 100; i++ {
		enqueue(t, pool, w, topic, newKey(t), i, 0)
	}
	waitFor(t, 30*time.Second, func() bool { return received(t, pool, consumer) == 100 })
	var handlers int
	if err := pool.QueryRow(context.Background(), `SELECT count(DISTINCT handler) FROM outbox_test_received WHERE consumer = $1`, consumer).Scan(&handlers); err != nil {
		t.Fatal(err)
	}
	if handlers != 1 {
		t.Fatalf("events handled by %d replicas, want 1", handlers)
	}
	if recs[0].duplicates.Load()+recs[1].duplicates.Load() != 0 {
		t.Fatal("duplicate deliveries")
	}
}

// TestPerAggregateOrder: event satu aggregate yang ditulis berurutan (single writer) diterima urut.
func TestPerAggregateOrder(t *testing.T) {
	pool, w, topic := setup(t)
	consumer := topic + ".order"
	aggregate := newKey(t)
	for i := int32(1); i <= 50; i++ {
		enqueue(t, pool, w, topic, aggregate, i, 0)
	}
	var mu sync.Mutex
	var seqs []int32
	relay := outbox.NewRelay(pool, quiet, outbox.Options{BatchSize: 7})
	err := relay.Register(outbox.Consumer{
		Name: consumer, Topics: []string{topic}, StartFrom: outbox.StartBeginning,
		Handle: func(_ context.Context, _ pgx.Tx, ev outbox.Event) error {
			mu.Lock()
			defer mu.Unlock()
			seqs = append(seqs, ev.AggregateSeq)
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := relay.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(seqs) != 50 {
		t.Fatalf("received %d events", len(seqs))
	}
	for i, s := range seqs {
		if s != int32(i+1) {
			t.Fatalf("event %d has aggregate_seq %d", i, s)
		}
	}
}

func TestPruneRemovesOnlyConsumedEvents(t *testing.T) {
	pool, w, topic := setup(t)
	ctx := context.Background()
	consumer := topic + ".prune"
	enqueue(t, pool, w, topic, newKey(t), 1, 0)
	relay := outbox.NewRelay(pool, quiet, outbox.Options{})
	rec := &recorder{consumer: consumer, handler: "drain"}
	if err := relay.Register(outbox.Consumer{Name: consumer, Topics: []string{topic}, Handle: rec.handle, StartFrom: outbox.StartBeginning}); err != nil {
		t.Fatal(err)
	}
	if err := relay.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	// Event masih di bawah kursor consumer lain (consumer test lama) sehingga mungkin tidak terhapus;
	// yang dijamin: event yang belum dilewati SEMUA kursor tidak pernah dihapus.
	lagging := topic + ".lagging"
	if _, err := pool.Exec(ctx, `INSERT INTO outbox_cursor (consumer, tx_id, event_id) VALUES ($1, '0'::xid8, '00000000-0000-0000-0000-000000000000')`, lagging); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = pool.Exec(ctx, `DELETE FROM outbox_cursor WHERE consumer = $1`, lagging) }()
	if _, err := outbox.Prune(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM outbox_event WHERE topic = $1`, topic).Scan(&left); err != nil || left != 1 {
		t.Fatalf("lagging consumer lost its event: %d left, %v", left, err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM outbox_cursor WHERE consumer = $1`, lagging); err != nil {
		t.Fatal(err)
	}
	if _, err := outbox.Prune(ctx, pool, 0); err != nil {
		t.Fatal(err)
	}
}
