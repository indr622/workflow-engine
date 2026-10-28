# Fase 3 — Persistensi dan Pemulihan

**Proyek:** Workflow Engine (kompatibel konsep Camunda 8 / BPMN 2.0)
**Stack:** Go (engine utama) + React (UI) dalam monorepo Nx
**Status:** 🚧 Sedang dieksekusi di branch `feat/phase-3-persistence` (mulai 2026-10-28)
**Tanggal:** 2026-10-28

---

## 0. Status eksekusi

Belum ada langkah yang dieksekusi. Bagian ini diisi dengan tabel yang sama seperti Fase 0
(langkah, status, bukti) begitu pekerjaan dimulai.

**Kondisi yang menjadi titik awal (dicek 2026-10-28; Fase 1 sedang dieksekusi, Fase 2 masih
rencana):**

| Area | Kondisi | Dampak ke Fase 3 |
|---|---|---|
| `apps/engine/internal/platform/db` | `Connect` memakai `pgxpool.New(databaseURL)` tanpa pengaturan pool/timeout; `Migrate` menjalankan goose dari `embed.FS` tanpa lock antar replika | Fase 3 menambah konfigurasi pool, `statement_timeout`/`lock_timeout`, dan session lock migrasi (9.1, 9.4) |
| `apps/engine/internal/platform/eventbus` | `InMemory` sinkron: `Publish` memanggil handler berurutan, error pertama menghentikan sisanya; belum ada subscriber (`main.go` hanya membuat bus) | Bus dipertahankan untuk **sinyal in-process** (best-effort); pengiriman durable pindah ke outbox (3.1, 4) |
| Modul engine | Hanya `health` (lima lapisan, `/healthz`, RPC Connect); `deployment` sedang dibangun di Fase 1; `execution` direncanakan Fase 2 | Paket platform baru (`outbox`, `recovery`, `idempotency`, `metrics`) dipakai modul lewat `application`/`infrastructure`/`interfaces`, tidak pernah dari `domain` |
| Migrasi | Hanya `00001_init.sql` (`engine_metadata`); Fase 1 menambah `00002_deployment.sql` (+ `engine_key_seq`), Fase 2 `00003_execution.sql` | Migrasi Fase 3 menjadi `00004_outbox.sql`, `00005_idempotency.sql`, `00006_history.sql` (nomor final mengikuti urutan merge) |
| Skema Fase 2 (rencana) | `execution_record` append-only (`position` identity global, `sequence` per instance tanpa celah, `UNIQUE (process_instance_key, sequence)`), tabel state `process_instance`, `element_instance`, `taken_sequence_flow`, `variable`, `job`, `incident`; semua ditulis dalam satu transaksi per command; `process_instance.version` naik per command | **Dipakai apa adanya.** Fase 3 menambah kolom `last_sequence`, `updated_at` di `process_instance`, tabel `execution_snapshot`, dan menjadikan record sumber kebenaran (5) |
| Domain Fase 2 (rencana) | `Processor` murni + `Apply(state, record)` satu-satunya tempat state berubah; `projector.go` memetakan setiap `(ValueType, Intent)` ke tabel state; test determinisme replay di DoD Fase 2 | Rebuild proyeksi = `Apply` + `Project` ulang per record; tidak ada logika eksekusi baru (5.4) |
| Port Fase 2 (rencana) | `domain.Repository.WithTx(fn func(tx TxRepository) error)`; `TxRepository.AppendAndProject(records)`; publikasi event di `command_runner.go` setelah commit | Unit of work dipindah ke `application.UnitOfWork` agar transaksi yang sama bisa menulis outbox dan klaim idempotensi; `domain` tidak berubah (3.3) |
| Key | Fase 1: `int64` dari `engine_key_seq` (`internal/platform/keygen`); Fase 2: alokasi blok 64 key per proses | Diputuskan tetap sequence global tanpa bit partisi (8, ADR-0017) |
| Konfigurasi | `config.Config` lima field (`ENGINE_HTTP_ADDR`, `DATABASE_URL`, `ENGINE_MIGRATE`, `ENGINE_LOG_LEVEL`, `ENGINE_SHUTDOWN_TIMEOUT`, `ENGINE_CORS_ORIGINS`) | Ditambah grup `ENGINE_DB_*`, `ENGINE_OUTBOX_*`, `ENGINE_RECOVERY_*`, `ENGINE_IDEMPOTENCY_*`, `ENGINE_HISTORY_*` (9.5) |
| `cmd/engine/main.go` | Subperintah `healthcheck`; `run()` merakit modul lalu `httpserver.Run` | Subperintah baru `verify`, `rebuild`, `prune`, `outbox`; urutan start ditambah tahap pemulihan dan gerbang readiness (5.2, 5.6) |
| Compose | `docker-compose.yml` (postgres 18-alpine, volume `postgres-data`, profil `tools`, `observability`); `docker-compose.prod.yml` (postgres + engine + web, `ENGINE_MIGRATE=true` di engine, `HEALTHCHECK` ke `/healthz`) | Ditambah volume `postgres-backups`, target `db-backup`/`db-restore`, file `docker-compose.chaos.yml`; `HEALTHCHECK` beralih ke `/readyz` |
| Makefile | 23 target; `test-integration` = `go test -tags=integration ./apps/engine/...` terhadap `DATABASE_URL`; Fase 1–2 menambah `fuzz`, `bench`, `parity`, `prop` | Target baru `chaos`, `db-backup`, `db-restore`, `verify`, `rebuild` (13) |
| CI | `.github/workflows/ci.yml`: job check, integration (Postgres 18), docker | Job `chaos` terjadwal (nightly), bukan per PR |
| `.golangci.yml` | depguard `domain-purity` melarang pgx, connect, `net/http`, `database/sql`, platform, `libs/go/api` di `domain` | Tidak berubah; paket platform baru otomatis terlarang untuk `domain` |
| ADR | ADR-0001 sampai ADR-0007; Fase 1 merencanakan ADR-0008/0009, Fase 2 ADR-0010 sampai ADR-0013 | Fase 3 melahirkan ADR-0014 sampai ADR-0018 |

## 1. Lingkup

Fase 3 menjadikan engine **tahan crash dan restart tanpa kehilangan atau menggandakan efek**. Fase 2
sudah menulis record append-only dan proyeksi state dalam satu transaksi; Fase 3 menutup tiga celah
yang sengaja ditinggalkan: event antarmodul yang hilang bila proses mati setelah commit, perintah
klien yang diulang setelah timeout, dan ketiadaan cara memverifikasi atau membangun ulang state
dari record. Ditambah disiplin operasional (backup, migrasi, pool, timeout) dan bukti berupa uji
injeksi kegagalan.

Termasuk dalam Fase 3:

1. **Outbox transaksional** di `internal/platform/outbox`: event domain ditulis dalam transaksi yang
   sama dengan state, dikirim oleh relay ke subscriber in-process **at-least-once** dengan kursor
   per consumer, urutan terjaga per aggregate, dedup berdasarkan `event_id`. Publisher Fase 1
   (`deployment.process_deployed`) dan Fase 2 (`execution.*`) dipindah ke outbox.
2. **Pemulihan setelah restart**: urutan start yang menjalankan migrasi → verifikasi konsistensi
   record vs proyeksi → pemulihan per modul → gerbang readiness; rebuild proyeksi dari
   `execution_record` (per instance atau seluruhnya) lewat subperintah CLI; rekonsiliasi
   subscription durable lewat kursor outbox.
3. **Idempotensi** RPC yang mengubah state (`CreateProcessInstance`, `CompleteJob`,
   `CancelProcessInstance`, `DeployResource`) lewat `request_id` opsional, tabel klaim dengan TTL,
   semantik retry yang terdefinisi.
4. **Snapshot state runtime** untuk instance berumur panjang, **kebijakan retensi** instance selesai,
   **paginasi keyset** untuk pembacaan riwayat (record dan element instance).
5. **Keputusan key dan kesiapan partisi**: sequence global dipertahankan; apa yang harus dijaga agar
   partisi/sharding bisa menyusul tanpa migrasi key (ADR-0017).
6. **Operasional**: runbook backup/restore `pg_dump`/`pg_restore` untuk deploy Compose, catatan PITR,
   aturan migrasi aman (goose, expand/contract, lock antar replika), ukuran pool, timeout statement.
7. **Uji injeksi kegagalan**: fault point di kode (build tag), skenario chaos berbasis Compose (kill
   engine di tengah command, restart/kill Postgres, pengiriman ganda, dua replika), tabel skenario
   dengan hasil yang diharapkan.
8. **Hook observability** yang dibutuhkan sebagai bukti pemulihan: log terstruktur tahap pemulihan
   dan relay, counter/gauge lewat API OpenTelemetry dengan provider no-op (exporter di Fase 8).

**Tidak termasuk** (fase lain): aktivasi job oleh worker, long polling, `FailJob`/retry (Fase 4 —
Fase 3 hanya menyediakan sinyal `job_created` durable dan in-process); timer dan message (Fase 5 —
Fase 3 menyediakan `recovery.Participant` dan consumer outbox durable yang akan dipakai scheduler);
Operate dan tampilan riwayat (Fase 6 — Fase 3 menyediakan RPC riwayat berpaginasi); exporter
OTel, backup terjadwal/offsite, zero-downtime restart, multi-tenant (Fase 8); partisi tabel
Postgres dan sharding instance antar replika (ditinjau bila terukur, lihat 8); kompaksi record
secara default (format snapshot ada, pemangkasan record mati sampai Fase 6 memutuskan penyimpanan
riwayat).

## 2. Definition of Done

- [ ] Paket `internal/platform/outbox` dengan tabel `outbox_event`, `outbox_cursor`, `outbox_dead_letter`; `Writer.Enqueue` dalam `pgx.Tx`; `Relay` dengan consumer durable (kursor per nama consumer, advisory lock per consumer, batch, retry backoff, dead letter setelah `MaxAttempts`).
- [ ] Pembacaan relay **bebas celah**: test integrasi dengan 200 transaksi paralel yang sengaja commit tidak urut (fault point menunda commit) tidak pernah melewatkan event; urutan `aggregate_seq` per `aggregate_key` monoton untuk setiap consumer (test 50 `CompleteJob` paralel pada satu instance, diulang 20 kali).
- [ ] Pengiriman at-least-once dibuktikan: fault point `outbox.before_cursor_commit` membuat batch dikirim dua kali; consumer uji mendeteksi `event_id` ganda dan efeknya tetap satu.
- [ ] `deployment.process_deployed` dan `execution.process_instance_completed`, `process_instance_terminated`, `job_created`, `incident_created` ditulis ke outbox dalam transaksi command; publikasi in-process pasca-commit tetap ada sebagai sinyal best-effort (ADR-0014).
- [ ] `process_instance.last_sequence` dan `updated_at` terisi oleh projector; `engine verify` tingkat cepat (SQL) memeriksa seluruh instance < 5 detik untuk 1 juta instance di mesin dev (angka awal, dikunci setelah pengukuran); tingkat dalam (replay in-memory vs proyeksi) untuk instance aktif yang berubah dalam jendela `ENGINE_RECOVERY_DEEP_WINDOW`.
- [ ] `engine rebuild --instance K` dan `--all`: hapus proyeksi instance lalu `Apply` + `Project` ulang semua record (dari snapshot bila ada) di bawah `FOR UPDATE`; test integrasi: untuk setiap skenario Fase 2, `TRUNCATE` tabel proyeksi → rebuild → isi tabel identik byte-per-byte (dump terurut) dengan sebelum truncate.
- [ ] Projector idempoten: menjalankan `Project` dua kali untuk record yang sama menghasilkan state yang sama (upsert di semua jalur); dibuktikan test per `(ValueType, Intent)`.
- [ ] Urutan start: migrasi → `recovery.Runner` (verify + `Participant.Recover` tiap modul) → relay outbox → HTTP listen; `/readyz` 503 sampai pemulihan selesai, `/healthz` tetap liveness; `HEALTHCHECK` Docker memakai `/readyz`.
- [ ] `request_id` opsional di `CreateProcessInstanceRequest`, `CompleteJobRequest`, `CancelProcessInstanceRequest`, `DeployResourceRequest`; tabel `idempotency_key` dengan TTL; dua request identik (berurutan maupun paralel) menghasilkan satu efek dan response byte-identik; key sama dengan payload berbeda → `InvalidArgument` dengan detail `IDEMPOTENCY_KEY_REUSED`; test integrasi 20 request paralel satu `request_id` → satu instance.
- [ ] `execution_snapshot` ditulis setiap `ENGINE_SNAPSHOT_INTERVAL_RECORDS` record untuk instance aktif; rebuild dari snapshot + record sisa menghasilkan state identik dengan rebuild dari nol (test untuk setiap skenario loop Fase 2).
- [ ] Janitor retensi: instance final dengan `end_time` lebih tua dari `ENGINE_HISTORY_RETENTION_DAYS` dihapus batch (semua tabel state + record + snapshot) tanpa mengganggu command paralel (test: 10.000 instance selesai dihapus sambil 50 instance aktif diproses; tidak ada error/deadlock); `0` = nonaktif (default).
- [ ] RPC `ListProcessInstanceRecords` dan `ListElementInstances` dengan keyset pagination (token opak, `page_size` maks 500); `GetProcessInstance` memotong `element_instances` di 1.000 dengan flag `element_instances_truncated`.
- [ ] `config` punya grup `ENGINE_DB_*`; pool pgx diset `MaxConns`, `MinConns`, `MaxConnLifetime`, `MaxConnIdleTime`, dan `RuntimeParams` (`application_name`, `statement_timeout`, `lock_timeout`, `idle_in_transaction_session_timeout`); test integrasi membuktikan query > `statement_timeout` dibatalkan dengan SQLSTATE `57014`.
- [ ] Migrasi memakai session lock goose sehingga dua replika yang start bersamaan tidak saling bentrok (test: dua proses `engine` paralel pada DB kosong, keduanya selesai, versi migrasi sama); panduan expand/contract tercatat di `docs/ops/migrations.md`.
- [ ] `make db-backup` menghasilkan `pg_dump -Fc` ke volume `postgres-backups`; `make db-restore FILE=...` memulihkan ke database baru lalu `engine verify --all` lolos; runbook di `docs/ops/backup-restore.md` diuji end-to-end sekali dan hasilnya dicatat di 0.
- [ ] `make chaos` menjalankan semua skenario 11.3 terhadap Compose; setiap skenario memverifikasi invarian yang tertulis; dijalankan hijau tiga kali berturut-turut di mesin dev dan satu kali di CI terjadwal.
- [ ] Log terstruktur untuk setiap tahap pemulihan dan relay (10) dan metrik `engine_outbox_*`, `engine_recovery_*`, `engine_idempotency_*`, `engine_command_*` terdaftar lewat `platform/metrics`; test unit memastikan nama metrik stabil (snapshot daftar).
- [ ] Benchmark contention: 50 cabang paralel `CompleteJob` pada satu instance dan 500 instance paralel; angka p50/p99 dicatat; keputusan "row lock tetap" atau "mulai desain partisi" ditulis di ADR-0017.
- [ ] ADR-0014 sampai ADR-0018 tersimpan; `docs/plan/README.md` menautkan dokumen ini.

## 3. Keputusan teknis

| Area | Keputusan | Alasan singkat |
|---|---|---|
| Bentuk outbox | **Tabel `outbox_event` generik milik platform**, ditulis oleh modul mana pun dalam `pgx.Tx` command-nya; `execution` menurunkan baris outbox dari record di `application` (bukan relay membaca `execution_record` langsung) | `deployment` tidak punya record log, dan modul Fase 4–5 juga butuh event durable; satu mekanisme untuk semua. Record tetap sumber kebenaran state, outbox hanya antrean pengiriman berumur pendek |
| Dua tingkat pengiriman event | **Durable** (consumer outbox bernama, kursor di DB, at-least-once, urutan per aggregate) dan **sinyal in-process** (bus `eventbus.InMemory` pasca-commit, best-effort, tanpa jaminan) (ADR-0014) | Long polling job (Fase 4) hanya butuh "bangunkan saya" dan boleh hilang karena worker toh melakukan poll; timer start (Fase 5) dan proyeksi Operate (Fase 6) tidak boleh kehilangan event |
| Bebas celah tanpa serialisasi | Kolom `tx_id xid8 DEFAULT pg_current_xact_id()`; relay hanya membaca baris dengan `tx_id < pg_snapshot_xmin(pg_current_snapshot())`, urut `(tx_id, id)`, kursor `(tx_id, id)` | Identity `id` tidak monoton terhadap urutan commit (transaksi dengan id kecil bisa commit belakangan dan dilewati kursor). Semua transaksi dengan xid di bawah `xmin` snapshot pasti sudah selesai, sehingga kursor tidak pernah melompati transaksi yang masih berjalan. Tanpa lock global, tanpa jendela waktu |
| Urutan per aggregate | Dijamin oleh single writer Fase 2: xid dialokasikan saat `SELECT ... FOR UPDATE`, sehingga urutan xid = urutan lock per instance; **aturan**: tidak ada statement yang mengalokasikan xid sebelum lock aggregate (klaim idempotensi dilakukan **setelah** lock) | Urutan xid hanya sama dengan urutan commit per aggregate bila xid diambil setelah lock; aturan ini ditegakkan test monotonisitas `aggregate_seq` |
| Pengiriman ke consumer | Handler dipanggil **di dalam transaksi relay** yang memajukan kursor (`Handle(ctx, tx, ev)`); efek handler ke DB ikut commit bersama kursor | Untuk handler yang menulis ke DB (timer, proyeksi) hasilnya exactly-once tanpa dedup tambahan; handler dengan efek di luar DB tetap wajib idempoten berdasarkan `event_id` |
| Identitas event | `event_id UUID DEFAULT uuidv7()` (PostgreSQL 18) | Terurut waktu, unik global, tanpa round-trip ke `keygen`; verifikasi ketersediaan `uuidv7()` pada image `postgres:18-alpine` saat 12.2 |
| Replika ganda | Satu relay per replika; setiap consumer durable dikunci `pg_try_advisory_lock(hashtext('outbox:' || consumer))` selama relay memegangnya; replika yang kalah mencoba ulang berkala | Setiap event diproses sekali per consumer meski engine di-scale; tanpa leader election terpisah |
| Sumber kebenaran | **`execution_record` adalah sumber kebenaran**; tabel state adalah proyeksi yang bisa dihapus dan dibangun ulang (ADR-0015) | Sudah dijanjikan Fase 2; memberi jalan keluar dari bug proyeksi tanpa restore backup |
| Verifikasi saat start | Dua tingkat: **cepat** (SQL agregat atas seluruh instance: `last_sequence` vs `MAX(sequence)`, state akhir vs record terakhir, job terbuka vs element instance) dan **dalam** (replay in-memory lalu bandingkan dengan `LoadRuntimeState`) untuk instance aktif yang berubah dalam jendela terakhir | Pemeriksaan penuh setiap start terlalu mahal untuk jutaan instance; pemeriksaan cepat menangkap divergensi struktural, pemeriksaan dalam menangkap bug `Apply`/projector pada instance yang paling mungkin terdampak crash |
| Reaksi atas ketidakcocokan | `ENGINE_RECOVERY_ON_MISMATCH=fail|rebuild|warn`, default **`fail`** (engine tidak ready, log ERROR, operator menjalankan `engine rebuild`) | Divergensi hanya terjadi karena bug (record + proyeksi satu transaksi); menyembunyikannya dengan rebuild otomatis menghilangkan bukti. Mode `rebuild` tersedia untuk operator yang mengutamakan ketersediaan |
| Idempotensi | `request_id` opsional **di body proto** (bukan header), scope `tenant + nama RPC`, hash request deterministik, response tersimpan, TTL default 24 jam; klaim lewat `INSERT ... ON CONFLICT DO NOTHING` **dalam transaksi command, setelah lock aggregate** (ADR-0016) | Di body: masuk kontrak proto dan SDK, bekerja sama di gRPC/Connect/REST. Insert konflik memblokir duplikat paralel sampai transaksi pertama selesai, tanpa status "pending" yang terlihat |
| Reuse key dengan payload berbeda | `InvalidArgument` + detail `IDEMPOTENCY_KEY_REUSED` | Hampir pasti bug klien; mengembalikan response lama akan menyesatkan |
| Key | **Tetap sequence global `engine_key_seq` dengan alokasi blok; tidak ada bit partisi** (ADR-0017) | Satu database adalah satu domain konsistensi; partisi yang mungkin dibutuhkan kelak adalah pembagian kepemilikan instance antar replika (hash `process_instance_key`), bukan log terpisah, jadi tidak butuh tata letak bit ala Zeebe. Mengubah tata letak key sekarang mengubah semantik `position`/`sequence` dan paritas tanpa manfaat terukur |
| Konkurensi | Row lock per instance **dipertahankan**; benchmark contention 12.11 menentukan apakah Fase 4+ perlu "instance affinity" (routing command ke replika pemilik) | Keputusan Fase 2 sudah benar untuk multi-replika; data dulu, baru desain |
| Snapshot | `execution_snapshot` berisi `domain.InstanceState` runtime (JSON) pada `sequence` tertentu, ditulis dalam transaksi command setiap N record (default 5.000) untuk instance aktif; rebuild mulai dari snapshot. **Kompaksi record mati secara default** (ADR-0018) | Membatasi waktu rebuild instance berloop panjang; record tetap utuh karena Operate (Fase 6) butuh riwayat; kompaksi hanya bila Fase 6 memutuskan riwayat disimpan di tempat lain |
| Retensi | **Hapus** instance final setelah N hari (`ENGINE_HISTORY_RETENTION_DAYS`, default 0 = simpan selamanya; `.env.prod.example` memberi 90) oleh janitor batch; tidak ada tabel arsip (ADR-0018) | Tabel arsip menggandakan skema tanpa pembaca; operator yang butuh arsip memakai `make db-backup` sebelum pemangkasan. Camunda 8 juga memakai TTL riwayat (verifikasi terhadap dokumentasi Camunda 8.8: nama dan default pengaturan retensi) |
| Paginasi riwayat | Keyset: record urut `sequence`, element instance urut `key`; token opak base64url JSON `{"k": <last>}`; helper `platform/paging` dipakai juga oleh list Fase 1–2 | Offset pagination melambat linear; keyset stabil saat data bertambah |
| Pool dan timeout | pgxpool `MaxConns` default `max(4, 2 × NumCPU)`, `MinConns` 2, `MaxConnLifetime` 30 m, `MaxConnIdleTime` 5 m; `RuntimeParams`: `statement_timeout` 10 s, `lock_timeout` 5 s, `idle_in_transaction_session_timeout` 15 s; relay dan janitor memakai koneksi dari pool yang sama dengan batas internal (maks 2) | Transaksi command pendek; timeout mencegah transaksi macet menahan `xmin` relay dan vacuum; satu pool memudahkan penalaran kapasitas: `replika × MaxConns + 10 ≤ max_connections` |
| Migrasi | goose tetap, dijalankan saat start **dengan session lock** (`goose.NewProvider` + `WithSessionLocker`, verifikasi API goose v3.29); aturan expand/contract; `CREATE INDEX CONCURRENTLY` lewat `-- +goose NO TRANSACTION` | Dua replika start bersamaan tidak boleh menjalankan migrasi ganda; perubahan skema harus kompatibel dengan binary versi sebelumnya selama rollout |
| Backup | `pg_dump -Fc` lewat `docker compose exec postgres`, ke volume `postgres-backups`; restore ke database baru lalu `engine verify --all` sebelum dipromosikan; PITR (WAL archiving) dicatat sebagai opsi Fase 8 | Compose satu node; `pg_dump` cukup untuk RPO harian; PITR butuh komponen tambahan (pgBackRest/WAL-G) yang masuk hardening Fase 8 |
| Injeksi kegagalan | `internal/platform/faultpoint` (aktif hanya dengan build tag `faultinject`, dikendalikan env `ENGINE_FAULTPOINTS`) + suite chaos Compose di `tools/scripts/chaos` | Fault point memberi skenario deterministik di level transaksi; chaos Compose membuktikan di level proses dan container dengan image yang sama dengan produksi |
| Metrik | API `go.opentelemetry.io/otel/metric` dibungkus `platform/metrics`, provider no-op sampai Fase 8 | Nama dan label metrik dikunci sekarang; exporter tinggal dipasang |

### 3.1 Model durabilitas: apa yang dijamin

| Kejadian | Jaminan |
|---|---|
| RPC mengembalikan sukses | Record + proyeksi + baris outbox + klaim idempotensi sudah di-commit dan di-fsync (`synchronous_commit=on`, default Postgres; tidak diubah) |
| Engine mati di tengah command | Transaksi rollback; tidak ada record, proyeksi, outbox, atau klaim setengah jadi; klien mendapat error transport dan mengulang dengan `request_id` yang sama |
| Engine mati setelah commit, sebelum event terkirim | Baris outbox tersimpan; relay mengirimnya setelah restart; sinyal in-process hilang (by design) |
| Engine mati di tengah pengiriman ke consumer durable | Transaksi relay rollback; event dikirim ulang (at-least-once); efek DB handler ikut rollback |
| Postgres mati | Engine menerima error; pool menyambung ulang; command yang belum commit hilang dan diulang klien; yang sudah commit aman |
| Proyeksi rusak (bug) | Terdeteksi `verify`; diperbaiki `rebuild` dari record; record tidak pernah diubah |
| Restore dari backup | State konsisten pada titik backup; event setelah titik backup hilang bersama state-nya (tidak ada event tanpa state); kursor consumer ikut dipulihkan |

### 3.2 Tata letak paket baru

```
apps/engine/internal/platform/
├─ outbox/
│  ├─ outbox.go              # Event, Writer, Consumer, Options
│  ├─ writer.go              # Enqueue(ctx, tx, events...) + NOTIFY
│  ├─ relay.go               # Relay: loop per consumer, advisory lock, batch, retry, dead letter
│  ├─ cursor.go              # kursor (tx_id, id)
│  ├─ queries.sql / db/      # sqlc
│  └─ relay_integration_test.go
├─ recovery/
│  ├─ recovery.go            # Participant, Runner, Report, OnMismatch
│  └─ readiness.go           # gerbang readiness (atomic), dipakai modul health untuk /readyz
├─ idempotency/
│  ├─ idempotency.go         # Claim, Begin, Store, Result, ErrKeyReused
│  ├─ hash.go                # hash request proto deterministik
│  └─ janitor.go             # hapus key kedaluwarsa
├─ paging/
│  └─ token.go               # Encode/Decode token keyset
├─ metrics/
│  └─ metrics.go             # Meter, nama metrik terdaftar
├─ faultpoint/
│  ├─ faultpoint.go          # Hit(name) no-op tanpa build tag
│  └─ faultpoint_inject.go   # //go:build faultinject
└─ db/
   ├─ db.go                  # Connect(cfg) dengan pool & RuntimeParams; Migrate dengan session lock
   └─ migrations/
      ├─ 00004_outbox.sql
      ├─ 00005_idempotency.sql
      └─ 00006_history.sql

apps/engine/internal/modules/execution/
├─ application/
│  ├─ unit_of_work.go        # UnitOfWork (pindahan dari domain.Repository.WithTx), lihat 3.3
│  ├─ outbox_events.go       # record → []outbox.Event
│  ├─ idempotent_command.go  # pembungkus command dengan request_id
│  ├─ verify.go              # VerifyInstances (cepat + dalam)
│  ├─ rebuild.go             # RebuildInstance, RebuildAll
│  ├─ snapshot.go            # kebijakan snapshot per N record
│  ├─ prune.go               # janitor retensi
│  ├─ list_records.go        # riwayat berpaginasi
│  └─ recovery_participant.go
├─ infrastructure/
│  ├─ postgres_unit_of_work.go
│  ├─ projector.go           # diperluas: upsert di semua jalur, last_sequence, updated_at
│  ├─ snapshot_codec.go      # InstanceState ↔ JSON
│  └─ queries/history.sql
└─ interfaces/
   └─ process_instance_handler.go  # RPC riwayat, request_id

apps/engine/cmd/engine/
├─ main.go                   # subperintah: healthcheck, verify, rebuild, prune, outbox
└─ admin.go

tools/scripts/chaos/         # runner chaos (Go) + skenario
infra/docker/docker-compose.chaos.yml
docs/ops/backup-restore.md
docs/ops/migrations.md
```

### 3.3 Perubahan kecil pada port Fase 2

`domain.Repository.WithTx` (Fase 2, 5.1) memberi `TxRepository` yang hanya tahu record dan state.
Fase 3 butuh transaksi yang sama untuk menulis outbox dan klaim idempotensi, dan keduanya bukan
urusan domain. Perubahan:

```go
// execution/application/unit_of_work.go
package application

// UnitOfWork membuka satu transaksi Postgres dan memberi semua port transaksional yang dibutuhkan
// use case. domain.TxRepository tidak berubah; outbox dan idempotensi hanya terlihat di application.
type UnitOfWork interface {
	Do(ctx context.Context, fn func(ctx context.Context, tx TxScope) error) error
}

type TxScope interface {
	Repo() domain.TxRepository
	Outbox() outbox.Writer      // Enqueue dalam transaksi ini
	Idempotency() idempotency.Tx // Begin/Store dalam transaksi ini
}
```

`domain.Repository` dihapus dari `domain/ports.go` (interface `TxRepository` tetap). Urutan
`command_runner.go` menjadi: lock/load → `Idempotency().Begin` → `Processor` → `AppendAndProject`
(+ `last_sequence`, `updated_at`, snapshot bila jatuh tempo) → `Outbox().Enqueue` →
`Idempotency().Store` → commit → `bus.Publish` (sinyal, best-effort, error hanya di-log).
Semantik domain tidak berubah; test skenario Fase 2 harus tetap lolos tanpa perubahan ekspektasi.

## 4. Outbox transaksional

### 4.1 Skema (`00004_outbox.sql`)

```sql
-- +goose Up
CREATE TABLE outbox_event (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tx_id          XID8 NOT NULL DEFAULT pg_current_xact_id(),  -- transaksi penulis; dasar pembacaan bebas celah
    event_id       UUID NOT NULL DEFAULT uuidv7(),              -- identitas untuk dedup di consumer
    topic          TEXT NOT NULL,                               -- "execution.job_created"
    aggregate_type TEXT NOT NULL,                               -- "process_instance", "deployment"
    aggregate_key  BIGINT NOT NULL,
    aggregate_seq  INT NOT NULL DEFAULT 0,                      -- sequence record pemicu; 0 bila tidak relevan
    payload        JSONB NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (event_id)
);
CREATE INDEX outbox_event_order_idx ON outbox_event (tx_id, id);
CREATE INDEX outbox_event_topic_idx ON outbox_event (topic, tx_id, id);

CREATE TABLE outbox_cursor (
    consumer    TEXT PRIMARY KEY,         -- nama consumer durable, stabil lintas versi
    tx_id       XID8 NOT NULL,
    id          BIGINT NOT NULL,
    attempts    INT NOT NULL DEFAULT 0,   -- percobaan gagal berturut-turut pada event di depan kursor
    last_error  TEXT,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE outbox_dead_letter (
    consumer    TEXT NOT NULL,
    event_id    UUID NOT NULL,
    topic       TEXT NOT NULL,
    payload     JSONB NOT NULL,
    error       TEXT NOT NULL,
    attempts    INT NOT NULL,
    failed_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer, event_id)
);

-- +goose Down
DROP TABLE outbox_dead_letter;
DROP TABLE outbox_cursor;
DROP TABLE outbox_event;
```

`XID8` dan `pg_current_xact_id()` tersedia sejak PostgreSQL 13; `uuidv7()` sejak PostgreSQL 18
(verifikasi pada image yang dipakai; cadangan: `gen_random_uuid()` dengan kehilangan urutan waktu,
yang tidak dipakai untuk pengurutan).

### 4.2 Tipe Go

```go
package outbox

type Event struct {
	ID            string          // UUID dari DB; kosong saat Enqueue, terisi saat diterima consumer
	Topic         string
	AggregateType string
	AggregateKey  int64
	AggregateSeq  int32
	Payload       json.RawMessage
	OccurredAt    time.Time
}

// Writer dipakai di dalam transaksi command. Implementasi juga mengirim NOTIFY outbox_wakeup
// (transaksional: baru sampai saat commit) agar relay bangun tanpa menunggu interval poll.
type Writer interface {
	Enqueue(ctx context.Context, tx pgx.Tx, events ...Event) error
}

// Consumer adalah subscriber durable. Handle dipanggil di dalam transaksi relay yang memajukan
// kursor; mengembalikan error membatalkan transaksi dan menjadwalkan percobaan ulang.
type Consumer struct {
	Name        string   // "timer.scheduler", "operate.projection"; wajib unik dan stabil
	Topics      []string // kosong = semua topik
	Handle      func(ctx context.Context, tx pgx.Tx, ev Event) error
	MaxAttempts int      // default 10; setelahnya event masuk dead letter dan kursor maju
	StartFrom   Start    // StartNow (default: kursor awal = posisi saat registrasi) | StartBeginning
}

type Options struct {
	PollInterval time.Duration // default 200 ms; LISTEN membuat jalur cepat < 10 ms
	BatchSize    int           // default 100
	Backoff      BackoffFunc   // default eksponensial 100 ms .. 30 s dengan jitter
}

type Relay struct{ /* pool, log, meter, consumers, opts */ }

func NewRelay(pool *pgxpool.Pool, log *slog.Logger, opts Options) *Relay
func (r *Relay) Register(c Consumer) error        // sebelum Run; nama ganda → error
func (r *Relay) Run(ctx context.Context) error    // blocking; satu goroutine per consumer
func (r *Relay) Drain(ctx context.Context) error  // untuk test: proses sampai tidak ada event baru
```

Event antarmodul tetap didefinisikan di `api` modul pemilik (Fase 1: `deployment/api`, Fase 2:
`execution/api/events.go`). Fase 3 menambahkan di tiap `api`:

```go
// execution/api/events.go
const (
	TopicProcessInstanceCompleted  = "execution.process_instance_completed"
	TopicProcessInstanceTerminated = "execution.process_instance_terminated"
	TopicJobCreated                = "execution.job_created"
	TopicIncidentCreated           = "execution.incident_created"
)

// DecodeEvent mengubah payload outbox menjadi tipe event api; dipakai consumer modul lain.
func DecodeEvent(topic string, payload []byte) (eventbus.Event, error)
```

Payload adalah JSON dari struct event `api` (field eksplisit, bukan `Record` mentah) sehingga
consumer tidak bergantung pada format internal record.

### 4.3 Relay: pembacaan bebas celah dan urutan

Loop per consumer:

```
1. pg_try_advisory_lock(hashtext('outbox:' || consumer)) pada koneksi khusus consumer;
   gagal → tidur 5 s, ulangi (replika lain memegang consumer ini).
2. Tunggu: NOTIFY outbox_wakeup, atau PollInterval.
3. BEGIN
   SELECT tx_id, id, attempts FROM outbox_cursor WHERE consumer = $1 FOR UPDATE;
   SELECT ... FROM outbox_event
     WHERE (tx_id, id) > ($cursor_tx, $cursor_id)
       AND tx_id < pg_snapshot_xmin(pg_current_snapshot())
       AND ($topics IS NULL OR topic = ANY($topics))
     ORDER BY tx_id, id
     LIMIT $batch;
   untuk setiap event: Handle(ctx, tx, ev)
   UPDATE outbox_cursor SET tx_id, id = event terakhir, attempts = 0;
   COMMIT
4. Handle gagal → ROLLBACK; ulangi batch satu per satu untuk mengisolasi event bermasalah;
   event yang gagal: attempts++ (di-commit sendiri), backoff; attempts ≥ MaxAttempts →
   dalam satu transaksi: INSERT outbox_dead_letter, kursor maju melewatinya, attempts = 0.
```

Mengapa `tx_id < pg_snapshot_xmin(...)`: `xmin` snapshot adalah xid terkecil yang **masih
berjalan**. Semua transaksi dengan xid lebih kecil sudah commit atau abort (baris abort tidak
terlihat). Kursor yang hanya maju melewati baris dengan `tx_id < xmin` tidak mungkin melompati
transaksi yang commit belakangan. Harga: event dari transaksi yang commit saat ada transaksi lebih
tua yang masih berjalan tertunda sampai transaksi tua itu selesai, dibatasi oleh
`idle_in_transaction_session_timeout` dan `statement_timeout` (9.1). Latensi normal: satu
interval LISTEN (< 10 ms).

Urutan per aggregate (lihat 3): untuk satu `process_instance_key`, transaksi command diserialkan
row lock, dan xid diambil saat lock, sehingga `tx_id` naik mengikuti urutan command. Relay
mengurutkan `(tx_id, id)`; dalam satu transaksi `id` naik sesuai urutan `Enqueue` (= urutan record).
Test 2 di DoD memeriksa `aggregate_seq` monoton per aggregate pada setiap consumer. Urutan **antar**
aggregate tidak dijanjikan.

Retensi outbox: janitor menghapus baris dengan `(tx_id, id)` di bawah kursor terendah semua consumer
dan `created_at < now() - ENGINE_OUTBOX_RETENTION` (default 24 jam). Consumer baru dengan
`StartBeginning` hanya melihat event di dalam retensi; dokumentasi consumer wajib menyebut ini.

### 4.4 Semantik pengiriman

| Jenis | Jaminan | Dipakai oleh |
|---|---|---|
| Consumer durable (`Relay.Register`) | At-least-once per consumer; urutan per aggregate; efek DB handler atomik dengan kursor; event bermasalah ke dead letter setelah `MaxAttempts` | Fase 5 timer start/message buffer, Fase 6 proyeksi Operate, test consumer Fase 3 |
| Sinyal in-process (`eventbus.Bus.Publish` pasca-commit) | Best-effort, hanya replika yang memproses command, hilang saat crash | Fase 1 cache model, Fase 4 pembangunan long polling (ditambah `LISTEN` bila lintas replika dibutuhkan) |

Handler durable dengan efek di luar DB (HTTP keluar, email) wajib menyimpan `event_id` yang sudah
diproses (tabel milik modul) karena transaksi relay bisa rollback setelah efek terjadi.
`outbox.Dedup(tx, consumer, eventID)` disediakan sebagai helper opsional di atas tabel
`outbox_processed(consumer, event_id)` dengan TTL sama dengan retensi outbox.

### 4.5 Migrasi publisher Fase 1–2

- `deployment/application` (`DeployResource` langkah 6 di dokumen Fase 1): `ProcessDeployed` ditulis
  ke outbox di dalam transaksi deploy; cache model tetap diisi setelah commit; `bus.Publish` tetap.
- `execution/application/outbox_events.go`: pemetaan record → event:

| Record | Event |
|---|---|
| `PROCESS_INSTANCE ELEMENT_COMPLETED` dengan `element_type = PROCESS` | `process_instance_completed` |
| `PROCESS_INSTANCE ELEMENT_TERMINATED` dengan `element_type = PROCESS` | `process_instance_terminated` |
| `JOB CREATED` | `job_created` (key, type, instance, element) |
| `INCIDENT CREATED` | `incident_created` |

`aggregate_seq` = `sequence` record pemicu. Satu command bisa menghasilkan banyak event; semua
dalam satu `Enqueue`.

## 5. Pemulihan setelah restart

### 5.1 Apa yang durable, apa yang in-flight

| Komponen | Durable (DB) | In-flight (hilang saat crash) | Pemulihan |
|---|---|---|---|
| Record + proyeksi | Ya, atomik | Transaksi command yang belum commit | Tidak ada; klien mengulang |
| Blok key `keygen` | `engine_key_seq` | Sisa blok yang belum dipakai | Tidak perlu; celah key diizinkan |
| Event domain | Baris outbox + kursor | Sinyal in-process; batch relay yang belum commit | Relay melanjutkan dari kursor |
| Klaim idempotensi | Baris `idempotency_key` | Klaim dalam transaksi yang rollback | Tidak perlu |
| Cache model BPMN (Fase 1) | — | LRU | Diisi ulang saat diminta |
| State menunggu (Fase 4 long polling, Fase 5 timer in-memory) | Sumber: tabel `job`, `timer` | Struktur in-memory | `recovery.Participant.Recover` membangun ulang dari tabel |

### 5.2 Urutan start (`cmd/engine/main.go`)

```
config → logger → db.Connect(cfg.DB) → db.Migrate (session lock)
→ recovery.Runner.Run:
     1. execution.Verify (cepat atas semua instance; dalam atas jendela terakhir)
     2. OnMismatch: fail → return error (proses keluar, HEALTHCHECK gagal, restart loop terlihat di log)
                    rebuild → RebuildInstance untuk setiap instance yang menyimpang, lalu Verify ulang
                    warn → lanjut, metrik engine_recovery_mismatch_total naik
     3. Participant.Recover setiap modul (urutan registrasi: deployment, execution, Fase 4+ job, timer, ...)
→ outbox.Relay.Run (goroutine)
→ janitor (outbox, idempotency, history) (goroutine, interval ENGINE_JANITOR_INTERVAL default 1 m)
→ readiness.Set(true) → httpserver.Run
```

Server HTTP **mendengarkan sejak awal** (agar `/healthz` menjawab liveness selama pemulihan panjang)
tetapi `/readyz` mengembalikan 503 dengan body tahap yang sedang berjalan sampai `readiness` true;
handler Connect mengembalikan `Unavailable` selama belum ready. Durasi pemulihan dibatasi
`ENGINE_RECOVERY_TIMEOUT` (default 10 m); melebihi → error start.

### 5.3 Verifikasi konsistensi

**Cepat (SQL, semua instance):**

```sql
-- name: VerifySequenceGaps :many  (last_sequence proyeksi ≠ record terakhir)
SELECT pi.key, pi.last_sequence, r.max_seq
FROM process_instance pi
JOIN (SELECT process_instance_key, MAX(sequence) AS max_seq, COUNT(*) AS n
      FROM execution_record GROUP BY process_instance_key) r ON r.process_instance_key = pi.key
WHERE pi.last_sequence <> r.max_seq OR r.n <> r.max_seq;   -- celah sequence

-- name: VerifyFinalState :many  (record terakhir PROCESS final tetapi state ACTIVE, atau sebaliknya)
-- name: VerifyOrphanJobs :many   (job CREATED yang element instance-nya final)
-- name: VerifyOpenIncidents :many (has_incident vs incident CREATED)
-- name: VerifyInstancesWithoutRecords :many
```

**Dalam (per instance):** `LoadRecords(key)` → `state := domain.NewInstanceState(process)` →
`Apply` setiap record → bandingkan dengan `LoadRuntimeState(key)` (element instance non-final,
counter join, variabel, job terbuka, incident terbuka) lewat perbandingan struktural dengan
pengurutan deterministik. Dipakai saat start untuk instance `ACTIVE` dengan `updated_at` dalam
`ENGINE_RECOVERY_DEEP_WINDOW` (default 10 m) ditambah sampel acak 100 instance; `engine verify
--all` untuk semua.

Hasil: `recovery.Report{Checked, Mismatched []Mismatch, Duration}`; setiap `Mismatch` dicatat log
ERROR dengan `process_instance_key`, jenis, dan ringkasan diff.

### 5.4 Rebuild proyeksi

```go
// execution/application/rebuild.go
func (uc *Rebuild) Instance(ctx context.Context, key int64) (RebuildResult, error) {
	return uc.uow.Do(ctx, func(ctx context.Context, tx TxScope) error {
		repo := tx.Repo()
		if err := repo.LockInstance(ctx, key); err != nil { return err }
		snap, ok, err := repo.LoadSnapshot(ctx, key)              // 7.1
		state := domain.NewInstanceState(...)
		from := int32(0)
		if ok { state = snap.State; from = snap.Sequence }
		if err := repo.DeleteProjection(ctx, key, from); err != nil { return err } // hapus proyeksi turunan
		for rec := range repo.StreamRecords(ctx, key, from+1) {    // urut sequence
			if err := domain.Apply(state, rec); err != nil { return fmt.Errorf("record %d: %w", rec.Sequence, err) }
			if err := repo.Project(ctx, rec); err != nil { return err }
		}
		return repo.FinalizeRebuild(ctx, key, state)              // last_sequence, version, updated_at
	})
}
```

- `DeleteProjection(key, 0)` menghapus baris `element_instance`, `taken_sequence_flow`, `variable`,
  `job`, `incident` instance tersebut; baris `process_instance` **dipertahankan** sebagai jangkar
  lock dan di-upsert oleh record `PROCESS_INSTANCE_CREATION CREATED`.
- Rebuild dari snapshot (`from > 0`) hanya untuk pemulihan **state runtime**; proyeksi elemen final
  sebelum snapshot tetap ada (tidak dihapus) karena record-nya masih ada. Mode `--full` mengabaikan
  snapshot.
- `RebuildAll` memproses instance berurutan key dengan paralelisme `ENGINE_REBUILD_WORKERS`
  (default 4); instance yang sedang dikunci command menunggu lock seperti command biasa.
- Rebuild **tidak** menyentuh outbox: event sudah terkirim saat command asli; rebuild bukan
  eksekusi ulang.

Prasyarat yang ditegakkan test: payload record memuat semua kolom proyeksi (element id, tipe,
flow scope, timestamp, variabel kanonik, header job); projector upsert di semua jalur.

### 5.5 Rekonsiliasi subscription

Pertanyaan Fase 1 ("subscriber Fase 2+ juga melakukan rekonsiliasi saat start") dijawab dengan dua
mekanisme:

1. **Consumer durable** tidak perlu rekonsiliasi: event yang belum terkirim masih di outbox dan
   dikirim setelah restart dari kursor.
2. **State in-memory turunan tabel** dibangun ulang oleh `Participant.Recover` dari tabel sumber:
   Fase 4 membaca `job WHERE state = 'CREATED'` untuk statistik antrean, Fase 5 membaca `timer
   WHERE due_at <= now() + horizon` untuk scheduler. Fase 3 menyediakan kontrak dan contoh
   (`execution` mendaftarkan participant yang mencatat jumlah instance aktif dan job terbuka ke log
   dan metrik).

```go
package recovery

type Participant interface {
	Name() string
	Recover(ctx context.Context) error // dipanggil sekali saat start, berurutan, sebelum ready
}

type Runner struct{ /* participants, verify, onMismatch, log, meter, timeout */ }
func (r *Runner) Run(ctx context.Context) (Report, error)
```

### 5.6 Subperintah CLI

| Perintah | Fungsi |
|---|---|
| `engine verify [--all] [--instance K] [--json]` | Verifikasi cepat + dalam; exit 0 bila konsisten, 2 bila ada mismatch |
| `engine rebuild --instance K [--full]` / `--all [--workers N]` | Rebuild proyeksi dari record |
| `engine prune [--dry-run] [--older-than 90d]` | Jalankan janitor retensi sekali |
| `engine outbox status` / `engine outbox requeue --consumer C --event-id E` | Lag per consumer, isi dead letter, kirim ulang dead letter |

Semua memakai `DATABASE_URL`, tanpa server HTTP, dan bisa dijalankan dari image yang sama:
`docker compose exec engine /engine verify --all`.

## 6. Idempotensi

### 6.1 Semantik

| Situasi | Perilaku |
|---|---|
| Request tanpa `request_id` | Seperti Fase 2: tidak ada dedup; `CompleteJob` ganda ditolak `FailedPrecondition` oleh state |
| `request_id` baru | Command dijalankan; response disimpan bersama hash request dalam transaksi yang sama |
| `request_id` sama, request identik, berurutan | Response tersimpan dikembalikan byte-identik tanpa menjalankan command; header response `Idempotent-Replayed: true` |
| `request_id` sama, request identik, **paralel** | Request kedua memblokir pada unique index sampai yang pertama commit/abort; commit → replay; abort → kedua menjalankan command |
| `request_id` sama, payload berbeda | `InvalidArgument`, detail `IDEMPOTENCY_KEY_REUSED` |
| Command pertama gagal dengan error domain | Transaksi rollback, tidak ada klaim; retry menjalankan ulang dan (deterministik) gagal dengan error yang sama |
| Key kedaluwarsa (TTL) | Diperlakukan sebagai key baru |

Scope klaim: `tenant_id + "/" + nama RPC lengkap`, sehingga `request_id` yang sama untuk
`CreateProcessInstance` dan `CompleteJob` tidak bertabrakan. `request_id`: 1–128 byte, UTF-8,
dianjurkan UUID; divalidasi di interfaces.

### 6.2 Skema (`00005_idempotency.sql`)

```sql
-- +goose Up
CREATE TABLE idempotency_key (
    scope        TEXT NOT NULL,
    key          TEXT NOT NULL,
    request_hash BYTEA NOT NULL,           -- SHA-256 request proto deterministik, request_id dikosongkan
    response     BYTEA,                    -- proto response; NULL hanya di dalam transaksi yang belum commit
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (scope, key)
);
CREATE INDEX idempotency_key_expires_idx ON idempotency_key (expires_at);

-- +goose Down
DROP TABLE idempotency_key;
```

### 6.3 Alur dan tipe Go

```go
package idempotency

type Claim struct {
	Scope       string
	Key         string
	RequestHash [32]byte
}

type Outcome uint8
const (
	Claimed Outcome = iota + 1 // jalankan command, lalu Store sebelum commit
	Replayed                   // Response terisi; jangan jalankan command
)

type Result struct {
	Outcome  Outcome
	Response []byte
}

var ErrKeyReused = errors.New("idempotency key reused with different request")

type Tx interface {
	// Begin: INSERT ... ON CONFLICT DO NOTHING RETURNING; 0 baris → baca baris yang ada:
	// hash sama → Replayed; hash beda → ErrKeyReused. Dipanggil SETELAH lock aggregate (3).
	Begin(ctx context.Context, c Claim, ttl time.Duration) (Result, error)
	// Store mengisi response pada baris yang diklaim transaksi ini.
	Store(ctx context.Context, c Claim, response proto.Message) error
}

// Hash menghasilkan SHA-256 dari proto.MarshalOptions{Deterministic: true} setelah request_id dikosongkan.
func Hash(msg proto.Message, clearRequestID func(proto.Message)) [32]byte
```

Pembungkus di `execution/application/idempotent_command.go` memakai `UnitOfWork` yang sama dengan
command: `Replayed` → use case mengembalikan response tersimpan tanpa menyentuh `Processor`.
Untuk `CreateProcessInstance` (belum ada aggregate untuk dikunci) klaim dilakukan di awal transaksi
— aman karena instance baru tidak punya pesaing. Untuk `DeployResource`, klaim setelah advisory
lock per `bpmn_process_id` (Fase 1, 5.2). Interceptor Connect di `platform/httpserver` menambahkan
header `Idempotent-Replayed` berdasarkan nilai di `context`.

### 6.4 Perubahan proto (aditif, `buf breaking` lolos)

```proto
// workflow/process/v1/process_instance.proto
message CreateProcessInstanceRequest {
  oneof definition { int64 process_definition_key = 1; ProcessDefinitionRef bpmn_process = 2; }
  string variables = 3;
  string request_id = 4;   // opsional; 1..128 byte; idempoten selama ENGINE_IDEMPOTENCY_TTL
}
message CancelProcessInstanceRequest { int64 process_instance_key = 1; string request_id = 2; }

// workflow/job/v1/job.proto
message CompleteJobRequest { int64 job_key = 1; string variables = 2; string request_id = 3; }

// workflow/deployment/v1/deployment.proto
message DeployResourceRequest { /* field Fase 1 */ string request_id = <berikutnya>; }

// Detail error (workflow/common/v1/errors.proto, baru)
message IdempotencyKeyReused { string scope = 1; string request_id = 2; }
```

Camunda 8.8 tidak mengekspos idempotency key pada `CreateProcessInstance`/`CompleteJob` sepengetahuan
penulis (verifikasi terhadap dokumentasi Camunda 8.8); ini perluasan yang tidak memengaruhi paritas
karena opsional.

## 7. Snapshot, retensi, dan paginasi riwayat

### 7.1 Snapshot (`00006_history.sql`, bagian 1)

```sql
ALTER TABLE process_instance
    ADD COLUMN last_sequence INT NOT NULL DEFAULT 0,
    ADD COLUMN updated_at    TIMESTAMPTZ NOT NULL DEFAULT now();
-- backfill dari record (dijalankan sekali; tabel kosong di dev)
UPDATE process_instance pi SET last_sequence = r.max_seq
FROM (SELECT process_instance_key, MAX(sequence) AS max_seq FROM execution_record GROUP BY 1) r
WHERE r.process_instance_key = pi.key;
CREATE INDEX process_instance_updated_idx ON process_instance (updated_at) WHERE state = 'ACTIVE';
CREATE INDEX process_instance_retention_idx ON process_instance (end_time) WHERE state <> 'ACTIVE';

CREATE TABLE execution_snapshot (
    process_instance_key BIGINT PRIMARY KEY REFERENCES process_instance (key) ON DELETE CASCADE,
    sequence             INT NOT NULL,         -- state setelah record ini di-Apply
    format_version       INT NOT NULL,         -- versi codec InstanceState; rebuild menolak versi tak dikenal → mulai dari nol
    state                JSONB NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

```go
// execution/domain/snapshot.go (stdlib saja)
type Snapshot struct {
	FormatVersion int
	Sequence      int32
	Elements      []ElementInstance
	TakenFlows    []TakenFlow
	Variables     []VariableEntry
	Jobs          []Job
	Incidents     []Incident
}
func (s *InstanceState) Snapshot(seq int32) Snapshot
func RestoreSnapshot(process *bpmn.Process, snap Snapshot) (*InstanceState, error)
```

Kebijakan: setelah `AppendAndProject`, jika `last_sequence / ENGINE_SNAPSHOT_INTERVAL_RECORDS`
bertambah dan instance masih `ACTIVE`, tulis/ganti snapshot dalam transaksi yang sama. Instance
final tidak di-snapshot (snapshot dihapus bersama retensi lewat `ON DELETE CASCADE`). Kompaksi
(pemangkasan record `sequence ≤ snapshot.sequence`) **tidak diimplementasikan**; format snapshot dan
`format_version` ada agar Fase 6 bisa mengaktifkannya bila riwayat dipindah ke penyimpanan lain.

### 7.2 Retensi

Janitor `execution/application/prune.go`, dijalankan setiap `ENGINE_JANITOR_INTERVAL` (default 1
menit) bila `ENGINE_HISTORY_RETENTION_DAYS > 0`:

```sql
-- name: PickExpiredInstances :many
SELECT key FROM process_instance
WHERE state <> 'ACTIVE' AND end_time < $1
ORDER BY end_time
LIMIT $2 FOR UPDATE SKIP LOCKED;
-- lalu, dalam transaksi yang sama, per batch key:
DELETE FROM execution_record WHERE process_instance_key = ANY($1);
DELETE FROM incident ...; DELETE FROM job ...; DELETE FROM variable ...;
DELETE FROM taken_sequence_flow ...; DELETE FROM element_instance ...;
DELETE FROM process_instance WHERE key = ANY($1);   -- snapshot ikut lewat CASCADE
```

Batch 500 instance, jeda `ENGINE_HISTORY_PRUNE_PAUSE` (default 200 ms) antar batch, `lock_timeout`
pool berlaku. `process_definition` tidak pernah dihapus (Fase 1 tidak punya `DeleteResource`).
Outbox dan idempotency punya janitor masing-masing (retensi 24 jam). Metrik
`engine_history_pruned_instances_total`.

### 7.3 Paginasi riwayat

```proto
service ProcessInstanceService {
  // ... RPC Fase 2
  rpc ListProcessInstanceRecords(ListProcessInstanceRecordsRequest) returns (ListProcessInstanceRecordsResponse) {}
  rpc ListElementInstances(ListElementInstancesRequest) returns (ListElementInstancesResponse) {}
}

message ListProcessInstanceRecordsRequest {
  int64 process_instance_key = 1;
  int32 page_size = 2;      // default 100, maks 500
  string page_token = 3;    // opak; keyset atas sequence
}
message ExecutionRecord {
  int64 position = 1;
  int32 sequence = 2;
  int64 record_key = 3;
  string value_type = 4;
  string intent = 5;
  string element_id = 6;
  string payload = 7;       // JSON
  google.protobuf.Timestamp timestamp = 8;
}
message ListProcessInstanceRecordsResponse { repeated ExecutionRecord records = 1; string next_page_token = 2; }

message ListElementInstancesRequest {
  int64 process_instance_key = 1;
  bool only_active = 2;
  int32 page_size = 3;
  string page_token = 4;    // keyset atas key
}
message ListElementInstancesResponse { repeated ElementInstance element_instances = 1; string next_page_token = 2; }

message GetProcessInstanceResponse {
  // ... field Fase 2
  bool element_instances_truncated = 6;  // true bila > 1.000; pakai ListElementInstances
}
```

```go
package paging

type Token struct{ Key int64 `json:"k"` }
func Encode(t Token) string              // base64url(JSON)
func Decode(s string) (Token, error)     // salah bentuk → ErrInvalidToken → InvalidArgument
```

Kueri: `WHERE process_instance_key = $1 AND sequence > $2 ORDER BY sequence LIMIT $3 + 1`
(baris ekstra menentukan `next_page_token`). `ListProcessInstances` dan `ListJobs` Fase 2 diubah
memakai helper yang sama (tanpa perubahan kontrak).

## 8. Key dan kesiapan partisi

| Opsi | Uraian | Kelebihan | Kekurangan |
|---|---|---|---|
| A. **Sequence global + alokasi blok** (Fase 1–2, dipertahankan) | `engine_key_seq`, blok 64 per proses | Sederhana; unik global; sudah ada; key kecil dan terurut kasar | Satu sequence per database (bukan hambatan: `nextval` blok ~1 µs) |
| B. Bit partisi ala Zeebe | Key = `partition_id` di bit atas + counter per partisi (verifikasi tata letak persis di dokumentasi/sumber Camunda 8.8) | Key menyebut partisi tanpa lookup | Butuh keputusan jumlah partisi sekarang; counter per partisi harus durable; mengubah `position`/`sequence`; tidak ada manfaat tanpa log per partisi |
| C. UUIDv7 | 128-bit | Tanpa koordinasi | Mengubah tipe di proto/skema/SDK; tidak kompatibel dengan konvensi key int64 Camunda |

**Rekomendasi: A** (ADR-0017). Yang dijaga agar partisi bisa menyusul tanpa migrasi key:

1. Tidak ada kode yang mengasumsikan key rapat atau terurut antar instance; urutan hanya lewat
   `position`/`sequence` record.
2. Semua tabel state punya `process_instance_key`; sharding kepemilikan instance antar replika (bila
   contention terukur) memakai `hash(process_instance_key) mod N` di lapisan routing, bukan isi key.
3. `position` identity global `execution_record` adalah kursor exporter; jika kelak log dipecah per
   partisi, kursor menjadi `(partition, position)` — kompatibel karena partisi default = 0.
4. Key tetap positif dan `< 2^63`; klien TS menerima `int64` sebagai string (Connect JSON) sehingga
   tidak ada batas 2^53.

Partisi tabel Postgres (range `position` untuk `execution_record`) **tidak** dilakukan sekarang;
dicatat sebagai pemicu: > 100 juta record atau pemangkasan retensi yang mengganggu (drop partition
lebih murah daripada `DELETE`).

## 9. Operasional

### 9.1 Pool dan timeout

```go
// platform/db/db.go
type Config struct {
	URL                string        `env:"DATABASE_URL"`
	MaxConns           int32         `env:"ENGINE_DB_MAX_CONNS"`                 // default max(4, 2*NumCPU)
	MinConns           int32         `env:"ENGINE_DB_MIN_CONNS" envDefault:"2"`
	MaxConnLifetime    time.Duration `env:"ENGINE_DB_MAX_CONN_LIFETIME" envDefault:"30m"`
	MaxConnIdleTime    time.Duration `env:"ENGINE_DB_MAX_CONN_IDLE_TIME" envDefault:"5m"`
	StatementTimeout   time.Duration `env:"ENGINE_DB_STATEMENT_TIMEOUT" envDefault:"10s"`
	LockTimeout        time.Duration `env:"ENGINE_DB_LOCK_TIMEOUT" envDefault:"5s"`
	IdleInTxTimeout    time.Duration `env:"ENGINE_DB_IDLE_IN_TX_TIMEOUT" envDefault:"15s"`
}

func Connect(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	pc, _ := pgxpool.ParseConfig(cfg.URL)
	pc.MaxConns, pc.MinConns = cfg.MaxConns, cfg.MinConns
	pc.MaxConnLifetime, pc.MaxConnIdleTime = cfg.MaxConnLifetime, cfg.MaxConnIdleTime
	pc.ConnConfig.RuntimeParams["application_name"] = "engine"
	pc.ConnConfig.RuntimeParams["statement_timeout"] = ms(cfg.StatementTimeout)
	pc.ConnConfig.RuntimeParams["lock_timeout"] = ms(cfg.LockTimeout)
	pc.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = ms(cfg.IdleInTxTimeout)
	// ...
}
```

Koneksi relay (LISTEN + advisory lock per consumer) memakai `SET statement_timeout = 0` lokal
karena menunggu notifikasi lama. Perintah admin (`rebuild --all`, `verify --all`) memakai
`statement_timeout` terpisah (`--statement-timeout`, default 5 m). Aturan kapasitas:
`replika × ENGINE_DB_MAX_CONNS + 10 ≤ max_connections` Postgres (default 100); dokumentasikan di
`docs/ops/`. Uji paritas dan integrasi memakai `ENGINE_DB_MAX_CONNS=8`.

### 9.2 Runbook backup/restore (`docs/ops/backup-restore.md`)

```
make db-backup                       # → infra volume postgres-backups: workflow-<UTC timestamp>.dump
  = docker compose -f docker-compose.prod.yml exec -T postgres \
      pg_dump -U $POSTGRES_USER -d $POSTGRES_DB -Fc -Z 6 -f /backups/workflow-$(date -u +%Y%m%dT%H%M%SZ).dump
make db-backup-list
make db-restore FILE=workflow-....dump [TARGET_DB=workflow_restore]
  1. createdb $TARGET_DB
  2. pg_restore -U ... -d $TARGET_DB --no-owner --exit-on-error /backups/$FILE
  3. docker compose run --rm -e DATABASE_URL=...$TARGET_DB engine /engine verify --all
  4. (manual) hentikan engine → ganti nama database (ALTER DATABASE ... RENAME) → start engine
```

Catatan runbook: backup konsisten per transaksi (snapshot `pg_dump`), engine tidak perlu berhenti;
event outbox yang belum terkirim pada saat backup **ikut** dipulihkan dan akan dikirim ulang setelah
restore (consumer harus idempoten — ini dijamin desain); event setelah titik backup hilang bersama
state-nya. Verifikasi restore adalah bagian DoD, bukan opsional. Backup terjadwal dan offsite:
Fase 8.

### 9.3 Catatan PITR

`postgres:18-alpine` default tidak mengarsipkan WAL. PITR membutuhkan `archive_mode=on`,
`archive_command` ke penyimpanan terpisah, base backup berkala (`pg_basebackup` atau pgBackRest /
WAL-G), dan prosedur `recovery_target_time`. Fase 3 hanya mencatat opsi dan menambahkan
`command: postgres -c wal_level=replica` di compose prod agar pengaktifan PITR di Fase 8 tidak
butuh restart dengan perubahan `wal_level`. RPO Fase 3 = interval `db-backup` yang dijalankan
operator.

### 9.4 Migrasi aman (`docs/ops/migrations.md`)

1. **Expand/contract**: setiap perubahan skema dipecah menjadi (a) expand — tambah kolom/tabel/index
   yang kompatibel dengan binary lama, (b) rilis binary baru, (c) contract — hapus yang tidak dipakai
   di rilis berikutnya. Tidak pernah `DROP`/`RENAME` kolom yang masih dibaca binary yang berjalan.
2. **Index besar**: `CREATE INDEX CONCURRENTLY` dalam file migrasi dengan `-- +goose NO TRANSACTION`.
3. **Backfill**: dalam batch oleh migrasi atau janitor, bukan satu `UPDATE` tabel penuh (contoh
   `last_sequence` di 7.1 diterima karena tabel kosong sebelum Fase 3; di produksi dengan data,
   migrasi menulis `DEFAULT 0` dan janitor mengisi).
4. **Lock**: `db.Migrate` memakai `goose.NewProvider(..., goose.WithSessionLocker(lock.NewPostgresSessionLocker()))`
   (verifikasi API goose v3.29); replika kedua menunggu, lalu melihat versi sudah terbaru.
5. **Down migration** wajib ada dan diuji di CI (`up` → `down` → `up` pada DB kosong).
6. Rollback binary tanpa rollback skema harus aman selama fase contract belum dijalankan; tes CI
   menjalankan integration test binary versi `main` terhadap skema branch (job `compat`, opsional).

### 9.5 Konfigurasi baru (ringkasan)

| Variabel | Default | Keterangan |
|---|---|---|
| `ENGINE_DB_MAX_CONNS`, `..._MIN_CONNS`, `..._MAX_CONN_LIFETIME`, `..._MAX_CONN_IDLE_TIME` | lihat 9.1 | pool |
| `ENGINE_DB_STATEMENT_TIMEOUT`, `..._LOCK_TIMEOUT`, `..._IDLE_IN_TX_TIMEOUT` | 10s, 5s, 15s | timeout sesi |
| `ENGINE_OUTBOX_POLL_INTERVAL`, `..._BATCH_SIZE`, `..._RETENTION`, `..._MAX_ATTEMPTS` | 200ms, 100, 24h, 10 | relay |
| `ENGINE_RECOVERY_ON_MISMATCH` | `fail` | `fail`/`rebuild`/`warn` |
| `ENGINE_RECOVERY_DEEP_WINDOW`, `..._TIMEOUT`, `ENGINE_REBUILD_WORKERS` | 10m, 10m, 4 | pemulihan |
| `ENGINE_IDEMPOTENCY_TTL` | 24h | klaim |
| `ENGINE_SNAPSHOT_INTERVAL_RECORDS` | 5000 | snapshot |
| `ENGINE_HISTORY_RETENTION_DAYS`, `..._PRUNE_PAUSE` | 0 (nonaktif), 200ms | retensi |
| `ENGINE_JANITOR_INTERVAL` | 1m | semua janitor |
| `ENGINE_FAULTPOINTS` | kosong | hanya build tag `faultinject` |

## 10. Hook observability (bukti pemulihan)

Log `slog` terstruktur, satu baris per kejadian, field konsisten (`component`, `phase`,
`duration_ms`, `process_instance_key`, `consumer`, `event_id`):

| Kejadian | Level | Field |
|---|---|---|
| `recovery started` / `recovery finished` | INFO | `phases`, `duration_ms`, `checked`, `mismatched` |
| `projection mismatch` | ERROR | `process_instance_key`, `kind`, `diff` (ringkas) |
| `instance rebuilt` | INFO/WARN | `process_instance_key`, `records`, `from_snapshot` |
| `outbox consumer acquired/lost` | INFO | `consumer`, `replica` |
| `outbox batch delivered` | DEBUG | `consumer`, `count`, `lag_ms` |
| `outbox event failed` / `dead lettered` | WARN/ERROR | `consumer`, `event_id`, `attempts`, `error` |
| `idempotent replay` | INFO | `scope`, `request_id` |
| `history pruned` | INFO | `instances`, `duration_ms` |
| `db pool stats` (tiap 30 s) | DEBUG | `acquired`, `idle`, `max`, `wait_ms` |

Metrik (`platform/metrics`, API OTel, provider no-op; Phase 8 memasang exporter):

| Nama | Tipe | Label |
|---|---|---|
| `engine_outbox_pending_events` | gauge | `consumer` |
| `engine_outbox_lag_seconds` | gauge | `consumer` |
| `engine_outbox_delivered_total`, `engine_outbox_failed_total`, `engine_outbox_dead_letter_total` | counter | `consumer`, `topic` |
| `engine_recovery_duration_seconds` | histogram | `phase` |
| `engine_recovery_mismatch_total` | counter | `kind` |
| `engine_idempotency_total` | counter | `rpc`, `outcome` (`claimed`/`replayed`/`reused`) |
| `engine_command_duration_seconds` | histogram | `command` |
| `engine_command_lock_wait_seconds` | histogram | `command` (bukti contention untuk 8) |
| `engine_history_pruned_instances_total` | counter | — |
| `engine_db_pool_acquire_wait_seconds` | histogram | — |

Sementara belum ada exporter, `engine verify --json` dan `engine outbox status` menjadi alat
operator; `/readyz` mengembalikan JSON tahap pemulihan terakhir.

## 11. Uji injeksi kegagalan

### 11.1 Fault point di kode

```go
package faultpoint

// Hit mengecek apakah fault point aktif. Tanpa build tag faultinject selalu no-op (dikompilasi
// hilang). Dengan tag, ENGINE_FAULTPOINTS="outbox.before_cursor_commit=panic,command.after_append=error"
// menentukan aksi: panic (simulasi crash proses), error (kembalikan error), sleep:<durasi>.
func Hit(ctx context.Context, name string) error
```

Titik yang dipasang: `command.after_lock`, `command.after_append` (sebelum outbox),
`command.before_commit`, `command.after_commit` (sebelum sinyal in-process),
`outbox.before_cursor_commit`, `outbox.after_handle`, `idempotency.after_claim`,
`rebuild.mid_stream`, `snapshot.before_write`. Integration test (`-tags=integration,faultinject`)
menjalankan handler dalam proses yang sama dan memeriksa invarian setelah "crash" tiruan (panic
ditangkap di goroutine test; untuk crash sungguhan dipakai 11.2).

### 11.2 Chaos berbasis Compose

`infra/docker/docker-compose.chaos.yml`: `postgres` + `engine` (image dari `make docker-build`,
`restart: "no"`, `ENGINE_FAULTPOINTS` opsional lewat image build tag `faultinject` untuk skenario
tertentu) + `engine-2` (profil `replicas`). Runner `tools/scripts/chaos` (Go, memakai Docker CLI):

1. Deploy korpus proses (linear 20 service task, fork 50 cabang, loop 1.000 iterasi).
2. Load generator: N goroutine membuat instance dan menyelesaikan job lewat API dengan
   `request_id` per command, mencatat setiap command yang **di-ack**.
3. Injeksi (per skenario), tunggu pemulihan (`/readyz` 200).
4. Verifikasi: setiap command yang di-ack terlihat di state; command yang tidak di-ack diulang
   dengan `request_id` sama dan hasil akhirnya tepat satu; `engine verify --all` exit 0; consumer
   uji (durable, menulis `event_id` ke tabel `chaos_received`) menerima setiap event tepat sekali dan
   `aggregate_seq` monoton; tidak ada dead letter.

`make chaos [SCENARIO=all|kill-engine|...] [DURATION=60s]`; CI menjalankannya nightly (job
`chaos`, bukan per PR).

### 11.3 Tabel skenario

| # | Skenario | Injeksi | Hasil yang diharapkan |
|---|---|---|---|
| 1 | Engine mati di tengah command | `docker kill -s KILL engine` saat load | Setelah restart: semua command ter-ack ada; tidak ada record/proyeksi parsial; `verify` bersih; retry dengan `request_id` sama → tidak ada instance/penyelesaian ganda |
| 2 | Engine mati setelah commit sebelum relay kirim | Fault point `command.after_commit=panic` | Event terkirim setelah restart; consumer menerima tepat sekali |
| 3 | Relay mati setelah handler sebelum kursor commit | `outbox.before_cursor_commit=panic` | Batch dikirim ulang; efek DB handler tidak ganda (rollback bersama kursor); consumer dengan efek luar mendeteksi `event_id` ganda |
| 4 | Postgres restart | `docker compose restart postgres` saat load | Command in-flight gagal `Unavailable`; pool menyambung ulang tanpa restart engine; command ter-ack utuh; relay melanjutkan |
| 5 | Postgres dibunuh paksa | `docker kill -s KILL postgres` | Sama dengan 4 ditambah crash recovery WAL; tidak ada command ter-ack yang hilang (`synchronous_commit=on`) |
| 6 | Partisi jaringan engine ↔ Postgres | `docker network disconnect` 10 s | Error sementara; setelah tersambung, relay dan janitor pulih tanpa event ganda ke consumer durable |
| 7 | Request idempoten paralel | 20 request identik serentak | Satu instance; 20 response byte-identik; `engine_idempotency_total{outcome="replayed"}` = 19 |
| 8 | Key dipakai ulang dengan payload beda | request kedua variabel berbeda | `InvalidArgument` + `IDEMPOTENCY_KEY_REUSED`; state tidak berubah |
| 9 | Proyeksi dirusak | Test menghapus baris `element_instance` dan mengubah `variable` | `verify` mendeteksi (cepat atau dalam); `rebuild --instance` memulihkan; dump identik dengan sebelum perusakan |
| 10 | Dua replika | profil `replicas`, load ke keduanya | Consumer durable memproses tiap event sekali; `aggregate_seq` monoton; `CompleteJob` pada instance sama dari dua replika tidak menghasilkan record ganda (`UNIQUE (process_instance_key, sequence)` tidak pernah terpicu) |
| 11 | Commit tidak urut | `command.before_commit=sleep:2s` pada separuh command | Relay tidak melewatkan event; kursor baru maju melewati transaksi lambat setelah commit |
| 12 | Start bersamaan pada DB kosong | Dua replika start serentak | Migrasi berjalan sekali; keduanya ready |
| 13 | Restore backup | `db-backup` saat load, lanjutkan load, `db-restore` ke DB baru | `verify --all` bersih pada DB restore; event setelah backup tidak ada (didokumentasikan) |
| 14 | Retensi saat load | `ENGINE_HISTORY_RETENTION_DAYS=0.001` (menit, via flag uji) | Instance final dihapus; instance aktif tidak terganggu; tidak ada deadlock |
| 15 | Loop panjang + snapshot | Proses loop 20.000 iterasi, kill di tengah | Rebuild dari snapshot < 2 s; state identik dengan `--full` |
| 16 | Consumer beracun | Handler uji selalu gagal untuk satu `event_id` | Setelah `MaxAttempts` masuk dead letter; consumer lanjut; `requeue` mengirim ulang setelah handler diperbaiki |

## 12. Rencana kerja Fase 3 (urutan eksekusi)

Setiap langkah punya hasil yang bisa diverifikasi. 12.2–12.3 (outbox) paralel dengan 12.6
(idempotensi) dan 12.8 (operasional); 12.4–12.5 (pemulihan) butuh projector Fase 2 final.

### 12.1 Prasyarat

- Fase 2 merge: `execution_record`, projector, `domain.Apply`, skenario dan test determinisme ada di
  `main`. Jika belum, 12.2 dan 12.6 dikerjakan terhadap modul `deployment` (Fase 1) dahulu.
- Branch `feat/phase-3-persistence` dari `main`; `make check` dan `make test-integration` hijau.

Hasil: branch siap; catatan kondisi awal di bagian 0 diperbarui.

### 12.2 Platform `outbox`: skema, writer, relay

- `00004_outbox.sql`, sqlc, `Writer.Enqueue` + `NOTIFY`, `Relay` (advisory lock, LISTEN + poll,
  batch, isolasi event gagal, backoff, dead letter), `Drain` untuk test.
- Integration test: 200 transaksi paralel dengan commit tertunda acak (fault point) → tidak ada event
  terlewat; consumer gagal → retry → dead letter; dua relay pada satu DB → satu pemegang per
  consumer; retensi.
- Verifikasi `uuidv7()` dan `XID8` di `postgres:18-alpine`.

Hasil: `go test -tags=integration ./apps/engine/internal/platform/outbox/...` lolos; ADR-0014 draf.

### 12.3 Migrasi publisher ke outbox + `UnitOfWork`

- `application.UnitOfWork`/`TxScope` (3.3); `postgres_unit_of_work.go`; `outbox_events.go`;
  `deployment` menulis `process_deployed` ke outbox.
- `main.go` merakit relay; consumer uji `execution.audit` (menulis `event_id` ke tabel test) di
  integration test; `bus.Publish` pasca-commit tetap.
- Test monotonisitas `aggregate_seq`: 50 `CompleteJob` paralel × 20 ulangan.

Hasil: semua skenario Fase 2 tetap lolos; DoD butir 2–4 terbukti.

### 12.4 Verifikasi dan rebuild

- `00006_history.sql` bagian `last_sequence`/`updated_at`; projector mengisi keduanya dan upsert di
  semua jalur (test idempotensi `Project`).
- `verify.go` (kueri cepat + replay dalam), `rebuild.go`, `DeleteProjection`, `StreamRecords`,
  `FinalizeRebuild`; `recovery` platform (`Participant`, `Runner`, readiness); `/readyz`; urutan
  start 5.2; `HEALTHCHECK` ke `/readyz`.
- Integration test: truncate → rebuild → dump identik untuk setiap skenario; perusakan disengaja
  terdeteksi; mode `fail`/`rebuild`/`warn`.

Hasil: `engine verify --all` dan `engine rebuild --all` berjalan dari CLI; ADR-0015 draf.

### 12.5 Snapshot

- `domain.Snapshot`/`RestoreSnapshot`, codec JSON `format_version = 1`, kebijakan interval, rebuild
  dari snapshot, `--full`.
- Test: skenario loop Fase 2 dengan interval kecil (10 record) → rebuild dari snapshot identik dengan
  dari nol; `format_version` tak dikenal → mulai dari nol dengan WARN.

Hasil: DoD butir snapshot; benchmark rebuild instance 100.000 record dicatat.

### 12.6 Idempotensi

- `00005_idempotency.sql`, paket `idempotency` (`Begin`, `Store`, `Hash`, janitor), proto
  `request_id` + `IdempotencyKeyReused`, `buf breaking` lolos, generate.
- `idempotent_command.go` di `execution`, pembungkus serupa di `deployment`; interceptor header
  `Idempotent-Replayed`; validasi `request_id` di interfaces.
- Test: berurutan, paralel 20, payload beda, TTL kedaluwarsa, error domain tidak diklaim.

Hasil: DoD butir idempotensi; ADR-0016 draf; SDK stub TS menerima `requestId`.

### 12.7 Riwayat berpaginasi dan retensi

- `paging` helper; RPC `ListProcessInstanceRecords`, `ListElementInstances`;
  `element_instances_truncated`; `ListProcessInstances`/`ListJobs` memakai helper.
- `prune.go` + janitor + `engine prune`; index retensi; test retensi saat load.

Hasil: `buf curl` riwayat 10.000 record berhalaman; `engine prune --dry-run` melaporkan kandidat;
ADR-0018 draf.

### 12.8 Operasional: pool, timeout, migrasi, backup

- `db.Config` + `Connect` (9.1); test `statement_timeout` (`57014`); stats pool ke log DEBUG.
- `Migrate` dengan session lock; test dua proses paralel; CI job `up → down → up`.
- `make db-backup`, `db-backup-list`, `db-restore`; volume `postgres-backups`; `wal_level=replica`
  di compose prod; `docs/ops/backup-restore.md`, `docs/ops/migrations.md`; restore diuji end-to-end.

Hasil: runbook dijalankan sekali dengan bukti di bagian 0; `.env.prod.example` diperbarui.

### 12.9 Observability hooks

- `platform/metrics` (OTel API, provider no-op, daftar nama terkunci + test snapshot); log
  terstruktur 10 di semua komponen baru; `engine outbox status`; `/readyz` JSON.

Hasil: `go test ./apps/engine/internal/platform/metrics` memverifikasi daftar; log pemulihan terlihat
saat `make dev`.

### 12.10 Fault point dan suite chaos

- `platform/faultpoint` + titik 11.1; integration test `-tags=integration,faultinject` untuk
  skenario 2, 3, 11.
- `docker-compose.chaos.yml`, `tools/scripts/chaos` (load generator, injeksi, verifikasi), `make
  chaos`, job CI nightly.

Hasil: `make chaos` hijau tiga kali berturut-turut; laporan per skenario di PR.

### 12.11 Benchmark contention dan keputusan key/partisi

- `BenchmarkCompleteJobContention50`, `BenchmarkCreateInstances500Parallel`, histogram
  `engine_command_lock_wait_seconds`; bandingkan dengan Fase 2.
- Tulis ADR-0017 (key sequence global; hasil benchmark; pemicu peninjauan partisi).

Hasil: angka p50/p99 dicatat; ADR-0017 final.

### 12.12 Dokumentasi dan penutupan

- Finalkan ADR-0014 sampai ADR-0018; README modul `execution` (verify/rebuild/snapshot/retensi);
  README `platform/outbox` (kontrak consumer, retensi, dead letter); `docs/plan/README.md` menautkan
  dokumen ini; review DoD.

Hasil: semua kotak DoD tercentang dengan bukti.

## 13. Target Nx dan Makefile

| Proyek | Target | Perintah |
|---|---|---|
| `engine` | `test` | sudah ada; mencakup `platform/{outbox,recovery,idempotency,paging,metrics,faultpoint}` |
| `engine` | `test-integration` (cache off) | `go test -count=1 -tags=integration ./apps/engine/...` (sudah ada lewat Makefile; ditambah ke Nx agar `affected` mengenalinya, opsional) |
| `engine` | `test-faultinject` (cache off) | `go test -count=1 -tags=integration,faultinject ./apps/engine/...` |
| `engine` | `bench` | diperluas: contention dan rebuild |
| `engine` | `sqlc` | diperluas: `platform/outbox`, `platform/idempotency`, `execution/infrastructure/queries/history.sql` |
| `proto` | `generate`, `lint`, `breaking` | mencakup `request_id`, RPC riwayat, `common/v1/errors.proto` |
| `chaos` (proyek baru di `tools/scripts/chaos`, tag `type:tool`) | `run` (cache off) | `go run ./tools/scripts/chaos -scenario $SCENARIO -duration $DURATION` |

Target Makefile baru/diperluas:

```
make chaos [SCENARIO=all] [DURATION=60s]     # build image, compose chaos up, jalankan runner, down
make db-backup / db-backup-list / db-restore FILE=... [TARGET_DB=...]
make verify [ARGS=--all]                     # go run ./apps/engine/cmd/engine verify $ARGS
make rebuild ARGS="--instance 123"
make test-faultinject
make migrate-redo                            # down + up migrasi terakhir (uji migrasi)
```

`.github/workflows/ci.yml`: job `integration` menambah `test-faultinject` dan `migrate-redo`;
workflow baru `chaos.yml` terjadwal nightly dan manual (`workflow_dispatch`).

## 14. Risiko dan mitigasi

| Risiko | Dampak | Mitigasi |
|---|---|---|
| Transaksi lama menahan `xmin` sehingga relay tertunda | Latensi event naik untuk semua consumer | `idle_in_transaction_session_timeout` 15 s dan `statement_timeout` 10 s membatasi; metrik `engine_outbox_lag_seconds`; admin `rebuild --all` memakai transaksi per instance, bukan satu transaksi panjang |
| Urutan xid ≠ urutan lock karena statement sebelum lock mengalokasikan xid | Event satu instance terkirim tidak urut | Aturan "klaim idempotensi setelah lock" dan tidak ada tulis sebelum `LockInstance`; test monotonisitas 20 ulangan; consumer Fase 5+ tetap memeriksa `aggregate_seq` bila peka urutan |
| `uuidv7()` tidak tersedia di image | Migrasi gagal | Verifikasi di 12.2; cadangan `gen_random_uuid()` (urutan tetap dari `(tx_id, id)`) |
| Rebuild mengungkap record Fase 2 yang kurang data proyeksi | Rebuild tidak identik | Test truncate → rebuild untuk setiap skenario sejak 12.4; kekurangan diperbaiki di projector/record Fase 2 sebelum data produksi ada |
| `proto` deterministic marshal berubah antar versi protobuf-go | Retry sah ditolak `IDEMPOTENCY_KEY_REUSED` setelah upgrade | TTL 24 jam; upgrade protobuf-go dicatat sebagai perubahan yang mengosongkan `idempotency_key`; alternatif hash kanonik JSON dicatat di ADR-0016 |
| Janitor retensi bersaing dengan command | Lock wait, latensi | `SKIP LOCKED`, batch kecil, jeda antar batch, hanya instance final; test 14 |
| Verifikasi start lambat pada database besar | Startup lama, `readyz` 503 lama | Tingkat cepat berbasis agregat berindeks; tingkat dalam dibatasi jendela; `ENGINE_RECOVERY_TIMEOUT`; `/healthz` tetap hidup selama pemulihan |
| Mode `fail` menahan layanan karena bug kecil proyeksi | Downtime | Pesan error menyebut perintah pemulihan persis; mode `rebuild` tersedia; keputusan default ditinjau di 15.2 #3 |
| Chaos suite flaky di CI | Sinyal palsu | Hanya nightly; setiap skenario menunggu `readyz` dengan timeout jelas; log container dilampirkan saat gagal; tiga kali hijau lokal sebelum diaktifkan |
| `docker compose` di CI tidak mendukung `kill -s KILL` atau network disconnect | Skenario tidak bisa jalan | Runner memakai Docker CLI langsung (`docker kill`, `docker network disconnect`) yang tersedia di runner GitHub |
| `goose.WithSessionLocker` berbeda dari asumsi | Migrasi ganda saat start bersamaan | Verifikasi di 12.8; cadangan `pg_advisory_lock` manual sebelum `goose.Up` |
| Snapshot codec menyimpang dari `InstanceState` saat Fase 5 menambah field | Rebuild dari snapshot salah | `format_version`; test "snapshot versi lama → mulai dari nol"; Fase 5 wajib menaikkan versi |
| Phase 2 belum merge saat Fase 3 mulai | Blokir 12.3–12.5 | 12.2, 12.6, 12.8, 12.9 bisa dikerjakan terhadap `deployment` dan platform saja |

## 15. Pertanyaan terbuka

Setiap pertanyaan diberi rekomendasi. Keputusan final dicatat di 15.1 setelah review.

### 15.1 Keputusan

Dicatat 2026-10-28. User meminta Fase 3 dimulai ("lanjut fase 3"); seperti fase sebelumnya, setiap
pertanyaan memakai rekomendasi dokumen ini, disesuaikan dengan keputusan yang diambil saat Fase 2:

| # | Keputusan |
|---|---|
| 1 | Tabel `outbox_event` generik milik platform |
| 2 | Pembacaan bebas celah dengan `xid8` + `pg_snapshot_xmin` |
| 3 | `ENGINE_RECOVERY_ON_MISMATCH` default `fail` |
| 4 | Verifikasi dalam saat start: jendela + sampel; `--all` lewat CLI |
| 5 | `request_id` di body proto |
| 6 | Key sama dengan payload berbeda → `InvalidArgument` + `IDEMPOTENCY_KEY_REUSED` |
| 7 | TTL idempotensi 24 jam |
| 8 | Tanpa kompaksi record di Fase 3 |
| 9 | Retensi menghapus instance final setelah N hari, default nonaktif |
| 10 | **Digantikan ADR-0011**: key sudah UUIDv7 sejak Fase 2; tidak ada bit partisi |
| 11 | Row lock per instance tetap; affinity hanya bila benchmark menunjukkan lock wait p99 > 50 ms |
| 12 | Handler consumer menerima `pgx.Tx` di `interfaces`/`infrastructure` modul |
| 13 | `GetProcessInstance` dipotong di 1.000 element instance + flag + RPC list |
| 14 | PITR tidak di Fase 3; hanya `wal_level=replica` dan catatan |
| 15 | Metrik lewat API OpenTelemetry dengan provider no-op |
| 16 | Tanpa sinyal in-process lintas replika; `NOTIFY outbox_wakeup` hanya untuk relay |

Penyesuaian terhadap kondisi setelah Fase 2 (berlaku untuk seluruh dokumen ini):

- **Tanpa auto-increment** (preferensi user): `outbox_event` memakai `event_id UUID` dari generator
  engine sebagai primary key; kursor relay `(tx_id, event_id)`. Urutan dalam satu transaksi tetap
  terjaga karena generator UUIDv7 monotonik di dalam satu proses. Semua kolom key bertipe `UUID`.
- **Bagian 8 (key dan partisi) sudah diputuskan ADR-0011**: key UUIDv7; kesiapan partisi memakai
  hash `process_instance_key` di lapisan routing.
- **`execution_record` tidak punya `position`**: riwayat berhalaman memakai `sequence`; field
  `position` di `ExecutionRecord` proto dihilangkan.
- **Penomoran digeser**: ADR Fase 3 menjadi ADR-0015 (dua tingkat pengiriman event), ADR-0016
  (record sebagai sumber kebenaran, verify dan rebuild), ADR-0017 (idempotensi), ADR-0018 (kesiapan
  partisi dan contention), ADR-0019 (snapshot dan retensi). Migrasi menjadi `00005_outbox.sql`,
  `00006_idempotency.sql`, `00007_history.sql`.
- **Port Fase 2 yang sebenarnya** berbeda dari sketsa 3.3: `domain.Repository` punya `WithTx`,
  `JobRef`, `DefinitionKeyOf`; `TxRepository` punya `LockAndLoad` dan `AppendAndProject`. Unit of work
  application dibangun di atas bentuk itu.

### 15.2 Pertanyaan dan rekomendasi

| # | Pertanyaan | Rekomendasi | Alasan |
|---|---|---|---|
| 1 | Outbox generik milik platform atau relay membaca `execution_record` langsung? | **Tabel `outbox_event` generik** | `deployment` dan modul Fase 4–5 butuh event durable tanpa record log; record tetap sumber kebenaran state, bukan antrean |
| 2 | Pembacaan bebas celah: `xid8` snapshot, jendela waktu, atau serialisasi tulis? | **`xid8` + `pg_snapshot_xmin`** | Tanpa lock global dan tanpa latensi tetap; dibatasi timeout sesi |
| 3 | Default `ENGINE_RECOVERY_ON_MISMATCH` | **`fail`** di semua lingkungan; `.env.prod.example` memberi komentar cara beralih ke `rebuild` | Divergensi = bug; bukti lebih penting daripada menyembunyikannya; operator punya `rebuild` satu perintah |
| 4 | Verifikasi dalam saat start: jendela 10 menit + sampel, atau semua instance aktif? | **Jendela + sampel**; `--all` lewat CLI | Semua instance aktif bisa jutaan; crash hanya menyentuh instance yang baru berubah |
| 5 | `request_id` di body proto atau header `Idempotency-Key`? | **Body** | Masuk kontrak proto/SDK; identik di gRPC, Connect, dan REST; header dipertimbangkan Fase 4 untuk SDK bila perlu |
| 6 | Key sama payload beda: error atau replay? | **`InvalidArgument` + `IDEMPOTENCY_KEY_REUSED`** | Hampir pasti bug klien |
| 7 | TTL idempotensi | **24 jam** | Menutup retry klien dan restart; tabel tetap kecil |
| 8 | Kompaksi record berdasarkan snapshot | **Tidak di Fase 3** (format snapshot ada, pemangkasan mati) | Operate Fase 6 butuh riwayat; keputusan penyimpanan riwayat belum ada |
| 9 | Retensi: hapus atau tabel arsip? | **Hapus** setelah N hari, default nonaktif; arsip = `db-backup` | Tabel arsip tanpa pembaca menggandakan skema; verifikasi perilaku TTL riwayat Camunda 8.8 untuk default yang lazim |
| 10 | Bit partisi di key sekarang? | **Tidak** (ADR-0017) | Tidak ada manfaat tanpa log per partisi; sharding kepemilikan memakai hash key |
| 11 | Row lock tetap atau "instance affinity" antar replika? | **Row lock tetap**; affinity hanya bila benchmark 12.11 menunjukkan lock wait p99 > 50 ms pada beban target | Data dulu; affinity menambah routing dan failover |
| 12 | Handler consumer menerima `pgx.Tx` (platform) — melanggar kemurnian lapisan? | **Ya, di `interfaces`/`infrastructure` modul** (consumer adalah adapter masuk); `application` menerima port transaksional lewat `UnitOfWork` | Konsisten dengan aturan 4.1 Fase 0: `interfaces` boleh bergantung pada platform |
| 13 | `GetProcessInstance` dipotong di 1.000 element instance atau selalu berhalaman? | **Potong + flag + RPC list** | Tidak mengubah kontrak Fase 2; UI Fase 2 tetap jalan |
| 14 | PITR di Fase 3? | **Tidak**; hanya `wal_level=replica` dan catatan | Butuh komponen dan penyimpanan tambahan; masuk hardening Fase 8 |
| 15 | Metrik Prometheus langsung (`promhttp`) atau API OTel no-op? | **API OTel** | Fase 0 memutuskan OpenTelemetry; exporter dipasang Fase 8 tanpa mengganti instrumentasi |
| 16 | Sinyal in-process lintas replika (`LISTEN/NOTIFY`) di Fase 3? | **Tidak**; `NOTIFY outbox_wakeup` hanya untuk relay | Belum ada subscriber ephemeral lintas replika; Fase 4 memutuskan untuk long polling |

## 16. Estimasi dan pembagian kerja

Perkiraan **13–16 hari kerja** untuk 1 engineer, atau 8–9 hari untuk 2 engineer paralel:

| Hari | Fokus |
|---|---|
| 1 | 12.1 prasyarat, verifikasi `uuidv7`/`XID8`/goose locker |
| 2–4 | 12.2 platform `outbox` + test bebas celah, dead letter, dua relay |
| 5 | 12.3 `UnitOfWork`, publisher ke outbox, test monotonisitas |
| 6–7 | 12.4 verify + rebuild + recovery runner + readiness |
| 8 | 12.5 snapshot |
| 9–10 | 12.6 idempotensi (paralel dengan 12.2 bila 2 orang) |
| 11 | 12.7 riwayat berpaginasi + retensi |
| 12 | 12.8 pool/timeout/migrasi/backup + runbook diuji |
| 13 | 12.9 observability, 12.11 benchmark + ADR-0017 |
| 14–15 | 12.10 fault point + chaos suite + CI nightly |
| 16 | 12.12 ADR, README, review DoD |

Pembagian jika 2 orang: **A** = outbox, `UnitOfWork`, recovery/verify/rebuild/snapshot, chaos
suite; **B** = idempotensi, proto + riwayat berpaginasi, retensi, pool/timeout/migrasi/backup,
observability. Titik temu: 12.3 (A memakai `UnitOfWork` yang juga dipakai B untuk idempotensi) dan
12.10 (skenario chaos memakai `request_id` B).

## 17. Keluaran Fase 3

Setelah fase ini selesai, Fase 4 dan seterusnya mendapat:

- **Outbox durable** (`outbox.Writer`, `Relay.Register(Consumer)`): Fase 4 menulis `job_activated`,
  `job_failed` dan membangun long polling dari sinyal in-process + `job_created`; Fase 5 mendaftar
  consumer `timer.scheduler` dan `message.correlation` atas `deployment.process_deployed` dan
  `execution.*`; Fase 6 mendaftar `operate.projection`.
- **`recovery.Participant`** dan `/readyz`: Fase 4 membangun ulang statistik antrean job, Fase 5
  memuat timer jatuh tempo, keduanya sebelum engine menerima request.
- **Jaminan durabilitas yang terdokumentasi** (3.1) dan **suite chaos** yang bisa diperluas per fase
  (Fase 4: worker mati di tengah `ActivateJobs`; Fase 5: timer terlewat saat engine mati).
- **Idempotensi `request_id`** sebagai pola untuk RPC mutasi baru (`ActivateJobs` tidak idempoten by
  design — diputuskan Fase 4; `FailJob`, `ThrowError`, `PublishMessage` memakai pola ini; SDK worker
  Go mengisi `request_id` otomatis saat retry).
- **`engine verify`/`rebuild`/`prune`/`outbox`** sebagai alat operator; RPC riwayat berpaginasi dan
  `ExecutionRecord` sebagai sumber data Operate (Fase 6) tanpa akses DB langsung.
- **Snapshot `format_version`**: Fase 5 menaikkan versi saat menambah timer/message ke
  `InstanceState`.
- **Konfigurasi pool/timeout dan runbook backup/migrasi**: Fase 8 menambah exporter OTel pada metrik
  yang sudah terdaftar, backup terjadwal/offsite, PITR, dan zero-downtime restart di atas gerbang
  readiness yang sudah ada.
- **ADR-0014 sampai ADR-0018** sebagai acuan keputusan persistensi yang tidak dibuka lagi tanpa ADR
  baru.
