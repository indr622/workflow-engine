# ADR-0005: Engine sebagai modular monolith dengan Clean Architecture per modul

- **Status:** Diterima
- **Tanggal:** 2026-10-28

## Konteks

Engine punya banyak area bisnis (deployment, execution, job, timer, message, user task, incident).
Microservice sejak awal menambah beban operasi. Monolith tanpa batas cepat menjadi kusut.

## Keputusan

- Engine adalah **satu binary** yang terdiri dari modul di `apps/engine/internal/modules/<modul>`.
- Setiap modul punya lima paket: `api`, `domain`, `application`, `infrastructure`, `interfaces`.
- Arah dependensi hanya ke dalam. `domain` hanya boleh stdlib.
- Modul lain hanya boleh meng-import paket `api`. Komunikasi antar modul lewat interface di `api`
  atau event bus in-process.
- Semua modul dirakit di composition root `apps/engine/cmd/engine/main.go`.
- Aturan ditegakkan dua lapis:
  - **depguard** di golangci-lint melarang `domain` meng-import driver, transport, dan platform.
  - **Test arsitektur** `apps/engine/internal/archtest` membaca graf impor `go list` dan gagal jika
    ada lapisan atau modul yang melanggar.

## Konsekuensi

- Satu modul bisa dipecah menjadi service sendiri kelak tanpa membongkar modul lain.
- Modul kecil terasa punya banyak paket. Modul `health` menjadi contoh rujukan untuk mengurangi biaya ini.
- Kernel murni seperti `libs/go/bpmn` belum boleh diimpor dari `domain`. Keputusan allowlist
  ditinjau di awal Fase 2.
