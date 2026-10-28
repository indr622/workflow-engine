//go:build integration && faultinject

package infrastructure_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/application"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/faultpoint"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/outbox"
)

// Skenario 2, 3, 11 tabel 11.3 dan kegagalan di tengah transaksi command, dalam satu proses.

func faultRelay(t *testing.T, f *fixture, consumer string) (*outbox.Relay, func() int) {
	t.Helper()
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS fault_test_received (
		consumer TEXT NOT NULL, event_id UUID NOT NULL, aggregate_key UUID NOT NULL, PRIMARY KEY (consumer, event_id))`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM outbox_cursor WHERE consumer = $1`, consumer)
	})
	relay := outbox.NewRelay(f.pool, slog.New(slog.NewTextHandler(io.Discard, nil)), outbox.Options{PollInterval: 20 * time.Millisecond})
	if err := relay.Register(outbox.Consumer{
		Name: consumer, Topics: []string{api.TopicJobCreated}, StartFrom: outbox.StartNow,
		Handle: func(ctx context.Context, tx pgx.Tx, ev outbox.Event) error {
			_, err := tx.Exec(ctx, `INSERT INTO fault_test_received (consumer, event_id, aggregate_key) VALUES ($1, $2::uuid, $3::uuid)`,
				consumer, ev.ID, ev.AggregateKey)
			return err // duplikat → error unique violation: harus tidak pernah terjadi
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := relay.Drain(ctx); err != nil { // buat kursor sebelum beban
		t.Fatal(err)
	}
	count := func() int {
		var n int
		if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM fault_test_received WHERE consumer = $1`, consumer).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	return relay, count
}

// recovered menjalankan fn dan mengubah panic (crash tiruan) menjadi nilai.
func recovered(fn func()) (crashed bool) {
	defer func() {
		if recover() != nil {
			crashed = true
		}
	}()
	fn()
	return false
}

// Skenario 2: proses mati setelah commit, sebelum sinyal in-process; event tetap terkirim via outbox.
func TestCrashAfterCommitStillDeliversEvent(t *testing.T) {
	f := setup(t)
	defer faultpoint.Reset()
	ctx := context.Background()
	processID := f.deployXML(t, "fault-after-commit", linearXML)
	relay, received := faultRelay(t, f, "fault.after_commit."+f.run)

	faultpoint.Set(faultpoint.CommandAfterCommit, func(context.Context) error { panic("crash after commit") })
	var created api.CreateInstanceResult
	crashed := recovered(func() {
		created, _ = f.commands.CreateProcessInstance(ctx, api.CreateInstanceRequest{BpmnProcessID: processID})
	})
	faultpoint.Reset()
	if !crashed {
		t.Fatal("fault point did not fire")
	}
	listed, _, err := f.queries.ListInstances(ctx, instanceFilter(processID))
	if err != nil || len(listed) != 1 || created.ProcessInstanceKey != "" {
		t.Fatalf("instance must be committed despite the crash: %d instances, %v", len(listed), err)
	}
	if err := relay.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if n := received(); n != 1 {
		t.Fatalf("consumer received %d job_created events, want 1", n)
	}
}

// Skenario 3: relay mati setelah handler, sebelum kursor commit; efek DB handler ikut rollback dan
// event dikirim ulang tepat sekali.
func TestCrashBeforeCursorCommitRedeliversOnce(t *testing.T) {
	f := setup(t)
	defer faultpoint.Reset()
	ctx := context.Background()
	processID := f.deployXML(t, "fault-cursor", linearXML)
	relay, received := faultRelay(t, f, "fault.cursor."+f.run)
	if _, err := f.commands.CreateProcessInstance(ctx, api.CreateInstanceRequest{BpmnProcessID: processID}); err != nil {
		t.Fatal(err)
	}
	faultpoint.Set(faultpoint.OutboxBeforeCursorCommit, func(context.Context) error { panic("relay crash") })
	if !recovered(func() { _ = relay.Drain(ctx) }) {
		t.Fatal("fault point did not fire")
	}
	faultpoint.Reset()
	if n := received(); n != 0 {
		t.Fatalf("handler effect survived the crashed batch: %d rows", n)
	}
	if err := relay.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if n := received(); n != 1 {
		t.Fatalf("after restart the consumer has %d rows, want 1", n)
	}
}

// Skenario 11: separuh command commit terlambat 1,5 detik; relay yang berjalan tidak melewatkan event.
func TestSlowCommitsDoNotHideEvents(t *testing.T) {
	f := setup(t)
	defer faultpoint.Reset()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	processID := f.deployXML(t, "fault-slow-commit", linearXML)
	relay, received := faultRelay(t, f, "fault.slow."+f.run)
	go func() { _ = relay.Run(ctx) }()

	var hits atomic.Int64
	faultpoint.Set(faultpoint.CommandBeforeCommit, func(ctx context.Context) error {
		if hits.Add(1)%2 == 0 {
			time.Sleep(1500 * time.Millisecond)
		}
		return nil
	})
	var wg sync.WaitGroup
	errs := make(chan error, 40)
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := f.commands.CreateProcessInstance(ctx, api.CreateInstanceRequest{BpmnProcessID: processID}); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for received() < 40 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if n := received(); n != 40 {
		t.Fatalf("consumer received %d of 40 events", n)
	}
}

// Kegagalan setelah append (sebelum commit) dan setelah klaim request_id: tidak ada yang tersimpan,
// dan retry dengan request_id yang sama menjalankan command (bukan replay).
func TestFailureInsideCommandLeavesNothing(t *testing.T) {
	f := setup(t)
	defer faultpoint.Reset()
	ctx := context.Background()
	processID := f.deployXML(t, "fault-inside", linearXML)
	idem := &api.Idempotency{Scope: "<default>/test", Key: "fault-" + f.run}

	for _, point := range []string{faultpoint.CommandAfterAppend, faultpoint.IdempotencyAfterClaim} {
		faultpoint.Set(point, func(context.Context) error { return faultpoint.ErrInjected })
		_, err := f.commands.CreateProcessInstance(ctx, api.CreateInstanceRequest{BpmnProcessID: processID, Idempotency: idem})
		faultpoint.Reset()
		if !errors.Is(err, faultpoint.ErrInjected) {
			t.Fatalf("%s: error = %v", point, err)
		}
		listed, _, err := f.queries.ListInstances(ctx, instanceFilter(processID))
		if err != nil || len(listed) != 0 {
			t.Fatalf("%s: %d instances persisted, %v", point, len(listed), err)
		}
		var claims int
		if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM idempotency_key WHERE key = $1`, idem.Key).Scan(&claims); err != nil || claims != 0 {
			t.Fatalf("%s: claim persisted (%d), %v", point, claims, err)
		}
	}
	res, err := f.commands.CreateProcessInstance(ctx, api.CreateInstanceRequest{BpmnProcessID: processID, Idempotency: idem})
	if err != nil || res.Replayed {
		t.Fatalf("retry after failures: replayed=%v, %v", res.Replayed, err)
	}
}

func instanceFilter(processID string) application.InstanceFilter {
	return application.InstanceFilter{BpmnProcessID: processID, Limit: 100}
}
