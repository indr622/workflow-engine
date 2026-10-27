# ADR-0003: Kontrak API dengan Protobuf dan Connect

- **Status:** Diterima
- **Tanggal:** 2026-10-28

## Konteks

Engine dipanggil oleh UI di browser, job worker, dan klien lain. Kontrak harus bertipe di Go dan
TypeScript, dan browser harus bisa memanggil tanpa proxy tambahan.

## Keputusan

- Semua API lahir dari `.proto` di `libs/proto`, dikelola dengan **buf** (lint `STANDARD`,
  breaking `FILE`).
- Server dan klien memakai **Connect** (`connectrpc.com/connect` di Go, `@connectrpc/connect-web`
  di browser). Satu handler melayani protokol Connect, gRPC, dan gRPC-Web.
- Stub hasil generate di-commit. CI memeriksa stub tetap sinkron dengan `.proto`.
- Endpoint REST yang ditulis tangan hanya untuk kebutuhan operasional, misalnya `/healthz`.

## Konsekuensi

- Browser memanggil engine dengan JSON lewat HTTP biasa, tanpa grpc-web proxy.
- Perubahan kontrak terlihat di diff Go dan TypeScript sekaligus.
- Clone baru bisa build tanpa menjalankan `buf generate`.
