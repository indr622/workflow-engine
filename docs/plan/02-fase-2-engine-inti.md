# Fase 2 — Engine Inti (Eksekusi Token)

**Proyek:** Workflow Engine (kompatibel konsep Camunda 8 / BPMN 2.0)
**Stack:** Go (engine utama) + React (UI) dalam monorepo Nx
**Status:** ✅ Selesai pada 2026-10-28, di-merge ke `main` (fast-forward dari `feat/phase-2-execution`, tanpa PR atas permintaan user); satu target latensi DoD belum tercapai di mesin dev, lihat bagian 0
**Tanggal:** 2026-10-28

---

## 0. Status eksekusi

Dieksekusi 2026-10-28 atas permintaan user ("lanjut fase 2"). Keputusan atas pertanyaan terbuka
dicatat di 11.1, termasuk keputusan user di tengah jalan untuk memakai UUID sebagai id.

| Langkah | Status | Bukti / catatan |
|---|---|---|
| 8.1 Prasyarat | ✅ | Fase 1 di-merge ke `main`; branch `feat/phase-2-execution` |
| 8.2 Allowlist kernel `archtest` | ✅ | `kernelAllowlist`, kasus baru di `TestCheckImportRules`, `TestKernelsArePure`; depguard deny `libs/go/sdk`; ADR-0010 |
| 8.3 `libs/go/feel` subset | ✅ | Proyek Nx `feel-go`; 206 kasus tabel (145 evaluasi, 61 error kompilasi); `FuzzCompile` 10 menit bersih |
| 8.4–8.6 Domain | ✅ | `Processor`, `Apply`, perilaku semua elemen matriks 4.1, inclusive join reachability; skenario domain dengan replay di setiap skenario |
| 8.7 Property test gateway | ✅ | `TestGatewayProperties` 1.000 graf per `go test` (~3 detik); 20.000 graf sekali jalan lolos |
| 8.8 Application | ✅ | `Commands` dan `Queries`; diuji lewat integrasi Postgres (lihat penyimpangan) |
| 8.9 Proto | ✅ | `workflow/process/v1`, `workflow/job/v1`; `buf lint` bersih; stub Go dan TS di-commit |
| 8.10 Migrasi, repository, proyektor | ✅ | `00004_execution.sql`; kunci + muat satu batch, semua tulisan satu batch |
| 8.11 Interfaces dan wiring | ✅ | Handler Connect, validasi key UUID, pemetaan error; dirakit di `main.go`; dicek dengan curl ke engine yang berjalan |
| 8.12 Skenario penuh | ✅ | Semua baris matriks 4.1 dan semua tipe incident 4.4 tercakup; daftar di README modul |
| 8.13 Paritas eksekusi | ✅ | 20 skenario, hasil observable sama dengan Camunda 8.8.44 tanpa deviasi; rekaman `domain/testdata/parity/expectations.json` |
| 8.14 Modeler-web `process-instance` | ✅ | Diuji manual di browser: validasi variabel, start dari halaman definition, marker aktif/selesai/incident, badge jumlah token, complete job, polling berhenti saat selesai |
| 8.15 Benchmark dan dokumentasi | ⚠️ | `Processor` di atas target; latensi DB di atas target di Windows (lihat di bawah); ADR-0011 sampai ADR-0014; README modul |
| 6.2 Perubahan `deployment` | ✅ | `GetByVersion`, cache metadata definition, kompilasi ekspresi saat deploy, `IO_MAPPING_NOT_SUPPORTED`, warning `INCLUSIVE_JOIN_IN_CYCLE` |

**Hasil benchmark** (i5-1145G7, Windows 11, Postgres 18 di Docker Desktop):

| Ukuran | Hasil | Target |
|---|---|---|
| `Processor` linear 20 service task | ~160.000–200.000 element instance/detik | > 50.000 |
| `Processor` fork 50 cabang | ~95.000 element instance/detik | > 50.000 |
| `Processor` inclusive join | ~120.000 element instance/detik | > 50.000 |
| Create + 20 `CompleteJob`, p50 | 190–235 ms | < 50 ms |
| Round trip `SELECT 1` ke Postgres | ~1,2 ms | — |

Target latensi 50 ms mengandaikan Postgres native dengan round trip sekitar 0,1 ms. Satu command
kini butuh sekitar lima round trip (turun dari sekitar 22), jadi di mesin ini angkanya dibatasi
round trip Docker Desktop dan fsync saat commit. `TestLinear20Latency` mencetak p50 di job
integrasi CI yang berjalan di Linux; target dikunci setelah angka itu terlihat.

**Penyimpangan dari rencana:**

- **Key UUIDv7, bukan `int64` dari sequence** (keputusan user). Semua key termasuk deployment Fase 1;
  migrasi `00003_uuid_keys.sql` mengonversi data lama secara deterministik dan satu arah. Mekanisme
  blok key dan `ErrKeysExhausted` dihapus. Proto deployment berubah breaking (`int64` menjadi
  `string`). ADR-0011.
- **ADR bergeser satu nomor**: model eksekusi ADR-0012, join ADR-0013, subset FEEL ADR-0014.
- **`execution_record` tanpa kolom `position` identity**: urutan per instance memakai `sequence`
  dengan primary key `(process_instance_key, sequence)`. Urutan global untuk exporter diputuskan Fase 3.
- **`FeelEvaluator` ada di domain**, bukan infrastructure, karena ADR-0010 mengizinkan domain
  memakai kernel `feel`. Port `ExpressionEvaluator` tetap ada.
- **Port repository berubah**: `LockAndLoad` (kunci + muat dalam satu batch), `JobRef` dan
  `DefinitionKeyOf` di luar transaksi agar model proses dimuat sebelum row lock. Statement proyeksi
  ditulis tangan dalam satu `pgx.Batch`; sqlc hanya untuk query baca.
- **Dua bug ditemukan test integrasi dan diperbaiki**: pool habis saat model proses dimuat sambil
  memegang row lock, dan state basi saat kunci dan muat digabung dalam satu statement.
- **Application diuji lewat integrasi Postgres**, bukan repository fake.
- **`CreateProcessInstanceResponse` menambah field `state`.**
- **Paritas membandingkan selesai/tidak, multiset tipe job, job terbuka, dan variabel akhir**; jalur
  elemen tidak dibandingkan karena profil Camunda tanpa secondary storage. Paritas mengonfirmasi
  keputusan 11.1 #13: kondisi bernilai null menjadi incident di Camunda, bahkan dengan default flow.
  Camunda mengembalikan angka dalam bentuk kanonik (`1.50` menjadi `1.5`); pembanding memakai
  desimal eksak.
- **Batas record per command** membuang token lain yang masih di antrean command itu.
- **Target `fuzz` dan `prop` di Makefile memanggil `go test` langsung**, karena sintaks
  `${VAR:-default}` di target Nx tidak diekspansi di Windows.
- **Commit**: perubahan `GetByVersion` di deployment ikut masuk commit modul eksekusi karena
  commit terpisahnya ditolak commitlint; commit itu sudah di-push sehingga tidak di-amend.

**Kondisi repo yang menjadi titik awal (dicek 2026-10-28, Fase 1 sedang dieksekusi):**

| Area | Kondisi | Dampak ke Fase 2 |
|---|---|---|
| `apps/engine` | Platform (`config`, `logger`, `db` + goose, `httpserver`, `eventbus`) dan modul `health`; modul `deployment` sedang dibangun di Fase 1 | Modul `execution` mengikuti pola `health`/`deployment`; dirakit di `cmd/engine/main.go` |
| `apps/engine/internal/archtest` | `domain` hanya boleh stdlib; antarmodul hanya lewat `api` | Diubah di awal Fase 2: allowlist kernel murni untuk `domain` (lihat 3.1, ADR-0010) |
| `.golangci.yml` | Aturan depguard `domain-purity` (deny pgx, connect, `net/http`, `database/sql`, platform, `libs/go/api`) | Tidak perlu diubah untuk allowlist (kernel tidak masuk daftar deny); ditambah deny `libs/go/sdk` agar SDK tidak bocor ke domain |
| `libs/go/bpmn` | Sedang dibangun Fase 1. Model yang diimplementasikan memakai **tipe kategori**: `*bpmn.Event`, `*bpmn.Activity`, `*bpmn.Gateway`, `*bpmn.SequenceFlow`, interface `FlowNode`; `Process.Element(id)`, `StartEvents()`, `NoneStartEvent()`, root `*Scope` | Fase 2 memakai model ini apa adanya (berbeda sedikit dari sketsa 4.3 dokumen Fase 1 yang memakai satu tipe per elemen) |
| `bpmn.ProfileExecutable` | Mengizinkan none start/end, sequence flow, service task, undefined/manual task, exclusive/parallel/inclusive gateway | Tepat elemen yang dieksekusi Fase 2; Fase 2 hanya **mengetatkan** profil (I/O mapping, ekspresi di luar subset, lihat 3.4) |
| `libs/go/bpmn/testdata/valid` | Korpus Fase 1 (≥ 30 file hasil Camunda Modeler) | Dipakai ulang sebagai fixture skenario eksekusi; file baru untuk pola join ditambahkan |
| Modul `deployment` (Fase 1) | `api.ProcessDefinitions` dengan `GetByKey`/`GetLatest` mengembalikan metadata + `*bpmn.Process` dari cache LRU; key `int64` dari sequence Postgres lewat `internal/platform/keygen`; event `deployment.process_deployed`; proto `workflow/deployment/v1` | `execution` hanya meng-import `deployment/api`; Fase 2 menambah `GetByVersion` (lihat 6.2) |
| Migrasi | `00001_init.sql`; Fase 1 menambah `00002_deployment.sql` (+ `engine_key_seq`) | Migrasi Fase 2 menjadi `00003_execution.sql` (nomor final mengikuti urutan saat merge) |
| `libs/go/feel` | Folder penanda (README "Dibuat di Fase 7") | Fase 2 melahirkan paket `feel` berisi **subset** kondisi (lihat 3.4); Fase 7 memperluasnya tanpa mengubah API |
| `libs/proto` | `workflow/health/v1`; Fase 1 menambah `workflow/deployment/v1` | Proto baru `workflow/process/v1` dan `workflow/job/v1` |
| Compose | `infra/docker/docker-compose.yml`; profil `camunda` direncanakan Fase 1 (langkah 6.2) | Uji paritas eksekusi memakai profil yang sama (`camunda/zeebe:8.8.44`) |
| `apps/modeler-web` | Fitur `health`; fitur `deployment` dibangun Fase 1 | Fitur `process-instance` minimal (lihat 6.14) |
| `apps/operate-web` | Folder penanda, dibuat Fase 6 | Tidak disentuh |

## 1. Lingkup

Fase 2 membuat engine **benar-benar menjalankan proses**: dari process definition yang sudah
di-deploy (Fase 1), engine membuat process instance, menggerakkan token melalui sequence flow dan
gateway, berhenti di service task sebagai job, lalu melanjutkan setelah job diselesaikan sampai
instance selesai. Setiap perubahan state dicatat sebagai **record append-only** dan diproyeksikan ke
tabel state di PostgreSQL dalam satu transaksi, sehingga Fase 3 (persistensi dan pemulihan) tinggal
menambah replay, idempotensi, dan outbox tanpa membongkar model eksekusi.

Termasuk dalam Fase 2:

1. **Modul engine `execution`** — lima lapisan: process instance, element instance dengan siklus
   hidup ala Zeebe (`ACTIVATING` → `ACTIVATED` → `COMPLETING` → `COMPLETED`, `TERMINATING` →
   `TERMINATED`), pengambilan sequence flow, variabel dokumen JSON (scope process instance), job
   untuk service task, incident minimal, pembatalan instance.
2. **Perilaku elemen**: none start event, none end event (termasuk implicit end), undefined task dan
   manual task (pass-through), service task (membuat job, menunggu), exclusive gateway (dengan
   default flow), parallel gateway (fork dan join), inclusive gateway (fork dengan kondisi dan join).
3. **`libs/go/feel` subset kondisi** — evaluator ekspresi terbatas (literal, path variabel,
   perbandingan, `and`/`or`/`not`) di balik interface yang dipertahankan Fase 7.
4. **Kontrak API** `workflow/process/v1` (`ProcessInstanceService`: create, get, list, cancel) dan
   `workflow/job/v1` (`JobService`: `ListJobs`, `CompleteJob` minimal; Fase 4 menambah
   `ActivateJobs`, `FailJob`, `ThrowError` tanpa breaking change).
5. **Perubahan aturan arsitektur** — `archtest` mengizinkan `domain` meng-import kernel murni
   (`libs/go/bpmn`, `libs/go/feel`), dicatat sebagai ADR-0010.
6. **Modeler-web fitur `process-instance` minimal** — mulai instance dari daftar definition, lihat
   state instance di viewer bpmn-js dengan marker token, selesaikan job secara manual.
7. **Strategi uji** — skenario tabel per pola BPMN, property test join gateway, test determinisme
   record, integration test Postgres 18, uji paritas observable terhadap Camunda 8.8.

**Tidak termasuk** (fase lain): replay state dari record setelah restart, idempotensi command,
outbox transaksional, partisi (Fase 3); aktivasi job oleh worker, `FailJob`/retry/backoff,
`ThrowError`, resolusi incident, `SetVariables`, SDK worker (Fase 4); timer, message, boundary
event, sub-process, call activity, multi-instance, error/escalation (Fase 5); Operate penuh
(Fase 6); FEEL penuh, input/output mapping, script task, DMN (Fase 7); multi-tenant dan otorisasi
(Fase 8); modifikasi dan migrasi instance, `startInstructions`, create instance "with result".

## 2. Definition of Done Fase 2

- [x] `archtest` mengizinkan `domain` meng-import tepat `libs/go/bpmn` dan `libs/go/feel` (allowlist), dibuktikan dengan kasus di `TestCheckImportRules`; ADR-0010 tersimpan.
- [x] Modul `execution` lima lapisan lolos `archtest`; `execution/domain` tidak meng-import apa pun selain stdlib dan kedua kernel.
- [x] `libs/go/feel` adalah proyek Nx (`feel-go`) dengan target `build`, `test`, `lint`, `fuzz`; hanya stdlib; seluruh tabel uji subset (≥ 80 kasus, termasuk semantik `null`) lolos; `FuzzCompile` 10 menit bersih.
- [x] `CreateProcessInstance` (dengan `process_definition_key`, atau `bpmn_process_id` + versi/terbaru, plus variabel JSON) mengembalikan `process_instance_key`; instance tanpa wait state langsung `COMPLETED` dalam satu panggilan.
- [x] Setiap pola di matriks 4.1 punya minimal satu skenario tabel yang lolos: urutan record, element instance akhir, variabel akhir, job, dan incident sesuai ekspektasi.
- [x] Parallel join: property test atas ≥ 1.000 graf blok acak (fork/join bersarang, urutan penyelesaian job acak) selalu berakhir `COMPLETED`, setiap join aktif tepat sekali per fork, tidak ada sisa counter flow.
- [x] Inclusive join: property test yang sama untuk graf blok inclusive dengan kondisi acak; skenario batas (cabang tidak diambil, cabang menunggu job, default flow) lolos; keterbatasan algoritma (siklus) tercatat di ADR-0013 dan diuji sebagai perilaku yang terdokumentasi.
- [x] Determinisme: menerapkan ulang (`Apply`) seluruh record sebuah instance ke state kosong menghasilkan state identik dengan proyeksi di database (test untuk semua skenario; fondasi replay Fase 3).
- [x] Service task membuat job (`CREATED`) dengan `type`, `retries`, header; `CompleteJob` dengan variabel menggabungkan variabel ke scope process instance dan melanjutkan token; job yang sudah selesai atau dibatalkan ditolak `FailedPrecondition`/`NotFound`.
- [x] Kondisi gagal dievaluasi atau tidak ada flow yang cocok di exclusive/inclusive gateway tanpa default membuat incident dengan tipe dan pesan yang jelas; instance tetap `ACTIVE` dengan `has_incident = true`.
- [x] `CancelProcessInstance` menghentikan semua element instance aktif (anak dahulu), membatalkan job terbuka, dan menandai instance `TERMINATED`.
- [x] Single writer per instance: integration test 50 `CompleteJob` paralel pada satu instance (fork 50 cabang) menghasilkan join tepat sekali dan instance `COMPLETED`, tanpa deadlock dan tanpa record ganda.
- [x] Integration test (`-tags=integration`) terhadap Postgres 18 untuk repository, proyeksi, dan alur end-to-end lewat handler Connect (`httptest`).
- [x] Uji paritas eksekusi: semua skenario bertanda `parity` memberi hasil observable yang sama dengan Camunda 8.8 referensi (selesai/incident, multiset tipe job yang diaktifkan, variabel akhir); deviasi disengaja tercatat di `expectations.json` dengan alasan.
- [x] Modeler-web: dari daftar definition, mulai instance dengan variabel JSON, lihat token aktif/selesai/incident sebagai marker di viewer bpmn-js, selesaikan job dari UI; uji manual tercatat di PR. Uji manual tercatat di bagian 0 (belum ada PR).
- [ ] Benchmark: proses linear 20 service task, create + 20 `CompleteJob` < 50 ms p50 total di mesin dev dengan Postgres lokal (angka awal, dikunci setelah pengukuran pertama); `Processor` murni (tanpa DB) > 50.000 element instance/detik. **Sebagian:** `Processor` di atas target; latensi DB 190–235 ms di Docker Desktop Windows (lihat bagian 0).
- [x] ADR-0010 sampai ADR-0014 tersimpan di `docs/adr/`; `docs/plan/README.md` menautkan dokumen ini.

## 3. Keputusan teknis

| Area | Keputusan | Alasan singkat |
|---|---|---|
| Lokasi logika eksekusi | `execution/domain` berisi state machine murni (`Processor`) yang bekerja langsung di atas `*bpmn.Process` | Perilaku elemen adalah aturan bisnis inti; menaruhnya di `application` membuat domain kosong. Membuat graf tiruan di domain hanya menduplikasi model Fase 1 |
| Kernel di domain | `archtest` diberi **allowlist kernel murni**: `libs/go/bpmn`, `libs/go/feel` boleh diimpor `domain` (ADR-0010) | Keduanya hanya stdlib (ditegakkan `deps_test.go` masing-masing), immutable, tanpa I/O; secara arsitektur setara stdlib domain |
| Model eksekusi | **Aggregate event-sourced**: command → `Processor` menghasilkan record (event) → state hanya berubah lewat `Apply(record)`; satu command diproses *run-to-completion* sampai semua token mencapai wait state atau selesai | Pola yang sama dengan Zeebe (command/event record); replay Fase 3 = `Apply` ulang record tanpa logika baru |
| Persistensi Fase 2 | Dalam **satu transaksi Postgres** per command: append record ke `execution_record` + proyeksi ke tabel state (`process_instance`, `element_instance`, `variable`, `job`, `incident`, `taken_sequence_flow`) | Tidak ada fase "in-memory saja" yang harus dibuang; Fase 3 menjadikan record sebagai sumber kebenaran (replay, verifikasi) dan menambah outbox serta idempotensi tanpa mengubah skema inti |
| Konkurensi | **Single writer per process instance lewat row lock**: `SELECT ... FROM process_instance WHERE key = $1 FOR UPDATE` di awal transaksi; state runtime instance dimuat ulang setiap command | Benar juga saat engine lebih dari satu replika; tanpa infrastruktur actor/partisi; contention hanya di dalam satu instance. Partisi ala Zeebe ditinjau di Fase 3 tanpa mengubah domain (ADR-0012) |
| Key | **UUIDv7** dari `internal/platform/keygen` yang dibuat di proses engine, tanpa sequence auto-increment (ADR-0011, diputuskan user 2026-10-28); key dicatat di record | Tanpa round trip ke database dan tanpa alokasi blok; tetap terurut waktu untuk indeks dan keyset. Replay memakai key dari record, bukan generator |
| Key process instance | `process_instance_key` = key element instance bertipe `PROCESS` | Sama dengan konvensi Zeebe; satu ruang key untuk instance dan elemen |
| Siklus hidup elemen | State `ACTIVATING`, `ACTIVATED`, `COMPLETING`, `COMPLETED`, `TERMINATING`, `TERMINATED`; intent record mengikuti nama Zeebe (`ELEMENT_ACTIVATING`, …, `SEQUENCE_FLOW_TAKEN`) | Memudahkan paritas dan pembacaan dokumentasi Camunda; Operate (Fase 6) memetakan langsung |
| Join parallel gateway | Counter per `(flow_scope_key, gateway_id, sequence_flow_id)`; join aktif saat **setiap** incoming flow punya counter ≥ 1, lalu tiap counter dikurangi 1 | Perilaku standar BPMN dan Zeebe; mendukung token berulang (loop) secara benar |
| Join inclusive gateway | **Reachability statis + cek token aktif saat quiescence** (detail 5.6) | Tanpa menyimpan "token mati"; benar untuk graf blok dan sebagian besar graf asiklik; keterbatasan di graf bersiklus didokumentasikan (ADR-0013) |
| Kondisi | **Subset FEEL** di `libs/go/feel` (`Compile`/`Evaluate`) di balik port `domain.ExpressionEvaluator`; ekspresi di luar subset **ditolak saat deploy** (lihat 3.4, ADR-0014) | Model dari Camunda Modeler hampir selalu memakai `=x > 10`; menolak semua kondisi membuat gateway tidak bisa diuji. Semantik subset dibuat identik dengan FEEL untuk konstruksi yang didukung, sehingga Fase 7 adalah perluasan, bukan penggantian perilaku |
| Variabel | Dokumen JSON; disimpan per `(scope_key, name)` dengan nilai `JSONB`; Fase 2 hanya **scope process instance** (scope lokal ada di skema, belum dipakai) | Skema siap untuk scope lokal Fase 5/7; semantik propagasi tanpa I/O mapping identik dengan Zeebe karena hanya satu scope |
| I/O mapping | **Ditolak saat deploy** di `ProfileExecutable` (`IO_MAPPING_NOT_SUPPORTED`) sampai Fase 7 | Mapping butuh FEEL penuh dan scope lokal; menerima lalu mengabaikannya diam-diam lebih berbahaya daripada menolak |
| Kepemilikan job | Job adalah **bagian state instance** di modul `execution` (tabel `job` ditulis dalam transaksi yang sama); `execution/api` mengekspos `Jobs` | Penyelesaian job dan kelanjutan token harus atomik; menaruh job di modul lain memaksa transaksi lintas modul. Modul `job` Fase 4 menjadi lapisan protokol worker (long polling, timeout) di atas `execution/api` |
| Jalur penyelesaian job Fase 2 | RPC `JobService.CompleteJob` dan `ListJobs` di proto `workflow/job/v1` | Uji end-to-end lewat API nyata, bukan test hook; Fase 4 hanya menambah RPC ke service yang sama |
| Incident | Record `INCIDENT CREATED` + tabel `incident` di modul `execution`; elemen yang gagal berhenti di state saat gagal; resolusi di Fase 4 | Pembuatan incident harus atomik dengan langkah yang gagal; resolusi butuh `SetVariables` yang belum ada |
| Event antarmodul | Publikasi **setelah commit** ke event bus: `execution.process_instance_completed`, `execution.process_instance_terminated`, `execution.job_created`, `execution.incident_created` | Fase 4 (long polling job) dan Fase 6 (Operate) berlangganan; outbox transaksional di Fase 3 |
| Batas eksekusi | Maks 10.000 record per command (`ENGINE_MAX_RECORDS_PER_COMMAND`); melebihi → incident `EXECUTION_LIMIT_EXCEEDED` di elemen yang sedang diaktifkan, record sebelumnya tetap di-commit | Loop tanpa wait state (task pass-through melingkar) tidak boleh menggantung engine |
| Waktu | Port `Clock`; timestamp dicatat di record | `Processor` deterministik, replay tidak bergantung jam |

### 3.1 Allowlist kernel di `archtest` (jawaban pertanyaan terbuka 9.1 #1 Fase 1)

Rekomendasi: **ya, izinkan**, terbatas pada daftar eksplisit.

```go
// kernelAllowlist: paket kernel murni (hanya stdlib, immutable, tanpa I/O) yang boleh diimpor domain.
// Setiap kernel wajib punya deps_test.go yang menegakkan "hanya stdlib".
var kernelAllowlist = []string{
	"github.com/indr622/workflow-engine/libs/go/bpmn",
	"github.com/indr622/workflow-engine/libs/go/feel",
}

func isKernel(importPath string) bool {
	for _, k := range kernelAllowlist {
		if importPath == k || strings.HasPrefix(importPath, k+"/") {
			return !strings.Contains(importPath, "/internal/") // paket internal kernel tetap terlarang
		}
	}
	return false
}

// di checkImport:
if from.layer == "domain" && !isStdlib(importPath) && !isKernel(importPath) && (!isModulePkg || to != from) {
	return "domain may only import the standard library and allowlisted kernels"
}
```

Kasus baru di `TestCheckImportRules`: `domain uses bpmn kernel` (ok), `domain uses feel kernel` (ok),
`domain uses sdk` (gagal), `domain uses bpmn internal` (gagal). Test tambahan `TestKernelsArePure`
menjalankan `go list -deps` untuk setiap kernel di allowlist dan gagal bila ada dependensi
non-stdlib, sehingga allowlist tidak bisa dipakai untuk menyelundupkan dependensi. depguard tidak
berubah selain menambah deny `libs/go/sdk`. Modul `deployment` boleh (tidak wajib) memindahkan
aturan yang bergantung pada BPMN ke domain-nya kelak.

### 3.2 Tata letak modul `execution`

```
apps/engine/internal/modules/execution/
├─ api/
│  ├─ api.go                 # interface ProcessInstances, Jobs; DTO; error publik
│  └─ events.go              # ProcessInstanceCompleted, ProcessInstanceTerminated, JobCreated, IncidentCreated
├─ domain/
│  ├─ instance.go            # aggregate InstanceState (runtime: element instance aktif, variabel, job, incident, counter flow)
│  ├─ element_instance.go    # ElementInstance, ElementState, transisi yang sah
│  ├─ record.go              # Record, ValueType, Intent, payload per ValueType
│  ├─ apply.go               # Apply(state, record): SATU-SATUNYA tempat state berubah
│  ├─ processor.go           # Processor.Process(cmd) → []Record; antrean kerja run-to-completion
│  ├─ command.go             # CreateInstance, CompleteJob, CancelInstance (command eksternal)
│  ├─ behavior_process.go    # perilaku per kategori elemen
│  ├─ behavior_event.go
│  ├─ behavior_task.go       # task, manual task, service task (job)
│  ├─ behavior_gateway.go    # exclusive, parallel, inclusive
│  ├─ inclusive_join.go      # analisis reachability + keputusan join
│  ├─ variables.go           # dokumen JSON, merge, batas ukuran
│  ├─ incident.go            # tipe incident
│  ├─ errors.go              # ErrInstanceNotFound, ErrJobNotFound, ErrJobNotActive, ErrInvalidVariables, ...
│  └─ ports.go               # Repository, KeyGenerator, ExpressionEvaluator, Clock
├─ application/
│  ├─ command_runner.go      # tx → lock/load → Processor → append + project → commit → publish
│  ├─ create_instance.go     # resolusi definition lewat deployment/api, validasi variabel
│  ├─ complete_job.go
│  ├─ cancel_instance.go
│  ├─ get_instance.go        # query (membaca proyeksi, termasuk elemen selesai)
│  ├─ list_instances.go
│  └─ list_jobs.go
├─ infrastructure/
│  ├─ sqlc.yaml
│  ├─ queries/execution.sql
│  ├─ db/                    # hasil sqlc (di-commit)
│  ├─ postgres_repository.go # load runtime state, lock, append record
│  ├─ projector.go           # record → upsert tabel state (dipakai ulang replay Fase 3)
│  ├─ keygen_block.go        # adapter KeyGenerator dengan alokasi blok
│  └─ feel_evaluator.go      # adapter ExpressionEvaluator di atas libs/go/feel
├─ interfaces/
│  ├─ process_instance_handler.go  # ProcessInstanceService (Connect)
│  └─ job_handler.go               # JobService (Connect)
└─ testdata/
   ├─ scenarios/             # skenario tabel (Go) + BPMN khusus eksekusi bila korpus Fase 1 kurang
   └─ parity/expectations.json
```

### 3.3 Alur satu command (`command_runner.go`)

1. Interfaces memvalidasi request (key > 0, variabel JSON object, ukuran) dan memanggil use case.
2. Application membuka transaksi. Untuk command pada instance yang sudah ada:
   `LockInstance(key)` (`FOR UPDATE`); tidak ada → `ErrInstanceNotFound`. Untuk `CompleteJob`,
   key instance dibaca dulu dari `job` tanpa lock, lalu instance dikunci, lalu state job dibaca ulang
   di bawah lock.
3. `LoadRuntimeState`: element instance non-final, counter `taken_sequence_flow`, variabel, job
   terbuka, incident terbuka. Model `*bpmn.Process` diambil dari `deployment/api.ProcessDefinitions`
   (cache).
4. `Processor.Process(cmd)` menghasilkan `[]Record`; setiap record langsung di-`Apply` ke state
   in-memory sehingga langkah berikutnya melihat state terbaru.
5. `AppendRecords` (batch `COPY`/multi-row insert) + `Project(records)` (upsert proyeksi) dalam
   transaksi yang sama; `process_instance.version` dinaikkan.
6. Commit, lalu publikasi event domain. Kegagalan handler dicatat di log (ditutup outbox Fase 3).
7. Error domain (`ErrJobNotActive`, `ErrInvalidVariables`, …) membatalkan transaksi tanpa record.

### 3.4 Subset kondisi `libs/go/feel` (ADR-0014)

Pilihan yang dipertimbangkan:

| Opsi | Kelebihan | Kekurangan |
|---|---|---|
| A. Tolak semua kondisi non-trivial | Nol risiko semantik | Exclusive/inclusive gateway praktis tidak bisa diuji dengan model nyata; paritas lemah |
| B. Evaluator ad-hoc di engine | Cepat | Semantik menyimpang dari FEEL, dibuang di Fase 7 |
| **C. Subset FEEL di `libs/go/feel`, semantik identik untuk konstruksi yang didukung** | Model Modeler umum langsung jalan; API dan perilaku dipertahankan Fase 7 | Perlu disiplin agar tidak ada konstruksi "hampir FEEL" |

**Dipilih C.** Grammar subset:

```
expr       = or
or         = and { "or" and }
and        = cmp { "and" cmp }
cmp        = unary [ ("=" | "!=" | "<" | "<=" | ">" | ">=") unary ]
unary      = "not" "(" expr ")" | primary
primary    = number | string | "true" | "false" | "null" | path | "(" expr ")"
path       = name { "." name }          // name: identifier FEEL tanpa spasi
```

Semantik (mengikuti spesifikasi FEEL, diverifikasi terhadap dokumentasi Camunda 8.8 dan uji paritas):

- Angka sebagai desimal eksak (`math/big.Rat`), bukan `float64`; nilai JSON dibaca dengan
  `json.Decoder.UseNumber`.
- Path ke variabel yang tidak ada atau field yang tidak ada bernilai `null`.
- Perbandingan dengan `null` atau tipe berbeda menghasilkan `null` (kecuali `=`/`!=` terhadap
  `null`, yang menghasilkan boolean); `and`/`or` memakai logika tiga nilai FEEL.
- Kondisi sequence flow yang hasil akhirnya bukan `true`/`false` (termasuk `null`) → incident
  `CONDITION_ERROR` di gateway. Verifikasi terhadap Camunda 8.8 apakah `null` diperlakukan sebagai
  `false` atau incident; skenario paritas `exclusive-null-condition` menjadi bukti.
- Ekspresi tanpa prefix `=` di `conditionExpression`: ditolak saat deploy sampai perilaku Zeebe
  diverifikasi (pertanyaan terbuka 9.2 #5).
- `zeebe:taskDefinition type`/`retries` berbentuk FEEL hanya boleh literal atau path (mis.
  `="payment"`, `=jobType`); hasil harus string / bilangan bulat, selain itu incident
  `EXTRACT_VALUE_ERROR` saat aktivasi service task.

API paket (dipertahankan Fase 7):

```go
package feel

// Compile mem-parse ekspresi (tanpa "=" di depan). Ekspresi di luar subset → *UnsupportedError
// dengan posisi; sintaks rusak → *SyntaxError.
func Compile(src string) (*Expression, error)

// Evaluate menjalankan ekspresi terhadap konteks variabel. Hasil berupa Value FEEL.
func (e *Expression) Evaluate(ctx Context) (Value, error)

type Context interface {
	Lookup(name string) (Value, bool)
}

type Value struct{ /* unexported: kind null/bool/number/string/context/list */ }
func (v Value) Bool() (b bool, ok bool)
func (v Value) String() (s string, ok bool)
func FromJSON(raw []byte) (Value, error)
```

Penolakan saat deploy: `deployment/application` (Fase 1) ditambah satu langkah setelah
`bpmn.Validate` — setiap kondisi sequence flow dan ekspresi task definition di proses executable
di-`feel.Compile`; gagal → issue `EXPRESSION_NOT_SUPPORTED` (error) atau `EXPRESSION_SYNTAX_ERROR`
(error) dengan element id. Fase 7 menghapus `UnsupportedError` secara bertahap tanpa mengubah
pemanggil.

## 4. Model dan cakupan

### 4.1 Matriks perilaku elemen

| Elemen | Aktivasi | Wait state | Penyelesaian | Catatan |
|---|---|---|---|---|
| Process (`PROCESS`) | Mengaktifkan none start event | Ya, selama ada token | Saat flow scope tidak punya element instance aktif dan tidak ada counter join tersisa | Counter join tersisa yang tidak mungkin terpenuhi membuat instance tetap aktif (perilaku sama dengan Zeebe untuk model "macet"; verifikasi) |
| None start event | Langsung | Tidak | Langsung, ambil semua outgoing | Satu-satunya start yang dieksekusi Fase 2 |
| None end event | Langsung | Tidak | Langsung, lalu cek penyelesaian flow scope | — |
| Implicit end (activity/gateway tanpa outgoing) | — | — | Diperlakukan seperti end event | Sesuai warning `IMPLICIT_END` Fase 1; verifikasi perilaku Camunda 8.8 |
| Undefined task, manual task | Langsung | Tidak | Langsung, ambil semua outgoing | Pass-through |
| Service task | Evaluasi `type`/`retries`, buat job `CREATED` | Ya | `CompleteJob` → merge variabel → `COMPLETING` → `COMPLETED` → ambil outgoing | Header disalin ke job |
| Exclusive gateway | Evaluasi kondisi urut dokumen | Tidak | Ambil flow pertama yang `true`, else default, else incident | Sebagai join: setiap token yang datang mengaktifkan gateway (tanpa sinkronisasi) |
| Parallel gateway | Join: tunggu semua incoming | Tidak (counter, bukan element instance) | Ambil semua outgoing | Fork tanpa kondisi (divalidasi Fase 1) |
| Inclusive gateway | Join: 5.6 | Tidak | Ambil semua flow yang `true`; tidak ada → default; tidak ada default → incident | Kondisi dievaluasi semua (bukan berhenti di pertama) |
| Sequence flow | — | — | Record `SEQUENCE_FLOW_TAKEN` lalu aktivasi target (atau increment counter join) | Kondisi pada flow dari non-gateway sudah ditolak Fase 1 |

Urutan record satu langkah elemen pass-through (contoh none start → task → end), mengikuti pola
Zeebe (verifikasi urutan persis terhadap ekspor record Camunda 8.8 bila tersedia):

```
PROCESS_INSTANCE_CREATION  CREATED
PROCESS_INSTANCE           ELEMENT_ACTIVATING   process
PROCESS_INSTANCE           ELEMENT_ACTIVATED    process
PROCESS_INSTANCE           ELEMENT_ACTIVATING   start
PROCESS_INSTANCE           ELEMENT_ACTIVATED    start
PROCESS_INSTANCE           ELEMENT_COMPLETING   start
PROCESS_INSTANCE           ELEMENT_COMPLETED    start
PROCESS_INSTANCE           SEQUENCE_FLOW_TAKEN  flow1
PROCESS_INSTANCE           ELEMENT_ACTIVATING   task
...
PROCESS_INSTANCE           ELEMENT_COMPLETED    process
```

Record command (`ACTIVATE_ELEMENT`, `COMPLETE_ELEMENT`, `TERMINATE_ELEMENT`) **tidak** disimpan;
yang disimpan hanya event. Antrean kerja internal `Processor` memakai command in-memory.

### 4.2 Variabel

- Variabel awal `CreateProcessInstance` harus JSON object; setiap key top-level menjadi satu
  variabel di scope process instance (`VARIABLE CREATED`).
- `CompleteJob` dengan variabel: setiap key top-level ditulis ke scope process instance
  (`VARIABLE CREATED` jika baru, `UPDATED` jika nilai berubah, tanpa record jika identik secara
  byte setelah kanonisasi). Tanpa I/O mapping, ini setara propagasi Zeebe ke scope tertinggi.
- Nama variabel: string non-kosong ≤ 256 byte; nilai: JSON valid; ukuran dokumen per request
  default maks 1 MiB (`ENGINE_MAX_VARIABLES_BYTES`).
- Nilai disimpan sebagai teks JSON kanonik (urutan key dipertahankan sesuai input, angka tidak
  dikonversi) agar `int64` dan desimal tidak kehilangan presisi.

### 4.3 Job

| Field | Sumber |
|---|---|
| `key` | keygen |
| `type` | `zeebe:taskDefinition type` (statis atau subset FEEL) |
| `retries` | `zeebe:taskDefinition retries`, default 3 (verifikasi default Camunda 8.8) |
| `custom_headers` | `zeebe:taskHeaders`, urutan dipertahankan |
| `element_instance_key`, `process_instance_key`, `element_id`, `bpmn_process_id`, `process_definition_key`, `process_definition_version` | konteks element instance |
| `state` | Fase 2: `CREATED`, `COMPLETED`, `CANCELED`. Fase 4 menambah `ACTIVATED`, `FAILED`, `TIMED_OUT`, `ERROR_THROWN` |

Job tidak menyalin variabel; worker (Fase 4) mengambil variabel saat aktivasi.

### 4.4 Incident

| Tipe | Kondisi | Elemen berhenti di |
|---|---|---|
| `CONDITION_ERROR` | Exclusive/inclusive: tidak ada flow cocok dan tidak ada default; kondisi tidak menghasilkan boolean; error evaluasi | Gateway (verifikasi state persis Zeebe: `ACTIVATING` vs `ACTIVATED`) |
| `EXTRACT_VALUE_ERROR` | `type`/`retries` FEEL gagal dievaluasi atau tipe salah | Service task `ACTIVATING` |
| `EXECUTION_LIMIT_EXCEEDED` | Batas record per command terlampaui | Elemen yang sedang diaktifkan |

Nama tipe mengikuti Zeebe bila ada padanannya (`CONDITION_ERROR`, `EXTRACT_VALUE_ERROR`; verifikasi
daftar `ErrorType` Camunda 8.8). Incident berisi `key`, `type`, `message`, `element_instance_key`,
`element_id`, `process_instance_key`, `state` (`CREATED`/`RESOLVED`). Pembatalan instance menandai
incident terbuka `RESOLVED` (verifikasi perilaku Zeebe). Resolusi manual: Fase 4.

## 5. Desain domain

### 5.1 Sketsa tipe Go

```go
package domain

type ElementState uint8

const (
	ElementActivating ElementState = iota + 1
	ElementActivated
	ElementCompleting
	ElementCompleted
	ElementTerminating
	ElementTerminated
)

type ElementInstance struct {
	Key                int64
	ProcessInstanceKey int64
	FlowScopeKey       int64 // 0 untuk element instance PROCESS
	ElementID          string
	ElementType        bpmn.ElementType
	State              ElementState
}

// InstanceState adalah state RUNTIME satu process instance (hanya yang dibutuhkan untuk memproses
// command berikutnya). Elemen final tidak dimuat; riwayat dibaca dari proyeksi.
type InstanceState struct {
	Key                  int64
	Definition           DefinitionRef // key, bpmnProcessId, version
	Process              *bpmn.Process
	Status               InstanceStatus // Active, Completed, Terminated
	Elements             map[int64]*ElementInstance
	ChildrenByScope      map[int64][]int64           // urutan deterministik (key naik)
	TakenFlows           map[JoinKey]int32           // counter parallel/inclusive join
	Variables            map[VariableKey]json.RawMessage
	Jobs                 map[int64]*Job
	Incidents            map[int64]*Incident
}

type JoinKey struct {
	FlowScopeKey   int64
	GatewayID      string
	SequenceFlowID string
}

type ValueType uint8 // ProcessInstance, ProcessInstanceCreation, Variable, Job, Incident

type Intent uint8    // ElementActivating, ..., SequenceFlowTaken, Created, Updated, Completed, Canceled, Resolved

type Record struct {
	Key                int64     // key entitas (element instance, job, variable, incident)
	ProcessInstanceKey int64
	ValueType          ValueType
	Intent             Intent
	Timestamp          time.Time
	Value              RecordValue // payload per ValueType (struct, di-serialisasi JSON di infrastructure)
}

// Ports.
type KeyGenerator interface{ Next() int64 } // dipanggil hanya oleh Processor; implementasi blok sudah di-prefetch oleh application
type ExpressionEvaluator interface {
	EvaluateBool(expr bpmn.Expression, vars VariableLookup) (bool, error)
	EvaluateString(expr bpmn.Expression, vars VariableLookup) (string, error)
	EvaluateInt(expr bpmn.Expression, vars VariableLookup) (int64, error)
}
type Clock interface{ Now() time.Time }

type Repository interface {
	WithTx(ctx context.Context, fn func(tx TxRepository) error) error
}
type TxRepository interface {
	LockInstance(ctx context.Context, key int64) error
	LoadRuntimeState(ctx context.Context, key int64) (InstanceSnapshot, error)
	InstanceKeyOfJob(ctx context.Context, jobKey int64) (int64, error)
	AppendAndProject(ctx context.Context, records []Record) error
}

// Processor murni dan deterministik: input sama + key dan waktu sama → record sama.
type Processor struct {
	keys      KeyGenerator
	evaluator ExpressionEvaluator
	clock     Clock
	limits    Limits
}

func (p *Processor) Process(state *InstanceState, cmd Command) ([]Record, error)
func Apply(state *InstanceState, r Record) error // dipakai Processor dan replay (Fase 3)
```

`KeyGenerator` di domain sengaja **tanpa** `context` dan error: application mengambil blok key dari
`keygen` sebelum memanggil `Processor`, dan blok diperpanjang di luar domain bila habis
(`Processor` mengembalikan `ErrKeysExhausted`, application menambah blok lalu mengulang command dari
awal — aman karena belum ada yang ditulis).

### 5.2 Antrean kerja run-to-completion

`Processor` memakai antrean FIFO command internal (`activate(element, flowScope)`,
`complete(elementInstance)`, `terminate(elementInstance)`, `takeFlow(flow, flowScope)`). Satu
command eksternal mengisi antrean awal; loop berjalan sampai antrean kosong, lalu menjalankan
**pengecekan quiescence** (join inclusive yang tertunda, penyelesaian flow scope). Jika pengecekan
menambah kerja, loop berlanjut. Urutan deterministik dijamin dengan: outgoing flow diproses sesuai
urutan dokumen, anak per scope diurutkan menurut key, tidak ada iterasi `map` tanpa pengurutan.

FIFO (breadth-first) dipilih karena urutan record yang dihasilkan cocok dengan cara Zeebe menulis
follow-up record per batch (verifikasi dengan membandingkan record Camunda 8.8 untuk skenario fork
paralel; urutan record **bukan** bagian kontrak paritas, hanya hasil observable).

### 5.3 Exclusive gateway

1. Satu outgoing tanpa kondisi → ambil.
2. Untuk setiap outgoing non-default sesuai urutan dokumen: evaluasi kondisi; `true` pertama diambil,
   sisanya tidak dievaluasi.
3. Tidak ada yang `true` → default jika ada; jika tidak → incident `CONDITION_ERROR` dengan pesan
   menyebut id gateway dan bahwa tidak ada flow yang cocok.
4. Error evaluasi di kondisi mana pun sebelum ditemukan `true` → incident `CONDITION_ERROR`.

### 5.4 Parallel gateway

- Fork: ambil semua outgoing dalam urutan dokumen.
- Join (incoming > 1): `takeFlow` ke gateway menaikkan `TakenFlows[{scope, gateway, flow}]` (record
  `SEQUENCE_FLOW_TAKEN` sudah cukup; counter adalah turunan record, di-`Apply`). Setelah increment,
  jika semua incoming punya counter ≥ 1: kurangi setiap counter 1, aktifkan gateway.
- Satu incoming: pass-through.
- Token kedua di flow yang sama sebelum join lengkap tetap tercatat (counter 2) dan memicu join
  berikutnya — benar untuk loop.

### 5.5 Penyelesaian flow scope

Setelah elemen selesai/dihentikan, flow scope (Fase 2: element instance `PROCESS`) selesai jika:
tidak ada anak berstatus non-final **dan** tidak ada counter `TakenFlows` > 0 di scope itu
**dan** scope tidak sedang `TERMINATING`. Counter > 0 yang tidak akan pernah lengkap (mis. exclusive
split diikuti parallel join) membuat instance tetap `ACTIVE` tanpa token aktif. Ini dicatat sebagai
perilaku yang diketahui; Operate (Fase 6) menampilkannya sebagai "token menunggu di join".

### 5.6 Inclusive gateway join: algoritma dan batasnya

**Prasyarat statis** (dihitung sekali per `*bpmn.Process` dan di-cache di `Processor` menurut
`process_definition_key`): untuk setiap inclusive gateway `G` dengan incoming > 1 dan setiap incoming
flow `f` dari `G`, `Upstream(G, f)` = himpunan flow node dan sequence flow yang dapat mencapai `f`
**tanpa melewati `G`** (BFS mundur dari `f.Source()`, berhenti di `G`).

**Aturan runtime** (dicek saat quiescence, untuk setiap pasangan `(scope, G)` dengan minimal satu
counter > 0):

1. Ambil `Waiting = { f ∈ incoming(G) : counter(f) = 0 }`.
2. Ambil posisi token aktif di scope: setiap element instance non-final di scope (posisinya = node
   elemennya) **dan** setiap counter join lain > 0 (posisinya = gateway join tersebut).
3. Join diaktifkan jika **tidak ada** posisi token yang berada di `Upstream(G, f)` untuk `f ∈ Waiting`.
4. Saat aktif: setiap counter incoming yang > 0 dikurangi 1 (bukan di-reset), gateway diaktifkan,
   lalu fork inclusive seperti biasa.

Pengecekan dilakukan **hanya saat antrean kosong** sehingga token yang sedang "terbang" dalam
command yang sama sudah sampai ke posisi akhirnya; ini menghindari join prematur. Kompleksitas
O(token aktif × incoming G) per join tertunda, cukup untuk ukuran instance yang wajar.

**Batas yang diketahui** (dicatat di ADR-0013, diuji sebagai perilaku terdokumentasi):

| Kasus | Perilaku kita |
|---|---|
| Graf blok (setiap fork inclusive punya join pasangan), asiklik | Benar |
| Join di dalam loop; token di belakang join yang akan kembali melewati `G` | Token tersebut **tidak** dihitung upstream (BFS berhenti di `G`), sehingga join tidak menunggu putaran berikutnya — sesuai semantik "tidak ada token yang bisa sampai tanpa melewati G" |
| Dua inclusive join yang saling menjadi upstream (siklus antar join) | Bisa deadlock (keduanya menunggu). Deploy memberi warning `INCLUSIVE_JOIN_IN_CYCLE` (aturan baru di `libs/go/bpmn`, hanya warning) |
| Token di wait state jauh di upstream (service task belum selesai) | Join menunggu, sesuai spesifikasi |
| Sub-process dan boundary event (Fase 5) | Element instance sub-process dihitung sebagai token di node sub-process; boundary event diperlakukan sebagai outgoing tambahan host pada analisis reachability. Ditinjau ulang di Fase 5 |

Dukungan dan semantik inclusive join di Camunda 8.8 (termasuk batasannya) harus diverifikasi
terhadap dokumentasi Camunda 8.8; deviasi masuk `expectations.json`.

### 5.7 Pembatalan

`CancelInstance` → `TERMINATING` pada element instance `PROCESS` → `terminate` setiap anak non-final
(urutan key menurun) → service task: job `CANCELED`, elemen `TERMINATED` → incident terbuka
`RESOLVED` → `PROCESS` `TERMINATED`. Counter join di scope tidak punya record sendiri; `Apply`
menghapusnya saat scope menjadi `TERMINATED`. Instance yang sudah final →
`FailedPrecondition`.

## 6. Persistensi

### 6.1 Skema database (`00003_execution.sql`)

```sql
-- +goose Up
CREATE TABLE process_instance (
    key                         BIGINT PRIMARY KEY,           -- = key element instance PROCESS
    tenant_id                   TEXT NOT NULL DEFAULT '<default>',
    process_definition_key      BIGINT NOT NULL REFERENCES process_definition (key),
    bpmn_process_id             TEXT NOT NULL,
    process_definition_version  INT NOT NULL,
    state                       TEXT NOT NULL CHECK (state IN ('ACTIVE', 'COMPLETED', 'TERMINATED')),
    has_incident                BOOLEAN NOT NULL DEFAULT false,
    version                     BIGINT NOT NULL DEFAULT 0,    -- naik setiap command; deteksi anomali
    start_time                  TIMESTAMPTZ NOT NULL,
    end_time                    TIMESTAMPTZ
);
CREATE INDEX process_instance_list_idx ON process_instance (tenant_id, bpmn_process_id, key DESC);
CREATE INDEX process_instance_active_idx ON process_instance (key) WHERE state = 'ACTIVE';

CREATE TABLE element_instance (
    key                   BIGINT PRIMARY KEY,
    process_instance_key  BIGINT NOT NULL REFERENCES process_instance (key),
    flow_scope_key        BIGINT,                              -- NULL untuk PROCESS
    element_id            TEXT NOT NULL,
    bpmn_element_type     TEXT NOT NULL,                       -- PROCESS, START_EVENT, SERVICE_TASK, ...
    state                 TEXT NOT NULL,                       -- ACTIVATING ... TERMINATED
    incident_key          BIGINT,
    start_time            TIMESTAMPTZ NOT NULL,
    end_time              TIMESTAMPTZ
);
CREATE INDEX element_instance_by_pi_idx ON element_instance (process_instance_key, key);
CREATE INDEX element_instance_runtime_idx ON element_instance (process_instance_key)
    WHERE state NOT IN ('COMPLETED', 'TERMINATED');

CREATE TABLE taken_sequence_flow (
    process_instance_key  BIGINT NOT NULL REFERENCES process_instance (key),
    flow_scope_key        BIGINT NOT NULL,
    gateway_id            TEXT NOT NULL,
    sequence_flow_id      TEXT NOT NULL,
    count                 INT NOT NULL CHECK (count > 0),     -- baris dihapus saat count = 0
    PRIMARY KEY (flow_scope_key, gateway_id, sequence_flow_id)
);

CREATE TABLE variable (
    key                   BIGINT PRIMARY KEY,
    process_instance_key  BIGINT NOT NULL REFERENCES process_instance (key),
    scope_key             BIGINT NOT NULL,                     -- Fase 2: selalu = process_instance_key
    name                  TEXT NOT NULL,
    value                 TEXT NOT NULL,                       -- teks JSON kanonik (bukan JSONB: presisi angka & urutan key)
    updated_at            TIMESTAMPTZ NOT NULL,
    UNIQUE (scope_key, name)
);
CREATE INDEX variable_by_pi_idx ON variable (process_instance_key);

CREATE TABLE job (
    key                         BIGINT PRIMARY KEY,
    process_instance_key        BIGINT NOT NULL REFERENCES process_instance (key),
    element_instance_key        BIGINT NOT NULL,
    element_id                  TEXT NOT NULL,
    bpmn_process_id             TEXT NOT NULL,
    process_definition_key      BIGINT NOT NULL,
    process_definition_version  INT NOT NULL,
    type                        TEXT NOT NULL,
    retries                     INT NOT NULL,
    custom_headers              JSONB NOT NULL DEFAULT '{}',
    state                       TEXT NOT NULL,                 -- CREATED, COMPLETED, CANCELED (Fase 4 menambah)
    created_at                  TIMESTAMPTZ NOT NULL,
    updated_at                  TIMESTAMPTZ NOT NULL
);
CREATE INDEX job_activatable_idx ON job (type, key) WHERE state = 'CREATED';  -- dipakai SKIP LOCKED Fase 4

CREATE TABLE incident (
    key                   BIGINT PRIMARY KEY,
    process_instance_key  BIGINT NOT NULL REFERENCES process_instance (key),
    element_instance_key  BIGINT NOT NULL,
    element_id            TEXT NOT NULL,
    error_type            TEXT NOT NULL,
    error_message         TEXT NOT NULL,
    state                 TEXT NOT NULL,                       -- CREATED, RESOLVED
    created_at            TIMESTAMPTZ NOT NULL,
    resolved_at           TIMESTAMPTZ
);
CREATE INDEX incident_open_idx ON incident (process_instance_key) WHERE state = 'CREATED';

-- Log append-only. Sumber kebenaran mulai Fase 3; di Fase 2 ditulis dalam transaksi yang sama
-- dengan proyeksi dan diuji determinismenya.
CREATE TABLE execution_record (
    position              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY, -- urutan global (exporter, Operate)
    process_instance_key  BIGINT NOT NULL,
    sequence              INT NOT NULL,                        -- urutan per instance, mulai 1, tanpa celah
    record_key            BIGINT NOT NULL,
    value_type            TEXT NOT NULL,
    intent                TEXT NOT NULL,
    element_id            TEXT,
    payload               JSONB NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL,
    UNIQUE (process_instance_key, sequence)
);

-- +goose Down
DROP TABLE execution_record;
DROP TABLE incident;
DROP TABLE job;
DROP TABLE variable;
DROP TABLE taken_sequence_flow;
DROP TABLE element_instance;
DROP TABLE process_instance;
```

Catatan:

- `UNIQUE (process_instance_key, sequence)` adalah jaring pengaman kedua bila row lock pernah
  dilewati (bug): dua writer akan bertabrakan, bukan menulis riwayat bercabang.
- Elemen final tetap di `element_instance` untuk `GetProcessInstance`; retensi/arsip diputuskan
  Fase 3/6. Indeks parsial membuat pemuatan runtime tetap murah.
- Foreign key ke `process_definition` lintas modul diterima di level SQL (satu database, satu
  binary); kode Go tetap hanya lewat `deployment/api`.

Kueri sqlc (`queries/execution.sql`): `InsertProcessInstance`, `LockProcessInstance`,
`UpdateProcessInstanceState`, `ListRuntimeElementInstances`, `UpsertElementInstance`,
`ListTakenFlows`, `UpsertTakenFlow`/`DeleteTakenFlow`, `ListVariables`, `UpsertVariable`,
`InsertJob`, `UpdateJobState`, `GetJob`, `ListJobs` (filter `type`, `state`, `process_instance_key`,
keyset), `InsertIncident`, `ResolveIncidents`, `AppendRecords` (`COPY`), `MaxSequence`,
`GetProcessInstance`, `ListElementInstances`, `ListProcessInstances` (keyset `key DESC`, filter
`bpmn_process_id`, `process_definition_key`, `state`, `has_incident`).

### 6.2 Perubahan kecil di modul `deployment`

- `api.ProcessDefinitions.GetByVersion(ctx, bpmnProcessID string, version int32)` untuk create
  instance dengan versi tertentu.
- Langkah kompilasi ekspresi (3.4) di `DeployResource`.
- `ProfileExecutable` di `libs/go/bpmn`: issue `IO_MAPPING_NOT_SUPPORTED` dan warning
  `INCLUSIVE_JOIN_IN_CYCLE`.

Perubahan ini dikerjakan setelah Fase 1 di-merge, dalam PR terpisah, dengan update golden yang
relevan.

### 6.3 Pembagian kerja Fase 2 vs Fase 3

| Aspek | Fase 2 | Fase 3 |
|---|---|---|
| Record | Ditulis, dibaca hanya oleh test determinisme | Sumber kebenaran: rebuild proyeksi dari record, verifikasi konsistensi saat start |
| Proyeksi | Ditulis bersama record, dibaca untuk runtime dan query | Bisa dibangun ulang; kemungkinan pemisahan runtime vs history |
| Konkurensi | Row lock per instance | Ditinjau: partisi per rentang key + actor in-memory bila contention terukur; domain tidak berubah |
| Event antarmodul | Publish setelah commit (bisa hilang saat crash) | Outbox transaksional dari `execution_record` |
| Idempotensi | Tidak ada; `CompleteJob` ganda → `FailedPrecondition` (sudah aman secara state) | `request_id` untuk `CreateProcessInstance` |
| Crash di tengah command | Transaksi rollback, tidak ada state setengah jadi | Sama; ditambah pemulihan pekerjaan terjadwal (timer Fase 5) |

## 7. Kontrak proto

### 7.1 `workflow/process/v1/process_instance.proto`

```proto
syntax = "proto3";
package workflow.process.v1;
option go_package = "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/process/v1;processv1";

import "google/protobuf/timestamp.proto";

service ProcessInstanceService {
  rpc CreateProcessInstance(CreateProcessInstanceRequest) returns (CreateProcessInstanceResponse) {}
  rpc GetProcessInstance(GetProcessInstanceRequest) returns (GetProcessInstanceResponse) {}
  rpc ListProcessInstances(ListProcessInstancesRequest) returns (ListProcessInstancesResponse) {}
  rpc CancelProcessInstance(CancelProcessInstanceRequest) returns (CancelProcessInstanceResponse) {}
}

message CreateProcessInstanceRequest {
  oneof definition {
    int64 process_definition_key = 1;
    ProcessDefinitionRef bpmn_process = 2;
  }
  string variables = 3;  // JSON object; kosong = {}
}

message ProcessDefinitionRef {
  string bpmn_process_id = 1;
  int32 version = 2;  // 0 = versi terbaru
}

message CreateProcessInstanceResponse {
  int64 process_instance_key = 1;
  int64 process_definition_key = 2;
  string bpmn_process_id = 3;
  int32 version = 4;
}

message GetProcessInstanceRequest {
  int64 process_instance_key = 1;
}

message GetProcessInstanceResponse {
  ProcessInstance process_instance = 1;
  repeated ElementInstance element_instances = 2;  // termasuk yang final, urut key
  repeated Variable variables = 3;
  repeated Incident incidents = 4;
  repeated Job jobs = 5;
}

message ProcessInstance {
  int64 process_instance_key = 1;
  int64 process_definition_key = 2;
  string bpmn_process_id = 3;
  int32 version = 4;
  ProcessInstanceState state = 5;
  bool has_incident = 6;
  google.protobuf.Timestamp start_time = 7;
  google.protobuf.Timestamp end_time = 8;
}

enum ProcessInstanceState {
  PROCESS_INSTANCE_STATE_UNSPECIFIED = 0;
  PROCESS_INSTANCE_STATE_ACTIVE = 1;
  PROCESS_INSTANCE_STATE_COMPLETED = 2;
  PROCESS_INSTANCE_STATE_TERMINATED = 3;
}

message ElementInstance {
  int64 element_instance_key = 1;
  int64 flow_scope_key = 2;
  string element_id = 3;
  string bpmn_element_type = 4;  // "SERVICE_TASK", "EXCLUSIVE_GATEWAY", ...
  ElementInstanceState state = 5;
  int64 incident_key = 6;
  google.protobuf.Timestamp start_time = 7;
  google.protobuf.Timestamp end_time = 8;
}

enum ElementInstanceState {
  ELEMENT_INSTANCE_STATE_UNSPECIFIED = 0;
  ELEMENT_INSTANCE_STATE_ACTIVATING = 1;
  ELEMENT_INSTANCE_STATE_ACTIVATED = 2;
  ELEMENT_INSTANCE_STATE_COMPLETING = 3;
  ELEMENT_INSTANCE_STATE_COMPLETED = 4;
  ELEMENT_INSTANCE_STATE_TERMINATING = 5;
  ELEMENT_INSTANCE_STATE_TERMINATED = 6;
}

message Variable {
  int64 scope_key = 1;
  string name = 2;
  string value = 3;  // JSON
}

message Incident {
  int64 incident_key = 1;
  int64 element_instance_key = 2;
  string element_id = 3;
  string error_type = 4;
  string error_message = 5;
  bool resolved = 6;
  google.protobuf.Timestamp created_at = 7;
}

message Job {
  int64 job_key = 1;
  string type = 2;
  string element_id = 3;
  int64 element_instance_key = 4;
  int32 retries = 5;
  string state = 6;  // CREATED, COMPLETED, CANCELED
}

message ListProcessInstancesRequest {
  string bpmn_process_id = 1;
  int64 process_definition_key = 2;
  ProcessInstanceState state = 3;
  bool only_with_incident = 4;
  int32 page_size = 5;     // default 50, maks 500
  string page_token = 6;
}

message ListProcessInstancesResponse {
  repeated ProcessInstance process_instances = 1;
  string next_page_token = 2;
}

message CancelProcessInstanceRequest {
  int64 process_instance_key = 1;
}

message CancelProcessInstanceResponse {}
```

### 7.2 `workflow/job/v1/job.proto`

```proto
syntax = "proto3";
package workflow.job.v1;
option go_package = "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1;jobv1";

service JobService {
  rpc ListJobs(ListJobsRequest) returns (ListJobsResponse) {}
  rpc CompleteJob(CompleteJobRequest) returns (CompleteJobResponse) {}
  // Fase 4: ActivateJobs, FailJob, ThrowError, UpdateJobRetries.
}

message ListJobsRequest {
  string type = 1;
  int64 process_instance_key = 2;
  bool only_open = 3;      // state CREATED
  int32 page_size = 4;
  string page_token = 5;
}

message ListJobsResponse {
  repeated JobInfo jobs = 1;
  string next_page_token = 2;
}

message JobInfo {
  int64 job_key = 1;
  string type = 2;
  int64 process_instance_key = 3;
  int64 element_instance_key = 4;
  string element_id = 5;
  string bpmn_process_id = 6;
  int32 retries = 7;
  map<string, string> custom_headers = 8;
  string state = 9;
}

message CompleteJobRequest {
  int64 job_key = 1;
  string variables = 2;  // JSON object; kosong = {}
}

message CompleteJobResponse {}
```

Pemetaan error: `ErrInstanceNotFound`/`ErrJobNotFound`/definition tidak ada → `NotFound`;
`ErrInvalidVariables`, `oneof` kosong → `InvalidArgument`; instance final, job bukan `CREATED` →
`FailedPrecondition`; batas ukuran → `InvalidArgument` (`connect.WithReadMaxBytes`). `JobInfo`
memakai `string state` (bukan enum) di Fase 2 agar penambahan state Fase 4 tidak memerlukan
keputusan ulang; dikonversi ke enum di Fase 4 bila diperlukan (catat sebagai pertanyaan 9.2 #7).

## 8. Rencana kerja Fase 2 (urutan eksekusi)

Setiap langkah punya **hasil yang bisa diverifikasi**. 8.3 (feel) bisa paralel dengan 8.4–8.7
(domain, memakai evaluator palsu); 8.9–8.10 (proto, skema) paralel dengan domain.

### 8.1 Prasyarat dari Fase 1

- `libs/go/bpmn` dengan `ProfileExecutable`, korpus `testdata/valid`, modul `deployment` dengan
  `api.ProcessDefinitions`, `keygen`, dan migrasi `00002` sudah di `main`.
- Jika Fase 1 belum merge: domain dan `feel` tetap bisa dikerjakan memakai `bpmn.Parse` langsung di
  test dan `ProcessDefinitions` palsu.

Hasil: branch `feat/phase-2-execution` dari `main` setelah merge Fase 1; `make check` hijau.

### 8.2 Allowlist kernel di `archtest` (ADR-0010)

- `kernelAllowlist`, `isKernel`, kasus test baru, `TestKernelsArePure` (3.1); depguard deny
  `libs/go/sdk` untuk domain.
- Bukti pelanggaran sengaja: domain meng-import `libs/go/bpmn/internal/...` → gagal.

Hasil: `go test ./apps/engine/internal/archtest` lolos; ADR-0010 ditulis.

### 8.3 `libs/go/feel` subset

- `project.json` (`feel-go`, tag `type:kernel`, `lang:go`), `deps_test.go` (hanya stdlib).
- Lexer, parser (grammar 3.4), evaluator dengan `big.Rat`, logika tiga nilai, `FromJSON`.
- Tabel uji ≥ 80 kasus: literal, path bersarang, `null`, perbandingan lintas tipe, string escape,
  angka besar (`9007199254740993`), presisi desimal (`0.1 + ...` tidak didukung → `UnsupportedError`).
- Setiap kasus yang semantiknya tidak pasti ditandai `// verify: Camunda 8.8` dan dimasukkan ke
  skenario paritas.
- `FuzzCompile`: tidak panic, `Compile` sukses ⇒ `Evaluate` tidak panic pada konteks acak.

Hasil: `pnpm nx run-many -t build test lint -p feel-go` lolos; `nx run feel-go:fuzz` 10 menit bersih.

### 8.4 Domain: state, record, `Apply`

- Tipe di 5.1; `Apply` untuk setiap `(ValueType, Intent)`; validasi transisi state yang sah
  (`ACTIVATING → ACTIVATED`, dst.) dengan error jika record melanggar.
- Serialisasi `RecordValue` tidak di domain; domain hanya struct.

Hasil: unit test transisi (tabel semua pasangan state), `Apply` atas transisi ilegal mengembalikan
error.

### 8.5 Domain: `Processor`, start/end/task, exclusive dan parallel gateway

- Antrean run-to-completion (5.2), perilaku 4.1 untuk elemen selain inclusive, penyelesaian flow
  scope (5.5), service task membuat job, `CompleteJob`, `CancelInstance`, batas record.
- Evaluator palsu (map ekspresi → hasil) sebelum 8.3 siap.

Hasil: skenario tabel in-memory untuk pola linear, exclusive (cabang, default, incident), parallel
(fork/join, join bersarang, loop melalui exclusive), cancel; test determinisme (Process dua kali
dengan key/jam palsu sama → record identik).

### 8.6 Domain: inclusive gateway

- Analisis `Upstream` (5.6) dengan cache per definition key; join saat quiescence.
- Aturan validator `INCLUSIVE_JOIN_IN_CYCLE` (warning) di `libs/go/bpmn` (dikerjakan bersama 6.2).

Hasil: skenario inclusive (semua cabang, sebagian cabang, default, satu cabang menunggu job, join
bersarang dalam parallel, loop melewati join) lolos; kasus siklus antar join terdokumentasi sebagai
deadlock + warning.

### 8.7 Property test gateway

- Generator graf blok acak (stdlib `math/rand/v2` dengan seed tercatat): blok sekuensial, parallel
  fork/join, inclusive fork/join dengan kondisi acak atas variabel acak, exclusive split/merge,
  service task di daun; kedalaman ≤ 4, lebar ≤ 5.
- Simulator: create instance, lalu selesaikan job terbuka dalam urutan acak sampai tidak ada job.
- Properti: instance `COMPLETED`; jumlah aktivasi tiap join = jumlah aktivasi fork pasangannya;
  `TakenFlows` kosong; jumlah element instance `COMPLETED` cocok dengan hitungan oracle yang
  dihitung dari graf + kondisi; replay record → state identik.
- Seed gagal dicetak dan bisa diulang `PROP_SEED=...`.

Hasil: `go test -run TestGatewayProperties` 1.000 iterasi < 30 detik di CI; target `prop` untuk
100.000 iterasi terjadwal.

### 8.8 Application: use case dan command runner

- `CreateProcessInstance` (resolusi lewat `deployment/api`), `CompleteJob`, `CancelProcessInstance`,
  query `Get`/`List`, `ListJobs`; prefetch blok key; publikasi event setelah commit.
- Unit test dengan repository fake (in-memory, menerapkan record lewat `Apply` + proyeksi palsu).

Hasil: semua skenario 8.5–8.6 lulus lewat application dengan repository fake.

### 8.9 Proto `workflow/process/v1` dan `workflow/job/v1`

- Tulis proto (7.1, 7.2), `buf lint` dan `buf breaking` lolos, `buf generate`, hasil di-commit.

Hasil: paket `processv1`, `jobv1` dan `*connect` terkompilasi; stub TS ada di `libs/web/api-client`.

### 8.10 Migrasi, sqlc, repository, proyektor

- `00003_execution.sql` (6.1); `sqlc.yaml` kedua di `execution/infrastructure`; `make gen`
  menjalankan semua `sqlc.yaml`.
- `postgres_repository.go`, `projector.go` (satu fungsi per `(ValueType, Intent)`), `keygen_block.go`,
  `feel_evaluator.go`.
- Integration test: proyeksi setiap skenario sama dengan state in-memory; 50 `CompleteJob` paralel
  pada satu instance; create 200 instance paralel.

Hasil: `make test-integration` lolos; `archtest` lolos.

### 8.11 Interfaces dan wiring

- Handler Connect `ProcessInstanceService` dan `JobService`, pemetaan error (7.2), batas baca.
- `cmd/engine/main.go` merakit `execution` dengan `deployment/api`, keygen, event bus, clock.
- Test handler (`httptest`) end-to-end: deploy (lewat `DeploymentService`) → create → list jobs →
  complete → get = `COMPLETED`.

Hasil: `buf curl` ke `CreateProcessInstance` dengan proses korpus mengembalikan key; `GetProcessInstance`
menampilkan element instance dan variabel.

### 8.12 Skenario tabel penuh

- Satu file Go per kelompok di `execution/testdata/scenarios` (atau `_test.go` di `application`):

```go
var scenarios = []Scenario{{
	Name:    "exclusive-default-flow",
	BPMN:    "libs/go/bpmn/testdata/valid/exclusive-gateway-default.bpmn",
	Start:   `{"amount": 50}`,
	Steps:   []Step{CompleteJob("approve", `{"approved": true}`)},
	Expect: Expect{
		State:         Completed,
		CompletedPath: []string{"start", "gw", "approve", "end_small"}, // multiset element id
		Variables:     `{"amount": 50, "approved": true}`,
		JobTypes:      []string{"approve"},
	},
	Parity: true,
}}
```

- Skenario sama dijalankan tiga kali: domain murni, application + Postgres (integration), dan
  runner paritas (8.13).

Hasil: semua baris matriks 4.1 dan setiap tipe incident 4.4 tercakup; daftar skenario dicetak di
README modul.

### 8.13 Uji paritas eksekusi

- Perluas `tools/scripts/parity-deploy` (Fase 1) menjadi `tools/scripts/parity` dengan suite
  `deploy` dan `execution`.
- Untuk setiap skenario `Parity: true`: deploy ke Camunda 8.8 referensi, create instance, lalu
  worker sederhana di runner mengaktifkan dan menyelesaikan job per langkah; bandingkan hasil
  observable: (1) selesai atau incident, (2) multiset tipe job yang diaktifkan, (3) variabel akhir.
  Endpoint REST v2 yang dipakai (`/v2/process-instances`, `/v2/jobs/activation`,
  `/v2/jobs/{jobKey}/completion`, dan pencarian element instance/variabel/incident) harus
  diverifikasi terhadap dokumentasi Camunda 8.8, termasuk apakah API pencarian membutuhkan
  secondary storage yang belum ada di profil `camunda`. Jika pencarian tidak tersedia, variabel akhir
  diambil lewat create instance dengan `awaitCompletion` (verifikasi) dan perbandingan jalur elemen
  dilewati.
- Hasil direkam di `execution/testdata/parity/expectations.json`; mode offline di CI membandingkan
  engine kita dengan rekaman, mode `--live` memperbarui rekaman.

Hasil: `make parity SUITE=execution` lolos; laporan deviasi terlampir di PR penutup fase.

### 8.14 Modeler-web fitur `process-instance` (minimal)

```
apps/modeler-web/src/features/process-instance/
├─ domain/          # ProcessInstance, ElementInstance, aturan marker (aktif/selesai/incident) per element id
├─ application/     # useCreateInstance, useProcessInstance (refetchInterval 1 s saat ACTIVE),
│                   # useProcessInstances, useCompleteJob; instance.store.ts (elemen terpilih)
├─ infrastructure/  # repository Connect ProcessInstanceService + JobService, mapper proto → domain
└─ presentation/    # StartInstanceDialog (editor JSON + validasi Zod), InstancesTable,
                    # InstanceViewer (bpmn-js viewer + marker + overlay jumlah token), JobsPanel
```

- Marker: `canvas.addMarker(id, 'token-active' | 'token-completed' | 'token-incident')`, overlay
  badge jumlah element instance aktif per elemen.
- Halaman `/instances` dan `/instances/$key` (TanStack Router); tombol "Start" di tabel definitions
  Fase 1.
- Fitur ini sengaja minimal dan akan dipindah/diperluas ke `operate-web` di Fase 6; domain dan
  infrastructure-nya dirancang agar bisa dipindah utuh.

Hasil: Vitest untuk mapper dan aturan marker; uji manual (fork paralel, incident gateway, cancel)
tercatat di PR dengan screenshot.

### 8.15 Benchmark dan dokumentasi

- `BenchmarkProcessorLinear`, `BenchmarkProcessorParallel50`, `BenchmarkInclusiveJoin`, benchmark
  integrasi create + complete.
- ADR-0010 (allowlist kernel), ADR-0012 (model eksekusi: record append-only + proyeksi, single
  writer row lock, kepemilikan job), ADR-0013 (algoritma join parallel dan inclusive beserta
  batasnya), ADR-0014 (subset FEEL dan penolakan saat deploy).
- README modul `execution` (siklus hidup, daftar record, daftar incident) dan README `libs/go/feel`
  (subset yang didukung, tabel semantik `null`).
- Perbarui `docs/plan/README.md`: Fase 2 menautkan dokumen ini.

Hasil: angka benchmark tercatat; ADR tersimpan; review DoD.

## 9. Target Nx dan Makefile

| Proyek | Target | Perintah |
|---|---|---|
| `feel-go` | `build` | `go build ./libs/go/feel/...` |
| `feel-go` | `test` | `go test -race ./libs/go/feel/...` |
| `feel-go` | `lint` | `golangci-lint run ./libs/go/feel/...` |
| `feel-go` | `fuzz` (cache off) | `go test -run=^$ -fuzz=FuzzCompile -fuzztime=$FUZZTIME ./libs/go/feel` |
| `engine` | `test` | sudah ada; kini mencakup `execution` (skenario domain + application fake, property 1.000 iterasi) |
| `engine` | `prop` (cache off) | `go test -run=TestGatewayProperties -count=1 ./apps/engine/internal/modules/execution/domain -prop.iter=$PROP_ITER` |
| `engine` | `bench` (cache off) | `go test -run=^$ -bench=. -benchmem ./apps/engine/internal/modules/execution/...` |
| `engine` | `sqlc` | diperluas: menjalankan `sqlc generate` untuk `deployment` dan `execution` |
| `proto` | `generate`, `lint`, `breaking` | sudah ada; mencakup `process/v1`, `job/v1` |
| `modeler-web` | `test`, `build` | sudah ada; mencakup fitur `process-instance` |

`engine` menambah `feel-go` ke `implicitDependencies`; input `engine:test` menyertakan
`libs/go/bpmn/testdata/**` karena skenario memakai korpus.

Target Makefile baru/diperluas: `make prop [PROP_ITER=100000] [PROP_SEED=...]`,
`make bench` (kini juga engine), `make parity [SUITE=deploy|execution|all] [LIVE=1]`,
`make fuzz [TARGET=bpmn|feel] [FUZZTIME=10m]`.

## 10. Risiko dan mitigasi

| Risiko | Dampak | Mitigasi |
|---|---|---|
| Semantik inclusive join berbeda dari Camunda 8.8 | Paritas gagal di model nyata | Skenario paritas khusus inclusive sejak 8.13; algoritma dan batas di ADR-0013; deviasi tercatat; warning deploy untuk siklus |
| Subset FEEL menyimpang dari FEEL penuh (`null`, tipe campuran, angka) | Fase 7 mengubah perilaku proses yang sudah jalan | Semantik diambil dari spesifikasi FEEL, kasus tak pasti ditandai `verify` dan masuk paritas; konstruksi di luar subset ditolak saat deploy, bukan ditebak |
| Non-determinisme (iterasi `map`, jam, urutan goroutine) | Replay Fase 3 gagal, test flaky | `Processor` tanpa goroutine; semua iterasi diurutkan; `Clock` dan `KeyGenerator` di-inject; test determinisme untuk setiap skenario + property test replay |
| Contention row lock pada instance dengan banyak cabang paralel | Latensi `CompleteJob` naik | Transaksi pendek (muat state runtime saja, tanpa elemen final); benchmark 50 cabang; partisi/actor ditinjau Fase 3 bila terukur |
| Pertumbuhan `execution_record` dan `element_instance` | Disk dan query lambat | Indeks parsial runtime; kebijakan retensi/arsip diputuskan Fase 3/6; ukuran per instance diukur di benchmark |
| Model "macet" (exclusive split → parallel join) tidak pernah selesai | Instance menggantung tanpa incident | Perilaku sama dengan Zeebe (verifikasi); dokumentasi; Operate Fase 6 menandai token di join; opsional warning lint di masa depan |
| Loop tanpa wait state | Command tidak berhenti | Batas record per command → incident `EXECUTION_LIMIT_EXCEEDED` |
| Fase 1 terlambat atau API `deployment` berubah | Fase 2 tertahan | Domain, `feel`, property test hanya butuh `libs/go/bpmn`; `ProcessDefinitions` palsu di test; perubahan `deployment` dikumpulkan di 6.2 |
| Kepemilikan job di `execution` membuat modul `job` Fase 4 tipis atau bocor | Batas modul kabur | Kontrak `execution/api.Jobs` ditetapkan di ADR-0012; modul `job` hanya protokol worker; ditinjau di awal Fase 4 |
| API pencarian Camunda 8.8 butuh secondary storage | Paritas jalur elemen tidak bisa dibandingkan | Paritas difokuskan pada hasil observable (selesai/incident, tipe job, variabel akhir); profil compose dengan secondary storage opsional |
| Event pasca-commit hilang saat crash | Subscriber (Operate, long polling) tertinggal | Tidak ada subscriber kritis di Fase 2; outbox dari `execution_record` di Fase 3 |
| Allowlist kernel dipakai untuk menyelundupkan dependensi | Domain tidak murni | `TestKernelsArePure` + `deps_test.go` setiap kernel; paket `internal` kernel tetap terlarang |

## 11. Pertanyaan terbuka

Setiap pertanyaan diberi rekomendasi. Keputusan final dicatat di 11.1 setelah review.

### 11.1 Keputusan

Dicatat 2026-10-28. User meminta Fase 2 dimulai tanpa review terpisah; seperti di Fase 1, setiap
pertanyaan memakai rekomendasi dokumen ini.

| # | Keputusan |
|---|---|
| 1 | Allowlist kernel `libs/go/bpmn` dan `libs/go/feel` untuk `domain`, paket `internal` kernel tetap terlarang, dijaga `TestKernelsArePure` (ADR-0010) |
| 2 | Record log append-only + proyeksi dalam satu transaksi |
| 3 | Row lock `FOR UPDATE` per instance; partisi ditinjau di Fase 3 |
| 4 | Subset FEEL di `libs/go/feel`; ekspresi di luar subset ditolak saat deploy |
| 5 | `conditionExpression` tanpa prefix `=` ditolak saat deploy (`EXPRESSION_NOT_SUPPORTED`) |
| 6 | State job dimiliki modul `execution`; modul `job` Fase 4 menjadi protokol worker |
| 7 | `state` job di proto berupa string di Fase 2; enum di Fase 4 |
| 8 | I/O mapping ditolak saat deploy (`IO_MAPPING_NOT_SUPPORTED`) sampai Fase 7 |
| 9 | Satu tabel `element_instance` dengan indeks parsial |
| 10 | Fitur `process-instance` minimal di modeler-web |
| 11 | Create instance "with result" ditunda ke Fase 4 |
| 12 | Default `retries` job = 3 |
| 13 | Kondisi bernilai `null` menghasilkan incident `CONDITION_ERROR` |

### 11.2 Pertanyaan dan rekomendasi

| # | Pertanyaan | Rekomendasi | Alasan |
|---|---|---|---|
| 1 | Allowlist kernel di `archtest` untuk `domain` (pertanyaan 9.1 #1 Fase 1) | **Ya**, tepat `libs/go/bpmn` dan `libs/go/feel`, paket `internal` kernel tetap terlarang, `TestKernelsArePure` sebagai penjaga | 3.1; tanpa allowlist, logika eksekusi terpaksa di `application` atau menduplikasi model BPMN |
| 2 | Persistensi Fase 2: in-memory + snapshot, atau langsung record log? | **Record log append-only + proyeksi dalam satu transaksi** | Fase 3 tidak menulis ulang; determinisme bisa diuji sejak sekarang |
| 3 | Single writer: row lock atau actor/partisi in-memory? | **Row lock `FOR UPDATE` per instance** di Fase 2; partisi ditinjau Fase 3 berdasarkan benchmark | Benar untuk multi-replika tanpa infrastruktur tambahan; domain tidak bergantung pada pilihan ini |
| 4 | Kondisi sebelum FEEL: subset atau tolak? | **Subset FEEL di `libs/go/feel`**, di luar subset ditolak saat deploy | 3.4 |
| 5 | `conditionExpression` tanpa prefix `=` | **Tolak saat deploy** (`EXPRESSION_NOT_SUPPORTED`) sampai perilaku Camunda 8.8 diverifikasi lewat paritas | Menghindari menebak semantik "bukan FEEL" |
| 6 | Kepemilikan job: `execution` atau modul `job`? | **`execution`** memiliki state job; modul `job` Fase 4 = protokol worker di atas `execution/api` | Atomisitas penyelesaian job dan kelanjutan token tanpa transaksi lintas modul |
| 7 | `state` job di proto: string atau enum? | **Enum di Fase 4** saat semua state diketahui; Fase 2 string | Menghindari enum yang berubah dua kali; `JobInfo` belum dipakai klien eksternal |
| 8 | I/O mapping di Fase 2 | **Tolak saat deploy** (`IO_MAPPING_NOT_SUPPORTED`) sampai Fase 7 | Butuh FEEL penuh dan scope lokal; diam-diam mengabaikan mapping lebih berbahaya |
| 9 | Simpan element instance final di tabel runtime atau tabel history? | **Satu tabel + indeks parsial** di Fase 2; pemisahan diputuskan Fase 3/6 | Sederhana; query runtime tetap murah |
| 10 | UI di Fase 2 atau tunda ke Fase 6? | **Fitur minimal di modeler-web** (8.14), dipindah ke operate-web di Fase 6 | Melihat token di diagram mempercepat debugging engine; biaya ±2 hari; lapisan domain/infrastructure bisa dipindah utuh |
| 11 | Create instance "with result" (`awaitCompletion`) | **Tunda ke Fase 4** | Butuh mekanisme menunggu lintas transaksi (event bus + timeout); tidak dibutuhkan untuk DoD |
| 12 | Default `retries` job bila atribut kosong | **3**, dikunci setelah verifikasi Camunda 8.8 | Nilai yang umum dipakai Zeebe; diverifikasi lewat paritas |
| 13 | Perlakuan kondisi bernilai `null` | **Incident `CONDITION_ERROR`** kecuali paritas membuktikan Camunda 8.8 menganggapnya `false` | Gagal keras lebih aman daripada mengambil cabang yang salah |

## 12. Estimasi dan pembagian kerja

Perkiraan **16–19 hari kerja** untuk 1 engineer, atau 10–11 hari untuk 2 engineer paralel:

| Hari | Fokus |
|---|---|
| 1 | 8.1 prasyarat, 8.2 allowlist `archtest` + ADR-0010 |
| 2–4 | 8.3 `libs/go/feel` subset + fuzz |
| 3–5 | 8.4 domain state/record/`Apply` (paralel dengan feel, evaluator palsu) |
| 6–8 | 8.5 `Processor`: start/end/task/service task/exclusive/parallel, cancel, batas |
| 9–10 | 8.6 inclusive join, 8.7 property test |
| 10–11 | 8.9 proto, 8.10 migrasi + sqlc + repository + proyektor |
| 12–13 | 8.8 application, 8.11 interfaces + wiring, 6.2 perubahan `deployment` |
| 14 | 8.12 skenario penuh |
| 15–16 | 8.13 paritas live + perbaikan deviasi |
| 17–18 | 8.14 modeler-web `process-instance` |
| 19 | 8.15 benchmark, ADR-0012 sampai ADR-0014, review DoD |

Pembagian jika 2 orang: **A** = `archtest`, domain (`Apply`, `Processor`, gateway), property test,
lalu paritas; **B** = `libs/go/feel`, proto, skema + sqlc + repository + proyektor, application,
interfaces, modeler-web. Titik temu: 8.8 (application memakai domain A dan infrastruktur B).

## 13. Keluaran Fase 2

Setelah fase ini selesai, Fase 3 dan seterusnya mendapat:

- `execution_record` append-only per instance (urutan `sequence` tanpa celah, `position` global) dan
  `domain.Apply` yang deterministik: Fase 3 cukup menambah rebuild proyeksi dari record, verifikasi
  saat start, outbox, dan idempotensi.
- `projector` yang memetakan setiap `(ValueType, Intent)` ke tabel state, dipakai ulang untuk replay.
- `domain.Processor` dengan antrean run-to-completion dan perilaku elemen terpisah per kategori:
  Fase 5 menambah timer/message/boundary/sub-process sebagai perilaku baru dan membuka elemennya di
  `ProfileExecutable`.
- `execution/api.Jobs` dan tabel `job` dengan indeks `job_activatable_idx`: Fase 4 menambah aktivasi
  `SKIP LOCKED`, `FailJob`, retry, dan `ThrowError` ke `JobService` yang sudah ada.
- `libs/go/feel` dengan API `Compile`/`Evaluate` yang stabil: Fase 7 memperluas grammar dan
  menghapus `UnsupportedError` tanpa mengubah pemanggil.
- Record dan event `execution.*` sebagai sumber data Operate (Fase 6); fitur `process-instance` di
  modeler-web sebagai titik awal Operate.
- Korpus skenario eksekusi + `expectations.json` paritas yang dijalankan ulang setiap fase.
