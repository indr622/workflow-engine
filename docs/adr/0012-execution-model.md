# ADR-0012: Model eksekusi: record append-only, proyeksi, dan single writer per instance

- **Status:** Diterima
- **Tanggal:** 2026-10-28

## Konteks

Fase 2 membuat engine menjalankan proses. Model eksekusi harus deterministik agar Fase 3 bisa
membangun ulang state dari log, aman saat engine berjalan lebih dari satu replika, dan tetap
sederhana tanpa infrastruktur actor atau partisi.

## Keputusan

- **Aggregate event-sourced.** `execution/domain.Processor` menerima command (`CreateInstance`,
  `CompleteJob`, `CancelInstance`) dan menghasilkan record (event). State instance hanya berubah
  lewat `domain.Apply(record)`, yang juga dipakai replay. Satu command diproses run-to-completion
  dengan antrean FIFO sampai semua token berhenti di wait state atau selesai.
- **Siklus hidup elemen dan intent mengikuti Zeebe** (`ELEMENT_ACTIVATING` … `ELEMENT_TERMINATED`,
  `SEQUENCE_FLOW_TAKEN`). Hanya event yang disimpan; command internal antrean tidak.
- **Persistensi dalam satu transaksi per command**: record ditulis ke `execution_record` dan
  proyeksinya ke tabel state (`process_instance`, `element_instance`, `variable`, `job`,
  `incident`, `taken_sequence_flow`). Urutan per instance memakai `sequence` yang dihitung engine,
  dengan `PRIMARY KEY (process_instance_key, sequence)` sebagai jaring pengaman. Tidak ada kolom
  auto-increment (ADR-0011).
- **Single writer per instance lewat row lock** `SELECT … FOR UPDATE` pada `process_instance`.
  Kunci dan pemuatan state dikirim sebagai dua statement dalam satu `pgx.Batch`: di READ COMMITTED
  statement muat mendapat snapshot baru setelah kunci didapat. Satu statement gabungan akan
  membaca state basi (ditangkap test 50 `CompleteJob` paralel).
- **Model proses dimuat sebelum transaksi dibuka.** Memuatnya sambil memegang row lock bisa
  meminta koneksi kedua dari pool dan membuat command paralel saling menunggu sampai pool habis.
- **Semua tulisan satu command dikirim dalam satu `pgx.Batch`** (statement proyeksi ditulis tangan,
  query baca memakai sqlc). Counter join dan `has_incident` diproyeksikan dari state akhir.
- **Job dimiliki modul `execution`**: penyelesaian job dan kelanjutan token atomik. Modul `job`
  Fase 4 menjadi lapisan protokol worker di atas `execution/api.Jobs`.
- **Incident** dibuat atomik dengan langkah yang gagal; elemen berhenti di state saat gagal
  (gateway dan service task di `ACTIVATING`). Pembatalan instance menyelesaikan incident terbuka.
- **Event antarmodul** (`execution.process_instance_completed`, `…_terminated`, `job_created`,
  `incident_created`) dipublikasikan setelah commit; outbox transaksional di Fase 3.
- **Batas record per command** (`ENGINE_MAX_RECORDS_PER_COMMAND`, default 10.000): melebihi batas
  saat mengaktifkan elemen menghasilkan incident `EXECUTION_LIMIT_EXCEEDED`; token lain yang masih
  di antrean command itu dibuang.

## Konsekuensi

- Replay = `Apply` ulang record; diuji di domain (setiap skenario) dan terhadap database
  (`TestReplayMatchesProjection`).
- Satu command butuh sekitar lima round trip (referensi job atau instance, begin, kunci + muat,
  batch tulis, commit). Latensi didominasi round trip database.
- Contention hanya terjadi di dalam satu instance. Partisi ditinjau di Fase 3 bila terukur, tanpa
  mengubah domain.
