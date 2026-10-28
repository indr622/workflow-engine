# ADR-0016: Record sebagai sumber kebenaran; verify dan rebuild proyeksi

- **Status:** Diterima
- **Tanggal:** 2026-10-28

## Konteks

Fase 2 menulis record append-only dan proyeksi dalam satu transaksi. Tanpa cara memeriksa dan
membangun ulang proyeksi, bug proyektor hanya bisa diperbaiki dengan restore backup.

## Keputusan

- **`execution_record` adalah sumber kebenaran.** Tabel state adalah proyeksi yang bisa dihapus dan
  dibangun ulang dengan `domain.Apply` + proyektor yang sama dengan jalur command.
- **Payload record bertipe `JSON`, bukan `JSONB`.** JSONB menormalkan isi (spasi, urutan key,
  key ganda) sehingga nilai variabel hasil replay berbeda dari input; ditemukan oleh test rebuild.
- **Proyektor idempoten**: insert memakai `ON CONFLICT`, penanda proyeksi (`last_sequence`,
  `version`, `updated_at`) diturunkan dari record terakhir sehingga rebuild menghasilkan baris
  identik byte per byte. `version` kini sama dengan `last_sequence` karena batas command tidak
  tercatat di record.
- **Verifikasi saat start** dua tingkat: cepat (SQL agregat atas semua instance: celah sequence,
  instance tanpa record, state akhir, job yatim, `has_incident`) dan dalam (replay in-memory lalu
  bandingkan dengan state runtime proyeksi) untuk instance aktif yang berubah dalam
  `ENGINE_RECOVERY_DEEP_WINDOW` ditambah sampel acak. Pembacaan verifikasi dalam memakai transaksi
  REPEATABLE READ read only tanpa lock.
- **Reaksi** `ENGINE_RECOVERY_ON_MISMATCH`: `fail` (default, engine tidak ready), `rebuild`
  (perbaiki lalu verifikasi ulang), `warn`.
- **Urutan start**: migrasi → server HTTP mendengarkan (`/healthz` liveness) → pemulihan →
  relay outbox dan janitor → `/readyz` 200. Selama pemulihan, RPC dijawab 503 (Connect
  `Unavailable`). `HEALTHCHECK` Docker memakai `/readyz`.
- **Kunci dan muat** dikirim sebagai dua statement dalam satu batch: di READ COMMITTED, statement
  muat mendapat snapshot baru setelah lock didapat. Satu statement gabungan membaca subquery dari
  snapshot sebelum menunggu lock dan menghasilkan state basi (ditemukan oleh test 50 command
  paralel).
- **Subperintah admin** dari binary yang sama: `engine verify [--all|--instance K] [--json]`
  (exit 3 bila ada mismatch), `engine rebuild --instance K|--all [--full]`, `engine prune`,
  `engine outbox status|requeue`. Subperintah tak dikenal keluar dengan error.

## Konsekuensi

- Kerusakan proyeksi terdeteksi dan bisa diperbaiki dari CLI tanpa restore; test merusak baris lalu
  memulihkannya identik.
- Rebuild tidak menulis outbox: rebuild bukan eksekusi ulang.
- Verifikasi penuh 2.071 instance di database dev memakan sekitar 10 detik; start memakai jendela +
  sampel agar tetap cepat.
