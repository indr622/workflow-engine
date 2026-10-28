# ADR-0017: Idempotensi RPC mutasi dengan request_id

- **Status:** Diterima
- **Tanggal:** 2026-10-28

## Konteks

Klien yang mengalami timeout tidak tahu apakah command sudah dijalankan. Mengulang tanpa
perlindungan bisa membuat instance ganda.

## Keputusan

- **`request_id` opsional di body proto** (1 sampai 128 byte UTF-8) untuk
  `CreateProcessInstance`, `CompleteJob`, `CancelProcessInstance`, dan `DeployResource`. Perubahan
  proto aditif; `buf breaking` lolos.
- **Scope klaim** = tenant + nama prosedur Connect lengkap, sehingga `request_id` yang sama untuk RPC
  berbeda tidak bertabrakan. Hash request = SHA-256 marshal proto deterministik dengan `request_id`
  dikosongkan.
- **Klaim di dalam transaksi command** lewat `INSERT ... ON CONFLICT DO NOTHING` ke
  `idempotency_key`, **setelah lock aggregate** (row lock instance, atau semua advisory lock proses
  untuk deploy); untuk create, di awal transaksi. Duplikat paralel menunggu di unique index sampai
  transaksi pertama selesai, lalu membaca response tersimpan. Response disimpan dalam batch yang
  sama dengan record.
- **Hasil**: request identik → response tersimpan byte-identik dengan header
  `Idempotent-Replayed: true`; payload berbeda → `INVALID_ARGUMENT` dengan detail
  `workflow.common.v1.IdempotencyKeyReused`; error domain membatalkan transaksi sehingga klaim tidak
  tersimpan dan retry menjalankan ulang; klaim kedaluwarsa (`ENGINE_IDEMPOTENCY_TTL`, default 24 jam)
  diperlakukan sebagai baru dan dibersihkan janitor.

## Konsekuensi

- Dua puluh request paralel dengan `request_id` sama menghasilkan satu instance dan 19 replay.
- Upgrade protobuf-go yang mengubah marshal deterministik bisa membuat retry sah ditolak dalam masa
  TTL; upgrade seperti itu dicatat sebagai perubahan yang mengosongkan `idempotency_key`.
- `ActivateJobs` (Fase 4) tidak idempoten by design; RPC mutasi baru mengikuti pola ini.
