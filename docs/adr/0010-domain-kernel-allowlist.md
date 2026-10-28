# ADR-0010: Allowlist kernel murni untuk lapisan domain

- **Status:** Diterima
- **Tanggal:** 2026-10-28

## Konteks

Aturan ADR-0005 membatasi lapisan `domain` setiap modul engine hanya pada standard library. Modul
`execution` (Fase 2) menjalankan token di atas model BPMN dan mengevaluasi kondisi. Perilaku elemen
adalah aturan bisnis inti, jadi tempatnya di `domain`. Tanpa akses ke `libs/go/bpmn`, domain harus
menduplikasi model proses atau logika eksekusi terpaksa pindah ke `application`.

`libs/go/bpmn` dan `libs/go/feel` sudah berupa kernel murni: hanya stdlib (ditegakkan
`deps_test.go` masing-masing), immutable setelah dibangun, tanpa I/O.

## Keputusan

- `apps/engine/internal/archtest` memiliki `kernelAllowlist` berisi tepat
  `github.com/indr622/workflow-engine/libs/go/bpmn` dan `.../libs/go/feel`. Lapisan `domain` boleh
  meng-import paket di allowlist beserta subpaketnya, kecuali paket `internal`.
- `TestKernelsArePure` menjalankan `go list -deps` untuk setiap kernel dan gagal bila ada
  dependensi di luar stdlib, sehingga allowlist tidak bisa dipakai menyelundupkan dependensi.
- depguard `domain-purity` menambah larangan `libs/go/sdk`.
- Menambah kernel baru ke allowlist membutuhkan ADR baru.

## Konsekuensi

- `execution/domain` bekerja langsung di atas `*bpmn.Process` tanpa graf tiruan.
- Perubahan API kernel kini berdampak ke domain modul, jadi kernel diperlakukan sebagai API stabil.
- Modul `deployment` boleh memindahkan aturan yang bergantung pada BPMN ke domain-nya kelak, tetapi
  tidak wajib.
