# ADR-0004: PostgreSQL 18 sebagai penyimpanan state engine

- **Status:** Diterima
- **Tanggal:** 2026-10-28

## Konteks

State process instance, job, timer, dan incident butuh transaksi yang kuat. Antrian job butuh
penguncian baris yang efisien agar banyak worker tidak mengambil job yang sama.

## Keputusan

- **PostgreSQL 18**, major stabil terbaru dengan dukungan komunitas sampai November 2030.
  PostgreSQL tidak punya label LTS; setiap major didukung lima tahun.
- Akses database dengan **pgx v5**; query bertipe dengan **sqlc** mulai Fase 1.
- Migrasi SQL murni dengan **goose**, di-embed ke binary dan dijalankan otomatis saat engine start.
- Setiap modul memiliki tabelnya sendiri. Tidak ada tabel yang dibagi dua modul.
- Postgres dev memakai port host **5433** agar tidak bentrok dengan Postgres lain di 5432.

## Konsekuensi

- `SELECT ... FOR UPDATE SKIP LOCKED` tersedia untuk aktivasi job.
- Satu dependensi infrastruktur untuk fase awal, tanpa broker atau Elasticsearch.
- Skala tulis sangat tinggi kelak mungkin butuh partisi tabel atau event log terpisah.
