# Migrasi skema yang aman

Berlaku sejak Fase 3 (rencana `docs/plan/03-fase-3-persistensi-pemulihan.md`, 9.1 dan 9.4).
Migrasi ada di `apps/engine/internal/platform/db/migrations`, di-embed ke binary, dan dijalankan
oleh `db.Migrate` saat engine start bila `ENGINE_MIGRATE=true`.

## Cara engine menjalankan migrasi

- `db.Migrate` memakai `goose.NewProvider(goose.DialectPostgres, ..., goose.WithSessionLocker(lock.NewPostgresSessionLocker()))`
  (goose v3.29). Session locker mengambil `pg_try_advisory_lock(4097083626)` pada satu koneksi dan
  mencoba ulang tiap 5 detik sampai 5 menit. Replika yang start bersamaan menunggu, lalu melihat
  versi sudah terbaru dan tidak menjalankan apa pun. Diuji oleh
  `TestConcurrentMigrateOnEmptyDatabase` (dua `Migrate` paralel pada database kosong).
- Tabel versi tetap `goose_db_version`, sama dengan CLI `goose` di Makefile (`make migrate-*`).
- Koneksi migrasi dibuka terpisah dari pool dengan `statement_timeout=0` dan
  `idle_in_transaction_session_timeout=0` (DDL boleh lama), tetapi `lock_timeout` dari
  `ENGINE_DB_LOCK_TIMEOUT` tetap berlaku: DDL yang tidak mendapat lock tabel dalam batas itu gagal
  alih-alih menahan antrean query lain. Ulangi deploy bila itu terjadi.
- Semua key adalah UUIDv7 bertipe teks (ADR-0011). Jangan pernah menambah `SERIAL`, `IDENTITY`,
  atau sequence.

## Aturan

1. **Expand/contract.** Setiap perubahan skema dipecah menjadi:
   (a) *expand* — tambah kolom/tabel/index yang kompatibel dengan binary lama (kolom baru nullable
   atau ber-`DEFAULT`);
   (b) rilis binary baru yang memakai skema baru;
   (c) *contract* — hapus kolom/tabel yang tidak dipakai lagi, di rilis **berikutnya**.
   Jangan pernah `DROP` atau `RENAME` kolom yang masih dibaca binary yang sedang berjalan. Selama
   rolling update, binary lama dan baru berjalan bersamaan terhadap skema yang sama.
2. **Index besar** memakai `CREATE INDEX CONCURRENTLY` di file migrasi tersendiri yang ditandai
   `-- +goose NO TRANSACTION` (CONCURRENTLY tidak boleh di dalam transaksi). Satu statement per
   file agar kegagalan di tengah mudah dipulihkan (`DROP INDEX CONCURRENTLY IF EXISTS` lalu ulangi).

   ```sql
   -- +goose NO TRANSACTION
   -- +goose Up
   CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_job_type_state ON job (type, state);
   -- +goose Down
   DROP INDEX CONCURRENTLY IF EXISTS idx_job_type_state;
   ```

3. **Backfill dalam batch**, oleh migrasi (loop batch) atau janitor, bukan satu `UPDATE` tabel
   penuh yang mengunci seluruh tabel dan membengkakkan WAL. Pola umum: migrasi menambah kolom
   dengan `DEFAULT`, janitor mengisi nilai sebenarnya per batch dengan `LIMIT` dan jeda.
4. **Session lock** (lihat di atas) sudah otomatis; jangan menjalankan `goose up` manual
   bersamaan dengan engine yang sedang start.
5. **Down migration wajib** dan diuji: `up` → `down` → `up` pada database kosong (CI). Secara lokal:
   `make migrate-redo` menjalankan `goose down` lalu `goose up` untuk migrasi terakhir terhadap
   `DATABASE_URL` (jangan di produksi).
6. **Rollback binary** tanpa rollback skema harus aman selama tahap *contract* belum dijalankan.

## Kapasitas koneksi

Setiap replika engine membuka pool sampai `ENGINE_DB_MAX_CONNS` koneksi (default
`max(4, 2 × NumCPU)`), ditambah satu koneksi migrasi singkat saat start. Aturan:

```
replika × ENGINE_DB_MAX_CONNS + 10 ≤ max_connections   (default Postgres: 100)
```

Sisa 10 untuk koneksi admin (`psql`, `pg_dump`, `make db-backup`, `verify`, superuser reserved).
Contoh: 4 replika × 16 koneksi + 10 = 74 ≤ 100. Uji paritas dan integrasi memakai
`ENGINE_DB_MAX_CONNS=8`.

## Timeout sesi

| Variabel | Default | Efek |
|---|---|---|
| `ENGINE_DB_STATEMENT_TIMEOUT` | `10s` | query lebih lama dibatalkan server dengan SQLSTATE `57014` |
| `ENGINE_DB_LOCK_TIMEOUT` | `5s` | menunggu lock baris/tabel lebih lama gagal dengan SQLSTATE `55P03` |
| `ENGINE_DB_IDLE_IN_TX_TIMEOUT` | `15s` | sesi yang idle di dalam transaksi diputus server |
| `ENGINE_DB_MIN_CONNS` / `..._MAX_CONN_LIFETIME` / `..._MAX_CONN_IDLE_TIME` | `2` / `30m` / `5m` | pool pgx |

Nilai `0` pada timeout berarti nonaktif. Koneksi pool juga memakai `application_name=engine`
(koneksi migrasi `engine-migrate`) sehingga mudah dikenali di `pg_stat_activity`.
