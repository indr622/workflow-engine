# Roadmap Workflow Engine

Engine workflow BPMN 2.0 yang dibangun dari nol dengan fitur-fitur setara Camunda 8.
Inti ditulis di Go sebagai modular monolith dengan Clean Architecture per modul; UI di React
(shadcn/ui, Zustand, TanStack, bpmn-js untuk BPMN, React Flow untuk graf lain) dengan Clean
Architecture per fitur; monorepo Nx; deploy lewat Docker.
Setiap fase punya dokumen sendiri di folder ini. Fase 0 dan 1 sudah dieksekusi dan di-merge; Fase 2 sudah dieksekusi di branch
`feat/phase-2-execution`; rencana Fase 3 sampai 8 berstatus draft dan menunggu review.

| Fase | Nama | Keluaran utama | Dokumen |
|---|---|---|---|
| 0 | Persiapan environment (✅ selesai 2026-10-28) | Monorepo Nx + Go + React jalan, Compose, proto toolchain, CI hijau | [00-fase-0-persiapan-environment.md](00-fase-0-persiapan-environment.md) |
| 1 | Parser dan model BPMN (✅ selesai 2026-10-28, di-merge ke `main`) | `libs/go/bpmn`: parse XML BPMN 2.0 ke model in-memory, validasi elemen yang didukung | [01-fase-1-parser-model-bpmn.md](01-fase-1-parser-model-bpmn.md) |
| 2 | Engine inti (✅ selesai 2026-10-28, menunggu review) | modul `execution`: eksekusi token, sequence flow, gateway (exclusive, parallel, inclusive), start/end event, service task sebagai job | [02-fase-2-engine-inti.md](02-fase-2-engine-inti.md) |
| 3 | Persistensi dan pemulihan | State instance di PostgreSQL, event log, replay setelah restart, idempotensi | [03-fase-3-persistensi-pemulihan.md](03-fase-3-persistensi-pemulihan.md) (draft, menunggu review) |
| 4 | API dan job worker | gRPC/Connect: deploy, create instance, activate jobs, complete/fail job, publish message; SDK worker Go | [04-fase-4-api-job-worker.md](04-fase-4-api-job-worker.md) (draft, menunggu review) |
| 5 | Timer, message, boundary event, sub-process | Scheduler timer, korelasi message, error dan escalation event | [05-fase-5-timer-message-boundary-subprocess.md](05-fase-5-timer-message-boundary-subprocess.md) (draft, menunggu review) |
| 6 | UI: Modeler, Operate, Tasklist | React: modeler bpmn-js dengan properties panel, monitoring instance (viewer bpmn-js + pohon instance React Flow), user task inbox | [06-fase-6-ui-modeler-operate-tasklist.md](06-fase-6-ui-modeler-operate-tasklist.md) (draft, menunggu review) |
| 7 | FEEL dan DMN | Evaluator FEEL untuk kondisi dan input/output mapping; DMN decision table (opsional) | [07-fase-7-feel-dmn.md](07-fase-7-feel-dmn.md) (draft, menunggu review) |
| 8 | Observability, keamanan, hardening deploy | OpenTelemetry, auth (OIDC), multi-tenant dasar, compose prod dengan reverse proxy/TLS, backup DB, zero-downtime restart | [08-fase-8-observability-keamanan-deploy.md](08-fase-8-observability-keamanan-deploy.md) (draft, menunggu review) |

Catatan review rencana Fase 2 sampai 8 (ditulis paralel pada 2026-10-28):

- Nomor ADR yang diusulkan tiap dokumen tumpang tindih di beberapa tempat (Fase 3 dan 4 sama-sama memakai ADR-0017 dan ADR-0018; Fase 7 memakai 0027 sampai 0029 yang juga berada di rentang Fase 5). Nomor final ditetapkan saat ADR benar-benar ditulis di `docs/adr/`; Fase 8 memuat langkah rekonsiliasi.
- Setiap dokumen punya bagian pertanyaan terbuka dengan rekomendasi; keputusan user dicatat di bagian 9.1 atau 11.1 dokumen masing-masing sebelum fase dimulai, seperti yang dilakukan di Fase 1.
- Estimasi per fase (1 engineer): Fase 2 16–19 hari, Fase 3 belum dihitung ulang setelah Fase 2, Fase 4 19–22, Fase 5 24–28, Fase 6 38–44, Fase 7 23–27 (+13–16 untuk DMN), Fase 8 34–40.

Prinsip lintas fase:

- Setiap modul engine baru lahir lewat generator dengan lima paket (`api`, `domain`, `application`, `infrastructure`, `interfaces`); setiap fitur UI baru lahir dengan empat lapisan. Batasnya ditegakkan lint.
- Deployment selalu berupa image Docker yang sama dari dev sampai produksi; `Makefile` adalah satu-satunya pintu masuk perintah.

- Format BPMN 100% kompatibel dengan Camunda Modeler, termasuk namespace `zeebe:` untuk task definition dan I/O mapping.
- Kontrak API selalu lahir dari `.proto` di `libs/proto`; tidak ada endpoint yang ditulis tangan.
- Setiap fase ditutup dengan uji paritas terhadap Camunda 8 referensi (profil `camunda` di Compose) untuk skenario yang relevan.
