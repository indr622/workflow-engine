# ADR-0018: Row lock per instance dipertahankan; kesiapan partisi

- **Status:** Diterima
- **Tanggal:** 2026-10-28

## Konteks

Rencana Fase 3 bertanya apakah row lock per instance (ADR-0012) cukup, atau engine perlu
"instance affinity" (routing command ke replika pemilik) atau partisi. Key sudah UUIDv7 (ADR-0011),
jadi pertanyaan tata letak bit partisi tidak berlaku lagi.

## Data (i5-1145G7, Windows 11, Postgres 18 di Docker Desktop, round trip ~1,2 ms)

| Beban | p50 | p99 |
|---|---|---|
| 50 `CompleteJob` serentak pada satu instance (5 kali) | 296 ms | 721 ms |
| 500 `CompleteJob` serentak pada instance berbeda | 1,83 s | 2,68 s |

Satu instance: command diserialkan row lock dengan biaya sekitar 14 ms per command di mesin ini,
jadi command ke-50 menunggu sekitar 700 ms. Instance berbeda: hambatannya ukuran pool (16 koneksi),
bukan lock; throughput sekitar 185 command per detik.

## Keputusan

- **Row lock per instance dipertahankan.** Contention hanya terjadi di dalam satu instance dan
  sebanding dengan fan-out paralel yang selesai bersamaan. Instance affinity tidak membantu beban
  lintas instance, yang dibatasi pool dan round trip.
- **Kesiapan partisi**: semua tabel state punya `process_instance_key`; bila kelak perlu membagi
  kepemilikan instance antar replika, routing memakai hash `process_instance_key`, bukan isi key.
  Tidak ada kode yang mengasumsikan key rapat; urutan per instance hanya lewat `sequence`.
- **Pemicu peninjauan ulang**: p99 `engine_command_lock_wait_seconds` di atas 50 ms pada beban
  produksi nyata (bukan fan-out 50 serentak buatan), atau throughput yang dibatasi lock setelah pool
  dan round trip dioptimalkan. Angka dari job integrasi CI di Linux dicatat sebagai pembanding.

## Konsekuensi

- Model dengan fan-out paralel sangat lebar yang selesai bersamaan akan melihat latensi bertingkat.
- Ukuran pool (`ENGINE_DB_MAX_CONNS`) adalah tuas kapasitas pertama, dibatasi
  `replika × ENGINE_DB_MAX_CONNS + 10 ≤ max_connections`.
