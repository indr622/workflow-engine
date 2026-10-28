# Fase 4 — API dan Job Worker

**Proyek:** Workflow Engine (kompatibel konsep Camunda 8 / BPMN 2.0)
**Stack:** Go (engine utama) + React (UI) dalam monorepo Nx
**Status:** Sedang dieksekusi di branch `feat/phase-4-job-worker`
**Tanggal:** 2026-10-28

---

## 0. Status eksekusi

| Langkah | Status | Bukti |
|---|---|---|
| 13.1 Prasyarat dan branch | Selesai | Branch `feat/phase-4-job-worker` dari `main` 3ef985f; keputusan penyesuaian di 16.1 |
| 13.2 Migrasi `00008_job_worker.sql` | Selesai | `make migrate-up` dan `make migrate-redo` bersih |
| 13.3 Proto dan generate | Selesai | `buf lint`, `buf breaking` terhadap `main` hijau; stub Go/TS di-commit; typecheck modeler-web hijau |
| 13.4 Domain `execution` | Selesai | Fail/ThrowError/UpdateRetries/Resolve/SetVariables, user task job-based; test domain + replay lolos; paritas deploy fixture v28 direkam |
| 13.5 Aktivasi, lease, proyektor | Selesai | 20 worker × 2.000 job: setiap job tepat satu kali; indeks `job_activatable_idx` dipakai; rebuild mempertahankan kolom turunan record |
| 13.6 Application dan api `execution` | Selesai | Use case + `CreateProcessInstanceWithResult` (waiter + polling 500 ms) diuji integrasi |
| 13.7 Platform `rpc` dan `sweeper` | Selesai | Interceptor (recover, logging, timeout, authn anonim, protovalidate), `ErrorInfo`; perakitan dipindah ke `internal/app` |
| 13.8 Modul `job` | Selesai | Long polling (notifier, polling cadangan, batas, shutdown) diuji unit; e2e: bangun < 1 s setelah job dibuat |
| 13.9 Modul `message` | Selesai | Duplikat → `ALREADY_EXISTS` + `message_key`; TTL 0 tidak disimpan; sweeper menghapus kedaluwarsa |
| 13.10 Handler incident/variables/result | Selesai | e2e `apps/engine/internal/e2e` untuk setiap RPC dan reason utama |
| 13.11–13.12 SDK Go + `sdktest` | Selesai | Coverage 91,3 %; `deps_test.go` hijau |
| 13.13 E2E SDK | Selesai | 40 instance × 50 cabang, 8 worker: selesai 9,4 s, `activation_count` = 1 untuk semua job |
| 13.14 worker-example, `make demo` | Selesai | `make demo`: 10 instance, 10 retry, 298 ms; container profil `demo` juga lolos |
| 13.15 Client TypeScript | Selesai | Vitest, typecheck, lint api-client dan modeler-web hijau |
| 13.16 Paritas suite `worker` | Belum | Lanjut berikutnya; Camunda 8.8 referensi (profil `camunda`) sudah bisa dinyalakan |
| 13.17 Benchmark, ADR-0020..0023, dokumentasi | Belum | |

Kondisi awal di bawah ditulis saat Fase 1 masih berjalan; keputusan 16.1 menyesuaikannya dengan
Fase 2 dan 3 yang sudah di-merge (key UUIDv7, outbox, idempotensi `request_id`, nomor migrasi dan
ADR).

**Kondisi repo yang menjadi titik awal (dicek 2026-10-28, Fase 1 sedang dieksekusi; Fase 2 dan 3
masih draft):**

| Area | Kondisi / asumsi dari fase sebelumnya | Dampak ke Fase 4 |
|---|---|---|
| `apps/engine` | Platform (`config`, `logger`, `db` + goose, `httpserver` dengan CORS, `eventbus` in-memory) dan modul `health`; modul `deployment` (Fase 1) dan `execution` (Fase 2) menyusul | Modul baru `job` dan `message`; interceptor Connect lintas modul lahir di `internal/platform/rpc` (bagian 8.5) |
| `apps/engine/cmd/engine/main.go` | Composition root manual; `connect` handler dipasang langsung ke `http.ServeMux` tanpa interceptor | Fase 4 menambah rantai interceptor (validasi, auth placeholder, logging, timeout) di satu tempat |
| Modul `execution` (Fase 2) | Memiliki tabel `job`, `incident`, `variable`; `execution/api.Jobs` dengan `Complete`; job hanya `CREATED`/`COMPLETED`/`CANCELED`; `CompleteJob` diterima pada job `CREATED`; event pasca-commit `execution.job_created`, `execution.incident_created`; indeks `job_activatable_idx` disiapkan untuk `SKIP LOCKED` | Fase 4 **memperluas** `execution/api.Jobs` (aktivasi, fail, error, retries) dan menambah `Incidents`, `Variables`; modul `job` adalah lapisan protokol worker di atasnya (keputusan 11.2 #6 Fase 2) |
| Fase 3 (persistensi, pemulihan) | Replay proyeksi dari `execution_record`, outbox transaksional, idempotensi `request_id` untuk `CreateProcessInstance` | Lease aktivasi job sengaja **di luar** record log (3.2); long polling bersandar pada outbox bila sudah ada, kalau belum pada polling berkala |
| `libs/proto` | `workflow/health/v1`; Fase 1 `workflow/deployment/v1` (`DeploymentRejection` sebagai detail error); Fase 2 `workflow/process/v1`, `workflow/job/v1` (`JobService.ListJobs`, `CompleteJob`) | Fase 4 menambah RPC ke `JobService`, `ProcessInstanceService`; paket baru `workflow/common/v1`, `workflow/incident/v1`, `workflow/message/v1` |
| `buf.yaml` | lint `STANDARD`, breaking `FILE` terhadap `main` | Kebijakan versi API (8.6) memakai pemeriksaan yang sudah ada |
| `libs/go/api` | Stub Go di-commit (`gen/`), proyek Nx `api-go` | Tidak berubah selain hasil generate |
| `libs/go/sdk`, `apps/worker-example` | Folder penanda dengan README "Dibuat di Fase 4" | Diisi di fase ini (bagian 9) |
| `libs/web/api-client` | `transport.ts` hanya `createHealthClient`; stub TS di `src/gen` di-commit; proyek Nx tanpa target | Ditambah factory client per service dan helper error (bagian 10); target `test`/`typecheck` ditambahkan |
| `.golangci.yml` / `archtest` | depguard `domain-purity`; Fase 2 menambah allowlist kernel dan deny `libs/go/sdk` untuk domain | `libs/go/sdk` wajib hanya meng-import `libs/go/api`, `connectrpc.com/connect`, stdlib (ditegakkan `deps_test.go`); engine **tidak** meng-import `libs/go/sdk` kecuali di test e2e |
| Compose | `infra/docker/docker-compose.yml` (profil `tools`, `observability`); profil `camunda` dengan `camunda/zeebe:8.8.44` direncanakan Fase 1 | Profil `demo` (worker-example) ditambahkan; paritas memakai profil `camunda` |
| `go.mod` | Satu module root, Go 1.27, `connectrpc.com/connect v1.21.1`, pgx v5 | Dependensi baru: `github.com/bufbuild/protovalidate-go` (engine saja, bukan SDK) |

## 1. Lingkup

Fase 4 membuat engine **bisa dipakai dari luar proses**: job worker nyata mengaktifkan job secara
batch dengan long polling, menyelesaikan, menggagalkan (dengan retry dan backoff), atau melempar
BPMN error; incident dapat diselesaikan; variabel dapat diubah; message dapat dipublikasikan dan
di-buffer. Semua lewat kontrak Connect yang sama untuk Go dan TypeScript, dengan SDK Go yang
menyembunyikan protokol worker, dan API yang sudah diperkeras (validasi, batas ukuran, timeout,
detail error terstruktur) agar Fase 6 (UI) dan Fase 8 (auth) tinggal menambah, bukan membongkar.

Termasuk dalam Fase 4:

1. **Protokol job worker** di `JobService`: `ActivateJobs` (batch, lock duration, long polling,
   filter variabel), `CompleteJob` (diperluas), `FailJob` (retries, backoff, pesan), `ThrowError`
   (BPMN error code), `UpdateJobRetries`, `UpdateJobTimeout`; siklus hidup job lengkap dan sweeper
   job yang kedaluwarsa.
2. **Modul engine `job`** — lima lapisan: lapisan protokol worker (long polling, notifier, sweeper)
   di atas `execution/api`; **tidak** memiliki tabel sendiri.
3. **Perluasan modul `execution`**: command domain `FailJob`, `ThrowError`, `UpdateJobRetries`,
   `ResolveIncident`, `SetVariables`; aktivasi batch `SELECT ... FOR UPDATE SKIP LOCKED`; lease job;
   incident `JOB_NO_RETRIES` dan `UNHANDLED_ERROR_EVENT`.
4. **`IncidentService`** (`ResolveIncident`, `ListIncidents`) dan `ProcessInstanceService.SetVariables`,
   serta `CreateProcessInstanceWithResult` (ditunda dari Fase 2, pertanyaan 11.2 #11).
5. **Modul engine `message`** dengan `MessageService.PublishMessage`: buffer message dengan TTL dan
   dedup `message_id`; korelasi ke subscription tetap Fase 5 (keputusan 3.3).
6. **User task berbasis job worker** (user task **tanpa** `zeebe:userTask`) dieksekusi seperti
   service task dengan tipe job `io.camunda.zeebe:userTask`; user task native (`zeebe:userTask`)
   tetap ditolak sampai Fase 6 (keputusan 3.4).
7. **API hardening**: `connect.WithReadMaxBytes`, timeout per RPC, validasi request dengan
   protovalidate, detail error `workflow.common.v1.ErrorInfo`, konvensi paginasi, kebijakan versi
   API, seam interceptor auth (anonim di Fase 4, OIDC di Fase 8).
8. **SDK Go `libs/go/sdk`**: client Connect dengan retry/backoff dan hook token, `JobWorker` builder,
   helper variabel JSON, `sdktest`; **`apps/worker-example`** sebagai demo end-to-end terhadap
   Compose.
9. **Client TypeScript** `libs/web/api-client` untuk semua service (generated) + helper error.
10. **Strategi uji**: unit semantik kueri aktivasi, integration N worker konkuren tanpa aktivasi
    ganda, reaktivasi setelah timeout, SDK terhadap engine in-process, paritas Camunda 8.8.

**Tidak termasuk** (fase lain): korelasi message ke subscription, message start event, timer,
boundary error event yang menangkap `ThrowError` (Fase 5; Fase 4 hanya mencatat dan membuat
incident); user task native dengan Tasklist API (assign/claim/form) (Fase 6); Operate (Fase 6);
I/O mapping dan scope variabel lokal (Fase 7); OIDC, otorisasi, multi-tenant di API (Fase 8);
`StreamActivatedJobs` server-streaming (pertanyaan terbuka 16.2 #1); SDK non-Go; publikasi SDK
sebagai module terpisah; partisi/pembagian beban antar replika engine (Fase 3/8).

## 2. Definition of Done Fase 4

- [ ] Modul `job` dan `message` lima lapisan lolos `archtest`; `job` tidak memiliki tabel; `message` memiliki tabel `message` sendiri; keduanya hanya meng-import `execution/api` / `deployment/api`.
- [ ] `ActivateJobs(type, max_jobs_to_activate, timeout, worker, fetch_variable, request_timeout)` mengembalikan ≤ `max` job bertipe `type` dalam state `CREATED` yang `activatable_at <= now()`, mengubahnya ke `ACTIVATED` dengan `deadline = now + timeout` dan `worker`, menyertakan variabel scope process instance (semua atau subset `fetch_variable`), header, retries, dan key konteks.
- [ ] Long polling: request dengan `request_timeout > 0` dan tidak ada job menunggu sampai job bertipe itu dibuat (dibangunkan ≤ 100 ms setelah commit pada replika yang sama; ≤ interval polling pada replika lain) atau sampai timeout, lalu mengembalikan daftar kosong, bukan error.
- [ ] Integration test: 20 worker konkuren (goroutine, pool koneksi terpisah) mengaktifkan 2.000 job bertipe sama; setiap job diaktifkan **tepat satu kali** sampai kedaluwarsa; tidak ada deadlock; tidak ada job tertinggal `CREATED` dengan `activatable_at` lampau.
- [ ] Sweeper: job `ACTIVATED` yang `deadline` lewat kembali `CREATED` ≤ 2 × interval sweep (default 1 s) dan dapat diaktifkan lagi; `CompleteJob` dari worker lama pada job yang sudah diaktifkan worker lain **tetap diterima** jika job masih `ACTIVATED`/`CREATED` (semantik Zeebe, verifikasi terhadap dokumentasi Camunda 8.8) dan ditolak `FailedPrecondition` bila sudah final.
- [ ] `FailJob(retries > 0, retry_back_off)` mengembalikan job ke `CREATED` dengan `activatable_at = now + backoff` dan record `JOB FAILED`; `FailJob(retries = 0)` membuat incident `JOB_NO_RETRIES` dengan `error_message` worker, job `FAILED`, element instance tetap `ACTIVATED`, `process_instance.has_incident = true`.
- [ ] `ThrowError(error_code)` pada job aktif: record `JOB ERROR_THROWN`, job `ERROR_THROWN`, incident `UNHANDLED_ERROR_EVENT` (karena belum ada catch di Fase 4) dengan `error_code` di pesan; Fase 5 menyisipkan pencarian catch event **sebelum** pembuatan incident tanpa mengubah kontrak API.
- [ ] `UpdateJobRetries` pada job `FAILED` lalu `ResolveIncident` membuat job dapat diaktifkan lagi dan incident `RESOLVED`; `ResolveIncident` pada incident `CONDITION_ERROR`/`EXTRACT_VALUE_ERROR` setelah `SetVariables` mengeksekusi ulang langkah yang gagal dan instance berlanjut; resolusi tanpa perbaikan membuat incident baru (bukan error RPC).
- [ ] `UpdateJobTimeout` memperpanjang/memperpendek `deadline` job `ACTIVATED`; ditolak `FailedPrecondition` pada state lain.
- [ ] `SetVariables(scope_key = process_instance_key atau element instance aktif, local = false)` menulis variabel ke scope process instance dengan record `VARIABLE CREATED/UPDATED`; `local = true` pada scope non-root → `Unimplemented` dengan `ErrorInfo.reason = LOCAL_SCOPE_NOT_SUPPORTED`.
- [ ] `PublishMessage(name, correlation_key, ttl, message_id, variables)` menyimpan message ke tabel `message` dengan `expires_at`; `message_id` yang sama dalam masa hidup → `AlreadyExists`; TTL 0 → diterima, tidak di-buffer; sweeper menghapus message kedaluwarsa; `message_key` dikembalikan.
- [ ] `CreateProcessInstanceWithResult` menunggu instance selesai (sampai `request_timeout`, default 10 s) dan mengembalikan variabel akhir; timeout → `DeadlineExceeded` dengan `process_instance_key` di `ErrorInfo.metadata` (instance tetap berjalan).
- [ ] User task tanpa `zeebe:userTask` lolos `ProfileExecutable` dan membuat job bertipe `io.camunda.zeebe:userTask` dengan header assignee/candidate groups/candidate users/form key/due date (nama header diverifikasi terhadap dokumentasi Camunda 8.8); user task dengan `zeebe:userTask` tetap `UNSUPPORTED_ELEMENT` dengan pesan yang menunjuk Fase 6.
- [ ] Setiap request divalidasi protovalidate di interceptor; pelanggaran → `InvalidArgument` + detail `ErrorInfo{reason: "REQUEST_INVALID"}` + daftar pelanggaran per field; semua handler memakai `connect.WithReadMaxBytes` (16 MiB) dan timeout default 30 s (ActivateJobs: `request_timeout + 5 s`, maksimum 60 s).
- [ ] Semua error domain yang dipetakan ke Connect membawa `workflow.common.v1.ErrorInfo` dengan `reason` stabil (tabel 8.4); `DeploymentRejection` Fase 1 tetap sebagai detail tambahan.
- [ ] Interceptor auth placeholder memasang `Principal{Subject: "anonymous"}` ke context setiap request; SDK mengirim `Authorization: Bearer` bila `TokenSource` diberi; tidak ada endpoint yang menolak request tanpa token di Fase 4 (dibuktikan test).
- [ ] `buf breaking` terhadap `main` hijau untuk seluruh Fase 4 (hanya penambahan); `JobInfo.state` string Fase 2 dipertahankan dan field baru `JobState state_enum` ditambahkan (bukan mengganti).
- [ ] `libs/go/sdk`: `Client` (semua RPC Fase 1–4), `JobWorker` builder dengan concurrency, poll interval, backoff kosong/error, graceful shutdown; `deps_test.go` memastikan hanya `libs/go/api`, `connectrpc.com/connect`, `google.golang.org/protobuf`, stdlib; coverage paket ≥ 80 %; godoc contoh untuk `NewClient`, `NewJobWorker`, `Variables.Unmarshal`.
- [ ] `sdktest.FakeEngine` (in-memory `JobService`+`ProcessInstanceService` minimal) memungkinkan unit test handler worker tanpa Postgres; test e2e `apps/engine/internal/e2e` menjalankan engine in-process (httptest + Postgres) dan SDK worker nyata dengan `-tags=integration`.
- [ ] `apps/worker-example`: `make demo` (engine + Postgres dari Compose, worker dari host) men-deploy `order-process.bpmn`, memulai 10 instance, tiga worker menyelesaikan semuanya termasuk satu retry yang disengaja; keluaran log menampilkan ringkasan; selesai < 30 s.
- [ ] `libs/web/api-client` mengekspor `createJobClient`, `createProcessInstanceClient`, `createIncidentClient`, `createMessageClient`, `createDeploymentClient`, helper `findErrorInfo(err)`; Vitest untuk helper; `typecheck` hijau.
- [ ] Uji paritas suite `worker` (bagian 12): semua skenario bertanda `parity` memberi hasil observable yang sama dengan Camunda 8.8 referensi; deviasi tercatat di `expectations.json`.
- [ ] Benchmark awal: aktivasi + complete 5.000 job/detik agregat dengan 8 worker × 32 job per batch di mesin dev dengan Postgres lokal (angka awal, dikunci setelah pengukuran pertama); latensi `ActivateJobs` saat ada job < 10 ms p50.
- [ ] ADR-0017 sampai ADR-0020 tersimpan di `docs/adr/`; `docs/plan/README.md` menautkan dokumen ini; README `libs/go/sdk` dan `apps/worker-example` berisi quickstart.

## 3. Keputusan teknis

| Area | Keputusan | Alasan singkat |
|---|---|---|
| Peran modul `job` | **Lapisan protokol worker** di atas `execution/api`: long polling, notifier per tipe job, sweeper deadline, klem parameter (timeout, max jobs); **tanpa tabel sendiri** | Menjalankan keputusan Fase 2 (job milik `execution` demi atomisitas dengan token); modul `job` berisi hal yang tidak ada hubungannya dengan token: menunggu, membangunkan, kedaluwarsa |
| Aktivasi | `SELECT ... FOR UPDATE SKIP LOCKED` + `UPDATE ... RETURNING` dalam satu transaksi di `execution/infrastructure`, **tanpa** row lock instance (5.2) | Aktivasi batch lintas instance tidak boleh mengunci puluhan instance; `SKIP LOCKED` tidak pernah menunggu sehingga bebas deadlock terhadap command instance |
| Lease di luar record log | `ACTIVATED`/`TIMED_OUT` adalah **lease** (kolom `worker`, `deadline`, `activated_at` di `job`), bukan record `execution_record`; yang masuk record hanya `FAILED`, `ERROR_THROWN`, `RETRIES_UPDATED`, `COMPLETED`, `CANCELED` (ADR-0017) | Record per instance bernomor urut dan butuh lock instance; lease adalah state transien at-least-once. Setelah replay Fase 3 job kembali `CREATED`, dan `CompleteJob` memang diterima pada `CREATED` (perilaku Fase 2 untuk UI) sehingga worker yang sedang memegang job tetap bisa menyelesaikannya |
| Mode pengambilan job | **Long polling unary** `ActivateJobs` (ADR-0017); `StreamActivatedJobs` ditunda | Semantik sama dengan gateway Zeebe klasik; bekerja lewat proxy/nginx HTTP/1.1 dan Connect-web tanpa konfigurasi streaming; SDK bisa menambah streaming tanpa mengubah handler |
| Notifier long polling | Subscriber event bus `execution.job_created` → channel per `type` di modul `job` + **polling cadangan** setiap `JOB_LONG_POLL_INTERVAL` (default 1 s) | Event in-process hanya membangunkan replika yang sama; polling cadangan menutupi replika lain, backoff yang berakhir, dan job yang kembali dari timeout |
| Sweeper | Satu `internal/platform/sweeper` (ticker + `SKIP LOCKED` batch) dipakai `job` (deadline) dan `message` (TTL); interval default 1 s, batch 500 | Satu mekanisme, satu metrik, aman dijalankan di banyak replika |
| Retries | Worker **menetapkan** `retries` tersisa di `FailJob` (SDK default `job.retries - 1`), engine tidak mengurangi sendiri; `retries = 0` → incident `JOB_NO_RETRIES` | Sama dengan Zeebe (verifikasi); memberi worker kendali penuh |
| Backoff | `retry_back_off` (durasi) di `FailJob` → `activatable_at`; dibatasi `JOB_MAX_BACKOFF` (default 24 jam) | Sesuai Zeebe 8.x (verifikasi nama field `retryBackOff`); batas mencegah job "hilang" setahun |
| `ThrowError` | Dicatat `ERROR_THROWN`; Fase 4 **selalu** incident `UNHANDLED_ERROR_EVENT`; pencarian catch disisipkan Fase 5 | Kontrak API lengkap sekarang; perilaku Fase 4 = Zeebe tanpa boundary event (verifikasi nama error type) |
| Resolusi incident | `ResolveIncident` adalah command `Processor`: incident → `RESOLVED`, lalu **langkah yang gagal dijalankan ulang** dari state elemen (gateway `ACTIVATING` → evaluasi ulang; service task `ACTIVATING` → evaluasi ulang type/retries; job `FAILED` → `CREATED` bila retries > 0, kalau tidak incident baru) | Sama pola Zeebe (resolve = retry); tidak perlu menyimpan command di incident karena state elemen cukup menentukan langkah |
| `SetVariables` | RPC di `ProcessInstanceService`; scope = `process_instance_key` atau element instance aktif; `local = false` saja di Fase 4 | Hanya ada scope root sampai Fase 7, sehingga propagasi ke atas identik dengan menulis ke root; `local = true` → `Unimplemented` daripada semantik palsu |
| `PublishMessage` | **Dikerjakan di Fase 4**: modul `message` dengan tabel `message` (buffer + TTL + dedup `message_id`); korelasi Fase 5 (3.3, ADR-0019) | SDK, client TS, dan paritas mendapat permukaan API lengkap; buffering tidak bergantung pada korelasi; Fase 5 hanya menambah tabel `message_subscription` dan korelasi saat publish/subscribe |
| User task | **Job-based di Fase 4** (tanpa `zeebe:userTask`, tipe job `io.camunda.zeebe:userTask`); **native Fase 6** dengan modul `usertask` (3.4, ADR-0018) | Nol logika baru di engine (perilaku service task); Fase 6 butuh lifecycle assign/claim/form yang milik Tasklist |
| Validasi request | **protovalidate** (`buf.build/bufbuild/protovalidate` sebagai dep buf; `protovalidate-go` di interceptor engine); SDK tidak memvalidasi | Aturan hidup di `.proto` sebagai dokumentasi + dipakai Go dan (opsional) TS; menghapus validasi tangan di tiap handler Fase 1–2 |
| Detail error | `workflow.common.v1.ErrorInfo{reason, domain, metadata}` (cermin `google.rpc.ErrorInfo`, tanpa dependensi googleapis) dilampirkan di **setiap** error non-`Internal`; `DeploymentRejection` tetap | Kode `reason` stabil untuk SDK/UI; `google.rpc` tidak di-generate ke TS dengan `include_imports: false` |
| Timeout RPC | Interceptor memasang `context.WithTimeout` default 30 s per RPC (`RPC_DEFAULT_TIMEOUT`), `ActivateJobs` = `request_timeout + 5 s` maks 60 s; header `Connect-Timeout-Ms` klien dihormati bila lebih kecil | Mencegah handler menggantung; long polling adalah satu-satunya RPC panjang |
| Auth seam | Interceptor `authn` di `internal/platform/rpc`: Fase 4 `authn.Anonymous` → `Principal` di context; `Principal` dibaca use case lewat `authn.FromContext` (ADR-0020) | Fase 8 mengganti satu implementasi (verifier OIDC) tanpa menyentuh handler |
| Versi API | Paket `v1` **beta sampai akhir Fase 6**: hanya penambahan (`buf breaking FILE`), field tidak dinomori ulang, penghapusan lewat `deprecated = true` minimal dua fase; perubahan tak kompatibel = paket `v2` berdampingan (8.6) | Klien eksternal pertama (SDK, worker-example, UI) lahir sekarang; biaya breaking mulai nyata |
| SDK | `libs/go/sdk` di module root (ADR-0002); dependensi hanya `libs/go/api`, `connect`, `protobuf`, stdlib; API `v0` (boleh berubah) sampai Fase 6; publikasi sebagai module terpisah dengan tag `libs/go/sdk/v0.x` ditinjau Fase 8 | Satu versi dependensi; SDK bisa dipindah tanpa mengubah import path pengguna di dalam repo |
| `CreateProcessInstanceWithResult` | Dikerjakan di Fase 4 di modul `execution/application` memakai `Waiter` (subscriber `execution.process_instance_completed/terminated` + polling cadangan 500 ms) | Ditunda dari Fase 2; dibutuhkan paritas (`awaitCompletion`) dan demo; polling cadangan membuatnya benar lintas replika |
| Proto state job | `JobInfo.state` (string Fase 2) dipertahankan; tambah `enum JobState` di field baru; `ActivatedJob` memakai enum | Menjawab pertanyaan 11.2 #7 Fase 2 tanpa breaking |

### 3.1 Tata letak modul baru dan perluasan

```
apps/engine/internal/modules/job/
├─ api/
│  └─ api.go                 # (kosong secara fungsional; modul lain tidak memanggil job) — dipertahankan demi pola lima paket
├─ domain/
│  ├─ activation.go          # ActivationRequest (type, worker, max, timeout, fetchVariables, requestTimeout), Clamp(limits)
│  ├─ limits.go              # Limits: MaxJobs, MinTimeout, MaxTimeout, MaxRequestTimeout, MaxBackoff
│  └─ errors.go              # ErrInvalidActivation
├─ application/
│  ├─ activate_jobs.go       # loop long polling: coba → tunggu notifier/ticker → coba lagi → kembali
│  ├─ update_job_timeout.go
│  └─ sweep_timeouts.go      # dipanggil sweeper: execution/api.Jobs.ReleaseExpiredLeases
├─ infrastructure/
│  ├─ notifier.go            # Subscribe("execution.job_created") → map[type]chan struct{} (coalescing)
│  └─ sweeper_task.go        # adapter ke internal/platform/sweeper
├─ interfaces/
│  └─ job_handler.go         # JobService (dipindah dari execution/interfaces Fase 2): ActivateJobs, CompleteJob, FailJob,
│                            # ThrowError, UpdateJobRetries, UpdateJobTimeout, ListJobs
└─ README.md

apps/engine/internal/modules/message/
├─ api/
│  ├─ api.go                 # Messages: Publish(ctx, PublishCommand) (MessageKey, error); Fase 5 menambah Correlate
│  └─ events.go              # MessagePublished (Fase 5 berlangganan)
├─ domain/
│  ├─ message.go             # Message{Key, Name, CorrelationKey, MessageID, TTL, Variables, PublishedAt, ExpiresAt}
│  ├─ policy.go              # aturan: name wajib, TTL ≥ 0, TTL 0 = tidak di-buffer, ukuran variabel
│  └─ ports.go               # Repository: Insert (unik name+correlation_key+message_id aktif), DeleteExpired
├─ application/
│  ├─ publish_message.go
│  └─ sweep_expired.go
├─ infrastructure/
│  ├─ sqlc.yaml, queries/message.sql, db/
│  └─ postgres_repository.go
└─ interfaces/
   └─ message_handler.go     # MessageService.PublishMessage

apps/engine/internal/modules/execution/            # perluasan Fase 4
├─ api/api.go                # Jobs += ActivateBatch, Fail, ThrowError, UpdateRetries, ExtendLease, ReleaseExpiredLeases
│                            # Incidents (Resolve, List); Variables (Set); ProcessInstances += CreateWithResult
├─ domain/command.go         # + FailJob, ThrowError, UpdateJobRetries, ResolveIncident, SetVariables
├─ domain/behavior_task.go   # + user task job-based (header dari zeebe:assignmentDefinition/formDefinition/taskSchedule)
├─ domain/incident.go        # + JOB_NO_RETRIES, UNHANDLED_ERROR_EVENT; aturan "langkah ulang" per tipe
├─ application/              # + fail_job.go, throw_error.go, update_job_retries.go, resolve_incident.go,
│                            #   set_variables.go, create_instance_with_result.go, waiter.go
├─ infrastructure/           # + activation.go (SKIP LOCKED), lease.go, queries/job_activation.sql
└─ interfaces/               # process_instance_handler += SetVariables, CreateProcessInstanceWithResult;
                             # incident_handler.go (IncidentService); job_handler.go DIPINDAH ke modul job

apps/engine/internal/platform/
├─ rpc/                      # interceptor: Validate (protovalidate), Authn (placeholder), Timeout, Logging, Recover;
│                            # ErrorInfo helper (rpc.Reject(code, reason, msg, meta))
└─ sweeper/                  # Runner: setiap interval, jalankan setiap Task(ctx, batch) sampai mengembalikan 0

libs/go/sdk/                 # bagian 9
apps/worker-example/         # bagian 9.7
```

Handler `JobService` berpindah dari `execution/interfaces` (Fase 2) ke `job/interfaces` karena satu
service Connect hanya boleh punya satu handler, dan `ActivateJobs` membutuhkan notifier dan klem
parameter milik modul `job`. `CompleteJob`, `FailJob`, `ThrowError`, `UpdateJobRetries` di handler
itu adalah panggilan tipis ke `execution/api.Jobs`. Jika Fase 2 belum di-merge saat Fase 4 mulai,
handler langsung dibuat di `job/interfaces`.

### 3.2 Siklus hidup job dan lease

```
                 ActivateJobs (lease)                CompleteJob
  CREATED ─────────────────────────────► ACTIVATED ─────────────► COMPLETED
    ▲  ▲                                   │  │  │
    │  │ sweeper: deadline lewat           │  │  └─ ThrowError ──► ERROR_THROWN ──(incident UNHANDLED_ERROR_EVENT)
    │  └───────────────────────────────────┘  │
    │        FailJob retries > 0 (activatable_at = now + backoff)
    └─────────────────────────────────────────┘
                                              └─ FailJob retries = 0 ──► FAILED ──(incident JOB_NO_RETRIES)
                                                                            │
                                   UpdateJobRetries(n>0) + ResolveIncident  │
  CREATED ◄─────────────────────────────────────────────────────────────────┘

  CancelProcessInstance dari state non-final mana pun ──► CANCELED
  CompleteJob juga diterima dari CREATED (UI Fase 2, worker setelah replay).
```

| State | Di `execution_record`? | Kolom lease | Dapat diaktifkan? |
|---|---|---|---|
| `CREATED` | ya (`JOB CREATED`, Fase 2) | `worker NULL`, `deadline NULL`, `activatable_at` | ya, bila `activatable_at <= now()` |
| `ACTIVATED` | **tidak** (lease) | `worker`, `deadline`, `activated_at`, `activation_count += 1` | tidak |
| `COMPLETED` | ya | — | tidak |
| `FAILED` | ya (`JOB FAILED` dengan `retries = 0`) | dibersihkan | tidak, sampai `UpdateJobRetries` + `ResolveIncident` |
| `ERROR_THROWN` | ya | dibersihkan | tidak |
| `CANCELED` | ya | dibersihkan | tidak |

Record `JOB FAILED` dengan `retries > 0` **tidak** mengubah state (tetap `CREATED` setelah lease
dibersihkan) tetapi mencatat `retries`, `error_message`, dan `activatable_at`; replay Fase 3
memulihkan ketiganya. `RETRIES_UPDATED` mencatat retries baru. `TIMED_OUT` tidak dicatat sebagai
record; sweeper menaikkan metrik `job_timeouts_total` dan log level debug (ADR-0017 mencatat
deviasi ini dari Zeebe, yang menulis `TIMED_OUT` ke log; hasil observable untuk worker sama).

### 3.3 Penempatan `PublishMessage` (ADR-0019)

| Opsi | Kelebihan | Kekurangan |
|---|---|---|
| A. Tunda seluruhnya ke Fase 5 | Tidak ada kode "setengah" | Roadmap menyebut publish message di Fase 4; SDK/TS/paritas Fase 4 tidak lengkap; Fase 5 harus mengerjakan modul, proto, SDK, dan korelasi sekaligus |
| **B. RPC + buffer + TTL + dedup di Fase 4, korelasi Fase 5** | Permukaan API dan SDK lengkap; semantik buffer (TTL, `message_id` unik, `AlreadyExists`) diuji terpisah dari korelasi; Fase 5 hanya menambah `message_subscription` dan dua titik korelasi (saat publish, saat subscribe) | Message yang dipublikasikan di Fase 4 tidak pernah dikorelasikan — hanya kedaluwarsa (didokumentasikan) |
| C. RPC yang mengembalikan `Unimplemented` | Kontrak ada | Tidak menguji apa pun |

**Dipilih B.** Perilaku observable di Fase 4 sengaja sama dengan Zeebe untuk proses tanpa
subscription: message dengan TTL > 0 di-buffer sampai kedaluwarsa; TTL 0 diterima dan langsung
dibuang; `message_id` ganda dalam masa hidup → ditolak `AlreadyExists` (verifikasi kode penolakan
Camunda 8.8 untuk duplikat `messageId`). Modul `message` tidak meng-import `execution` di Fase 4.

### 3.4 User task: job-based di Fase 4, native di Fase 6 (ADR-0018)

| Opsi | Kelebihan | Kekurangan |
|---|---|---|
| A. Semua user task ditunda ke Fase 6 | Fokus | Model dengan user task tetap ditolak selama dua fase; tidak ada paritas user task sampai Fase 6 |
| **B. Job-based di Fase 4, native di Fase 6** | Nol perilaku baru di `Processor` (user task tanpa `zeebe:userTask` = service task dengan tipe tetap dan header); worker Go bisa menjadi "tasklist" sederhana; paritas job-based terhadap Camunda 8.8 (verifikasi: Camunda 8.8 masih mendukung user task berbasis job worker dengan tipe `io.camunda.zeebe:userTask`) | Camunda Modeler terbaru membuat user task native secara default, sehingga sebagian besar model baru tetap ditolak sampai Fase 6 |
| C. Native di Fase 4 | Modeler default langsung jalan | Butuh lifecycle (`CREATING/CREATED/ASSIGNING/UPDATING/COMPLETING`, listener), assignment, form, Tasklist API: bukan "API dan job worker", tapi setengah Fase 6 |

**Dipilih B.** Aturan: `ProfileExecutable` menerima `userTask` **tanpa** `zeebe:userTask`;
perilaku = service task dengan `type = "io.camunda.zeebe:userTask"`, `retries` default, header
`io.camunda.zeebe:assignee`, `io.camunda.zeebe:candidateGroups`, `io.camunda.zeebe:candidateUsers`,
`io.camunda.zeebe:formKey`, `io.camunda.zeebe:dueDate`, `io.camunda.zeebe:followUpDate` (nilai
statis disalin apa adanya; ekspresi FEEL dievaluasi dengan subset Fase 2, di luar subset ditolak
saat deploy). Nama header persis **harus diverifikasi terhadap dokumentasi Camunda 8.8**. User task
dengan `zeebe:userTask` ditolak `UNSUPPORTED_ELEMENT` dengan pesan "native user task tersedia di
Fase 6". Fase 6 menambah modul `usertask` dan membuka `zeebe:userTask` tanpa mengubah jalur
job-based.

### 3.5 Validasi dengan protovalidate

Pilihan: validasi tangan di tiap handler (Fase 1–2) vs anotasi `buf.validate` di proto +
interceptor. **Dipilih protovalidate** karena aturan menjadi bagian kontrak yang terbaca di `.proto`
dan di dokumentasi hasil generate, satu interceptor menggantikan kode berulang, dan UI dapat
memakai anotasi yang sama (protovalidate-es, opsional, tidak di DoD). Dampak:

- `libs/proto/buf.yaml` menambah `deps: [buf.build/bufbuild/protovalidate]`; `buf dep update`
  menghasilkan `buf.lock` (di-commit). `buf.gen.yaml` tetap `include_imports: false`; stub TS tidak
  membutuhkan runtime validate.
- Proto Fase 1–2 **diberi anotasi** (bukan perubahan wire): `process_instance_key > 0`,
  `page_size <= 500`, `variables` JSON object (CEL `this == '' || this.startsWith('{')` sebagai
  pra-cek murah; validasi JSON penuh tetap di use case), panjang `type` ≤ 256, dll.
- Interceptor `rpc.Validate` dipasang sebelum `Authn`; pelanggaran → `InvalidArgument` +
  `ErrorInfo{reason: "REQUEST_INVALID", metadata: {"violations": "<field>: <msg>; ..."}}` plus
  detail `buf.validate.Violations` bila tersedia di versi library (verifikasi API protovalidate-go).
- SDK **tidak** memvalidasi di klien (menghindari dependensi protovalidate di SDK); error validasi
  dari server sudah deskriptif.

## 4. Protokol job worker

### 4.1 Parameter `ActivateJobs` dan klem server

| Parameter request | Semantik | Klem (`job/domain.Limits`, env) |
|---|---|---|
| `type` | Tipe job (`zeebe:taskDefinition type` yang sudah dievaluasi) | wajib, ≤ 256 byte |
| `worker` | Nama worker untuk observability dan kolom `job.worker`; tidak dipakai untuk otorisasi completion (verifikasi: Zeebe juga tidak mencocokkan worker saat complete) | default `"default"`, ≤ 128 byte |
| `timeout` | Durasi lease: job kembali `CREATED` bila tidak selesai sebelum `activated_at + timeout` | default 5 menit; min `JOB_MIN_TIMEOUT` 1 s; maks `JOB_MAX_TIMEOUT` 24 jam |
| `max_jobs_to_activate` | Batas job dalam satu respons | default 32; maks `JOB_MAX_ACTIVATE` 1.000 |
| `fetch_variable` | Daftar nama variabel yang disertakan; kosong = semua variabel scope process instance | ≤ 256 nama |
| `request_timeout` | Batas long polling; 0 = jawab segera (satu kali kueri) | maks `JOB_MAX_REQUEST_TIMEOUT` 55 s (di bawah timeout RPC 60 s) |

Klem dilakukan di `job/domain.ActivationRequest.Clamp(limits)` dan **tidak** menolak request
(nilai di luar batas dipotong, dicatat di log level debug), kecuali `type` kosong dan
`max_jobs_to_activate < 0` yang sudah ditolak protovalidate. Alasan: worker SDK lama yang memakai
nilai default besar tidak boleh gagal total setelah server menurunkan batas.

### 4.2 Alur `ActivateJobs` (long polling)

```
Worker                      job/application.ActivateJobs            execution/api.Jobs (Postgres)
  |  ActivateJobs(type=T,     |                                        |
  |   max=32, timeout=30s,    |                                        |
  |   request_timeout=10s)    |                                        |
  |-------------------------->| clamp                                  |
  |                           | subscribe notifier[T] (sebelum kueri)  |
  |                           |--- ActivateBatch(T, 32, worker, 30s) ->| tx: SELECT..SKIP LOCKED; UPDATE..RETURNING;
  |                           |                                        |     SELECT variable (filter); commit
  |                           |<---------- []ActivatedJob (0) ---------|
  |                           | loop sampai ctx/deadline:              |
  |                           |   select { notifier[T], ticker 1s, ctx }
  |                           |   |--- ActivateBatch ----------------->|
  |                           |   |<-- []ActivatedJob (n>0) -----------|
  |<-- ActivateJobsResponse --| unsubscribe                            |
  |      {jobs: n}            |                                        |
```

Aturan:

1. Notifier didaftarkan **sebelum** kueri pertama agar job yang dibuat di antara kueri dan tunggu
   tidak terlewat (channel berkapasitas 1, coalescing).
2. Setiap siklus tunggu bangun oleh salah satu: notifikasi tipe `T`, ticker polling cadangan
   (`JOB_LONG_POLL_INTERVAL`, default 1 s), context dibatalkan (klien putus → berhenti tanpa
   kueri lagi), atau `request_timeout` habis → respons kosong.
3. Satu request hanya mengembalikan **satu batch**: begitu `n > 0`, respons dikirim. Tidak ada
   penggabungan beberapa batch untuk mendekati `max`.
4. Saat engine shutdown (`ctx` server dibatalkan), semua long polling dijawab kosong segera
   sehingga worker SDK mengulang ke replika lain.
5. Jumlah long polling aktif per engine dibatasi `JOB_MAX_LONG_POLLING` (default 1.024); di atas
   itu request dijawab segera (satu kueri saja), bukan ditolak.

### 4.3 `CompleteJob`, `FailJob`, `ThrowError`, `UpdateJobRetries`, `UpdateJobTimeout`

Semua kecuali `UpdateJobTimeout` adalah command `Processor` dalam transaksi berlock instance
(alur Fase 2 bagian 3.3: key instance dibaca dari `job`, instance dikunci, job dibaca ulang).

| RPC | Prasyarat state job | Efek (record) | Penolakan |
|---|---|---|---|
| `CompleteJob(variables)` | `CREATED` atau `ACTIVATED` | `VARIABLE *`, `JOB COMPLETED`, elemen `COMPLETING → COMPLETED`, token lanjut (Fase 2) | final → `FailedPrecondition` `JOB_NOT_ACTIVE`; tidak ada → `NotFound` `JOB_NOT_FOUND` |
| `FailJob(retries, retry_back_off, error_message, variables)` | `CREATED` atau `ACTIVATED` | `VARIABLE *` (bila `variables` diberi; verifikasi dukungan variabel pada fail di Camunda 8.8), `JOB FAILED{retries, error_message, activatable_at}`; `retries > 0` → state `CREATED`, lease dibersihkan; `retries = 0` → state `FAILED`, `INCIDENT CREATED{JOB_NO_RETRIES, error_message}`, `has_incident = true` | sama dengan complete; `retries < 0` → `InvalidArgument` |
| `ThrowError(error_code, error_message, variables)` | `CREATED` atau `ACTIVATED` | `JOB ERROR_THROWN{error_code}`; Fase 4: `INCIDENT CREATED{UNHANDLED_ERROR_EVENT, "no catch event found for error code 'X'"}`; Fase 5: cari boundary/event sub-process **sebelum** incident | sama; `error_code` kosong → `InvalidArgument` |
| `UpdateJobRetries(retries)` | bukan final | `JOB RETRIES_UPDATED{retries}`; **tidak** menyelesaikan incident (butuh `ResolveIncident` terpisah, sama dengan Zeebe — verifikasi) | `retries <= 0` → `InvalidArgument` |
| `UpdateJobTimeout(timeout)` | `ACTIVATED` | lease: `deadline = now + timeout` (tanpa record; tanpa lock instance) | state lain → `FailedPrecondition` `JOB_NOT_ACTIVATED` |

Pembersihan lease saat `FailJob`/`ThrowError`/`CompleteJob`/`CANCELED` dilakukan oleh proyektor
(`projector.go`) pada record yang bersangkutan: `worker = NULL, deadline = NULL`.

### 4.4 Sweeper deadline

```sql
-- name: ReleaseExpiredLeases :many
WITH expired AS (
    SELECT key FROM job
    WHERE state = 'ACTIVATED' AND deadline < now()
    ORDER BY deadline
    LIMIT $1
    FOR UPDATE SKIP LOCKED
)
UPDATE job j
SET state = 'CREATED', worker = NULL, deadline = NULL, timeout_count = timeout_count + 1,
    updated_at = now()
FROM expired
WHERE j.key = expired.key
RETURNING j.key, j.type, j.process_instance_key;
```

Sweeper (`internal/platform/sweeper`) memanggil task ini setiap `JOB_TIMEOUT_SWEEP_INTERVAL`
(default 1 s) dengan batch 500 sampai mengembalikan 0 baris, lalu memberi tahu notifier per `type`
agar long polling yang menunggu bangun. Dijalankan di setiap replika; `SKIP LOCKED` membuatnya
aman. Jitter ±20 % mencegah sinkronisasi antar replika. Jam server Postgres (`now()`) menjadi
acuan tunggal untuk `deadline` dan `activatable_at` sehingga replika dengan jam berbeda tidak
salah mengembalikan lease.

### 4.5 Urutan interaksi: fail → retry → incident → resolve

```
Worker A        Engine (JobService / IncidentService)              DB
  | ActivateJobs(T)                                                  |
  |-----------------------------------------------------------------> lease job 1001 (retries 3)
  | FailJob(1001, retries=2, backoff=2s, "timeout upstream")          |
  |-----------------------------------------------------------------> record FAILED; CREATED; activatable_at=+2s
  | ActivateJobs(T, request_timeout=10s)  [menunggu ~2s: ticker]      |
  |<----------------------------------------------------------------- lease job 1001 (retries 2)
  | FailJob(1001, retries=0, "permanent")                             |
  |-----------------------------------------------------------------> record FAILED; state FAILED; INCIDENT JOB_NO_RETRIES
Operator (UI / sdk)                                                   |
  | UpdateJobRetries(1001, 3)                                         |
  |-----------------------------------------------------------------> record RETRIES_UPDATED
  | ResolveIncident(inc 2001)                                         |
  |-----------------------------------------------------------------> INCIDENT RESOLVED; job -> CREATED (retries 3)
Worker A                                                              |
  | ActivateJobs(T) <------------------------------------------------- lease job 1001
  | CompleteJob(1001, {"ok":true})                                    |
  |-----------------------------------------------------------------> COMPLETED; token lanjut
```

## 5. Persistensi: aktivasi, lease, indeks

### 5.1 Migrasi `00005_job_worker.sql` (nomor final mengikuti urutan saat merge)

```sql
-- +goose Up
ALTER TABLE job
    ADD COLUMN activatable_at   TIMESTAMPTZ NOT NULL DEFAULT now(),   -- backoff setelah FailJob
    ADD COLUMN worker           TEXT,
    ADD COLUMN deadline         TIMESTAMPTZ,                          -- lease; NULL bila tidak ACTIVATED
    ADD COLUMN activated_at     TIMESTAMPTZ,
    ADD COLUMN activation_count INT NOT NULL DEFAULT 0,
    ADD COLUMN timeout_count    INT NOT NULL DEFAULT 0,
    ADD COLUMN error_message    TEXT,                                 -- FailJob terakhir
    ADD COLUMN error_code       TEXT;                                 -- ThrowError

-- Daftar state eksplisit (Fase 2 tidak memakai CHECK untuk job).
ALTER TABLE job ADD CONSTRAINT job_state_check
    CHECK (state IN ('CREATED', 'ACTIVATED', 'COMPLETED', 'FAILED', 'ERROR_THROWN', 'CANCELED'));
ALTER TABLE job ADD CONSTRAINT job_lease_check
    CHECK ((state = 'ACTIVATED') = (deadline IS NOT NULL));

DROP INDEX job_activatable_idx;
CREATE INDEX job_activatable_idx ON job (type, activatable_at, key) WHERE state = 'CREATED';
CREATE INDEX job_deadline_idx    ON job (deadline)                  WHERE state = 'ACTIVATED';
CREATE INDEX job_by_pi_idx       ON job (process_instance_key, key);

CREATE TABLE message (
    key              BIGINT PRIMARY KEY,
    tenant_id        TEXT NOT NULL DEFAULT '<default>',
    name             TEXT NOT NULL,
    correlation_key  TEXT NOT NULL,
    message_id       TEXT,                                  -- NULL = tanpa dedup
    variables        TEXT NOT NULL DEFAULT '{}',            -- JSON kanonik (konsisten dengan tabel variable)
    published_at     TIMESTAMPTZ NOT NULL,
    expires_at       TIMESTAMPTZ NOT NULL                   -- = published_at + ttl; TTL 0 tidak disimpan
);
CREATE UNIQUE INDEX message_dedup_idx ON message (tenant_id, name, correlation_key, message_id)
    WHERE message_id IS NOT NULL;
CREATE INDEX message_correlate_idx ON message (tenant_id, name, correlation_key, published_at);  -- Fase 5
CREATE INDEX message_expires_idx   ON message (expires_at);

-- +goose Down
DROP TABLE message;
DROP INDEX job_by_pi_idx;
DROP INDEX job_deadline_idx;
DROP INDEX job_activatable_idx;
CREATE INDEX job_activatable_idx ON job (type, key) WHERE state = 'CREATED';
ALTER TABLE job DROP CONSTRAINT job_lease_check, DROP CONSTRAINT job_state_check;
ALTER TABLE job DROP COLUMN activatable_at, DROP COLUMN worker, DROP COLUMN deadline,
    DROP COLUMN activated_at, DROP COLUMN activation_count, DROP COLUMN timeout_count,
    DROP COLUMN error_message, DROP COLUMN error_code;
```

Dedup `message_id` memakai unique index parsial; message kedaluwarsa dihapus sweeper sehingga
`message_id` yang sama dapat dipakai lagi setelah TTL (verifikasi semantik Camunda 8.8: dedup
berlaku selama message masih di buffer). Bila Fase 3 mengubah `job` (mis. partisi), kolom lease
mengikuti.

### 5.2 Kueri aktivasi

```sql
-- name: ActivateJobs :many
WITH picked AS (
    SELECT key
    FROM job
    WHERE type = $1
      AND state = 'CREATED'
      AND activatable_at <= now()
    ORDER BY activatable_at, key                 -- FIFO: backoff lebih awal dulu, lalu key naik
    LIMIT $2
    FOR UPDATE SKIP LOCKED
)
UPDATE job j
SET state = 'ACTIVATED',
    worker = $3,
    deadline = now() + $4::interval,
    activated_at = now(),
    activation_count = j.activation_count + 1,
    updated_at = now()
FROM picked
WHERE j.key = picked.key
RETURNING j.key, j.type, j.process_instance_key, j.element_instance_key, j.element_id,
          j.bpmn_process_id, j.process_definition_key, j.process_definition_version,
          j.retries, j.custom_headers, j.deadline, j.worker;

-- name: VariablesForInstances :many
SELECT process_instance_key, name, value
FROM variable
WHERE scope_key = ANY($1::bigint[])              -- Fase 4: scope root = process_instance_key
  AND ($2::text[] IS NULL OR name = ANY($2))
ORDER BY process_instance_key, name;
```

Properti yang diuji (bagian 11.1):

- **Tepat satu kali**: dua transaksi konkuren tidak pernah mengembalikan key yang sama; baris yang
  sedang dikunci transaksi lain dilewati, bukan ditunggu.
- **Tidak menunggu lock instance**: `CompleteJob` yang sedang memegang lock instance dan baris job
  tidak memblokir aktivasi (job itu dilewati). Urutan lock: command instance = `process_instance`
  → `job`; aktivasi = `job` saja tanpa menunggu → tidak ada siklus.
- **Terlihat oleh proyektor**: `CompleteJob` membaca job ulang di bawah lock instance dan menerima
  `ACTIVATED` maupun `CREATED`.
- **Index path**: `EXPLAIN` menunjukkan `Index Scan` pada `job_activatable_idx` dengan `LIMIT`;
  tidak ada `Seq Scan` pada tabel `job` berisi 1 juta baris (benchmark 13.17).

Catatan Postgres: `FOR UPDATE SKIP LOCKED` dengan `ORDER BY` + `LIMIT` di CTE lalu `UPDATE` adalah
pola antrean standar; `now()` konstan dalam satu transaksi sehingga `deadline` seragam per batch.
Pada Postgres 18, verifikasi tidak ada perubahan perilaku `SKIP LOCKED` terhadap indeks parsial.

### 5.3 Keadilan (fairness) dan starvation

| Kasus | Perilaku |
|---|---|
| Banyak worker, satu tipe | FIFO global menurut `(activatable_at, key)`; setiap request mengambil ≤ `max`; worker cepat mendapat lebih banyak (sesuai kapasitas), tidak ada round-robin eksplisit |
| Job retry vs job baru | Job yang backoff-nya berakhir memiliki `activatable_at` lampau → diprioritaskan sebelum job baru; verifikasi perilaku Zeebe (job retry masuk antrean activatable yang sama) |
| Satu instance dengan 1.000 job paralel | Tidak ada batas per instance; job diambil FIFO. Dicatat sebagai pertanyaan 16.2 #5 (batas per instance) |
| Worker memegang lease lalu mati | Job kembali setelah `timeout`; worker SDK default `timeout` 5 menit — pengguna menyetel sesuai durasi kerja; `UpdateJobTimeout` untuk kerja panjang |
| Satu worker meminta `max` besar | Dibatasi `JOB_MAX_ACTIVATE`; SDK default `MaxJobsActive` 32 per worker |

### 5.4 Perubahan `execution/api`

```go
package api

type ActivationQuery struct {
	Type           string
	Worker         string
	MaxJobs        int32
	Timeout        time.Duration
	FetchVariables []string // nil = semua
}

type ActivatedJob struct {
	Key, ProcessInstanceKey, ElementInstanceKey, ProcessDefinitionKey int64
	ElementID, BPMNProcessID, Type, Worker                            string
	ProcessDefinitionVersion                                         int32
	Retries                                                          int32
	CustomHeaders                                                    map[string]string
	Variables                                                        json.RawMessage // JSON object
	Deadline                                                         time.Time
}

type JobFailure struct {
	JobKey       int64
	Retries      int32
	RetryBackoff time.Duration
	ErrorMessage string
	Variables    json.RawMessage
}

type JobError struct {
	JobKey                  int64
	ErrorCode, ErrorMessage string
	Variables               json.RawMessage
}

type Jobs interface {
	ActivateBatch(ctx context.Context, q ActivationQuery) ([]ActivatedJob, error) // tanpa lock instance
	Complete(ctx context.Context, jobKey int64, variables json.RawMessage) error
	Fail(ctx context.Context, f JobFailure) error
	ThrowError(ctx context.Context, e JobError) error
	UpdateRetries(ctx context.Context, jobKey int64, retries int32) error
	ExtendLease(ctx context.Context, jobKey int64, timeout time.Duration) error
	ReleaseExpiredLeases(ctx context.Context, limit int32) ([]ReleasedJob, error) // dipanggil sweeper modul job
	List(ctx context.Context, f JobFilter, page Page) (JobPage, error)
}

type Incidents interface {
	Resolve(ctx context.Context, incidentKey int64) error
	List(ctx context.Context, f IncidentFilter, page Page) (IncidentPage, error)
}

type Variables interface {
	Set(ctx context.Context, scopeKey int64, variables json.RawMessage, local bool) error
}

// Perluasan ProcessInstances Fase 2.
type ProcessInstances interface {
	// ...Create, Get, List, Cancel (Fase 2)
	CreateWithResult(ctx context.Context, cmd CreateInstance, fetchVariables []string, timeout time.Duration) (InstanceResult, error)
}
```

## 6. Resolusi incident dan `SetVariables`

### 6.1 Aturan "langkah ulang" per tipe incident

`ResolveIncident` adalah command `Processor` (`domain/command.go`): memvalidasi incident `CREATED`,
mengeluarkan `INCIDENT RESOLVED`, lalu memasukkan ke antrean kerja internal langkah ulang sesuai
tipe dan state elemen; selanjutnya run-to-completion seperti biasa. Jika langkah ulang gagal lagi,
incident **baru** dibuat (key baru) — RPC tetap sukses, sama dengan Zeebe (verifikasi).

| Tipe incident | State elemen saat gagal | Langkah ulang |
|---|---|---|
| `CONDITION_ERROR` | Gateway `ACTIVATING`/`ACTIVATED` (sesuai verifikasi Fase 2) | Evaluasi ulang outgoing gateway dengan variabel saat ini |
| `EXTRACT_VALUE_ERROR` | Service task / user task `ACTIVATING` | Evaluasi ulang `type`/`retries`/header → buat job |
| `EXECUTION_LIMIT_EXCEEDED` | Elemen `ACTIVATING` | Aktifkan ulang elemen (batas berlaku lagi) |
| `JOB_NO_RETRIES` | Service task `ACTIVATED`, job `FAILED` | Jika `job.retries > 0` (sudah `UpdateJobRetries`): job → `CREATED`, `activatable_at = now`; jika masih 0: incident baru `JOB_NO_RETRIES` ("job has no retries left") — verifikasi perilaku Zeebe 8.8 |
| `UNHANDLED_ERROR_EVENT` | Service task `ACTIVATED`, job `ERROR_THROWN` | Fase 4: ulangi pencarian catch → tidak ada → incident baru. Didokumentasikan: satu-satunya jalan keluar di Fase 4 adalah `CancelProcessInstance`; verifikasi perilaku Zeebe untuk resolve pada unhandled error |

`Processor` cukup membaca state elemen dan job untuk menentukan langkah; incident tidak perlu
menyimpan "command yang gagal". `has_incident` instance dihitung ulang proyektor (`EXISTS incident
CREATED`).

### 6.2 `SetVariables`

- `scope_key` harus `process_instance_key` **atau** key element instance non-final milik instance
  itu; selain itu `NotFound` `SCOPE_NOT_FOUND`.
- `local = false` (default): variabel ditulis ke scope root (satu-satunya scope sampai Fase 7)
  dengan record `VARIABLE CREATED/UPDATED` di bawah lock instance; semantik identik dengan Zeebe
  "propagate ke scope tempat variabel ada, else root" karena tidak ada scope lokal.
- `local = true` pada `scope_key = process_instance_key` sama dengan `false`; pada element instance
  → `Unimplemented` `LOCAL_SCOPE_NOT_SUPPORTED` (Fase 7 membuka).
- Instance final → `FailedPrecondition` `PROCESS_INSTANCE_NOT_ACTIVE`. Ukuran ≤
  `ENGINE_MAX_VARIABLES_BYTES` (Fase 2).

### 6.3 `CreateProcessInstanceWithResult`

Use case `execution/application.create_instance_with_result.go`: `Waiter` mendaftar ke event bus
untuk `execution.process_instance_completed/terminated` dengan key instance **sebelum** memanggil
`Create` (instance tanpa wait state selesai dalam panggilan yang sama; hasil langsung dikembalikan
tanpa menunggu). Jika belum selesai: tunggu event, polling cadangan `GetProcessInstance` setiap
500 ms (replika lain), atau `request_timeout` (default 10 s, maks 55 s) → `DeadlineExceeded`
`RESULT_TIMEOUT` dengan `metadata.process_instance_key`; instance **tidak** dibatalkan. Respons:
state akhir, variabel akhir (atau subset `fetch_variables`). `TERMINATED` → respons sukses dengan
state `TERMINATED` (verifikasi: apa yang dikembalikan Camunda 8.8 saat instance dibatalkan sebelum
selesai).

## 7. Modul `message`: `PublishMessage`

| Field | Aturan Fase 4 |
|---|---|
| `name` | wajib, ≤ 256 byte |
| `correlation_key` | string (boleh kosong: Zeebe mengizinkan untuk message start event; verifikasi) |
| `time_to_live` | durasi ≥ 0; default **0** mengikuti Zeebe (verifikasi default `timeToLive`); TTL 0 → message diterima, `message_key` dikembalikan, tidak disimpan; maks `MESSAGE_MAX_TTL` (default 30 hari) |
| `message_id` | opsional; bila ada dan masih ada message aktif dengan `(name, correlation_key, message_id)` sama → `AlreadyExists` `MESSAGE_DUPLICATE` |
| `variables` | JSON object ≤ `ENGINE_MAX_VARIABLES_BYTES` |

Alur: validasi → `keygen` → `INSERT` (unique index menangkap duplikat secara atomik) → commit →
publish event `message.published` (Fase 5 berlangganan untuk korelasi; Fase 4 tanpa subscriber) →
respons `{message_key}`. Sweeper `DeleteExpiredMessages` (batch 500, `SKIP LOCKED`). Modul tidak
meng-import `execution`; Fase 5 menambah `message_subscription` dan use case korelasi di modul yang
sama, memakai `execution/api` untuk melanjutkan token.

## 8. Kontrak proto

Semua perubahan **aditif** terhadap Fase 1–2. Anotasi `buf.validate` ditampilkan hanya pada contoh
pertama; aturan lengkap ada di file proto.

### 8.1 `workflow/common/v1/error.proto` (baru)

```proto
syntax = "proto3";
package workflow.common.v1;
option go_package = "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/common/v1;commonv1";

// ErrorInfo dilampirkan sebagai detail error Connect pada setiap penolakan non-Internal.
// Cermin google.rpc.ErrorInfo tanpa dependensi googleapis (tidak di-generate ke TS).
message ErrorInfo {
  string reason = 1;              // kode stabil SCREAMING_SNAKE, lihat tabel 8.4
  string domain = 2;              // "workflow-engine"
  map<string, string> metadata = 3;  // mis. job_key, process_instance_key, state
}

// Page adalah konvensi paginasi keyset bersama (8.5).
message PageRequest {
  int32 page_size = 1;            // default 50, maks 500
  string page_token = 2;          // opaque; kosong = halaman pertama
}
message PageResponse {
  string next_page_token = 1;     // kosong = akhir
}
```

RPC Fase 1–2 yang sudah memakai `page_size`/`page_token` langsung di request **tidak diubah**;
`PageRequest` dipakai service baru (`IncidentService`, `MessageService` bila kelak ada `List`).

### 8.2 `workflow/job/v1/job.proto` (diperluas)

```proto
syntax = "proto3";
package workflow.job.v1;
option go_package = "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1;jobv1";

import "buf/validate/validate.proto";
import "google/protobuf/duration.proto";
import "google/protobuf/timestamp.proto";

service JobService {
  rpc ListJobs(ListJobsRequest) returns (ListJobsResponse) {}            // Fase 2
  rpc CompleteJob(CompleteJobRequest) returns (CompleteJobResponse) {}   // Fase 2
  // Fase 4
  rpc ActivateJobs(ActivateJobsRequest) returns (ActivateJobsResponse) {}
  rpc FailJob(FailJobRequest) returns (FailJobResponse) {}
  rpc ThrowError(ThrowErrorRequest) returns (ThrowErrorResponse) {}
  rpc UpdateJobRetries(UpdateJobRetriesRequest) returns (UpdateJobRetriesResponse) {}
  rpc UpdateJobTimeout(UpdateJobTimeoutRequest) returns (UpdateJobTimeoutResponse) {}
}

message ActivateJobsRequest {
  string type = 1 [(buf.validate.field).string = {min_len: 1, max_len: 256}];
  string worker = 2 [(buf.validate.field).string.max_len = 128];
  google.protobuf.Duration timeout = 3;                 // lease; 0 = default server (5m)
  int32 max_jobs_to_activate = 4 [(buf.validate.field).int32.gte = 0];  // 0 = default server (32)
  repeated string fetch_variable = 5 [(buf.validate.field).repeated.max_items = 256];
  google.protobuf.Duration request_timeout = 6;         // long polling; 0 = jawab segera
}

message ActivateJobsResponse {
  repeated ActivatedJob jobs = 1;
}

message ActivatedJob {
  int64 job_key = 1;
  string type = 2;
  int64 process_instance_key = 3;
  string bpmn_process_id = 4;
  int32 process_definition_version = 5;
  int64 process_definition_key = 6;
  string element_id = 7;
  int64 element_instance_key = 8;
  map<string, string> custom_headers = 9;
  string worker = 10;
  int32 retries = 11;
  google.protobuf.Timestamp deadline = 12;
  string variables = 13;                                // JSON object
  string tenant_id = 14;                                // "<default>" sampai Fase 8
}

message FailJobRequest {
  int64 job_key = 1 [(buf.validate.field).int64.gt = 0];
  int32 retries = 2 [(buf.validate.field).int32.gte = 0];
  string error_message = 3 [(buf.validate.field).string.max_len = 4096];
  google.protobuf.Duration retry_back_off = 4;
  string variables = 5;                                 // JSON object; kosong = tidak ada
}
message FailJobResponse {}

message ThrowErrorRequest {
  int64 job_key = 1;
  string error_code = 2 [(buf.validate.field).string = {min_len: 1, max_len: 256}];
  string error_message = 3;
  string variables = 4;
}
message ThrowErrorResponse {}

message UpdateJobRetriesRequest {
  int64 job_key = 1;
  int32 retries = 2 [(buf.validate.field).int32.gt = 0];
}
message UpdateJobRetriesResponse {}

message UpdateJobTimeoutRequest {
  int64 job_key = 1;
  google.protobuf.Duration timeout = 2;                 // deadline baru = now + timeout
}
message UpdateJobTimeoutResponse {}

enum JobState {
  JOB_STATE_UNSPECIFIED = 0;
  JOB_STATE_CREATED = 1;
  JOB_STATE_ACTIVATED = 2;
  JOB_STATE_COMPLETED = 3;
  JOB_STATE_FAILED = 4;
  JOB_STATE_ERROR_THROWN = 5;
  JOB_STATE_CANCELED = 6;
}

// JobInfo Fase 2 dipertahankan; field baru ditambah (string state = 9 tetap diisi).
message JobInfo {
  // ...field 1–9 Fase 2...
  JobState state_enum = 10;
  string worker = 11;
  google.protobuf.Timestamp deadline = 12;
  google.protobuf.Timestamp activatable_at = 13;
  string error_message = 14;
  string error_code = 15;
  int32 activation_count = 16;
}

message ListJobsRequest {
  // ...field 1–5 Fase 2...
  JobState state = 6;        // filter tambahan; UNSPECIFIED = semua (only_open tetap berlaku)
}
```

`CompleteJobRequest` Fase 2 tidak berubah. `google.protobuf.Duration` dipilih (bukan `int64 ms`)
agar TS menerima objek `Duration` dari protobuf-es dan validasi rentang berada di server.

### 8.3 `workflow/process/v1` (tambahan), `workflow/incident/v1`, `workflow/message/v1`

```proto
// workflow/process/v1/process_instance.proto — tambahan
service ProcessInstanceService {
  // ...Fase 2...
  rpc CreateProcessInstanceWithResult(CreateProcessInstanceWithResultRequest) returns (CreateProcessInstanceWithResultResponse) {}
  rpc SetVariables(SetVariablesRequest) returns (SetVariablesResponse) {}
}

message CreateProcessInstanceWithResultRequest {
  CreateProcessInstanceRequest request = 1;
  google.protobuf.Duration request_timeout = 2;   // default 10s, maks 55s
  repeated string fetch_variables = 3;            // kosong = semua
}
message CreateProcessInstanceWithResultResponse {
  int64 process_instance_key = 1;
  int64 process_definition_key = 2;
  string bpmn_process_id = 3;
  int32 version = 4;
  ProcessInstanceState state = 5;                 // COMPLETED atau TERMINATED
  string variables = 6;                           // JSON object
}

message SetVariablesRequest {
  int64 scope_key = 1;                            // process_instance_key atau element_instance_key aktif
  string variables = 2;                           // JSON object, wajib non-kosong
  bool local = 3;                                 // Fase 4: hanya false (atau true pada scope root)
}
message SetVariablesResponse {}
```

```proto
// workflow/incident/v1/incident.proto (baru; handler di modul execution)
syntax = "proto3";
package workflow.incident.v1;
option go_package = "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/incident/v1;incidentv1";

import "google/protobuf/timestamp.proto";
import "workflow/common/v1/error.proto";

service IncidentService {
  rpc ResolveIncident(ResolveIncidentRequest) returns (ResolveIncidentResponse) {}
  rpc ListIncidents(ListIncidentsRequest) returns (ListIncidentsResponse) {}
}

message ResolveIncidentRequest { int64 incident_key = 1; }
message ResolveIncidentResponse {}

message ListIncidentsRequest {
  int64 process_instance_key = 1;      // 0 = semua
  string error_type = 2;               // filter opsional
  bool only_open = 3;
  workflow.common.v1.PageRequest page = 4;
}
message ListIncidentsResponse {
  repeated Incident incidents = 1;
  workflow.common.v1.PageResponse page = 2;
}

message Incident {
  int64 incident_key = 1;
  int64 process_instance_key = 2;
  int64 element_instance_key = 3;
  string element_id = 4;
  string bpmn_process_id = 5;
  int64 process_definition_key = 6;
  string error_type = 7;               // CONDITION_ERROR, EXTRACT_VALUE_ERROR, JOB_NO_RETRIES, UNHANDLED_ERROR_EVENT, EXECUTION_LIMIT_EXCEEDED
  string error_message = 8;
  int64 job_key = 9;                   // 0 bila bukan incident job
  IncidentState state = 10;
  google.protobuf.Timestamp created_at = 11;
  google.protobuf.Timestamp resolved_at = 12;
}
enum IncidentState {
  INCIDENT_STATE_UNSPECIFIED = 0;
  INCIDENT_STATE_CREATED = 1;
  INCIDENT_STATE_RESOLVED = 2;
}
```

```proto
// workflow/message/v1/message.proto (baru)
syntax = "proto3";
package workflow.message.v1;
option go_package = "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/message/v1;messagev1";

import "google/protobuf/duration.proto";

service MessageService {
  rpc PublishMessage(PublishMessageRequest) returns (PublishMessageResponse) {}
}

message PublishMessageRequest {
  string name = 1;
  string correlation_key = 2;
  google.protobuf.Duration time_to_live = 3;   // 0 = tidak di-buffer
  string message_id = 4;                       // opsional, dedup
  string variables = 5;                        // JSON object
}
message PublishMessageResponse {
  int64 message_key = 1;
  string tenant_id = 2;
}
```

`Incident` di `GetProcessInstanceResponse` (Fase 2, `process/v1`) tetap ada untuk kompatibilitas;
`IncidentService.ListIncidents` adalah sumber untuk Operate (Fase 6). Perbedaan dua tipe `Incident`
(process/v1 vs incident/v1) dicatat sebagai hutang kecil untuk `v2`.

### 8.4 Pemetaan error dan `ErrorInfo.reason`

| `reason` | Kode Connect | Sumber |
|---|---|---|
| `REQUEST_INVALID` | `InvalidArgument` | protovalidate; `metadata.violations` |
| `VARIABLES_INVALID` | `InvalidArgument` | JSON bukan object / terlalu besar |
| `JOB_NOT_FOUND`, `PROCESS_INSTANCE_NOT_FOUND`, `INCIDENT_NOT_FOUND`, `PROCESS_DEFINITION_NOT_FOUND`, `SCOPE_NOT_FOUND` | `NotFound` | `ErrXNotFound` domain |
| `JOB_NOT_ACTIVE` | `FailedPrecondition` | complete/fail/error pada job final; `metadata.state` |
| `JOB_NOT_ACTIVATED` | `FailedPrecondition` | `UpdateJobTimeout` pada job bukan `ACTIVATED` |
| `PROCESS_INSTANCE_NOT_ACTIVE` | `FailedPrecondition` | cancel/set variables pada instance final |
| `INCIDENT_ALREADY_RESOLVED` | `FailedPrecondition` | resolve dua kali |
| `MESSAGE_DUPLICATE` | `AlreadyExists` | `message_id` ganda; `metadata.message_key` yang ada |
| `LOCAL_SCOPE_NOT_SUPPORTED` | `Unimplemented` | `SetVariables local=true` non-root |
| `RESULT_TIMEOUT` | `DeadlineExceeded` | `CreateProcessInstanceWithResult`; `metadata.process_instance_key` |
| `DEPLOYMENT_REJECTED` | `InvalidArgument` | Fase 1; ditambah `ErrorInfo` di samping `DeploymentRejection` |
| `ENGINE_SHUTTING_DOWN` | `Unavailable` | command diterima saat shutdown; SDK retry |

Helper `rpc.Reject(code, reason, msg, metadata)` membuat `*connect.Error` dengan detail; error
tanpa pemetaan → `Internal` tanpa `ErrorInfo` dan dicatat log dengan stack. SDK memetakan `reason`
ke tipe error Go (`sdk.ErrJobNotActive`, …) lewat `errors.Is`.

### 8.5 Konvensi paginasi dan filter

- `page_size` 0 → 50; > 500 → dipotong 500 (bukan ditolak).
- `page_token` opaque: base64url dari `{"k": <key terakhir>, "d": "desc"}`; token tidak valid →
  `InvalidArgument` `PAGE_TOKEN_INVALID`. Token tidak mengandung filter; mengganti filter dengan
  token lama → hasil tidak terdefinisi tetapi tidak error (didokumentasikan).
- Urutan tetap per RPC: `ListJobs` dan `ListIncidents` `key DESC`; tidak ada parameter sort di `v1`.
- Nilai nol pada field filter = tanpa filter; filter enum `UNSPECIFIED` = tanpa filter.

### 8.6 Hardening dan kebijakan versi API (ADR-0020)

Rantai interceptor (urutan luar → dalam) di `internal/platform/rpc`, dipasang di
`cmd/engine/main.go` untuk **semua** handler lewat `connect.WithInterceptors`:

1. `Recover` — panic → `Internal`, log dengan stack.
2. `Logging` — satu baris per RPC: prosedur, kode, durasi, `principal.subject`, `request_id`
   (header `X-Request-Id` atau dibuat); level debug untuk `ActivateJobs` kosong agar long polling
   tidak membanjiri log.
3. `Timeout` — `context.WithTimeout(RPC_DEFAULT_TIMEOUT=30s)`; `ActivateJobs` dan
   `CreateProcessInstanceWithResult` memakai `request_timeout + 5 s` (maks 60 s); header
   `Connect-Timeout-Ms` dari klien tetap dihormati bila lebih kecil (connect-go memasangnya
   sendiri).
4. `Authn` — placeholder (3, ADR-0020): membaca `Authorization`, Fase 4 selalu `Anonymous`;
   context memuat `authn.Principal{Subject, Tenant: "<default>"}`.
5. `Validate` — protovalidate.

Opsi handler: `connect.WithReadMaxBytes(16 << 20)` (semua service), `connect.WithSendMaxBytes`
tidak dibatasi, kompresi gzip default connect-go. `httpserver` menambah `ReadTimeout` 65 s
(di atas long polling maksimum) dan `IdleTimeout` 120 s; nginx produksi (`infra/docker/nginx`)
menambah `proxy_read_timeout 70s` untuk `/api/`.

Kebijakan versi:

| Aturan | Penegakan |
|---|---|
| Paket `workflow.*.v1` beta sampai Fase 6 selesai: perubahan hanya aditif | `buf breaking --against main` (sudah di `proto:breaking`) wajib di CI untuk setiap PR yang menyentuh `libs/proto` |
| Field/RPC usang: `[deprecated = true]` + komentar fase penghapusan; dihapus paling cepat dua fase kemudian dan hanya di `v2` | Review PR; lint `STANDARD` |
| Enum baru selalu `*_UNSPECIFIED = 0`; klien harus memperlakukan nilai tak dikenal sebagai `UNSPECIFIED` | Konvensi SDK/TS |
| Perubahan tak kompatibel = paket `v2` berdampingan dengan `v1` minimal satu fase | ADR-0020 |
| Stub Go/TS selalu di-commit dan diperiksa sinkron | CI Fase 0 |

## 9. SDK Go `libs/go/sdk` dan `apps/worker-example`

### 9.1 Tata letak

```
libs/go/sdk/
├─ project.json              # Nx "sdk-go": build, test, lint, bench
├─ doc.go
├─ client.go                 # Client, NewClient, Option (WithBaseURL, WithHTTPClient, WithTokenSource, WithRetry, WithUserAgent)
├─ auth.go                   # TokenSource func(ctx) (string, error); interceptor Authorization: Bearer
├─ retry.go                  # interceptor retry unary idempoten (Unavailable, DeadlineExceeded jaringan) dengan backoff + jitter
├─ errors.go                 # ErrorInfo extraction; ErrJobNotActive, ErrNotFound, ...; AsErrorInfo(err)
├─ variables.go              # Variables (json.RawMessage) → Unmarshal(&T), Get(name), Marshal(map/any)
├─ deployment.go             # Deploy(ctx, Resource...) → DeployResult
├─ process.go                # CreateInstance, CreateInstanceWithResult, GetInstance, CancelInstance, SetVariables
├─ job.go                    # ActivateJobs (raw), CompleteJob, FailJob, ThrowError, UpdateJobRetries, UpdateJobTimeout
├─ incident.go               # ResolveIncident, ListIncidents
├─ message.go                # PublishMessage
├─ worker.go                 # JobWorker, NewJobWorker (builder), Start/Close, loop polling
├─ handler.go                # Handler, Job, Fail(...), BPMNError(...)
├─ backoff.go                # kebijakan backoff poll kosong / error
├─ deps_test.go              # hanya libs/go/api, connect, protobuf, stdlib
├─ sdktest/
│  ├─ fake_engine.go         # FakeEngine: JobService + ProcessInstanceService in-memory (httptest)
│  └─ handler.go             # RunHandler(t, h, job) Outcome untuk unit test handler
└─ README.md
```

### 9.2 Client

```go
package sdk

type Client struct { /* connect clients per service, opsi */ }

func NewClient(baseURL string, opts ...Option) (*Client, error)

type Option func(*options)
func WithHTTPClient(c *http.Client) Option          // default: http.Client{Timeout: 0} + h2c opsional
func WithTokenSource(ts TokenSource) Option         // Fase 8: OIDC client credentials
func WithRetry(p RetryPolicy) Option                // default: 5 percobaan, 100ms→5s, jitter, hanya Unavailable/koneksi
func WithUserAgent(ua string) Option                // "workflow-sdk-go/<version>"
func WithJSON() Option                              // protokol Connect JSON (debug); default proto biner

type TokenSource func(ctx context.Context) (string, error)

// Operasi tingkat tinggi (semua menerima ctx dan mengembalikan error ber-ErrorInfo).
func (c *Client) Deploy(ctx context.Context, resources ...Resource) (*DeployResult, error)
func (c *Client) CreateInstance(ctx context.Context, ref ProcessRef, vars any) (*InstanceRef, error)
func (c *Client) CreateInstanceWithResult(ctx context.Context, ref ProcessRef, vars any, opts ...ResultOption) (*InstanceResult, error)
func (c *Client) CompleteJob(ctx context.Context, jobKey int64, vars any) error
func (c *Client) FailJob(ctx context.Context, jobKey int64, f Failure) error
func (c *Client) ThrowError(ctx context.Context, jobKey int64, code, msg string, vars any) error
func (c *Client) PublishMessage(ctx context.Context, m Message) (int64, error)
func (c *Client) SetVariables(ctx context.Context, scopeKey int64, vars any) error
func (c *Client) ResolveIncident(ctx context.Context, incidentKey int64) error

// Akses mentah bila perlu.
func (c *Client) Jobs() jobv1connect.JobServiceClient
```

Retry hanya untuk error transport/`Unavailable` pada RPC yang aman diulang (`ActivateJobs`,
query, `PublishMessage` dengan `message_id`, `CompleteJob`/`FailJob` — ulangan menghasilkan
`FailedPrecondition` yang dipetakan sukses-idempoten **tidak** dilakukan di SDK; dicatat sebagai
pertanyaan 16.2 #6). `vars any` menerima `map[string]any`, struct, `json.RawMessage`, atau
`string` JSON; selain itu `ErrInvalidVariables`.

### 9.3 `JobWorker`

```go
type Handler func(ctx context.Context, job *Job) (Result, error)

type Job struct {
	Key, ProcessInstanceKey, ElementInstanceKey, ProcessDefinitionKey int64
	Type, ElementID, BPMNProcessID, Worker                            string
	ProcessDefinitionVersion, Retries                                 int32
	CustomHeaders                                                     map[string]string
	Variables                                                         Variables
	Deadline                                                          time.Time
}

type Result struct{ Variables any } // nil = tanpa variabel

// Nilai kembali handler menentukan RPC:
//   (Result, nil)                → CompleteJob(result.Variables)
//   (_, *FailError)              → FailJob(retries, backoff, message)         — sdk.Fail(msg, opts...)
//   (_, *BPMNError)              → ThrowError(code, message, variables)       — sdk.BPMNError(code, msg)
//   (_, error lain)              → FailJob(job.Retries-1, backoff kebijakan worker, err.Error())
//   ctx dibatalkan (shutdown)    → tidak ada RPC; job kedaluwarsa di engine

func NewJobWorker(c *Client) *WorkerBuilder
func (b *WorkerBuilder) JobType(t string) *WorkerBuilder
func (b *WorkerBuilder) Handler(h Handler) *WorkerBuilder
func (b *WorkerBuilder) Name(n string) *WorkerBuilder                 // default hostname-pid
func (b *WorkerBuilder) Concurrency(n int) *WorkerBuilder             // goroutine handler, default 8
func (b *WorkerBuilder) MaxJobsActive(n int) *WorkerBuilder           // batas job dalam proses, default 32
func (b *WorkerBuilder) Timeout(d time.Duration) *WorkerBuilder       // lease, default 5m
func (b *WorkerBuilder) RequestTimeout(d time.Duration) *WorkerBuilder // long polling, default 10s
func (b *WorkerBuilder) PollInterval(d time.Duration) *WorkerBuilder  // jeda saat batch kosong tanpa long polling, default 100ms
func (b *WorkerBuilder) FetchVariables(names ...string) *WorkerBuilder
func (b *WorkerBuilder) FailureBackoff(p BackoffPolicy) *WorkerBuilder // backoff retry default untuk error biasa
func (b *WorkerBuilder) PollBackoff(p BackoffPolicy) *WorkerBuilder    // backoff saat ActivateJobs error (100ms→30s)
func (b *WorkerBuilder) Build() (*JobWorker, error)

func (w *JobWorker) Start(ctx context.Context) error   // non-blocking; loop polling + pool handler
func (w *JobWorker) Close(ctx context.Context) error   // berhenti polling, tunggu handler aktif sampai ctx habis
```

Loop polling: `max = MaxJobsActive - inflight`; bila `max == 0` tunggu slot; `ActivateJobs(max,
RequestTimeout)`; batch kosong → `PollInterval`; error → `PollBackoff` (reset saat sukses);
`Unavailable`/koneksi tidak dianggap fatal. Graceful shutdown: `Close` menghentikan polling,
menunggu handler, lalu membatalkan context handler yang tersisa; job yang tidak selesai kembali
lewat timeout engine (tidak ada "release" eksplisit di `v1`; pertanyaan 16.2 #7).

### 9.4 Helper variabel

```go
type Variables json.RawMessage
func (v Variables) Unmarshal(dst any) error            // UseNumber agar int64 besar tidak hilang presisi
func (v Variables) Get(name string) (json.RawMessage, bool)
func (v Variables) String(name string) (string, error) // helper tipe umum: String, Int64, Float64, Bool
func MarshalVariables(v any) (string, error)           // dipakai Client untuk semua vars any
```

### 9.5 `sdktest`

- `sdktest.NewFakeEngine(t)` → `*FakeEngine` dengan `URL()`, menjalankan `JobService`,
  `ProcessInstanceService` (create/get minimal), `IncidentService` in-memory di `httptest`:
  `AddJob(type, vars, headers)`, `Jobs()` state, semantik lease/timeout/fail yang sama dengan
  engine (versi disederhanakan, tanpa Postgres). Dipakai untuk test `JobWorker` dan untuk unit
  test aplikasi pengguna.
- `sdktest.RunHandler(t, h, job) Outcome` memanggil handler secara langsung dan mengembalikan
  `Outcome{Completed, Variables, Failed, Retries, Backoff, ErrorCode}`.
- Test e2e dengan engine **nyata** in-process tidak bisa berada di `libs/go/sdk` (paket
  `apps/engine/internal` tidak dapat di-import dari luar `apps/engine`); ia berada di
  `apps/engine/internal/e2e/sdk_test.go` (`-tags=integration`), merakit engine seperti
  `main.go` lewat helper `apps/engine/internal/app.Build(cfg)` yang diekstrak dari `main.go`
  (refactor kecil, tanpa perubahan perilaku).

### 9.6 Versioning dan publikasi SDK

- Fase 4: `libs/go/sdk` tetap di module root (`github.com/indr622/workflow-engine/libs/go/sdk`),
  API `v0` (boleh berubah dengan catatan di CHANGELOG paket).
- `deps_test.go` menjamin SDK hanya bergantung pada `libs/go/api`, `connectrpc.com/connect`,
  `google.golang.org/protobuf`, stdlib → pemisahan module di masa depan tidak menyeret pgx/dll.
- Fase 8 (atau saat ada konsumen eksternal): `go.work` + module `libs/go/sdk/go.mod` dengan tag
  `libs/go/sdk/v0.x.y` (ADR-0002); import path **tidak berubah**.

### 9.7 `apps/worker-example`

```
apps/worker-example/
├─ project.json              # Nx "worker-example": build, serve, lint, test
├─ cmd/worker/main.go        # flag --engine-url, --instances; deploy, start, tiga worker, ringkasan
├─ internal/handlers/        # validate_order.go, charge_payment.go (gagal sekali per instance → retry), ship_order.go
├─ processes/order-process.bpmn   # start → validate-order → charge-payment → ship-order → end (dibuat Camunda Modeler)
└─ README.md                 # quickstart: make up; make dev-engine; make demo
```

`make demo` = `go run ./apps/worker-example/cmd/worker --instances 10` terhadap engine di
`localhost:8080`; keluar dengan kode 0 bila 10 instance `COMPLETED` (lewat
`CreateInstanceWithResult`) dalam 30 s. Profil Compose `demo` menjalankan worker sebagai container
dari `infra/docker/worker.Dockerfile` (distroless, sama pola engine) untuk uji deploy.

## 10. Client TypeScript `libs/web/api-client`

- `src/lib/transport.ts` menambah `createDeploymentClient`, `createProcessInstanceClient`,
  `createJobClient`, `createIncidentClient`, `createMessageClient` (pola `createHealthClient`).
- `src/lib/errors.ts`: `findErrorInfo(err: unknown): ErrorInfo | undefined` (memakai
  `ConnectError.findDetails(ErrorInfoSchema)`), `isReason(err, "JOB_NOT_ACTIVE")`,
  `findDeploymentRejection` (dipindah dari fitur `deployment` Fase 1 agar dipakai semua app).
- `src/lib/duration.ts`: `durationFromMs(ms)` / `durationToMs(d)` untuk `google.protobuf.Duration`.
- `index.ts` meng-export semua `gen/workflow/*/v1/*_pb`; `int64` tetap `bigint` (catatan Fase 1).
- `project.json` mendapat target `test` (Vitest) dan `typecheck`; test untuk `errors.ts` dan
  `duration.ts`.
- Konsumen: Fase 6 (Operate: incidents, jobs; Tasklist: `ActivateJobs` **tidak** dipakai UI —
  user task native Fase 6 punya API sendiri), fitur `process-instance` Fase 2 (menambah
  `SetVariables`, `ResolveIncident`).

## 11. Strategi uji

### 11.1 Lapisan uji

| Lapisan | Lokasi | Cakupan |
|---|---|---|
| Unit domain `execution` | `execution/domain/*_test.go` | Tabel transisi job (semua pasangan state × command); `FailJob` retries > 0 / = 0; `ThrowError` → incident; `ResolveIncident` per tipe (6.1) termasuk resolve tanpa perbaikan → incident baru; `SetVariables`; determinisme record (Fase 2) diperluas untuk command baru |
| Unit domain `job` | `job/domain` | `Clamp` setiap parameter (batas bawah/atas, default), `Limits` dari env |
| Unit application `job` | `job/application` dengan `execution/api.Jobs` palsu + notifier palsu + jam palsu | Long polling: bangun oleh notifier, oleh ticker, oleh timeout (respons kosong), oleh ctx; notifier didaftarkan sebelum kueri (job masuk di antara → tetap terbaca); batas long polling aktif |
| Integration kueri aktivasi | `execution/infrastructure/activation_test.go` (`-tags=integration`) | **Semantik kueri**: (a) 20 goroutine × pool terpisah mengaktifkan 2.000 job → union key = 2.000, tidak ada duplikat; (b) job dengan `activatable_at` masa depan tidak terambil; (c) baris yang dikunci transaksi lain dilewati (tx terbuka memegang `FOR UPDATE` pada 10 job → aktivasi mengembalikan job lain); (d) `EXPLAIN (FORMAT JSON)` memakai `job_activatable_idx`; (e) variabel subset `fetch_variable`; (f) `deadline = now + timeout` |
| Integration sweeper | sama | Job `ACTIVATED` dengan deadline lampau kembali `CREATED` dalam satu sweep; batch 500 berulang sampai 0; job yang sedang dikunci dilewati; jam Postgres (bukan jam Go) |
| Integration handler | `job/interfaces`, `execution/interfaces`, `message/interfaces` (`httptest` + Postgres) | Setiap RPC jalur sukses dan setiap `reason` 8.4; protovalidate menolak input salah; `ReadMaxBytes` menolak 17 MiB; timeout interceptor memutus handler yang disumbat |
| Konkurensi end-to-end | `apps/engine/internal/e2e/concurrency_test.go` | 1 proses dengan fork 50 service task × 40 instance; 8 worker SDK konkuren; semua instance `COMPLETED`; `activation_count` = 1 untuk setiap job (tidak ada reaktivasi tanpa timeout); dengan worker yang sengaja "mati" (tidak complete) → job kembali dan diselesaikan worker lain |
| SDK unit | `libs/go/sdk` dengan `sdktest.FakeEngine` | Builder default & validasi; pemetaan hasil handler → RPC; backoff poll kosong/error; `Close` menunggu handler; retry transport; `TokenSource` memasang header; `Variables.Unmarshal` presisi `int64` |
| SDK e2e | `apps/engine/internal/e2e/sdk_test.go` | Deploy → create → worker complete/fail/error → incident → resolve → selesai, dengan engine in-process |
| TS | `libs/web/api-client` Vitest | `findErrorInfo`, `duration` |
| Paritas | `tools/scripts/parity` suite `worker` | Bagian 12 |
| Benchmark | `execution/infrastructure` `BenchmarkActivateJobs` (1 juta job, 8 worker), `job/application` `BenchmarkLongPollingWakeup` | Angka DoD |
| Race | semua paket Go dengan `-race` (sudah di target `test`) | Notifier, worker pool |

### 11.2 Fixture

- `libs/go/bpmn/testdata/valid` Fase 1 dipakai ulang; file baru: `user-task-job-based.bpmn`,
  `service-task-retries-zero.bpmn` (`retries="1"` untuk incident cepat),
  `parallel-50-service-tasks.bpmn` (konkurensi), `order-process.bpmn` (worker-example, juga di
  korpus).
- Skenario tabel Fase 2 (`execution/testdata/scenarios`) diperluas dengan `Step` baru:
  `ActivateJobs(type, max)`, `FailJob(key, retries, backoff)`, `ThrowError(key, code)`,
  `UpdateJobRetries`, `ResolveIncident(nth)`, `SetVariables`, `AdvanceClock(d)` (jam palsu untuk
  backoff/deadline di jalur domain; di jalur Postgres memakai `activatable_at` yang ditulis ke
  masa lalu lewat helper test).

## 12. Paritas terhadap Camunda 8.8

Runner `tools/scripts/parity` (Fase 1–2) mendapat suite `worker`. Terhadap Camunda 8.8 referensi
(`camunda/zeebe:8.8.44`, profil `camunda`) runner memakai REST v2 (`/v2/jobs/activation`,
`/v2/jobs/{jobKey}/completion`, `/v2/jobs/{jobKey}/failure`, `/v2/jobs/{jobKey}/error`,
`/v2/jobs/{jobKey}` untuk update retries/timeout, `/v2/incidents/{incidentKey}/resolution`,
`/v2/element-instances/{elementInstanceKey}/variables`, `/v2/messages/publication`,
`/v2/process-instances` dengan `awaitCompletion`); **semua path dan bentuk body harus diverifikasi
terhadap dokumentasi Camunda 8.8**, termasuk apakah pencarian incident membutuhkan secondary
storage di profil `camunda` (bila tidak tersedia, incident dideteksi lewat percobaan aktivasi
berikutnya yang kosong + `awaitCompletion` timeout, dan dicatat sebagai keterbatasan).

| Skenario (`parity: true`) | Hasil observable yang dibandingkan |
|---|---|
| `worker-complete-linear` | Multiset tipe job teraktivasi, variabel akhir, instance selesai |
| `worker-activate-fetch-variables` | Variabel yang diterima worker = subset yang diminta |
| `worker-activate-max-jobs` | 10 job, `max=3` → 3 job per respons, 4 respons |
| `worker-fail-retry-backoff` | Setelah fail retries 2 backoff 2 s: aktivasi segera kosong, aktivasi setelah 2 s berisi job yang sama dengan retries 2 |
| `worker-retries-exhausted-incident` | Fail retries 0 → aktivasi kosong, instance tidak selesai, ada incident (bila bisa dibaca) |
| `worker-update-retries-resolve` | Setelah update retries + resolve → job aktif lagi, selesai |
| `worker-timeout-reactivation` | Lease 1 s tanpa complete → aktivasi ulang setelah ≥ 1 s mengembalikan job yang sama |
| `worker-complete-after-timeout` | Complete oleh pemegang lease lama setelah timeout tetapi sebelum aktivasi lain: diterima (verifikasi) |
| `worker-complete-twice` | Complete kedua ditolak (kode dicatat; tidak dibandingkan persis) |
| `throw-error-unhandled-incident` | Instance tidak selesai, aktivasi kosong (incident) |
| `set-variables-resolve-condition-incident` | Gateway tanpa flow cocok → incident; set variables + resolve → selesai di cabang yang benar |
| `publish-message-duplicate-id` | Publikasi kedua dengan `messageId` sama dalam TTL ditolak; setelah TTL diterima (verifikasi) |
| `publish-message-ttl-zero` | Diterima, `messageKey` dikembalikan |
| `user-task-job-based` | Job bertipe `io.camunda.zeebe:userTask` dengan header assignee/candidate (verifikasi nama header); complete → selesai |
| `create-with-result` | `awaitCompletion` mengembalikan variabel akhir |

Deviasi yang sudah diperkirakan (dicatat di `expectations.json` bila terkonfirmasi): `TIMED_OUT`
tidak dicatat sebagai record (tidak observable lewat API); format pesan incident berbeda (hanya
`error_type` dibandingkan); `UpdateJobTimeout` mungkin bernama berbeda di REST v2.

## 13. Rencana kerja Fase 4 (urutan eksekusi)

Setiap langkah punya **hasil yang bisa diverifikasi**. 13.3 (proto) dan 13.2 (skema) paralel
dengan 13.4–13.5 (domain, memakai record palsu); 13.11–13.13 (SDK) bisa dimulai begitu 13.3
selesai dengan `sdktest.FakeEngine`.

### 13.1 Prasyarat dan branch

- Fase 2 (modul `execution`, proto `process/v1`, `job/v1`) di `main`; Fase 3 **tidak wajib**
  (lease di luar record log sengaja tidak bergantung replay; bila Fase 3 sudah ada, outbox
  menggantikan publish pasca-commit untuk notifier).
- Branch `feat/phase-4-job-worker-api`; `make check` hijau.

Hasil: branch ada; dokumen ini ditautkan di `docs/plan/README.md`.

### 13.2 Migrasi `00005_job_worker.sql` dan sqlc

- Skema 5.1; kueri `ActivateJobs`, `VariablesForInstances`, `ReleaseExpiredLeases`, `ExtendLease`,
  `UpdateJobLeaseCleared`, `InsertMessage`, `DeleteExpiredMessages`, `ListIncidents` di
  `execution` dan `message`; `make gen` menjalankan ketiga `sqlc.yaml`.

Hasil: `make migrate-up`/`down` bolak-balik bersih; `sqlc generate` tanpa diff setelah commit.

### 13.3 Proto dan generate

- `common/v1`, perluasan `job/v1` dan `process/v1`, `incident/v1`, `message/v1` (bagian 8);
  `buf.yaml` deps protovalidate + `buf.lock`; anotasi pada proto Fase 1–2.
- `buf lint`, `buf breaking --against main`, `buf generate`; stub Go dan TS di-commit.

Hasil: `pnpm nx run-many -t lint breaking generate -p proto` hijau; paket `commonv1`, `incidentv1`,
`messagev1`, `*connect` terkompilasi; `libs/web/api-client` typecheck hijau.

### 13.4 Domain `execution`: command baru

- `FailJob`, `ThrowError`, `UpdateJobRetries`, `ResolveIncident`, `SetVariables` di `Processor`;
  record `JOB FAILED/ERROR_THROWN/RETRIES_UPDATED`, `INCIDENT RESOLVED`; `Apply` untuk masing-
  masing; incident `JOB_NO_RETRIES`, `UNHANDLED_ERROR_EVENT`; aturan langkah ulang 6.1; titik
  sisip `findErrorCatch(...)` yang di Fase 4 selalu mengembalikan "tidak ada".
- User task job-based di `behavior_task.go`; `ProfileExecutable` membuka `userTask` tanpa
  `zeebe:userTask` (perubahan di `libs/go/bpmn`, PR kecil terpisah dengan golden).

Hasil: tabel uji transisi dan skenario domain baru lolos; determinisme record untuk semua command.

### 13.5 Infrastruktur `execution`: aktivasi, lease, proyektor

- `activation.go` (kueri 5.2 + variabel), `lease.go` (`ExtendLease`, `ReleaseExpiredLeases`),
  proyektor membersihkan lease pada record final/fail, `has_incident` dihitung ulang.
- Integration test semantik kueri (11.1 baris 4–5).

Hasil: `make test-integration` lolos termasuk test 20 worker × 2.000 job tanpa duplikat.

### 13.6 Application dan `api` `execution`

- Use case `fail_job`, `throw_error`, `update_job_retries`, `resolve_incident`, `set_variables`,
  `create_instance_with_result` (+ `waiter.go`), `list_incidents`; perluasan `api.go` (5.4).
- Unit test dengan repository fake Fase 2.

Hasil: semua skenario 13.4 lolos lewat application; `CreateWithResult` dengan instance langsung
selesai dan dengan instance yang diselesaikan goroutine lain (event bus) dan dengan timeout.

### 13.7 Platform `rpc` dan `sweeper`

- Interceptor `Recover`, `Logging`, `Timeout`, `Authn` (placeholder), `Validate`; `rpc.Reject`
  dan `ErrorInfo`; `authn.Principal`/`FromContext`.
- `sweeper.Runner` dengan jitter dan metrik hitung sederhana (expvar/slog; OpenTelemetry Fase 8).
- `cmd/engine/main.go` → ekstrak `internal/app.Build(cfg) (*App, error)` + `App.Run(ctx)`; wiring
  interceptor ke semua handler termasuk `health` dan `deployment`.

Hasil: test interceptor (validasi gagal → `REQUEST_INVALID`; panic → `Internal`; timeout → 
`DeadlineExceeded`; principal anonim di context); handler Fase 1–2 tetap lolos test-nya dengan
interceptor terpasang.

### 13.8 Modul `job`

- Lima paket 3.1; `ActivateJobs` long polling (4.2); notifier dari event bus; sweeper task;
  handler `JobService` lengkap (pindahkan `CompleteJob`/`ListJobs` dari `execution/interfaces`).
- Konfigurasi `JOB_*` di `platform/config`.

Hasil: unit test long polling (notifier/ticker/timeout/ctx); integration handler untuk setiap
RPC dan setiap `reason`; `buf curl` `ActivateJobs` dengan `request_timeout` 5 s menunggu lalu
kembali kosong, dan kembali < 100 ms setelah `CreateProcessInstance` di terminal lain.

### 13.9 Modul `message`

- Lima paket; `PublishMessage`; dedup; sweeper TTL; event `message.published`.

Hasil: integration test: duplikat → `AlreadyExists`; TTL 0 tidak tersimpan; kedaluwarsa dihapus
sweeper; `buf curl` mengembalikan `message_key`.

### 13.10 `IncidentService`, `SetVariables`, `CreateProcessInstanceWithResult` di interfaces

- `incident_handler.go` di `execution/interfaces`; perluasan `process_instance_handler.go`.

Hasil: test `httptest` end-to-end: gateway incident → `SetVariables` → `ResolveIncident` →
`COMPLETED`; `CreateProcessInstanceWithResult` mengembalikan variabel akhir.

### 13.11 SDK: client, error, variabel

- `client.go`, `auth.go`, `retry.go`, `errors.go`, `variables.go`, operasi tingkat tinggi;
  `deps_test.go`; `project.json` `sdk-go`.

Hasil: unit test dengan `httptest` handler palsu untuk retry/auth/error mapping; `go vet`,
golangci-lint bersih; godoc contoh terkompilasi (`Example*`).

### 13.12 SDK: `JobWorker` dan `sdktest`

- `worker.go`, `handler.go`, `backoff.go`, `sdktest.FakeEngine`, `sdktest.RunHandler`.

Hasil: test worker terhadap `FakeEngine` (complete/fail/error/auto-fail/shutdown/backoff) dengan
`-race`; coverage paket ≥ 80 %.

### 13.13 E2E engine in-process + SDK

- `apps/engine/internal/e2e`: `sdk_test.go`, `concurrency_test.go` (11.1).

Hasil: `make test-integration` menjalankan e2e; 40 instance × 50 cabang selesai dengan 8 worker
< 60 s di CI; `activation_count` = 1 untuk semua job pada jalur tanpa timeout.

### 13.14 `apps/worker-example`, Compose profil `demo`, `make demo`

- 9.7; `infra/docker/worker.Dockerfile`; target Nx `worker-example:serve`.

Hasil: `make up && make dev-engine` (atau `make dev`) lalu `make demo` keluar 0 dengan ringkasan
"10 instances completed, 10 retries"; `docker compose --profile demo up` menjalankan worker
container yang menyelesaikan instance.

### 13.15 Client TypeScript

- Bagian 10; fitur `process-instance` (Fase 2) mendapat tombol "Set variables" dan "Resolve
  incident" minimal (opsional, bila fitur Fase 2 sudah ada).

Hasil: Vitest hijau; `typecheck` hijau; `modeler-web` build hijau.

### 13.16 Paritas suite `worker`

- Perluas runner; verifikasi endpoint REST v2; rekam `expectations.json`.

Hasil: `make parity SUITE=worker LIVE=1` menghasilkan rekaman; mode offline hijau di CI; deviasi
tercatat dengan alasan.

### 13.17 Benchmark, ADR, dokumentasi

- `BenchmarkActivateJobs` (tabel 1 juta job), `BenchmarkLongPollingWakeup`, benchmark SDK
  throughput (8 worker × 32).
- ADR-0017 (protokol job worker: long polling, `SKIP LOCKED`, lease di luar record log),
  ADR-0018 (user task: job-based Fase 4, native Fase 6), ADR-0019 (`PublishMessage` dan buffer
  message di Fase 4, korelasi Fase 5), ADR-0020 (hardening API: protovalidate, `ErrorInfo`,
  interceptor auth placeholder, kebijakan versi `v1`).
- README modul `job`, `message`, `libs/go/sdk` (quickstart + tabel opsi worker),
  `apps/worker-example`; `docs/plan/README.md`.

Hasil: angka benchmark tercatat di README `job`; ADR tersimpan; review DoD.

## 14. Target Nx dan Makefile

| Proyek | Target | Perintah |
|---|---|---|
| `proto` | `generate`, `lint`, `breaking` | sudah ada; kini mencakup `common/v1`, `incident/v1`, `message/v1`, dep protovalidate (`buf dep update` saat menambah dep) |
| `engine` | `test` | sudah ada; mencakup `job`, `message`, perluasan `execution`, `platform/rpc`, `platform/sweeper` |
| `engine` | `sqlc` | diperluas: `deployment`, `execution`, `message` |
| `engine` | `bench` (cache off) | diperluas dengan `./apps/engine/internal/modules/job/...` |
| `sdk-go` | `build` | `go build ./libs/go/sdk/...` |
| `sdk-go` | `test` | `go test -race -cover ./libs/go/sdk/...` |
| `sdk-go` | `lint` | `golangci-lint run ./libs/go/sdk/...` |
| `worker-example` | `build` | `go build -o dist/apps/worker-example/worker ./apps/worker-example/cmd/worker` |
| `worker-example` | `serve` (cache off) | `go run ./apps/worker-example/cmd/worker --engine-url http://localhost:8080` |
| `worker-example` | `lint`, `test` | golangci-lint, `go test ./apps/worker-example/...` (handler dengan `sdktest`) |
| `api-client` | `test`, `typecheck` | Vitest; `tsc -p tsconfig.lib.json --noEmit` |

`sdk-go` dan `worker-example` menambah `api-go` ke `implicitDependencies`; `worker-example`
menambah `sdk-go`; `engine:test` menyertakan `libs/go/sdk/**` (e2e meng-import SDK) sebagai input.

Target Makefile baru/diperluas: `make demo [INSTANCES=10] [ENGINE_URL=http://localhost:8080]`,
`make parity SUITE=worker [LIVE=1]`, `make test-integration` (kini termasuk e2e),
`make bench` (kini `job`, `sdk`), `make gen` (tiga `sqlc.yaml`; `buf dep update` bila
`buf.lock` berubah), `make up PROFILES="camunda demo"`.

## 15. Risiko dan mitigasi

| Risiko | Dampak | Mitigasi |
|---|---|---|
| `SKIP LOCKED` + indeks parsial lambat pada tabel `job` besar (banyak baris final) | Latensi aktivasi naik | Indeks parsial hanya `CREATED`; benchmark 1 juta baris; retensi job final diputuskan bersama Fase 3/6; `VACUUM` dipantau |
| Lease di luar record log membuat replay Fase 3 "melupakan" aktivasi | Job diaktifkan dua worker (at-least-once) | Didokumentasikan ADR-0017; `CompleteJob` kedua ditolak `FailedPrecondition`; handler worker dianjurkan idempoten (README SDK) |
| Long polling menahan goroutine dan koneksi | Memori/koneksi habis saat ribuan worker | Batas `JOB_MAX_LONG_POLLING`; koneksi DB hanya dipakai saat kueri, tidak selama menunggu; HTTP/2 di SDK |
| Notifier in-process tidak menjangkau replika lain | Latensi aktivasi sampai 1 s pada replika lain | Polling cadangan 1 s (dapat diperkecil); `LISTEN/NOTIFY` Postgres sebagai opsi (pertanyaan 16.2 #2) |
| Jam server Go vs Postgres berbeda | Lease dikembalikan terlalu cepat/lambat | Semua waktu lease dihitung dengan `now()` Postgres; `deadline` di respons berasal dari DB |
| protovalidate memperlambat RPC panas (`ActivateJobs`) | Latensi | Validator di-cache per tipe message (protovalidate-go); benchmark interceptor; bisa dimatikan per prosedur bila terukur |
| Perilaku detail Zeebe (default TTL, retries, nama header user task, resolve pada unhandled error) tidak sesuai asumsi | Paritas gagal | Semua ditandai "verifikasi"; keputusan dikunci setelah 13.16; `expectations.json` |
| Handler `JobService` pindah modul saat Fase 2 belum merge | Konflik merge | Jika Fase 2 belum merge, handler lahir langsung di `job/interfaces`; koordinasi lewat PR kecil |
| SDK menyeret dependensi engine | Pemisahan module sulit | `deps_test.go`; e2e di sisi engine, bukan SDK |
| Backoff worker terlalu agresif saat engine down | Badai request setelah pulih | Backoff eksponensial + jitter sampai 30 s; respons `Unavailable` saat shutdown |
| `CreateProcessInstanceWithResult` menahan koneksi HTTP sampai 55 s | Konsumsi koneksi | Timeout default 10 s; dokumentasi "bukan untuk proses panjang" |
| Kebijakan "hanya aditif" membekukan kesalahan desain `v1` | Hutang API | Dicatat (dua tipe `Incident`); `v2` direncanakan setelah Fase 6 bila perlu |

## 16. Pertanyaan terbuka

Setiap pertanyaan diberi rekomendasi. Keputusan final dicatat di 16.1 setelah review.

### 16.1 Keputusan

Dicatat 2026-10-28 saat eksekusi dimulai, setelah Fase 2 dan 3 di-merge ke `main`. Rekomendasi 16.2
diterima seluruhnya kecuali yang disebut di bawah; baris berikut juga menyesuaikan rencana dengan
kondisi repo yang sebenarnya.

| # | Keputusan | Alasan |
|---|---|---|
| 1 | Branch `feat/phase-4-job-worker` (bukan `feat/phase-4-job-worker-api`) | Sudah dibuat dari `main` 3ef985f |
| 2 | Semua key di proto baru adalah **string UUIDv7** (`job_key`, `incident_key`, `scope_key`, `message_key`, `process_instance_key`, …), bukan `int64` seperti contoh bagian 5.4 dan 8 | ADR-0011; konsisten dengan proto Fase 2–3 |
| 3 | Migrasi bernama `00008_job_worker.sql`; `message.key` bertipe `UUID` | Nomor 00005–00007 dipakai Fase 3 |
| 4 | ADR Fase 4: **ADR-0020** protokol job worker (long polling, `SKIP LOCKED`, lease di luar record log), **ADR-0021** user task job-based, **ADR-0022** buffer message, **ADR-0023** hardening API dan kebijakan versi | ADR-0017–0019 sudah dipakai Fase 3 (idempotensi, contention, snapshot) |
| 5 | `ErrorInfo` masuk paket yang sudah ada `workflow.common.v1` di file baru `error_info.proto`, di samping `errors.proto` (`IdempotencyKeyReused`) | Paket sudah ada sejak Fase 3 |
| 6 | `PageRequest`/`PageResponse` **tidak** dibuat; `ListIncidents` memakai `page_size`/`page_token` langsung dengan token opak `platform/paging` | Konsisten dengan semua RPC daftar Fase 2–3 |
| 7 | RPC mutasi baru yang menulis record (`FailJob`, `ThrowError`, `UpdateJobRetries`, `ResolveIncident`, `SetVariables`) menerima `request_id` (pola ADR-0017). `UpdateJobTimeout` (lease, tanpa record), `PublishMessage` (dedup lewat `message_id`), dan `ActivateJobs` tidak | ADR-0017 menyatakan RPC mutasi baru mengikuti pola yang sama |
| 8 | Notifier long polling berlangganan event bus in-process `execution.job_created` (sudah dipublikasikan pasca-commit sejak Fase 2) + polling cadangan 1 s; outbox **tidak** dipakai untuk notifier. Job yang kembali dapat diaktifkan (backoff habis, resolve) ditangkap polling cadangan; sweeper membangunkan notifier secara langsung | Outbox at-least-once dengan jeda poll relay; sinyal bangun cukup best-effort karena polling cadangan menutup sisanya |
| 9 | Lease kompatibel dengan verifikasi Fase 3: pemuatan state memetakan `ACTIVATED` ke `CREATED`, pemeriksaan cepat `orphan_job` mencakup `ACTIVATED`/`FAILED`/`ERROR_THROWN`. Rebuild proyeksi mengembalikan job ke `CREATED` tanpa lease (dicatat di ADR-0020) | Lease tidak ada di record; replay tidak boleh melaporkannya sebagai selisih |
| 10 | `activatable_at` = timestamp record `JOB FAILED` + `retry_back_off` (jam engine, deterministik untuk replay); `deadline` lease memakai `now()` Postgres | Proyeksi harus bisa dibangun ulang identik dari record; lease tidak |
| 11 | `SnapshotFormatVersion` naik ke **2** (job membawa state, retries, pesan error) | ADR-0019: perubahan bentuk state wajib menaikkan versi |
| 12 | `CompleteJob`/`FailJob`/`ThrowError` hanya diterima pada job `CREATED` (termasuk lease `ACTIVATED`); job `FAILED`/`ERROR_THROWN` ditolak `FailedPrecondition` `JOB_NOT_ACTIVE` | Sama dengan Zeebe: job gagal tidak bisa diselesaikan sebelum incident diselesaikan |
| 13 | `ResolveIncident` pada `UNHANDLED_ERROR_EVENT` membuat job **dapat diaktifkan lagi** (bukan incident baru seperti draf 6.1); pada `JOB_NO_RETRIES` dengan retries masih 0 menghasilkan incident baru | Resolver incident Zeebe mengembalikan job `ERROR_THROWN` ke antrean; diverifikasi di suite paritas `worker` |
| 14 | Perakitan engine dipindah dari `cmd/engine/wiring.go` ke paket `internal/app` (`app.Build`) agar e2e bisa merakit engine yang sama | Bagian 9.5 |
| 15 | Batas request 16 MiB untuk semua handler (sebelumnya 2 MiB di execution); dokumen variabel tetap dibatasi `ENGINE_MAX_VARIABLES_BYTES` | Bagian 8.6 |
| 16 | Metrik sweeper dan long polling memakai `platform/metrics` (OpenTelemetry, Fase 3), bukan expvar | Platform metrik sudah ada |
| 17 | protovalidate: `buf.build/go/protovalidate` v1.4.0 dan dep buf `buf.build/bufbuild/protovalidate`; validasi format key memakai aturan `string.uuid` | Kedua sumber dapat dijangkau; versi lebih dari dua minggu |

### 16.2 Pertanyaan dan rekomendasi

| # | Pertanyaan | Rekomendasi | Alasan |
|---|---|---|---|
| 1 | `StreamActivatedJobs` (server-streaming ala Zeebe 8.4+) di Fase 4? | **Tidak**; long polling dulu, streaming ditinjau Fase 8 setelah ada metrik latensi | Long polling cukup untuk latensi < 100 ms satu replika / ≤ 1 s lintas replika; streaming menambah manajemen koneksi dan tidak lewat nginx tanpa konfigurasi; handler `ActivateBatch` sama dipakai keduanya |
| 2 | Notifikasi lintas replika: polling cadangan atau `LISTEN/NOTIFY` Postgres? | **Polling cadangan 1 s** di Fase 4; `LISTEN/NOTIFY` dievaluasi Fase 8 bersama multi-replika | Satu mekanisme lebih sedikit; pgx mendukung `LISTEN` tetapi butuh koneksi khusus per replika dan penanganan reconnect |
| 3 | `PublishMessage` di Fase 4 atau Fase 5? | **Fase 4** (3.3) | Roadmap; permukaan SDK/TS lengkap; Fase 5 hanya korelasi |
| 4 | User task: job-based di Fase 4? | **Ya**, native Fase 6 (3.4) | Nol logika baru; paritas; Fase 6 fokus pada lifecycle dan Tasklist |
| 5 | Batas jumlah job aktif per process instance saat aktivasi? | **Tidak ada** di Fase 4 | FIFO global sederhana; tidak ada padanan di Zeebe; ditinjau bila ada kasus nyata |
| 6 | SDK: `CompleteJob` yang gagal `Unavailable` lalu diulang dan mendapat `FailedPrecondition` (sudah selesai) — dianggap sukses? | **Tidak otomatis**; kembalikan `ErrJobNotActive` dengan `metadata.state`; dokumentasi menyarankan handler idempoten | SDK tidak bisa membedakan "selesai oleh saya" dari "selesai oleh worker lain"; menyembunyikan ini bisa menutupi bug |
| 7 | RPC "release job" (kembalikan lease lebih awal saat worker shutdown)? | **Tidak di `v1` Fase 4**; SDK memakai `UpdateJobTimeout(timeout kecil, mis. 1 s)` saat `Close` untuk job yang belum dimulai handler | Hasil sama tanpa RPC baru; verifikasi apakah Zeebe menolak timeout di bawah batas minimum |
| 8 | protovalidate vs validasi tangan | **protovalidate** (3.5) | Aturan di kontrak; satu interceptor |
| 9 | `ErrorInfo` sendiri atau `google.rpc.ErrorInfo`? | **Sendiri** di `workflow.common.v1` | Tanpa dep googleapis di TS/Go; bentuk identik sehingga migrasi mudah |
| 10 | Default `timeout` lease dan `request_timeout` bila 0 | **5 menit** dan **jawab segera (0)** di server; SDK default `RequestTimeout` 10 s | Zeebe: default long polling di gateway (verifikasi nilai); lease 5 menit konservatif agar job tidak berpindah saat handler lambat |
| 11 | `ActivatedJob.variables` string JSON atau `google.protobuf.Struct`? | **String JSON** (konsisten Fase 2) | Presisi angka besar dan urutan key terjaga; `Struct` memakai `double` |
| 12 | Apakah `UpdateJobRetries` otomatis menyelesaikan incident `JOB_NO_RETRIES`? | **Tidak**; perlu `ResolveIncident` | Sama dengan Zeebe (verifikasi); UI Fase 6 menggabungkan keduanya dalam satu aksi |
| 13 | `CreateProcessInstanceWithResult` dikerjakan sekarang? | **Ya** | Dibutuhkan paritas dan demo; biaya ±1 hari dengan `Waiter` |
| 14 | SDK sebagai module terpisah sekarang? | **Belum**; `deps_test.go` menjaga kesiapan | ADR-0002; belum ada konsumen eksternal |
| 15 | Auth placeholder: header `Authorization` wajib ada (nilai apa pun) di Fase 4? | **Tidak wajib** | Mencegah UI/worker dev harus memalsukan token; Fase 8 mengaktifkan verifikasi dengan flag |

## 17. Estimasi dan pembagian kerja

Perkiraan **19–22 hari kerja** untuk 1 engineer, atau 11–13 hari untuk 2 engineer paralel:

| Hari | Fokus |
|---|---|
| 1 | 13.1 prasyarat, 13.2 migrasi + sqlc |
| 2–3 | 13.3 proto + protovalidate + generate |
| 3–5 | 13.4 domain `execution` (command baru, incident, user task job-based) |
| 6–7 | 13.5 infrastruktur aktivasi/lease + integration test konkurensi |
| 8 | 13.6 application `execution`, `CreateWithResult` |
| 9 | 13.7 platform `rpc` + `sweeper` + ekstraksi `app.Build` |
| 10–11 | 13.8 modul `job` (long polling, sweeper, handler) |
| 12 | 13.9 modul `message`, 13.10 handler incident/variables |
| 13–14 | 13.11–13.12 SDK client + worker + `sdktest` |
| 15 | 13.13 e2e in-process |
| 16 | 13.14 worker-example + demo + Compose |
| 17 | 13.15 client TS |
| 18–19 | 13.16 paritas live + perbaikan deviasi |
| 20–21 | 13.17 benchmark, ADR-0017..0020, README, review DoD |

Pembagian jika 2 orang: **A** = engine (13.2, 13.4–13.10, paritas); **B** = proto (13.3),
platform `rpc` (13.7, bersama A), SDK (13.11–13.12), worker-example, TS, e2e (13.13). Titik temu:
13.3 (proto dikunci hari 3) dan 13.13 (SDK B terhadap engine A).

## 18. Keluaran Fase 4

Setelah fase ini selesai, fase berikutnya mendapat:

- **Fase 5**: `ThrowError` sudah mencatat `error_code` dan memiliki titik sisip `findErrorCatch`
  untuk error boundary/event sub-process; tabel `message` dengan indeks korelasi dan event
  `message.published` sehingga Fase 5 hanya menambah `message_subscription` dan korelasi; sweeper
  bersama untuk timer; `SetVariables` dan `ResolveIncident` untuk incident baru (timer, message).
- **Fase 6**: `IncidentService.ListIncidents`, `JobService.ListJobs` dengan state enum, `ErrorInfo`
  dan helper TS untuk Operate; `SetVariables`/`ResolveIncident` untuk aksi operator; jalur user
  task job-based sebagai pembanding saat merancang user task native dan Tasklist; `authn.Principal`
  di context untuk atribusi aksi.
- **Fase 7**: `SetVariables(local)` dan `fetch_variable` siap untuk scope lokal; header user task
  memakai evaluator yang sama.
- **Fase 8**: seam interceptor `Authn` (ganti `Anonymous` dengan verifier OIDC), `TokenSource` di
  SDK, `tenant_id` sudah dibawa di `ActivatedJob`/`PublishMessageResponse`; metrik sweeper dan
  long polling siap diekspor ke OpenTelemetry; kebijakan versi `v1`.
- **Semua**: SDK Go dan `sdktest.FakeEngine` untuk menulis worker uji di fase mana pun;
  `worker-example` sebagai smoke test deploy; suite paritas `worker`.
