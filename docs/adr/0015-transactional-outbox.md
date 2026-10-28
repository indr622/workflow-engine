# ADR-0015: Dua tingkat pengiriman event: outbox transaksional dan sinyal in-process

- **Status:** Diterima
- **Tanggal:** 2026-10-28

## Konteks

Fase 1 dan 2 mempublikasikan event antarmodul ke bus in-process setelah commit. Bila proses mati di
antara commit dan publikasi, event hilang. Fase 5 (timer, message) dan Fase 6 (proyeksi Operate)
tidak boleh kehilangan event; Fase 4 (long polling) hanya butuh "bangunkan saya".

## Keputusan

- **Outbox generik milik platform** (`internal/platform/outbox`, tabel `outbox_event`). Modul menulis
  event dalam transaksi command yang sama dengan state. Eksekusi mengantrekan baris outbox ke
  `pgx.Batch` yang sama dengan proyeksi, sehingga tidak ada round trip tambahan; `aggregate_seq`
  adalah sequence record pemicu.
- **Tanpa kolom auto-increment** (ADR-0011): `event_id` adalah UUIDv7 dari generator engine, yang
  monotonik di dalam satu proses sehingga urutan dalam satu transaksi terjaga.
- **Pembacaan bebas celah**: kolom `tx_id xid8 DEFAULT pg_current_xact_id()`; relay hanya membaca
  baris dengan `tx_id < pg_snapshot_xmin(pg_current_snapshot())`, urut dan berkursor
  `(tx_id, event_id)`. Semua transaksi di bawah xmin sudah selesai, jadi kursor tidak pernah
  melompati transaksi yang commit belakangan, tanpa lock global atau jendela waktu.
- **Urutan per aggregate** mengikuti single writer: xid dialokasikan setelah row lock (klaim
  idempotensi juga setelah lock), sehingga urutan `tx_id` sama dengan urutan command per instance.
- **Consumer durable**: kursor per nama consumer, satu pemegang per consumer lewat
  `pg_try_advisory_lock`, handler dipanggil di dalam transaksi relay yang memajukan kursor (efek DB
  handler exactly-once), at-least-once untuk efek di luar DB, `NOTIFY outbox_wakeup` untuk latensi
  rendah. Bila handler gagal di tengah batch, event sebelumnya di-commit dulu; event bermasalah
  dicoba ulang dengan backoff lalu masuk `outbox_dead_letter` setelah `MaxAttempts`.
- **Sinyal in-process** (`eventbus.InMemory`) tetap dikirim setelah commit sebagai jalur cepat
  best-effort.
- Retensi: event dihapus janitor setelah dilewati semua kursor dan lebih tua dari
  `ENGINE_OUTBOX_RETENTION`.

## Konsekuensi

- Event tidak hilang saat crash; terbukti oleh test 200 transaksi dengan commit acak, crash setelah
  commit, dan crash sebelum kursor relay di-commit.
- Transaksi lama menahan xmin dan menunda pengiriman untuk semua consumer; dibatasi
  `statement_timeout` dan `idle_in_transaction_session_timeout`.
- `outbox status` menghitung pending di semua topik di belakang kursor, bukan per topik consumer.
- Kursor consumer yang ditinggalkan menahan prune; consumer yang dihapus harus dihapus juga
  kursornya.
