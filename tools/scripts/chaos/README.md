# Suite chaos Compose

Runner Go untuk rencana Fase 3 bagian 11.2–11.3 (langkah 12.10). Suite ini membuktikan jaminan
durabilitas (bagian 3.1) di level proses dan container, memakai image engine yang sama dengan
produksi (`infra/docker/engine.Dockerfile`).

## Menjalankan

```sh
make chaos                                   # semua skenario, load 30s per skenario
make chaos SCENARIO=kill-engine DURATION=15s
make chaos SCENARIO=kill-engine,replicas     # nama atau nomor, dipisah koma
go run ./tools/scripts/chaos -list           # daftar skenario dan yang dilewati
```

`make chaos` membangun image engine (`$(REGISTRY)/engine:$(VERSION)`, tag yang sama dengan
`make docker-build`; image web tidak dibangun karena tidak dipakai), menyalakan postgres chaos,
menjalankan runner, lalu **selalu** menjalankan `down -v` untuk project chaos, juga saat gagal.

Lingkungan chaos (`infra/docker/docker-compose.chaos.yml`) terpisah dari compose dev: project
`workflow-engine-chaos`, network `workflow-engine-chaos`, volume `workflow-engine-chaos_postgres-data`,
Postgres di `127.0.0.1:5544`, engine di `127.0.0.1:8180`, dan `engine-2` (profil `replicas`) di
`127.0.0.1:8181`. Postgres dev (5433) dan volume-nya tidak disentuh.

Menjalankan runner langsung (image harus sudah ada):

```sh
CHAOS_ENGINE_IMAGE=ghcr.io/indr622/workflow-engine/engine:<tag> \
  go run ./tools/scripts/chaos -scenario all -duration 15s -v
```

Flag penting: `-scenario`, `-duration` (load per skenario), `-compose`, `-engine`, `-engine2`, `-db`,
`-workers` (default 8), `-ready-timeout`, `-retry-timeout`, `-logs` (default `tmp/chaos-logs`),
`-keep` (biarkan project terakhir menyala untuk diperiksa), `-v` (log runner langsung ke layar).

Runner mencetak tabel PASS/FAIL/SKIP per skenario dan keluar dengan kode bukan nol bila ada yang
gagal. Untuk skenario yang gagal, log runner dan `docker compose logs` disimpan di
`tmp/chaos-logs/<skenario>.log` (diabaikan git; diunggah sebagai artifact oleh workflow `Chaos`).
CI menjalankan suite nightly dan manual (`.github/workflows/chaos.yml`), bukan per PR.

## Alur per skenario

1. Project chaos dibuat ulang dari nol (`down -v`, lalu `up --wait`), lalu korpus di-deploy:
   proses linear 10 service task, fork paralel 10 cabang, dan loop 20 iterasi.
2. Load generator: N goroutine membuat instance dan menyelesaikan job-nya lewat API Connect, dengan
   `request_id` baru per command. Job paralel (fork) diselesaikan serentak, lewat replika acak bila
   ada dua. Setiap command yang di-ack dicatat; command yang gagal dicatat pending dan instance-nya
   ditinggalkan.
3. Injeksi setelah ~40% durasi load.
4. Pemulihan: tunggu `/readyz` 200, lalu ulang setiap command pending dengan request yang sama persis
   (termasuk `request_id`) sampai berhasil.
5. Invarian umum (dicek untuk setiap skenario):
   - setiap create yang di-ack ada di `process_instance`, dan tidak ada instance yang tidak pernah
     di-ack (instance ganda karena retry akan terdeteksi di sini);
   - tidak ada job dengan lebih dari satu record `JOB COMPLETED`; himpunan job selesai di database
     sama persis dengan himpunan penyelesaian yang di-ack, dan proyeksi `job` berstatus `COMPLETED`;
   - `engine verify --all` exit 0 (dijalankan dengan `docker compose run --rm engine`);
   - outbox: setiap record pemicu event (job/incident `CREATED`, instance `ELEMENT_COMPLETED` /
     `ELEMENT_TERMINATED`) punya tepat satu baris `outbox_event` dengan `(aggregate_key,
     aggregate_seq)` yang cocok, tidak ada event tanpa record, `aggregate_seq` naik tegas per instance
     dalam urutan baca relay `(tx_id, event_id)`, dan `outbox_dead_letter` kosong;
   - log engine tidak memuat `23505` / `duplicate key value` (mis. `UNIQUE (process_instance_key,
     sequence)`), `panic:`, atau `outbox relay stopped`.

Catatan: wiring produksi belum mendaftarkan consumer outbox durable, sehingga "tepat sekali per
consumer" (tabel `chaos_received` di rencana 11.2) diwakili pemeriksaan level database di atas.
Pemeriksaan pengiriman ke consumer durable sungguhan menunggu consumer yang ditambahkan fase
berikutnya (timer start Fase 5, proyeksi Operate Fase 6).

## Skenario

| # | Nama | Injeksi | Invarian tambahan |
|---|---|---|---|
| 1 | `kill-engine` | `docker kill -s KILL` engine saat load, lalu `docker compose start engine` | engine ready kembali; semua command ter-ack ada; retry tidak membuat instance/penyelesaian ganda |
| 4 | `restart-postgres` | `docker compose restart postgres` saat load | engine **tidak** restart (pool menyambung ulang sendiri) |
| 5 | `kill-postgres` | `docker kill -s KILL` postgres, lalu start lagi (crash recovery WAL) | sama dengan 4; tidak ada command ter-ack yang hilang |
| 6 | `network-partition` | postgres diputus dari network chaos selama 10 s, lalu disambung lagi (alias `postgres`) | sama dengan 4 |
| 7 | `idempotent-parallel` | 20 `CreateProcessInstance` identik serentak, lalu 20 `CompleteJob` identik serentak | satu instance / satu penyelesaian; 20 response identik byte per byte; tepat 19 bertanda `Idempotent-Replayed: true` |
| 8 | `key-reuse` | `request_id` dipakai ulang dengan payload berbeda (create dan complete) | `InvalidArgument` + detail `IdempotencyKeyReused`; jumlah baris state tidak berubah; payload asli tetap di-replay |
| 9 | `corrupt-projection` | hapus `element_instance` aktif dan ubah `variable` lewat SQL | `verify --all` exit 3 dan menyebut instance itu; `rebuild --instance` exit 0; dump proyeksi identik dengan sebelum perusakan; `verify --all` exit 0 |
| 10 | `replicas` | dua replika (profil `replicas`), load ke keduanya, job fork diselesaikan serentak lewat replika berbeda | tidak ada record ganda; tidak ada 23505 di log kedua replika |
| 12 | `concurrent-start` | dua replika start serentak pada database kosong | keduanya ready; `goose_db_version` berisi tiap migrasi tepat sekali; total `applied` di log kedua replika = jumlah file migrasi; lalu load ke keduanya |

Partisi dilakukan dengan memutus **postgres**, bukan engine: di Docker Desktop, memutus network
sebuah container ikut mematikan port host-nya, sedangkan runner perlu tetap memanggil API engine
selama partisi. Efeknya sama (engine kehilangan Postgres).

## Skenario yang dilewati

| # | Skenario | Alasan |
|---|---|---|
| 2 | Engine mati setelah commit sebelum relay kirim | fault point `command.after_commit=panic`, integration test `-tags=integration,faultinject` |
| 3 | Relay mati setelah handler sebelum kursor commit | fault point `outbox.before_cursor_commit=panic`, integration test faultinject |
| 11 | Commit tidak urut | fault point `command.before_commit=sleep`, integration test faultinject |
| 13 | Restore backup | sudah diuji lewat `make db-backup` / `make db-restore` dan latihan runbook `docs/ops/backup-restore.md` |
| 14 | Retensi saat load | butuh umur retensi khusus uji; integration test faultinject |
| 15 | Loop panjang + snapshot | waktu rebuild dari snapshot; integration test faultinject |
| 16 | Consumer beracun | butuh consumer durable (belum ada di wiring produksi); integration test faultinject |

Skenario 2, 3, 11, 14, 15, dan 16 deterministik di level transaksi, sehingga lebih tepat diuji dengan
fault point in-process daripada dengan Compose.
