# Panduan Kontribusi

## Alur kerja

1. Buat branch dari `main`: `feat/<ringkasan>`, `fix/<ringkasan>`, atau `chore/<ringkasan>`.
2. Commit kecil per perubahan logis. Hook pre-commit menjalankan gofmt dan Prettier pada file yang
   di-stage. Hook commit-msg menjalankan commitlint.
3. Sebelum push, jalankan `make check`.
4. Buka PR ke `main`. CI harus hijau. PR digabung dengan squash merge, judul PR mengikuti format commit.

Jangan memakai `--no-verify` dan jangan force push ke branch yang sudah dibagikan.

## Konvensi bahasa

- **Identifier wajib bahasa Inggris**: nama file, paket, fungsi, tipe, variabel, kolom database,
  field proto, dan route.
- **Komentar kode, docstring, ADR, dan dokumen boleh bahasa Indonesia.**
- **Pesan commit seluruhnya bahasa Inggris.**

```go
// ActivateJobs mengambil job yang siap dikerjakan untuk tipe tertentu,
// mengunci barisnya dengan SKIP LOCKED agar tidak diambil worker lain.
func (s *JobService) ActivateJobs(ctx context.Context, req ActivateJobsRequest) ([]Job, error)
```

## Format pesan commit

Conventional Commits:

```
<type>(<scope>): <subject>

<body: mengapa, dibungkus di 100 kolom>
```

- `type`: `feat`, `fix`, `refactor`, `perf`, `test`, `docs`, `build`, `ci`, `chore`, `style`, `revert`.
- `scope`: daftar resmi ada di [commitlint.config.mjs](commitlint.config.mjs), misalnya `engine`,
  `engine/job`, `modeler-web`, `ui`, `proto`, `docker`.
- `subject`: imperatif, huruf kecil di awal, tanpa titik, maksimal 72 karakter.

## Arsitektur engine (Go)

Setiap modul di `apps/engine/internal/modules/<modul>` punya lima paket:

| Paket | Isi | Boleh meng-import |
|---|---|---|
| `domain` | Entity, aturan bisnis, port repository | Hanya stdlib |
| `application` | Use case | `domain`, `api` (event yang dipublikasikan, interface yang diimplementasikan) |
| `infrastructure` | Adapter keluar: Postgres, event bus | `domain`, `application`, `api` |
| `interfaces` | Adapter masuk: handler Connect dan HTTP | `domain`, `application`, `api` |
| `api` | Kontrak publik untuk modul lain | Tidak ada paket modul lain; boleh kernel `libs/go/*` |

Contoh rujukan dua modul: `health` (paling kecil) dan `deployment` (dengan repository Postgres,
sqlc, cache, dan event).

Modul lain hanya boleh meng-import paket `api`. Aturan ini ditegakkan oleh depguard dan test
`apps/engine/internal/archtest`. Modul baru dirakit di `apps/engine/cmd/engine/main.go`.

Contoh rujukan: modul `health`.

## Arsitektur UI (React)

Setiap fitur di `apps/<app>/src/features/<fitur>` punya empat lapisan:

| Lapisan | Isi | Larangan |
|---|---|---|
| `domain` | Tipe, aturan bisnis murni, port repository | React, TanStack, Zustand, transport |
| `application` | Hook TanStack Query, store Zustand, provider port | `infrastructure`, `presentation`, transport |
| `infrastructure` | Implementasi repository lewat Connect, mapper DTO | — |
| `presentation` | Komponen React dengan shadcn/ui | `infrastructure`, transport |

- State dari engine selalu di TanStack Query. Zustand hanya untuk state klien.
- Route hanya memakai API publik fitur (`features/<fitur>/index.ts`).
- Implementasi repository dirakit di `src/app/dependencies.ts`.
- Batas antar-lib ditegakkan tag Nx: `type:app`, `type:ui`, `type:api-client`, `type:core`.

Contoh rujukan: fitur `health` di modeler-web.

## Menambah komponen shadcn/ui

Komponen tinggal di `libs/web/ui/src/components/ui` dan diekspor dari `libs/web/ui/src/index.ts`.
Konfigurasi alias ada di `components.json` di root.

## Kontrak API

1. Ubah atau tambah `.proto` di `libs/proto/workflow/<area>/v1`.
2. Jalankan `make gen`. Hasil generate di-commit.
3. `pnpm nx run proto:breaking` memeriksa perubahan yang merusak kompatibilitas.

## Migrasi database

```bash
make migrate-new name=add_jobs
```

File SQL dibuat di `apps/engine/internal/platform/db/migrations` dan dijalankan otomatis saat engine
start.
