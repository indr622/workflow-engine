# Fase 0 — Persiapan Environment

**Proyek:** Workflow Engine (kompatibel konsep Camunda 8 / BPMN 2.0)
**Stack:** Go (engine utama) + React (UI) dalam monorepo Nx
**Status:** Draft rencana — belum dieksekusi
**Tanggal:** 2026-10-27

---

## 1. Lingkup

Proyek ini **membangun engine workflow dari nol** di Go dengan fitur-fitur setara Camunda 8 (Zeebe):
deploy BPMN 2.0, process instance, job worker, user task, timer, message, incident, FEEL expression,
serta UI modeler/operate/tasklist di React. Tidak ada komponen Camunda yang dipakai di runtime.
Camunda 8 asli hanya dijalankan sebagai **pembanding** (profil opsional di Docker Compose) untuk uji
paritas perilaku, dan format BPMN yang diterima harus kompatibel dengan Camunda Modeler.

Fase 0 **tidak** menulis logika engine. Tujuannya satu: setiap engineer bisa clone repo, menjalankan
satu perintah, dan mendapatkan engine Go + UI React + database hidup, dengan lint/test/build lewat Nx
dan CI hijau.

## 2. Definition of Done Fase 0

- [ ] `git clone` → `make setup` → `make check` berjalan hijau di Windows dan CI.
- [ ] `make up` menyalakan PostgreSQL 18 (dan opsional Camunda 8 referensi lewat profil).
- [ ] `make dev` menjalankan engine Go (modul `health` dengan lima lapisan) dan modeler-web (fitur `health` dengan empat lapisan).
- [ ] Modeler-web memakai shadcn/ui dari `libs/web/ui`, TanStack Query untuk `/healthz`, TanStack Router untuk halaman, Zustand untuk state klien.
- [ ] Satu file `.proto` contoh menghasilkan stub Go dan TypeScript lewat `make gen`, dan hasilnya terpakai di kedua sisi.
- [ ] Aturan batas lapisan dan antar modul ditegakkan lint (depguard, enforce-module-boundaries, eslint-plugin-boundaries) dan dibuktikan dengan satu pelanggaran sengaja yang gagal lint.
- [ ] `make docker-build` menghasilkan image `engine` dan `web`; `make deploy-local` menjalankan keduanya bersama Postgres lewat compose prod dan `/healthz` menjawab ok.
- [ ] `nx affected` di CI hanya mem-build proyek yang berubah (cache Nx bekerja); CI mem-build image Docker.
- [ ] Pre-commit hook menjalankan format + lint pada file yang di-stage.
- [ ] README, CONTRIBUTING, dan 7 ADR tersimpan di `docs/`.

## 3. Kondisi mesin saat ini (hasil cek 2026-10-27)

| Tool | Status | Versi terpasang | Tindakan |
|---|---|---|---|
| Go | ✅ | 1.27.1 windows/amd64 | — |
| Node.js | ✅ | 22.23.4 (LTS) | — |
| pnpm | ✅ | 12.3.4 | — |
| Docker Desktop + Compose | ✅ | 29.8.0 / v5.5.1 | pastikan backend WSL2 aktif |
| Git | ✅ | 2.55.0 | set `core.autocrlf=false` |
| golangci-lint | ✅ | 2.15.0 | — |
| GNU Make | ✅ | 4.4.1 | — |
| buf | ❌ | — | install (lihat 5.1) |
| protoc plugins (Go, connect) | ❌ | — | install via `go install` |
| PostgreSQL | lewat Docker | `postgres:18` | tidak perlu instal native |

Folder `D:\workflow-engine` masih kosong dan **belum** menjadi repositori git.

## 4. Keputusan teknis (dibekukan di Fase 0, dicatat sebagai ADR)

| Area | Keputusan | Alasan singkat |
|---|---|---|
| Monorepo | Nx 21 + pnpm workspace | Nx punya affected graph + cache; pnpm hemat disk dan strict |
| Go di Nx | Plugin `@nx-go/nx-go` dengan **satu `go.mod` di root** | Plugin komunitas sudah stabil; satu module menghindari ribet `go.work` di awal. Fallback: target `nx:run-commands` biasa |
| Go versi | 1.27 (ikuti yang terpasang), pin di `go.mod` dan CI | Konsisten lokal vs CI |
| Arsitektur engine | **Modular monolith** + **Clean Architecture per modul** (lihat 4.1) | Satu binary mudah dioperasikan; batas modul tegas sejak awal sehingga bisa dipecah jadi service bila perlu |
| Frontend | React 19 + Vite + TypeScript strict, Tailwind v4 | Vite cepat, didukung `@nx/vite` resmi |
| Arsitektur frontend | **Clean Architecture per fitur** (domain → application → infrastructure → presentation, lihat 4.2) | Logika bisnis tidak bergantung pada React maupun transport |
| UI kit | **shadcn/ui** (Radix + Tailwind), komponen di-copy ke `libs/web/ui` | Komponen milik sendiri, bisa dikustom penuh, tidak terkunci versi library |
| State klien | **Zustand** (satu store per fitur, slice pattern) | Ringan, tanpa boilerplate, mudah diuji tanpa React |
| State server | **TanStack Query** | Cache, retry, invalidasi; memisahkan state server dari state klien |
| Routing | **TanStack Router** (file-based, type-safe) | Route params dan search params bertipe, loader terintegrasi Query |
| Tabel dan form | **TanStack Table** + **TanStack Form** + Zod | Satu ekosistem, headless, cocok dengan shadcn |
| Modeler BPMN | `bpmn-js` (bpmn.io) | Library yang sama dipakai Camunda Modeler, format XML 100% kompatibel |
| API engine | gRPC + REST via **Connect** (`connectrpc.com/connect`) dan `buf` | Satu definisi proto menghasilkan client Go dan TS; browser bisa panggil tanpa grpc-web proxy |
| Database | **PostgreSQL 18** (major stabil terbaru, rilis Sep 2025, didukung komunitas sampai Nov 2030) | PostgreSQL tidak punya label "LTS"; setiap major didukung 5 tahun. Versi 18 adalah yang terbaru dengan umur dukungan terpanjang dan sudah matang satu tahun. Transaksi kuat untuk state instance; `SKIP LOCKED` untuk job queue |
| Deployment | **Docker**: image `engine` (distroless) dan `web` (nginx static), dijalankan lewat `docker-compose.prod.yml` | Satu cara deploy untuk dev, staging, dan server; tidak butuh Kubernetes di awal |
| Pintasan perintah | **Makefile** (GNU Make, sudah terpasang) | Satu entry point untuk dev, CI, dan deploy; membungkus perintah Nx, Docker, buf |
| Migrasi DB | `goose` (SQL murni, embed ke binary) | Sederhana, dijalankan otomatis saat engine start atau lewat `make migrate-up` |
| Query DB | `pgx` v5 + `sqlc` | Type-safe tanpa ORM berat |
| Logging/metrics | `log/slog` + OpenTelemetry (otel-go) | Standar library, siap untuk tracing lintas worker |
| Lint Go | golangci-lint v2 (config di root) | Sudah terpasang |
| Lint/format TS | ESLint flat config + Prettier | Standar Nx |
| Test | `go test` + `testify`; Vitest; Playwright (e2e, Fase berikutnya) | — |
| Bahasa kode | **Identifier bahasa Inggris**: nama file, paket, fungsi, tipe, variabel, kolom DB, field proto, route. **Komentar dan dokumentasi boleh bahasa Indonesia** (komentar kode, ADR, docs) | Identifier Inggris selaras dengan library dan istilah BPMN/Camunda; komentar Indonesia memudahkan tim |
| Commit | Conventional Commits, **seluruh pesan bahasa Inggris**; ditegakkan `commitlint` + `husky`; **auto-commit dan push** oleh agen setiap unit kerja selesai (aturan lengkap di `CLAUDE.md` root) | Changelog otomatis; riwayat konsisten tanpa menunggu commit manual |
| CI | GitHub Actions dengan `nx affected` | Gratis untuk mulai; Nx Cloud opsional |
| Line ending | LF di semua file (`.gitattributes` + `.editorconfig`) | Hindari diff palsu di Windows |

### 4.1 Arsitektur core engine: modular monolith + clean architecture

Engine adalah **satu binary Go** yang tersusun dari modul-modul bisnis yang saling terisolasi.
Setiap modul memiliki empat lapisan Clean Architecture dan satu paket kontrak publik.

```
internal/modules/<modul>/
├─ api/             # KONTRAK PUBLIK modul: interface service, DTO, event. Satu-satunya paket yang boleh di-import modul lain
├─ domain/          # entity, value object, aturan bisnis, domain error, interface repository (port keluar). Tidak import apa pun selain stdlib
├─ application/     # use case (command/query handler), orkestrasi domain + port, transaksi. Import: domain
├─ infrastructure/  # adapter keluar: repository Postgres (pgx/sqlc), event bus, clock, id generator. Import: domain, application
└─ interfaces/      # adapter masuk: handler Connect/gRPC, HTTP, consumer event. Import: application, api
```

Modul awal yang direncanakan (dibuat bertahap di fase berikutnya; Fase 0 hanya modul `health`):

| Modul | Tanggung jawab |
|---|---|
| `deployment` | Deploy, versi, dan penyimpanan process definition (BPMN) |
| `execution` | Process instance, token, sequence flow, gateway, variable scope |
| `job` | Job activation, complete, fail, retry, backoff |
| `timer` | Penjadwalan timer start/intermediate/boundary |
| `message` | Korelasi message dan buffering |
| `usertask` | User task, assignment, form |
| `incident` | Pencatatan dan resolusi incident |

Aturan dependency (ditegakkan lint, bukan konvensi):

1. `domain` tidak meng-import lapisan lain, framework, maupun driver.
2. Arah import hanya ke dalam: `interfaces`/`infrastructure` → `application` → `domain`.
3. Modul lain hanya boleh meng-import `internal/modules/<x>/api`, tidak pernah `domain`/`application` milik modul lain.
4. Komunikasi antar modul: panggilan sinkron lewat interface di `api`, atau event in-process lewat
   `internal/platform/eventbus`. Tidak ada tabel database yang dibagi dua modul.
5. **Composition root** di `cmd/engine/main.go` yang merakit semua modul (dependency injection manual atau `wire`).
6. Kernel yang murni dan dapat dipakai ulang (parser BPMN, FEEL, SDK, stub API) tinggal di `libs/go/*`,
   bukan di dalam modul, agar bisa dipakai worker dan tooling lain.

### 4.2 Arsitektur frontend: clean architecture per fitur

Setiap aplikasi React disusun **per fitur** (bukan per jenis file), dan setiap fitur memiliki lapisan
yang sama seperti engine.

```
src/features/<fitur>/
├─ domain/          # tipe entity, value object, aturan bisnis murni (TypeScript tanpa React, tanpa fetch)
├─ application/     # use case: hook TanStack Query (query/mutation + key factory), store Zustand per fitur, orkestrasi
├─ infrastructure/  # adapter: pemanggilan Connect client, mapper DTO proto → domain, repository impl
└─ presentation/    # komponen React (shadcn/ui), TanStack Form/Table, tanpa logika bisnis
```

Aturan:

1. `domain` dan `application` tidak meng-import React component maupun `libs/web/api-client` langsung; akses data lewat interface repository yang diimplementasi di `infrastructure`.
2. `presentation` hanya memanggil hook dari `application`; tidak memanggil client API.
3. State server (data dari engine) **selalu** di TanStack Query; Zustand hanya untuk state klien (seleksi elemen di kanvas, panel terbuka, filter, draft form).
4. Routing: TanStack Router file-based di `src/routes/`; `loader` route memanggil `queryClient.ensureQueryData` dari `application`.
5. Komponen shadcn/ui tinggal di `libs/web/ui` sebagai kode milik repo; aplikasi tidak menginstal shadcn sendiri-sendiri.
6. Batas lintas lib ditegakkan `@nx/enforce-module-boundaries` dengan tag (`type:ui`, `type:api-client`, `type:core`, `scope:modeler`, …).

## 5. Rencana kerja Fase 0 (urutan eksekusi)

Setiap langkah punya **hasil yang bisa diverifikasi**. Kerjakan berurutan; 5.6 – 5.9 bisa paralel.

### 5.1 Lengkapi tooling lokal

```bash
# buf (protobuf toolchain)
go install github.com/bufbuild/buf/cmd/buf@latest
# generator Go + Connect
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest
# goose (migrasi) dan sqlc
go install github.com/pressly/goose/v3/cmd/goose@latest
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
# git: LF di repo, jangan dikonversi
git config --global core.autocrlf false
```

Verifikasi: `buf --version`, `protoc-gen-go --version`, `goose --version`, `sqlc version`.

Catatan Windows: `make` dijalankan dari Git Bash (sudah ada bersama Git for Windows) atau dari
PowerShell dengan GNU Make 4.4.1 yang sudah terpasang. Resep Makefile ditulis agar portabel (tanpa
`sed`/`xargs` khusus Linux; pekerjaan rumit dipindah ke skrip Go/Node di `tools/scripts`).

Pastikan `%USERPROFILE%\go\bin` ada di `PATH`.

### 5.2 Inisialisasi repositori git

- `git init -b main`
- `.gitignore` gabungan Node + Go + Nx (`.nx/cache`, `dist/`, `tmp/`, `node_modules/`, `*.exe`, `.env`).
- `.gitattributes`: `* text=auto eol=lf`, binari (`*.png`, `*.woff2`) ditandai `binary`.
- `.editorconfig`: indent 2 untuk TS/JSON/YAML, tab untuk Go (`[*.go] indent_style = tab`).
- Remote: `git remote add origin <url>` (GitHub, org pancaran-logistic) dan `git push -u origin main`
  pada commit pertama. Tanpa remote, aturan auto-push di `CLAUDE.md` tidak bisa berjalan.
- Strategi branch: `main` dilindungi setelah Fase 0 selesai; fitur lewat `feat/*` + PR, squash merge.
  Selama bootstrap Fase 0, commit langsung ke `main`.
- Konvensi commit: Conventional Commits, bahasa Inggris penuh, ditegakkan `commitlint`
  (`@commitlint/config-conventional` + daftar `scope-enum` sesuai area monorepo) lewat hook
  `commit-msg` husky. Aturan auto-commit/push untuk agen ada di `CLAUDE.md`.

Hasil: commit pertama `chore: init repository`, commit kedua `docs(plan): add phase 0 environment plan`.

### 5.3 Inisialisasi workspace Nx

```bash
cd D:\
pnpm dlx create-nx-workspace@latest workflow-engine --preset=ts --packageManager=pnpm --nxCloud=skip
cd workflow-engine
pnpm add -D @nx/react @nx/vite @nx/js @nx/eslint @nx-go/nx-go
pnpm nx g @nx-go/nx-go:init
```

> Catatan: `create-nx-workspace` menolak folder yang sudah berisi file. Karena `D:\workflow-engine`
> saat ini kosong, jalankan dari `D:\` atau buat di folder sementara lalu pindahkan isinya.
> Jika dokumen `docs/` ini sudah ada di folder, pindahkan dulu ke luar, generate, lalu kembalikan.

Konfigurasi `nx.json`:

- `namedInputs`: `default`, `production` (exclude test/spec), `goMod` (`go.mod`, `go.sum`).
- `targetDefaults`: `build` dan `test` depend on `^build`, cache aktif untuk `build|lint|test|generate`.
- `plugins`: `@nx/vite/plugin`, `@nx/eslint/plugin`, `@nx-go/nx-go`.

Hasil: `pnpm nx graph` menampilkan graph kosong tanpa error.

### 5.4 Tata letak folder monorepo

```
workflow-engine/
├─ apps/
│  ├─ engine/                       # Go: modular monolith, satu binary
│  │  ├─ cmd/engine/main.go         # composition root (wiring modul, server, migrasi)
│  │  └─ internal/
│  │     ├─ platform/               # infra bersama: config, logger, db pool, httpserver, eventbus, otel
│  │     └─ modules/
│  │        ├─ health/              # modul pertama (Fase 0) untuk membuktikan layering
│  │        │  ├─ api/  domain/  application/  infrastructure/  interfaces/
│  │        ├─ deployment/          # Fase 1+
│  │        ├─ execution/  job/  timer/  message/  usertask/  incident/   # fase berikutnya
│  ├─ worker-example/               # Go: contoh job worker memakai SDK (Fase 4)
│  ├─ modeler-web/                  # React: BPMN modeler
│  │  └─ src/
│  │     ├─ app/                    # providers: QueryClient, Router, Theme; composition root UI
│  │     ├─ routes/                 # TanStack Router file-based
│  │     ├─ features/
│  │     │  └─ health/              # fitur pertama (Fase 0): domain/ application/ infrastructure/ presentation/
│  │     └─ shared/                 # util lintas fitur milik app ini saja
│  ├─ operate-web/                  # React: monitoring instance (Fase 6, placeholder)
│  └─ tasklist-web/                 # React: user task UI (Fase 6, placeholder)
├─ libs/
│  ├─ go/
│  │  ├─ bpmn/                      # parser & model BPMN 2.0 (Fase 1), kernel murni
│  │  ├─ feel/                      # evaluator FEEL (Fase 7), kernel murni
│  │  ├─ api/                       # hasil generate proto (Go + Connect)
│  │  └─ sdk/                       # client/worker SDK Go (Fase 4)
│  ├─ web/
│  │  ├─ ui/                        # shadcn/ui components + tema (tag type:ui)
│  │  ├─ api-client/                # hasil generate proto (Connect-ES) + transport (tag type:api-client)
│  │  ├─ core/                      # tipe & util domain bersama, Result/Error (tag type:core)
│  │  └─ bpmn-viewer/               # pembungkus bpmn-js untuk React (tag type:ui)
│  └─ proto/                        # sumber kebenaran .proto (buf module)
├─ infra/
│  └─ docker/
│     ├─ docker-compose.yml         # dev: postgres (+ profil camunda, observability)
│     ├─ docker-compose.prod.yml    # deploy: engine + web + postgres
│     ├─ engine.Dockerfile
│     ├─ web.Dockerfile             # build Vite → nginx static
│     └─ nginx/default.conf
├─ tools/
│  └─ scripts/                      # skrip bantu (seed DB, gen, dsb.) — ditulis Go/Node, bukan bash
├─ docs/
│  ├─ adr/                          # Architecture Decision Records
│  └─ plan/                         # dokumen ini + fase berikutnya
├─ .github/workflows/ci.yml
├─ Makefile                         # SATU pintu masuk perintah untuk dev, CI, dan deploy
├─ go.mod  go.sum                   # satu module: github.com/<org>/workflow-engine
├─ nx.json  package.json  pnpm-workspace.yaml  tsconfig.base.json  components.json
├─ .golangci.yml  eslint.config.mjs  .prettierrc
└─ .env.example
```

Di Fase 0 yang dibuat: **`apps/engine`** (platform + modul `health`), **`apps/modeler-web`**
(fitur `health`), **`libs/proto`**, **`libs/go/api`**, **`libs/web/api-client`**, **`libs/web/ui`**,
**`libs/web/core`**, dan **`infra/docker`** lengkap. Sisanya cukup folder kosong dengan `README.md`
penanda. Migrasi dan akses DB (pgx/sqlc) tinggal di `internal/platform/db` dan `infrastructure`
tiap modul, bukan lib terpisah.

### 5.5 Scaffold aplikasi "hello" untuk memvalidasi pipeline

**Engine (Go)**

```bash
go mod init github.com/<org>/workflow-engine
pnpm nx g @nx-go/nx-go:application engine --directory=apps/engine
```

Isi minimal, sudah mengikuti struktur modular monolith di 4.1:

- `internal/platform/config`: baca env (`ENGINE_HTTP_ADDR`, `DATABASE_URL`) memakai `envconfig`/`koanf`.
- `internal/platform/logger`: `slog` JSON.
- `internal/platform/db`: pool `pgx` + runner migrasi `goose` (folder `migrations/`).
- `internal/platform/httpserver`: `net/http` mux + Connect handler mount, graceful shutdown (`signal.NotifyContext`).
- `internal/platform/eventbus`: interface publish/subscribe in-process (implementasi channel sederhana).
- `internal/modules/health`: modul pertama dan paling kecil, tetapi lengkap lima paketnya:
  - `domain`: `type Status struct`, aturan "ok hanya jika semua dependency ok".
  - `application`: use case `CheckHealth` dengan port `DependencyProbe`.
  - `infrastructure`: `PostgresProbe` memakai `db.Ping()`.
  - `interfaces`: handler Connect `HealthService.Check` + `GET /healthz` JSON.
  - `api`: interface `HealthChecker` untuk modul lain.
- `cmd/engine/main.go`: composition root yang merakit semuanya.

Target Nx yang harus ada dan lolos: `build`, `serve`, `test`, `lint`, `tidy`.

**Modeler Web (React)**

```bash
pnpm nx g @nx/react:application modeler-web --directory=apps/modeler-web --bundler=vite --style=css --routing=false --unitTestRunner=vitest --e2eTestRunner=none
pnpm add zustand @tanstack/react-query @tanstack/react-router @tanstack/react-table @tanstack/react-form zod
pnpm add -D @tanstack/router-plugin @tanstack/react-query-devtools @tanstack/router-devtools
pnpm add bpmn-js @connectrpc/connect @connectrpc/connect-web @bufbuild/protobuf
```

Generator `routing=false` karena routing memakai TanStack Router (plugin Vite `@tanstack/router-plugin`
untuk file-based routes di `src/routes/`).

**shadcn/ui di monorepo**

```bash
pnpm nx g @nx/react:library ui --directory=libs/web/ui --bundler=none --unitTestRunner=vitest
pnpm dlx shadcn@latest init        # pilih Tailwind v4, base color, CSS variables
pnpm dlx shadcn@latest add button card badge dialog table input form
```

- `components.json` di root dengan alias `@workflow/ui` → `libs/web/ui/src` (lewat `tsconfig.base.json` paths).
- CSS global + token tema di `libs/web/ui/src/styles/globals.css`; app meng-import file ini.
- Tailwind v4: `@source` di CSS app menunjuk ke `libs/web/**` agar class dari lib ikut terbundel.

Isi minimal `modeler-web`:

- `src/app/providers.tsx`: `QueryClientProvider`, `RouterProvider`, theme provider shadcn.
- `src/routes/__root.tsx` + `src/routes/index.tsx`.
- Fitur `health` lengkap empat lapisan: `domain/health.ts`, `application/use-health.ts` (TanStack Query)
  + `application/health.store.ts` (Zustand untuk state klien, mis. auto-refresh on/off),
  `infrastructure/health.repository.ts` (Connect client → domain), `presentation/health-card.tsx` (shadcn `Card`, `Badge`).
- Kanvas `bpmn-js` kosong untuk membuktikan library bisa dibundel.

Target Nx yang harus lolos: `build`, `serve`, `test`, `lint`.

### 5.6 Docker: infrastruktur lokal dan deployment

**Dev — `infra/docker/docker-compose.yml`** (hanya dependensi; engine dan web jalan native lewat Nx):

| Service | Image | Port | Catatan |
|---|---|---|---|
| `postgres` | `postgres:18-alpine` | 5432 | volume persisten, healthcheck `pg_isready` |
| `pgweb` (profil `tools`) | `sosedoff/pgweb` | 8081 | UI query ringan |
| `otel-collector` (profil `observability`) | `otel/opentelemetry-collector-contrib` | 4317 | siapkan dari awal agar tracing tidak jadi utang |

Profil `camunda` (opsional): Zeebe, Operate, Tasklist, Elasticsearch dari `docker-compose` resmi
Camunda 8 self-managed. Dipakai hanya untuk uji paritas perilaku; butuh RAM ≥ 8 GB untuk Docker.

**Image aplikasi**

- `infra/docker/engine.Dockerfile`: multi-stage, `golang:1.27` → `gcr.io/distroless/static`,
  `CGO_ENABLED=0`, `-ldflags "-s -w -X main.version=$(VERSION)"`, binary menjalankan migrasi
  saat start (`ENGINE_MIGRATE=true`) lalu serve. User non-root, `HEALTHCHECK` ke `/healthz`.
- `infra/docker/web.Dockerfile`: multi-stage, `node:22-alpine` (`pnpm nx build modeler-web`) →
  `nginx:alpine` menyajikan `dist/` dengan `nginx/default.conf` (SPA fallback, proxy `/api` ke engine).
- Keduanya memakai `.dockerignore` di root (`node_modules`, `.nx`, `dist`, `.git`).
- Build memanfaatkan cache: `--mount=type=cache` untuk `GOMODCACHE` dan pnpm store.

**Deploy — `infra/docker/docker-compose.prod.yml`**

| Service | Image | Catatan |
|---|---|---|
| `postgres` | `postgres:18-alpine` | volume bernama, backup lewat `pg_dump` cron di host |
| `engine` | `ghcr.io/<org>/workflow-engine/engine:<tag>` | env dari `.env.prod`, `depends_on` postgres healthy, restart `unless-stopped` |
| `web` | `ghcr.io/<org>/workflow-engine/web:<tag>` | port 80/443; TLS lewat reverse proxy di depan (Caddy/Traefik) bila dibutuhkan |

Tag image mengikuti git tag (`v0.1.0`) atau SHA pendek untuk build non-rilis.

Lengkapi `.env.example` (di-commit), `.env` dan `.env.prod` (di-ignore). `DATABASE_URL` default
menunjuk ke compose dev.

Hasil: `make up` lalu `make dev` → `curl localhost:8080/healthz` mengembalikan
`{"status":"ok","db":"ok"}`; `make docker-build` menghasilkan dua image yang bisa dijalankan dengan
`make deploy-local` (compose prod di mesin sendiri).

### 5.7 Toolchain protobuf / API contract

- `libs/proto/buf.yaml` (module `buf.build/<org>/workflow-engine`, lint `STANDARD`, breaking `FILE`).
- `libs/proto/buf.gen.yaml` dengan dua target:
  - Go → `libs/go/api` (`protoc-gen-go`, `protoc-gen-connect-go`)
  - TS → `libs/web/api-client/src/gen` (`@bufbuild/protoc-gen-es`)
- Proto contoh pertama: `health/v1/health.proto` dengan RPC `Check`.
- Target Nx `proto:generate` + `proto:lint` + `proto:breaking`; `build` engine dan web `dependsOn`
  `proto:generate`.
- Hasil generate **di-commit** (agar `pnpm install` saja sudah cukup) dan CI memverifikasi tidak ada
  diff setelah regenerate.

### 5.8 Kualitas kode dan hook

- `.golangci.yml` v2: enable `govet, staticcheck, errcheck, revive, gocritic, gosec, misspell`,
  timeout 5m.
- **Penegakan batas arsitektur Go** lewat `depguard` di golangci-lint:
  - paket `**/domain/**` dilarang import `pgx`, `connect`, `net/http`, dan paket `application|infrastructure|interfaces` mana pun;
  - paket `**/application/**` dilarang import `infrastructure|interfaces`;
  - paket `internal/modules/<a>/**` dilarang import `internal/modules/<b>/(domain|application|infrastructure|interfaces)`, hanya boleh `<b>/api`.
  Ditambah `go-arch-lint` (`.go-arch-lint.yml`) sebagai pemeriksa kedua yang lebih ekspresif.
- `eslint.config.mjs` dari Nx + `@typescript-eslint` strict + `eslint-plugin-react-hooks`.
- **Penegakan batas arsitektur TS**:
  - `@nx/enforce-module-boundaries` dengan tag di `project.json`: `type:ui` hanya boleh depend `type:core`;
    `type:api-client` hanya `type:core`; app boleh semua.
  - `eslint-plugin-boundaries` di dalam app untuk lapisan fitur: `domain` → tidak import apa pun dari
    `application|infrastructure|presentation` maupun `react`; `application` → hanya `domain`;
    `presentation` → `application` dan `domain`, dilarang `infrastructure` dan `@workflow/api-client`.
- Keduanya dibuktikan di Fase 0 dengan satu commit sengaja melanggar yang harus gagal lint (lalu di-revert).
- `.prettierrc` (singleQuote, printWidth 100).
- `husky` + `lint-staged`: `gofmt -l` / `goimports` untuk `*.go`, `eslint --fix` + `prettier` untuk
  `*.{ts,tsx,json,md}`.
- `commitlint.config.mjs`: extends `@commitlint/config-conventional`, `scope-enum` berisi
  `engine, modeler-web, operate-web, tasklist-web, ui, api-client, core, proto, bpmn, feel, sdk, infra,
  docker, ci, nx, deps, docs, plan` plus pola `engine/<modul>`; `subject-case` lowercase;
  `header-max-length` 72. Hook `commit-msg` menjalankan `commitlint --edit`.
- Target Nx `format:check` di CI; CI juga menjalankan `commitlint --from origin/main` pada PR.

### 5.9 CI (GitHub Actions)

`.github/workflows/ci.yml`:

1. Checkout dengan `fetch-depth: 0` (dibutuhkan `nx affected`).
2. `pnpm/action-setup`, `actions/setup-node` (cache pnpm), `actions/setup-go` (versi dari `go.mod`, cache module).
3. Install `buf` dan plugin protoc; jalankan `proto:generate` lalu `git diff --exit-code`.
4. `make check` (membungkus `pnpm nx affected -t lint test build --parallel=3`).
5. Job `docker` : `make docker-build` untuk image `engine` dan `web`; pada push tag `v*` dilanjutkan
   `make docker-push` ke GHCR memakai `GITHUB_TOKEN`.
6. Job `integration` yang menyalakan service container `postgres:18-alpine` dan menjalankan
   `make test-integration` (test Go bertag `integration`).

Matrix OS: `ubuntu-latest` saja dulu; Windows runner ditambah bila ada bug spesifik Windows.

### 5.10 Dokumentasi awal

- `README.md`: tujuan proyek, prasyarat, "mulai dalam 5 menit", peta folder.
- `CONTRIBUTING.md`: alur branch/PR, konvensi commit, cara menambah lib/app lewat generator Nx,
  dan **konvensi bahasa**: identifier (file, paket, fungsi, tipe, variabel, kolom, field proto) wajib
  bahasa Inggris; komentar kode, docstring, ADR, dan dokumen boleh bahasa Indonesia. Contoh:

  ```go
  // ActivateJobs mengambil job yang siap dikerjakan untuk tipe tertentu,
  // mengunci barisnya dengan SKIP LOCKED agar tidak diambil worker lain.
  func (s *JobService) ActivateJobs(ctx context.Context, req ActivateJobsRequest) ([]Job, error)
  ```
- `docs/adr/`:
  - ADR-0001 Monorepo dengan Nx + pnpm
  - ADR-0002 Engine ditulis di Go, satu go.mod
  - ADR-0003 API kontrak protobuf + Connect
  - ADR-0004 PostgreSQL 18 sebagai penyimpanan state engine
  - ADR-0005 Engine sebagai modular monolith dengan Clean Architecture per modul
  - ADR-0006 Frontend: Clean Architecture per fitur dengan shadcn/ui, Zustand, TanStack
  - ADR-0007 Deployment lewat image Docker dan Docker Compose, Makefile sebagai entry point
- `docs/plan/`: dokumen ini + `README.md` roadmap.

### 5.11 Pengalaman developer (DX)

- `.vscode/extensions.json`: `golang.go`, `nrwl.angular-console` (Nx Console), `esbenp.prettier-vscode`,
  `dbaeumer.vscode-eslint`, `bufbuild.vscode-buf`, `bpmn-io.vs-code-bpmn-io`.
- `.vscode/settings.json`: format on save, `go.lintTool=golangci-lint`, `eslint.useFlatConfig`.
- **`Makefile`** di root sebagai satu pintu masuk. Target yang harus ada di Fase 0:

  | Target | Fungsi |
  |---|---|
  | `make help` | daftar target (default) |
  | `make setup` | `pnpm install`, `go mod download`, install tool Go (`buf`, `goose`, `sqlc`, plugin protoc), `husky` |
  | `make up` / `make down` | compose dev naik/turun (`--profile camunda` lewat `PROFILES=camunda`) |
  | `make dev` | `nx run-many -t serve -p engine,modeler-web` |
  | `make gen` | `buf generate` + `sqlc generate` |
  | `make lint` / `make test` / `make build` | `nx run-many` untuk semua proyek |
  | `make check` | `nx affected -t lint test build` (dipakai CI) |
  | `make test-integration` | `go test -tags=integration ./...` dengan `DATABASE_URL` dari env |
  | `make migrate-up` / `make migrate-down` / `make migrate-new name=…` | goose |
  | `make docker-build` | build image `engine` dan `web`, tag dari `VERSION ?= $(git describe --tags --always)` |
  | `make docker-push` | push ke registry (`REGISTRY ?= ghcr.io/<org>/workflow-engine`) |
  | `make deploy-local` | `docker compose -f infra/docker/docker-compose.prod.yml up -d` |
  | `make clean` | hapus `dist/`, `.nx/cache`, binary |

  Variabel di atas resep: `VERSION`, `REGISTRY`, `PROFILES`, `DATABASE_URL`. Semua target `.PHONY`.
- Dokumentasikan opsi WSL2 untuk engineer yang lebih nyaman di Linux; repo harus tetap jalan
  native di Windows (Makefile diuji di Git Bash dan PowerShell).

## 6. Risiko dan mitigasi

| Risiko | Dampak | Mitigasi |
|---|---|---|
| Plugin `@nx-go/nx-go` tidak cocok dengan Nx terbaru | build Go tidak masuk graph Nx | Fallback: definisikan target `go build/test/lint` via `nx:run-commands` di `project.json`; tetap dapat affected + cache |
| Masalah path/CRLF di Windows | diff palsu, skrip bash gagal | `.gitattributes` LF, resep Makefile portabel, logika rumit di skrip Node/Go (bukan bash murni) |
| shadcn CLI mengasumsikan satu app, bukan monorepo | komponen ter-generate di tempat salah | `components.json` di root dengan alias ke `libs/web/ui`; verifikasi dengan `shadcn add button` sebelum menambah komponen lain |
| Lapisan clean architecture terasa berlebihan untuk modul kecil | engineer melewati aturan | Modul `health` di Fase 0 menjadi contoh rujukan; generator Nx custom (`tools/generators/module`) untuk membuat kerangka modul/fitur agar biaya mengikuti pola mendekati nol |
| Stack Camunda referensi berat (Elasticsearch) | laptop lambat | Jadikan profil opsional; jalankan hanya saat uji paritas |
| Proto hasil generate tidak sinkron | build patah di CI | CI memeriksa `git diff` setelah generate |
| Versi Go lokal vs CI berbeda | perilaku berbeda | `setup-go` membaca `go-version-file: go.mod`, `toolchain` directive di go.mod |
| Nx cache menyimpan artefak Go yang platform-specific | cache hit salah antar OS | tambahkan `{os}` / `runtime("go version")` ke `namedInputs` target Go |

## 7. Estimasi dan pembagian kerja

Perkiraan **3–5 hari kerja** untuk 1 engineer, atau 2–3 hari untuk 2 engineer paralel:

| Hari | Fokus |
|---|---|
| 1 | 5.1 – 5.4 (tooling, git, Nx, struktur) |
| 2 | 5.5 (engine modul `health`, modeler-web fitur `health`, shadcn/TanStack/Zustand) |
| 3 | 5.6 (compose dev, Dockerfile, compose prod) dan 5.7 (proto) |
| 4 | 5.8 (lint + penegakan batas arsitektur) dan 5.9 (CI + docker build) |
| 5 | 5.10 (docs/ADR), 5.11 (Makefile, DX), generator modul/fitur, review DoD |

Dengan tambahan lapisan arsitektur dan deployment Docker, estimasi realistis menjadi **5–6 hari kerja**
untuk 1 engineer, atau 3–4 hari untuk 2 engineer.

Pembagian jika 2 orang: **A** = Go/infra/proto/CI, **B** = Nx/React/lint/docs.

## 8. Keluaran Fase 0

Setelah fase ini selesai, Fase 1 (parser BPMN) bisa langsung dimulai dengan menambahkan
`libs/go/bpmn` lewat generator Nx tanpa menyentuh infrastruktur lagi.
