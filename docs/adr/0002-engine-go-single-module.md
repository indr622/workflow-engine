# ADR-0002: Engine ditulis di Go dengan satu go.mod

- **Status:** Diterima
- **Tanggal:** 2026-10-28

## Konteks

Engine workflow butuh konkurensi tinggi untuk eksekusi token, job, dan timer, serta binary yang
mudah di-deploy. Kode Go tersebar di aplikasi (`apps/engine`, `apps/worker-example`) dan lib
(`libs/go/*`).

## Keputusan

- Engine ditulis di **Go 1.27**.
- **Satu `go.mod` di root** dengan module `github.com/indr622/workflow-engine`. Tidak memakai
  `go.work` di tahap awal.
- Kernel yang murni dan dipakai ulang (parser BPMN, FEEL, SDK, stub API) tinggal di `libs/go/*`.

## Konsekuensi

- Satu set versi dependensi untuk seluruh kode Go. Tidak ada drift versi antar paket.
- Semua paket Go bisa saling impor dengan path module yang sama. Batas yang tidak boleh dilanggar
  ditegakkan lint (lihat ADR-0005).
- Jika kelak SDK perlu dirilis terpisah, `libs/go/sdk` dipindah ke module sendiri dengan `go.work`.
