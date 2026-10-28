// Package metrics membungkus API metrik OpenTelemetry untuk engine. Nama dan label metrik dikunci di
// sini (test membandingkannya dengan daftar tetap); provider default no-op sampai Fase 8 memasang
// exporter lewat otel.SetMeterProvider, tanpa mengubah instrumentasi.
package metrics

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Nama metrik yang terdaftar. Mengubah nama adalah perubahan kontrak bagi dashboard (Fase 8).
const (
	OutboxDelivered         = "engine_outbox_delivered_total"
	OutboxFailed            = "engine_outbox_failed_total"
	OutboxDeadLetter        = "engine_outbox_dead_letter_total"
	RecoveryDuration        = "engine_recovery_duration_seconds"
	RecoveryMismatch        = "engine_recovery_mismatch_total"
	IdempotencyTotal        = "engine_idempotency_total"
	CommandDuration         = "engine_command_duration_seconds"
	CommandLockWait         = "engine_command_lock_wait_seconds"
	HistoryPrunedInstances  = "engine_history_pruned_instances_total"
	DBPoolAcquireWait       = "engine_db_pool_acquire_wait_seconds"
	OutboxPendingEvents     = "engine_outbox_pending_events"
	OutboxLagSeconds        = "engine_outbox_lag_seconds"
	JobsActivated           = "engine_jobs_activated_total"
	JobTimeouts             = "engine_job_timeouts_total"
	LongPollsActive         = "engine_job_long_polls_active"
	SweeperSwept            = "engine_sweeper_swept_total"
	MessagesPublished       = "engine_messages_published_total"
	instrumentationScope    = "github.com/indr622/workflow-engine/apps/engine"
	unitSeconds, unitEvents = "s", "{event}"
)

// Names mengembalikan semua nama metrik terdaftar (urut sesuai deklarasi).
func Names() []string {
	return []string{
		OutboxDelivered, OutboxFailed, OutboxDeadLetter, RecoveryDuration, RecoveryMismatch, IdempotencyTotal,
		CommandDuration, CommandLockWait, HistoryPrunedInstances, DBPoolAcquireWait, OutboxPendingEvents, OutboxLagSeconds,
		JobsActivated, JobTimeouts, LongPollsActive, SweeperSwept, MessagesPublished,
	}
}

// Instruments adalah instrumen yang dipakai komponen engine.
type Instruments struct {
	OutboxDelivered        metric.Int64Counter
	OutboxFailed           metric.Int64Counter
	OutboxDeadLetter       metric.Int64Counter
	RecoveryDuration       metric.Float64Histogram
	RecoveryMismatch       metric.Int64Counter
	Idempotency            metric.Int64Counter
	CommandDuration        metric.Float64Histogram
	CommandLockWait        metric.Float64Histogram
	HistoryPrunedInstances metric.Int64Counter
	DBPoolAcquireWait      metric.Float64Histogram
	JobsActivated          metric.Int64Counter
	JobTimeouts            metric.Int64Counter
	LongPollsActive        metric.Int64UpDownCounter
	SweeperSwept           metric.Int64Counter
	MessagesPublished      metric.Int64Counter
}

var (
	once sync.Once
	inst *Instruments
)

// Get mengembalikan instrumen dari meter global. Meter global mendelegasikan ke provider yang
// dipasang belakangan, jadi instrumen yang dibuat sekarang ikut terekspor setelah Fase 8.
func Get() *Instruments {
	once.Do(func() { inst = build(otel.Meter(instrumentationScope)) })
	return inst
}

func build(m metric.Meter) *Instruments {
	i := &Instruments{}
	// Error pembuatan instrumen hanya terjadi untuk nama tidak valid; nama di atas sudah tetap,
	// dan API OTel tetap mengembalikan instrumen no-op yang aman dipakai.
	i.OutboxDelivered, _ = m.Int64Counter(OutboxDelivered, metric.WithUnit(unitEvents), metric.WithDescription("Events delivered to durable consumers"))
	i.OutboxFailed, _ = m.Int64Counter(OutboxFailed, metric.WithUnit(unitEvents), metric.WithDescription("Failed event deliveries"))
	i.OutboxDeadLetter, _ = m.Int64Counter(OutboxDeadLetter, metric.WithUnit(unitEvents), metric.WithDescription("Events moved to the dead letter table"))
	i.RecoveryDuration, _ = m.Float64Histogram(RecoveryDuration, metric.WithUnit(unitSeconds), metric.WithDescription("Startup recovery duration per phase"))
	i.RecoveryMismatch, _ = m.Int64Counter(RecoveryMismatch, metric.WithDescription("Projection mismatches found during recovery"))
	i.Idempotency, _ = m.Int64Counter(IdempotencyTotal, metric.WithDescription("request_id claims by outcome"))
	i.CommandDuration, _ = m.Float64Histogram(CommandDuration, metric.WithUnit(unitSeconds), metric.WithDescription("Command duration"))
	i.CommandLockWait, _ = m.Float64Histogram(CommandLockWait, metric.WithUnit(unitSeconds), metric.WithDescription("Time waiting for the instance row lock"))
	i.HistoryPrunedInstances, _ = m.Int64Counter(HistoryPrunedInstances, metric.WithDescription("Finished instances deleted by retention"))
	i.DBPoolAcquireWait, _ = m.Float64Histogram(DBPoolAcquireWait, metric.WithUnit(unitSeconds), metric.WithDescription("Time waiting for a pool connection"))
	i.JobsActivated, _ = m.Int64Counter(JobsActivated, metric.WithDescription("Jobs leased to workers by job type"))
	i.JobTimeouts, _ = m.Int64Counter(JobTimeouts, metric.WithDescription("Job leases that expired before completion"))
	i.LongPollsActive, _ = m.Int64UpDownCounter(LongPollsActive, metric.WithDescription("ActivateJobs requests currently waiting"))
	i.SweeperSwept, _ = m.Int64Counter(SweeperSwept, metric.WithDescription("Rows handled by sweeper tasks"))
	i.MessagesPublished, _ = m.Int64Counter(MessagesPublished, metric.WithDescription("Messages published by outcome"))
	return i
}

// Attr membuat opsi atribut untuk Add/Record.
func Attr(kv ...string) metric.MeasurementOption {
	attrs := make([]attribute.KeyValue, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		attrs = append(attrs, attribute.String(kv[i], kv[i+1]))
	}
	return metric.WithAttributes(attrs...)
}

// Since merekam durasi sejak start pada histogram detik.
func Since(ctx context.Context, h metric.Float64Histogram, start time.Time, opts ...metric.RecordOption) {
	h.Record(ctx, time.Since(start).Seconds(), opts...)
}
