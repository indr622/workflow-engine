# Runbook backup dan restore PostgreSQL

Berlaku sejak Fase 3 (rencana `docs/plan/03-fase-3-persistensi-pemulihan.md`, 9.2 dan 9.3).
Seluruh state engine (record eksekusi, proyeksi, outbox) ada di satu database Postgres, jadi
backup database = backup engine.

## Ringkasan perintah

```sh
make db-backup                                   # pg_dump -Fc -Z 6 → volume postgres-backups
make db-backup-list                              # daftar file di /backups
make db-restore FILE=workflow-<ts>.dump          # restore ke database baru + engine verify --all
make db-restore FILE=workflow-<ts>.dump TARGET_DB=workflow_restore_0917
```

Variabel Makefile:

| Variabel | Default | Keterangan |
|---|---|---|
| `BACKUP_ENV` | `prod` | `prod` = `infra/docker/docker-compose.prod.yml`; `dev` = `infra/docker/docker-compose.yml` (latihan lokal) |
| `COMPOSE_FILE_BACKUP` | sesuai `BACKUP_ENV` | file compose yang memuat service Postgres |
| `BACKUP_SERVICE` | `postgres` | nama service Postgres di file compose |
| `TARGET_DB` | `workflow_restore` | database tujuan restore; harus belum ada dan **tidak boleh** sama dengan `POSTGRES_DB` |
| `RESTORE_VERIFY_CMD` | `engine verify --all` lewat compose (prod) atau `go run` (dev) | perintah verifikasi setelah restore |

Di server produksi yang memakai `--env-file .env.prod`, ekspor variabel yang sama ke shell
(`set -a; . ./.env.prod; set +a`) sebelum menjalankan `make`, karena Makefile memanggil
`docker compose -f ...` tanpa `--env-file`.

## Backup

```sh
make db-backup
# = docker compose -f infra/docker/docker-compose.prod.yml exec -T postgres \
#     pg_dump -U $POSTGRES_USER -d $POSTGRES_DB -Fc -Z 6 -f /backups/workflow-<UTC timestamp>.dump
```

- File ditulis ke volume Docker `postgres-backups` (di-mount ke `/backups` pada service
  `postgres`), nama `workflow-YYYYMMDDTHHMMSSZ.dump` (UTC).
- `pg_dump` mengambil snapshot konsisten satu transaksi; **engine tidak perlu berhenti**.
- Volume ada di host yang sama dengan database. Salin file ke penyimpanan terpisah
  (`docker cp`, atau `docker run --rm -v workflow-engine_postgres-backups:/backups ...`) bila
  backup harus selamat dari kehilangan host. Backup terjadwal dan offsite: Fase 8.

## Restore

```sh
make db-restore FILE=workflow-20261028T094107Z.dump TARGET_DB=workflow_restore
```

Langkah yang dijalankan target:

1. Menolak bila `TARGET_DB` sama dengan database live (`POSTGRES_DB`) atau file tidak ada.
2. `createdb $TARGET_DB` (gagal bila database sudah ada; restore tidak pernah menimpa).
3. `pg_restore -d $TARGET_DB --no-owner --exit-on-error /backups/$FILE`.
4. `engine verify --all` terhadap `TARGET_DB` dengan `ENGINE_MIGRATE=false`
   (prod: `docker compose run --rm --no-deps engine verify --all`; image memakai
   `ENTRYPOINT ["/engine"]` sehingga argumen langsung `verify --all`).

Verifikasi restore adalah bagian Definition of Done, bukan opsional. Restore yang tidak lolos
`verify --all` tidak boleh dipromosikan.

> **Catatan:** subperintah `engine verify` ditambahkan oleh langkah pemulihan Fase 3 (12.4/12.5).
> Sampai subperintah itu ada, langkah 4 gagal; langkah 1–3 tetap dapat dipakai. Hati-hati: binary
> yang belum mengenal `verify` mengabaikan argumen dan menjalankan server engine biasa terhadap
> `TARGET_DB` (dengan `ENGINE_MIGRATE=false`), jadi hentikan dengan Ctrl+C bila itu terjadi.

Langkah manual setelah verifikasi lolos (promosi):

```sh
docker compose -f infra/docker/docker-compose.prod.yml stop engine
docker compose -f infra/docker/docker-compose.prod.yml exec -T postgres \
  psql -U "$POSTGRES_USER" -d postgres -c 'ALTER DATABASE workflow RENAME TO workflow_broken_<ts>' \
                                       -c 'ALTER DATABASE workflow_restore RENAME TO workflow'
docker compose -f infra/docker/docker-compose.prod.yml start engine
```

Database lama disimpan sampai insiden ditutup, lalu di-drop manual.

## Semantik data setelah restore

- **Outbox**: event yang belum terkirim (belum melewati cursor consumer) pada saat backup **ikut**
  dipulihkan dan akan **dikirim ulang** oleh relay setelah engine start. Event yang sudah terkirim
  sebelum backup tetapi cursor-nya belum maju juga dapat terkirim ulang. Consumer wajib idempoten;
  ini dijamin desain (at-least-once).
- **Setelah titik backup**: semua command, record, dan event yang terjadi setelah snapshot `pg_dump`
  **hilang bersama state-nya**. Sistem hilir yang sudah menerima event tersebut bisa melihat
  instance yang "mundur"; rekonsiliasi hilir adalah tanggung jawab operator.
- **RPO** Fase 3 = interval `make db-backup` yang dijalankan operator. **RTO** = waktu
  `pg_restore` + `verify --all` + promosi.

## Catatan PITR (point-in-time recovery)

`postgres:18-alpine` secara default tidak mengarsipkan WAL, jadi Fase 3 hanya punya pemulihan ke
titik backup. PITR membutuhkan:

1. `archive_mode=on` dan `archive_command` yang menyalin segmen WAL ke penyimpanan terpisah
   (atau tool seperti pgBackRest / WAL-G yang mengelola arsip).
2. Base backup berkala (`pg_basebackup`, pgBackRest, atau WAL-G).
3. Prosedur restore dengan `recovery_target_time` (base backup + replay WAL sampai waktu target).

Compose produksi sudah menjalankan `postgres -c wal_level=replica` sehingga pengaktifan
`archive_mode` di Fase 8 tidak memerlukan perubahan `wal_level` (yang butuh restart tersendiri).
`archive_mode` sendiri tetap butuh restart Postgres saat diaktifkan.

## Latihan lokal (compose dev)

```sh
make up
make db-backup BACKUP_ENV=dev
make db-backup-list BACKUP_ENV=dev
make db-restore BACKUP_ENV=dev FILE=workflow-<ts>.dump TARGET_DB=workflow_restore_test
docker compose -f infra/docker/docker-compose.yml exec -T postgres \
  psql -U workflow -d postgres -c 'DROP DATABASE workflow_restore_test WITH (FORCE)'
```

## Catatan uji end-to-end

| Tanggal | Lingkungan | Hasil |
|---|---|---|
| 2026-10-28 | compose dev, Postgres 18, DB `workflow` 32 MB (123 deployment, 1547 process instance, 38629 execution record, migrasi versi 5) | `db-backup` menghasilkan `workflow-20261028T094107Z.dump` (1,6 MB); `db-backup-list` menampilkannya; `db-restore` ke `workflow_restore_test`: `createdb` + `pg_restore --exit-on-error` sukses, jumlah baris tabel utama dan versi migrasi sama dengan sumber; langkah `verify --all` gagal karena subperintah belum ada (binary saat itu juga tidak terkompilasi akibat perubahan paralel di modul eksekusi). Database restore di-drop setelahnya. |
