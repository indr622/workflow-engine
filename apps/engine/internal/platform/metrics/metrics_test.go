package metrics

import (
	"context"
	"reflect"
	"testing"
	"time"
)

// TestNamesAreStable: daftar nama metrik adalah kontrak dashboard; perubahan harus disengaja.
func TestNamesAreStable(t *testing.T) {
	want := []string{
		"engine_outbox_delivered_total", "engine_outbox_failed_total", "engine_outbox_dead_letter_total",
		"engine_recovery_duration_seconds", "engine_recovery_mismatch_total", "engine_idempotency_total",
		"engine_command_duration_seconds", "engine_command_lock_wait_seconds", "engine_history_pruned_instances_total",
		"engine_db_pool_acquire_wait_seconds", "engine_outbox_pending_events", "engine_outbox_lag_seconds",
	}
	if got := Names(); !reflect.DeepEqual(got, want) {
		t.Fatalf("metric names changed:\ngot  %v\nwant %v", got, want)
	}
}

func TestInstrumentsAreUsableWithNoopProvider(t *testing.T) {
	i := Get()
	if i != Get() {
		t.Fatal("instruments must be created once")
	}
	ctx := context.Background()
	i.OutboxDelivered.Add(ctx, 1, Attr("consumer", "test", "topic", "t"))
	Since(ctx, i.CommandDuration, time.Now(), Attr("command", "create"))
}
