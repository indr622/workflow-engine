# Fase 8 — Observability, Keamanan, dan Hardening Deploy

**Proyek:** Workflow Engine (kompatibel konsep Camunda 8 / BPMN 2.0)
**Stack:** Go (engine utama) + React (UI) dalam monorepo Nx
**Status:** Draft rencana — belum dieksekusi
**Tanggal:** 2026-10-28

---

## 0. Status eksekusi

Belum ada langkah yang dieksekusi. Bagian ini diisi dengan tabel yang sama seperti Fase 0
(langkah, status, bukti) begitu pekerjaan dimulai.

**Kondisi yang menjadi titik awal (dicek 2026-10-28; Fase 0 selesai, Fase 1 selesai di branch
`feat/phase-1-bpmn-deployment`, Fase 2–7 masih draft). Baris bertanda "asumsi" bergantung pada
rencana fase sebelumnya dan harus dicek ulang di 10.1 saat Fase 8 dimulai:**

| Area | Kondisi / asumsi dari fase sebelumnya | Dampak ke Fase 8 |
|---|---|---|
| `apps/engine/internal/platform/config` | `Config` lima field (`ENGINE_HTTP_ADDR`, `DATABASE_URL`, `ENGINE_MIGRATE`, `ENGINE_LOG_LEVEL`, `ENGINE_SHUTDOWN_TIMEOUT`, `ENGINE_CORS_ORIGINS`) via `caarlos0/env`; Fase 3 (asumsi) menambah grup `ENGINE_DB_*`, `ENGINE_OUTBOX_*`, dst. | Ditambah grup `OTEL_*`/`ENGINE_OTEL_*`, `ENGINE_AUTH_*`, `ENGINE_OIDC_*`, `ENGINE_RATE_LIMIT_*`, `ENGINE_TENANT_*`, `ENGINE_ADMIN_ADDR`; dukungan `<NAMA>_FILE` untuk rahasia (5.7) |
| `platform/logger` | `slog` JSON ke stdout, level dari string | Dibungkus handler yang menambah `trace_id`/`span_id` dari context dan `tenant_id`/`principal` (4.3); format `text` untuk dev |
| `platform/httpserver` | `net/http` + graceful shutdown + CORS tangan; Fase 4 (asumsi) menambah `ReadTimeout` 65 s, `IdleTimeout` 120 s | Ditambah listener admin terpisah (`/metrics`, `/debug/pprof`), urutan drain (7.2), header keamanan untuk respons API |
| `platform/rpc` (Fase 4, asumsi) | Rantai interceptor `Recover → Logging → Timeout → Authn(Anonymous) → Validate`; `authn.Principal{Subject, Tenant: "<default>"}`; `rpc.Reject` + `ErrorInfo{reason}`; ADR-0020 menjanjikan "Fase 8 mengganti satu implementasi (verifier OIDC) tanpa menyentuh handler" | Dipegang: `authn.Anonymous` diganti `authn.OIDC`; interceptor baru `Tracing`, `Metrics`, `Authz`, `RateLimit`, `Tenant` disisipkan di rantai yang sama (5.1); `Principal` diperluas (`Roles`, `Groups`, `TenantIDs`, `Permissions`) |
| `authn.DevIdentity` (Fase 6, asumsi) | Header `X-Dev-User`/`X-Dev-Groups` dibaca hanya bila `ENGINE_DEV_IDENTITY=true`; Fase 6 menulis "Fase 8 menghapusnya" | Diganti `ENGINE_AUTH_MODE=dev` (tetap ada untuk e2e dan dev lokal, dilarang di produksi lewat `ENGINE_ENV=production`), lihat 5.1 dan pertanyaan 13.2 #4 |
| `platform/metrics` (Fase 3, asumsi) | API `go.opentelemetry.io/otel/metric` dengan provider **no-op**; nama metrik `engine_outbox_*`, `engine_recovery_*`, `engine_idempotency_*`, `engine_command_*`, `engine_db_pool_*` terkunci oleh test snapshot; Fase 4–7 menambah `job_*`, `engine_timer_*`, `engine_message_*`, `feel_*` | Fase 8 **memasang provider dan exporter OTLP** tanpa mengganti instrumentasi; metrik baru di 4.2 ditambahkan lewat paket yang sama |
| `platform/outbox` (Fase 3, asumsi) | `outbox_event{id, tx_id, event_id, topic, aggregate_*, payload}`; relay dengan consumer durable; handler `Handle(ctx, tx, ev)` | Kolom `trace_context TEXT` ditambahkan agar consumer membuat span yang **ter-link** ke command penghasil (4.1); `ctx` handler memuat span |
| `platform/recovery` + `/readyz` (Fase 3, asumsi) | Gerbang readiness atomic; `/readyz` 503 sampai pemulihan selesai; `HEALTHCHECK` Docker memakai `/readyz` | Zero-downtime rollout 7.2 memakai gerbang yang sama untuk **drain** (readiness → false saat SIGTERM) |
| `platform/sweeper` (Fase 4), `platform/clock` (Fase 5) | Ticker + batch `SKIP LOCKED`; aman multi-replika | Dua replika engine di 7.2 tidak butuh perubahan; metrik sweeper diekspor |
| Migrasi (Fase 1–7, asumsi) | goose dengan session lock (Fase 3); aturan expand/contract di `docs/ops/migrations.md`; `tenant_id TEXT NOT NULL DEFAULT '<default>'` sudah ada di `deployment`, `process_definition`, `process_instance`, `message*`, `timer`, `message_subscription`, `user_task`, `form`, `decision_*` (7b) — **belum** di `job`, `incident`, `element_instance`, `variable`, `execution_record` | Migrasi Fase 8 (6.2) menambah `tenant_id` ke `job` dan `incident` (dibutuhkan filter aktivasi dan daftar incident tanpa join), indeks tenant-aware, tanpa backfill mahal (default konstan) |
| Proto (Fase 1–7, asumsi) | `ActivatedJob.tenant_id` dan `PublishMessageResponse.tenant_id` sudah ada ("`<default>` sampai Fase 8"); `workflow/common/v1` (`ErrorInfo`, `PageRequest`); kebijakan `v1` aditif (ADR-0020) | Semua perubahan proto Fase 8 **aditif** (`tenant_id`/`tenant_ids` di request, `trace_context` di `ActivatedJob`, paket baru `workflow/identity/v1`, opsi method `permission`); `buf breaking` tetap hijau |
| `libs/go/sdk` (Fase 4, asumsi) | `Client` dengan hook `TokenSource` (`Authorization: Bearer`), `JobWorker`, retry/backoff; dependensi hanya `libs/go/api`, `connect`, `protobuf`, stdlib | Ditambah sub-paket `sdk/oidc` (client credentials) dan `sdk/otel` (propagasi trace) sebagai **paket opsional terpisah** agar paket inti tetap tanpa dependensi (5.4, 4.1) |
| `libs/web/app-shell`, `api-client` (Fase 6, asumsi) | `AuthProvider`/`useAuth()` placeholder; `createEngineTransport(baseUrl, { interceptors })`; `window.__APP_CONFIG__` dari `/config.js` nginx (`MODELER_URL`, `OPERATE_URL`, `TASKLIST_URL`, `DEV_IDENTITY`) | Satu titik ganti per app: `AuthProvider` memakai `oidc-client-ts` (PKCE); `config.js` ditambah `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `AUTH_MODE` (5.5) |
| `infra/docker` | `engine.Dockerfile` distroless nonroot + `HEALTHCHECK` subperintah; `web.Dockerfile` nginx (Fase 6: `ARG APP`, tiga image); `docker-compose.yml` dev (profil `tools`, `observability` dengan `otel-collector` kosong, `camunda`); `docker-compose.prod.yml` (postgres, engine, web, `WEB_PORT`); Fase 3 menambah volume `postgres-backups`, `wal_level=replica`, `docker-compose.chaos.yml` | Compose prod ditulis ulang: Caddy (TLS otomatis, host-based), dua slot engine, backup cron, batas sumber daya, rotasi log, secrets (7.1); profil `observability` dev diganti image LGTM + provisioning dashboard; profil `auth` (Keycloak) di dev dan prod |
| `nginx/default.conf` | SPA fallback, `/api/` → `engine:8080` (`proxy_read_timeout 70s`), cache `/assets/` | Ditambah header keamanan dan CSP (5.9), `client_max_body_size`, `gzip`; proxy `/api/` tetap sama-origin sehingga CORS tidak dibutuhkan di produksi |
| `.github/workflows/ci.yml` | Job `check`, `integration`, `docker` (push GHCR saat tag `v*`); Fase 3 `chaos.yml` nightly; Fase 6 job `e2e` | Job `security` (govulncheck, pnpm audit, Trivy, SBOM) per PR; workflow `release.yml` (tag → image + SBOM + GitHub Release + changelog); job `loadtest` terjadwal (5.8, 7.6, 8.3) |
| `Makefile` | 23 target Fase 0; Fase 1–7 menambah `fuzz`, `bench`, `parity`, `prop`, `chaos`, `db-backup/restore`, `verify`, `rebuild`, `demo`, `dev-ui` | Target baru di bagian 11 (`observe`, `auth-up`, `rollout`, `rollback`, `release`, `sbom`, `scan`, `loadtest`, `tls-local`) |
| Dokumentasi | `docs/adr` (0001–0009 nyata; Fase 2–7 merencanakan 0010–0036 dengan **tumpang tindih** 0017/0018 antara Fase 3 dan 4, dan 0027/0028 antara Fase 5 dan 7); `docs/plan`; Fase 3 merencanakan `docs/ops/{backup-restore,migrations}.md`; Fase 6 `docs/licenses.md`; belum ada `LICENSE` (`package.json`: `UNLICENSED`) | Fase 8 memakai **ADR-0040 sampai ADR-0049** dan menambah langkah rekonsiliasi nomor ADR (9.4); `docs/ops/` dilengkapi runbook; keputusan lisensi (9.3) |
| Paritas | Profil `camunda` (`camunda/zeebe:8.8.44`) hanya sebagai pembanding uji; tidak ada kode Camunda di runtime | Tidak ada uji paritas baru di Fase 8 (auth/observability bukan perilaku BPMN); suite paritas Fase 1–7 dijalankan ulang di checklist rilis (9.1) |

## 1. Lingkup

Fase 8 adalah fase terakhir sebelum rilis 1.0: engine yang secara fungsional lengkap (Fase 1–7)
dibuat **dapat dioperasikan dengan aman di luar laptop developer**. Setelah fase ini, operator bisa
men-deploy seluruh sistem ke satu server dengan satu perintah dan mendapatkan TLS otomatis, login
OIDC untuk tiga aplikasi web, service account untuk worker, isolasi data antar tenant, trace dan
metrik di Grafana, backup terjadwal yang terbukti bisa dipulihkan, dan prosedur upgrade tanpa
memutus worker yang sedang long polling.

Termasuk dalam Fase 8:

1. **Observability** (bagian 4): OpenTelemetry end-to-end — trace dari RPC → command → outbox →
   consumer → worker SDK dengan propagasi context; metrik bisnis dan sistem lewat API OTel yang
   sudah dipakai Fase 3–7 ditambah exporter OTLP; log `slog` berkorelasi `trace_id`; profil
   Compose `observability` dengan Grafana + Tempo + Prometheus/Mimir + Loki; dashboard dan aturan
   alert sebagai kode; definisi SLO.
2. **Keamanan** (bagian 5): autentikasi OIDC (JWT bearer) di interceptor Connect, pemetaan klaim →
   `Principal` (subject, roles, groups, tenant), otorisasi RBAC (`admin`, `operator`, `developer`,
   `worker`, `user`) dengan izin per RPC yang dideklarasikan di `.proto`, service account worker
   (client credentials) di SDK, alur login SPA (Authorization Code + PKCE), penguncian CORS, rate
   limiting, batas ukuran request, manajemen rahasia (env + Docker secrets), pemindaian dependensi
   dan image, SBOM, header keamanan di nginx, TLS di reverse proxy.
3. **Multi-tenant dasar** (bagian 6): tenant dari klaim token, `tenant_id` di setiap tabel yang
   dibaca tanpa join ke `process_instance`, scoping setiap kueri, field `tenant_id`/`tenant_ids`
   di proto mengikuti bentuk Camunda 8, migrasi baris lama ke `<default>`, batas per tenant.
4. **Hardening deploy** (bagian 7): Caddy sebagai reverse proxy dengan TLS otomatis dan routing
   host-based untuk tiga app + API; rollout engine tanpa downtime (dua slot engine, gerbang
   readiness, graceful drain) beserta batasnya; backup `pg_dump` terjadwal dengan retensi dan
   latihan restore; PITR dasar dengan arsip WAL; batas sumber daya; rotasi log; listener admin;
   rilis berversi (tag git → image GHCR → `compose pull`); prosedur upgrade/migrasi dan rollback;
   manifest Kubernetes/Helm sebagai **stretch** (Compose tetap utama, ADR-0007).
5. **Operasional** (bagian 8): runbook (insiden, restore, upgrade, rollback, rotasi rahasia,
   onboarding tenant), panduan kapasitas, load test Go terhadap SLO, suite chaos Fase 3 diperluas
   dengan partisi jaringan dan kegagalan IdP.
6. **Kesiapan rilis 1.0** (bagian 9): checklist lintas fase, dokumentasi (Markdown biasa di
   `docs/`), keputusan lisensi (termasuk kewajiban watermark bpmn.io), rekonsiliasi nomor ADR.

**Tidak termasuk** (dicatat di bagian 15 sebagai pekerjaan lanjutan): database per tenant atau
enkripsi per tenant; otorisasi tingkat resource (ACL per process definition / per instance);
registry tenant dan API admin tenant; Postgres high availability / failover otomatis; clustering
dan partisi engine lintas host; Kubernetes operator; SSE/streaming pembaruan langsung ke UI
(pertanyaan 13.2 #9); `StreamActivatedJobs`; audit log terpisah dari `execution_record`; SSO lewat
SAML/LDAP langsung (dilakukan oleh IdP); WAF; signing image dengan cosign (opsional, 5.8);
penyimpanan diagram di server (Fase 6 #4); DMN 7b bila belum dikerjakan.

## 2. Definition of Done

Observability:

- [ ] `platform/otel` memasang `TracerProvider`, `MeterProvider`, dan propagator W3C (`traceparent`, `baggage`) dari env standar OTel (`OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`, `OTEL_TRACES_SAMPLER`); tanpa endpoint, provider no-op seperti Fase 3 dan engine start tanpa warning; shutdown mem-flush exporter dalam `ENGINE_SHUTDOWN_TIMEOUT`.
- [ ] Satu trace utuh untuk alur `CreateProcessInstance → command → outbox → consumer timer/message → ActivateJobs (worker SDK) → CompleteJob → command` terlihat di Tempo dengan span yang saling terhubung (parent/child di dalam proses, **span link** lintas outbox dan lintas worker); test integrasi memakai exporter in-memory dan memverifikasi struktur span (nama, atribut `process_instance_key`, `bpmn_process_id`, `tenant_id`, link).
- [ ] Semua metrik Fase 3–7 yang sudah terdaftar muncul di Prometheus/Mimir tanpa perubahan nama; metrik baru 4.2 (`engine_process_instances_*`, `engine_jobs_*`, `engine_incidents_*`, `engine_timer_trigger_lag_seconds`, `engine_outbox_lag_seconds`, `db.client.connection.*`) terekspor; test snapshot daftar nama metrik diperbarui; tidak ada label berkardinalitas tak terbatas (lint `TestMetricLabels` menolak `process_instance_key`, `job_key`, `subject`).
- [ ] Setiap baris log yang dikeluarkan dalam konteks request/command/consumer memuat `trace_id`, `span_id`, `tenant_id`, dan `principal` (bila ada); di Grafana, klik dari log ke trace (Loki → Tempo derived field) bekerja pada profil `observability`.
- [ ] `make up PROFILES=observability` menyalakan Grafana dengan datasource dan **empat dashboard** ter-provision dari `infra/observability/grafana/dashboards/*.json` (Engine Overview, Jobs & Workers, Timers/Messages/Outbox, Database & Runtime) dan aturan alert 4.6 ter-provision; `make observe` membuka URL Grafana; dashboard dibuktikan dengan screenshot saat load test (8.3).
- [ ] SLO 4.6 didefinisikan sebagai recording rule + panel; load test 8.3 pada beban referensi memenuhi semua SLO di mesin dev (angka awal, dikunci setelah pengukuran pertama).

Keamanan:

- [ ] `ENGINE_AUTH_MODE=oidc`: interceptor `authn.OIDC` memverifikasi JWT bearer (tanda tangan JWKS dengan cache dan refresh, `iss`, `aud`, `exp`/`nbf` dengan toleransi `ENGINE_OIDC_CLOCK_SKEW`); token tidak ada/invalid → `Unauthenticated` + `ErrorInfo{reason: "TOKEN_MISSING" | "TOKEN_INVALID" | "TOKEN_EXPIRED"}`; `/healthz`, `/readyz` tetap tanpa auth; test unit dengan JWKS lokal (kunci RSA dan ES256 yang dibuat test) mencakup setiap jalur penolakan.
- [ ] `ENGINE_AUTH_MODE=none` mempertahankan perilaku anonim Fase 4–7 (CI, test integrasi, paritas); `ENGINE_AUTH_MODE=dev` mempertahankan `X-Dev-User`/`X-Dev-Groups` (+ `X-Dev-Roles`, `X-Dev-Tenants`) untuk e2e; keduanya menulis log WARN saat start dan **ditolak** bila `ENGINE_ENV=production` (engine keluar dengan error konfigurasi).
- [ ] `Principal{Subject, Name, Roles, Groups, TenantIDs, Permissions}` dipetakan dari klaim yang path-nya dapat dikonfigurasi (`ENGINE_OIDC_ROLES_CLAIM` default `realm_access.roles`, `ENGINE_OIDC_TENANT_CLAIM` default `tenant_ids`, `ENGINE_OIDC_GROUPS_CLAIM` default `groups`); test tabel dengan payload token Keycloak dan payload generik.
- [ ] Setiap RPC di `libs/proto` mendeklarasikan `option (workflow.common.v1.permission) = "<resource>:<action>"`; lint `buf` kustom atau test Go `TestEveryRPCHasPermission` gagal bila ada RPC tanpa opsi; interceptor `authz` menolak principal tanpa izin dengan `PermissionDenied` + `ErrorInfo{reason: "PERMISSION_DENIED", metadata: {permission}}`; matriks peran → izin 5.3 dibuktikan test tabel untuk kelima peran terhadap semua RPC.
- [ ] Modul `usertask` menegakkan kepemilikan (claim/complete) dan `SearchUserTasks` memakai `Principal` (bukan request) ketika principal bukan anonim (janji Fase 6 #12–13), dibuktikan test dengan dua principal.
- [ ] `IdentityService.GetMe` (paket baru `workflow/identity/v1`) mengembalikan subject, nama, roles, tenant, dan daftar izin; `app-shell` memakainya untuk menyembunyikan aksi yang tidak diizinkan (UI hanya kosmetik, server sumber kebenaran).
- [ ] SDK Go: `sdk/oidc.ClientCredentials(issuer, clientID, secret, scopes)` mengimplementasikan `TokenSource` dengan cache dan refresh sebelum kedaluwarsa; `worker-example` berjalan terhadap engine dengan `ENGINE_AUTH_MODE=oidc` dan Keycloak profil `auth` (`make demo AUTH=1`); token kedaluwarsa di tengah long polling → SDK memperbarui token dan mencoba ulang tanpa kehilangan job (test dengan token berumur 5 s).
- [ ] UI: `AuthProvider` memakai Authorization Code + PKCE (`oidc-client-ts`, lisensi diverifikasi), token hanya di memori, silent renew; tanpa token → redirect ke IdP; `401` dari API → login ulang; logout memanggil end-session IdP; e2e Playwright ketiga app login lewat Keycloak profil `auth` dan menjalankan smoke flow Fase 6 dengan peran yang sesuai; pengguna dengan peran `user` tidak melihat tombol cancel instance di Operate dan `CancelProcessInstance` oleh peran itu ditolak server.
- [ ] CORS: produksi tanpa origin (`ENGINE_CORS_ORIGINS` kosong → tidak ada header CORS); dev tetap `localhost:4200–4202`; test memastikan header `Access-Control-*` tidak muncul untuk origin yang tidak terdaftar.
- [ ] Rate limiting token bucket per principal (fallback per IP untuk anonim) dengan `ENGINE_RATE_LIMIT_RPS`/`_BURST`; melebihi → `ResourceExhausted` + `ErrorInfo{reason: "RATE_LIMITED"}` + header `Retry-After`; `ActivateJobs` long polling dihitung satu per request; test 1.000 request serentak memverifikasi jumlah yang diterima ≈ burst + rps × durasi.
- [ ] Rahasia: setiap field string konfigurasi dapat diisi dari file lewat `<NAMA>_FILE` (`DATABASE_URL_FILE`, `ENGINE_OIDC_CLIENT_SECRET_FILE`, `POSTGRES_PASSWORD_FILE`); compose prod memakai `secrets:` berbasis file; nilai rahasia tidak pernah muncul di log (test memastikan `Config.String()` menyamarkan).
- [ ] CI job `security`: `govulncheck ./...`, `pnpm audit --audit-level=high`, Trivy (filesystem + keempat image) gagal pada CRITICAL/HIGH yang punya fix; SBOM (SPDX JSON) untuk setiap image diunggah sebagai artefak CI dan dilampirkan ke GitHub Release; Dependabot aktif untuk `gomod`, `npm`, `github-actions`, `docker`.
- [ ] nginx tiap app mengirim `Content-Security-Policy` (5.9), `X-Content-Type-Options: nosniff`, `Referrer-Policy`, `Permissions-Policy`, `X-Frame-Options: DENY`; Caddy mengirim `Strict-Transport-Security`; pemindaian header (mis. `make scan-headers` memakai curl) membuktikan; ketiga app tetap berfungsi penuh di bawah CSP (e2e berjalan dengan CSP aktif, pelanggaran CSP dilaporkan ke `/csp-report` → log).

Multi-tenant:

- [ ] `ENGINE_MULTI_TENANCY=false` (default): semua request memakai `<default>`; `tenant_id` di request selain kosong/`<default>` → `InvalidArgument` `TENANT_DISABLED`; perilaku dan hasil semua test Fase 1–7 tidak berubah.
- [ ] `ENGINE_MULTI_TENANCY=true`: request mutasi wajib menyebut tenant (field `tenant_id`, atau satu-satunya tenant principal); tenant di luar `Principal.TenantIDs` → `PermissionDenied` `TENANT_FORBIDDEN`; pembacaan by-key untuk entitas tenant lain → `NotFound` (bukan `PermissionDenied`, mencegah enumerasi); `ActivateJobs{tenant_ids}` hanya mengembalikan job tenant yang diminta ∩ tenant principal; `PublishMessage` hanya berkorelasi di tenant yang sama; test integrasi dua tenant pada satu DB: deploy proses dengan `bpmn_process_id` sama di dua tenant, versi independen, instance saling tidak terlihat, worker tenant A tidak pernah menerima job tenant B.
- [ ] Migrasi menambah `tenant_id` ke `job` dan `incident` (default `<default>`, tanpa backfill mahal), indeks `(tenant_id, type, key) WHERE state='CREATED'` dibuat `CONCURRENTLY`; `engine verify` memeriksa konsistensi `tenant_id` anak vs `process_instance` (`VerifyTenantMismatch`).
- [ ] Lint statis `TestTenantScopedQueries`: setiap kueri sqlc pada tabel ber-`tenant_id` memiliki predikat `tenant_id` (daftar pengecualian eksplisit untuk janitor dan kueri admin).
- [ ] `ENGINE_TENANT_MAX_ACTIVE_INSTANCES` (0 = tanpa batas) ditegakkan di `CreateProcessInstance`, message start, dan timer start → `ResourceExhausted` `TENANT_QUOTA_EXCEEDED` (timer/message start: incident tidak dibuat, kejadian di-log + metrik).

Deploy dan operasional:

- [ ] `docker compose -f infra/docker/docker-compose.prod.yml --env-file .env.prod up -d` di server kosong dengan DNS yang menunjuk ke host menghasilkan `https://modeler.<domain>`, `operate.<domain>`, `tasklist.<domain>`, `api.<domain>` dengan sertifikat Let's Encrypt otomatis (Caddy); lokal `make deploy-local` memakai `*.localhost` dengan CA internal Caddy; `curl --fail https://api.localhost/healthz` berjalan setelah CA internal dipercaya (`make tls-local`).
- [ ] `make rollout VERSION=<baru>`: dengan load worker-example berjalan (3 worker, 200 instance/menit), rollout dua slot engine menyelesaikan upgrade tanpa satu pun RPC gagal selain `Unavailable` yang di-retry SDK secara transparan, tanpa job hilang/ganda (`engine verify --all` bersih, hitungan instance selesai = dibuat); durasi < 2 menit; dibuktikan skenario chaos `rolling-upgrade`.
- [ ] Graceful drain: SIGTERM → `/readyz` 503 → tunggu `ENGINE_DRAIN_DELAY` → long polling aktif dikembalikan kosong → `http.Server.Shutdown` → relay/sweeper berhenti → pool ditutup; waktu total ≤ `ENGINE_SHUTDOWN_TIMEOUT`; `stop_grace_period` compose ≥ nilai itu; test integrasi memverifikasi urutan lewat log.
- [ ] Backup: container `db-backup` menjalankan `pg_dump -Fc` tiap `BACKUP_CRON` (default harian 02:00) ke volume `postgres-backups` dengan retensi `BACKUP_KEEP_DAYS` (default 14) dan `BACKUP_KEEP_WEEKLY`; metrik/file `last_backup_ok` dibaca alert "backup tua > 26 jam"; **latihan restore** `make db-restore-drill` (restore ke DB sementara + `engine verify --all`) dijalankan di CI terjadwal mingguan dan hasilnya dicatat.
- [ ] PITR dasar: `POSTGRES_PITR=true` mengaktifkan `archive_mode=on` dan `archive_command` ke volume `postgres-wal-archive` + base backup mingguan (`pg_basebackup`); runbook `docs/ops/restore.md` berisi prosedur `recovery_target_time` yang **diuji sekali** end-to-end (restore ke titik sebelum penghapusan yang disengaja) dengan bukti di bagian 0.
- [ ] Batas sumber daya (`deploy.resources.limits`), `GOMEMLIMIT` otomatis dari batas memori, rotasi log `json-file` (`max-size`, `max-file`) berlaku untuk semua service; `docker stats` selama load test tidak melebihi batas.
- [ ] Listener admin `ENGINE_ADMIN_ADDR` (default `:9090`, hanya jaringan internal Compose) menyajikan `/metrics` (format Prometheus, opsional `ENGINE_METRICS_PROMETHEUS=true`), `/debug/pprof` (bila `ENGINE_PPROF_ENABLED=true`), `/readyz` dan `/healthz` (duplikat); Caddy tidak pernah mengekspos port admin (test: `curl https://api.<domain>/debug/pprof` → 404).
- [ ] Rilis: `make release VERSION=v1.0.0` memverifikasi `make check`, changelog, dan membuat tag; workflow `release.yml` membangun keempat image dengan tag `v1.0.0` dan `v1.0`, mendorong ke GHCR, melampirkan SBOM dan bundel deploy (`workflow-engine-deploy-v1.0.0.tar.gz`: compose prod, Caddyfile, `.env.prod.example`, runbook) ke GitHub Release; `docs/ops/upgrade.md` dan `rollback.md` diuji dengan upgrade `v0.x → v1.0.0-rc` dan rollback pada DB berisi data (expand/contract Fase 3 dipatuhi).
- [ ] `make loadtest` (Go, `tools/scripts/loadtest`) menghasilkan laporan p50/p95/p99 untuk skenario 8.3 dan membandingkan dengan SLO; CI terjadwal mingguan menyimpan laporan sebagai artefak; angka kapasitas referensi dicatat di `docs/ops/capacity.md`.
- [ ] Suite chaos Fase 3 diperluas (8.4): `rolling-upgrade`, `partition-engine-postgres` (sudah ada), `partition-engine-idp` (JWKS tidak terjangkau: token yang sudah diverifikasi tetap diterima dari cache; login UI gagal dengan pesan jelas), `partition-caddy-engine`, `kill-one-replica-under-load`; semua hijau tiga kali di mesin dev dan satu kali nightly.
- [ ] Runbook `docs/ops/{incident-response,restore,upgrade,rollback,rotate-secrets,oidc-setup,tls,tenant-onboarding,capacity}.md` ada dan setiap prosedur yang bisa dijalankan sudah dijalankan sekali dengan hasil dicatat di bagian 0.

Rilis 1.0:

- [ ] Checklist 9.1 terisi dengan bukti (link PR/run CI) untuk setiap fase, termasuk uji paritas Fase 1–7 dijalankan ulang terhadap `camunda/zeebe:8.8.44` pada rc terakhir.
- [ ] `docs/README.md` menjadi indeks dokumentasi (mulai, arsitektur, API, operasi, ADR, rencana); `README.md` root memuat "deploy produksi dalam 10 menit"; tidak ada tautan rusak (`make docs-check`).
- [ ] `LICENSE` dan `NOTICE`/`docs/licenses.md` tersimpan sesuai keputusan 9.3; watermark "Powered by bpmn.io" tetap tampil di kanvas bpmn-js/dmn-js (e2e memeriksa elemen `.bjs-powered-by` ada dan terlihat — nama kelas diverifikasi).
- [ ] Nomor ADR direkonsiliasi (9.4): `docs/adr/README.md` berisi indeks final tanpa duplikat; dokumen rencana Fase 2–7 diperbarui dengan nomor final; ADR-0040 sampai ADR-0049 tersimpan.
- [ ] `docs/plan/README.md` menandai Fase 8 dan seluruh roadmap selesai; tag `v1.0.0` dibuat dari `main`.

## 3. Keputusan teknis

| Area | Keputusan | Alasan singkat |
|---|---|---|
| Telemetri | **OpenTelemetry SDK Go** (`go.opentelemetry.io/otel`, `otel/sdk`, exporter `otlptracegrpc`/`otlpmetricgrpc`; varian `otlp*http` bila `OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf`) di `platform/otel`; instrumentasi Connect lewat `connectrpc.com/otelconnect` (verifikasi API versi saat 10.2); konfigurasi memakai **env standar OTel**, bukan `ENGINE_*` sendiri | Fase 0 memutuskan OTel; Fase 3 sudah memakai API metric dengan provider no-op; env standar membuat collector, Grafana, dan LGTM bekerja tanpa dokumentasi khusus (ADR-0040) |
| Propagasi lintas batas asinkron | **Span link**, bukan parent/child: command menulis `trace_context` (W3C `traceparent` + `tracestate`) ke `outbox_event`, ke baris `timer` dan `message_subscription`, dan ke `ActivatedJob.trace_context`; consumer dan worker membuat span root baru dengan `Link` ke span asal (4.1) | Trace parent/child yang menyeberangi jam atau hari (timer, message) tidak terbaca dan menahan retensi; link mempertahankan navigasi dua arah di Tempo (verifikasi dukungan UI link di Tempo/Grafana) |
| Metrik | API OTel yang sama dengan Fase 3; nama yang sudah terkunci tidak berubah; metrik baru memakai prefiks `engine_` untuk bisnis dan **semantic conventions OTel** untuk RPC (`rpc.server.duration` dari otelconnect) dan DB (`db.client.connection.*` dari `pgxpool.Stat`); ekspor **push OTLP** (utama) + `/metrics` Prometheus opsional di listener admin | Push OTLP cocok dengan collector dan LGTM; `/metrics` opsional untuk operator yang sudah punya Prometheus scrape; satu sumber instrumentasi |
| Log | Tetap **`slog` JSON ke stdout** (Docker `json-file` dengan rotasi); handler pembungkus menambah `trace_id`, `span_id`, `tenant_id`, `principal`; pengiriman ke Loki lewat collector (receiver `filelog` atau `docker_stats`/Alloy; verifikasi) ; ekspor log OTLP langsung (bridge `otelslog`) **opsional** di balik `ENGINE_OTEL_LOGS=true` | stdout adalah kontrak 12-factor dan tidak bergantung pada ketersediaan collector; korelasi cukup dengan `trace_id` di JSON |
| Stack observability dev | **`grafana/otel-lgtm`** satu image (Grafana + Tempo + Loki + Mimir/Prometheus + collector; verifikasi nama image, port, dan cara provisioning dashboard/alert) menggantikan `otel-collector` kosong di profil `observability` | Satu container, nol konfigurasi untuk developer; dashboard yang sama di-provision ke stack prod |
| Stack observability prod | **Opsional**: overlay `docker-compose.observability.yml` (collector + Prometheus + Tempo + Loki + Grafana dari image resmi masing-masing) **atau** `OTEL_EXPORTER_OTLP_ENDPOINT` ke platform eksternal (Grafana Cloud, dsb.); bukan bagian compose prod inti | Banyak organisasi sudah punya platform; engine hanya butuh endpoint OTLP |
| Dashboard dan alert sebagai kode | JSON dashboard Grafana + provisioning YAML di `infra/observability/grafana/`; aturan alert sebagai **Grafana alerting provisioning** (file) dengan sumber kueri Prometheus; aturan yang sama disalin sebagai Prometheus rule file untuk operator non-Grafana | Dapat di-review di PR; dipakai dev dan prod tanpa klik di UI |
| Autentikasi | **OIDC / JWT bearer** diverifikasi di engine lewat JWKS (`github.com/coreos/go-oidc/v3` + `golang.org/x/oauth2`; verifikasi dukungan `ProviderConfig` untuk discovery URL internal yang berbeda dari `iss` publik); engine **tidak** menerbitkan token dan tidak menyimpan pengguna | Standar; bekerja dengan Keycloak, Zitadel, Entra ID, Auth0 tanpa kode khusus; sejalan dengan Camunda 8 self-managed (verifikasi) (ADR-0041) |
| IdP untuk dev dan referensi | **Keycloak** di profil Compose `auth` dengan realm ter-import (`infra/keycloak/realm-workflow.json`: tiga client publik PKCE, satu client confidential worker, pengguna contoh per peran); port host `KEYCLOAK_PORT` default **8180** agar tidak bentrok dengan Keycloak lokal pengguna di 8080; `ENGINE_OIDC_ISSUER` bisa diarahkan ke Keycloak yang sudah ada | Tim sudah menjalankan Keycloak untuk proyek lain; realm sebagai file membuat e2e deterministik |
| Mode auth | `ENGINE_AUTH_MODE=oidc` (default) / `dev` (header `X-Dev-*`, menggantikan `ENGINE_DEV_IDENTITY`) / `none` (anonim, Fase 4–7); `dev` dan `none` ditolak saat `ENGINE_ENV=production` | CI, paritas, dan e2e butuh jalur tanpa IdP; produksi tidak boleh salah konfigurasi diam-diam |
| Model identitas | `authn.Principal{Subject, Name, Roles, Groups, TenantIDs, Permissions, Anonymous}`; pemetaan klaim dengan path yang dapat dikonfigurasi; **roles → permissions** dihitung di engine dari tabel 5.3 (bukan dari token) | Token tetap kecil dan IdP-agnostik; perubahan matriks izin tidak butuh perubahan realm |
| Otorisasi | **RBAC lima peran** (`admin`, `operator`, `developer`, `worker`, `user`) dengan izin `resource:action`; **izin per RPC dideklarasikan di `.proto`** lewat `option (workflow.common.v1.permission)`; interceptor `authz` membaca opsi dari descriptor registry berdasarkan `Spec().Procedure`; tingkat resource ditunda (15) (ADR-0042) | "Kontrak lahir dari proto" (ADR-0003): izin terlihat di dokumentasi API dan tidak bisa terlewat (test memaksa setiap RPC punya opsi) |
| Tasklist dan kepemilikan | Peran `user` hanya `usertask:*` dan `process:read`; pembatasan `process:read` ke instance yang memuat task milik pengguna ditunda bersama ACL per instance (pertanyaan 13.2 #6) | Pengguna bisnis tidak boleh mengoperasikan instance; ACL per instance adalah pekerjaan lanjutan |
| Service account worker | **Client credentials** (`sdk/oidc.ClientCredentials` memakai `golang.org/x/oauth2/clientcredentials`) → token dengan peran `worker`; SDK inti tidak berubah dependensinya karena `sdk/oidc` adalah paket terpisah | Worker adalah mesin; refresh otomatis; paket terpisah menjaga `deps_test.go` SDK inti |
| Login SPA | **Authorization Code + PKCE di browser** (`oidc-client-ts`, verifikasi lisensi MIT) dengan token di memori dan silent renew; **bukan BFF** (3.2) (ADR-0041) | Tiga app di tiga host berbagi SSO lewat sesi IdP; tanpa komponen server baru; CSP ketat menutup risiko XSS utama |
| Topologi jaringan | Caddy di depan: `modeler.`, `operate.`, `tasklist.` → nginx per app (tetap mem-proxy `/api/` ke engine, **sama-origin**), `api.` → engine langsung (SDK/worker), `auth.` → Keycloak (profil `auth`); CORS produksi **kosong** | UI tidak butuh CORS; `api.` tanpa browser; satu tempat TLS |
| Reverse proxy | **Caddy 2** (`caddy:2-alpine`, verifikasi versi), Caddyfile di `infra/docker/caddy/Caddyfile`, host-based (keputusan Fase 6 #1), TLS otomatis ACME dan CA internal untuk `*.localhost`, health check `/readyz` ke dua slot engine (3.1) (ADR-0044) | Konfigurasi terpendek untuk TLS otomatis; tidak butuh label Docker; Traefik ditinjau bila pindah ke Swarm/K8s |
| Replika engine | **Dua slot** `engine-a`/`engine-b` di compose prod (YAML anchor, image per slot `ENGINE_IMAGE_A`/`ENGINE_IMAGE_B`), Caddy round-robin dengan health check; rollout = ganti satu slot, tunggu ready, ganti slot lain (`tools/scripts/rollout`) (3.3) (ADR-0045) | Compose tidak punya rolling update; desain Fase 2–3 sudah aman multi-replika (row lock per instance, advisory lock consumer, `SKIP LOCKED` sweeper, session lock migrasi) sehingga dua versi binary boleh berjalan bersama selama fase expand |
| Graceful drain | SIGTERM → readiness false → `ENGINE_DRAIN_DELAY` (default 5 s, lebih besar dari `health_interval` Caddy) → notifier long polling ditutup (jawab kosong) → `Shutdown` → relay, sweeper, janitor berhenti → flush OTel → pool close; `stop_grace_period: 40s` | Worker tidak melihat error; request baru sudah dialihkan Caddy sebelum listener ditutup |
| Batas zero-downtime | Dijamin: upgrade engine dan web tanpa RPC gagal. **Tidak** dijamin: restart Postgres (downtime beberapa detik, klien retry), kegagalan host (satu node), upgrade major Postgres (prosedur manual di `upgrade.md`) | Compose satu host; HA database di luar lingkup (15) |
| Backup | Container `db-backup` dari image `postgres:18-alpine` + skrip `crond` milik repo (`infra/docker/backup/backup.sh`): `pg_dump -Fc` terjadwal, retensi harian/mingguan, file status untuk alert; bukan image pihak ketiga | Tanpa dependensi tambahan yang harus dipercaya; `pg_dump` sudah dipakai Fase 3 (ADR-0046) |
| PITR | **Arsip WAL dengan fitur bawaan Postgres** (`archive_mode`, `archive_command` menyalin ke volume `postgres-wal-archive`, `pg_basebackup` mingguan dari container backup) di balik `POSTGRES_PITR=true`; pgBackRest/WAL-G **tidak** dipakai di 1.0 | Cukup untuk RPO menit pada satu host tanpa komponen baru; alat khusus masuk pekerjaan lanjutan bila butuh offsite atau inkremental (ADR-0046) |
| Backup offsite | Volume backup disinkronkan oleh operator (rclone/rsync) sesuai contoh di runbook; **tidak** ada container sync di compose inti | Kredensial penyimpanan objek berbeda-beda; tanggung jawab operator |
| Rahasia | `.env.prod` (chmod 600) untuk konfigurasi; **Docker secrets berbasis file** (`secrets:` compose, dibaca engine lewat `<NAMA>_FILE`) untuk password DB dan client secret; tidak ada rahasia di image, log, atau repo | Bekerja di Compose non-Swarm; pola `_FILE` sama dengan image Postgres resmi |
| Rate limiting | Di engine (interceptor `ratelimit`, `golang.org/x/time/rate`, bucket per principal/IP/tenant, in-memory per replika); **bukan** di Caddy | Plugin rate limit Caddy bukan bagian image resmi; engine mengenal principal dan tenant; per-replika cukup untuk melindungi DB |
| Batas request | Connect `WithReadMaxBytes` 16 MiB (Fase 4) tetap; Caddy `request_body max_size 20MB`, nginx `client_max_body_size 20m`; `ActivateJobs` ≤ 60 s tetap | Lapisan luar sedikit lebih longgar agar error yang dilihat klien konsisten dari engine (`ErrorInfo`) |
| Header keamanan | CSP ketat di nginx tiap app (5.9) dengan `report-uri /csp-report` → log nginx; HSTS di Caddy; `frame-ancestors 'none'` | SPA + bpmn-js butuh `style-src 'unsafe-inline'` (verifikasi) tetapi tidak `unsafe-eval`; CSP adalah mitigasi utama untuk token di memori |
| Supply chain | `govulncheck`, `pnpm audit`, Trivy (fs + image), SBOM SPDX via `syft` (atau atestasi BuildKit `--sbom`; verifikasi), Dependabot; `gosec` sudah ada di golangci; signing image (cosign) opsional | Standar yang tersedia gratis di GitHub Actions (ADR-0049) |
| Model tenant | Tenant = string `^[A-Za-z0-9_.-]{1,63}$` dari klaim; **tanpa registry tenant** di 1.0; `tenant_id` di request mengikuti bentuk Camunda 8 (`tenantId` pada deploy/create/publish, `tenantIds` pada aktivasi dan pencarian; verifikasi nama field di REST v2 8.8); key tetap global (`engine_key_seq`, tanpa prefiks tenant) dan setiap pembacaan by-key menambah predikat tenant (ADR-0043) | Kompatibel dengan klien Camunda; key global menghindari migrasi; isolasi ditegakkan di kueri + lint statis, bukan skema terpisah |
| Multi-tenancy off secara default | `ENGINE_MULTI_TENANCY=false` → semua `<default>`; proto tetap menerima `tenant_id` kosong | Sebagian besar deploy satu organisasi; perilaku Fase 1–7 tidak berubah |
| Batas per tenant | Rate limit per tenant (kunci `tenant`) + `ENGINE_TENANT_MAX_ACTIVE_INSTANCES`; kuota berbeda per tenant (tabel) ditunda | Konfigurasi global cukup untuk 1.0; tabel kuota butuh registry tenant |
| Rilis | SemVer; tag `vX.Y.Z` dari `main`; image `vX.Y.Z` + `vX.Y` (tanpa `latest` di prod); changelog dari Conventional Commits (`git-cliff`, verifikasi; alternatif `conventional-changelog`); bundel deploy dilampirkan ke GitHub Release (ADR-0047) | Memakai konvensi commit yang sudah ditegakkan sejak Fase 0 |
| Upgrade / rollback | Upgrade = backup → `compose pull` → `make rollout`; migrasi berjalan saat slot pertama start (session lock); rollback binary diizinkan selama fase contract belum dijalankan (aturan Fase 3); migrasi `down` **tidak** dijalankan otomatis di produksi | Mengikuti `docs/ops/migrations.md` Fase 3 |
| Kubernetes | **Stretch**: Helm chart minimal `infra/helm/workflow-engine` (Deployment engine 2 replika + readiness probe, tiga Deployment web, Ingress, Secret, Postgres eksternal) bila waktu tersisa; tidak masuk DoD; Compose tetap jalur utama (ADR-0007) | Desain engine sudah cocok dengan rolling update K8s; chart hanya pembungkus |
| Load test | **Go** (`tools/scripts/loadtest`) memakai `libs/go/sdk`; k6 opsional untuk uji HTTP murni | SDK sudah ada; protokol Connect/protobuf dan long polling lebih alami di Go |
| Dokumentasi | **Markdown biasa** di `docs/` dengan `docs/README.md` sebagai indeks dan pemeriksa tautan; tanpa mkdocs/Docusaurus di 1.0 | Tim kecil; GitHub merender Markdown; generator statis bisa ditambahkan kapan saja (pertanyaan 13.2 #11) |
| Lisensi | Rekomendasi **Apache-2.0** + `NOTICE` + `docs/licenses.md` (pihak ketiga, termasuk kewajiban watermark bpmn.io); keputusan akhir milik pemilik repo (pertanyaan 13.2 #12) (ADR-0048) | Lihat 9.3 |
| ADR | Fase 8 memakai **ADR-0040 sampai ADR-0049**; langkah 10.1 merekonsiliasi nomor Fase 2–7 yang tumpang tindih sebelum ADR Fase 8 ditulis (9.4) | Nomor 0037–0039 disisakan untuk hasil rekonsiliasi |

### 3.1 Reverse proxy: Caddy vs Traefik vs nginx

| Opsi | Kelebihan | Kekurangan |
|---|---|---|
| **A. Caddy 2** | TLS otomatis (ACME HTTP-01/TLS-ALPN, CA internal untuk lokal) tanpa konfigurasi tambahan; Caddyfile pendek; `reverse_proxy` dengan health check aktif, `lb_policy`, retry (`lb_try_duration`; verifikasi nama direktif pada versi yang dipakai); HTTP/2 dan HTTP/3 bawaan; metrik Prometheus bawaan (verifikasi cara mengaktifkan) | Rate limiting dan plugin lain membutuhkan image kustom (`xcaddy`); komunitas Compose lebih kecil daripada Traefik |
| B. Traefik | Konfigurasi lewat label container; dashboard; ACME; cocok untuk Swarm/K8s | Label menyebar ke setiap service compose; konfigurasi lebih panjang untuk empat host sederhana |
| C. nginx + certbot | Sudah dipakai di image web | Pembaruan sertifikat butuh container/cron terpisah dan reload; health check upstream aktif hanya di nginx Plus |

**Dipilih A.** Sketsa `infra/docker/caddy/Caddyfile`:

```caddyfile
# DOMAIN=example.com (lokal: DOMAIN=localhost → sertifikat CA internal)
{
	email {$ACME_EMAIL}
	servers {
		metrics
	}
}

(security_headers) {
	header {
		Strict-Transport-Security "max-age=31536000; includeSubDomains"
		-Server
	}
}

(engine_upstream) {
	reverse_proxy engine-a:8080 engine-b:8080 {
		lb_policy round_robin
		health_uri /readyz
		health_interval 2s
		health_timeout 2s
		fail_duration 10s
		max_fails 1
		lb_try_duration 5s
		transport http {
			read_timeout 70s
		}
	}
}

modeler.{$DOMAIN} {
	import security_headers
	reverse_proxy modeler:80
}
operate.{$DOMAIN} {
	import security_headers
	reverse_proxy operate:80
}
tasklist.{$DOMAIN} {
	import security_headers
	reverse_proxy tasklist:80
}
api.{$DOMAIN} {
	import security_headers
	request_body {
		max_size 20MB
	}
	@blocked path /debug/* /metrics
	respond @blocked 404
	import engine_upstream
}
auth.{$DOMAIN} {
	import security_headers
	reverse_proxy keycloak:8080
}
```

Untuk `DOMAIN=localhost`, Caddy memakai CA internal secara otomatis untuk nama `*.localhost`
(verifikasi perilaku auto-HTTPS untuk subdomain `localhost`; bila tidak otomatis, tambahkan
`tls internal` lewat snippet). `make tls-local` mengekspor root CA Caddy
(`docker compose cp caddy:/data/caddy/pki/authorities/local/root.crt`) dan mencetak cara
memasangnya di trust store OS/browser. Nginx di tiap app **tetap** mem-proxy `/api/` ke
`engine-a`/`engine-b` lewat blok `upstream` nginx dengan `max_fails`/`fail_timeout` (pasif) —
cukup karena nginx hanya satu hop di belakang Caddy; alternatif nginx → Caddy → engine ditolak
karena menambah hop dan menyulitkan rollout.

### 3.2 Login SPA: PKCE di browser vs BFF

| Opsi | Kelebihan | Kekurangan |
|---|---|---|
| **A. Authorization Code + PKCE di SPA** (`oidc-client-ts`), token di memori, silent renew | Tanpa komponen server baru; tiga app di tiga host mendapat SSO dari sesi IdP (login sekali); SDK/worker dan UI memakai jalur verifikasi yang sama di engine (bearer); cocok dengan nginx statis | Access token hidup di memori JS (XSS → pencurian token selama masa hidupnya; dimitigasi CSP + token pendek 5–15 menit); reload halaman butuh silent renew (`prompt=none` di iframe) atau login ulang cepat lewat sesi IdP |
| B. BFF (server sesi + cookie `HttpOnly`) per app | Token tidak pernah di browser; cookie `SameSite` | Butuh proses server baru per app atau di engine (engine lalu memegang sesi pengguna dan cookie, mencampur peran); cookie per host → tiga sesi; CSRF harus ditangani; nginx statis tidak cukup |
| C. Token di `localStorage` | Bertahan reload | Dapat dibaca skrip apa pun; ditolak |

**Dipilih A.** Mitigasi: CSP ketat (5.9), access token ≤ 15 menit (konfigurasi realm), refresh
token rotation di Keycloak (verifikasi nama opsi), `useAuth()` menyimpan token di closure (bukan
`window`), interceptor transport hanya menyisipkan `Authorization` ke `baseUrl` API sama-origin. BFF
dicatat sebagai pekerjaan lanjutan bila kebijakan keamanan organisasi mensyaratkannya; kontrak
`useAuth()` tidak berubah untuk kedua opsi.

### 3.3 Zero-downtime di Compose: opsi

| Opsi | Kelebihan | Kekurangan |
|---|---|---|
| A. Satu engine, `docker compose up -d` biasa | Paling sederhana | Downtime = stop + start + pemulihan Fase 3 (detik sampai menit); worker long polling mendapat error |
| **B. Dua slot `engine-a`/`engine-b` + skrip rollout** | Bekerja dengan Compose standar; Caddy mengalihkan lewat health check `/readyz`; dua versi berjalan bersama hanya selama rollout | Dua kali sumber daya engine (ringan; verifikasi RSS dengan load test); skrip rollout milik repo harus dirawat |
| C. `docker compose up --scale engine=2` | Tanpa dua service | Compose membuat ulang **semua** container service saat image berubah (tidak rolling); nama container dinamis mempersulit Caddy |
| D. Docker Swarm (`update_config: order: start-first`) | Rolling update bawaan | Mengubah model deploy (`docker stack`), bertentangan dengan ADR-0007 tanpa manfaat lain |

**Dipilih B.** Prasyarat yang **sudah dijamin** fase sebelumnya dan diuji ulang oleh skenario chaos
`rolling-upgrade`: single writer per instance lewat row lock (Fase 2), consumer outbox dengan
advisory lock per consumer (Fase 3), sweeper `SKIP LOCKED` (Fase 4–5), migrasi dengan session lock
dan aturan expand/contract (Fase 3), lease job yang tidak terikat replika (Fase 4), long polling
yang diakhiri saat shutdown (7.2). Urutan rollout di 7.2.

### 3.4 Stack observability: opsi

| Opsi | Kelebihan | Kekurangan |
|---|---|---|
| **A. OTel SDK + OTLP → LGTM (dev) / collector + Grafana stack atau platform eksternal (prod)** | Satu instrumentasi untuk trace, metrik, log; vendor-netral; sesuai Fase 0 dan 3 | Collector sebagai komponen tambahan di prod bila tidak memakai platform eksternal |
| B. Prometheus `promhttp` + Jaeger + Loki terpisah | Masing-masing matang | Dua API instrumentasi; Fase 3 sudah memutuskan API OTel (pertanyaan 15.2 #15) |
| C. Hanya metrik + log, tanpa trace | Lebih sedikit kode | Alur asinkron (outbox, timer, worker) sulit didiagnosis tanpa trace |

**Dipilih A.**

### 3.5 Model tenant: opsi

| Opsi | Kelebihan | Kekurangan |
|---|---|---|
| **A. Kolom `tenant_id` di tabel akar + predikat tenant di setiap kueri, key global** | Sudah disiapkan Fase 1–7; tanpa migrasi key; satu pool koneksi; sesuai model Camunda 8 (verifikasi) | Isolasi bergantung pada disiplin kueri → ditegakkan lint statis dan test dua tenant |
| B. Skema Postgres per tenant (`search_path`) | Isolasi fisik dalam satu DB | Migrasi per skema; pool per tenant atau `SET search_path` per transaksi; outbox/relay per skema; tidak kompatibel dengan desain Fase 3 |
| C. Row Level Security Postgres | Isolasi di DB, bukan di kode | Butuh `SET LOCAL app.tenant` per transaksi dan role DB non-superuser; interaksi dengan `SKIP LOCKED`, advisory lock, dan janitor lintas tenant perlu kajian; bisa ditambahkan **di atas** A kelak (15) |
| D. Database per tenant | Isolasi penuh | Di luar lingkup (1) |

**Dipilih A**, dengan C dicatat sebagai pengerasan lanjutan.

### 3.6 Tata letak paket baru

```
apps/engine/internal/platform/
├─ otel/
│  ├─ otel.go                 # Setup(ctx, cfg) (shutdown func, error): resource, propagator, tracer + meter provider, exporter OTLP
│  ├─ propagate.go            # Inject(ctx) string / Extract(ctx, string) ctx — serialisasi traceparent untuk outbox dan job
│  └─ pgx_tracer.go           # pgx.QueryTracer → span (opsional, ENGINE_OTEL_DB_TRACING)
├─ logger/
│  └─ otel_handler.go         # slog.Handler pembungkus: trace_id, span_id, tenant_id, principal dari ctx
├─ rpc/                       # perluasan Fase 4
│  ├─ tracing.go              # otelconnect interceptor (server; client dipakai SDK)
│  ├─ metrics.go              # otelconnect metrics; label tenant dan principal.role (bukan subject)
│  ├─ authn/
│  │  ├─ principal.go         # Principal diperluas; FromContext / WithPrincipal
│  │  ├─ oidc.go              # Verifier: discovery, JWKS cache, pemetaan klaim
│  │  ├─ dev.go               # X-Dev-* (mode dev)
│  │  └─ anonymous.go         # mode none (Fase 4)
│  ├─ authz/
│  │  ├─ permissions.go       # daftar izin, matriks peran → izin
│  │  ├─ interceptor.go       # baca option permission dari descriptor; PermissionDenied
│  │  └─ options_test.go      # TestEveryRPCHasPermission
│  ├─ ratelimit/
│  │  └─ interceptor.go       # token bucket per kunci (principal | ip | tenant)
│  └─ tenant/
│     ├─ tenant.go            # Resolve(ctx, requestTenant) (string, error); FromContext
│     └─ interceptor.go       # memasang tenant efektif ke ctx; validasi terhadap Principal.TenantIDs
├─ admin/
│  └─ server.go               # listener admin: /metrics, /debug/pprof, /healthz, /readyz
├─ config/
│  ├─ config.go               # + grup baru
│  └─ secretfile.go           # dukungan <NAMA>_FILE
└─ httpserver/
   └─ drain.go                # Drainer: readiness false → delay → tutup notifier → Shutdown

apps/engine/internal/modules/identity/        # modul tipis tanpa DB: IdentityService.GetMe
├─ api/ domain/ application/ infrastructure/ interfaces/

libs/go/sdk/
├─ oidc/                      # ClientCredentials TokenSource (dependensi: golang.org/x/oauth2)
└─ otel/                      # WorkerTracing: span per job dengan link dari ActivatedJob.trace_context (dependensi: otel)

libs/proto/workflow/
├─ common/v1/options.proto    # extend google.protobuf.MethodOptions { string permission = 50001; }
└─ identity/v1/identity.proto # IdentityService.GetMe

infra/
├─ docker/
│  ├─ docker-compose.yml               # profil observability (otel-lgtm), auth (keycloak)
│  ├─ docker-compose.prod.yml          # caddy, engine-a/b, tiga web, postgres, db-backup, keycloak (profil auth)
│  ├─ docker-compose.observability.yml # overlay prod opsional
│  ├─ caddy/Caddyfile
│  ├─ backup/{Dockerfile,backup.sh,restore-drill.sh,basebackup.sh}
│  ├─ postgres/{pitr.conf,archive.sh}
│  └─ nginx/default.conf               # + header keamanan, CSP, upstream engine-a/b
├─ keycloak/realm-workflow.json
├─ observability/
│  ├─ otel-collector.yaml
│  ├─ grafana/{provisioning/datasources.yaml, provisioning/dashboards.yaml, provisioning/alerting.yaml, dashboards/*.json}
│  └─ prometheus/rules.yml
└─ helm/workflow-engine/               # stretch

tools/scripts/
├─ rollout/                   # Go: rollout dua slot (pull, up slot, tunggu readyz, drain slot lain)
├─ loadtest/                  # Go: skenario beban + laporan SLO
├─ chaos/                     # + skenario Fase 8
└─ docscheck/                 # pemeriksa tautan Markdown

docs/
├─ README.md                  # indeks dokumentasi
├─ ops/{incident-response,restore,upgrade,rollback,rotate-secrets,oidc-setup,tls,tenant-onboarding,capacity,observability}.md
├─ adr/README.md              # indeks ADR (hasil rekonsiliasi 9.4)
└─ licenses.md
```

## 4. Observability

### 4.1 Trace: propagasi dari RPC sampai worker

Rantai span yang harus terlihat untuk satu proses sederhana (start → service task → end):

```
[UI/SDK] POST /ProcessInstanceService/CreateProcessInstance        span klien (otelconnect client di SDK; browser tanpa span)
  └─ [engine] rpc.server CreateProcessInstance                       span server (otelconnect)
       └─ command.create_instance {process_instance_key, bpmn_process_id, tenant_id}
            ├─ db.tx (opsional, ENGINE_OTEL_DB_TRACING)
            └─ outbox.enqueue {events=2}            ← trace_context ditulis ke outbox_event.trace_context
[engine] outbox.deliver {consumer="timer.start_scheduler"}         span root baru, Link → command.create_instance
[worker] job.handle {job_type, job_key}                              span root di SDK, Link → command yang membuat job
  └─ rpc.client CompleteJob ─── traceparent header ───► [engine] rpc.server CompleteJob
                                                          └─ command.complete_job {…}  (parent/child normal)
```

Aturan:

| Batas | Mekanisme | Catatan |
|---|---|---|
| HTTP masuk/keluar | Interceptor `otelconnect` server dan client; propagator komposit `tracecontext` + `baggage` | Browser tidak mengirim `traceparent`; span server menjadi root. Header `X-Request-Id` (Fase 4) dipertahankan dan dicatat sebagai atribut `request.id` |
| RPC → command | `tracer.Start(ctx, "command."+name)` di `command_runner.go` (`execution/application`), atribut `process_instance_key`, `bpmn_process_id`, `tenant_id`, `records` (jumlah record), `lock_wait_ms` | Satu span per command; **tanpa** span per record |
| Command → outbox → consumer | Kolom baru `outbox_event.trace_context TEXT` (migrasi `000NN_otel.sql`) diisi `otel.Inject(ctx)` saat `Enqueue`; relay memanggil `otel.Extract` lalu `tracer.Start(ctx, "outbox.deliver", trace.WithLinks(link), trace.WithNewRoot())` per event (atau per batch dengan link ke setiap event; verifikasi batas jumlah link di Tempo) | Consumer lama yang tidak peduli trace tetap bekerja (kolom nullable) |
| Command → timer/subscription → trigger | Kolom `trace_context` di `timer` dan `message_subscription` (diisi projector dari record `TIMER CREATED`/`MESSAGE_SUBSCRIPTION CREATED` yang membawa `trace_context` di payload); `TriggerTimer`/`CorrelateMessage` membuat span root baru dengan link | Timer berhari-hari tidak menahan trace induk tetap "terbuka" |
| Engine → worker | Field aditif `ActivatedJob.trace_context` (string W3C) diisi dari `job.trace_context` (kolom baru di `job`, diisi saat `JOB CREATED`); `sdk/otel.WorkerTracing` membungkus handler: span `job.handle` root dengan link; `ctx` handler memuat span sehingga `CompleteJob`/`FailJob` lewat SDK menyebarkan `traceparent` | SDK inti tidak bergantung pada otel; pengguna memilih `sdk/otel` |
| Lintas instance (call activity, Fase 5) | Internal command lewat outbox → aturan outbox berlaku | — |
| Record | **Tidak** menyimpan `trace_id` di `execution_record` di 1.0 (pertanyaan 13.2 #3) | Tautan Operate → Tempo lewat `process_instance_key` sebagai atribut span sudah cukup untuk pencarian (TraceQL `{ .process_instance_key = 123 }`) |

Sampling: default produksi `OTEL_TRACES_SAMPLER=parentbased_traceidratio`,
`OTEL_TRACES_SAMPLER_ARG=0.1`; `ActivateJobs` yang kembali kosong **tidak** menghasilkan span
(interceptor memeriksa hasil dan membatalkan rekaman lewat sampler kustom `DropEmptyLongPoll`,
atau cukup atribut `rpc.result=empty` dengan sampling lebih rendah — diputuskan saat 10.2
setelah melihat volume). Error (`code != OK` selain `NotFound`/`InvalidArgument`) selalu disampel
lewat `sdktrace.Sampler` komposit (tail sampling penuh ada di collector bila dibutuhkan).

Sketsa `platform/otel`:

```go
package otel

type Config struct {
	Enabled      bool          `env:"ENGINE_OTEL_ENABLED"`            // default: true bila OTEL_EXPORTER_OTLP_ENDPOINT terisi
	ServiceName  string        `env:"OTEL_SERVICE_NAME" envDefault:"engine"`
	Endpoint     string        `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`     // mis. http://otel-lgtm:4317
	Protocol     string        `env:"OTEL_EXPORTER_OTLP_PROTOCOL" envDefault:"grpc"` // grpc | http/protobuf
	MetricPeriod time.Duration `env:"ENGINE_METRICS_INTERVAL" envDefault:"15s"`
	DBTracing    bool          `env:"ENGINE_OTEL_DB_TRACING" envDefault:"false"`
	Logs         bool          `env:"ENGINE_OTEL_LOGS" envDefault:"false"`
}

// Setup memasang provider global. Tanpa endpoint: provider no-op (perilaku Fase 3), tanpa error.
// Shutdown mem-flush exporter; dipanggil di akhir drain (7.2).
func Setup(ctx context.Context, cfg Config, version string) (shutdown func(context.Context) error, err error)

// Inject/Extract membawa konteks trace melewati batas asinkron (outbox, timer, job) sebagai
// string "traceparent[;tracestate]" (format W3C; verifikasi pemisah yang dipakai propagator).
func Inject(ctx context.Context) string
func Extract(ctx context.Context, carrier string) context.Context
func LinkFrom(carrier string) trace.Link
```

Atribut resource: `service.name`, `service.version` (dari `main.version`), `service.instance.id`
(nama container/slot dari `ENGINE_INSTANCE_ID`, default hostname), `deployment.environment`
(`ENGINE_ENV`). SDK worker memakai `OTEL_SERVICE_NAME` sendiri (mis. `worker-example`).

### 4.2 Metrik

Prinsip: nama metrik Fase 3–7 tidak berubah; label hanya dari himpunan berkardinalitas terbatas
(`tenant_id`, `bpmn_process_id`, `job_type`, `error_type`, `consumer`, `command`, `rpc.method`,
`rpc.grpc.status_code`, `role`); **dilarang** `process_instance_key`, `job_key`, `subject`,
`correlation_key` (ditegakkan `TestMetricLabels` di `platform/metrics`).

| Nama | Tipe | Label | Sumber |
|---|---|---|---|
| `rpc.server.duration`, `rpc.server.request.size`, `rpc.server.response.size` | histogram | `rpc.service`, `rpc.method`, `rpc.grpc.status_code` (+ `tenant_id` lewat atribut kustom) | otelconnect (verifikasi nama atribut yang dihasilkan versi yang dipakai) |
| `engine_process_instances_created_total`, `_completed_total`, `_terminated_total` | counter | `tenant_id`, `bpmn_process_id` | `execution/application` (dari record `PROCESS_INSTANCE_CREATION CREATED` dan `ELEMENT_COMPLETED/TERMINATED` tipe `PROCESS`) |
| `engine_process_instances_active` | observable gauge | `tenant_id` | kueri `COUNT(*) ... WHERE state='ACTIVE'` tiap `ENGINE_METRICS_INTERVAL` (indeks parsial Fase 2) |
| `engine_jobs_created_total`, `_activated_total`, `_completed_total`, `_failed_total`, `_error_thrown_total`, `_timed_out_total` | counter | `tenant_id`, `job_type` | modul `job`/`execution` (`job_timeouts_total` Fase 4 diganti nama secara kompatibel: nama lama tetap diekspor satu rilis dengan deprecation) |
| `engine_jobs_queue_depth` | observable gauge | `job_type` | `COUNT(*) GROUP BY type WHERE state='CREATED' AND activatable_at <= now()` tiap interval; dibatasi 100 tipe teratas |
| `engine_jobs_activation_wait_seconds` | histogram | `job_type` | `activated_at - created_at` saat aktivasi |
| `engine_job_long_poll_active` | up-down counter | `job_type` | jumlah `ActivateJobs` yang sedang menunggu |
| `engine_incidents_created_total`, `_resolved_total` | counter | `tenant_id`, `error_type` | `execution/application` |
| `engine_incidents_open` | observable gauge | `tenant_id` | kueri `incident WHERE state='CREATED'` |
| `engine_timer_trigger_lag_seconds` | histogram | — | alias `engine_timer_trigger_latency_seconds` Fase 5 (`TRIGGERED - due_at`); nama Fase 5 dipertahankan, Fase 8 menambah panel |
| `engine_timers_due_backlog` | observable gauge | — | `COUNT(*) WHERE state IN ('CREATED','DUE') AND due_at < now()` |
| `engine_outbox_lag_seconds`, `engine_outbox_pending_events` | gauge | `consumer` | Fase 3 (tidak berubah) |
| `engine_message_buffered`, `engine_message_correlated_total` | gauge, counter | `path` | Fase 5 |
| `engine_usertasks_open`, `engine_usertask_search_duration_seconds` | gauge, histogram | `tenant_id` | modul `usertask` (metrik yang diminta Fase 6) |
| `engine_command_duration_seconds`, `engine_command_lock_wait_seconds` | histogram | `command` | Fase 3 |
| `db.client.connection.count` (`state=idle|used`), `db.client.connection.wait_time`, `db.client.connection.max` | observable gauge / histogram | `pool.name` | `pgxpool.Stat()` tiap interval; nama mengikuti semconv OTel (verifikasi versi semconv; nama lama `engine_db_pool_*` Fase 3 tetap) |
| `feel_eval_duration_seconds`, `feel_cache_hits_total`, `feel_cache_misses_total` | histogram, counter | — | Fase 7 |
| `engine_auth_requests_total` | counter | `mode`, `outcome` (`ok`, `missing`, `invalid`, `expired`, `denied`) | interceptor authn/authz |
| `engine_rate_limited_total` | counter | `key_kind` | interceptor ratelimit |
| `engine_tenant_quota_rejections_total` | counter | `tenant_id`, `kind` | tenant |
| `engine_build_info` | gauge (1) | `version`, `go_version` | start |
| `process.runtime.go.*` (goroutine, GC, heap) | — | — | `go.opentelemetry.io/contrib/instrumentation/runtime` (verifikasi nama paket) |

Metrik gauge berbasis kueri dikumpulkan oleh satu goroutine `metrics.Collector` dengan satu
koneksi pool dan `statement_timeout` 5 s; bila kueri gagal, nilai terakhir dipertahankan dan
`engine_metrics_collect_errors_total` naik. Pada multi-replika, gauge berbasis DB dilaporkan oleh
**setiap** replika dengan nilai yang sama → dashboard memakai `max by (tenant_id)` (dicatat di
dashboard, bukan dikoordinasikan lewat lock).

### 4.3 Log

Handler pembungkus di `platform/logger/otel_handler.go`:

```go
// Handle menambah atribut korelasi dari context sebelum meneruskan ke handler JSON.
func (h *ctxHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r.AddAttrs(slog.String("trace_id", sc.TraceID().String()), slog.String("span_id", sc.SpanID().String()))
	}
	if t, ok := tenant.FromContext(ctx); ok {
		r.AddAttrs(slog.String("tenant_id", t))
	}
	if p, ok := authn.FromContext(ctx); ok && !p.Anonymous {
		r.AddAttrs(slog.String("principal", p.Subject)) // subject, bukan nama; tidak pernah token
	}
	return h.next.Handle(ctx, r)
}
```

Konvensi field (melengkapi Fase 3 bagian 10): `component`, `rpc` (`workflow.job.v1.JobService/ActivateJobs`),
`code`, `duration_ms`, `request_id`, `trace_id`, `span_id`, `tenant_id`, `principal`,
`process_instance_key`, `job_key`. `ENGINE_LOG_FORMAT=json|text` (text untuk `make dev`).
Redaksi: nilai header `Authorization`, variabel proses, dan payload request **tidak pernah**
di-log di level ≥ INFO; level DEBUG boleh mencatat ukuran, bukan isi. Log nginx dan Caddy memakai
format JSON (`log { format json }` Caddy; `log_format json` nginx) agar Loki bisa memparsenya.

Pengiriman ke Loki: collector dengan receiver `filelog` membaca `/var/lib/docker/containers/*/*-json.log`
(mount read-only) dan parser JSON Docker, atau Grafana Alloy dengan `loki.source.docker` —
keduanya didokumentasikan di `docs/ops/observability.md`; profil dev LGTM memakai cara pertama
(verifikasi apakah image LGTM mengaktifkan receiver `filelog` secara default; bila tidak, collector
kecil terpisah ditambahkan ke profil).

### 4.4 Konfigurasi exporter

| Variabel | Default | Keterangan |
|---|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | kosong (no-op) | dev: `http://localhost:4317`; prod compose: `http://otel-collector:4317` atau endpoint eksternal |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `grpc` | `http/protobuf` untuk proxy HTTP |
| `OTEL_EXPORTER_OTLP_HEADERS` | kosong | kredensial platform eksternal (`authorization=Basic …`), diisi lewat `_FILE` bila rahasia |
| `OTEL_SERVICE_NAME` | `engine` | — |
| `OTEL_RESOURCE_ATTRIBUTES` | kosong | `deployment.environment=prod,service.namespace=workflow` |
| `OTEL_TRACES_SAMPLER` / `_ARG` | `parentbased_always_on` (dev), `parentbased_traceidratio` / `0.1` (`.env.prod.example`) | — |
| `ENGINE_OTEL_ENABLED` | `true` bila endpoint terisi | matikan paksa |
| `ENGINE_METRICS_INTERVAL` | `15s` | periode reader metrik dan collector gauge |
| `ENGINE_OTEL_DB_TRACING` | `false` | span per kueri pgx (berisik; untuk diagnosis) |
| `ENGINE_OTEL_LOGS` | `false` | ekspor log lewat OTLP selain stdout |
| `ENGINE_METRICS_PROMETHEUS` | `false` | `/metrics` di listener admin (exporter Prometheus OTel) |
| `ENGINE_ADMIN_ADDR` | `:9090` | listener admin (7.5) |
| `ENGINE_PPROF_ENABLED` | `false` | `/debug/pprof` di listener admin |
| `ENGINE_INSTANCE_ID` | hostname | `service.instance.id`; compose mengisi `engine-a`/`engine-b` |

SDK worker membaca env standar OTel yang sama lewat `sdk/otel.Setup()` (opsional).

### 4.5 Profil Compose `observability`

Dev (`infra/docker/docker-compose.yml`), menggantikan `otel-collector` kosong:

```yaml
  otel-lgtm:
    image: grafana/otel-lgtm:latest      # verifikasi: kunci ke tag versi saat 10.3
    profiles: ['observability']
    ports:
      - '127.0.0.1:${GRAFANA_PORT:-3000}:3000'
      - '127.0.0.1:4317:4317'
      - '127.0.0.1:4318:4318'
    volumes:
      - ../observability/grafana/dashboards:/otel-lgtm/grafana/conf/provisioning/dashboards/workflow:ro   # path verifikasi
      - ../observability/grafana/provisioning/alerting.yaml:/otel-lgtm/grafana/conf/provisioning/alerting/workflow.yaml:ro
      - lgtm-data:/data
    environment:
      ENABLE_LOGS_ALL: 'false'
```

`make up PROFILES=observability` + `make dev` dengan `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317`
(ditambahkan ke `.env.example` sebagai komentar) → Grafana `http://localhost:3000` (`make observe`
membuka URL). Bila path provisioning image LGTM berbeda, profil memakai collector + Grafana terpisah
(overlay yang sama dengan prod).

Prod opsional (`docker-compose.observability.yml`, dipakai dengan `-f` kedua): `otel-collector`
(`otel/opentelemetry-collector-contrib`, konfigurasi `infra/observability/otel-collector.yaml`:
receiver OTLP + filelog, exporter Prometheus remote write/Tempo/Loki), `prometheus` (atau Mimir),
`tempo`, `loki`, `grafana` dengan provisioning yang sama; Grafana diekspos lewat Caddy di
`grafana.{$DOMAIN}` dengan auth Grafana sendiri (OIDC Grafana ke Keycloak opsional, didokumentasikan).
Retensi default: metrik 15 hari, trace 3 hari, log 7 hari (variabel env overlay).

### 4.6 Dashboard, alert, dan SLO

Dashboard (`infra/observability/grafana/dashboards/`):

| File | Panel utama |
|---|---|
| `engine-overview.json` | RPS dan error ratio per service/method; p50/p95/p99 `rpc.server.duration`; instance dibuat/selesai/diterminasi per menit; instance aktif; incident terbuka; versi engine per slot; readiness |
| `jobs-workers.json` | queue depth per tipe; activation wait p95; aktivasi/selesai/gagal/time-out per tipe; long poll aktif; worker (dari `rpc.client` metrik SDK bila dikirim) |
| `timers-messages-outbox.json` | timer backlog dan lag; message buffered dan korelasi per jalur; outbox lag/pending/dead letter per consumer; cross-instance |
| `database-runtime.json` | pool used/idle/wait; `pg_stat_*` lewat `postgres_exporter` opsional (verifikasi image) atau kueri collector; goroutine, heap, GC; lock wait command; durasi command |
| `security-tenants.json` | auth outcome per mode; rate limited; kuota tenant; instance aktif per tenant |

Aturan alert (Grafana provisioning + salinan `prometheus/rules.yml`), semua dengan `for` dan label
`severity`:

| Alert | Kondisi | Severity |
|---|---|---|
| `EngineDown` | `up{job="engine"} == 0` atau tidak ada `engine_build_info` 2 m | critical |
| `EngineNotReady` | `/readyz` gagal (metrik `engine_ready == 0`) > 2 m pada semua slot | critical |
| `RpcErrorRatioHigh` | rasio `status_code` ∉ {OK, NotFound, InvalidArgument, FailedPrecondition, Unauthenticated, PermissionDenied} > 1 % selama 5 m | warning (critical > 5 %) |
| `RpcLatencyHigh` | p95 `CreateProcessInstance`/`CompleteJob` > 500 ms selama 10 m | warning |
| `OutboxLagHigh` | `engine_outbox_lag_seconds > 30` selama 5 m | warning |
| `OutboxDeadLetter` | `increase(engine_outbox_dead_letter_total[10m]) > 0` | warning |
| `TimerLagHigh` | p95 `engine_timer_trigger_lag_seconds > 30` atau `engine_timers_due_backlog > 1000` selama 5 m | warning |
| `JobQueueGrowing` | `engine_jobs_queue_depth` naik monoton 15 m dan `engine_jobs_activated_total` rate = 0 untuk tipe itu | warning |
| `IncidentsRising` | `increase(engine_incidents_created_total[15m]) > 50` | warning |
| `DbPoolSaturated` | `db.client.connection.count{state="used"} / max > 0.9` selama 5 m atau wait p95 > 100 ms | warning |
| `PostgresDiskLow` | penggunaan volume > 80 % (node exporter / `df` via collector `hostmetrics`) | warning (critical > 90 %) |
| `BackupStale` | `time() - backup_last_success_timestamp > 26h` (dari file status yang di-scrape `textfile`/collector) | critical |
| `WalArchiveFailing` | `pg_stat_archiver.failed_count` naik (bila PITR aktif) | critical |
| `AuthFailuresSpike` | `rate(engine_auth_requests_total{outcome="invalid"}[5m]) > 5/s` | warning |
| `CertificateExpiring` | metrik Caddy sertifikat < 7 hari (verifikasi ketersediaan metrik; cadangan: cek `openssl s_client` oleh skrip backup harian) | warning |

SLO (didefinisikan sebagai recording rule, dipantau di panel "SLO" dashboard overview, dan
menjadi kriteria lulus load test 8.3):

| SLI | Target | Jendela |
|---|---|---|
| Ketersediaan API: rasio RPC sukses (bukan error server `Internal`/`Unavailable`/`DeadlineExceeded` sisi engine) | ≥ 99,9 % | 30 hari |
| Latensi `CreateProcessInstance` p95 | ≤ 200 ms pada beban referensi | 5 menit |
| Latensi `CompleteJob` p95 | ≤ 200 ms | 5 menit |
| Latensi `ActivateJobs` dengan job tersedia p95 | ≤ 50 ms | 5 menit |
| Keterlambatan timer p95 (`TRIGGERED - due_at`) | ≤ 2 s | 5 menit |
| Lag outbox p95 | ≤ 1 s | 5 menit |
| Korelasi message end-to-end p95 | ≤ 500 ms | 5 menit |
| Kesegaran backup | ≤ 26 jam | selalu |

Angka adalah target awal untuk mesin referensi (8.2) dan dikunci setelah load test pertama.

### 4.7 Hook instrumentasi yang harus sudah ada dari fase sebelumnya

Diperiksa di 10.1; bila belum ada, ditambahkan di Fase 8 sebagai perubahan kecil pada modul
pemilik:

| Fase | Hook | Dipakai Fase 8 untuk |
|---|---|---|
| 3 | `platform/metrics` (API OTel, daftar nama + test snapshot); log terstruktur tahap pemulihan; `engine_command_*`, `engine_outbox_*` | Exporter dan dashboard tanpa perubahan nama |
| 3 | `outbox.Writer.Enqueue(ctx, tx, …)` menerima `ctx` command | `trace_context` diambil dari `ctx` |
| 3 | Consumer `Handle(ctx, tx, ev)` | `ctx` diisi span dengan link |
| 4 | Rantai interceptor di satu tempat (`platform/rpc`), `authn.Principal`, `X-Request-Id`, log per RPC | Sisipan `Tracing`, `Metrics`, `Authz`, `RateLimit`, `Tenant` |
| 4 | `ActivatedJob` membawa `tenant_id`; `job_timeouts_total` | `trace_context` aditif; metrik job |
| 5 | `engine_timer_trigger_latency_seconds`, `engine_message_*`, `engine_cross_instance_total`; `clock.Clock` | Panel timer/message |
| 6 | Hook query dengan `queryKey` stabil; `config.js` runtime; `AuthProvider` | Pengganti auth; (SSE bila dikerjakan) |
| 7 | `feel_*` metrik | Panel FEEL |
| 2–7 | Atribut `process_instance_key`, `bpmn_process_id` tersedia di `command_runner` | Atribut span |

## 5. Keamanan

### 5.1 Autentikasi OIDC di interceptor Connect

Rantai interceptor final (luar → dalam), menggantikan urutan Fase 4 bagian 8.6:

```
1. Recover      (Fase 4)
2. Tracing      otelconnect — span server, ekstraksi traceparent
3. Logging      (Fase 4; kini memuat trace_id, principal, tenant)
4. Metrics      otelconnect — rpc.server.*
5. Timeout      (Fase 4)
6. Authn        OIDC | dev | none → Principal di ctx
7. Authz        izin RPC dari option proto vs Principal.Permissions
8. RateLimit    bucket per principal / ip / tenant
9. Tenant       tenant efektif dari request + Principal → ctx
10. Validate    protovalidate (Fase 4)
```

`Validate` tetap paling dalam agar penolakan validasi hanya terjadi setelah caller terautentikasi
(mengurangi informasi bagi anonim). Endpoint non-Connect (`/healthz`, `/readyz`, `/csp-report`
di nginx) tidak melewati rantai.

Konfigurasi:

| Variabel | Default | Keterangan |
|---|---|---|
| `ENGINE_ENV` | `development` | `production` menolak `ENGINE_AUTH_MODE` ≠ `oidc`, CORS non-kosong tanpa `ENGINE_CORS_ALLOW_IN_PROD=true`, `ENGINE_PPROF_ENABLED=true` tanpa admin addr terpisah |
| `ENGINE_AUTH_MODE` | `oidc` | `oidc` / `dev` / `none` |
| `ENGINE_OIDC_ISSUER` | — (wajib di mode `oidc`) | nilai `iss` yang diharapkan, mis. `https://auth.example.com/realms/workflow` |
| `ENGINE_OIDC_DISCOVERY_URL` | `<issuer>/.well-known/openid-configuration` | override bila engine menjangkau IdP lewat alamat internal (`http://keycloak:8080/realms/workflow/...`) |
| `ENGINE_OIDC_AUDIENCES` | kosong (tidak dicek) | daftar `aud` yang diterima; `.env.prod.example` mengisi `workflow-engine` |
| `ENGINE_OIDC_JWKS_CACHE_TTL` | `1h` | refresh JWKS; `kid` tak dikenal memicu refresh segera (dibatasi 1×/menit) |
| `ENGINE_OIDC_CLOCK_SKEW` | `30s` | toleransi `exp`/`nbf`/`iat` |
| `ENGINE_OIDC_USERNAME_CLAIM` | `preferred_username` | `Principal.Name` |
| `ENGINE_OIDC_ROLES_CLAIM` | `realm_access.roles` | path titik ke array string (Keycloak realm roles); alternatif `resource_access.workflow-engine.roles`, `roles` |
| `ENGINE_OIDC_GROUPS_CLAIM` | `groups` | candidate groups user task |
| `ENGINE_OIDC_TENANT_CLAIM` | `tenant_ids` | array string atau string tunggal |
| `ENGINE_OIDC_ROLE_PREFIX` | kosong | mis. `wf:` → hanya role berawalan itu yang dipetakan, prefiks dibuang |
| `ENGINE_OIDC_REQUIRED_SCOPE` | kosong | bila diisi, `scope` token harus memuatnya |

Sketsa verifier:

```go
package authn

type Principal struct {
	Subject     string
	Name        string
	Roles       []string          // setelah prefix dibuang
	Groups      []string
	TenantIDs   []string          // kosong + multi-tenancy off → ["<default>"]
	Permissions map[string]bool   // dihitung dari Roles (authz.PermissionsFor)
	Anonymous   bool
	Issuer      string
	ExpiresAt   time.Time
}

type OIDC struct {
	verifier *oidc.IDTokenVerifier // go-oidc; verifikasi: Verify() memeriksa iss, aud (bila SkipClientIDCheck=false), exp, tanda tangan via KeySet dengan cache
	cfg      Config
	log      *slog.Logger
	meter    metrics.Meter
}

func NewOIDC(ctx context.Context, cfg Config) (*OIDC, error) {
	// Discovery dari cfg.DiscoveryURL (atau issuer), tetapi klaim iss dicocokkan dengan cfg.Issuer:
	// oidc.ProviderConfig{IssuerURL: cfg.Issuer, JWKSURL: discovered.JWKS}.NewProvider(ctx) (verifikasi API)
	// Verifier: provider.VerifierContext(ctx, &oidc.Config{SkipClientIDCheck: len(cfg.Audiences)==0, ...})
}

// Authenticate dipanggil interceptor untuk setiap request.
func (o *OIDC) Authenticate(ctx context.Context, header http.Header) (Principal, error) {
	raw, ok := bearer(header.Get("Authorization"))
	if !ok { return Principal{}, rpc.Reject(connect.CodeUnauthenticated, "TOKEN_MISSING", "missing bearer token", nil) }
	tok, err := o.verifier.Verify(ctx, raw)
	if err != nil {
		reason := "TOKEN_INVALID"
		if errors.As(err, &oidc.TokenExpiredError{}) { reason = "TOKEN_EXPIRED" }  // verifikasi tipe error go-oidc
		return Principal{}, rpc.Reject(connect.CodeUnauthenticated, reason, "invalid token", nil)
	}
	var claims map[string]any
	if err := tok.Claims(&claims); err != nil { return Principal{}, rpc.Reject(connect.CodeUnauthenticated, "TOKEN_INVALID", "unreadable claims", nil) }
	if len(o.cfg.Audiences) > 0 && !audienceMatches(tok.Audience, o.cfg.Audiences) {
		return Principal{}, rpc.Reject(connect.CodeUnauthenticated, "TOKEN_INVALID", "audience mismatch", nil)
	}
	p := Principal{
		Subject:   tok.Subject,
		Name:      stringClaim(claims, o.cfg.UsernameClaim),
		Roles:     stripPrefix(stringsClaim(claims, o.cfg.RolesClaim), o.cfg.RolePrefix),
		Groups:    stringsClaim(claims, o.cfg.GroupsClaim),
		TenantIDs: stringsClaim(claims, o.cfg.TenantClaim),
		Issuer:    tok.Issuer,
		ExpiresAt: tok.Expiry,
	}
	p.Permissions = authz.PermissionsFor(p.Roles)
	return p, nil
}
```

`stringsClaim` menerima path bertitik (`realm_access.roles`), array string, atau string tunggal
dipisah spasi/koma. Kegagalan JWKS (IdP tidak terjangkau) **tidak** menolak token yang `kid`-nya
sudah ada di cache; `kid` baru saat IdP mati → `Unavailable` + `ErrorInfo{reason:"IDP_UNAVAILABLE"}`
(bukan `Unauthenticated`) agar SDK mencoba ulang. Perilaku ini diuji skenario chaos
`partition-engine-idp`.

Mode `dev` (`authn.Dev`): header `X-Dev-User` (subject), `X-Dev-Groups`, `X-Dev-Roles` (default
`admin` bila kosong agar e2e lama tetap jalan), `X-Dev-Tenants`; tanpa header → anonim seperti mode
`none`. UI (`app-shell`) menampilkan `DevIdentityPicker` hanya bila `AUTH_MODE=dev` di `config.js`.

Mode `none`: `Principal{Subject: "anonymous", Anonymous: true, Roles: ["admin"]}` — anonim
mendapat semua izin agar test dan paritas Fase 1–7 tidak berubah (dibedakan dari mode `dev` yang
dapat memberi peran terbatas).

### 5.2 Identitas → tenant dan peran

- `Principal.TenantIDs` kosong: bila `ENGINE_MULTI_TENANCY=false` → `["<default>"]`; bila `true`
  → principal **tidak** bisa melakukan operasi apa pun yang butuh tenant (`PermissionDenied`
  `TENANT_FORBIDDEN`) kecuali `admin` yang memakai `tenant_id` eksplisit di request (admin tanpa
  klaim tenant = akses semua tenant, lihat 6.1).
- Peran `admin` dapat berasal dari klaim atau dari `ENGINE_ADMIN_SUBJECTS` (daftar subject
  bootstrap untuk instalasi pertama, dicatat WARN saat dipakai).
- Nama peran dibandingkan case-sensitive setelah prefix dibuang; peran tak dikenal diabaikan dan
  dihitung di `engine_auth_unknown_roles_total`.

### 5.3 Otorisasi RBAC

Izin (`resource:action`) dan matriks peran:

| Izin | admin | operator | developer | worker | user |
|---|---|---|---|---|---|
| `deployment:read` (`GetProcessDefinition`, `ListProcessDefinitions`, `GetForm`, decision read) | ✓ | ✓ | ✓ | ✓ | ✓ |
| `deployment:write` (`DeployResource`, `ValidateResource`) | ✓ | — | ✓ | — | — |
| `process:create` (`CreateProcessInstance[WithResult]`) | ✓ | ✓ | ✓ | ✓ | — |
| `process:read` (`Get/List*`, records, statistik, incident list, timer/subscription list) | ✓ | ✓ | ✓ | ✓ | ✓ (lihat 13.2 #6) |
| `process:modify` (`CancelProcessInstance`, `SetVariables`, `ResolveIncident`, `UpdateJobRetries`, `UpdateJobTimeout`) | ✓ | ✓ | ✓ (non-prod) | — | — |
| `job:activate` (`ActivateJobs`) | ✓ | — | ✓ | ✓ | — |
| `job:complete` (`CompleteJob`, `FailJob`, `ThrowError`) | ✓ | ✓ (aksi debug Operate) | ✓ | ✓ | — |
| `job:read` (`ListJobs`) | ✓ | ✓ | ✓ | ✓ | — |
| `message:publish` | ✓ | ✓ | ✓ | ✓ | — |
| `usertask:read` (`SearchUserTasks`, `GetUserTask`, `GetUserTaskForm`) | ✓ | ✓ | ✓ | — | ✓ |
| `usertask:claim` (`Assign` ke diri sendiri / `Unassign` milik sendiri) | ✓ | ✓ | ✓ | — | ✓ |
| `usertask:assign` (`Assign` ke orang lain, `allow_override`, `UpdateUserTask`) | ✓ | ✓ | — | — | — |
| `usertask:complete` (`CompleteUserTask`; kepemilikan ditegakkan modul `usertask` kecuali `allow_override` + `usertask:assign`) | ✓ | ✓ | ✓ | — | ✓ |
| `expression:evaluate` (`EvaluateExpression`), `decision:evaluate` (7b) | ✓ | ✓ | ✓ | ✓ | — |
| `identity:me` (`GetMe`) | ✓ | ✓ | ✓ | ✓ | ✓ |
| `admin:*` (RPC admin masa depan; CLI `engine verify/rebuild` tidak lewat RPC) | ✓ | — | — | — | — |

"✓ (non-prod)" berarti izin diberikan bila `ENGINE_ENV != production` (developer boleh memodifikasi
instance di staging). Matriks adalah tabel Go di `authz/permissions.go` dan diuji
`TestRoleMatrix` (snapshot). Kustomisasi peran per instalasi (mis. memberi `operator` izin
`deployment:write`) lewat `ENGINE_AUTHZ_EXTRA_GRANTS="operator=deployment:write;user=process:read"`
(sintaks sederhana; file kebijakan penuh ditunda).

Deklarasi di proto:

```proto
// workflow/common/v1/options.proto
syntax = "proto3";
package workflow.common.v1;
import "google/protobuf/descriptor.proto";

extend google.protobuf.MethodOptions {
  // Izin yang dibutuhkan untuk memanggil RPC. Wajib untuk setiap RPC (ditegakkan test).
  string permission = 50001;
}

// contoh di workflow/job/v1/job.proto
rpc ActivateJobs(ActivateJobsRequest) returns (ActivateJobsResponse) {
  option (workflow.common.v1.permission) = "job:activate";
}
```

Interceptor:

```go
func (a *Authz) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		perm, ok := a.permissions[req.Spec().Procedure] // dibangun sekali dari protoregistry.GlobalFiles (opsi permission)
		if !ok {
			return nil, rpc.Reject(connect.CodeInternal, "PERMISSION_UNDECLARED", "rpc has no permission option", nil) // tidak boleh terjadi: test menjaga
		}
		p, _ := authn.FromContext(ctx)
		if !p.Permissions[perm] && !p.Permissions["admin:*"] {
			a.meter.Denied(perm)
			return nil, rpc.Reject(connect.CodePermissionDenied, "PERMISSION_DENIED", "missing permission", map[string]string{"permission": perm})
		}
		return next(ctx, req)
	}
}
```

Nomor field extension `50001` berada di rentang yang diizinkan protobuf untuk pemakaian internal
(verifikasi rentang dan pastikan tidak bentrok dengan `buf.validate` / protovalidate). Lint:
`buf lint` tidak memeriksa kehadiran opsi, sehingga `TestEveryRPCHasPermission` di
`platform/rpc/authz` mengiterasi semua service `workflow.*` di registry dan gagal bila ada RPC
tanpa opsi atau dengan izin yang tidak ada di daftar.

Modul `usertask` (Fase 6 #12–13): bila `Principal.Anonymous == false`, `SearchUserTasks{claimable_by}`
dari request diabaikan dan diganti dari `Principal` (subject + groups); `CompleteUserTask`/`Unassign`
oleh bukan assignee ditolak `PermissionDenied` `USER_TASK_NOT_ASSIGNEE` kecuali `allow_override` dan
`usertask:assign`.

### 5.4 Service account untuk worker (client credentials)

```go
// libs/go/sdk/oidc/client_credentials.go
package oidc

// ClientCredentials mengembalikan sdk.TokenSource yang mengambil token lewat grant
// client_credentials dari <issuer>/.well-known/openid-configuration (token_endpoint),
// menyimpan di cache, dan memperbarui 30 detik sebelum kedaluwarsa.
func ClientCredentials(ctx context.Context, issuer, clientID, clientSecret string, scopes ...string) (sdk.TokenSource, error)
```

Implementasi memakai `golang.org/x/oauth2/clientcredentials` + discovery sederhana (HTTP GET
JSON; tanpa go-oidc agar dependensi SDK kecil). `worker-example` membaca `WORKER_OIDC_ISSUER`,
`WORKER_OIDC_CLIENT_ID`, `WORKER_OIDC_CLIENT_SECRET[_FILE]`. Realm Keycloak contoh mendefinisikan
client `worker-example` (confidential, service account diaktifkan, role `worker`). Token
kedaluwarsa di tengah long polling: engine menolak `Unauthenticated TOKEN_EXPIRED` → SDK (Fase 4
retry) mengambil token baru dan mengulang `ActivateJobs` tanpa backoff tambahan; `CompleteJob`
dengan `request_id` (Fase 3) tetap idempoten.

Untuk `ActivateJobs` yang berjalan sampai 60 s, SDK memperbarui token **sebelum** memulai poll
bila sisa masa berlaku < `request_timeout + 10 s`.

### 5.5 Alur login UI

- `app-shell/auth-context.tsx`: `AuthProvider` membaca `window.__APP_CONFIG__.AUTH_MODE`:
  `oidc` → `UserManager` dari `oidc-client-ts` (`authority = OIDC_ISSUER`, `client_id =
  OIDC_CLIENT_ID`, `redirect_uri = <origin>/auth/callback`, `response_type = code`, PKCE default,
  `scope = openid profile email`, `automaticSilentRenew = true`, `userStore` **in-memory**
  (`InMemoryWebStorage`), `silent_redirect_uri = <origin>/auth/silent`); `dev` → picker identitas
  dev (Fase 6); `none` → anonim.
- Route `/auth/callback` dan `/auth/silent` (halaman statis kecil, bagian `app-shell`).
- Saat start: `getUser()`; tidak ada → `signinRedirect()` (menyimpan `returnTo`). Setelah login,
  `GetMe` dipanggil sekali dan hasilnya (`roles`, `permissions`, `tenants`) masuk `useAuth()`;
  komponen memakai `useAuth().can('process:modify')` untuk menyembunyikan tombol.
- Transport: `createEngineTransport(baseUrl, { getToken })` menambah `Authorization: Bearer`
  untuk setiap request; respons `Unauthenticated` → `signinSilent()` sekali lalu ulangi; gagal →
  `signinRedirect()`.
- Logout: `signoutRedirect()` ke end-session endpoint IdP; `post_logout_redirect_uri = <origin>`.
- Multi-tenant di UI: `useAuth().tenants`; pemilih tenant di header bila > 1; tenant terpilih
  dikirim sebagai `tenant_id`/`tenant_ids` di request oleh lapisan `infrastructure` fitur (dibaca
  dari `TenantContext` di `app-shell`); disimpan di `sessionStorage` (per-viewer).
- Realm Keycloak contoh: client `modeler-web`, `operate-web`, `tasklist-web` (public, PKCE
  S256, `Valid redirect URIs` = `https://<app>.<domain>/auth/*` dan `http://localhost:420x/auth/*`,
  `Web origins` = origin app), mapper klaim `tenant_ids` (atribut pengguna) dan `groups`;
  pengguna contoh `admin`, `operator`, `developer`, `alice` (`user`, grup `sales`), `bob` (`user`,
  grup `finance`); client `worker-example`. Password contoh hanya di realm dev dan dicatat di
  `docs/ops/oidc-setup.md` sebagai contoh, bukan di kode.

Playwright: fixture `login(role)` mengisi form Keycloak (dev) → e2e Fase 6 dijalankan ulang untuk
tiga peran; test negatif: `user` membuka `/instances/$key` tidak melihat tombol Cancel dan
memanggil `CancelProcessInstance` via client → `PermissionDenied`.

### 5.6 CORS, rate limiting, dan batas request

- CORS (Fase 0/4) tidak berubah secara mekanis; produksi `ENGINE_CORS_ORIGINS=` kosong karena
  nginx mem-proxy `/api/` sama-origin. `ENGINE_ENV=production` dengan origin terisi menolak start
  kecuali `ENGINE_CORS_ALLOW_IN_PROD=true` (untuk deploy yang memisahkan host API dan UI).
  `Access-Control-Allow-Credentials` tidak pernah dikirim (token di header, bukan cookie).
- Rate limiting:

| Variabel | Default | Keterangan |
|---|---|---|
| `ENGINE_RATE_LIMIT_ENABLED` | `true` | — |
| `ENGINE_RATE_LIMIT_RPS` | `200` | per kunci, per replika |
| `ENGINE_RATE_LIMIT_BURST` | `400` | — |
| `ENGINE_RATE_LIMIT_KEY` | `principal` | `principal` (subject; anonim → IP) / `tenant` / `ip` |
| `ENGINE_RATE_LIMIT_EXEMPT_ROLES` | `admin` | — |
| `ENGINE_RATE_LIMIT_TRUST_PROXY` | `true` | pakai `X-Forwarded-For` hop pertama (Caddy menuliskannya) hanya bila sumber di `ENGINE_TRUSTED_PROXIES` (default jaringan Docker compose) |

Implementasi: `map[string]*rate.Limiter` dengan LRU 10.000 kunci (menghindari pertumbuhan
tak terbatas), `Allow()` per request; `ActivateJobs` dihitung satu; penolakan `ResourceExhausted`
+ `ErrorInfo{reason:"RATE_LIMITED", metadata:{retry_after_ms}}` + header `Retry-After`. SDK Fase 4
memperlakukan `ResourceExhausted` sebagai retryable dengan backoff dari `retry_after_ms`.

- Batas ukuran: Connect 16 MiB tetap (resource BPMN besar dan variabel 1 MiB sudah di bawahnya);
  Caddy `request_body max_size 20MB`; nginx `client_max_body_size 20m`; `httpserver.ReadHeaderTimeout`
  10 s, `MaxHeaderBytes` 64 KiB (token JWT besar dengan banyak klaim masih di bawah 8 KiB).

### 5.7 Manajemen rahasia

- Setiap field string `config.Config` dapat diisi dari `<NAMA>_FILE` (isi file di-trim newline);
  bila keduanya diisi, `_FILE` menang dan WARN dicatat. Implementasi di `config/secretfile.go`
  sebagai pra-proses env sebelum `env.Parse` (iterasi field lewat refleksi tag `env`).
- Compose prod:

```yaml
secrets:
  postgres_password:
    file: ${SECRETS_DIR:-./secrets}/postgres_password
  oidc_client_secret:          # hanya bila engine memanggil IdP (tidak dibutuhkan untuk verifikasi JWT)
    file: ${SECRETS_DIR:-./secrets}/oidc_client_secret
services:
  engine-a:
    secrets: [postgres_password]
    environment:
      DATABASE_URL_FILE: /run/secrets/database_url      # atau DATABASE_URL dirakit dari POSTGRES_PASSWORD_FILE
```

  Karena `DATABASE_URL` memuat password, engine mendukung **dua** cara: `DATABASE_URL_FILE`, atau
  `DATABASE_URL` tanpa password + `PGPASSWORD_FILE` (pgx membaca `PGPASSWORD`? — tidak dari file;
  jadi engine merakit: bila `ENGINE_DB_PASSWORD_FILE` terisi, password disisipkan ke URL sebelum
  `pgxpool.ParseConfig`). `make secrets-init` membuat folder `secrets/` (di-ignore git) dengan
  password acak.
- Rotasi: `docs/ops/rotate-secrets.md` — password DB (buat user baru → ganti secret → rollout →
  hapus user lama), client secret worker, kunci JWKS (otomatis oleh IdP; engine mengikuti `kid`).
- `Config.String()`/log start mencetak konfigurasi dengan nilai rahasia disamarkan (`***`);
  test memastikan tidak ada field bertag `secret:"true"` yang bocor.
- Secrets tidak pernah dilewatkan sebagai argumen proses (terlihat di `ps`).

### 5.8 Pemindaian dependensi, SBOM, dan supply chain

CI job `security` (per PR dan nightly):

```yaml
  security:
    name: Security scan
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5
      - uses: actions/setup-go@v6
        with: { go-version-file: go.mod }
      - run: go install golang.org/x/vuln/cmd/govulncheck@latest && govulncheck ./...
      - uses: pnpm/action-setup@v4
      - run: pnpm install --frozen-lockfile && pnpm audit --audit-level=high
      - uses: aquasecurity/trivy-action@master          # kunci ke versi/SHA saat 10.9
        with: { scan-type: fs, severity: CRITICAL,HIGH, ignore-unfixed: true, exit-code: 1 }
```

Job `docker` menambah setelah build: Trivy image scan untuk keempat image (`exit-code: 1` pada
CRITICAL/HIGH yang punya fix; pengecualian di `.trivyignore` dengan alasan dan tanggal kedaluwarsa),
SBOM SPDX per image (`anchore/sbom-action` / `syft`; alternatif `docker buildx build --sbom=true
--provenance=true` — verifikasi dukungan di `docker build` biasa), diunggah sebagai artefak dan
dilampirkan ke release. `.github/dependabot.yml`: `gomod`, `npm`, `github-actions`, `docker`
mingguan, grup minor/patch. Semua action di-pin ke SHA (bukan tag) oleh langkah 10.9. Signing
image dengan cosign keyless (`sigstore/cosign-installer`) **opsional** — dinyalakan bila
organisasi memverifikasi tanda tangan (pertanyaan 13.2 #10).

Image: `engine` tetap distroless nonroot read-only (`read_only: true` + `tmpfs: /tmp` di compose);
nginx berjalan sebagai non-root (`nginxinc/nginx-unprivileged:1.29-alpine`, port 8080 — verifikasi
image dan sesuaikan Caddyfile/`EXPOSE`); `cap_drop: [ALL]`, `security_opt: [no-new-privileges:true]`
untuk semua service aplikasi.

### 5.9 Header keamanan dan TLS

nginx (`infra/docker/nginx/default.conf`, berlaku untuk tiga app):

```nginx
add_header X-Content-Type-Options "nosniff" always;
add_header X-Frame-Options "DENY" always;
add_header Referrer-Policy "strict-origin-when-cross-origin" always;
add_header Permissions-Policy "camera=(), microphone=(), geolocation=()" always;
# CSP: 'unsafe-inline' untuk style karena bpmn-js/diagram-js dan form-js menyuntikkan style (verifikasi;
# bila nonce memungkinkan, perketat). Tanpa 'unsafe-eval'. connect-src memuat issuer OIDC untuk discovery/token.
add_header Content-Security-Policy "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self' ${OIDC_ISSUER_ORIGIN}; frame-src ${OIDC_ISSUER_ORIGIN}; frame-ancestors 'none'; base-uri 'self'; form-action 'self' ${OIDC_ISSUER_ORIGIN}; object-src 'none'; report-uri /csp-report" always;

location = /csp-report { access_log /dev/stdout csp; return 204; }
location = /config.js   { add_header Cache-Control "no-store"; }
client_max_body_size 20m;
```

`${OIDC_ISSUER_ORIGIN}` diisi `envsubst` saat start container (pola `config.js` Fase 6).
`frame-src` dibutuhkan untuk silent renew iframe. HSTS dikirim Caddy (hanya di HTTPS). TLS:
Caddy default (TLS 1.2+, cipher modern); verifikasi `make scan-headers` (curl) dan, manual,
pemindai eksternal (mis. Mozilla Observatory) pada staging — dicatat di 0.

Trafik internal Docker (Caddy → nginx → engine → Postgres) tetap plaintext di jaringan bridge
privat compose (`internal: true` untuk jaringan backend; hanya Caddy di jaringan `edge`). Postgres
memakai `scram-sha-256` (default image 18); `sslmode=disable` di jaringan internal dicatat sebagai
keputusan (TLS Postgres bila DB di host lain: `sslmode=verify-full` + CA, dokumentasi di `tls.md`).

### 5.10 Perubahan proto Fase 8 (ringkasan, semua aditif)

| File | Perubahan |
|---|---|
| `common/v1/options.proto` (baru) | `extend MethodOptions { string permission = 50001; }` |
| semua service | `option (workflow.common.v1.permission)` pada setiap RPC |
| `identity/v1/identity.proto` (baru) | `IdentityService.GetMe(GetMeRequest) → GetMeResponse{subject, name, roles, groups, tenant_ids, permissions, auth_mode}` |
| `job/v1` | `ActivatedJob.trace_context` (string), `ActivateJobsRequest.tenant_ids` (repeated) |
| `deployment/v1` | `DeployResourceRequest.tenant_id`, `ValidateResourceRequest.tenant_id`, `ListProcessDefinitionsRequest.tenant_ids`, `ProcessDefinition.tenant_id` |
| `process/v1` | `CreateProcessInstanceRequest.tenant_id` (+ `WithResult`), `ListProcessInstancesRequest.tenant_ids`, `ProcessInstance.tenant_id`, `Incident.tenant_id` |
| `message/v1` | `PublishMessageRequest.tenant_id`, `ListBufferedMessagesRequest.tenant_ids` |
| `incident/v1`, `usertask/v1`, `decision/v1` (7b) | `tenant_ids` pada pencarian; `tenant_id` pada entitas |
| `common/v1/error.proto` | reason baru: `TOKEN_MISSING`, `TOKEN_INVALID`, `TOKEN_EXPIRED`, `IDP_UNAVAILABLE`, `PERMISSION_DENIED`, `RATE_LIMITED`, `TENANT_DISABLED`, `TENANT_FORBIDDEN`, `TENANT_REQUIRED`, `TENANT_INVALID`, `TENANT_QUOTA_EXCEEDED` (didokumentasikan di tabel 8.4 Fase 4 yang diperluas) |

## 6. Multi-tenant dasar

### 6.1 Model

| Aspek | Aturan |
|---|---|
| Identitas tenant | String `^[A-Za-z0-9_.-]{1,63}$`; `<default>` adalah tenant khusus yang selalu ada (nama mengikuti konvensi Camunda 8 `<default>`, verifikasi) |
| Sumber | `Principal.TenantIDs` dari klaim (5.1); `admin` tanpa klaim tenant = semua tenant |
| Tenant efektif per request | Interceptor `Tenant`: (1) `tenant_id` di request bila ada; (2) jika kosong dan principal punya tepat satu tenant → tenant itu; (3) jika kosong dan multi-tenancy off → `<default>`; (4) selain itu → `InvalidArgument` `TENANT_REQUIRED`. Tenant hasil (1) harus ∈ `Principal.TenantIDs` (atau admin) → selain itu `PermissionDenied` `TENANT_FORBIDDEN`. Format tidak valid → `TENANT_INVALID`; multi-tenancy off dan tenant ≠ `<default>` → `TENANT_DISABLED` |
| Pencarian/aktivasi multi-tenant | `tenant_ids` di request: kosong → semua tenant principal; terisi → irisan dengan tenant principal (admin: apa adanya); irisan kosong → hasil kosong, bukan error |
| Pembacaan by-key | Kueri selalu `WHERE key = $1 AND tenant_id = ANY($2)`; tidak ada baris → `NotFound` (bukan `PermissionDenied`) |
| Key | Tetap global dari `engine_key_seq`; key tidak menyandikan tenant |
| Korelasi dan keunikan | Semua lane lock, indeks unik, dan dedup Fase 1–7 sudah memuat `tenant_id` (`UNIQUE (tenant_id, bpmn_process_id, version)`, `message_dedup_idx`, `message_subscription_match_idx`, dst.); korelasi message hanya dalam tenant yang sama; call activity hanya memanggil proses tenant yang sama |
| Timer start / message start | Membuat instance di tenant definition; subscription start per tenant |
| Outbox, janitor, sweeper, recovery | **Lintas tenant** (infrastruktur), tidak difilter; payload event membawa `tenant_id` agar consumer bisa memfilter bila perlu |
| Idempotensi (Fase 3) | Scope klaim sudah `tenant + nama RPC` → tidak berubah |
| Cache model BPMN (Fase 1) | Key cache = `process_definition_key` (global) → tidak berubah; `GetLatest(bpmnProcessID)` menjadi `GetLatest(tenant, bpmnProcessID)` |

Interceptor:

```go
package tenant

type ctxKey struct{}

// Resolve menentukan tenant efektif untuk request mutasi (satu tenant).
func Resolve(ctx context.Context, requested string) (string, error) {
	p, _ := authn.FromContext(ctx)
	cfg := fromConfig(ctx)
	switch {
	case !cfg.MultiTenancy:
		if requested != "" && requested != DefaultTenant { return "", rpc.Reject(connect.CodeInvalidArgument, "TENANT_DISABLED", "multi-tenancy is disabled", nil) }
		return DefaultTenant, nil
	case requested == "":
		if len(p.TenantIDs) == 1 { return p.TenantIDs[0], nil }
		return "", rpc.Reject(connect.CodeInvalidArgument, "TENANT_REQUIRED", "tenant_id is required", nil)
	case !validID(requested):
		return "", rpc.Reject(connect.CodeInvalidArgument, "TENANT_INVALID", "invalid tenant id", nil)
	case !p.IsAdmin() && !slices.Contains(p.TenantIDs, requested):
		return "", rpc.Reject(connect.CodePermissionDenied, "TENANT_FORBIDDEN", "tenant not allowed", nil)
	}
	return requested, nil
}

// Visible mengembalikan daftar tenant untuk kueri baca/aktivasi.
func Visible(ctx context.Context, requested []string) ([]string, error)

func FromContext(ctx context.Context) (string, bool)
```

Interceptor generik tidak tahu field mana yang berisi tenant; ia memasang `Principal` dan
konfigurasi ke ctx, dan **handler** (`interfaces`) memanggil `tenant.Resolve(ctx, req.Msg.TenantId)`
/ `tenant.Visible(ctx, req.Msg.TenantIds)` lalu meneruskan tenant ke use case sebagai argumen
eksplisit (bukan lewat ctx) — use case Fase 1–7 mendapat parameter `tenant string` baru pada
command/query DTO-nya. Perubahan ini bersifat mekanis dan dilindungi lint 6.3.

### 6.2 Skema dan migrasi (`000NN_tenant.sql`, expand saja)

```sql
-- +goose Up
-- +goose NO TRANSACTION
ALTER TABLE job      ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT '<default>';  -- default konstan: metadata-only di PG ≥ 11
ALTER TABLE incident ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT '<default>';
ALTER TABLE job      ADD COLUMN IF NOT EXISTS trace_context TEXT;              -- 4.1
ALTER TABLE outbox_event ADD COLUMN IF NOT EXISTS trace_context TEXT;
ALTER TABLE timer    ADD COLUMN IF NOT EXISTS trace_context TEXT;
ALTER TABLE message_subscription ADD COLUMN IF NOT EXISTS trace_context TEXT;

-- Aktivasi job per tenant: menggantikan job_activatable_idx (type, key) Fase 2 setelah indeks baru siap.
CREATE INDEX CONCURRENTLY IF NOT EXISTS job_activatable_tenant_idx
    ON job (type, tenant_id, activatable_at, key) WHERE state = 'CREATED';
CREATE INDEX CONCURRENTLY IF NOT EXISTS incident_open_tenant_idx
    ON incident (tenant_id, created_at DESC) WHERE state = 'CREATED';
CREATE INDEX CONCURRENTLY IF NOT EXISTS process_instance_active_tenant_idx
    ON process_instance (tenant_id) WHERE state = 'ACTIVE';            -- kuota dan gauge per tenant

-- +goose Down
DROP INDEX IF EXISTS process_instance_active_tenant_idx;
DROP INDEX IF EXISTS incident_open_tenant_idx;
DROP INDEX IF EXISTS job_activatable_tenant_idx;
ALTER TABLE message_subscription DROP COLUMN IF EXISTS trace_context;
ALTER TABLE timer DROP COLUMN IF EXISTS trace_context;
ALTER TABLE outbox_event DROP COLUMN IF EXISTS trace_context;
ALTER TABLE job DROP COLUMN IF EXISTS trace_context;
ALTER TABLE incident DROP COLUMN IF EXISTS tenant_id;
ALTER TABLE job DROP COLUMN IF EXISTS tenant_id;
```

Catatan:

- Baris lama otomatis `<default>` — benar karena sebelum Fase 8 hanya ada satu tenant. Untuk
  `job`/`incident` yang dibuat **setelah** migrasi oleh binary lama (selama rollout), nilai
  `<default>` juga benar karena binary lama hanya mengenal `<default>`.
- Projector Fase 2 menulis `tenant_id` ke `job` dan `incident` dari `process_instance.tenant_id`
  (ada di `InstanceState.Definition`); `VerifyTenantMismatch` (Fase 3 `verify`) memeriksa
  `job.tenant_id = process_instance.tenant_id`.
- `job_activatable_idx` lama di-drop di migrasi **contract** rilis berikutnya (1.1), sesuai
  aturan expand/contract.
- `element_instance`, `variable`, `execution_record`, `taken_sequence_flow`, `execution_snapshot`
  **tidak** diberi `tenant_id`: selalu diakses lewat `process_instance_key` yang sudah divalidasi
  tenantnya (lint 6.3 memeriksa bahwa kueri pada tabel ini memuat `process_instance_key`).
- Snapshot Fase 3/5: `InstanceState` sudah memuat tenant lewat `DefinitionRef` → `format_version`
  tidak naik (verifikasi saat 10.6; bila `DefinitionRef` belum memuat tenant, versi naik ke 3).

### 6.3 Scoping kueri dan penegakan

- Setiap kueri sqlc pada tabel ber-`tenant_id` menerima parameter `tenant_id`/`tenant_ids`;
  nama parameter dibakukan agar lint statis sederhana: `sqlc.arg(tenant_id)` atau
  `tenant_id = ANY(sqlc.arg(tenant_ids)::text[])`.
- `TestTenantScopedQueries` (`apps/engine/internal/archtest`): memparse semua `queries/*.sql`,
  untuk setiap statement yang menyentuh tabel di daftar `tenantTables` memastikan ada predikat
  `tenant_id`, atau untuk tabel anak ada predikat `process_instance_key`/`key` join; pengecualian
  eksplisit dengan komentar `-- tenant: cross (alasan)` untuk janitor, sweeper, verify, recovery,
  gauge metrik.
- Test integrasi dua tenant (`tenant_isolation_test.go`): deploy `order.bpmn` ke `acme` dan `globex`
  (versi 1 di keduanya), start instance di masing-masing, worker `acme` hanya mendapat job `acme`,
  `GetProcessInstance` lintas tenant → `NotFound`, `PublishMessage` di `acme` tidak mengkorelasikan
  subscription `globex`, timer start versi terbaru per tenant, `ListProcessInstances{tenant_ids: []}`
  oleh principal dua tenant mengembalikan keduanya dengan `tenant_id` terisi, janitor retensi
  menghapus keduanya.
- Kuota: `ENGINE_TENANT_MAX_ACTIVE_INSTANCES` dicek di `CreateInstance` (termasuk dari timer start,
  message start, dan call activity — call activity yang gagal kuota → incident `TENANT_QUOTA_EXCEEDED`
  di call activity induk, bukan error senyap); hitungan dari indeks parsial per tenant; race kecil di
  sekitar batas diterima (bukan hard limit transaksional).

### 6.4 Proto dan SDK

Field `tenant_id`/`tenant_ids` di 5.10. SDK Go: `sdk.WithDefaultTenant("acme")` pada client
mengisi `tenant_id` yang kosong; `JobWorker` menerima `TenantIDs`. Client TS: `TenantContext` di
`app-shell` (5.5). `ActivatedJob.tenant_id` (Fase 4) kini terisi nilai nyata. `GetMe` memberi
daftar tenant untuk pemilih UI.

Paritas: Camunda 8 memakai `tenantId` pada request dan `<default>` sebagai tenant bawaan
(verifikasi; bila nama berbeda, kita tetap memakai `<default>` dan mencatat deviasi karena nilai
tidak memengaruhi perilaku BPMN).

### 6.5 Onboarding tenant (`docs/ops/tenant-onboarding.md`)

1. Aktifkan `ENGINE_MULTI_TENANCY=true` (rollout).
2. Di IdP: tambahkan nilai `tenant_ids` ke pengguna/grup dan ke service account worker.
3. Deploy proses untuk tenant baru (`tenant_id` di `DeployResource`, dari Modeler lewat pemilih
   tenant).
4. Verifikasi isolasi dengan `engine verify --tenant acme` (laporan jumlah entitas per tenant).

Tidak ada langkah "buat tenant" karena tidak ada registry (3.5).

### 6.6 Di luar lingkup

Database/skema per tenant, enkripsi per tenant, kuota berbeda per tenant (tabel), registry
tenant dan API admin tenant, ACL per resource di dalam tenant, pemisahan metrik/trace per tenant ke
backend berbeda, penagihan. Dicatat di 15.

## 7. Hardening deploy

### 7.1 Compose produksi dan Caddy

Sketsa `infra/docker/docker-compose.prod.yml` (inti; nilai dari `.env.prod`):

```yaml
name: workflow-engine

x-logging: &logging
  driver: json-file
  options: { max-size: '50m', max-file: '5' }

x-hardening: &hardening
  security_opt: ['no-new-privileges:true']
  cap_drop: [ALL]
  restart: unless-stopped
  logging: *logging

x-engine: &engine
  <<: *hardening
  read_only: true
  tmpfs: [/tmp]
  environment: &engine-env
    ENGINE_ENV: production
    ENGINE_HTTP_ADDR: ':8080'
    ENGINE_ADMIN_ADDR: ':9090'
    ENGINE_MIGRATE: 'true'
    ENGINE_AUTH_MODE: oidc
    ENGINE_OIDC_ISSUER: ${OIDC_ISSUER:?}
    ENGINE_OIDC_DISCOVERY_URL: ${OIDC_DISCOVERY_URL:-}
    ENGINE_OIDC_AUDIENCES: ${OIDC_AUDIENCES:-workflow-engine}
    ENGINE_MULTI_TENANCY: ${ENGINE_MULTI_TENANCY:-false}
    ENGINE_DB_PASSWORD_FILE: /run/secrets/postgres_password
    DATABASE_URL: postgres://${POSTGRES_USER:-workflow}@postgres:5432/${POSTGRES_DB:-workflow}?sslmode=disable
    ENGINE_LOG_LEVEL: ${ENGINE_LOG_LEVEL:-info}
    ENGINE_SHUTDOWN_TIMEOUT: 35s
    ENGINE_DRAIN_DELAY: 5s
    OTEL_EXPORTER_OTLP_ENDPOINT: ${OTEL_EXPORTER_OTLP_ENDPOINT:-}
    OTEL_TRACES_SAMPLER: parentbased_traceidratio
    OTEL_TRACES_SAMPLER_ARG: ${OTEL_SAMPLE_RATIO:-0.1}
    GOMEMLIMIT: ${ENGINE_GOMEMLIMIT:-800MiB}        # ~80 % dari batas memori
  secrets: [postgres_password]
  networks: [backend]
  stop_grace_period: 40s
  deploy:
    resources:
      limits: { cpus: '${ENGINE_CPUS:-2}', memory: ${ENGINE_MEMORY:-1g} }
  depends_on:
    postgres: { condition: service_healthy }

services:
  caddy:
    image: caddy:2-alpine                      # kunci versi saat 10.11
    <<: *hardening
    cap_add: [NET_BIND_SERVICE]
    ports: ['80:80', '443:443', '443:443/udp']
    environment: { DOMAIN: ${DOMAIN:?}, ACME_EMAIL: ${ACME_EMAIL:-} }
    volumes:
      - ./caddy/Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy-data:/data
      - caddy-config:/config
    networks: [edge, backend]

  postgres:
    image: postgres:18-alpine
    <<: *hardening
    command: >
      postgres -c wal_level=replica -c max_connections=${POSTGRES_MAX_CONNECTIONS:-100}
               -c shared_buffers=${POSTGRES_SHARED_BUFFERS:-512MB}
               -c archive_mode=${POSTGRES_PITR:-off} -c archive_command='/archive.sh %p %f'
    environment:
      POSTGRES_USER: ${POSTGRES_USER:-workflow}
      POSTGRES_DB: ${POSTGRES_DB:-workflow}
      POSTGRES_PASSWORD_FILE: /run/secrets/postgres_password
    secrets: [postgres_password]
    volumes:
      - postgres-data:/var/lib/postgresql
      - postgres-wal-archive:/wal-archive
      - ./postgres/archive.sh:/archive.sh:ro
    networks: [backend]
    deploy:
      resources:
        limits: { cpus: '${POSTGRES_CPUS:-2}', memory: ${POSTGRES_MEMORY:-2g} }
    healthcheck:
      test: ['CMD-SHELL', 'pg_isready -U $${POSTGRES_USER} -d $${POSTGRES_DB}']
      interval: 5s
      timeout: 3s
      retries: 20

  engine-a:
    <<: *engine
    image: ${ENGINE_IMAGE_A:-${REGISTRY}/engine:${VERSION}}
    environment: { <<: *engine-env, ENGINE_INSTANCE_ID: engine-a }
  engine-b:
    <<: *engine
    image: ${ENGINE_IMAGE_B:-${REGISTRY}/engine:${VERSION}}
    environment: { <<: *engine-env, ENGINE_INSTANCE_ID: engine-b }

  modeler:
    image: ${REGISTRY}/web-modeler:${VERSION}
    <<: *hardening
    environment: &web-env
      OIDC_ISSUER: ${OIDC_ISSUER:?}
      OIDC_CLIENT_ID: modeler-web
      AUTH_MODE: oidc
      MODELER_URL: https://modeler.${DOMAIN}
      OPERATE_URL: https://operate.${DOMAIN}
      TASKLIST_URL: https://tasklist.${DOMAIN}
      ENGINE_UPSTREAMS: 'engine-a:8080 engine-b:8080'
    networks: [backend]
    depends_on: { engine-a: { condition: service_healthy } }
  operate:
    image: ${REGISTRY}/web-operate:${VERSION}
    <<: *hardening
    environment: { <<: *web-env, OIDC_CLIENT_ID: operate-web }
    networks: [backend]
  tasklist:
    image: ${REGISTRY}/web-tasklist:${VERSION}
    <<: *hardening
    environment: { <<: *web-env, OIDC_CLIENT_ID: tasklist-web }
    networks: [backend]

  db-backup:
    build: ./backup                            # atau image ${REGISTRY}/db-backup:${VERSION} yang dibangun CI
    image: ${REGISTRY}/db-backup:${VERSION}
    <<: *hardening
    environment:
      PGHOST: postgres
      PGUSER: ${POSTGRES_USER:-workflow}
      PGDATABASE: ${POSTGRES_DB:-workflow}
      PGPASSWORD_FILE: /run/secrets/postgres_password
      BACKUP_CRON: ${BACKUP_CRON:-0 2 * * *}
      BACKUP_KEEP_DAYS: ${BACKUP_KEEP_DAYS:-14}
      BACKUP_KEEP_WEEKLY: ${BACKUP_KEEP_WEEKLY:-8}
      BASEBACKUP_CRON: ${BASEBACKUP_CRON:-0 3 * * 0}   # hanya bila POSTGRES_PITR=on
      POSTGRES_PITR: ${POSTGRES_PITR:-off}
    secrets: [postgres_password]
    volumes:
      - postgres-backups:/backups
      - postgres-wal-archive:/wal-archive
    networks: [backend]

  keycloak:                                    # opsional: profil auth
    image: quay.io/keycloak/keycloak:26        # verifikasi versi dan opsi start
    profiles: [auth]
    <<: *hardening
    command: start --import-realm --optimized
    environment:
      KC_DB: postgres
      KC_DB_URL: jdbc:postgresql://postgres:5432/keycloak
      KC_DB_USERNAME: ${POSTGRES_USER:-workflow}
      KC_DB_PASSWORD_FILE: /run/secrets/postgres_password     # verifikasi dukungan _FILE di Keycloak; cadangan: env dari skrip entrypoint
      KC_HOSTNAME: https://auth.${DOMAIN}
      KC_PROXY_HEADERS: xforwarded
      KC_HTTP_ENABLED: 'true'
      KC_BOOTSTRAP_ADMIN_USERNAME: admin
      KC_BOOTSTRAP_ADMIN_PASSWORD_FILE: /run/secrets/keycloak_admin_password
    volumes: ['../keycloak:/opt/keycloak/data/import:ro']
    secrets: [postgres_password, keycloak_admin_password]
    networks: [backend]

networks:
  edge: {}
  backend: { internal: true }

volumes:
  postgres-data: {}
  postgres-backups: {}
  postgres-wal-archive: {}
  caddy-data: {}
  caddy-config: {}

secrets:
  postgres_password: { file: ${SECRETS_DIR:-./secrets}/postgres_password }
  keycloak_admin_password: { file: ${SECRETS_DIR:-./secrets}/keycloak_admin_password }
```

Catatan: Caddy butuh jaringan `edge` (akses internet untuk ACME) dan `backend`; `backend`
`internal: true` memutus akses keluar engine — engine hanya butuh IdP (lewat Caddy di `backend`,
atau `ENGINE_OIDC_DISCOVERY_URL` ke `keycloak:8080`) dan endpoint OTLP (collector di `backend`, atau
eksternal → jaringan `edge` ditambahkan ke engine dan didokumentasikan). Database `keycloak` dibuat
oleh skrip init Postgres (`infra/docker/postgres/init-keycloak.sql`, hanya saat volume baru).
`web.Dockerfile` nginx menjadi `nginx-unprivileged` (5.8) sehingga `cap_drop ALL` aman; `ENGINE_UPSTREAMS`
di-render `envsubst` ke blok `upstream engine { server engine-a:8080 max_fails=1 fail_timeout=5s; … }`.

`.env.prod.example` memuat semua variabel di atas dengan komentar; `make deploy-local` memakai
`DOMAIN=localhost`, `ACME_EMAIL` kosong, `SECRETS_DIR=./secrets` hasil `make secrets-init`,
`ENGINE_AUTH_MODE=oidc` dengan Keycloak profil `auth` (atau `make deploy-local AUTH=none` untuk
smoke test tanpa IdP, yang menyetel `ENGINE_ENV=development`).

### 7.2 Rollout tanpa downtime dan graceful drain

Urutan `make rollout VERSION=v1.1.0` (`tools/scripts/rollout`, Go memakai Docker CLI):

```
0. Prasyarat: `make db-backup` (atau konfirmasi backup < 24 jam); `docker compose pull` image VERSION.
1. ENGINE_IMAGE_B=<baru> docker compose up -d --no-deps engine-b
   → engine-b start: migrasi (session lock; expand saja) → recovery → ready.
2. Tunggu `docker inspect` health engine-b = healthy (/readyz 200) ≤ ROLLOUT_TIMEOUT (default 5 m);
   gagal → `docker compose logs engine-b`, kembalikan ENGINE_IMAGE_B lama, keluar dengan error (engine-a tidak disentuh).
3. Caddy memasukkan engine-b ke rotasi (health check 2 s). Tunggu 2 × health_interval.
4. ENGINE_IMAGE_A=<baru> docker compose up -d --no-deps engine-a
   → compose mengirim SIGTERM ke engine-a lama → drain (di bawah) → container baru start → ready.
5. Verifikasi: kedua slot healthy, `engine_build_info{version}` sama di keduanya (lewat /metrics admin
   atau `docker exec engine-a /engine version`), `curl https://api.<domain>/readyz`.
6. Tulis VERSION, ENGINE_IMAGE_A, ENGINE_IMAGE_B ke .env.prod (agar `compose up` berikutnya konsisten).
7. Web: `docker compose up -d --no-deps modeler operate tasklist` (nginx start < 1 s; Caddy health
   check pada nginx (`/healthz` nginx) menahan trafik selama restart — verifikasi apakah Caddy
   reverse_proxy ke satu upstream dengan health check cukup; alternatif: dua replika web juga, atau
   terima jeda < 1 s untuk aset statis).
```

Drain di engine (`httpserver/drain.go`, dipanggil dari `main.go` saat `ctx` sinyal dibatalkan):

```go
func (d *Drainer) Run(ctx context.Context) error {
	<-ctx.Done()                                   // SIGTERM
	d.log.Info("drain started")
	d.readiness.Set(false)                         // /readyz → 503 (Caddy dan nginx berhenti mengirim request baru)
	time.Sleep(d.cfg.DrainDelay)                   // > health_interval Caddy + propagasi
	d.longPoll.Close()                             // semua ActivateJobs menunggu dikembalikan kosong (worker poll ulang ke slot lain)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), d.cfg.ShutdownTimeout)
	defer cancel()
	err := d.http.Shutdown(shutdownCtx)            // tunggu request berjalan (command pendek) selesai
	d.background.Stop(shutdownCtx)                 // relay outbox, sweeper, janitor, collector metrik: selesaikan batch berjalan, lepas advisory lock
	d.otelShutdown(shutdownCtx)                    // flush span/metrik
	d.pool.Close()
	d.log.Info("drain finished")
	return err
}
```

Jaminan dan batas (ADR-0045):

| Hal | Jaminan |
|---|---|
| RPC unary yang sedang berjalan | Selesai normal (command < `statement_timeout` 10 s) |
| `ActivateJobs` yang menunggu | Kembali kosong segera; SDK melakukan poll berikutnya ke slot lain lewat Caddy |
| Job yang sedang di-lease oleh worker | Tidak terpengaruh (lease di DB, bukan di replika) |
| Consumer outbox / sweeper | Batch berjalan selesai atau rollback; advisory lock dilepas saat koneksi ditutup; slot lain mengambil alih ≤ 5 s (interval coba ulang Fase 3) |
| Timer jatuh tempo selama drain | Diambil slot lain; lag ≤ sweep interval + 5 s |
| Dua versi binary bersamaan | Aman selama migrasi mematuhi expand/contract Fase 3; `release.md` mewajibkan setiap rilis menyatakan "fase contract" yang dijalankan |
| Request yang tiba tepat saat listener ditutup | Caddy `lb_try_duration 5s` mencoba upstream lain (verifikasi semantik retry Caddy untuk POST — hanya aman bila request belum terkirim; Connect SDK juga retry `Unavailable` dengan `request_id` idempoten) |
| Kegagalan host / Postgres restart | **Tidak** zero-downtime; klien retry; didokumentasikan |

Skenario chaos `rolling-upgrade` (8.4) menjalankan urutan ini di bawah beban dan memverifikasi
invarian Fase 3 ditambah "tidak ada RPC gagal selain `Unavailable` yang di-retry".

### 7.3 Backup, retensi, dan PITR

`infra/docker/backup/backup.sh` (dijalankan `crond` di container `db-backup` berbasis
`postgres:18-alpine`, sehingga versi `pg_dump` = versi server):

```sh
#!/bin/sh
set -eu
export PGPASSWORD="$(cat "$PGPASSWORD_FILE")"
ts=$(date -u +%Y%m%dT%H%M%SZ)
out=/backups/daily/workflow-$ts.dump
mkdir -p /backups/daily /backups/weekly
pg_dump -Fc -Z 6 -f "$out.tmp" && mv "$out.tmp" "$out"
# salinan mingguan (Minggu) untuk retensi panjang
[ "$(date -u +%u)" = 7 ] && cp "$out" /backups/weekly/
find /backups/daily  -name '*.dump' -mtime +"$BACKUP_KEEP_DAYS" -delete
find /backups/weekly -name '*.dump' -mtime +$((BACKUP_KEEP_WEEKLY * 7)) -delete
# status untuk alert (dibaca collector hostmetrics/textfile atau skrip cek)
printf 'backup_last_success_timestamp %s\nbackup_last_size_bytes %s\n' "$(date +%s)" "$(stat -c %s "$out")" > /backups/status.prom
```

Status backup diekspos ke Prometheus lewat receiver `filelog`/`textfile` collector (verifikasi
cara termudah; cadangan: engine membaca `/backups/status.prom` lewat volume read-only dan
mengekspor `engine_backup_last_success_timestamp` — dipilih bila collector tidak ada).

Latihan restore (`restore-drill.sh`, `make db-restore-drill`, CI mingguan `backup-drill.yml` pada
runner dengan Compose): restore dump terbaru ke database `workflow_drill` → `engine verify --all`
→ `DROP DATABASE workflow_drill` → catat durasi; gagal → alert/issue.

PITR (`POSTGRES_PITR=on`):

- `archive.sh`: `test ! -f /wal-archive/$2 && cp $1 /wal-archive/$2` (sederhana, idempoten);
  `basebackup.sh` mingguan: `pg_basebackup -D /backups/base/<ts> -Ft -z -X none`
  (verifikasi opsi; WAL diambil dari arsip) + pemangkasan arsip WAL yang lebih tua dari base backup
  tertua yang disimpan.
- `docs/ops/restore.md` prosedur PITR: hentikan engine → tarik base backup → `recovery.signal` +
  `restore_command='cp /wal-archive/%f %p'` + `recovery_target_time='...'` + `recovery_target_action=promote`
  → start Postgres → `engine verify --all` → start engine. Diuji sekali end-to-end (DoD) dengan
  skenario "tabel dihapus pukul T, pulihkan ke T−1 menit".
- RPO: ≤ `archive_timeout` (diset 60 s bila PITR aktif) ; RTO: waktu salin base backup + replay WAL
  (diukur di latihan).

### 7.4 Batas sumber daya, log, dan runtime Go

- `deploy.resources.limits` berlaku di Compose v2 non-Swarm (verifikasi versi Compose yang
  mendukung; cadangan `mem_limit`/`cpus`). Default: engine 2 CPU / 1 GiB per slot, Postgres
  2 CPU / 2 GiB, web 0,5 CPU / 128 MiB, Caddy 1 CPU / 256 MiB, backup 1 CPU / 512 MiB.
- `GOMEMLIMIT` diisi dari `.env.prod` (≈ 80 % batas); Go 1.25+ menyetel `GOMAXPROCS` sesuai kuota
  cgroup secara otomatis (verifikasi untuk Go 1.27; bila tidak, `automaxprocs`).
- Rotasi log: `json-file` `max-size 50m` × `max-file 5` per container (≈ 250 MB maksimum); Caddy
  dan nginx log JSON ke stdout; akses log nginx untuk aset statis dimatikan (`access_log off` pada
  `/assets/`).
- Postgres: `shared_buffers`, `max_connections` dari env; aturan kapasitas Fase 3
  (`2 × ENGINE_DB_MAX_CONNS + 10 + keycloak ≤ max_connections`) dicatat di `capacity.md`;
  `log_min_duration_statement=500ms` untuk diagnosis.

### 7.5 Endpoint kesehatan dan listener admin

| Endpoint | Listener | Auth | Fungsi |
|---|---|---|---|
| `GET /healthz` | utama + admin | tidak | liveness (Fase 0) |
| `GET /readyz` | utama + admin | tidak | readiness (Fase 3) + drain (7.2); body JSON `{ready, phase, version, instance_id}` |
| `GET /metrics` | admin | tidak (jaringan internal) | Prometheus, bila `ENGINE_METRICS_PROMETHEUS=true` |
| `GET /debug/pprof/*` | admin | tidak | bila `ENGINE_PPROF_ENABLED=true` |
| `GET /version` | utama | tidak | `{version, commit, go_version}` (dipakai rollout) |

Listener admin tidak pernah dipublikasikan ke `edge`; Caddy menolak `/debug/*` dan `/metrics`
pada `api.` sebagai lapisan kedua. Subperintah CLI `engine version`.

### 7.6 Rilis berversi

- `make release VERSION=v1.0.0`: memeriksa branch `main` bersih dan `make check` hijau, menghasilkan
  `CHANGELOG.md` (git-cliff dari Conventional Commits; konfigurasi `cliff.toml`), commit
  `chore(release): v1.0.0`, tag beranotasi, push.
- `.github/workflows/release.yml` (pada tag `v*`): build keempat image + `db-backup` dengan tag
  `vX.Y.Z` dan `vX.Y`, scan Trivy, SBOM, push GHCR; bundel deploy
  `workflow-engine-deploy-vX.Y.Z.tar.gz` (`docker-compose.prod.yml`, `docker-compose.observability.yml`,
  `caddy/Caddyfile`, `nginx/`, `backup/`, `postgres/`, `keycloak/realm-workflow.json`,
  `.env.prod.example`, `docs/ops/*.md`); GitHub Release dengan catatan dari changelog.
- Versi engine tercetak di log start, `/version`, `engine_build_info`, dan header UI (`GetMe`
  memuat `engine_version`).
- Kebijakan: 1.0 membekukan proto `v1` (ADR-0020 "beta sampai Fase 6" berakhir di 1.0); perubahan
  tak kompatibel = `v2` berdampingan.

### 7.7 Prosedur upgrade dan rollback (`docs/ops/upgrade.md`, `rollback.md`)

Upgrade: baca catatan rilis (termasuk "migrasi contract" dan "perubahan konfigurasi") →
`make db-backup` → `VERSION=vX.Y.Z docker compose pull` → `make rollout VERSION=vX.Y.Z` → cek dashboard
15 menit. Upgrade major Postgres: prosedur terpisah dengan `pg_dumpall`/`pg_upgrade` dalam jendela
pemeliharaan.

Rollback: `make rollback` membaca `ENGINE_IMAGE_*` sebelumnya dari `.env.prod.bak` dan menjalankan
rollout terbalik; **diizinkan** bila rilis yang dibatalkan tidak menjalankan migrasi contract
(dicatat di catatan rilis); bila sudah, rollback = restore backup (data setelah backup hilang) —
keputusan operator, didokumentasikan dengan jelas. Migrasi `down` tidak dijalankan otomatis.

Diuji di CI terjadwal (`upgrade-drill.yml`): deploy `v0.N` (rilis rc sebelumnya) dengan data
contoh → rollout ke build saat ini → verify → rollback → verify.

### 7.8 Kubernetes / Helm (stretch)

Bila 10.19 masih punya waktu: `infra/helm/workflow-engine` dengan `values.yaml` minimal (image,
replika engine = 2, `readinessProbe /readyz`, `livenessProbe /healthz`, `preStop sleep 5`,
`terminationGracePeriodSeconds 40`, tiga Deployment web, Ingress (host-based, cert-manager
opsional), `Secret` untuk password DB, `ConfigMap` Caddy tidak dibutuhkan (Ingress menggantikan),
Postgres eksternal (`DATABASE_URL`). Diuji dengan `kind` di laptop saja; tidak masuk CI dan DoD;
dicatat di README sebagai "eksperimental".

## 8. Operasional

### 8.1 Runbook (`docs/ops/`)

Setiap runbook memakai format yang sama: gejala/pemicu → prasyarat → langkah bernomor dengan
perintah persis → verifikasi → rollback langkah → kapan eskalasi. Prosedur yang dapat dijalankan
dijalankan sekali saat fase ini dan hasilnya (tanggal, durasi, catatan) ditulis di bagian 0.

| Runbook | Isi |
|---|---|
| `incident-response.md` | Peta alert → tindakan (tabel 4.6): engine down (cek `docker compose ps`, log, `/readyz` body), error ratio (dashboard overview → trace error terbaru di Tempo), outbox lag (`engine outbox status`, dead letter → `requeue`), timer lag, pool saturated (`ENGINE_DB_MAX_CONNS`, kueri lambat `pg_stat_activity`), disk, auth spike; template catatan insiden; pengumpulan bukti (`make support-bundle`: versi, konfigurasi tersamar, `docker compose ps/logs --since 1h`, `engine verify --json`, `outbox status`) |
| `restore.md` | Perluasan Fase 3: restore `pg_dump` ke DB baru + verify + promosi; PITR (7.3); urutan menghentikan engine (`make stop-engine` = scale kedua slot ke 0) dan menyalakan kembali; apa yang hilang (event setelah titik restore) |
| `upgrade.md`, `rollback.md` | 7.7 |
| `rotate-secrets.md` | 5.7 |
| `oidc-setup.md` | Keycloak (realm contoh, client, mapper klaim `tenant_ids`/`groups`, service account), Zitadel/Entra ID (pemetaan klaim yang berbeda: `roles`, `groups` sebagai object id → butuh `ENGINE_OIDC_*_CLAIM`), pemeriksaan token dengan `engine token-inspect <jwt>` (subperintah CLI baru yang mencetak klaim dan `Principal` hasil pemetaan tanpa rahasia) |
| `tls.md` | Caddy ACME (DNS, port 80/443, email), CA internal lokal (`make tls-local`), sertifikat milik sendiri (`tls cert.pem key.pem` di Caddyfile), TLS Postgres eksternal |
| `tenant-onboarding.md` | 6.5 |
| `capacity.md` | 8.2 |
| `observability.md` | Cara menghubungkan ke collector/LGTM/platform eksternal, daftar metrik dan arti, TraceQL/LogQL contoh (`{ .process_instance_key = 123 }`, `{container="engine-a"} |= "trace_id=…"`), impor dashboard manual |

### 8.2 Panduan kapasitas (`docs/ops/capacity.md`)

Diisi dengan angka dari load test 8.3 pada **mesin referensi** (dicatat: CPU, RAM, disk, versi
Docker; mesin dev tim dan satu VM cloud kelas 4 vCPU/8 GB bila tersedia). Struktur:

| Dimensi | Cara menghitung | Contoh (diisi setelah pengukuran) |
|---|---|---|
| Throughput command | instance/detik dan job/detik per CPU engine pada p95 < SLO; batas biasanya Postgres (fsync, row lock per instance) | `__` instance/s, `__` job/s |
| Koneksi DB | `2 slot × ENGINE_DB_MAX_CONNS + db-backup 1 + keycloak ≤ max_connections − 10` | `2 × 16 + …` |
| Pertumbuhan disk | byte/record (benchmark Fase 2) × record per instance × instance/hari; `element_instance`, `variable`; retensi `ENGINE_HISTORY_RETENTION_DAYS`; WAL arsip/hari bila PITR | `__` MB/hari pada `__` instance/hari |
| Memori engine | RSS idle + per long-poll aktif + cache model/FEEL | `__` MB |
| Worker | `max_jobs_to_activate × worker` vs `engine_jobs_queue_depth`; saran `request_timeout` dan `timeout` | — |
| Timer | timer jatuh tempo per detik yang terproses (benchmark Fase 5) | `__`/s |
| Ukuran backup | `pg_dump -Fc` ≈ `__` % ukuran DB; durasi | — |
| Kapan scale-up | CPU engine > 70 % 15 m, pool wait p95 > 50 ms, lock wait p95 > 50 ms (ADR-0017 pemicu partisi), disk > 70 % | — |

### 8.3 Load test terhadap SLO

`tools/scripts/loadtest` (Go, memakai `libs/go/sdk`, proyek Nx `loadtest` tag `type:tool`):

| Skenario | Beban | Durasi | Lulus bila |
|---|---|---|---|
| `baseline` | 20 instance/s proses linear 5 service task; 8 worker × 32 job; 2 slot engine | 10 m | SLO 4.6 terpenuhi; error 0 selain yang disengaja |
| `fanout` | 5 instance/s fork 50 cabang paralel | 5 m | lock wait p95 < 50 ms; selesai semua |
| `timers` | 10.000 timer jatuh tempo dalam jendela 60 s + beban baseline 50 % | 5 m | timer lag p95 < 2 s |
| `messages` | 50 publish/s dengan 50 % subscription sudah terbuka, 50 % buffered | 5 m | korelasi p95 < 500 ms |
| `usertasks` | 2.000 task terbuka, 50 `SearchUserTasks`/s, 10 complete/s | 5 m | `engine_usertask_search_duration_seconds` p95 < 100 ms |
| `tenants` | `baseline` dibagi 10 tenant | 5 m | isolasi (hitungan per tenant), SLO sama |
| `soak` | 25 % baseline | 8 jam (nightly/mingguan) | tanpa kebocoran memori (RSS stabil ±10 %), outbox lag stabil, retensi berjalan |
| `rollout` | `baseline` 50 % sambil `make rollout` | 5 m | tanpa error selain `Unavailable` yang di-retry; semua instance selesai |

Laporan: JSON + Markdown (p50/p95/p99 per RPC, throughput, metrik engine yang diambil dari
`/metrics` admin atau Prometheus, hasil SLO ✓/✗) ke `tmp/loadtest/<ts>/` dan artefak CI;
`make loadtest SCENARIO=baseline DURATION=10m`. CI `loadtest.yml` mingguan pada runner biasa
(hanya `baseline` dan `tenants`, 3 menit, angka dicatat bukan di-gate; gate hanya "tidak ada
error" karena runner GitHub tidak stabil secara performa).

k6 (opsional): skrip `tools/k6/api.js` untuk uji HTTP murni (JSON Connect) bagi tim yang sudah
memakai k6; bukan bagian DoD.

### 8.4 Chaos: perluasan suite Fase 3

Skenario baru di `tools/scripts/chaos` (nomor melanjutkan tabel 11.3 Fase 3 dan Fase 4–5):

| # | Skenario | Injeksi | Hasil yang diharapkan |
|---|---|---|---|
| 17 | `rolling-upgrade` | `make rollout` ke image yang sama dengan tag berbeda saat load (worker-example + load generator) | Tidak ada RPC gagal selain `Unavailable` yang di-retry SDK; tidak ada job hilang/ganda; `verify --all` bersih; durasi < 2 m |
| 18 | `kill-one-replica-under-load` | `docker kill -s KILL engine-a` saat load | Caddy mengeluarkan slot ≤ 2 × health_interval; consumer/sweeper diambil alih engine-b ≤ 5 s; worker tidak melihat error setelah retry; engine-a restart (`unless-stopped`) dan kembali ke rotasi |
| 19 | `partition-engine-idp` | `docker network disconnect` keycloak selama 60 s | Token dengan `kid` di cache tetap diterima; `kid` baru → `Unavailable IDP_UNAVAILABLE`; login UI gagal dengan pesan "identity provider unreachable"; setelah tersambung semua pulih tanpa restart |
| 20 | `partition-caddy-engine` | disconnect `engine-a` dari jaringan `backend` 30 s | Caddy mengalihkan ke engine-b; engine-a kehilangan DB → `/readyz` 503 (Fase 3 pool reconnect); setelah tersambung, kembali ready; tidak ada event ganda |
| 21 | `idp-down-at-start` | engine start saat Keycloak mati | Engine **tetap start** (discovery ditunda, retry backoff; `/readyz` ready karena DB ok) tetapi semua RPC berauth → `Unavailable IDP_UNAVAILABLE` sampai discovery berhasil; metrik `engine_auth_requests_total{outcome="idp_unavailable"}` |
| 22 | `rate-limit-flood` | 2.000 req/s dari satu principal | Dibatasi ke rps+burst; principal lain tidak terpengaruh (latensi p95 tetap); tidak ada peningkatan pool wait |
| 23 | `tenant-noise` | tenant A menjalankan `fanout`, tenant B `baseline` | Tenant B tetap memenuhi SLO (satu DB; tidak ada isolasi performa yang dijanjikan — skenario mendokumentasikan dampak nyata, gate hanya "tidak ada error") |
| 24 | `backup-under-load` + `restore-drill` | `db-backup` dipicu manual saat load, lalu drill | Backup sukses; `verify --all` pada DB drill bersih |
| 25 | `pitr-restore` | hapus tabel sengaja pada T; restore ke T−1 m | Data sebelum T utuh; engine start dan verify bersih |
| 26 | `disk-full-wal` (manual, bukan CI) | isi volume `postgres-wal-archive` | `archive_command` gagal → alert `WalArchiveFailing`; Postgres tetap melayani sampai `pg_wal` penuh (didokumentasikan; pemangkasan arsip diperbaiki) |

Skenario 17–22 masuk `make chaos` dan CI nightly; 23–26 dijalankan manual sebelum fase ditutup.

## 9. Kesiapan rilis 1.0

### 9.1 Checklist lintas fase

Diisi di `docs/plan/release-1.0-checklist.md` dengan tautan bukti (PR, run CI, laporan):

| Fase | Item | Bukti |
|---|---|---|
| 0 | `make setup` → `make check` hijau dari clone bersih (Windows dan Linux); CI hijau di `main` | run CI |
| 1 | Korpus BPMN + paritas deploy (59 file) terhadap `camunda/zeebe:8.8.44` hijau pada rc | `make parity SUITE=deploy LIVE=1` |
| 2 | Skenario eksekusi, property test gateway 100.000 iterasi, paritas `execution` | `make prop PROP_ITER=100000`, `make parity SUITE=execution LIVE=1` |
| 3 | Chaos 1–16 hijau; backup/restore drill; `verify`/`rebuild` di DB produksi-mirip | `make chaos`, laporan |
| 4 | Paritas `worker`; benchmark aktivasi; SDK quickstart diikuti dari nol | laporan |
| 5 | Paritas `events`, `scopes`; chaos timer; property test interupsi | laporan |
| 6 | e2e Playwright tiga app (dengan OIDC); axe tanpa pelanggaran serius; paritas `usertask` | run CI `e2e` |
| 7 | Golden FEEL ≥ 600, konformansi, paritas FEEL ≥ 150 kasus; (7b) paritas decision bila dikerjakan | laporan |
| 8 | Semua DoD bagian 2; load test SLO; chaos 17–22; scan keamanan bersih; restore + PITR drill | laporan |
| Semua | `ProfileExecutable` = daftar elemen yang didukung di README; daftar deviasi dari Camunda 8.8 terkumpul di `docs/compatibility.md` (gabungan `expectations.json` semua suite) | dokumen |
| Semua | Tidak ada `TODO`/`verifikasi` tersisa di ADR yang berstatus Diterima; pertanyaan terbuka setiap fase punya keputusan (tabel 9.1/11.1/… terisi) | grep |
| Semua | Dependensi: tidak ada lisensi copyleft kuat (GPL/AGPL) di runtime tanpa persetujuan; `docs/licenses.md` lengkap | `make licenses` (go-licenses + license-checker; verifikasi alat) |

### 9.2 Dokumentasi

Markdown biasa (keputusan 3; pertanyaan 13.2 #11). Struktur `docs/`:

```
docs/
├─ README.md                 # indeks: mulai cepat, konsep, panduan, operasi, referensi, ADR, rencana
├─ getting-started.md        # deploy produksi 10 menit (Compose), dev 5 menit (dari README root)
├─ concepts/                 # proses, instance, job/worker, timer, message, user task, FEEL, tenant
├─ guides/                   # modeler, operate, tasklist, menulis worker (SDK), auth/OIDC untuk klien
├─ reference/
│  ├─ api.md                 # dihasilkan dari proto (protoc-gen-doc atau buf plugin; verifikasi) + daftar ErrorInfo.reason + izin per RPC
│  ├─ configuration.md       # dihasilkan dari struct Config (go generate) — satu sumber
│  ├─ metrics.md             # dihasilkan dari daftar `platform/metrics`
│  ├─ bpmn-coverage.md       # dari ProfileExecutable (Fase 1 `go generate`)
│  └─ feel.md                # dari katalog fungsi Fase 7
├─ ops/                      # runbook (8.1)
├─ compatibility.md          # deviasi dari Camunda 8.8
├─ licenses.md
├─ adr/                      # + README.md indeks
└─ plan/
```

`tools/scripts/docscheck` (Go): memeriksa tautan relatif dan anchor; `make docs-check` di CI job
`check`. Dokumen referensi yang di-generate diperiksa sinkron seperti stub proto (`git diff
--exit-code` setelah `make docs-gen`). Bahasa: dokumentasi pengguna **bahasa Inggris** (UI dan
API bahasa Inggris, pengguna SDK eksternal); dokumen rencana/ADR tetap boleh Indonesia (CLAUDE.md).
Keputusan bahasa dokumentasi pengguna dicatat sebagai pertanyaan 13.2 #13.

### 9.3 Lisensi

Fakta yang harus diverifikasi sebelum keputusan:

| Komponen | Lisensi (diduga; verifikasi teks aktual) | Kewajiban |
|---|---|---|
| bpmn-js, diagram-js, dmn-js, form-js (bpmn.io) | "bpmn.io license" — bebas dipakai dengan syarat logo/watermark "Powered by bpmn.io" tidak dihapus/disembunyikan | Watermark tetap tampil di kanvas Modeler, Operate viewer, Tasklist pratinjau, DMN modeler; e2e memeriksa elemen watermark terlihat; dokumentasi menyebutnya |
| bpmn-js-properties-panel, @bpmn-io/properties-panel, zeebe-bpmn-moddle, camunda-bpmn-js-behaviors, @bpmn-io/feel-editor, lezer-feel | MIT (diduga) | Atribusi di `docs/licenses.md`/`NOTICE` |
| @xyflow/react | MIT; atribusi UI dapat disembunyikan dengan `proOptions` (Fase 6 mencatat "cek ketentuan") | Verifikasi; bila ada kewajiban, tampilkan |
| shadcn/ui, Radix, TanStack, Zustand, Vite, React | MIT | Atribusi |
| Go: connect-go (Apache-2.0), pgx (MIT), goose (MIT), go-oidc (Apache-2.0), otel (Apache-2.0), protovalidate (Apache-2.0) | — | Atribusi |
| Camunda 8 (`camunda/zeebe` image) | Lisensi Camunda (source-available; verifikasi versi lisensi 8.8) | **Hanya** dijalankan sebagai pembanding uji di profil opsional; tidak ada kode/binary Camunda yang didistribusikan atau dipakai runtime; dicatat eksplisit di `docs/licenses.md` dan README |
| DMN TCK (7b) | Apache-2.0 (diduga) | Atribusi bila kasus di-port |
| Keycloak, Caddy, Grafana stack, Postgres, nginx | Apache-2.0 / Apache-2.0 / AGPL-3.0 (Grafana, Loki, Tempo, Mimir) / PostgreSQL / BSD-2 | Dipakai sebagai layanan terpisah (tidak ditautkan ke kode kita); AGPL Grafana **tidak** memengaruhi lisensi kode kita, tetapi dicatat untuk operator |

Rekomendasi (ADR-0048): **Apache-2.0** untuk seluruh repo (`LICENSE`), dengan `NOTICE` dan
`docs/licenses.md`. Alasan: OSI, memuat pemberian hak paten, kompatibel dengan semua dependensi
runtime (MIT/Apache/BSD/bpmn.io), menjelaskan posisi terhadap Camunda (tidak ada kode Camunda
dipakai; kompatibilitas format dan perilaku bukan turunan kode), dan memungkinkan SDK dipakai
pihak luar. Alternatif bila repo tetap privat/proprietary: `LICENSE` berisi "Copyright (c) 2026
<pemilik>. All rights reserved." + `docs/licenses.md` tetap wajib untuk kepatuhan pihak ketiga.
Keputusan milik pemilik repo (pertanyaan 13.2 #12); `package.json` `license` diselaraskan;
header SPDX tidak dipaksakan di setiap file (cukup `LICENSE` + `go.mod` module path).

### 9.4 Rekonsiliasi ADR

Keadaan: nyata 0001–0009; rencana Fase 2 0010–0013; Fase 3 0014–0018; Fase 4 0017–0020
(**tumpang tindih 0017, 0018**); Fase 5 0023–0028; Fase 7 0027–0029 (**tumpang tindih 0027,
0028**); Fase 6 0030–0036; Fase 8 0040–0049. Karena fase dieksekusi berurutan dan nomor nyata
diberikan saat merge, langkah 10.1 Fase 8:

1. Membuat `docs/adr/README.md` (indeks: nomor, judul, status, fase, tanggal) dari file aktual.
2. Jika saat Fase 8 dimulai masih ada draft rencana yang nomornya bertabrakan dengan file nyata,
   nomor di **dokumen rencana** digeser (dokumen rencana bukan sumber kebenaran; file ADR yang
   ada adalah sumber kebenaran). Konvensi: nomor berikutnya = `max(nomor file) + 1`, tanpa celah
   yang disengaja; nomor 0037–0039 dipakai bila rekonsiliasi membutuhkan.
3. Setiap ADR Fase 8 dibuat dengan nomor dari indeks pada saat ditulis; jika 0040 ternyata sudah
   terpakai, Fase 8 memakai nomor berikutnya dan tabel di dokumen ini diperbarui di 13.1.
4. Lint ringan `tools/scripts/docscheck`: nomor ADR unik, setiap ADR punya `Status`, setiap
   rujukan `ADR-NNNN` di `docs/` menunjuk file yang ada.

ADR Fase 8 (judul rencana):

| ADR | Judul |
|---|---|
| 0040 | OpenTelemetry sebagai satu jalur telemetri; span link untuk batas asinkron; stack LGTM dev |
| 0041 | Autentikasi OIDC bearer di engine; PKCE di SPA; mode auth dev/none |
| 0042 | Otorisasi RBAC dengan izin per RPC yang dideklarasikan di proto |
| 0043 | Model multi-tenant: kolom tenant + scoping kueri, key global, tanpa registry |
| 0044 | Reverse proxy Caddy, routing host-based, TLS otomatis, API sama-origin lewat nginx |
| 0045 | Rollout dua slot engine di Compose dan graceful drain; batas zero-downtime |
| 0046 | Backup `pg_dump` terjadwal, latihan restore, PITR berbasis arsip WAL bawaan |
| 0047 | Rilis berversi: tag → image → bundel deploy; kebijakan upgrade/rollback; pembekuan `v1` |
| 0048 | Lisensi proyek dan kepatuhan pihak ketiga (watermark bpmn.io, posisi terhadap Camunda) |
| 0049 | Supply chain: pemindaian, SBOM, pin action, image non-root read-only |

## 10. Rencana kerja Fase 8 (urutan eksekusi)

Setiap langkah punya hasil yang bisa diverifikasi. Jalur **O** (observability: 10.2–10.4), **S**
(keamanan: 10.5–10.9), **T** (tenant: 10.10), **D** (deploy: 10.11–10.15), **R** (rilis: 10.16–10.20)
sebagian besar independen; titik temu dicatat.

### 10.1 Prasyarat, verifikasi, dan rekonsiliasi ADR

- Fase 1–7 di `main` (atau minimal Fase 1–6; FEEL 7a diinginkan, 7b opsional). Branch
  `feat/phase-8-hardening`; sub-branch per jalur bila > 1 orang.
- Periksa setiap baris "asumsi" di bagian 0 terhadap repo nyata; perbarui tabel.
- Verifikasi hal yang ditandai "verifikasi" di dokumen ini yang memengaruhi desain: API
  `otelconnect`, `go-oidc` `ProviderConfig`, image LGTM dan path provisioning, direktif Caddy,
  `deploy.resources` Compose, `oidc-client-ts` lisensi, `nginx-unprivileged`, dukungan `_FILE`
  Keycloak, `pg_basebackup` opsi, nama field tenant REST v2 Camunda 8.8. Hasil ditulis ke 13.1.
- Rekonsiliasi nomor ADR (9.4): `docs/adr/README.md`, perbarui dokumen rencana yang bertabrakan.
- Verifikasi hook 4.7; buat daftar perubahan kecil yang dibutuhkan pada modul fase sebelumnya.

Hasil: branch siap; tabel 0 dan 13.1 terisi; `docs/adr/README.md` ada; `make check` hijau.

### 10.2 Platform `otel`: provider, exporter, propagasi, interceptor

- `platform/otel` (4.1): `Setup`, `Inject/Extract/LinkFrom`, resource, sampler komposit.
- `platform/rpc/tracing.go`, `metrics.go` dengan otelconnect; atribut `tenant_id`, `request.id`.
- `logger/otel_handler.go` (4.3); `ENGINE_LOG_FORMAT`.
- Test: exporter in-memory (`sdktrace/tracetest`, `sdkmetric/metricdata`) memverifikasi span RPC
  dan atribut log.

Hasil: `make dev` dengan `OTEL_EXPORTER_OTLP_ENDPOINT` mengirim span ke LGTM (10.3) — dibuktikan
screenshot; tanpa endpoint, tidak ada perubahan perilaku; `go test ./apps/engine/internal/platform/otel/...` lolos.

### 10.3 Profil `observability` dev dan dashboard

- Ganti `otel-collector` dengan `otel-lgtm` (4.5); `make observe`.
- Provisioning datasource, dashboard (empat JSON awal dengan panel dari metrik yang **sudah ada**),
  alert YAML.
- `docs/ops/observability.md` draf.

Hasil: `make up PROFILES=observability` → Grafana menampilkan dashboard Engine Overview dengan data
dari `make demo` (Fase 4).

### 10.4 Trace lintas batas dan metrik bisnis

- Migrasi `000NN_otel.sql` (kolom `trace_context` di `outbox_event`, `job`, `timer`,
  `message_subscription`) — digabung dengan migrasi tenant 6.2 menjadi satu file expand.
- `command_runner` span `command.*`; `outbox.Writer.Enqueue` menulis `trace_context`; relay
  membuat span dengan link; projector menulis `trace_context` job/timer/subscription; trigger timer
  dan korelasi message membuat span dengan link.
- `ActivatedJob.trace_context` (proto aditif) + `sdk/otel.WorkerTracing`; `worker-example`
  memakainya.
- Metrik bisnis 4.2 + `metrics.Collector` gauge; `TestMetricLabels`; snapshot nama diperbarui.
- Test integrasi "satu trace utuh" (DoD) dengan exporter in-memory di engine dan SDK.

Hasil: DoD trace dan metrik; dashboard 10.3 diperbarui dengan panel bisnis; screenshot trace
end-to-end di PR.

### 10.5 Authn OIDC, mode auth, Keycloak profil `auth`

- `platform/rpc/authn/{oidc,dev,anonymous}.go`, `Principal` diperluas, konfigurasi 5.1, `_FILE`
  (5.7) karena `ENGINE_OIDC_*` mungkin rahasia.
- `infra/keycloak/realm-workflow.json`; service `keycloak` profil `auth` di compose dev
  (`start-dev --import-realm`, port 8180); `make auth-up`.
- Subperintah `engine token-inspect`.
- Test unit dengan JWKS lokal (semua jalur penolakan, cache `kid`, IdP mati); test integrasi
  terhadap Keycloak (`-tags=integration,auth`, dijalankan di CI dengan service container Keycloak
  — verifikasi waktu start; bila > 60 s, hanya nightly).

Hasil: `curl` tanpa token → 401 `TOKEN_MISSING`; dengan token Keycloak → sukses; `ENGINE_ENV=production
ENGINE_AUTH_MODE=none` menolak start; ADR-0041 draf.

### 10.6 Authz: opsi proto, matriks peran, interceptor, `IdentityService`

- `common/v1/options.proto`; tambahkan `option permission` ke **semua** RPC Fase 1–7; `buf generate`.
- `authz/{permissions,interceptor}.go`, `TestEveryRPCHasPermission`, `TestRoleMatrix`,
  `ENGINE_AUTHZ_EXTRA_GRANTS`.
- Modul `identity` + `identity/v1/identity.proto` (`GetMe`).
- Penegakan kepemilikan di `usertask` (5.3) bila Fase 6 belum menegakkan untuk principal non-anonim.
- Test tabel lima peran × semua RPC (lewat `httptest` dan mode `dev` dengan `X-Dev-Roles`).

Hasil: DoD authz; `buf breaking` hijau; ADR-0042 draf.

### 10.7 SDK: client credentials, propagasi token; client TS: bearer

- `libs/go/sdk/oidc` (5.4); `JobWorker` memperbarui token sebelum long poll; `worker-example`
  dengan `WORKER_OIDC_*`; `make demo AUTH=1`.
- `libs/web/api-client`: interceptor `bearerToken(getToken)`; `findErrorInfo` mengenal reason baru.
- Test: token berumur 5 s di Keycloak dev (client scope dengan lifetime pendek) → worker lanjut
  tanpa kehilangan job.

Hasil: `make demo AUTH=1` selesai < 30 s; SDK `deps_test.go` tetap lolos (paket `oidc`/`otel`
dikecualikan dengan aturan eksplisit).

### 10.8 UI: login PKCE, `GetMe`, pemilih tenant, e2e dengan OIDC

- `app-shell` `AuthProvider` (5.5) + route callback/silent; `useAuth().can()`; `TenantContext`;
  `config.js` baru (`OIDC_ISSUER`, `OIDC_CLIENT_ID`, `AUTH_MODE`); hapus `VITE_DEV_IDENTITY`
  (diganti `AUTH_MODE=dev`).
- Sembunyikan aksi per izin di Modeler/Operate/Tasklist; penanganan `Unauthenticated`/`PermissionDenied`
  (toast + login ulang).
- Playwright: fixture `login(role)` terhadap Keycloak profil `auth`; smoke Fase 6 untuk tiga peran;
  test negatif `user`; CI job `e2e` menjalankan Keycloak (service container atau compose).

Hasil: DoD UI; screenshot alur login di PR.

### 10.9 CORS, rate limit, batas request, rahasia, supply chain, header

- `ratelimit` interceptor + konfigurasi; `ENGINE_ENV` guard untuk CORS; `MaxHeaderBytes`.
- `config/secretfile.go`, `ENGINE_DB_PASSWORD_FILE`, `Config.String()` tersamar, `make secrets-init`.
- CI job `security`; Trivy image di job `docker`; SBOM; `.github/dependabot.yml`; pin action ke SHA;
  `.trivyignore` kosong dengan format.
- nginx: header + CSP + `/csp-report` + `client_max_body_size`; `nginx-unprivileged`; `make scan-headers`.
- Engine image: `read_only`, `cap_drop` di compose; `web.Dockerfile` port 8080.

Hasil: DoD CORS/rate limit/rahasia/scan/header; e2e Fase 6 tetap hijau di bawah CSP; ADR-0049 draf.

### 10.10 Multi-tenant

- Migrasi 6.2 (gabung dengan kolom `trace_context` 10.4); projector menulis `tenant_id` job/incident;
  `VerifyTenantMismatch`.
- Interceptor `tenant`, `Resolve`/`Visible`; parameter `tenant` ke semua use case dan kueri sqlc
  (perubahan mekanis per modul: deployment, execution, job, message, timer, usertask, decision);
  `GetLatest(tenant, id)`; `ProcessDefinitions` cache.
- Proto aditif 5.10 (`tenant_id`/`tenant_ids`); SDK `WithDefaultTenant`, `JobWorker.TenantIDs`;
  UI pemilih tenant (10.8) mengirim field.
- `TestTenantScopedQueries`; test integrasi dua tenant; kuota `ENGINE_TENANT_MAX_ACTIVE_INSTANCES`.
- Semua test Fase 1–7 tetap lolos dengan multi-tenancy off **dan** on (parameter test).

Hasil: DoD multi-tenant; `docs/ops/tenant-onboarding.md`; ADR-0043 draf.

### 10.11 Compose produksi, Caddy, TLS lokal

- `docker-compose.prod.yml` (7.1), `caddy/Caddyfile` (3.1), `.env.prod.example`, jaringan
  `edge`/`backend`, `secrets`, `init-keycloak.sql`, nginx `upstream` dua slot + `config.js`.
- `make deploy-local` dengan `DOMAIN=localhost`; `make tls-local`; `make deploy-local AUTH=none`.
- Uji di VM dengan DNS nyata (staging) → sertifikat Let's Encrypt; dicatat di 0.

Hasil: DoD deploy; `curl --fail https://api.localhost/healthz`; ADR-0044 draf.

### 10.12 Graceful drain, listener admin, rollout dua slot

- `httpserver/drain.go`, `longPoll.Close()` di modul `job`, `background.Stop` (relay, sweeper,
  janitor, collector), urutan di `main.go`; `platform/admin`; `/version`; `engine version`.
- `tools/scripts/rollout` + `make rollout` / `make rollback`; `.env.prod.bak`.
- Test integrasi urutan drain (log); skenario chaos 17 dan 18.

Hasil: DoD rollout dan drain; `make chaos SCENARIO=rolling-upgrade` hijau 3×; ADR-0045 draf.

### 10.13 Backup terjadwal, latihan restore, PITR

- `infra/docker/backup/*`, image `db-backup` dibangun CI; `postgres/archive.sh`, `pitr.conf`;
  `make db-backup` (Fase 3) kini memicu container; `make db-restore-drill`; CI `backup-drill.yml`.
- `docs/ops/restore.md` (pg_dump + PITR); latihan PITR end-to-end dicatat.
- Alert `BackupStale`/`WalArchiveFailing` dengan sumber metrik yang terverifikasi.

Hasil: DoD backup/PITR; skenario chaos 24–25; ADR-0046 draf.

### 10.14 Batas sumber daya, rotasi log, overlay observability prod

- `x-logging`, `deploy.resources`, `GOMEMLIMIT`, `stop_grace_period`; `docker-compose.observability.yml`
  + `otel-collector.yaml` + Grafana di `grafana.{$DOMAIN}`; retensi.
- Load test singkat untuk memverifikasi batas tidak terlampaui.

Hasil: `docker stats` selama `make loadtest SCENARIO=baseline` di bawah batas; overlay prod
menampilkan dashboard yang sama dengan dev.

### 10.15 Chaos Fase 8 dan load test

- Skenario 19–23 di `tools/scripts/chaos`; `tools/scripts/loadtest` dengan delapan skenario dan
  laporan; `make loadtest`; CI `loadtest.yml` mingguan; `docs/ops/capacity.md` terisi angka.
- Ulangi `baseline` pada rc dan kunci angka SLO (4.6) di dokumen ini (13.1).

Hasil: DoD load test dan chaos; laporan terlampir di PR.

### 10.16 Rilis berversi, upgrade/rollback drill

- `cliff.toml`, `make release`, `release.yml` (image, SBOM, bundel deploy, GitHub Release),
  `upgrade-drill.yml`; `docs/ops/upgrade.md`, `rollback.md`; pembekuan `v1` dicatat di ADR-0020
  (amendemen) dan ADR-0047.
- Rilis `v1.0.0-rc.1` nyata ke GHCR untuk menguji seluruh alur.

Hasil: GitHub Release `v1.0.0-rc.1` dengan artefak lengkap; drill upgrade/rollback hijau; ADR-0047.

### 10.17 Runbook dan dokumentasi operasi

- Semua runbook 8.1 ditulis dan dijalankan sekali; `make support-bundle`; `engine token-inspect`
  di `oidc-setup.md`.

Hasil: tabel 0 memuat tanggal pelaksanaan setiap runbook.

### 10.18 Dokumentasi 1.0, lisensi, indeks

- `docs/README.md`, `getting-started.md`, `concepts/`, `guides/`, `reference/` (generator
  `make docs-gen` untuk konfigurasi, metrik, API, cakupan BPMN, FEEL), `compatibility.md`,
  `tools/scripts/docscheck` + `make docs-check` di CI.
- `LICENSE`, `NOTICE`, `docs/licenses.md` (`make licenses` memverifikasi alat), e2e watermark
  bpmn.io; `package.json` `license`.

Hasil: `make docs-check` hijau; ADR-0048; keputusan lisensi tercatat di 13.1.

### 10.19 Stretch: Helm chart

- Hanya bila 10.1–10.18 selesai lebih cepat dari estimasi; `infra/helm/workflow-engine`, uji `kind`,
  README "eksperimental".

Hasil: `helm template` lolos `kubeconform` (verifikasi alat); tidak masuk DoD.

### 10.20 Checklist rilis 1.0, ADR final, penutupan

- `docs/plan/release-1.0-checklist.md` terisi (9.1) termasuk paritas Fase 1–7 live pada rc terakhir;
  ADR-0040–0049 final; pertanyaan 13.1 terisi; `docs/plan/README.md` diperbarui; review DoD.
- `make release VERSION=v1.0.0`.

Hasil: tag `v1.0.0`; semua kotak DoD tercentang dengan bukti.

## 11. Target Nx dan Makefile

| Proyek | Target | Perintah |
|---|---|---|
| `engine` | `test` | sudah ada; mencakup `platform/{otel,rpc/authn,rpc/authz,rpc/ratelimit,rpc/tenant,admin,config}`, modul `identity` |
| `engine` | `test-integration` | ditambah test dua tenant, drain, trace end-to-end |
| `engine` | `test-auth` (cache off) | `go test -count=1 -tags=integration,auth ./apps/engine/...` terhadap Keycloak profil `auth` |
| `engine` | `docs-gen` | `go generate ./apps/engine/internal/platform/{config,metrics}/...` → `docs/reference/{configuration,metrics}.md` |
| `proto` | `generate`, `lint`, `breaking` | mencakup `common/v1/options.proto`, `identity/v1`, field tenant/trace |
| `sdk-go` | `test` | mencakup `sdk/oidc`, `sdk/otel` |
| `api-client`, `app-shell`, tiga app | `test`, `typecheck`, `build`, `e2e` | e2e kini memakai Keycloak |
| `rollout`, `loadtest`, `docscheck` (proyek baru `tools/scripts/*`, tag `type:tool`) | `run` (cache off) | `go run ./tools/scripts/<nama> …` |
| `chaos` | `run` | skenario baru |
| `db-backup` (proyek `infra/docker/backup`, tag `type:infra`) | `docker-build` | image backup |

Target Makefile baru/diperluas:

```
make observe                         # buka Grafana dev (profil observability)
make auth-up / auth-down             # Keycloak dev (profil auth), realm ter-import, port 8180
make demo AUTH=1                     # worker-example dengan client credentials
make secrets-init [SECRETS_DIR=…]    # buat password acak untuk compose prod
make tls-local                       # ekspor root CA Caddy lokal + petunjuk pasang
make deploy-local [AUTH=none|oidc]   # compose prod lokal dengan DOMAIN=localhost
make rollout VERSION=vX.Y.Z          # rollout dua slot
make rollback                        # kembali ke image sebelumnya
make db-backup-now                   # picu backup di container db-backup
make db-restore-drill                # restore ke DB sementara + verify
make loadtest [SCENARIO=baseline] [DURATION=10m]
make chaos SCENARIO=rolling-upgrade|…
make scan                            # govulncheck + pnpm audit + trivy fs (lokal)
make scan-headers [URL=https://modeler.localhost]
make sbom                            # SBOM keempat image ke tmp/sbom
make licenses                        # daftar lisensi dependensi → docs/licenses.md (periksa)
make docs-gen / docs-check
make release VERSION=vX.Y.Z
make support-bundle                  # tmp/support-<ts>.tar.gz
make stop-engine / start-engine      # scale kedua slot (untuk restore)
```

CI: job `security` (per PR), `e2e` memakai Keycloak, `docker` + Trivy + SBOM, workflow
`release.yml`, `backup-drill.yml` (mingguan), `upgrade-drill.yml` (mingguan), `loadtest.yml`
(mingguan), `chaos.yml` (nightly, diperluas).

## 12. Risiko dan mitigasi

| Risiko | Dampak | Mitigasi |
|---|---|---|
| Fakta pustaka/produk yang ditandai "verifikasi" ternyata berbeda (LGTM provisioning, direktif Caddy, `go-oidc` discovery override, `deploy.resources` Compose, `nginx-unprivileged`) | Desain detail bergeser | 10.1 memverifikasi semua sebelum kode; setiap item punya cadangan yang disebut di tempatnya (collector + Grafana terpisah; Traefik; verifier JWT manual dengan `lestrrat-go/jwx` — verifikasi; `mem_limit`; nginx root dengan `cap_add`) |
| Fase 2–7 belum merge saat Fase 8 mulai | Hook 4.7 tidak ada; metrik tidak ada | Jalur O/S/D bisa dikerjakan terhadap `health` + `deployment` dulu; metrik bisnis ditambahkan saat modulnya ada; 10.1 mencatat urutan |
| Token di memori SPA dicuri lewat XSS | Pencurian sesi | CSP ketat tanpa `unsafe-eval`, token ≤ 15 m, rotation, `useAuth` closure; e2e dengan CSP; BFF sebagai jalur lanjutan (3.2) |
| CSP memecahkan bpmn-js/form-js/properties panel (style/inline, blob, worker) | UI tidak berfungsi | `report-uri` dulu (mode report-only di langkah pertama 10.9, lalu enforce); e2e penuh di bawah CSP; daftar pengecualian minimum terdokumentasi |
| IdP tidak terjangkau dari engine (DNS `auth.<domain>` tidak resolve di Docker) | Semua RPC gagal | `ENGINE_OIDC_DISCOVERY_URL` internal + `iss` publik; chaos 19/21; `IDP_UNAVAILABLE` retryable |
| Isolasi tenant terlewat di satu kueri | Kebocoran data antar tenant | `TestTenantScopedQueries` + test dua tenant yang memanggil **setiap** RPC baca; review wajib untuk kueri baru; RLS sebagai lapisan kedua lanjutan |
| Rollout gagal di tengah (slot B baru rusak) | Hanya satu slot melayani | Skrip berhenti sebelum menyentuh slot A; slot A tetap versi lama; `make rollback` |
| Migrasi contract tidak sengaja masuk rilis minor | Rollback tidak aman | Checklist rilis mewajibkan label migrasi (`-- +phase: expand|contract`) dan `release.yml` menolak contract tanpa catatan rilis eksplisit |
| Dua versi binary menulis record dengan payload berbeda selama rollout | Replay/verify gagal | Aturan Fase 3: payload record hanya bertambah (aditif), `format_version` snapshot; test `upgrade-drill` |
| `ActivateJobs` retry Caddy mengirim POST dua kali | Aktivasi ganda? | Aktivasi `SKIP LOCKED` tetap tepat satu kali per job; `lb_try_duration` hanya mencoba ulang bila upstream tidak terjangkau sebelum request terkirim (verifikasi); SDK `request_id` idempoten untuk mutasi |
| Kardinalitas metrik meledak (`job_type`, `bpmn_process_id` tak terbatas) | Prometheus berat | Lint label; batas 100 tipe teratas untuk gauge; dokumentasi "tipe job bukan id unik" |
| Sampling 10 % menyembunyikan trace yang dibutuhkan | Diagnosis sulit | Error selalu disampel; tail sampling di collector untuk prod bila diperlukan; TraceQL by `process_instance_key` pada 10 % tetap berguna; `OTEL_TRACES_SAMPLER_ARG` dapat dinaikkan sementara |
| LGTM satu image berat di laptop | Dev lambat | Profil opsional; `ENABLE_LOGS_ALL=false`; batas memori di compose dev |
| Backup berjalan saat beban tinggi memperlambat DB | Latensi | Jadwal malam; `pg_dump` snapshot MVCC; `nice`; dicatat di capacity |
| PITR arsip WAL memenuhi disk | Postgres berhenti menulis | Pemangkasan arsip setelah base backup; alert disk dan `WalArchiveFailing`; skenario 26 |
| Keycloak di compose prod menjadi beban operasi (DB, upgrade) | Operator kewalahan | Profil opsional; dokumentasi mengutamakan IdP yang sudah ada; realm export dicadangkan bersama `db-backup` (DB `keycloak` ikut di-dump: `pg_dumpall` opsional atau `pg_dump` kedua) |
| `docker compose` di Windows (Docker Desktop) untuk `deploy-local` dengan `*.localhost` dan CA internal | Developer Windows tidak bisa menguji TLS | `make tls-local` memberi langkah `certutil`; alternatif `AUTH=none` + HTTP tanpa Caddy untuk smoke (`docker-compose.prod.yml` tetap butuh Caddy; profil `edge-none` mengekspos nginx langsung di port) |
| Lisensi bpmn.io berubah atau watermark dianggap tidak dapat diterima pemilik | Pelanggaran atau ganti library | Verifikasi teks lisensi; e2e menjaga watermark; alternatif komersial bpmn.io dicatat |
| Grafana stack AGPL mengkhawatirkan operator | Penolakan stack | Overlay opsional; endpoint OTLP ke platform mana pun; tidak ada kode AGPL di produk |
| Estimasi meleset karena cakupan luas | Fase panjang | Prioritas: DoD inti (observability, auth, deploy, backup) sebelum tenant on-by-request dan stretch; pertanyaan 13.2 #1 memutuskan apa yang boleh digeser ke 1.1 |

## 13. Pertanyaan terbuka

Setiap pertanyaan diberi rekomendasi. Keputusan final dicatat di 13.1 setelah review.

### 13.1 Keputusan

Belum ada. Diisi dengan tabel yang sama seperti 9.1 dokumen Fase 1, ditambah hasil verifikasi
10.1 dan nomor ADR final.

### 13.2 Pertanyaan dan rekomendasi

| # | Pertanyaan | Rekomendasi | Alasan |
|---|---|---|---|
| 1 | Cakupan Fase 8 terlalu luas untuk satu fase: apa yang boleh bergeser ke 1.1? | **Boleh bergeser**: PITR (tetap `wal_level=replica` + runbook), overlay observability prod (cukup OTLP ke eksternal), Helm, k6, `ENGINE_METRICS_PROMETHEUS`, kuota tenant. **Tidak boleh**: OIDC + RBAC, CORS/CSP/rate limit, trace+metrik+dashboard dasar, Caddy/TLS, rollout dua slot, backup terjadwal + drill restore, scan keamanan, rilis berversi, lisensi | Tanpa yang terakhir, 1.0 tidak aman dioperasikan; yang pertama adalah kenyamanan |
| 2 | Reverse proxy: Caddy atau Traefik? | **Caddy** (3.1) | TLS otomatis paling sederhana; host-based tanpa label |
| 3 | Simpan `trace_id` di `execution_record` untuk tautan Operate → Tempo per record? | **Tidak di 1.0**; atribut `process_instance_key` di span + TraceQL cukup; kolom dapat ditambah aditif kelak | Menghindari perubahan payload record dan ukuran tabel terbesar |
| 4 | Hapus `DevIdentity` (janji Fase 6) atau pertahankan sebagai `ENGINE_AUTH_MODE=dev`? | **Pertahankan sebagai mode `dev`**, dilarang di `ENGINE_ENV=production` | e2e dan dev lokal tanpa Keycloak tetap cepat; guard produksi menutup risikonya |
| 5 | Verifikasi JWT di engine (bearer) atau di Caddy (forward auth)? | **Di engine** | Caddy tidak mengenal tenant/izin; SDK memanggil `api.` langsung; satu tempat kebenaran |
| 6 | Peran `user`: `process:read` penuh atau hanya instance yang memuat task-nya? | **Penuh di 1.0** (tenant-scoped), dengan catatan; pembatasan per instance bersama ACL resource di 1.x | ACL per instance butuh model izin resource yang belum ada; Tasklist menampilkan konteks proses |
| 7 | Rate limit di engine (per replika) atau terpusat (Redis)? | **Engine, per replika** | Tanpa dependensi baru; dua replika = 2× batas nominal, cukup sebagai pelindung |
| 8 | Login SPA: PKCE di browser atau BFF? | **PKCE** (3.2) | Tanpa komponen baru; CSP sebagai mitigasi |
| 9 | SSE/streaming pembaruan langsung (ditunda Fase 6 ke Fase 8)? | **Tidak di 1.0**; polling adaptif tetap; `useInstanceEvents` dan consumer `operate.projection` masuk 1.1 bila load test menunjukkan biaya polling > 10 % beban DB | Fase 8 sudah padat; kontrak hook Fase 6 membuat penggantian murah |
| 10 | Signing image (cosign) di 1.0? | **Opsional, off**; `release.yml` punya langkah yang dinyalakan lewat variabel repo | Verifikasi tanda tangan hanya bermanfaat bila operator memverifikasi |
| 11 | Situs dokumentasi: Markdown biasa atau mkdocs/Docusaurus? | **Markdown biasa** + indeks + pemeriksa tautan; generator statis ditinjau setelah ada pengguna eksternal | Biaya nol; GitHub merender |
| 12 | Lisensi proyek? | **Apache-2.0** (9.3); keputusan pemilik repo | OSI, paten, kompatibel dependensi; posisi jelas terhadap Camunda |
| 13 | Bahasa dokumentasi pengguna (`docs/getting-started`, `guides`, `reference`)? | **Inggris**; rencana/ADR tetap Indonesia | UI/API Inggris; SDK untuk pihak luar |
| 14 | Keycloak di compose prod (profil `auth`) atau hanya dev? | **Keduanya**, prod sebagai profil opsional dengan DB di Postgres yang sama | Organisasi tanpa IdP butuh jalur lengkap; yang punya IdP tidak menyalakannya |
| 15 | Multi-tenancy on atau off secara default di `.env.prod.example`? | **Off** | Mayoritas deploy satu organisasi; menyalakan = keputusan sadar + onboarding runbook |
| 16 | PITR: arsip WAL bawaan atau pgBackRest/WAL-G? | **Bawaan** di 1.0 (7.3); alat khusus bila butuh offsite/inkremental/enkripsi | Tanpa komponen baru; cukup untuk satu host |
| 17 | Dua slot engine selalu (default) atau satu slot dengan `ENGINE_SLOTS=1`? | **Dua slot default**; `make deploy-local` boleh satu (`ENGINE_SLOTS=1` → `engine-b` di profil `ha`) | Produksi mendapat zero-downtime tanpa langkah tambahan; laptop hemat |
| 18 | `ActivateJobs` kosong: dihilangkan dari trace sama sekali atau disampel rendah? | **Disampel rendah (1 %)** dengan atribut `rpc.result=empty`, metrik tetap penuh | Masih bisa mendiagnosis worker yang salah tipe |
| 19 | Metrik gauge berbasis DB di dua replika: keduanya lapor atau hanya pemegang advisory lock? | **Keduanya**, dashboard `max by` | Lebih sederhana; nilai identik |
| 20 | `StreamActivatedJobs` (Fase 4 #1) dan `LISTEN/NOTIFY` lintas replika (Fase 4 #2) di Fase 8? | **Tidak**; dicatat di 15 | Polling cadangan 1 s cukup untuk SLO; prioritas hardening |
| 21 | Pembekuan `v1` di 1.0 — perlukah `v2` untuk merapikan hal yang sudah `deprecated`? | **Tidak**; `v1` dibekukan apa adanya, `deprecated` dipertahankan sampai `v2` ada alasan nyata | Stabilitas untuk klien |

## 14. Estimasi dan pembagian kerja

Perkiraan **34–40 hari kerja** untuk 1 engineer, atau **18–22 hari** untuk 2 engineer paralel
(tanpa 10.19; PITR dan overlay prod termasuk):

| Hari | Fokus |
|---|---|
| 1–2 | 10.1 prasyarat, verifikasi pustaka/produk, rekonsiliasi ADR |
| 3–4 | 10.2 platform `otel`, interceptor tracing/metrics, log berkorelasi |
| 5 | 10.3 profil `observability` LGTM, dashboard awal |
| 6–8 | 10.4 trace lintas batas (outbox, timer, message, job), metrik bisnis, test trace utuh |
| 9–11 | 10.5 authn OIDC, mode auth, Keycloak profil `auth`, `token-inspect` |
| 12–13 | 10.6 authz: opsi proto di semua RPC, matriks, interceptor, `IdentityService` |
| 14 | 10.7 SDK client credentials, client TS bearer, demo auth |
| 15–17 | 10.8 UI login PKCE, `GetMe`, pemilih tenant, e2e dengan Keycloak |
| 18–19 | 10.9 rate limit, rahasia `_FILE`, CI security/SBOM/Dependabot, CSP + header |
| 20–23 | 10.10 multi-tenant: migrasi, interceptor, parameter tenant di semua modul, lint, test dua tenant |
| 24–25 | 10.11 compose prod, Caddy, TLS lokal, uji staging dengan Let's Encrypt |
| 26–27 | 10.12 drain, listener admin, rollout dua slot, chaos 17–18 |
| 28–29 | 10.13 backup terjadwal, drill restore, PITR + latihan |
| 30 | 10.14 batas sumber daya, rotasi log, overlay observability prod |
| 31–32 | 10.15 chaos 19–25, load test delapan skenario, capacity |
| 33–34 | 10.16 rilis, `release.yml`, upgrade/rollback drill, rc.1 |
| 35–36 | 10.17 runbook dijalankan; 10.18 dokumentasi, lisensi, docs-check |
| 37–38 | 10.20 checklist rilis 1.0 (paritas live Fase 1–7 ulang), ADR final, review DoD, `v1.0.0` |
| (39–40) | cadangan untuk temuan verifikasi 10.1 dan perbaikan paritas |

Pembagian jika 2 orang: **A** = observability (10.2–10.4), deploy (10.11–10.14), rilis
(10.16), chaos/load (10.15); **B** = keamanan (10.5–10.9), tenant (10.10), UI (10.8), dokumentasi
dan lisensi (10.17–10.18). Titik temu: 10.4 ↔ 10.10 (satu file migrasi expand bersama), 10.8 ↔ 10.11
(`config.js` dan Caddy host), 10.12 ↔ 10.15 (skenario rollout), 10.20 bersama.

Estimasi untuk 1 engineer bisa ditekan ke **28–30 hari** dengan menggeser item pertanyaan 13.2 #1
ke 1.1.

## 15. Setelah Fase 8

Dengan `v1.0.0`, roadmap `docs/plan/README.md` selesai: engine BPMN 2.0 yang kompatibel Camunda
Modeler, dengan persistensi tahan crash, protokol worker dan SDK, timer/message/scope, tiga UI, FEEL
penuh (dan DMN bila 7b dikerjakan), serta dapat dioperasikan dengan auth, observability, dan deploy
yang terbukti. Kandidat pekerjaan lanjutan, diurutkan menurut nilai yang penulis perkirakan;
masing-masing membutuhkan dokumen rencana sendiri:

| Kandidat | Ringkasan | Prasyarat yang sudah disiapkan |
|---|---|---|
| DMN (7b) bila belum | `libs/go/dmn`, modul `decision`, dmn-js | Fase 7 bagian 7; `tenant_id` dan izin `decision:evaluate` sudah dirancang di Fase 8 |
| Clustering dan partisi | Pembagian kepemilikan instance antar replika (`hash(process_instance_key) mod N`), routing command, partisi tabel `execution_record` per rentang `position`; Postgres HA (Patroni/replika streaming + failover) | ADR-0017 (key global tanpa bit partisi, pemicu peninjauan), benchmark contention Fase 3, dua slot engine Fase 8 |
| Otorisasi tingkat resource | ACL per process definition / instance / tenant (siapa boleh start proses X, melihat instance Y); kebijakan sebagai file atau tabel; peran kustom | Interceptor `authz` dan izin per RPC Fase 8; `Principal.Groups` |
| Registry tenant dan kuota per tenant | Tabel `tenant` (nama, status, kuota, pengaturan), API admin tenant, kuota berbeda per tenant, RLS Postgres sebagai lapisan kedua | Model tenant Fase 8 (3.5 opsi C) |
| Pembaruan langsung UI | SSE `/api/events` dari consumer outbox `operate.projection`, `useInstanceEvents`; `StreamActivatedJobs` untuk worker | Kontrak hook Fase 6 (`queryKey` stabil), outbox durable Fase 3 |
| Signal event | Broadcast lintas instance, signal start subscription, `BroadcastSignal` RPC | Parser Fase 1 (v18), pola subscription Fase 5 |
| Element templates dan editor form | Element templates Camunda di properties panel; form-js editor di Modeler; penyimpanan diagram di server dengan pemilik | Fase 6 pertanyaan #4/#6; identitas pemilik dari Fase 8 |
| Modifikasi dan migrasi instance | `ModifyProcessInstance` (activate/terminate elemen), migrasi instance antar versi definisi, aksi operator pada timer/subscription | Fase 5 pertanyaan #9; `terminateSubtree` |
| Exporter dan integrasi | Exporter record ke Kafka/webhook (kursor `position`), audit log terpisah, integrasi Camunda Connectors-like lewat worker | `execution_record.position` (ADR-0017 butir 3) |
| Kubernetes operator / Helm resmi | Chart yang dirawat, operator untuk rollout dan backup | Stretch 10.19 |
| Backup lanjutan | pgBackRest/WAL-G offsite, enkripsi, inkremental; drill otomatis bulanan | Arsip WAL bawaan Fase 8 |
| BFF auth | Sesi cookie `HttpOnly` untuk organisasi yang melarang token di browser | Kontrak `useAuth()` Fase 6/8 |
| SDK non-Go | TypeScript Node worker (dari stub yang sudah ada), Java/Python | Kontrak proto `v1` beku |
| Pemangkasan/kompaksi record | Kompaksi berdasarkan snapshot bila riwayat dipindah ke exporter | ADR-0018, `format_version` |
| i18n id-ID | Katalog pesan dari `messages.ts` Fase 6 | Fase 6 pertanyaan #8 |
