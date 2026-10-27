# ADR-0006: Frontend dengan Clean Architecture per fitur, shadcn/ui, Zustand, dan TanStack

- **Status:** Diterima
- **Tanggal:** 2026-10-28

## Konteks

Tiga aplikasi React (modeler, operate, tasklist) berbagi komponen dan pola. Logika bisnis UI harus
bisa diuji tanpa React dan tanpa jaringan.

## Keputusan

- **Struktur per fitur** dengan empat lapisan: `domain`, `application`, `infrastructure`,
  `presentation`.
- **shadcn/ui** sebagai kode milik repo di `libs/web/ui`, dengan tema Tailwind v4 bersama.
- **TanStack Query** untuk state dari engine, **Zustand** untuk state klien.
- **TanStack Router** file-based. Loader route melakukan prefetch lewat query yang sama dengan
  komponen.
- **TanStack Table** dan **TanStack Form** dengan Zod untuk tabel dan form.
- **bpmn-js** di `libs/web/bpmn-viewer` adalah satu-satunya editor BPMN XML. **React Flow** di
  `libs/web/flow-canvas` untuk graf non-BPMN. Graf React Flow tidak pernah dikirim ke engine.
- Port repository didefinisikan di `domain` fitur dan diimplementasikan di `infrastructure`.
  Implementasi dirakit di `src/app/dependencies.ts`.
- Penegakan:
  - Tag Nx (`type:app`, `type:ui`, `type:api-client`, `type:core`) dengan
    `@nx/enforce-module-boundaries`.
  - Aturan `no-restricted-imports` per lapisan di konfigurasi ESLint aplikasi.

## Konsekuensi

- Domain dan mapper bisa diuji dengan Vitest murni.
- Komponen shadcn bisa dikustom penuh tanpa terkunci versi library.
- Plugin `eslint-plugin-boundaries` tidak dipakai. Aturan bawaan ESLint sudah cukup dan lebih stabil
  antar versi.
