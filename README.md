# Workflow Engine

Engine workflow BPMN 2.0 yang dibangun dari nol dengan fitur setara Camunda 8.
Inti engine ditulis di Go, UI di React, dalam satu monorepo Nx.

| Bagian | Teknologi |
|---|---|
| Engine | Go 1.27, modular monolith, Clean Architecture per modul, Connect RPC, PostgreSQL 18 |
| UI | React 19, Vite, shadcn/ui, Tailwind v4, Zustand, TanStack Query/Router/Table/Form, bpmn-js, React Flow |
| Kontrak API | Protobuf + buf, stub Go dan TypeScript di-generate |
| Monorepo | Nx 24 + pnpm |
| Deployment | Image Docker + Docker Compose |

Rencana dan status tiap fase ada di [docs/plan](docs/plan/README.md). Keputusan arsitektur ada di
[docs/adr](docs/adr).

## Prasyarat

- Go 1.27
- Node.js 22 dan pnpm 12 (lewat corepack)
- Docker Desktop
- Git for Windows (di Windows, Makefile memakai `sh` miliknya)

## Mulai dalam 5 menit

```bash
make setup     # dependensi Node dan Go, tool Go, git hooks
make up        # PostgreSQL 18 di localhost:5433
make dev       # engine di :8080, modeler-web di http://localhost:4200
```

Cek engine:

```bash
curl localhost:8080/healthz
```

Perintah lain: `make help`.

## Peta folder

```
apps/
  engine/          Go: modular monolith (cmd/engine = composition root, internal/modules/*)
  modeler-web/     React: modeler BPMN dan dashboard
  operate-web/     React: monitoring instance (Fase 6)
  tasklist-web/    React: user task (Fase 6)
  worker-example/  Go: contoh job worker (Fase 4)
libs/
  proto/           Sumber kebenaran kontrak API (.proto)
  go/api/          Stub Go hasil generate
  go/bpmn/         Parser dan model BPMN (Fase 1)
  web/core/        Tipe domain bersama (Result, AppError)
  web/api-client/  Stub TypeScript hasil generate + transport Connect
  web/ui/          Komponen shadcn/ui dan tema
  web/flow-canvas/ React Flow dengan tema shadcn dan auto-layout
  web/bpmn-viewer/ bpmn-js untuk React
infra/docker/      Compose dev dan produksi, Dockerfile engine dan web
docs/              Rencana fase dan ADR
```

## Deployment

```bash
make docker-build                 # image engine dan web, tag dari git describe
make deploy-local                 # compose produksi di mesin ini, web di http://localhost:8088
```

Di server, isi `.env.prod` (minimal `POSTGRES_PASSWORD`, `VERSION`) lalu jalankan:

```bash
docker compose -f infra/docker/docker-compose.prod.yml --env-file .env.prod up -d
```

## Kontribusi

Lihat [CONTRIBUTING.md](CONTRIBUTING.md).
