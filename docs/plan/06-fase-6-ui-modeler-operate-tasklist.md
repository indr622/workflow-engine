# Fase 6 — UI: Modeler, Operate, dan Tasklist

**Proyek:** Workflow Engine (kompatibel konsep Camunda 8 / BPMN 2.0)
**Stack:** Go (engine utama) + React (UI) dalam monorepo Nx
**Status:** Draft rencana — belum dieksekusi
**Tanggal:** 2026-10-28

---

## 0. Status eksekusi

Belum ada langkah yang dieksekusi. Bagian ini diisi dengan tabel yang sama seperti Fase 0
(langkah, status, bukti) begitu pekerjaan dimulai.

**Kondisi repo yang menjadi titik awal (dicek 2026-10-28, Fase 1 sedang dieksekusi; Fase 2–5
masih draft):**

| Area | Kondisi / asumsi dari fase sebelumnya | Dampak ke Fase 6 |
|---|---|---|
| `apps/modeler-web` | Fitur `health`, `deployment` (toolbar validate/deploy, marker issue di bpmn-js, tabel definition, viewer detail — **sudah diimplementasikan**), `modeler` (`BpmnPanel`), `process-map` (React Flow dengan data contoh); route `/`, `/definitions`, `/definitions/$key`; composition root `src/app/{app.tsx,dependencies.ts,router.ts}`; ESLint per lapisan di `apps/modeler-web/eslint.config.mjs` | Fitur `deployment` dan `process-instance` (Fase 2 bagian 8.14) dipindah ke lib fitur bersama (3.2); modeler mendapat properties panel, file, draft, XML view, project explorer |
| `apps/operate-web`, `apps/tasklist-web` | Folder penanda dengan README "Dibuat di Fase 6" | Dibuat di fase ini dengan generator `@nx/react` (Vite, Vitest, Tailwind v4, TanStack Router) mengikuti `modeler-web` |
| `libs/web/ui` | shadcn/ui: `button`, `card`, `badge`, `table`, `input`, `cn`; tema `globals.css` dengan `.dark` dan token `--success` | Daftar komponen diperluas (10.2); toggle tema lahir di `app-shell` |
| `libs/web/bpmn-viewer` | `BpmnCanvas` (`mode: modeler \| viewer`), handle `saveXML/importXML/setMarkers/selectElement`, marker `issue-error`/`issue-warning`, `EMPTY_DIAGRAM` dengan `modeler:executionPlatformVersion="8.8.0"` | Diperluas: modul tambahan (properties panel, minimap, moddle `zeebe`), marker token, overlay badge, API `getInstance()` terbatas, event `onSelectionChanged`/`onChanged` |
| `libs/web/flow-canvas` | `FlowCanvas` (React Flow + tema shadcn), `CardNode`, `layoutGraph` (dagre), `proOptions.hideAttribution` | Diperluas: node `tree` untuk pohon instance, `direction: 'TB'`, `readOnly`, `onNodeClick`; cek ketentuan atribusi React Flow (15) |
| `libs/web/api-client` | `createEngineTransport('/api')`, `createHealthClient`, `createDeploymentClient`; Fase 4 menambah `createProcessInstanceClient`, `createJobClient`, `createIncidentClient`, `createMessageClient`, `findErrorInfo`, `duration.ts` | Ditambah `createUserTaskClient`, (bila Fase 5 menyediakan) `createTimerClient`; `index.ts` meng-export `usertask/v1` |
| `libs/web/core` | `Result`, `AppError`, `toAppError` | Ditambah util format (durasi, tanggal relatif, key `bigint`), `messages` i18n minimal (10.3) |
| Proto (Fase 1–5) | `deployment/v1`, `process/v1` (+ `SetVariables`, `CreateProcessInstanceWithResult`, Fase 3 `ListProcessInstanceRecords`, `ListElementInstances`), `job/v1`, `incident/v1`, `message/v1`, `common/v1` (`ErrorInfo`, `PageRequest`); Fase 5 diasumsikan `timer/v1` dan RPC subscription message | Fase 6 menambah `usertask/v1`, perluasan aditif `deployment/v1` (form) dan `process/v1` (statistik); bila Fase 5 tidak menyediakan RPC daftar timer/subscription, Fase 6 menambahkannya (9.4) |
| Engine | Modul `deployment`, `execution` (tabel `process_instance`, `element_instance`, `variable`, `job`, `incident`, `execution_record`), `job`, `message`; Fase 3 outbox durable + `Relay.Register(Consumer)`; Fase 4 interceptor `rpc` (`Validate`, `Authn` placeholder dengan `authn.Principal`), `sweeper`; user task **job-based** (`io.camunda.zeebe:userTask`), `zeebe:userTask` ditolak `UNSUPPORTED_ELEMENT` (ADR-0018) | Modul baru `usertask` (bagian 8), tabel `user_task` dan `form`; `ProfileExecutable` membuka `zeebe:userTask`; `.form` diterima deploy |
| `infra/docker` | `web.Dockerfile` hanya membangun `modeler-web`; `nginx/default.conf` SPA + proxy `/api/` ke `engine:8080`; compose prod satu service `web` (port `WEB_PORT` 8088) | Satu image per app lewat `ARG APP` (bagian 11); compose prod tiga service web |
| CI | Job `check` (`make check` affected), `integration`, `docker` | Job `e2e` (Playwright) ditambahkan; `docker` membangun tiga image web |
| Lint | `@nx/enforce-module-boundaries` dengan tag `type:app \| ui \| api-client \| core`; aturan lapisan hanya untuk `src/features/*/<lapisan>` di app | Tag baru `type:feature`; aturan lapisan dipindah ke `libs/web/eslint.feature.config.mjs` agar berlaku juga untuk lib fitur |
| ADR | Fase 0: 0001–0007; Fase 1: 0008–0009; Fase 2: 0010–0013; Fase 3: 0014–0018; Fase 4: 0017–0020 (**nomor 0017–0018 tumpang tindih** antara Fase 3 dan Fase 4 — harus dirapikan saat salah satunya di-merge); Fase 5 dan Fase 7 diperkirakan memakai 0021–0029 | Fase 6 **memakai ADR-0030 sampai ADR-0036** agar tidak bertabrakan dengan Fase 5/7 yang ditulis bersamaan |

## 1. Lingkup

Fase 6 memberi engine **tiga antarmuka pengguna** di atas API Fase 1–5, dan **satu kemampuan
engine baru** yang dibutuhkan antarmuka ketiga: user task native. Setelah fase ini, seorang
analis bisa menggambar proses lengkap dengan properti `zeebe:` di Modeler dan men-deploy-nya;
operator bisa melihat instance berjalan di Operate, menemukan incident, memperbaiki variabel dan
melanjutkan proses; pengguna bisnis bisa membuka inbox di Tasklist, mengklaim tugas, mengisi form,
dan menyelesaikannya — semuanya dari browser, tanpa SDK.

Termasuk dalam Fase 6:

1. **Topologi UI** (ADR-0030): tiga aplikasi terpisah (`modeler-web`, `operate-web`,
   `tasklist-web`) di atas **lib fitur bersama** `libs/web/feature-*` (empat lapisan, tag
   `type:feature`) dan `libs/web/app-shell` (layout, navigasi, tema, auth placeholder, error
   boundary). Fitur `deployment` dan `process-instance` dipindah dari `modeler-web` ke lib bersama.
2. **Modeler**: properties panel Camunda 8 (`bpmn-js-properties-panel` + provider `zeebe`,
   ADR-0031), buka/simpan/unduh file, draft lokal (localStorage, per-viewer), tampilan XML,
   minimap, pintasan keyboard, project explorer minimal (banyak diagram), tombol "Start instance"
   dari daftar definition. Element templates dan editor FEEL mandiri ditunda (Fase 7).
3. **Operate**: daftar instance dengan filter (state, proses, versi, incident), detail instance
   (viewer bpmn-js dengan marker token aktif/selesai/incident dan overlay jumlah), panel variabel
   (edit lewat `SetVariables`), panel incident (`ResolveIncident`, gabungan `UpdateJobRetries` +
   resolve), panel job, pohon instance dan statistik flow node dengan React Flow, timer dan
   subscription message (data Fase 5), cancel instance, dialog start instance (TanStack Form + Zod),
   halaman process definition dengan jumlah instance berjalan dan statistik per elemen, pembaruan
   langsung dengan polling adaptif TanStack Query (ADR-0032).
4. **User task native di engine** (ADR-0033): `ProfileExecutable` menerima `zeebe:userTask`; tabel
   `user_task` dan record `USER_TASK *` milik `execution` (atomik dengan token, seperti `job`);
   modul **`usertask`** lima lapisan sebagai lapisan Tasklist (kebijakan assign/claim/unclaim,
   pencocokan candidate, pencarian inbox, resolusi form) tanpa tabel sendiri; state `CREATED`,
   `ASSIGNED`, `COMPLETED`, `CANCELED`; `UserTaskService` di `workflow/usertask/v1`.
5. **Form** (ADR-0034): resource `.form` (Camunda Forms JSON) diterima deploy, disimpan di tabel
   `form` milik `deployment`, dirujuk `zeebe:formDefinition formId`, dirender `@bpmn-io/form-js`
   di Tasklist dan dipratinjau di Modeler; fallback editor variabel JSON bila form tidak ada.
6. **Tasklist**: inbox dengan filter (milik saya, bisa diklaim oleh saya, semua, selesai), detail
   task dengan form + submit → `CompleteUserTask` dengan variabel; assign/claim/unclaim; due date
   dan follow-up; identitas pengguna dari `authn.Principal` (anonim sampai Fase 8, pemilih identitas
   dev di UI).
7. **Lintas aplikasi**: sistem desain `libs/web/ui` dilengkapi; UI bahasa Inggris dengan string
   terpusat (id-ID menyusul); aksesibilitas dasar; error boundary dan empty state; baseline
   responsif; toggle dark mode; e2e Playwright (smoke) di CI; Vitest + Testing Library tanpa
   Storybook (ADR-0036).
8. **Docker/deploy** (ADR-0035): satu image nginx per app dari `web.Dockerfile` ber-`ARG APP`;
   compose prod tiga service web; `make docker-build` membangun semuanya; Makefile `dev` menjalankan
   tiga app.

**Tidak termasuk** (fase lain): element templates, editor FEEL mandiri dan DMN modeler (Fase 7);
OIDC, otorisasi per peran, multi-tenant di UI, SSE/streaming pembaruan langsung, reverse proxy dan
TLS (Fase 8); task listener dan state transien `ASSIGNING`/`UPDATING`/`COMPLETING` ala Camunda
8.8 (ditinjau setelah Fase 7); user task multi-instance (Fase 7 bersama multi-instance); kolaborasi
real-time di Modeler; penyimpanan diagram di server (Modeler tetap berbasis file + draft lokal;
pertanyaan 16.2 #4); riwayat instance lengkap (audit log per record) di Operate selain tampilan
record berpaginasi sederhana; i18n id-ID aktual.

## 2. Definition of Done Fase 6

- [ ] `pnpm nx show projects` memuat `modeler-web`, `operate-web`, `tasklist-web`, `app-shell`, `feature-deployment`, `feature-process-instance`, `feature-user-task`, tiga proyek `*-e2e`; `make check` hijau; `@nx/enforce-module-boundaries` menolak app meng-import lapisan internal lib fitur (dibuktikan test lint negatif).
- [ ] Aturan lapisan ESLint (`domain` tanpa framework, `application` tanpa `infrastructure`/`presentation`, `presentation` tanpa `infrastructure`, transport hanya di `infrastructure` dan `src/app`) berlaku untuk **app dan lib fitur** lewat satu konfigurasi `libs/web/eslint.feature.config.mjs`.
- [ ] Modeler: properties panel menampilkan dan mengedit `zeebe:taskDefinition`, `zeebe:ioMapping`, `zeebe:taskHeaders`, `zeebe:assignmentDefinition`, `zeebe:formDefinition`, `zeebe:taskSchedule`, `zeebe:subscription`, timer definition, kondisi sequence flow, `zeebe:calledElement`; XML hasil `saveXML` dari diagram yang dibuat di Modeler kita **dibuka tanpa perubahan semantik** oleh Camunda Modeler 5.x dan lolos `ValidateResource` profil `EXECUTABLE` (korpus `libs/go/bpmn/testdata`).
- [ ] Modeler: buka file `.bpmn` (drag-drop dan dialog), simpan/unduh, draft tersimpan otomatis ke localStorage ≤ 1 s setelah perubahan dan dipulihkan setelah reload (dengan konfirmasi), project explorer menampilkan ≥ 2 diagram dan berpindah tanpa kehilangan perubahan, tampilan XML sinkron dua arah, minimap dan pintasan (undo/redo, hapus, zoom, cari elemen) berfungsi; "Start instance" dari tabel definition membuka dialog dan membuat instance.
- [ ] Operate: `/instances` memfilter berdasarkan `bpmn_process_id`, versi, state, `only_with_incident`, dan paginasi keyset; `/instances/$key` menampilkan viewer dengan marker **aktif** (biru), **selesai** (abu), **incident** (merah) dan badge jumlah per elemen yang benar untuk fork paralel 50 cabang dan loop (diuji e2e terhadap engine nyata); panel variabel mengedit dan menyimpan lewat `SetVariables` dengan validasi JSON; panel incident menyelesaikan incident (untuk `JOB_NO_RETRIES` sekaligus `UpdateJobRetries`); cancel instance dengan konfirmasi; pohon instance React Flow menampilkan scope (process → sub-process/call activity → elemen aktif) dan klik node memilih elemen di viewer.
- [ ] Operate: `/definitions/$key` menampilkan jumlah instance `ACTIVE`/`COMPLETED`/`TERMINATED`/dengan incident dan statistik per elemen (`GetProcessDefinitionStatistics`) sebagai overlay pada viewer; polling adaptif: detail instance `ACTIVE` di-refetch tiap 1 s, daftar tiap 5 s, berhenti saat tab tersembunyi (`refetchIntervalInBackground: false`), dan berhenti saat instance final.
- [ ] Engine: `ProfileExecutable` menerima `userTask` dengan `zeebe:userTask`; aktivasi membuat baris `user_task` (`CREATED`, atau `ASSIGNED` bila `assignee` statis/terevaluasi) dalam transaksi yang sama dengan token, record `USER_TASK CREATED` di `execution_record`, dan replay Fase 3 memulihkan task beserta assignee; `CompleteUserTask` menulis variabel, record `USER_TASK COMPLETED`, dan token lanjut; `CancelProcessInstance` → `USER_TASK CANCELED`.
- [ ] `UserTaskService`: `AssignUserTask` (dengan `allow_override`), `UnassignUserTask`, `UpdateUserTask` (candidate, due/follow-up, priority), `CompleteUserTask`, `GetUserTask` (opsional variabel), `SearchUserTasks` (filter state, assignee, candidate group/user, `bpmn_process_id`, `process_instance_key`, `element_id`, due sebelum/sesudah; urut `created_at`/`due_date`), `GetUserTaskForm`; setiap penolakan membawa `ErrorInfo.reason` (tabel 9.3); protovalidate pada semua request.
- [ ] Form: deploy `.form` menyimpan ke tabel `form` dengan versi per `form_id` dan dedup checksum (sama dengan proses); `DeployResourceResponse.forms` terisi; `GetForm(form_id, version=0)` mengembalikan JSON; user task dengan `formId` tak dikenal menghasilkan **warning** `FORM_NOT_FOUND` saat deploy (bukan penolakan) dan Tasklist menampilkan fallback editor JSON.
- [ ] Tasklist: inbox menampilkan task `CREATED`/`ASSIGNED` sesuai filter "Assigned to me", "Claimable by me" (candidate user/group cocok atau tanpa candidate), "All open", "Completed"; detail task merender form-js dari `GetUserTaskForm` dengan nilai awal dari variabel instance, validasi form-js memblokir submit, submit memanggil `CompleteUserTask` dengan data form sebagai variabel; claim/unclaim/assign ke pengguna lain; due date lewat ditandai; task hilang dari inbox ≤ 5 s setelah diselesaikan pengguna lain (polling).
- [ ] `app-shell`: layout + navigasi per app, toggle tema (terang/gelap/sistem, localStorage, `prefers-color-scheme`), `AuthProvider` placeholder (`Principal{subject:'anonymous'}` + pemilih identitas dev yang mengirim header `X-Dev-User`/`X-Dev-Groups` hanya bila `VITE_DEV_IDENTITY=true`), `ErrorBoundary` per route dengan tombol retry, komponen `EmptyState`, `PageSkeleton`; semua halaman lolos axe-core tanpa pelanggaran `serious`/`critical` (dijalankan di e2e).
- [ ] `libs/web/ui` memiliki komponen tabel 10.2; semua memakai token tema (tidak ada warna hard-coded selain marker kanvas); `pnpm nx test ui` hijau.
- [ ] e2e Playwright (`@nx/playwright`) untuk ketiga app berjalan di CI job `e2e` terhadap engine nyata + Postgres: (a) modeler: buka file korpus → set `zeebe:taskDefinition` → deploy → tampil di definitions; (b) operate: start instance → marker aktif → complete job lewat API → marker selesai → instance `COMPLETED`; (c) tasklist: deploy proses user task + form → start → claim → isi form → complete → instance selesai. Total < 5 menit di CI.
- [ ] `make docker-build` menghasilkan `engine`, `web-modeler`, `web-operate`, `web-tasklist`; `make deploy-local` menyajikan ketiganya di port `MODELER_PORT`/`OPERATE_PORT`/`TASKLIST_PORT`; healthcheck nginx hijau; `/api/` diproksi dengan `proxy_read_timeout 70s`.
- [ ] Bundle: chunk awal (tanpa bpmn-js/form-js yang di-lazy-load per route) ≤ 350 kB gzip per app; `pnpm nx build` ketiganya < 3 menit total di CI; tidak ada `import '@xyflow/react'` atau `import 'bpmn-js'` di luar `libs/web/{flow-canvas,bpmn-viewer}` (lint `no-restricted-imports` di root).
- [ ] Paritas: suite `usertask` (bagian 12.3) membandingkan perilaku user task native dan form terhadap Camunda 8.8 (REST v2 user task); deviasi tercatat di `expectations.json`.
- [ ] ADR-0030 sampai ADR-0036 tersimpan di `docs/adr/`; `docs/plan/README.md` menautkan dokumen ini; README tiap app dan lib fitur berisi peta fitur dan cara menjalankan.

## 3. Keputusan teknis

| Area | Keputusan | Alasan singkat |
|---|---|---|
| Topologi app | **Tiga app terpisah** + lib fitur bersama `libs/web/feature-*` + `libs/web/app-shell` (ADR-0030, 3.1) | Roadmap dan ADR-0006 sudah menyebut tiga app; image dan rilis independen; Nx affected hanya membangun app yang berubah; duplikasi dihindari lewat lib fitur empat lapisan |
| Lib fitur bersama | `feature-deployment` (dipindah dari modeler-web), `feature-process-instance` (dipindah dari Fase 2, diperluas: instance, variabel, incident, job, statistik), `feature-user-task`; tag `type:feature`; port repository di `domain`, provider context di `application`, dirakit di `dependencies.ts` tiap app | Satu implementasi untuk dialog start instance (Modeler + Operate), panel incident (Operate), complete task (Tasklist + Operate) |
| Properties panel | `bpmn-js-properties-panel` + `@bpmn-io/properties-panel` + `zeebe-bpmn-moddle` + `camunda-bpmn-js-behaviors` (ADR-0031, 3.3); dibungkus di `libs/web/bpmn-viewer` sebagai opsi `propertiesPanel` | Panel resmi bpmn.io yang dipakai Camunda Modeler; menghasilkan XML `zeebe:` identik; **verifikasi lisensi** tiap paket sebelum dipasang (3.3) |
| Pembaruan langsung Operate/Tasklist | **Polling TanStack Query adaptif** (ADR-0032, 6.6): `refetchInterval` fungsi dari state data; SSE ditinjau Fase 8 | Tanpa infrastruktur baru di engine/nginx; `GetProcessInstance` murah; `refetchIntervalInBackground: false` menekan beban; Fase 8 bisa mengganti dengan SSE tanpa mengubah hook (`queryKey` sama) |
| Tempat state user task | **Tabel `user_task` dan record `USER_TASK *` milik `execution`** (seperti `job`); modul `usertask` = lapisan Tasklist tanpa tabel (ADR-0033, 3.5) | Pembuatan/penyelesaian task harus atomik dengan token dan dapat di-replay (Fase 3); keputusan Fase 2 #6 untuk job berlaku identik; `usertask` menyimpan kebijakan dan pencarian, bukan state |
| State user task | `CREATED`, `ASSIGNED`, `COMPLETED`, `CANCELED` persisten; `ASSIGNED` = ada `assignee`; unassign → `CREATED` | Filter inbox sederhana lewat indeks; state transien Camunda 8.8 (`ASSIGNING`, `UPDATING`, `COMPLETING`) hanya bermakna dengan task listener yang belum ada |
| Form | Camunda Forms JSON sebagai resource `.form` di tabel `form` milik `deployment`; render `@bpmn-io/form-js` (viewer) di Tasklist, editor form-js **tidak** dibundel (Fase 7+, pertanyaan 16.2 #6) (ADR-0034) | Format yang sama dengan Camunda Modeler; `formId` di `zeebe:formDefinition` sudah diparse Fase 1; **verifikasi lisensi form-js** |
| Identitas pengguna | `authn.Principal` dari interceptor Fase 4; Fase 6 menambah **identitas dev** opsional (`X-Dev-User`, `X-Dev-Groups`) yang hanya dibaca bila `ENGINE_DEV_IDENTITY=true` | Tasklist butuh "saya" untuk claim dan filter; Fase 8 mengganti sumber `Principal` tanpa mengubah modul `usertask` |
| Marker token di viewer | `canvas.addMarker` kelas `token-active`/`token-completed`/`token-incident` + `overlays.add` badge jumlah; dihitung dari `ElementInstance[]` oleh fungsi murni `domain/markers.ts` | Pola yang sama dengan marker issue Fase 1; fungsi murni teruji Vitest; bpmn-js tidak perlu plugin tambahan |
| Pohon instance | React Flow (`flow-canvas`) node `TreeNode` dari `element_instance.flow_scope_key` (+ `parent_process_instance_key` bila call activity tersedia dari Fase 5/7), layout `TB` | Sesuai pembagian peran 4.3 Fase 0: graf non-BPMN di React Flow |
| Form dialog | TanStack Form + Zod (`zod` v4 sudah ada) untuk dialog start instance, set variables, update task; form-js untuk form bisnis | Dua dunia berbeda: form teknis (JSON) vs form bisnis (skema Camunda) |
| State klien | Zustand per fitur: seleksi elemen, panel terbuka, filter inbox, draft diagram (persist ke localStorage hanya untuk draft modeler dan preferensi tampilan) | Aturan ADR-0006; localStorage hanya untuk kenyamanan per-viewer, bukan state yang harus andal |
| i18n | **Bahasa Inggris** di Fase 6; string terpusat per fitur (`messages.ts`) dengan helper `t()` tipis di `libs/web/core`; library i18n dipilih bersama id-ID (pertanyaan 16.2 #8) | Menghindari library sebelum ada kebutuhan nyata; string terpusat membuat migrasi murah |
| Uji UI | Vitest + Testing Library + fake repository (port); **tanpa Storybook, tanpa MSW** (ADR-0036); e2e Playwright terhadap engine nyata | Port repository sudah memisahkan transport; fake lebih murah dari mock jaringan; Storybook menambah toolchain tanpa pengguna desain |
| Docker | Satu image per app (`web-modeler`, `web-operate`, `web-tasklist`) dari `web.Dockerfile` dengan `ARG APP`; nginx conf sama (ADR-0035, bagian 11) | Rilis independen; image kecil; Phase 8 reverse proxy memilih host-based atau path-based tanpa membangun ulang |
| Lazy load | bpmn-js (+ properties panel), form-js, React Flow di-`lazy()` per route; `manualChunks` Vite untuk `bpmn`, `form`, `flow` | Tasklist tidak memuat bpmn-js kecuali membuka pratinjau diagram; chunk awal kecil |

### 3.1 Topologi aplikasi (ADR-0030)

| Opsi | Kelebihan | Kekurangan |
|---|---|---|
| A. Tiga app, fitur lokal per app (tanpa lib fitur) | Paling sederhana; tidak ada tag baru | Dialog start instance, panel incident, complete task, mapper proto → domain ditulis dua–tiga kali; Fase 2 sudah memperingatkan fitur `process-instance` "dipindah utuh" |
| B. Satu app shell dengan tiga area (`/modeler`, `/operate`, `/tasklist`) | Satu image, satu origin (auth cookie Fase 8 mudah), satu `QueryClient`, navigasi lintas area tanpa reload | Nx affected selalu membangun semuanya; satu bundle besar (bpmn-js modeler + properties panel + form-js + React Flow) meski bisa di-split per route; satu deploy untuk tiga peran pengguna yang jadwal rilisnya berbeda; roadmap dan ADR-0006 menyebut tiga aplikasi; e2e satu app besar lebih lambat |
| **C. Tiga app + lib fitur bersama + `app-shell`** | Rilis dan image independen; affected tepat sasaran; duplikasi nol; auth Fase 8 diimplementasikan sekali di `app-shell`; tiap app tetap tipis (route + composition root) | Tag dan aturan lint baru; lib fitur butuh provider repository per app; sub-path/host per app harus diputuskan di reverse proxy Fase 8 |

**Dipilih C.** Konsekuensi:

- Tag baru `type:feature` dengan constraint: `type:app` → `type:feature | ui | core | api-client`;
  `type:feature` → `type:feature | ui | core | api-client` (hanya `infrastructure` yang boleh
  menyentuh `api-client`, ditegakkan `no-restricted-imports`); `type:ui` dan `type:core` tidak
  berubah.
- Lib fitur **tidak** meng-import lib fitur lain secara siklik; graf yang diizinkan:
  `feature-user-task` → `feature-process-instance` (membuka instance dari task) →
  `feature-deployment` (definition untuk start instance). Nx `enforce-module-boundaries` menolak
  siklus secara default.
- Setiap app memiliki `src/app/dependencies.ts` yang merakit **semua** repository yang dipakai
  fitur-fiturnya (pola yang sudah ada), `src/routes/`, dan fitur lokal di `src/features/` untuk
  hal yang hanya ada di app itu.
- Auth Fase 8: `app-shell` mengekspor `AuthProvider` + `useAuth()`; interceptor Connect yang
  memasang `Authorization` dibuat di `api-client` (`createEngineTransport(baseUrl, { getToken })`)
  dan dipakai `dependencies.ts` — satu titik ganti per app.
- Origin: dev `localhost:4200` (modeler), `4201` (operate), `4202` (tasklist), semua proxy `/api`
  ke engine; prod tiga container nginx (bagian 11). Keputusan host-based vs path-based di reverse
  proxy ditunda ke Fase 8 (pertanyaan 16.2 #1); app dibangun dengan `base: '/'` dan mendukung
  `VITE_BASE_PATH` agar path-based tidak memerlukan perubahan kode.

### 3.2 Tata letak lib fitur bersama dan app

```
libs/web/
├─ app-shell/                          # tag type:ui (bergantung ui, core; tanpa api-client)
│  └─ src/
│     ├─ index.ts
│     └─ lib/
│        ├─ app-layout.tsx             # header, nav (props), <Outlet/> slot, footer versi
│        ├─ theme-provider.tsx         # ThemeProvider, useTheme, ThemeToggle (localStorage "theme")
│        ├─ auth-context.tsx           # AuthProvider, useAuth(): { principal, devIdentity, setDevIdentity }
│        ├─ dev-identity-picker.tsx    # hanya dirender bila import.meta.env.VITE_DEV_IDENTITY === 'true'
│        ├─ error-boundary.tsx         # RouteErrorBoundary (TanStack Router errorComponent) + retry
│        ├─ empty-state.tsx, page-skeleton.tsx, not-found.tsx
│        └─ query-client.ts            # createAppQueryClient(): staleTime, retry, onError toast
├─ eslint.feature.config.mjs           # aturan lapisan (dipakai app dan lib fitur)
├─ feature-deployment/                 # tag type:feature, scope:shared — DIPINDAH dari apps/modeler-web/src/features/deployment
│  └─ src/{domain,application,infrastructure,presentation,index.ts}
│     # domain: ProcessDefinition, ValidationIssue, Form, DeploymentRepository (+ listForms, getForm)
│     # application: definitionsQuery, definitionQuery, formQuery, useDeployResource, useValidateResource, deployment.store
│     # infrastructure: ConnectDeploymentRepository (+ mapper Form)
│     # presentation: DeployToolbar, IssuesPanel, DefinitionsTable (prop `actions` untuk tombol Start/Open)
├─ feature-process-instance/           # tag type:feature — DIPINDAH dari Fase 2 (8.14), diperluas
│  └─ src/
│     ├─ domain/
│     │  ├─ process-instance.ts        # ProcessInstance, ElementInstance, Variable, Job, state enum domain
│     │  ├─ incident.ts                # Incident, aturan "aksi resolusi" per error_type (6.4)
│     │  ├─ markers.ts                 # markersFor(elementInstances, incidents): MarkerSet; badge counts
│     │  ├─ instance-tree.ts           # buildInstanceTree(elementInstances): TreeNode[] (scope → anak)
│     │  ├─ statistics.ts              # FlowNodeStatistics, mergeStatistics
│     │  ├─ variables.ts               # parseVariablesJson (Zod), diffVariables, formatValue
│     │  └─ repository.ts              # ProcessInstanceRepository, IncidentRepository, JobRepository, TimerRepository?, SubscriptionRepository?
│     ├─ application/
│     │  ├─ process-instance.queries.ts  # key factory; instanceQuery (refetchInterval adaptif), instancesQuery, recordsQuery,
│     │  │                               # statisticsQuery, definitionStatisticsQuery, timersQuery, subscriptionsQuery
│     │  ├─ process-instance.mutations.ts # useCreateInstance, useCancelInstance, useSetVariables, useResolveIncident,
│     │  │                               # useRetryJob (UpdateJobRetries + ResolveIncident), useCompleteJob
│     │  ├─ instance-view.store.ts       # Zustand: elemen terpilih, tab panel, filter daftar (bukan data engine)
│     │  └─ repository.context.tsx       # ProcessInstanceRepositoryProvider + hook
│     ├─ infrastructure/
│     │  ├─ connect-process-instance.repository.ts, connect-incident.repository.ts, connect-job.repository.ts
│     │  └─ mappers.ts                   # proto → domain (bigint → string key, Timestamp → Date, enum → union)
│     └─ presentation/
│        ├─ instances-table.tsx, instance-filters.tsx
│        ├─ instance-viewer.tsx          # BpmnCanvas viewer + marker + overlay + klik elemen → store
│        ├─ instance-tree.tsx            # FlowCanvas readOnly, TreeNode
│        ├─ variables-panel.tsx, variable-editor-dialog.tsx
│        ├─ incidents-panel.tsx, jobs-panel.tsx, timers-panel.tsx, subscriptions-panel.tsx, records-panel.tsx
│        ├─ start-instance-dialog.tsx    # TanStack Form + Zod (JSON object), pilih definition/versi
│        ├─ cancel-instance-button.tsx
│        └─ definition-statistics-overlay.tsx
└─ feature-user-task/                  # tag type:feature
   └─ src/
      ├─ domain/
      │  ├─ user-task.ts                # UserTask, UserTaskState, TaskFilter, aturan canClaim(principal, task), isOverdue
      │  ├─ form.ts                     # FormSchema (JSON), formDataToVariables, variablesToFormData
      │  └─ repository.ts              # UserTaskRepository
      ├─ application/
      │  ├─ user-task.queries.ts        # tasksQuery (polling 5 s), taskQuery (polling 2 s saat terbuka), taskFormQuery
      │  ├─ user-task.mutations.ts      # useClaimTask, useUnclaimTask, useAssignTask, useUpdateTask, useCompleteTask
      │  ├─ inbox.store.ts              # filter aktif, task terpilih, draft isian form per task (sessionStorage)
      │  └─ repository.context.tsx
      ├─ infrastructure/
      │  ├─ connect-user-task.repository.ts, mappers.ts
      └─ presentation/
         ├─ task-list.tsx, task-filters.tsx, task-row.tsx
         ├─ task-detail.tsx             # header, metadata, aksi claim/unclaim/assign, panel form
         ├─ task-form.tsx               # form-js viewer (lazy), onSubmit → complete; fallback JsonVariablesForm
         ├─ assign-dialog.tsx, update-task-dialog.tsx (due/follow-up/candidates)
         └─ task-badges.tsx             # state, overdue, candidate

apps/modeler-web/src/
├─ app/{app.tsx,dependencies.ts,router.ts}
├─ routes/
│  ├─ __root.tsx                       # AppLayout dari app-shell, nav: Modeler, Definitions
│  ├─ index.tsx                        # /: ModelerPage (explorer | kanvas | properties panel)
│  ├─ definitions.index.tsx            # tabel + tombol Start (StartInstanceDialog) + Open in modeler
│  └─ definitions.$key.tsx             # viewer + statistik ringkas + tautan ke Operate (URL dari env)
└─ features/
   ├─ health/                          # tetap
   ├─ modeler/                         # diperluas (bagian 5)
   │  ├─ domain/        diagram.ts (Diagram{id,name,xml,updatedAt}), draft.ts (aturan autosave, batas ukuran), shortcuts.ts
   │  ├─ application/   modeler.store.ts (diagram aktif, dirty, panel), drafts.store.ts (persist localStorage), useDiagramFile
   │  ├─ infrastructure/ file-system.ts (File System Access API + fallback), local-draft.repository.ts (localStorage)
   │  └─ presentation/  modeler-page.tsx, modeler-toolbar.tsx, project-explorer.tsx, properties-panel-host.tsx,
   │                    xml-view.tsx, shortcuts-help-dialog.tsx, form-preview-dialog.tsx
   └─ process-map/                     # tetap; data contoh diganti definition dari feature-deployment (opsional, 16.2 #9)

apps/operate-web/src/
├─ app/{app.tsx,dependencies.ts,router.ts}
├─ routes/
│  ├─ __root.tsx                       # nav: Dashboard, Instances, Definitions, Incidents
│  ├─ index.tsx                        # /: dashboard ringkas (jumlah instance per state, incident terbuka, definition terbaru)
│  ├─ instances.index.tsx              # /instances?process=&version=&state=&incident=
│  ├─ instances.$key.tsx               # /instances/$key?tab=variables|incidents|jobs|timers|records&element=
│  ├─ definitions.index.tsx            # /definitions (jumlah instance per definition)
│  ├─ definitions.$key.tsx             # /definitions/$key (viewer + statistik + daftar instance)
│  └─ incidents.index.tsx              # /incidents (lintas instance, ListIncidents)
└─ features/
   └─ dashboard/                       # lokal: agregasi query dari lib fitur (domain: ringkasan; presentation: kartu)

apps/tasklist-web/src/
├─ app/{app.tsx,dependencies.ts,router.ts}
├─ routes/
│  ├─ __root.tsx                       # nav: Tasks, Processes (start instance oleh pengguna bisnis)
│  ├─ tasks.index.tsx                  # /tasks?filter=me|claimable|all|completed&process=
│  ├─ tasks.$key.tsx                   # /tasks/$key (detail + form); layout master–detail di ≥ lg
│  └─ processes.index.tsx              # /processes: definition yang bisa dimulai (StartInstanceDialog)
└─ features/
   └─ inbox/                           # lokal: komposisi master–detail, pintasan (j/k), preferensi tampilan
```

Pemindahan `deployment` dan `process-instance` ke lib dilakukan dengan `git mv` + perubahan import
(`../features/deployment` → `@workflow/feature-deployment`); test Vitest ikut pindah; tidak ada
perubahan perilaku. Alias path `@workflow/feature-*` ditambahkan ke `tsconfig.base.json`.

### 3.3 Properties panel Camunda 8 (ADR-0031)

Paket yang dibutuhkan (versi dikunci saat instalasi; **verifikasi lisensi dan dokumentasi
sebelum dipasang**):

| Paket | Fungsi | Lisensi (verifikasi) |
|---|---|---|
| `bpmn-js` (sudah ada, 18.x) | Modeler/viewer | bpmn.io licence: bebas dipakai dengan syarat logo "Powered by bpmn.io" di kanvas tidak dihapus — **verifikasi teks lisensi** dan pastikan kita tidak menyembunyikan watermark |
| `bpmn-js-properties-panel` | `BpmnPropertiesPanelModule`, `BpmnPropertiesProviderModule`, `ZeebePropertiesProviderModule` | diyakini MIT — verifikasi |
| `@bpmn-io/properties-panel` | Komponen panel + CSS (`@bpmn-io/properties-panel/dist/assets/properties-panel.css`) | diyakini MIT — verifikasi |
| `zeebe-bpmn-moddle` | Deskriptor moddle `zeebe:` (`resources/zeebe.json`) agar atribut `zeebe:` dibaca/ditulis | diyakini MIT — verifikasi |
| `camunda-bpmn-js-behaviors` | `ZeebeBehaviorsModule`: perilaku editor khusus Zeebe (mis. membersihkan ekstensi saat tipe elemen diganti) | diyakini MIT — verifikasi |
| `diagram-js-minimap` | Minimap | verifikasi lisensi |
| `@bpmn-io/form-js` (viewer; editor tidak dibundel) | Render Camunda Forms di Tasklist, pratinjau di Modeler | **verifikasi lisensi** (diduga bpmn.io licence seperti bpmn-js) |
| `bpmn-js-element-templates` | Element templates — **ditunda** (Fase 7+) | verifikasi saat dipakai |

Integrasi di `libs/web/bpmn-viewer`:

```ts
// libs/web/bpmn-viewer/src/lib/bpmn-canvas.tsx (perluasan props)
export interface BpmnCanvasProps {
  // ...Fase 0–1
  /** Container DOM untuk properties panel; hanya mode modeler. */
  propertiesPanelParent?: HTMLElement | null;
  /** Modul tambahan: 'zeebe' (moddle + behaviors + provider), 'minimap'. Default mode modeler: ['zeebe']. */
  extensions?: Array<'zeebe' | 'minimap'>;
  /** Dipanggil pada commandStack.changed (debounce 250 ms) — dipakai dirty flag dan autosave draft. */
  onChanged?: () => void;
  onSelectionChanged?: (elementIds: string[]) => void;
}
export type MarkerClass = 'issue-error' | 'issue-warning' | 'token-active' | 'token-completed' | 'token-incident';
export interface BpmnCanvasHandle {
  // ...Fase 0–1
  setOverlays(group: string, overlays: Array<{ elementId: string; html: string; position: OverlayPosition }>): void;
  zoom(step: 'in' | 'out' | 'fit' | 'reset'): void;
  undo(): void; redo(): void;
  findElements(query: string): Array<{ id: string; name: string; type: string }>;
}
```

Modul-modul bpmn-js hanya di-import di `libs/web/bpmn-viewer` (aturan `no-restricted-imports` di
root: `bpmn-js*`, `@bpmn-io/*`, `diagram-js*`, `zeebe-bpmn-moddle`, `camunda-bpmn-js-behaviors`
dilarang di luar lib ini; `@bpmn-io/form-js` dilarang di luar `libs/web/form-renderer`, 4.4).
Konstruktor modeler menerima `additionalModules` dan `moddleExtensions: { zeebe }` sesuai dokumentasi
`bpmn-js-properties-panel` (verifikasi nama ekspor di versi yang dipasang). Pratinjau: Modeler
kita harus menghasilkan `zeebe:taskDefinition`, `zeebe:ioMapping`, `zeebe:formDefinition` dengan
atribut yang **persis** seperti Camunda Modeler 5.x — diuji dengan membandingkan XML (setelah
normalisasi id) terhadap korpus `libs/go/bpmn/testdata/valid` yang berasal dari Camunda Modeler.

### 3.4 Pembaruan langsung: polling adaptif vs streaming (ADR-0032)

| Opsi | Kelebihan | Kekurangan |
|---|---|---|
| **A. Polling TanStack Query adaptif** | Tidak ada endpoint baru; bekerja lewat nginx/HTTP/1.1; `refetchInterval` sebagai fungsi `(query) => ms` menyesuaikan state (instance final → berhenti); `refetchIntervalInBackground: false` | Latensi ≤ interval; beban N klien × RPC/detik (ringan: `GetProcessInstance` adalah beberapa kueri berindeks) |
| B. SSE `/events?process_instance_key=` dari outbox consumer `operate.projection` (Fase 3) | Latensi < 100 ms; satu koneksi per tab | Butuh consumer durable + fan-out in-process + konfigurasi nginx (`proxy_buffering off`), reconnect, dan model event untuk UI; Fase 8 lebih tepat bersama observability |
| C. Connect server-streaming `WatchProcessInstance` | Tipe aman dari proto | Connect-web streaming butuh HTTP/2 atau fetch streaming; nginx `proxy_http_version 1.1` memerlukan penyetelan; sama kompleksnya dengan B |

**Dipilih A** untuk Fase 6. Kontrak hook dibuat agar B bisa menggantikannya: `instanceQuery(key)`
mengembalikan `queryOptions` dengan `queryKey ['process-instance', 'detail', key]`; Fase 8 cukup
menambah `useInstanceEvents(key)` yang memanggil `queryClient.setQueryData`/`invalidateQueries`
pada key yang sama dan mematikan `refetchInterval`. Interval (dapat diubah lewat env
`VITE_POLL_*`): detail instance `ACTIVE` 1 s, instance final tidak di-poll, daftar instance 5 s,
daftar incident 5 s, definition statistics 10 s, inbox Tasklist 5 s, detail task terbuka 2 s,
dashboard 10 s. Semua query memakai `staleTime` = interval − 100 ms agar navigasi antar halaman
tidak memicu refetch ganda, dan `placeholderData: keepPreviousData` agar tabel tidak berkedip.

### 3.5 User task native: penempatan state dan modul (ADR-0033)

| Opsi | Kelebihan | Kekurangan |
|---|---|---|
| A. Modul `usertask` memiliki tabel `user_task` sendiri; `execution` memublikasikan `execution.user_task_created` (outbox durable Fase 3) dan `usertask` memproyeksikannya; `CompleteUserTask` memanggil `execution/api` untuk melanjutkan token | Kepemilikan tabel per modul bersih; `usertask` bisa dikembangkan tanpa menyentuh `execution` | Konsistensi eventual: task muncul beberapa ms–detik setelah token; dua sumber kebenaran untuk "task aktif" (element instance vs baris task); replay Fase 3 harus membangun ulang dua proyeksi; complete butuh dua transaksi (risiko task `COMPLETED` tetapi token gagal lanjut) |
| **B. Tabel `user_task` dan record `USER_TASK *` milik `execution`** (seperti `job`); modul `usertask` adalah lapisan Tasklist (kebijakan, pencarian, form) tanpa tabel | Atomik dengan token dan replayable; satu sumber kebenaran; pola persis Fase 2/4 (`job` di `execution`, modul `job` = protokol); `CancelProcessInstance` membatalkan task dalam transaksi yang sama | `execution` bertambah satu value type; aturan assign/unassign sebagai command `Processor` berlock instance meski tidak menggerakkan token |
| C. User task native = job-based + metadata di modul `usertask` | Nol perubahan di `execution` | Tasklist harus `ActivateJobs` dengan lease — semantik lease (timeout, reactivation) tidak cocok untuk tugas manusia yang terbuka berhari-hari |

**Dipilih B.** Alasan tambahan: assignment harus **durable** (bertahan restart dan replay) sehingga
harus menjadi record, bukan lease; dan tugas manusia tidak memerlukan `SKIP LOCKED` karena
operasi per task jarang dan konflik ditangani lewat `FailedPrecondition`. Lock instance pada
`AssignUserTask` diterima (operasi manusia, bukan jalur panas).

Perilaku saat aktivasi `userTask` dengan `zeebe:userTask` di `execution/domain/behavior_user_task.go`:

1. Evaluasi `zeebe:assignmentDefinition` (`assignee`, `candidateGroups`, `candidateUsers`),
   `zeebe:taskSchedule` (`dueDate`, `followUpDate`), `zeebe:formDefinition` (`formId` → dicari
   versi terbaru di `deployment/api.Forms`, `formKey`/`externalReference` disalin), dan
   `zeebe:priorityDefinition` (bila sudah diparse Fase 1, default 50 — verifikasi default Camunda
   8.8). Ekspresi dievaluasi dengan subset FEEL Fase 2 (Fase 7 memperluas); gagal → incident
   `EXTRACT_VALUE_ERROR` pada elemen `ACTIVATING` (langkah ulang 6.1 Fase 4 berlaku).
2. Record `USER_TASK CREATED{userTaskKey, elementInstanceKey, assignee, candidateGroups,
   candidateUsers, dueDate, followUpDate, formKey, formId, externalFormReference, priority}`; bila
   `assignee` terisi, record tambahan `USER_TASK ASSIGNED{assignee}` (mengikuti Zeebe yang memisahkan
   `CREATED` dan `ASSIGNED` — verifikasi intent Camunda 8.8).
3. Element instance `ACTIVATED` (wait state). `CompleteUserTask(variables)` → `VARIABLE *`,
   `USER_TASK COMPLETED`, `COMPLETING → COMPLETED`, token lanjut. `CancelProcessInstance` →
   `USER_TASK CANCELED`.
4. Event pasca-commit `execution.user_task_created/assigned/completed/canceled` (sinyal
   in-process + outbox durable Fase 3 bila ada) — Tasklist Fase 6 memakai polling, event
   disiapkan untuk SSE Fase 8 dan task listener.

Transisi state (tabel uji `execution/domain`):

```
                 Assign(assignee)                  Complete
  CREATED ──────────────────────────► ASSIGNED ───────────────► COMPLETED
    ▲  │                                 │  ▲
    │  │ Complete (tanpa assignee;       │  │ Assign(assignee lain, allow_override=true)
    │  │ diizinkan, verifikasi Zeebe)    │  └──────────────────────────────────┘
    │  ▼                                 │
    │  COMPLETED        Unassign         │
    └────────────────────────────────────┘
  Update(candidates, due, followUp, priority): CREATED/ASSIGNED → state tetap, record USER_TASK UPDATED
  CancelProcessInstance dari CREATED/ASSIGNED ──► CANCELED
```

Penolakan: `Assign` pada `ASSIGNED` tanpa `allow_override` → `FailedPrecondition`
`USER_TASK_ALREADY_ASSIGNED` (`metadata.assignee`); `Complete`/`Assign`/`Update` pada final →
`FailedPrecondition` `USER_TASK_NOT_ACTIVE`; `Unassign` pada `CREATED` → sukses idempoten.
Kebijakan "siapa boleh claim" (candidate cocok dengan `Principal`) tinggal di `usertask/domain`,
bukan di `execution`, dan hanya ditegakkan bila `Principal` bukan anonim (Fase 6: dev identity;
Fase 8: OIDC); operator Operate memakai `allow_override`.

### 3.6 Form: resource `.form` dan rendering (ADR-0034)

| Opsi | Kelebihan | Kekurangan |
|---|---|---|
| **A. Camunda Forms JSON sebagai resource deploy (`.form`), render form-js viewer** | Format sama dengan Camunda Modeler dan Tasklist; `formId` sudah diparse Fase 1; validasi dan tipe field bawaan form-js | Dependensi form-js (lisensi diverifikasi); skema versi form-js harus dikunci |
| B. Form sendiri (JSON Schema + react-jsonschema-form atau form dari Zod) | Lisensi bebas | Tidak kompatibel dengan Modeler/Camunda; harus menulis renderer; user task Camunda yang ada tidak bisa dipakai |
| C. Tanpa form; selalu editor variabel JSON | Nol dependensi | Tasklist tidak layak dipakai pengguna bisnis |

**Dipilih A**, dengan fallback C saat form tidak ada (formId tidak dikenal, `formKey` embedded lama,
atau `externalReference`). Aturan:

- `deployment`: resource dengan ekstensi `.form` diparse sebagai JSON; wajib object dengan
  `id` (string, = `form_id`), `components` (array), `type: "default"`, `schemaVersion` (int ≤
  `FORM_MAX_SCHEMA_VERSION`, dikunci pada versi yang didukung form-js yang dipasang; verifikasi
  nomor). Versi per `form_id` naik seperti process (`version` mulai 1), dedup checksum terhadap
  versi terbaru, tabel `form` (8.4). Deploy BPMN yang merujuk `formId` tak dikenal **saat itu**
  (termasuk dalam deployment yang sama, resolusi setelah semua resource diproses) → warning
  `FORM_NOT_FOUND` (kode issue baru di validator modul `deployment`, bukan parser `libs/go/bpmn`);
  pemilihan versi form dilakukan saat **aktivasi task** (versi terbaru; verifikasi semantik
  Camunda 8.8: binding `latest` default; `versionTag`/`deployment` binding ditunda).
- `usertask`: `GetUserTaskForm(user_task_key)` mengembalikan `{form_key, form_id, version,
  schema}` atau `NotFound` `FORM_NOT_FOUND` (Tasklist fallback JSON editor); `formKey` dengan
  prefix `camunda-forms:bpmn:` (embedded form di XML, Camunda 7/8 lama) **tidak didukung** →
  fallback; `externalReference` dikembalikan apa adanya untuk ditampilkan sebagai tautan.
- UI: `libs/web/form-renderer` (tag `type:ui`) membungkus `@bpmn-io/form-js` viewer:
  `FormRenderer({ schema, data, onSubmit, onChange, readOnly })`; CSS form-js di-import di lib;
  nilai awal = variabel instance yang namanya cocok dengan `key` komponen; hasil submit = object
  `data` form-js → variabel (`formDataToVariables` di `feature-user-task/domain`, memetakan
  tanggal ke ISO string, number tetap number). Modeler memakai lib yang sama untuk pratinjau form
  dari file `.form` yang dibuka (read-only).

## 4. Fondasi bersama `libs/web`

### 4.1 `libs/web/ui`: komponen yang ditambahkan

Semua lewat `pnpm dlx shadcn@latest add <nama>` ke `libs/web/ui/src/components/ui` (alias di
`components.json` root), lalu di-export dari `index.ts`. Daftar lengkap ada di 10.2; yang wajib
sebelum fitur pertama ditulis: `dialog`, `alert-dialog`, `sheet`, `dropdown-menu`, `select`,
`tabs`, `tooltip`, `sonner` (toast), `skeleton`, `separator`, `scroll-area`, `textarea`,
`checkbox`, `label`, `switch`, `popover`, `command`, `resizable`, `breadcrumb`, `pagination`,
`alert`, `avatar`, `calendar` + `date-picker` (komposisi), `form` (pembungkus field untuk TanStack
Form: `FieldLabel`, `FieldError`, `FieldDescription`). Komponen domain-netral milik repo (bukan
shadcn): `KeyValueTable`, `JsonEditor` (textarea monospace + validasi + format; CodeMirror ditunda
ke Fase 7 bersama editor FEEL), `StateBadge` (pemetaan union state → varian), `RelativeTime`,
`CopyButton`, `DataTable` (pembungkus TanStack Table dengan kolom, urut, paginasi keyset
`Previous/Next` seperti `DefinitionsTable` saat ini).

### 4.2 `libs/web/core`

- `format.ts`: `formatDuration(ms)`, `formatRelative(date)`, `formatKey(bigint | string)`.
- `json.ts`: `parseJsonObject(text): Result<Record<string, unknown>>` (dipakai Zod schema
  `jsonObjectSchema`), `stableStringify`.
- `i18n.ts`: `defineMessages<T>(m: T): T` dan `t(message, params)` dengan interpolasi `{name}` —
  tanpa library (10.3).
- `poll.ts`: `pollingInterval(kind, state)` membaca `import.meta.env.VITE_POLL_*` dengan default
  3.4 — dipakai lib fitur agar interval konsisten lintas app.

### 4.3 `libs/web/api-client`

- `transport.ts`: `createEngineTransport(baseUrl, { interceptors })`; `createUserTaskClient`;
  interceptor `devIdentity(getIdentity)` yang menambah header `X-Dev-User`/`X-Dev-Groups` (hanya
  bila `VITE_DEV_IDENTITY=true`); hook `getToken` untuk Fase 8.
- `index.ts` meng-export `gen/workflow/usertask/v1/*_pb`, dan (bila ada) `timer/v1`.
- Tetap: `int64` = `bigint`; lib fitur mengubahnya ke `string` di mapper.

### 4.4 `libs/web/bpmn-viewer`, `libs/web/flow-canvas`, `libs/web/form-renderer`

- `bpmn-viewer`: perluasan 3.3; CSS marker token di `bpmn-canvas.css`:
  `token-active` (stroke `--color-primary` tebal + fill lembut), `token-completed` (stroke abu,
  opacity), `token-incident` (stroke `--color-destructive`); overlay badge memakai class Tailwind
  (`rounded-full bg-primary text-primary-foreground text-xs px-1.5`) yang dipindai `@source`
  di `globals.css`. Kanvas tetap latar putih di dark mode (warna bawaan bpmn-js dirancang untuk
  putih; tema gelap kanvas = pertanyaan 16.2 #10). Modul properties panel, minimap, moddle `zeebe`
  dimuat lewat `import()` dinamis di dalam lib agar viewer Operate tidak membundelnya.
- `flow-canvas`: `TreeNode` (judul = nama/id elemen, badge state, jumlah anak, ikon tipe elemen),
  `layoutGraph` mendukung `direction: 'TB'` (sudah) dan `rankSep` per opsi; prop `onNodeClick`,
  `selectedNodeId`; `readOnly` menonaktifkan drag/connect (sudah). Atribusi React Flow:
  `hideAttribution: true` saat ini — **verifikasi** ketentuan React Flow (MIT, atribusi diminta
  untuk pemakaian non-Pro); bila diwajibkan, tampilkan atribusi (risiko 15).
- `form-renderer`: 3.6; lazy di aplikasi (`React.lazy(() => import('@workflow/form-renderer'))`).

### 4.5 `libs/web/app-shell`

Komponen 3.2. `AppLayout` menerima `nav: Array<{ to, label, icon }>`, `appName`, `links`
(tautan silang antar app dari env `VITE_MODELER_URL`, `VITE_OPERATE_URL`, `VITE_TASKLIST_URL`;
kosong = tautan disembunyikan). `ThemeProvider` menulis class `dark` ke `<html>` (cocok dengan
`@custom-variant dark` yang ada). `RouteErrorBoundary` menampilkan `ErrorInfo.reason` bila error
berasal dari Connect (lewat `findErrorInfo` yang dipanggil **di lapisan infrastructure** fitur dan
dipetakan ke `AppError{code, message}` dari `libs/web/core`; `app-shell` tidak mengenal Connect).
`createAppQueryClient` memasang `QueryCache.onError` → toast kecuali error ditandai
`meta.silent`, `retry` 1 untuk query, 0 untuk mutation.

## 5. Modeler (`apps/modeler-web`)

### 5.1 Halaman dan komponen

| Komponen | Lapisan | Perilaku |
|---|---|---|
| `ModelerPage` | presentation (fitur `modeler`) | Tiga panel `Resizable`: project explorer (kiri, bisa dilipat), kanvas + toolbar (tengah), properties panel (kanan, 320 px, bisa dilipat); panel bawah bertab: Issues (`IssuesPanel` dari `feature-deployment`), XML |
| `ModelerToolbar` | presentation | New, Open (file), Save (File System Access bila tersedia, else download), Download, Undo/Redo, Zoom in/out/fit, Validate/Deploy (`DeployToolbar` yang sudah ada, `resourceName` = nama diagram), Start instance (setelah deploy sukses, membuka `StartInstanceDialog` dengan definition hasil deploy), Shortcuts help |
| `ProjectExplorer` | presentation | Daftar diagram draft (nama, diubah kapan, dirty) + "Deployed" (definition terbaru dari `feature-deployment`, "Open as copy" memuat XML ke draft baru); rename/delete dengan `AlertDialog` |
| `PropertiesPanelHost` | presentation | `div` kontainer yang diserahkan ke `BpmnCanvas` lewat `propertiesPanelParent`; CSS properties panel diadaptasi ke token tema (override variabel `--properties-panel-*`? verifikasi nama variabel CSS yang disediakan `@bpmn-io/properties-panel`) |
| `XmlView` | presentation | Textarea monospace read-only dengan "Edit" → mode edit; "Apply" memanggil `importXML` (gagal → pesan error bpmn-js, draft tidak diubah); "Copy" |
| `FormPreviewDialog` | presentation | Membuka `.form` lokal dan merender `FormRenderer` read-only (pratinjau; editor form ditunda) |
| `ShortcutsHelpDialog` | presentation | Tabel pintasan dari `domain/shortcuts.ts` |

### 5.2 Diagram, draft, dan file

- `domain/diagram.ts`: `Diagram{id: string (nanoid), name, xml, updatedAt, source: 'new' | 'file' |
  'definition', fileHandle?: unknown}`; `domain/draft.ts`: `shouldPersist(diagram, prev)`,
  `MAX_DRAFT_BYTES` (2 MiB per diagram, 20 diagram; di atas itu draft tertua tidak lagi
  disimpan dan pengguna diberi tahu), `draftKey(id) = 'modeler.draft.' + id`.
- `application/modeler.store.ts` (Zustand, memori): `activeDiagramId`, `dirty`, `selection`,
  `panels{explorer, properties, bottomTab}`; `application/drafts.store.ts` (Zustand +
  `persist` middleware ke localStorage, versi skema `1`, migrasi bila berubah): `diagrams:
  Record<id, Diagram tanpa fileHandle>`. Sesuai aturan localStorage: setiap akses dibungkus
  try/catch; bila storage tidak tersedia (private window, kuota) modeler tetap berjalan tanpa
  draft dan menampilkan banner.
- Autosave: `onChanged` kanvas (debounce 250 ms) → `dirty = true`; debounce 1 s → `saveXML()` →
  `drafts.store.update(id, xml)`. Reload: bila ada draft untuk `activeDiagramId` terakhir dan
  tidak ada file handle, dialog "Restore draft from <waktu>?".
- `infrastructure/file-system.ts`: `openBpmnFile(): Promise<{name, xml, handle?}>` memakai
  `window.showOpenFilePicker` bila ada (Chromium) dengan tipe `.bpmn`/`.xml`/`.form`; fallback
  `<input type="file">`; `saveBpmnFile(diagram)` → `handle.createWritable()` bila ada handle,
  else download `Blob`. Drag-drop file ke kanvas memanggil jalur yang sama.
- Pintasan (`domain/shortcuts.ts`, dipasang di `ModelerPage` dengan `useEffect` keydown; bpmn-js
  memiliki binding keyboard bawaan untuk undo/redo/hapus/zoom bila `keyboard.bindTo` dikonfigurasi
  — **verifikasi API keyboard pada bpmn-js 18**, yang mengubah binding implisit): `Ctrl+S` simpan,
  `Ctrl+O` buka, `Ctrl+Shift+D` deploy, `Ctrl+Shift+V` validate, `Ctrl+F` cari elemen
  (`Command` palette memakai `findElements`), `?` bantuan pintasan.
- Minimap: ekstensi `'minimap'` aktif di mode modeler, toggle di toolbar, preferensi di localStorage.

### 5.3 Dari Modeler ke instance

- `DefinitionsTable` (lib) menerima prop `renderActions(definition)`; Modeler memasang "Start"
  (membuka `StartInstanceDialog` dari `feature-process-instance`) dan "Open as copy".
- `/definitions/$key` menampilkan viewer (sudah ada) + ringkasan statistik (`CountProcessInstances`
  per definition, 9.2) + tautan "Open in Operate" (`VITE_OPERATE_URL` + `/definitions/<key>`) bila
  env tersedia.
- Modeler **tidak** memiliki daftar instance; itu milik Operate. Dialog start instance ada di
  Modeler sebagai alat uji cepat untuk analis.

## 6. Operate (`apps/operate-web`)

### 6.1 Halaman

| Route | Isi | Sumber data |
|---|---|---|
| `/` dashboard | Kartu: instance `ACTIVE`, dengan incident, selesai 24 jam terakhir (bila `CountProcessInstances` mendukung `since`), incident terbuka per `error_type`, 10 definition terbaru dengan jumlah instance berjalan | `CountProcessInstances` (9.2), `ListIncidents`, `ListProcessDefinitions` |
| `/instances` | `InstanceFilters` (process id autocomplete dari definitions, versi, state multi, hanya incident, rentang waktu mulai bila API mendukung) + `InstancesTable` (key, process, versi, state badge, incident ikon, mulai, selesai, durasi) + paginasi keyset; filter di URL search params (TanStack Router `validateSearch` dengan Zod) | `ListProcessInstances` |
| `/instances/$key` | Header (key, process, versi, state, mulai/selesai, tombol Cancel); `InstanceViewer` (bpmn-js viewer + marker + overlay); sidebar kanan `InstanceTree`; panel bawah bertab: Variables, Incidents, Jobs, User tasks, Timers, Subscriptions, Records; klik elemen di viewer memfilter panel ke elemen itu (`?element=`) | `GetProcessInstance`, `GetProcessDefinition` (XML, di-cache tak berbatas per key), `ListIncidents`, `ListJobs`, `SearchUserTasks`, timer/subscription (9.4), `ListProcessInstanceRecords` |
| `/definitions` | Tabel definition (lib) + kolom jumlah instance `ACTIVE`/incident per definition (`CountProcessInstances` group by definition) + tombol Start | `ListProcessDefinitions`, `CountProcessInstances` |
| `/definitions/$key` | Viewer dengan `DefinitionStatisticsOverlay` (badge per elemen: aktif, selesai, incident, dibatalkan) + tabel instance definition itu (filter dikunci) | `GetProcessDefinitionStatistics` (9.2), `ListProcessInstances` |
| `/incidents` | Tabel incident terbuka lintas instance dengan filter `error_type`, process; aksi resolve massal (satu per satu di klien, laporan sukses/gagal) | `ListIncidents` |

### 6.2 Marker dan overlay token (`feature-process-instance/domain/markers.ts`)

```ts
export type MarkerKind = 'active' | 'completed' | 'incident';
export interface MarkerSet { active: string[]; completed: string[]; incident: string[]; counts: Record<string, { active: number; completed: number; incidents: number }> }

/** markersFor menghitung marker per element_id dari element instance (termasuk yang final) dan incident terbuka. */
export function markersFor(elements: ElementInstance[], incidents: Incident[]): MarkerSet
```

Aturan: elemen dengan ≥ 1 element instance non-final (`ACTIVATING|ACTIVATED|COMPLETING|TERMINATING`)
→ `active`; elemen yang semua instance-nya final dan ≥ 1 `COMPLETED` → `completed`; elemen yang
memiliki incident `CREATED` → `incident` (menang atas `active`); `PROCESS` dan sub-process scope
tidak diberi marker (hanya badge). Sequence flow yang telah diambil (`SEQUENCE_FLOW_TAKEN` dari
records, atau `taken_sequence_flow` untuk join yang menunggu) diberi marker `completed` bila
records halaman pertama tersedia — opsional, dicatat sebagai peningkatan. Badge: `active > 1` atau
`completed > 1` (loop) menampilkan angka; incident menampilkan ikon. `ElementInstance[]` dari
`GetProcessInstance` (≤ 1.000; bila `element_instances_truncated` Fase 3, panel memakai
`ListElementInstances(only_active=true)` dan marker `completed` dinonaktifkan dengan catatan).

`InstanceViewer` memanggil `setMarkers` untuk tiga kelas dan `setOverlays('tokens', …)` setiap
data berubah (memo pada `MarkerSet` agar tidak menggambar ulang bila sama). Elemen diklik →
`instance-view.store.selectedElementId` → panel memfilter; `selectElement` dipanggil saat node pohon
diklik.

### 6.3 Pohon instance dan statistik (React Flow)

`buildInstanceTree(elements)`: node untuk setiap element instance bertipe scope (`PROCESS`,
`SUB_PROCESS`, `EVENT_SUB_PROCESS`, `CALL_ACTIVITY`; Fase 5) dan untuk setiap element instance
**aktif** non-scope; anak dikaitkan lewat `flow_scope_key`; elemen final non-scope diringkas per
`element_id` sebagai satu node "× n completed" (dapat diperluas). Bila `ProcessInstance` memiliki
`parent_process_instance_key`/`parent_element_instance_key` (call activity, bila disediakan Fase
5/7), node akar menampilkan tautan ke induk dan node `CALL_ACTIVITY` menautkan ke instance anak
(`ListProcessInstances(parent_process_instance_key=…)` bila filter ada; jika tidak, tautan
disembunyikan). Layout `TB`, `readOnly`, `onNodeClick` → pilih elemen. Statistik definition
(`GetProcessDefinitionStatistics`) digambar sebagai overlay di viewer, bukan di React Flow.

### 6.4 Aksi operator

| Aksi | RPC | UI |
|---|---|---|
| Set variables | `SetVariables(scope_key = process_instance_key, variables, local=false)` | `VariableEditorDialog`: editor JSON satu variabel (nama + nilai) atau "Edit all" (object); validasi Zod `jsonObjectSchema`; diff ditampilkan sebelum simpan; sukses → invalidasi detail |
| Resolve incident | `ResolveIncident` | `IncidentsPanel` tombol per incident; untuk `JOB_NO_RETRIES` dialog "Retries" (default 3) → `UpdateJobRetries` lalu `ResolveIncident` (`useRetryJob`); untuk `CONDITION_ERROR`/`EXTRACT_VALUE_ERROR` tombol membuka `VariableEditorDialog` dulu dengan petunjuk; `UNHANDLED_ERROR_EVENT` hanya "Cancel instance" sampai Fase 5 menyediakan catch |
| Cancel instance | `CancelProcessInstance` | `AlertDialog` konfirmasi; setelah sukses polling berhenti (state final) |
| Complete job (debug) | `CompleteJob` | `JobsPanel` tombol "Complete…" dengan editor variabel; disembunyikan bila `VITE_OPERATE_DEBUG_ACTIONS` tidak `true` |
| Assign/complete user task (admin) | `AssignUserTask(allow_override=true)`, `CompleteUserTask` | Panel "User tasks" memakai komponen dari `feature-user-task` |
| Start instance | `CreateProcessInstance` | `StartInstanceDialog` (lib): pilih definition (autocomplete) + versi + JSON variabel; sukses → navigasi ke `/instances/$key` |

### 6.5 Records (riwayat)

`RecordsPanel` menampilkan `ListProcessInstanceRecords` (Fase 3) berpaginasi: `sequence`,
`value_type`, `intent`, `element_id`, waktu, payload (lipat). Filter per elemen terpilih. Ini
tampilan riwayat **sederhana**; timeline visual dan diff variabel per record = peningkatan
setelah Fase 6.

### 6.6 Polling dan beban

Interval 3.4. Halaman detail instance memakai **satu** query `GetProcessInstance` (instance,
element instances, variabel, incident, job) + query panel yang hanya aktif saat tab-nya terbuka
(`enabled: tab === 'records'`). Definition XML di-cache dengan `staleTime: Infinity` (definition
immutable). Dengan 20 tab Operate terbuka pada instance aktif: 20 RPC/s — diterima; nginx tidak
perlu konfigurasi khusus. `Document.visibilityState` `hidden` menghentikan polling (opsi TanStack
Query); kembali ke tab → refetch segera.

## 7. Tasklist (`apps/tasklist-web`)

### 7.1 Halaman

| Route | Isi |
|---|---|
| `/tasks` | Layout master–detail (≥ `lg`: daftar kiri 380 px + detail kanan; < `lg`: daftar saja, detail di `/tasks/$key` layar penuh). `TaskFilters`: tab "Assigned to me", "Claimable by me", "All open", "Completed"; filter proses; urut `created_at` / `due_date`; pencarian nama task. `TaskList`: `TaskRow` (nama elemen, proses, key instance, assignee/candidate badge, due relatif dengan warna lewat/dekat, prioritas). Pintasan `j`/`k` berpindah, `Enter` membuka, `c` claim |
| `/tasks/$key` | `TaskDetail`: header (nama, proses, dibuat, due, follow-up, prioritas, assignee), aksi (Claim bila `canClaim`, Unclaim bila milik saya, Assign to… bila `allow_override` diizinkan untuk peran operator — Fase 6: selalu tampil dengan konfirmasi), `TaskForm` (form-js atau fallback JSON), tombol Complete (dinonaktifkan bila task bukan milik saya **dan** memiliki assignee lain), tautan "View in Operate" |
| `/processes` | Definition yang bisa dimulai (semua definition terbaru; Fase 8 menyaring per otorisasi) + `StartInstanceDialog` dengan form-js bila proses memiliki **start form** (`zeebe:formDefinition` pada none start event, Fase 1 parse; verifikasi) — bila tidak, editor JSON |

### 7.2 Identitas dan kebijakan klaim

- `useAuth().principal` = `{ subject, groups }`; Fase 6: `subject = 'anonymous'` kecuali
  `VITE_DEV_IDENTITY=true` menampilkan `DevIdentityPicker` (nama + grup, disimpan localStorage,
  dikirim header `X-Dev-User`/`X-Dev-Groups`; engine hanya mempercayainya bila
  `ENGINE_DEV_IDENTITY=true`, default false; Fase 8 menghapusnya).
- `canClaim(principal, task)` di `feature-user-task/domain`: task `CREATED` dan (tanpa candidate,
  atau `subject ∈ candidateUsers`, atau `groups ∩ candidateGroups ≠ ∅`); anonim → `true` (tidak ada
  identitas untuk ditegakkan, konsisten dengan server). Server menegakkan aturan yang sama di
  `usertask/domain` (sumber kebenaran; UI hanya menyembunyikan tombol).
- Filter "Assigned to me" = `assignee = subject`; "Claimable by me" = `state = CREATED` +
  `candidate_user = subject` **atau** `candidate_groups ∩ groups` **atau** tanpa candidate — satu
  request `SearchUserTasks{ claimable_by: { user, groups } }` (9.1) agar paginasi benar.

### 7.3 Form dan penyelesaian

- `taskFormQuery(key)` → `GetUserTaskForm`; `NotFound FORM_NOT_FOUND` → `data = null` (bukan error)
  → fallback `JsonVariablesForm` (editor object JSON, prefilled variabel instance).
- Nilai awal form = `variablesToFormData(task.variables, schema)` (hanya key yang ada di skema);
  draft isian disimpan `sessionStorage` per task key (per-viewer, hilang saat tab ditutup) agar
  berpindah task tidak kehilangan isian.
- Submit: validasi form-js (wajib, pola) → `CompleteUserTask{ user_task_key, variables:
  JSON(formDataToVariables(data)) }` → toast, invalidasi daftar, pilih task berikutnya di daftar
  (pola inbox). `FailedPrecondition USER_TASK_NOT_ACTIVE` (diselesaikan orang lain) → pesan khusus
  + refetch.
- Variabel yang bukan bagian form tidak dikirim (tidak menimpa). "Save without completing"
  (menyimpan variabel tanpa menyelesaikan, ala Tasklist Camunda) ditunda: pertanyaan 16.2 #7.

## 8. Engine: user task native dan form

### 8.1 Tata letak modul `usertask` dan perluasan

```
apps/engine/internal/modules/usertask/
├─ api/
│  └─ api.go                 # (tanpa konsumen modul lain di Fase 6) — dipertahankan demi pola lima paket
├─ domain/
│  ├─ principal.go           # Principal{Subject, Groups} (salinan nilai dari authn, tanpa import platform)
│  ├─ policy.go              # CanClaim(p, task), CanComplete(p, task), CanAssign(p, task, allowOverride)
│  ├─ filter.go              # SearchFilter{State, Assignee, ClaimableBy, CandidateGroup, CandidateUser, BPMNProcessID,
│  │                         #   ProcessInstanceKey, ElementID, DueBefore, DueAfter, Sort}, Normalize()
│  ├─ form.go                # FormRef{Key, ID, Version}; ResolveFormRef rules (formId > formKey > external)
│  └─ errors.go              # ErrNotClaimable, ErrNotOwner, ErrFormNotFound
├─ application/
│  ├─ assign_user_task.go    # claim/assign: policy → execution/api.UserTasks.Assign
│  ├─ unassign_user_task.go
│  ├─ update_user_task.go
│  ├─ complete_user_task.go  # policy → execution/api.UserTasks.Complete
│  ├─ get_user_task.go       # + variabel instance (execution/api.Variables.List) bila diminta
│  ├─ search_user_tasks.go   # filter → execution/api.UserTasks.Search
│  └─ get_user_task_form.go  # task → deployment/api.Forms.Get(formKey) → schema
├─ infrastructure/
│  └─ form_cache.go          # cache LRU schema form per key (immutable)
├─ interfaces/
│  └─ user_task_handler.go   # UserTaskService (workflow/usertask/v1)
└─ README.md

apps/engine/internal/modules/execution/            # perluasan Fase 6
├─ api/api.go                # UserTasks{Assign, Unassign, Update, Complete, Get, Search}; Variables += List(scopeKey)
├─ domain/behavior_user_task.go   # aktivasi userTask native (3.5); job-based Fase 4 tetap di behavior_task.go
├─ domain/user_task.go       # UserTask state, transisi (tabel), record USER_TASK CREATED/ASSIGNED/UNASSIGNED/UPDATED/COMPLETED/CANCELED
├─ domain/command.go         # + AssignUserTask, UnassignUserTask, UpdateUserTask, CompleteUserTask
├─ application/              # + assign_user_task.go, unassign_user_task.go, update_user_task.go, complete_user_task.go, search_user_tasks.go
├─ infrastructure/           # + queries/user_task.sql, proyektor USER_TASK → tabel user_task
└─ interfaces/               # tidak berubah (handler UserTaskService di modul usertask)

apps/engine/internal/modules/deployment/            # perluasan Fase 6
├─ api/api.go                # Forms{Get(key), GetLatest(formID), List}; event deployment.form_deployed
├─ domain/form.go            # Form{Key, FormID, Version, ResourceName, Schema, Checksum}; ValidateFormSchema
├─ application/deploy_resource.go  # .form diterima; resolusi formId lintas resource; warning FORM_NOT_FOUND
├─ infrastructure/           # queries/form.sql; repository
└─ interfaces/               # DeploymentService += GetForm, ListForms

apps/engine/internal/platform/rpc/authn/           # perluasan: DevIdentity (header X-Dev-User/X-Dev-Groups) bila ENGINE_DEV_IDENTITY=true
```

`usertask` meng-import hanya `execution/api` dan `deployment/api` (ditegakkan `archtest`);
`execution` **tidak** meng-import `usertask`. `authn.Principal` (Fase 4) ditambah field `Groups
[]string` (diisi dev identity sekarang, klaim OIDC Fase 8); `usertask/application` membacanya
lewat `authn.FromContext` dan menyalin ke `domain.Principal`.

### 8.2 Migrasi `0000N_user_task_form.sql` (nomor mengikuti urutan saat merge)

```sql
-- +goose Up
CREATE TABLE user_task (
    key                         BIGINT PRIMARY KEY,
    tenant_id                   TEXT NOT NULL DEFAULT '<default>',
    process_instance_key        BIGINT NOT NULL REFERENCES process_instance (key),
    element_instance_key        BIGINT NOT NULL,
    element_id                  TEXT NOT NULL,
    element_name                TEXT NOT NULL DEFAULT '',     -- nama BPMN untuk inbox (disalin saat aktivasi)
    bpmn_process_id             TEXT NOT NULL,
    process_definition_key      BIGINT NOT NULL,
    process_definition_version  INT NOT NULL,
    state                       TEXT NOT NULL CHECK (state IN ('CREATED', 'ASSIGNED', 'COMPLETED', 'CANCELED')),
    assignee                    TEXT,
    candidate_groups            JSONB NOT NULL DEFAULT '[]',   -- array string
    candidate_users             JSONB NOT NULL DEFAULT '[]',
    due_date                    TIMESTAMPTZ,
    follow_up_date              TIMESTAMPTZ,
    priority                    INT NOT NULL DEFAULT 50,
    form_key                    BIGINT,                         -- key tabel form bila formId terselesaikan saat aktivasi
    form_id                     TEXT,
    external_form_reference     TEXT,
    created_at                  TIMESTAMPTZ NOT NULL,
    updated_at                  TIMESTAMPTZ NOT NULL,
    completed_at                TIMESTAMPTZ,
    CONSTRAINT user_task_assignee_check CHECK ((state = 'ASSIGNED') = (assignee IS NOT NULL AND state <> 'CREATED') OR state IN ('COMPLETED', 'CANCELED'))
);
CREATE INDEX user_task_open_assignee_idx ON user_task (tenant_id, assignee, created_at DESC) WHERE state IN ('CREATED', 'ASSIGNED');
CREATE INDEX user_task_open_created_idx  ON user_task (tenant_id, created_at DESC, key DESC) WHERE state IN ('CREATED', 'ASSIGNED');
CREATE INDEX user_task_open_due_idx      ON user_task (tenant_id, due_date, key) WHERE state IN ('CREATED', 'ASSIGNED');
CREATE INDEX user_task_cand_groups_idx   ON user_task USING GIN (candidate_groups) WHERE state = 'CREATED';
CREATE INDEX user_task_cand_users_idx    ON user_task USING GIN (candidate_users)  WHERE state = 'CREATED';
CREATE INDEX user_task_by_pi_idx         ON user_task (process_instance_key, key);

CREATE TABLE form (
    key              BIGINT PRIMARY KEY,
    tenant_id        TEXT NOT NULL DEFAULT '<default>',
    form_id          TEXT NOT NULL,
    version          INT NOT NULL,
    resource_name    TEXT NOT NULL,
    schema           TEXT NOT NULL,                              -- JSON asli (teks; presisi dan urutan key)
    checksum         BYTEA NOT NULL,
    deployment_key   BIGINT NOT NULL REFERENCES deployment (key),
    created_at       TIMESTAMPTZ NOT NULL,
    UNIQUE (tenant_id, form_id, version)
);
CREATE INDEX form_latest_idx ON form (tenant_id, form_id, version DESC);

-- +goose Down
DROP TABLE form;
DROP TABLE user_task;
```

Catatan: constraint `user_task_assignee_check` disederhanakan menjadi dua aturan saat
implementasi (`state = 'ASSIGNED' → assignee NOT NULL`, `state = 'CREATED' → assignee IS NULL`);
ditulis di sini sebagai niat. `candidate_*` JSONB (bukan `TEXT[]`) agar GIN `?|` dipakai kueri
"claimable by". Replay Fase 3: proyektor `USER_TASK *` membangun ulang tabel dari record; `form`
bukan proyeksi (resource deployment).

### 8.3 Kueri pencarian

```sql
-- name: SearchUserTasks :many
SELECT * FROM user_task
WHERE tenant_id = $1
  AND ($2::text[] IS NULL OR state = ANY($2))
  AND ($3::text IS NULL OR assignee = $3)
  AND ($4::text IS NULL OR bpmn_process_id = $4)
  AND ($5::bigint = 0 OR process_instance_key = $5)
  AND ($6::text IS NULL OR element_id = $6)
  AND ($7::timestamptz IS NULL OR due_date < $7)
  AND ($8::timestamptz IS NULL OR due_date >= $8)
  AND (NOT $9::bool OR (                                   -- claimable_by
        state = 'CREATED' AND (
          (candidate_groups = '[]'::jsonb AND candidate_users = '[]'::jsonb)
          OR candidate_users ? $10::text
          OR candidate_groups ?| $11::text[]
        )))
  AND (sqlc.narg('after_key')::bigint IS NULL OR key < sqlc.narg('after_key'))   -- keyset; urutan due_date memakai (due_date, key)
ORDER BY created_at DESC, key DESC
LIMIT $12;
```

Urutan `due_date` memakai kueri kembar dengan `ORDER BY due_date NULLS LAST, key` dan token
keyset `{d, k}`. `EXPLAIN` harus memakai indeks parsial `user_task_open_*` untuk filter inbox
(dicek integration test seperti Fase 4 untuk `job_activatable_idx`).

### 8.4 Form di modul `deployment`

- `deploy_resource.go`: resource diurutkan `.form` dulu lalu `.bpmn` agar `formId` dalam
  deployment yang sama terselesaikan; `.dmn` tetap `UNSUPPORTED_RESOURCE_TYPE` (Fase 7).
- `ValidateFormSchema(json)`: object; `id` string non-kosong (= `form_id`; jika nama file dan `id`
  berbeda, `id` menang — verifikasi perilaku Camunda); `components` array; `schemaVersion` ≤
  batas; ukuran ≤ `DEPLOYMENT_MAX_RESOURCE_BYTES`. Pelanggaran → issue `FORM_INVALID` (error,
  menolak seluruh deployment seperti BPMN).
- Versi: `version = max(version) + 1` per `(tenant, form_id)`; checksum sama dengan versi terbaru
  → `duplicate = true`, versi tidak naik (pola Fase 1).
- `api.Forms.GetLatest(ctx, formID)` dipakai `execution` saat aktivasi user task; cache in-memory
  per `(formID)` diinvalidasi oleh event `deployment.form_deployed` (pola cache process Fase 1).

## 9. Kontrak proto

Semua perubahan **aditif**; `buf breaking --against main` hijau. Anotasi `buf.validate`
ditampilkan sebagian.

### 9.1 `workflow/usertask/v1/usertask.proto` (baru)

```proto
syntax = "proto3";
package workflow.usertask.v1;
option go_package = "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/usertask/v1;usertaskv1";

import "buf/validate/validate.proto";
import "google/protobuf/timestamp.proto";
import "workflow/common/v1/error.proto";

service UserTaskService {
  rpc GetUserTask(GetUserTaskRequest) returns (GetUserTaskResponse) {}
  rpc SearchUserTasks(SearchUserTasksRequest) returns (SearchUserTasksResponse) {}
  rpc AssignUserTask(AssignUserTaskRequest) returns (AssignUserTaskResponse) {}
  rpc UnassignUserTask(UnassignUserTaskRequest) returns (UnassignUserTaskResponse) {}
  rpc UpdateUserTask(UpdateUserTaskRequest) returns (UpdateUserTaskResponse) {}
  rpc CompleteUserTask(CompleteUserTaskRequest) returns (CompleteUserTaskResponse) {}
  rpc GetUserTaskForm(GetUserTaskFormRequest) returns (GetUserTaskFormResponse) {}
}

enum UserTaskState {
  USER_TASK_STATE_UNSPECIFIED = 0;
  USER_TASK_STATE_CREATED = 1;
  USER_TASK_STATE_ASSIGNED = 2;
  USER_TASK_STATE_COMPLETED = 3;
  USER_TASK_STATE_CANCELED = 4;
}

message UserTask {
  int64 user_task_key = 1;
  int64 process_instance_key = 2;
  int64 element_instance_key = 3;
  string element_id = 4;
  string element_name = 5;
  string bpmn_process_id = 6;
  int64 process_definition_key = 7;
  int32 process_definition_version = 8;
  UserTaskState state = 9;
  string assignee = 10;                               // kosong = belum di-assign
  repeated string candidate_groups = 11;
  repeated string candidate_users = 12;
  google.protobuf.Timestamp due_date = 13;
  google.protobuf.Timestamp follow_up_date = 14;
  int32 priority = 15;
  int64 form_key = 16;                                // 0 = tidak terselesaikan
  string form_id = 17;
  string external_form_reference = 18;
  google.protobuf.Timestamp created_at = 19;
  google.protobuf.Timestamp completed_at = 20;
  string tenant_id = 21;
}

message GetUserTaskRequest {
  int64 user_task_key = 1 [(buf.validate.field).int64.gt = 0];
  bool include_variables = 2;                         // variabel scope process instance
}
message GetUserTaskResponse {
  UserTask user_task = 1;
  string variables = 2;                               // JSON object; kosong bila tidak diminta
}

message ClaimableBy {                                 // "bisa diklaim oleh": tanpa candidate, atau user/grup cocok
  string user = 1;
  repeated string groups = 2;
}

message SearchUserTasksRequest {
  repeated UserTaskState states = 1;                  // kosong = CREATED + ASSIGNED
  string assignee = 2;
  ClaimableBy claimable_by = 3;
  string bpmn_process_id = 4;
  int64 process_instance_key = 5;
  string element_id = 6;
  google.protobuf.Timestamp due_before = 7;
  google.protobuf.Timestamp due_after = 8;
  UserTaskSort sort = 9;
  workflow.common.v1.PageRequest page = 10;
}
enum UserTaskSort {
  USER_TASK_SORT_UNSPECIFIED = 0;                     // = CREATED_AT_DESC
  USER_TASK_SORT_CREATED_AT_DESC = 1;
  USER_TASK_SORT_DUE_DATE_ASC = 2;
}
message SearchUserTasksResponse {
  repeated UserTask user_tasks = 1;
  workflow.common.v1.PageResponse page = 2;
}

message AssignUserTaskRequest {
  int64 user_task_key = 1 [(buf.validate.field).int64.gt = 0];
  string assignee = 2 [(buf.validate.field).string = {min_len: 1, max_len: 256}];  // claim = assignee diri sendiri
  bool allow_override = 3;                            // ganti assignee yang sudah ada
}
message AssignUserTaskResponse {}

message UnassignUserTaskRequest { int64 user_task_key = 1; }
message UnassignUserTaskResponse {}

message UpdateUserTaskRequest {                       // field yang tidak disebut di update_mask tidak diubah
  int64 user_task_key = 1;
  repeated string candidate_groups = 2;
  repeated string candidate_users = 3;
  google.protobuf.Timestamp due_date = 4;
  google.protobuf.Timestamp follow_up_date = 5;
  int32 priority = 6 [(buf.validate.field).int32 = {gte: 0, lte: 100}];
  repeated string update_mask = 7 [(buf.validate.field).repeated.items.string = {in: ["candidate_groups", "candidate_users", "due_date", "follow_up_date", "priority"]}];
}
message UpdateUserTaskResponse {}

message CompleteUserTaskRequest {
  int64 user_task_key = 1;
  string variables = 2;                               // JSON object; kosong = {}
  string request_id = 3;                              // idempotensi Fase 3 (opsional)
}
message CompleteUserTaskResponse {}

message GetUserTaskFormRequest { int64 user_task_key = 1; }
message GetUserTaskFormResponse {
  int64 form_key = 1;
  string form_id = 2;
  int32 version = 3;
  string schema = 4;                                  // Camunda Forms JSON
}
```

`update_mask` sebagai `repeated string` (bukan `google.protobuf.FieldMask`) agar tidak menarik
tipe well-known tambahan ke TS; protovalidate membatasi nilainya.

### 9.2 Perluasan `workflow/process/v1` dan `workflow/deployment/v1`

```proto
// process_instance.proto — tambahan
service ProcessInstanceService {
  // ...Fase 2–4
  rpc GetProcessInstanceStatistics(GetProcessInstanceStatisticsRequest) returns (GetProcessInstanceStatisticsResponse) {}
  rpc GetProcessDefinitionStatistics(GetProcessDefinitionStatisticsRequest) returns (GetProcessDefinitionStatisticsResponse) {}
  rpc CountProcessInstances(CountProcessInstancesRequest) returns (CountProcessInstancesResponse) {}
}

message FlowNodeStatistics {
  string element_id = 1;
  int64 active = 2;        // element instance non-final
  int64 completed = 3;
  int64 terminated = 4;
  int64 incidents = 5;     // incident CREATED pada elemen ini
}
message GetProcessInstanceStatisticsRequest { int64 process_instance_key = 1; }
message GetProcessInstanceStatisticsResponse { repeated FlowNodeStatistics statistics = 1; }

message GetProcessDefinitionStatisticsRequest {
  int64 process_definition_key = 1;
  bool only_active_instances = 2;     // true: hanya instance ACTIVE (Operate default)
}
message GetProcessDefinitionStatisticsResponse { repeated FlowNodeStatistics statistics = 1; }

message CountProcessInstancesRequest {
  int64 process_definition_key = 1;   // 0 = semua, dikelompokkan per definition
  string bpmn_process_id = 2;
  google.protobuf.Timestamp started_after = 3;
}
message ProcessInstanceCount {
  int64 process_definition_key = 1;
  string bpmn_process_id = 2;
  int32 version = 3;
  int64 active = 4;
  int64 completed = 5;
  int64 terminated = 6;
  int64 with_incident = 7;
}
message CountProcessInstancesResponse { repeated ProcessInstanceCount counts = 1; }

// ProcessInstance — tambahan (diisi bila call activity tersedia, Fase 5/7; 0 = akar)
message ProcessInstance {
  // ...field 1–8
  int64 parent_process_instance_key = 9;
  int64 parent_element_instance_key = 10;
}
message ListProcessInstancesRequest {
  // ...field 1–6
  int64 parent_process_instance_key = 7;   // 0 = tanpa filter
  int32 version = 8;                       // 0 = semua versi (bersama bpmn_process_id)
}
```

```proto
// deployment.proto — tambahan
service DeploymentService {
  // ...Fase 1
  rpc GetForm(GetFormRequest) returns (GetFormResponse) {}
  rpc ListForms(ListFormsRequest) returns (ListFormsResponse) {}
}
message Form {
  int64 form_key = 1;
  string form_id = 2;
  int32 version = 3;
  string resource_name = 4;
  int64 deployment_key = 5;
  google.protobuf.Timestamp created_at = 6;
  bool duplicate = 7;
}
message DeployResourceResponse {
  // ...field 1–3
  repeated Form forms = 4;
}
message GetFormRequest {
  oneof ref {
    int64 form_key = 1;
    string form_id = 2;          // versi terbaru
  }
}
message GetFormResponse { Form form = 1; string schema = 2; }
message ListFormsRequest { string form_id = 1; bool latest_only = 2; int32 page_size = 3; string page_token = 4; }
message ListFormsResponse { repeated Form forms = 1; string next_page_token = 2; }
```

Statistik dihitung dengan agregasi `element_instance` (dan `incident`) per `element_id`; untuk
definition dengan jutaan element instance final, kueri bisa lambat — Fase 6 menambah indeks
`element_instance (process_instance_key, element_id, state)`? Tidak: cukup indeks yang ada +
`only_active_instances` default `true` memakai `process_instance_active_idx`; agregasi penuh
(termasuk selesai) dibatasi `timeout statement` 5 s dan dicatat sebagai kandidat proyeksi
`operate.projection` (Fase 3 consumer) bila terukur lambat (risiko 15).

### 9.3 Pemetaan error baru (`ErrorInfo.reason`)

| `reason` | Kode Connect | Sumber |
|---|---|---|
| `USER_TASK_NOT_FOUND` | `NotFound` | key tidak ada |
| `USER_TASK_NOT_ACTIVE` | `FailedPrecondition` | assign/update/complete pada `COMPLETED`/`CANCELED`; `metadata.state` |
| `USER_TASK_ALREADY_ASSIGNED` | `FailedPrecondition` | assign tanpa `allow_override`; `metadata.assignee` |
| `USER_TASK_NOT_CLAIMABLE` | `PermissionDenied` | principal bukan candidate (hanya bila principal bukan anonim) |
| `USER_TASK_NOT_OWNER` | `PermissionDenied` | complete/unassign oleh bukan assignee tanpa override (hanya bila principal bukan anonim) |
| `FORM_NOT_FOUND` | `NotFound` | `GetUserTaskForm`, `GetForm` |
| `FORM_INVALID` | `InvalidArgument` | deploy `.form`; juga sebagai `ValidationIssue.code` |
| `FORM_NOT_FOUND` (warning deploy) | — | `ValidationIssue` severity `WARNING` pada user task dengan `formId` tak dikenal |

### 9.4 Timer dan subscription message (ketergantungan Fase 5)

Operate menampilkan timer aktif dan subscription message per instance. Dokumen Fase 5 belum ada
saat dokumen ini ditulis; asumsi kontrak minimum yang Fase 6 butuhkan — **bila Fase 5 tidak
menyediakannya, Fase 6 menambahkannya sebagai RPC aditif** di paket Fase 5:

```proto
// workflow/timer/v1 (asumsi) — ListTimers(process_instance_key) → Timer{timer_key, element_instance_key, element_id, due_date, repetitions, state}
// workflow/message/v1 — ListMessageSubscriptions(process_instance_key) → MessageSubscription{key, element_instance_key, element_id, message_name, correlation_key, state}
```

`feature-process-instance` mendefinisikan port `TimerRepository`/`SubscriptionRepository`; bila
client belum ada, `dependencies.ts` memasang implementasi `Unavailable` yang membuat panel
menampilkan empty state "not available in this engine version" (bukan error).

## 10. Lintas aplikasi

### 10.1 Auth placeholder (menuju Fase 8)

- `app-shell/auth-context.tsx`: `AuthProvider({ principal, children })`; `useAuth()` →
  `{ principal: { subject, groups, displayName }, isAnonymous, devIdentity, setDevIdentity }`.
  Fase 6 principal berasal dari dev identity (localStorage) atau anonim; Fase 8 mengganti dengan
  OIDC (`oidc-client-ts` atau serupa, diputuskan di sana) tanpa mengubah konsumen `useAuth`.
- `api-client`: `createEngineTransport(baseUrl, { interceptors: [devIdentity(getIdentity)] })`;
  Fase 8 menambah `bearerToken(getToken)`.
- Engine: `authn.DevIdentity` membaca header hanya bila `ENGINE_DEV_IDENTITY=true` (dev compose
  `true`, prod `false`); log peringatan saat aktif.
- Tidak ada halaman login; `app-shell` menyediakan slot `userMenu` (Fase 6: identitas dev / "anonymous").

### 10.2 Sistem desain: daftar komponen `libs/web/ui`

| Kelompok | Komponen (shadcn kecuali ditandai) | Dipakai oleh |
|---|---|---|
| Sudah ada | `button`, `card`, `badge`, `table`, `input` | semua |
| Overlay | `dialog`, `alert-dialog`, `sheet`, `popover`, `tooltip`, `dropdown-menu`, `command` | start instance, konfirmasi cancel, filter, palette cari elemen |
| Form | `label`, `textarea`, `checkbox`, `switch`, `select`, `calendar`, `date-picker` (komposisi `popover` + `calendar`), `form` (field wrappers untuk TanStack Form), `radio-group` | dialog variabel, update task, filter |
| Navigasi/tata letak | `tabs`, `separator`, `scroll-area`, `resizable`, `breadcrumb`, `pagination`, `sidebar` (opsional, evaluasi), `collapsible` | Modeler panel, Operate detail, Tasklist master–detail |
| Umpan balik | `sonner` (toast), `skeleton`, `alert`, `progress` | semua |
| Data | `avatar`, `DataTable` (milik repo), `KeyValueTable`, `StateBadge`, `RelativeTime`, `CopyButton`, `JsonEditor` (milik repo) | Operate, Tasklist |

Aturan: komponen milik repo mengikuti pola file shadcn (`cva` varian, `cn`), tanpa logika
domain; setiap komponen punya test render minimal (Vitest + Testing Library) dan diperiksa
`jsx-a11y` (plugin sudah terpasang di ESLint React).

### 10.3 i18n

Keputusan 3: UI bahasa Inggris. Mekanisme: setiap fitur punya `presentation/messages.ts`
(`defineMessages({ claim: 'Claim', overdueBy: 'Overdue by {duration}' })`) dan komponen memakai
`t(messages.overdueBy, { duration })`. Tanggal/angka memakai `Intl` dengan locale browser.
Saat id-ID dibutuhkan: pilih library (rekomendasi 16.2 #8), ganti `t` di `libs/web/core`, dan
ekstrak `messages.ts` ke katalog — tanpa menyentuh komponen.

### 10.4 Aksesibilitas, error boundary, empty state, responsif, tema

- **A11y**: semua aksi ikon memiliki `aria-label`; tabel memakai `caption`/`scope`; dialog
  shadcn (Radix) sudah menangani fokus; kanvas bpmn-js/React Flow diberi `role="img"` +
  `aria-label` dan alternatif teks (daftar elemen aktif di panel); kontras token tema dicek
  (`--muted-foreground` pada `--background` ≥ 4.5:1, verifikasi dengan axe); e2e menjalankan
  `@axe-core/playwright` pada setiap halaman utama; `eslint-plugin-jsx-a11y` sudah aktif.
- **Error boundary**: TanStack Router `defaultErrorComponent` = `RouteErrorBoundary`
  (`app-shell`), `notFoundComponent`; mutasi gagal → toast dengan `ErrorInfo.reason` manusiawi
  (tabel pemetaan di `libs/web/core/errors.ts`: `JOB_NOT_ACTIVE` → "This job is no longer active").
- **Empty state**: komponen `EmptyState({ icon, title, description, action })` untuk daftar
  kosong (instance, task, incident) dan "not available" (timer/subscription bila engine tidak
  mendukung).
- **Responsif**: baseline ≥ 1024 px untuk Modeler (kanvas); Operate dan Tasklist dapat dipakai
  di 768 px (panel menjadi tab/stack, tabel scroll horizontal); mobile (≤ 640) hanya Tasklist
  daftar + detail layar penuh. Tidak ada target "mobile-first" untuk Modeler/Operate.
- **Dark mode**: `ThemeToggle` di header (`light` / `dark` / `system`), class `dark` di
  `<html>`, preferensi localStorage `theme`; komponen ui sudah memakai token; properties panel dan
  form-js memiliki CSS sendiri — override variabel CSS-nya ke token tema (verifikasi variabel yang
  tersedia); kanvas bpmn-js tetap putih (3.3/16.2 #10).

### 10.5 Uji end-to-end dengan Playwright (ADR-0036)

- `@nx/playwright` menghasilkan `apps/modeler-web-e2e`, `operate-web-e2e`, `tasklist-web-e2e`
  (target `e2e`, `webServer` = `nx serve <app>` atau `nx preview`); `playwright.config.ts` bersama
  di root `tools/playwright/base.config.ts` (Chromium saja di CI; WebKit/Firefox manual).
- Fixture `tools/playwright/engine.ts`: menunggu `/api/healthz`, helper `deploy(xml)`,
  `createInstance`, `completeJob` lewat Connect (client TS dari `api-client`) untuk menyiapkan
  state — e2e tidak mengetes engine, hanya UI terhadap engine nyata.
- Smoke flow (DoD): modeler (buka korpus → properties → deploy), operate (start → marker →
  complete job via API → selesai; set variables + resolve incident pada `service-task-retries-zero`
  + `FailJob` via API), tasklist (deploy `user-task-native.bpmn` + `approve.form` → start → claim →
  isi form → complete → instance selesai).
- CI job `e2e` (baru): service Postgres, `go build` engine + jalankan dengan `ENGINE_DEV_IDENTITY=true`,
  `pnpm nx run-many -t e2e` (affected bila memungkinkan), artefak trace/screenshot saat gagal;
  waktu target < 5 menit.
- **Tanpa Storybook**: komponen diuji dengan Vitest + Testing Library; dokumentasi komponen =
  README `libs/web/ui` + halaman `/dev/kitchen-sink` di app (hanya `import.meta.env.DEV`) untuk
  melihat semua komponen di kedua tema. Ditinjau ulang bila ada desainer atau > 40 komponen.

## 11. Docker dan deploy (ADR-0035)

| Opsi | Kelebihan | Kekurangan |
|---|---|---|
| **A. Satu image nginx per app** (`web-modeler`, `web-operate`, `web-tasklist`) | Rilis independen; Nx affected → hanya image yang berubah dibangun; image ~15 MB; nginx conf sama | Tiga container, tiga port di compose prod; reverse proxy Fase 8 memetakan host/path |
| B. Satu image menyajikan tiga sub-path (`/modeler/`, `/operate/`, `/tasklist/`) | Satu container/port; satu origin untuk auth cookie | Build Vite dengan `base` per app; satu perubahan membangun ulang tiga app; rilis terkunci bersama; `web.Dockerfile` menjalankan tiga build |

**Dipilih A.** Perubahan:

```dockerfile
# infra/docker/web.Dockerfile (perubahan inti)
ARG APP=modeler-web
# ...
RUN pnpm nx build ${APP} --skip-nx-cache
FROM nginx:1.29-alpine
ARG APP
COPY infra/docker/nginx/default.conf /etc/nginx/conf.d/default.conf
COPY --from=build /src/dist/apps/${APP} /usr/share/nginx/html
```

- `nginx/default.conf`: tetap satu file (SPA fallback, `/api/` → `engine:8080`,
  `proxy_read_timeout 70s` dari Fase 4, `gzip` untuk js/css/json, header
  `Cache-Control` untuk `/assets/`). Env runtime untuk tautan silang (`VITE_*` dibaca saat build)
  → agar satu image berlaku di banyak lingkungan, tautan silang dibaca dari
  `window.__APP_CONFIG__` yang disajikan nginx dari `/config.js` (template `envsubst` saat start,
  `MODELER_URL`, `OPERATE_URL`, `TASKLIST_URL`, `DEV_IDENTITY`); `app-shell` membaca
  `window.__APP_CONFIG__` lalu fallback `import.meta.env`.
- `docker-compose.prod.yml`: service `modeler`, `operate`, `tasklist` (image
  `${REGISTRY}/web-<app>:${VERSION}`, port `MODELER_PORT` 8088, `OPERATE_PORT` 8089,
  `TASKLIST_PORT` 8090, `depends_on engine healthy`); `web` lama dihapus.
- `Makefile`: `docker-build` membangun empat image (loop `for app in modeler-web operate-web
  tasklist-web`), `docker-push` sama; `dev` menjalankan `engine modeler-web operate-web
  tasklist-web`; `dev-ui APP=operate-web` untuk satu app; `deploy-local` mencetak tiga URL.
- CI `docker`: build empat image; push saat tag.
- Vite: `operate-web` port 4201, `tasklist-web` 4202; proxy `/api` sama; `manualChunks`:
  `bpmn` (`bpmn-js`, `bpmn-js-properties-panel`, `@bpmn-io/*`, `diagram-js*`), `flow`
  (`@xyflow/react`, `@dagrejs/dagre`), `form` (`@bpmn-io/form-js`), `vendor` (react, tanstack).

## 12. Strategi uji

### 12.1 Lapisan uji

| Lapisan | Lokasi | Cakupan |
|---|---|---|
| Unit domain Go `execution` | `execution/domain/user_task_test.go` | Tabel transisi user task × command; aktivasi dengan/tanpa assignee; ekspresi gagal → incident; determinisme record; cancel instance → `CANCELED` |
| Unit domain Go `usertask` | `usertask/domain` | `CanClaim`/`CanComplete`/`CanAssign` untuk anonim, candidate user, grup, override; `SearchFilter.Normalize` |
| Unit domain Go `deployment` | `deployment/domain/form_test.go` | `ValidateFormSchema` (valid, bukan object, tanpa `components`, schemaVersion terlalu baru); dedup checksum; resolusi `formId` lintas resource |
| Integration Go | `execution/infrastructure`, `usertask/interfaces`, `deployment/interfaces` (`-tags=integration`) | Kueri pencarian memakai indeks parsial (EXPLAIN); `claimable_by` dengan GIN; handler setiap RPC + setiap `reason` 9.3; deploy `.form` + BPMN dalam satu request; `GetUserTaskForm` setelah form versi baru di-deploy (task lama tetap versi saat aktivasi); replay Fase 3 memulihkan `user_task` |
| E2E engine in-process | `apps/engine/internal/e2e/user_task_test.go` | Deploy → start → search claimable → claim → complete dengan variabel → instance selesai; dua klien claim bersamaan → satu `USER_TASK_ALREADY_ASSIGNED` |
| Vitest domain TS | `libs/web/feature-*/src/domain/*.spec.ts` | `markersFor` (fork 50, loop, incident menang), `buildInstanceTree`, `canClaim`, `formDataToVariables`/`variablesToFormData`, `parseVariablesJson`, `shouldPersist` draft |
| Vitest application TS | `*/application/*.spec.ts` | Store Zustand; `refetchInterval` fungsi mengembalikan 0/false pada instance final; mutasi `useRetryJob` memanggil dua RPC berurutan (fake repository) |
| Vitest infrastructure TS | `*/infrastructure/mappers.spec.ts` | Mapper proto → domain dengan fixture `create(Schema, …)`; `bigint` → string; `findErrorInfo` → `AppError` |
| Vitest presentation TS | `*/presentation/*.spec.tsx` (Testing Library, fake repository lewat provider) | `IncidentsPanel` menampilkan aksi per tipe; `TaskDetail` menonaktifkan Complete bila assignee lain; `StartInstanceDialog` menolak JSON non-object; `ThemeToggle`; `EmptyState` |
| Vitest lib UI | `libs/web/{ui,bpmn-viewer,flow-canvas,form-renderer,app-shell}` | Render + a11y dasar; `BpmnCanvas.setMarkers` dengan kelas token (jsdom + bpmn-js: hanya API tanpa render SVG penuh — bila jsdom tidak cukup, uji di Playwright component atau e2e) |
| E2E Playwright | `apps/*-e2e` | 10.5 |
| Paritas | `tools/scripts/parity` suite `usertask` | 12.3 |
| Lint arsitektur | ESLint boundaries + `archtest` Go | Lapisan dan tag; `usertask` tidak meng-import `execution` selain `api` |

### 12.2 Fixture

- `libs/go/bpmn/testdata/valid`: `user-task-native.bpmn` (zeebe:userTask, assignmentDefinition
  statis, formDefinition formId `approve`), `user-task-native-expressions.bpmn` (assignee dari
  variabel), `user-task-candidates.bpmn`, `user-task-start-form.bpmn` (start event dengan
  formDefinition); `testdata/forms/approve.form`, `invalid.form`.
- Skenario tabel Fase 2 mendapat `Step`: `AssignUserTask`, `UnassignUserTask`,
  `CompleteUserTask(key, vars)`, `SearchUserTasks(filter)`.
- TS: `libs/web/feature-process-instance/src/testing/fixtures.ts` (instance paralel 50, instance
  dengan incident, instance selesai) dan `FakeProcessInstanceRepository` (diekspor dari
  `testing/` untuk app dan e2e helper).

### 12.3 Paritas terhadap Camunda 8.8 (suite `usertask`)

Runner Fase 1–4 mendapat suite `usertask` memakai REST v2 Camunda 8.8 (`/v2/user-tasks/search`,
`/v2/user-tasks/{key}/assignment`, `/v2/user-tasks/{key}/assignee` (DELETE), `/v2/user-tasks/{key}`
(PATCH), `/v2/user-tasks/{key}/completion`, `/v2/user-tasks/{key}/form`, deploy `.form` lewat
`/v2/deployments`; **semua path dan body diverifikasi**; pencarian user task membutuhkan secondary
storage di profil `camunda` — bila tidak tersedia, perbandingan dibatasi pada hasil observable
lewat instance).

| Skenario | Hasil observable yang dibandingkan |
|---|---|
| `user-task-native-complete` | Task dibuat; complete dengan variabel → instance selesai, variabel akhir sama |
| `user-task-assignee-static` | Task lahir dengan assignee terisi |
| `user-task-claim-conflict` | Assign kedua tanpa override ditolak; dengan override diterima |
| `user-task-unassign` | Setelah unassign, assignee kosong, task tetap terbuka |
| `user-task-cancel-instance` | Cancel instance → task tidak lagi terbuka |
| `user-task-form-deploy` | Deploy `.form` + BPMN; form terkait task dapat diambil; versi form naik pada deploy kedua yang berubah |
| `user-task-form-missing` | BPMN merujuk formId yang tidak ada: Camunda menerima deploy (verifikasi) — kita warning; task tetap dibuat; form tidak tersedia |
| `user-task-expression-failure` | Assignee dari variabel yang tidak ada → incident (verifikasi tipe incident Camunda) |

Deviasi yang diperkirakan: state transien Camunda (`ASSIGNING`, …) tidak ada; `priority`
default; task listener tidak ada; `FORM_NOT_FOUND` warning adalah tambahan kita.

## 13. Rencana kerja Fase 6 (urutan eksekusi)

Setiap langkah punya **hasil yang bisa diverifikasi**. Tiga jalur paralel: **E** (engine: 13.2,
13.3, 13.5–13.7), **L** (lib bersama: 13.4, 13.8–13.10), **A** (app: 13.11–13.16). Titik temu:
13.3 (proto dikunci) sebelum 13.9/13.15; 13.7 (handler user task) sebelum e2e Tasklist.

### 13.1 Prasyarat dan branch

- Fase 4 (`IncidentService`, `SetVariables`, client TS, interceptor `authn`) di `main`; Fase 5
  diinginkan (timer/subscription, sub-process) tetapi tidak wajib — panel terkait memakai empty
  state (9.4). Fase 2 fitur `process-instance` di `modeler-web` ada atau tidak: bila tidak, lib
  dibuat langsung dari desain 3.2.
- Branch `feat/phase-6-ui`; sub-branch per jalur bila dua orang; `make check` hijau.
- Verifikasi lisensi dan versi paket 3.3 (bpmn-js, properties panel, moddle, behaviors, minimap,
  form-js) dan ketentuan atribusi React Flow; catat hasil di ADR-0031/0034 dan `docs/licenses.md`.

Hasil: branch ada; tabel lisensi terisi; dokumen ini ditautkan di `docs/plan/README.md`.

### 13.2 Migrasi `user_task` + `form`, sqlc

- Skema 8.2; kueri `InsertUserTask`, `UpdateUserTaskAssignment`, `UpdateUserTaskFields`,
  `UpdateUserTaskState`, `GetUserTask`, `SearchUserTasks` (dua varian urutan), `ListUserTasksByInstance`,
  `InsertForm`, `GetForm`, `GetLatestForm`, `ListForms`, `MaxFormVersion`; agregasi
  `FlowNodeStatisticsByInstance`, `FlowNodeStatisticsByDefinition`, `CountProcessInstancesGrouped`.

Hasil: `make migrate-up`/`down` bolak-balik bersih; `sqlc generate` tanpa diff.

### 13.3 Proto dan generate

- `usertask/v1`, perluasan `process/v1` (statistik, count, parent key, filter versi),
  `deployment/v1` (form) sesuai bagian 9; anotasi protovalidate.
- `buf lint`, `buf breaking`, `buf generate`; `api-client` `createUserTaskClient`, export.

Hasil: `pnpm nx run-many -t lint breaking generate -p proto` hijau; `api-client` typecheck hijau.

### 13.4 Tag `type:feature`, `eslint.feature.config.mjs`, pemindahan fitur ke lib

- Buat `libs/web/feature-deployment`, `feature-process-instance`, `feature-user-task`
  (kerangka), `app-shell`, `form-renderer` dengan generator `@nx/react:library`; tag; alias
  `tsconfig.base.json`; constraint di `eslint.config.mjs` root; aturan lapisan dipindah ke
  `libs/web/eslint.feature.config.mjs` (pola `**/domain/**` dst.) dan dipakai app + lib.
- `git mv` fitur `deployment` (dan `process-instance` bila ada) ke lib; perbaiki import; test ikut.

Hasil: `pnpm nx graph --file=tmp/graph.json` menunjukkan `modeler-web → feature-deployment`;
`make check` hijau; test lint negatif (import `feature-deployment/src/infrastructure/...` dari
app → error).

### 13.5 Domain `execution`: user task native

- `behavior_user_task.go`, `user_task.go`, command baru, record, `Apply`, proyektor; `ProfileExecutable`
  membuka `zeebe:userTask` (PR kecil di `libs/go/bpmn` dengan golden); `deployment/api.Forms`
  dipakai lewat port di `execution` (interface di `execution/domain/ports.go`, adapter di
  `infrastructure`).

Hasil: tabel transisi dan skenario domain lolos; determinisme record; skenario tabel
`user-task-native-complete`, `user-task-candidates` lolos di jalur domain dan Postgres.

### 13.6 Modul `deployment`: form

- 8.4: parse/validasi `.form`, versi, dedup, warning `FORM_NOT_FOUND`, `GetForm`/`ListForms`,
  cache + event.

Hasil: integration test deploy `.form` valid/invalid/duplikat; `buf curl GetForm` mengembalikan
skema; BPMN dengan `formId` tak dikenal → warning di `DeployResourceResponse.warnings`.

### 13.7 Modul `usertask` dan `authn.DevIdentity`

- Lima paket 8.1; kebijakan; pencarian; handler `UserTaskService`; `authn.Principal.Groups`;
  `DevIdentity` di interceptor (flag); konfigurasi `ENGINE_DEV_IDENTITY`, `FORM_MAX_SCHEMA_VERSION`.
- `GetProcessInstanceStatistics`, `GetProcessDefinitionStatistics`, `CountProcessInstances` di
  `execution/interfaces` (langkah kecil, dimasukkan di sini).

Hasil: integration test setiap RPC + setiap `reason` 9.3; e2e in-process 12.1; `archtest` hijau;
`buf curl SearchUserTasks` dengan `claimable_by` mengembalikan hasil yang benar untuk tiga kasus
(tanpa candidate, user cocok, grup cocok).

### 13.8 `libs/web/ui`, `core`, `app-shell`, `form-renderer`

- Komponen 10.2 (batch pertama); util `core` 4.2; `app-shell` 4.5 (tema, auth placeholder, error
  boundary, empty state, layout); `form-renderer` (lazy form-js).
- Halaman `/dev/kitchen-sink` di `modeler-web` (DEV saja).

Hasil: Vitest lib hijau; kitchen-sink menampilkan semua komponen di dua tema; `form-renderer`
merender `approve.form` dan mengembalikan data saat submit (test).

### 13.9 `libs/web/bpmn-viewer` dan `flow-canvas`: perluasan

- Properties panel + moddle `zeebe` + behaviors + minimap lewat `import()` dinamis; marker token;
  overlay; `zoom/undo/redo/findElements`; `onChanged/onSelectionChanged`; CSS tema untuk panel.
- `TreeNode`, `onNodeClick`, `selectedNodeId` di `flow-canvas`.

Hasil: uji manual: properties panel menampilkan tab Zeebe pada service task dan menulis
`zeebe:taskDefinition`; XML dibandingkan dengan korpus Camunda Modeler (normalisasi id) — test
Vitest/Playwright; `setMarkers('token-active', …)` memberi kelas pada `.djs-element`.

### 13.10 `feature-process-instance` dan `feature-user-task`: domain → infrastructure → application

- Domain 3.2 (markers, tree, statistics, variables, incident actions, canClaim, form data);
  repository Connect + mapper; query/mutation hooks dengan polling 3.4; store.
- Fake repository di `testing/`.

Hasil: Vitest domain/mapper/application hijau (termasuk `markersFor` fork 50 + loop + incident);
coverage domain ≥ 90 %.

### 13.11 `feature-process-instance` dan `feature-user-task`: presentation

- Komponen 3.2: viewer dengan marker/overlay, pohon, panel variabel/incident/job/timer/
  subscription/records, dialog start/cancel/variabel, daftar + filter; task list/detail/form/
  assign/update.

Hasil: Vitest presentation dengan fake repository; screenshot uji manual di PR.

### 13.12 Modeler-web

- 5.1–5.3: tata letak tiga panel, toolbar, explorer, draft localStorage, file system, XML view,
  pintasan, minimap, pratinjau form, tombol Start di definitions, tautan Operate; pindah ke
  `app-shell` layout; `process-map` opsional memakai definition nyata.

Hasil: DoD Modeler; uji manual: buat proses user task dengan form di Modeler kita → deploy →
Camunda Modeler membuka XML tanpa perubahan; reload memulihkan draft.

### 13.13 Operate-web

- Generator app; `dependencies.ts`; route 6.1; dashboard lokal; polling; `?element=` sinkron dengan
  seleksi; aksi 6.4; records 6.5.

Hasil: DoD Operate; uji manual terhadap `parallel-50-service-tasks.bpmn` (badge 50 aktif),
`service-task-retries-zero` + `FailJob` (incident → retries → resolve → selesai), cancel.

### 13.14 Tasklist-web

- Generator app; route 7.1; master–detail; identitas dev; filter `claimable_by`; form + fallback;
  pintasan; `/processes` dengan start form.

Hasil: DoD Tasklist; dua browser dengan identitas dev berbeda: claim di satu, hilang dari
"Claimable" di yang lain ≤ 5 s.

### 13.15 Docker, compose, Makefile, CI docker

- 11: `ARG APP`, `config.js` runtime, compose prod tiga service, Makefile `docker-build` loop,
  `dev`, `dev-ui`, CI build empat image; Vite port dan `manualChunks` per app.

Hasil: `make docker-build && make deploy-local` menyajikan tiga app; tautan silang bekerja dari
`config.js`; ukuran chunk awal ≤ 350 kB gzip (laporan `vite build`).

### 13.16 E2E Playwright dan CI job `e2e`

- 10.5: tiga proyek e2e, fixture engine, smoke flow, axe; job CI.

Hasil: `pnpm nx run-many -t e2e` hijau lokal dan di CI < 5 menit; artefak trace saat gagal.

### 13.17 Paritas `usertask`, ADR, dokumentasi

- Suite 12.3 (`make parity SUITE=usertask LIVE=1`); `expectations.json`.
- ADR-0030 (topologi tiga app + lib fitur + app-shell), ADR-0031 (properties panel dan paket
  bpmn.io: lisensi, pembungkusan di `bpmn-viewer`), ADR-0032 (pembaruan langsung: polling adaptif,
  SSE Fase 8), ADR-0033 (user task native: state di `execution`, modul `usertask` sebagai lapisan
  Tasklist, state `CREATED/ASSIGNED/COMPLETED/CANCELED`), ADR-0034 (form: resource `.form`,
  form-js viewer, fallback JSON), ADR-0035 (satu image per app, `config.js` runtime), ADR-0036
  (uji UI: Vitest + Testing Library + fake repository, Playwright e2e, tanpa Storybook/MSW).
- README `apps/{modeler,operate,tasklist}-web`, `libs/web/feature-*`, `libs/web/app-shell`,
  modul `usertask`; `docs/plan/README.md`; `docs/licenses.md`.

Hasil: paritas offline hijau di CI; ADR tersimpan; review DoD.

## 14. Target Nx dan Makefile

| Proyek | Target | Perintah / catatan |
|---|---|---|
| `proto` | `generate`, `lint`, `breaking` | mencakup `usertask/v1`, perluasan `process/v1`, `deployment/v1` |
| `engine` | `test`, `sqlc`, `bench` | `usertask`, perluasan `execution`/`deployment`; `BenchmarkSearchUserTasks` (100 k task terbuka, filter claimable) |
| `modeler-web`, `operate-web`, `tasklist-web` | `serve` (4200/4201/4202), `build`, `test`, `lint`, `typecheck`, `preview` | plugin `@nx/vite` (inferensi); tag `type:app`, `scope:modeler|operate|tasklist` |
| `*-e2e` | `e2e` | `@nx/playwright`; `implicitDependencies` ke app-nya; CI job `e2e` |
| `feature-deployment`, `feature-process-instance`, `feature-user-task` | `test`, `lint`, `typecheck` | tag `type:feature`, `scope:shared` |
| `app-shell`, `form-renderer`, `bpmn-viewer`, `flow-canvas`, `ui`, `core` | `test`, `lint`, `typecheck` | tag `type:ui`/`type:core` |
| `api-client` | `test`, `typecheck` | `createUserTaskClient`, interceptor dev identity |

Makefile: `dev` → `engine modeler-web operate-web tasklist-web`; `dev-ui APP=<app>`; `e2e
[APP=<app>]` (menjalankan engine + Postgres dev lalu `nx run <app>-e2e:e2e`); `docker-build` /
`docker-push` empat image; `deploy-local` mencetak tiga URL; `parity SUITE=usertask [LIVE=1]`;
`gen` tetap (sqlc tiga/empat `sqlc.yaml`). Variabel baru: `MODELER_PORT`, `OPERATE_PORT`,
`TASKLIST_PORT`, `ENGINE_DEV_IDENTITY`.

## 15. Risiko dan mitigasi

| Risiko | Dampak | Mitigasi |
|---|---|---|
| Lisensi paket bpmn.io (bpmn-js, form-js) mensyaratkan watermark/atribusi, atau tidak cocok dengan pemakaian internal | Pelanggaran lisensi | Verifikasi di 13.1 sebelum kode; jangan sembunyikan logo bpmn.io; bila form-js tidak memenuhi, fallback opsi B 3.6 (renderer sendiri untuk subset komponen) dengan biaya +5 hari |
| Atribusi React Flow (`hideAttribution: true` saat ini) | Pelanggaran ketentuan | Verifikasi; tampilkan atribusi bila diwajibkan (satu baris) |
| Versi `bpmn-js-properties-panel` tidak kompatibel dengan `bpmn-js` 18 / React 19 / Vite 8 | Panel tidak bisa dipasang | Kunci versi saat 13.1 pada kombinasi yang didokumentasikan bpmn.io; properties panel adalah Preact mandiri (tidak bergantung React) — konflik kecil kemungkinannya; cadangan: panel properti minimal sendiri untuk `zeebe:taskDefinition`/`ioMapping` (+4 hari) |
| XML Modeler kita berbeda dari Camunda Modeler (urutan atribut, `modeler:executionPlatform`) | Paritas/validasi gagal | Uji perbandingan XML ternormalisasi terhadap korpus; `EMPTY_DIAGRAM` sudah memakai namespace dan atribut platform yang sama |
| Lock instance pada assign/unassign bertabrakan dengan command engine yang panas | Latensi aksi Tasklist | Operasi manusia jarang; lock singkat; diukur di benchmark `BenchmarkAssignUserTask` bersama job aktif |
| Pencarian inbox lambat pada ratusan ribu task terbuka dengan filter grup | Inbox lambat | GIN parsial `state = 'CREATED'`; `EXPLAIN` di integration test; keyset; bila lambat, materialized `candidate_match` ditinjau |
| Agregasi statistik definition berat pada tabel besar | Operate lambat / timeout | Default `only_active_instances`; `statement_timeout` 5 s pada kueri ini; proyeksi `operate.projection` (consumer outbox Fase 3) sebagai langkah berikut |
| Polling 1 s dari banyak tab Operate | Beban engine | `refetchIntervalInBackground: false`; interval dapat diubah lewat env; SSE Fase 8 |
| localStorage draft hilang/penuh (private window, kuota) | Pengguna kehilangan pekerjaan | Semua akses try/catch; banner "drafts unavailable"; Save/Download selalu tersedia; batas 2 MiB/diagram |
| File System Access API hanya Chromium | Save langsung tidak ada di Firefox/Safari | Fallback unduh; dokumentasi |
| Identitas dev dipercaya di produksi | Bypass auth | `ENGINE_DEV_IDENTITY` default `false`; log peringatan; compose prod tidak menyetelnya; Fase 8 menghapus kode |
| Fase 5 belum menyediakan RPC timer/subscription saat Fase 6 mulai | Panel kosong | Port di lib dengan implementasi `Unavailable` + empty state; RPC ditambahkan Fase 6 bila perlu (9.4) |
| Bundle besar (bpmn-js + properties panel + form-js + React Flow) | Waktu muat | Lazy per route + `manualChunks`; DoD ≤ 350 kB gzip chunk awal |
| Pemindahan fitur ke lib bentrok dengan Fase 1/2 yang sedang berjalan di `modeler-web` | Konflik merge | 13.4 dilakukan sebagai PR kecil pertama setelah Fase 1/2 merge; `git mv` menjaga riwayat |
| Tumpang tindih nomor ADR Fase 3/4 (0017–0018) dan kemungkinan Fase 5/7 | Referensi ADR ambigu | Fase 6 memakai 0030–0036; dicatat di 0 untuk dirapikan oleh fase yang merge lebih dulu |

## 16. Pertanyaan terbuka

Setiap pertanyaan diberi rekomendasi. Keputusan final dicatat di 16.1 setelah review.

### 16.1 Keputusan

Belum ada. Diisi dengan tabel yang sama seperti 9.1 dokumen Fase 1.

### 16.2 Pertanyaan dan rekomendasi

| # | Pertanyaan | Rekomendasi | Alasan |
|---|---|---|---|
| 1 | Reverse proxy Fase 8: host-based (`modeler.example`, `operate.example`) atau path-based (`/modeler/`)? | **Host-based**; app tetap `base: '/'`; `VITE_BASE_PATH` disiapkan agar path-based tetap mungkin | Tanpa `base` per build; cookie auth per host diatur di proxy; path-based membutuhkan `basepath` router dan rewrite aset |
| 2 | Satu `QueryClient`/sesi lintas app (B 3.1) vs tiga app? | **Tiga app** (ADR-0030) | Rilis independen, affected tepat; kehilangan navigasi tanpa reload antar app dapat diterima |
| 3 | Properties panel resmi bpmn.io vs panel sendiri dengan shadcn? | **Resmi** (ADR-0031) | Cakupan atribut `zeebe:` lengkap dan terawat; panel sendiri hanya sebagai cadangan risiko |
| 4 | Penyimpanan diagram di server (tabel `diagram`, RPC `SaveDiagram`) di Fase 6? | **Tidak**; file + draft lokal; ditinjau setelah Fase 8 (butuh identitas pemilik) | Tanpa auth, penyimpanan server tanpa pemilik menimbulkan masalah; deploy sudah menyimpan versi resmi |
| 5 | State user task: empat state persisten vs meniru Camunda 8.8 (`CREATING`, `ASSIGNING`, `UPDATING`, `COMPLETING`, …)? | **Empat state** (ADR-0033); intent Camunda dipetakan saat task listener hadir | State transien hanya bermakna dengan listener; API `UserTaskState` dapat ditambah nilai enum secara aditif |
| 6 | Editor form (form-js editor) di Modeler di Fase 6? | **Tidak**; pratinjau saja; editor di Fase 7 bersama FEEL editor | Bundle dan lisensi; pengguna dapat membuat `.form` di Camunda Modeler |
| 7 | "Save without completing" (simpan variabel dari form tanpa menyelesaikan task) di Tasklist? | **Tidak di Fase 6**; draft isian di `sessionStorage` per-viewer; ditinjau bila ada permintaan | Membutuhkan semantik variabel lokal task (Fase 7 scope lokal) agar tidak menimpa variabel proses |
| 8 | Library i18n saat id-ID dibutuhkan? | **Tunda**; kandidat `i18next` + `react-i18next` atau Lingui; string sudah terpusat | Tidak ada pengguna id-ID UI sampai keputusan produk |
| 9 | `process-map` di Modeler: data nyata (call activity dari definition) atau tetap contoh? | **Data nyata minimal** bila `zeebe:calledElement` statis tersedia dari `GetProcessDefinition` XML (parse di klien dengan `bpmn-moddle`), selainnya tetap contoh | Biaya ±1 hari; memberi nilai nyata pada React Flow di Modeler |
| 10 | Tema gelap untuk kanvas bpmn-js? | **Tidak**; kanvas tetap putih di dark mode | Warna elemen BPMN (hitam) dirancang untuk putih; tema gelap kanvas butuh override CSS luas dan berisiko pada ekspor |
| 11 | Operate: aksi debug `CompleteJob` di UI? | **Ya, di balik flag** `VITE_OPERATE_DEBUG_ACTIONS` | Berguna untuk demo tanpa worker; disembunyikan di produksi |
| 12 | `SearchUserTasks` memakai `claimable_by` dari request atau selalu dari `Principal` server? | **Keduanya**: bila `Principal` bukan anonim, server **mengabaikan** `claimable_by.user/groups` dari request dan memakai `Principal` | Mencegah klien menebak inbox orang lain setelah Fase 8; di Fase 6 anonim, request menentukan |
| 13 | Apakah `CompleteUserTask` oleh bukan assignee ditolak? | **Hanya bila principal bukan anonim** dan assignee berbeda tanpa `allow_override` (Operate memakai override) | Konsisten dengan Camunda (verifikasi: Camunda 8.8 tidak menegakkan kepemilikan di engine, Tasklist yang menegakkan) |
| 14 | Satu nginx conf untuk tiga image vs conf per app? | **Satu conf** + `config.js` runtime | Perbedaan antar app hanya tautan silang |
| 15 | Storybook? | **Tidak** (ADR-0036); kitchen-sink DEV | Toolchain tambahan tanpa pengguna; ditinjau bila ada desainer |
| 16 | Versi form yang dipakai task: versi saat aktivasi (dibekukan) vs selalu terbaru saat dibuka? | **Dibekukan saat aktivasi** (`form_key`) | Task yang sedang diisi tidak berubah bentuk; sama dengan binding `latest` Camunda yang diselesaikan saat task dibuat (verifikasi) |

## 17. Estimasi dan pembagian kerja

Perkiraan **38–44 hari kerja** untuk 1 engineer, atau **16–19 hari** untuk 3 engineer paralel
(E engine, L lib bersama, A app):

| Area | Langkah | Hari |
|---|---|---|
| Prasyarat, lisensi, branch | 13.1 | 1 |
| Engine: migrasi, proto, domain user task, form, modul `usertask`, statistik | 13.2, 13.3, 13.5–13.7 | 8–9 |
| Lib bersama: tag/lint/pemindahan, `ui`/`core`/`app-shell`/`form-renderer`, `bpmn-viewer`/`flow-canvas` | 13.4, 13.8, 13.9 | 6–7 |
| Lib fitur: domain/infra/application + presentation | 13.10, 13.11 | 6–7 |
| **Modeler-web** | 13.12 | 5–6 |
| **Operate-web** | 13.13 | 6–7 |
| **Tasklist-web** | 13.14 | 4–5 |
| Docker, compose, Makefile, CI | 13.15 | 1–2 |
| E2E Playwright + CI | 13.16 | 2 |
| Paritas, ADR, dokumentasi, review DoD | 13.17 | 2 |

Per app (termasuk bagian lib yang hanya dipakainya): Modeler ≈ 9 hari (app 5–6 + perluasan
`bpmn-viewer` 3), Operate ≈ 12 hari (app 6–7 + lib `feature-process-instance` 5), Tasklist ≈ 13
hari (app 4–5 + lib `feature-user-task` 3 + engine `usertask`/form 5–6 bagian Tasklist).

Pembagian 3 orang: **E** = 13.2, 13.3, 13.5–13.7, 12.3; **L** = 13.4, 13.8–13.11; **A** = 13.12–13.16.
Urutan: minggu 1 — E proto + migrasi, L tag/lint/pemindahan + `ui`/`app-shell`, A kerangka tiga
app + Docker; minggu 2 — E domain/handler, L `bpmn-viewer`/lib fitur, A Modeler; minggu 3 — E
form + statistik + paritas, L presentation lib, A Operate; minggu 4 — A Tasklist + e2e, semua:
ADR, DoD.

## 18. Keluaran Fase 6

Setelah fase ini selesai, fase berikutnya mendapat:

- **Fase 7 (FEEL dan DMN)**: properties panel yang sudah menampung editor FEEL bawaan untuk
  ekspresi (`=`); titik sisip editor FEEL mandiri dan element templates di `bpmn-viewer`
  (`extensions`); `JsonEditor` untuk diganti CodeMirror; `form-renderer` siap untuk form editor;
  `deployment` sudah menerima resource non-BPMN (`.form`) sehingga `.dmn` tinggal menambah tipe;
  Operate menampilkan incident `EXTRACT_VALUE_ERROR` dengan editor variabel sebagai jalur
  perbaikan untuk ekspresi FEEL yang gagal; user task memakai evaluator yang sama untuk
  assignee/candidate/due date; `SetVariables(local)` dan scope lokal dapat ditampilkan di panel
  variabel (kolom `scope_key` sudah ada di domain UI).
- **Fase 8 (observability, auth, hardening)**: `AuthProvider`/`useAuth` dan interceptor transport
  sebagai satu titik ganti OIDC per app; `authn.Principal.Groups` dan kebijakan `usertask` yang
  sudah menegakkan candidate saat principal bukan anonim; `DevIdentity` untuk dihapus; hook query
  dengan `queryKey` stabil untuk diganti SSE (`operate.projection` consumer outbox); tiga image
  web + `config.js` runtime siap di belakang reverse proxy/TLS; e2e Playwright sebagai smoke test
  deploy; metrik yang perlu: latensi `SearchUserTasks`, jumlah polling per detik.
- **Semua**: lib fitur empat lapisan dan `app-shell` sebagai pola untuk UI baru; fake repository
  di `testing/` untuk uji UI tanpa engine; suite paritas `usertask`; korpus `user-task-*.bpmn` dan
  `.form`.
