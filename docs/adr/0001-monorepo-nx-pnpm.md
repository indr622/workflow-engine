# ADR-0001: Monorepo dengan Nx dan pnpm

- **Status:** Diterima
- **Tanggal:** 2026-10-28

## Konteks

Proyek terdiri dari engine Go, beberapa aplikasi React, lib bersama, dan kontrak proto yang
menghasilkan kode untuk dua bahasa. Perubahan kontrak harus terlihat dampaknya di kedua sisi dalam
satu PR.

## Keputusan

- Satu repositori dengan **Nx 24** sebagai orkestrator dan **pnpm** sebagai package manager.
- Proyek Go didaftarkan ke Nx lewat target `nx:run-commands` di `project.json`, bukan plugin
  komunitas, agar tidak bergantung pada kompatibilitas plugin dengan versi Nx terbaru.
- CI memakai `nx affected` sehingga hanya proyek yang terdampak yang diperiksa.
- Daemon Nx dimatikan (`useDaemonProcess: false`) karena macet di lingkungan Windows tim.

## Konsekuensi

- Graf dependensi dan cache Nx berlaku untuk Go dan TypeScript.
- Target Go perlu ditulis manual per proyek. Ini sengaja: perintahnya sederhana dan terlihat jelas.
- Tanpa daemon, setiap perintah Nx menghitung graf proyek dari awal, beberapa detik lebih lambat.
