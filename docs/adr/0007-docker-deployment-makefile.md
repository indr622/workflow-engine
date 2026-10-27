# ADR-0007: Deployment lewat image Docker dan Docker Compose, Makefile sebagai entry point

- **Status:** Diterima
- **Tanggal:** 2026-10-28

## Konteks

Tim butuh satu cara deploy yang sama untuk dev, staging, dan server, tanpa Kubernetes di awal.
Perintah dev, CI, dan deploy harus konsisten di Windows dan Linux.

## Keputusan

- Dua image:
  - **engine**: build statis Go di `gcr.io/distroless/static-debian12:nonroot`. Subperintah
    `engine healthcheck` dipakai `HEALTHCHECK` karena distroless tidak punya curl.
  - **web**: build Nx modeler-web disajikan **nginx**, dengan fallback SPA dan proxy `/api` ke engine.
- `infra/docker/docker-compose.yml` untuk dependensi dev, `docker-compose.prod.yml` untuk deploy.
- Image di-push ke GHCR oleh CI saat ada tag `v*`. Tag image mengikuti `git describe`.
- **Makefile** di root adalah satu-satunya pintu masuk perintah. Versi tool Go dipin di Makefile.
  Di Windows, resep dijalankan dengan `sh` milik Git for Windows.

## Konsekuensi

- Image yang sama berjalan dari laptop sampai server.
- Engine menjalankan migrasi sendiri saat start, sehingga deploy cukup `docker compose up -d`.
- TLS dan reverse proxy di depan web belum disiapkan. Itu bagian dari Fase 8.
