# platform/outbox

Outbox transaksional engine (ADR-0015). Modul menulis event dalam transaksi command yang sama dengan
state; `Relay` mengirimnya ke consumer durable.

## Menulis event

```go
writer := outbox.NewWriter(keys)              // keys: generator UUIDv7
writer.Queue(ctx, batch, events...)           // ikut pgx.Batch command (tanpa round trip tambahan)
writer.Enqueue(ctx, tx, events...)            // atau langsung di dalam pgx.Tx
```

`Event.ID` diisi UUIDv7 bila kosong. `AggregateKey` wajib; `AggregateSeq` dipakai consumer yang peka
urutan (eksekusi mengisinya dengan sequence record pemicu).

## Kontrak consumer

```go
relay.Register(outbox.Consumer{
    Name:        "timer.scheduler",          // unik dan stabil lintas versi
    Topics:      []string{"execution.job_created"},
    Handle:      func(ctx context.Context, tx pgx.Tx, ev outbox.Event) error { ... },
    MaxAttempts: 10,
    StartFrom:   outbox.StartNow,            // atau StartBeginning (hanya event dalam retensi)
})
```

- `Handle` berjalan di dalam transaksi relay yang memajukan kursor: efek DB-nya commit bersama
  kursor (exactly-once). Efek di luar DB wajib idempoten berdasarkan `ev.ID`, karena batch bisa
  dikirim ulang (at-least-once).
- Urutan dijamin per aggregate, bukan antar aggregate.
- Satu replika memegang satu consumer pada satu waktu (`pg_try_advisory_lock`).
- Event yang terus gagal masuk `outbox_dead_letter` setelah `MaxAttempts`; kirim ulang dengan
  `engine outbox requeue --consumer C --event-id E`.
- Hapus kursor consumer yang tidak dipakai lagi (`DELETE FROM outbox_cursor WHERE consumer = ...`);
  kursor tertinggal menahan prune.

## Operasional

| Perintah / variabel | Fungsi |
|---|---|
| `engine outbox status` | Pending (semua topik di belakang kursor), attempts, dead letter per consumer |
| `ENGINE_OUTBOX_POLL_INTERVAL`, `ENGINE_OUTBOX_BATCH_SIZE` | Interval poll cadangan (LISTEN untuk jalur cepat), ukuran batch |
| `ENGINE_OUTBOX_RETENTION` | Umur minimum event sebelum dihapus janitor (setelah dilewati semua kursor) |
