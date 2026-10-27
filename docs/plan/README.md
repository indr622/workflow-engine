# Roadmap Workflow Engine

Engine workflow BPMN 2.0 yang dibangun dari nol dengan fitur-fitur setara Camunda 8.
Inti ditulis di Go sebagai modular monolith dengan Clean Architecture per modul; UI di React
(shadcn/ui, Zustand, TanStack, bpmn-js untuk BPMN, React Flow untuk graf lain) dengan Clean
Architecture per fitur; monorepo Nx; deploy lewat Docker.
Setiap fase punya dokumen sendiri di folder ini. Fase 0 dan Fase 1 sudah dirinci.

| Fase | Nama | Keluaran utama | Dokumen |
|---|---|---|---|
| 0 | Persiapan environment (✅ selesai 2026-10-28) | Monorepo Nx + Go + React jalan, Compose, proto toolchain, CI hijau | [00-fase-0-persiapan-environment.md](00-fase-0-persiapan-environment.md) |
| 1 | Parser dan model BPMN (✅ selesai 2026-10-28, menunggu review) | `libs/go/bpmn`: parse XML BPMN 2.0 ke model in-memory, validasi elemen yang didukung | [01-fase-1-parser-model-bpmn.md](01-fase-1-parser-model-bpmn.md) |
| 2 | Engine inti | modul `execution`: eksekusi token, sequence flow, gateway (exclusive, parallel, inclusive), start/end event, service task sebagai job | _belum ditulis_ |
| 3 | Persistensi dan pemulihan | State instance di PostgreSQL, event log, replay setelah restart, idempotensi | _belum ditulis_ |
| 4 | API dan job worker | gRPC/Connect: deploy, create instance, activate jobs, complete/fail job, publish message; SDK worker Go | _belum ditulis_ |
| 5 | Timer, message, boundary event, sub-process | Scheduler timer, korelasi message, error dan escalation event | _belum ditulis_ |
| 6 | UI: Modeler, Operate, Tasklist | React: modeler bpmn-js dengan properties panel, monitoring instance (viewer bpmn-js + pohon instance React Flow), user task inbox | _belum ditulis_ |
| 7 | FEEL dan DMN | Evaluator FEEL untuk kondisi dan input/output mapping; DMN decision table (opsional) | _belum ditulis_ |
| 8 | Observability, keamanan, hardening deploy | OpenTelemetry, auth (OIDC), multi-tenant dasar, compose prod dengan reverse proxy/TLS, backup DB, zero-downtime restart | _belum ditulis_ |

Prinsip lintas fase:

- Setiap modul engine baru lahir lewat generator dengan lima paket (`api`, `domain`, `application`, `infrastructure`, `interfaces`); setiap fitur UI baru lahir dengan empat lapisan. Batasnya ditegakkan lint.
- Deployment selalu berupa image Docker yang sama dari dev sampai produksi; `Makefile` adalah satu-satunya pintu masuk perintah.

- Format BPMN 100% kompatibel dengan Camunda Modeler, termasuk namespace `zeebe:` untuk task definition dan I/O mapping.
- Kontrak API selalu lahir dari `.proto` di `libs/proto`; tidak ada endpoint yang ditulis tangan.
- Setiap fase ditutup dengan uji paritas terhadap Camunda 8 referensi (profil `camunda` di Compose) untuk skenario yang relevan.
