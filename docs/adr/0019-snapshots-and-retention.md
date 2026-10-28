# ADR-0019: Snapshot state runtime dan retensi riwayat

- **Status:** Diterima
- **Tanggal:** 2026-10-28

## Konteks

Instance berloop panjang mengumpulkan ribuan record; rebuild dari nol menjadi lambat. Instance
selesai menumpuk tanpa batas.

## Keputusan

- **Snapshot** `execution_snapshot` berisi `domain.Snapshot` (state runtime, JSON) pada sequence
  tertentu, ditulis dalam batch command setiap kali `last_sequence` melewati kelipatan
  `ENGINE_SNAPSHOT_INTERVAL_RECORDS` (default 5.000) untuk instance aktif.
- **`format_version` di kolom** adalah acuan; snapshot versi lain diabaikan dan rebuild mulai dari
  record pertama. Fase berikutnya yang menambah bentuk state wajib menaikkan versi.
- **Rebuild bertahap**: `engine rebuild --instance K` mulai dari snapshot dan memproyeksikan ulang
  hanya record sesudahnya di atas proyeksi yang ada (proyektor idempoten). Ini mengandaikan proyeksi
  sebelum snapshot utuh. `--full` dan perbaikan otomatis saat start selalu menghapus proyeksi lalu
  memutar ulang semua record.
- **Tanpa kompaksi record**: record tetap utuh karena Operate (Fase 6) butuh riwayat.
- **Retensi**: janitor menghapus instance final dengan `end_time` lebih tua dari
  `ENGINE_HISTORY_RETENTION_DAYS` (default 0 = nonaktif) per batch 500 dengan `FOR UPDATE SKIP LOCKED`
  dan jeda `ENGINE_HISTORY_PRUNE_PAUSE`; semua tabel state, record, dan snapshot ikut terhapus. Tanpa
  tabel arsip; operator yang butuh arsip memakai `make db-backup`. `engine prune --older-than 90d
  [--dry-run]` menjalankannya sekali.
- **Riwayat berhalaman**: `ListProcessInstanceRecords` (keyset `sequence`) dan
  `ListElementInstances` (keyset key) dengan token opak; `GetProcessInstance` memotong element
  instance di 1.000 dengan flag `element_instances_truncated`.

## Konsekuensi

- Test: 10.000 instance selesai dihapus sementara 50 instance aktif terus diproses tanpa error dan
  tetap lolos verifikasi; rebuild dari snapshot, rebuild penuh, dan fallback format menghasilkan
  proyeksi identik.
- Daftar `ListProcessInstances` dan `ListJobs` dari Fase 2 masih memakai key mentah sebagai token
  halaman; tetap opak bagi klien.
