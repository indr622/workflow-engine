# Fase 5 — Timer, Message, Boundary Event, dan Sub-process

**Proyek:** Workflow Engine (kompatibel konsep Camunda 8 / BPMN 2.0)
**Stack:** Go (engine utama) + React (UI) dalam monorepo Nx
**Status:** Draft rencana — belum dieksekusi
**Tanggal:** 2026-10-28

---

## 0. Status eksekusi

Belum ada langkah yang dieksekusi. Bagian ini diisi dengan tabel yang sama seperti Fase 0
(langkah, status, bukti) begitu pekerjaan dimulai.

**Kondisi yang menjadi titik awal (dicek 2026-10-28; Fase 1 sedang dieksekusi, Fase 2–4 masih
draft; Fase 4 belum lengkap saat dokumen ini ditulis):**

| Area | Kondisi / asumsi dari fase sebelumnya | Dampak ke Fase 5 |
|---|---|---|
| `libs/go/bpmn` (Fase 1, sedang dibangun) | Model kategori: `*bpmn.Event` dengan `Definition().Kind()` (`Timer`/`Message`/`Error`/`Escalation`/`Signal`/`Terminate`), `Definition().Timer()` (`Kind` date/duration/cycle + `Value Expression`), `AttachedTo()`, `CancelActivity()`, `IsInterrupting()`, `TaskDefinition()` untuk throw berbasis job; `*bpmn.Activity` dengan `Inner() *Scope`, `BoundaryEvents()`, `MultiInstance()`, `CalledElement()`, `Message()` untuk receive task; `*bpmn.Scope` dengan `StartEvents()`, `Children()`, `Owner()`; `*bpmn.Message.CorrelationKey()`; `*bpmn.ErrorDef.ErrorCode()`, `*bpmn.Escalation.EscalationCode()`; validasi ISO 8601 di `iso8601.go` (durasi, tanggal dengan zone id, cycle `R[n]/…`, cron 6 field); `ProfileExecutable` = fungsi `executableInPhase2` | Parser **tidak disentuh** kecuali: `ProfileExecutable` diperluas per elemen yang dieksekusi (4.2), beberapa aturan validasi baru (4.3), dan helper publik untuk evaluasi ISO 8601 (6.3) |
| Korpus `libs/go/bpmn/testdata/valid` | `v08_timer_events` (timer start cycle `R/PT1H`, boundary interrupting `PT30M`, boundary non-interrupting `R3/PT10M`, intermediate `timeDate`), `v09_message_events` (message start tanpa correlation key, receive task, message boundary, intermediate catch), `v10_message_throw` (throw/send/end berbasis job), `v11_error_events` (error end di sub-process + error boundary), `v12_escalation` (escalation throw + boundary non-interrupting), `v13_embedded_subprocess` (dua tingkat), `v14_event_subprocess` (timer interrupting, message non-interrupting, error catch-all), `v15_call_activity`, `v16_multi_instance` (parallel dengan output, sequential dengan completion condition), `v17_terminate_end`, `v18_signal_events`, `v19_event_based_gateway` | Dipakai langsung sebagai fixture skenario (12.2); file tambahan hanya untuk pola yang tidak ada (interupsi bersarang, join di dalam sub-process, call activity bertingkat) |
| Modul `execution` (Fase 2) | `Processor` + `Apply` event-sourced, antrean run-to-completion, `ElementInstance{FlowScopeKey}` + `ChildrenByScope`, counter join per `(flow_scope_key, gateway, flow)`, penyelesaian flow scope (5.5 Fase 2, baru untuk `PROCESS`), `CancelInstance` yang menghentikan anak dahulu, incident, job di tabel `job`, port `Clock`, `KeyGenerator`, `ExpressionEvaluator`; `variable.scope_key` sudah ada tetapi selalu = instance | Fase 5 **memperluas** model yang sama: scope bersarang, subtree termination, timer dan subscription sebagai state instance (record baru), command baru. Tidak ada desain ulang |
| Modul `execution` (Fase 4) | `ThrowError` dari job: record `JOB ERROR_THROWN` lalu **selalu** incident `UNHANDLED_ERROR_EVENT`; DoD Fase 4 menjanjikan "Fase 5 menyisipkan pencarian catch event **sebelum** pembuatan incident tanpa mengubah kontrak API" | Dipegang: 8.2 menyisipkan pencarian catch di `behavior_error.go`; incident hanya bila tidak tertangkap |
| Modul `message` (Fase 4, ADR-0019) | `MessageService.PublishMessage(name, correlation_key, ttl, message_id, variables)` → tabel `message` (buffer, `expires_at`, dedup `message_id` → `AlreadyExists`, TTL 0 tidak di-buffer), `api.Messages.Publish`, event `MessagePublished`, sweeper TTL; "korelasi Fase 5 hanya menambah `message_subscription` dan dua titik korelasi (saat publish, saat subscribe)" | Dipegang, dengan satu penyesuaian: Fase 4 (bagian 7) menyebut `message_subscription` "di modul yang sama"; Fase 5 menempatkannya di **`execution`** (atomik dengan record dan terminate subtree, alasan di 3), sedangkan `message_start_subscription` dan `message_correlation` tetap di `message`. Dua titik korelasi (7.3) sesuai janji Fase 4. Event `message.published` Fase 4 **tidak** dipakai untuk korelasi (korelasi sisi publish terjadi di transaksi publish, 7.3). Bila Fase 4 belum merge, RPC dan tabel `message` dibuat di Fase 5 persis seperti sketsa Fase 4 bagian 7 dan 8.3 |
| Fase 3 | Outbox durable (`Relay.Register(Consumer)`, handler dalam transaksi relay), `recovery.Participant`, idempotensi `request_id`, snapshot `format_version`, `UnitOfWork`/`TxScope` | Consumer durable baru: `timer.start_scheduler`, `message.start_subscription`, `execution.message_correlation`, `execution.cross_instance`; participant `timer` dan `message`; snapshot `format_version` naik ke 2 |
| `internal/platform/sweeper` (Fase 4) | Ticker + batch `SKIP LOCKED`, dipakai `job` (deadline) dan `message` (TTL) | Dipakai ulang oleh `timer` (jatuh tempo) |
| `internal/platform/keygen`, `eventbus`, `db` | Alokasi blok key; bus in-process; `db.Connect` dengan pool/timeout (Fase 3) | Tidak berubah; `db.WithTx/TxFromContext` ditambah untuk panggilan antarmodul dalam satu transaksi (3.3) |
| Migrasi | `00001_init` … `00006_history` (Fase 3); Fase 4 menambah `00005_job_worker.sql` dan tabel `message` (nomor di dokumen Fase 3 dan 4 saling tumpang tindih; nomor final mengikuti urutan merge) | Migrasi Fase 5: `000NN_scope.sql`, `000NN_timer.sql`, `000NN_message_subscription.sql`; di dokumen ini disebut `scope.sql`, `timer.sql`, `message_subscription.sql` |
| Proto | `workflow/process/v1`, `workflow/job/v1`, `workflow/message/v1` (Fase 4), `workflow/common/v1/errors.proto` | Penambahan aditif saja (bagian 11); `buf breaking` tetap hijau |
| Modul `job` (Fase 4) | Lapisan protokol worker; `JobService.ThrowError` memanggil `execution/api.Jobs.ThrowError` | Tidak berubah; perilaku `ThrowError` berubah di dalam `execution` |
| `apps/modeler-web` fitur `process-instance` (Fase 2) | Viewer bpmn-js dengan marker token, panel job | Ditambah panel timer/subscription dan tautan instance anak (11.3); Operate penuh tetap Fase 6 |
| Compose | Profil `camunda` dengan `camunda/zeebe:8.8.44` | Paritas memakai profil yang sama; timer paritas memakai durasi pendek |
| ADR | Fase 1–4 memakai ADR-0008 sampai ±ADR-0022 | Fase 5 memakai **ADR-0023 sampai ADR-0028** |

## 1. Lingkup

Fase 5 membuat engine **bereaksi terhadap waktu dan dunia luar, dan menata eksekusi dalam scope
bersarang**: timer yang jatuh tempo menggerakkan token tanpa ada klien yang memanggil, message dari
luar dikorelasikan ke instance yang menunggu atau memulai instance baru, error dan escalation
dilempar lalu ditangkap oleh boundary event atau event sub-process, dan activity bisa berupa
sub-process, event sub-process, call activity ke proses lain, atau multi-instance. Semua dibangun
di atas model event-sourced Fase 2 (`Processor` + `Apply`), persistensi dan pemulihan Fase 3
(outbox, participant, snapshot), serta protokol worker dan buffer message Fase 4.

Termasuk dalam Fase 5:

1. **Pohon scope** di `execution/domain`: element instance bertipe scope (`PROCESS`, `SUB_PROCESS`,
   `EVENT_SUB_PROCESS`, `MULTI_INSTANCE_BODY`), penyelesaian flow scope yang digeneralisasi,
   algoritma **terminate subtree** (anak, job, timer, subscription, incident) sebagai satu-satunya
   mekanisme interupsi (boundary interrupting, event sub-process interrupting, terminate end,
   cancel instance, cancel call activity).
2. **Timer**: state timer di `execution` (record `TIMER CREATED/TRIGGERED/CANCELED`, tabel
   `timer`), modul `timer` sebagai scheduler (sweeper jatuh tempo dengan klaim `FOR UPDATE SKIP
   LOCKED`, timer start event per versi terbaru, re-schedule saat deploy, cycle), evaluasi ISO 8601
   tanpa FEEL, abstraksi clock untuk test, pemulihan setelah restart, firing at-least-once dengan
   command trigger idempoten.
3. **Message**: subscription per element instance (receive task, intermediate catch, boundary,
   event sub-process start, event-based gateway) dan subscription start event per versi terbaru;
   korelasi dua arah (saat publish dan saat subscribe) di atas buffer Fase 4; semantik Zeebe
   untuk correlation key, keunikan message start per `(name, correlationKey)` per proses,
   `messageId`; message throw/end/send task tetap berbasis job (Fase 4).
4. **Error dan escalation**: `ThrowError` dari job dan error end event dipropagasikan ke error
   boundary / error event sub-process terdekat yang cocok (`errorCode`, catch-all), termasuk lintas
   call activity; tidak tertangkap → incident `UNHANDLED_ERROR_EVENT` (Fase 4). Escalation throw /
   end event dengan semantik non-interrupting default dan catch interrupting/non-interrupting.
5. **Activity baru**: embedded sub-process, event sub-process (interrupting / non-interrupting), call
   activity (instance anak dengan `parent_*`, propagasi variabel, penyelesaian dan pembatalan
   lintas instance), multi-instance parallel dan sequential dengan input/output collection dan
   completion condition memakai subset FEEL Fase 2 (keputusan 3.2), terminate end event, event-based
   gateway.
6. **Persistensi dan pemulihan**: migrasi, snapshot `format_version` 2, participant pemulihan
   `timer` dan `message`, consumer outbox durable untuk deploy dan korelasi.
7. **Proto dan UI minimal**: timer dan subscription terlihat per instance; instance anak tertaut.
8. **Strategi uji**: skenario tabel dengan jam palsu untuk korpus v08–v19, property test invarian
   interupsi, paritas Camunda 8.8.

**Tidak termasuk** (fase lain): FEEL penuh untuk timer (`=` di timer definition), kondisi
multi-instance di luar subset, I/O mapping dan scope variabel lokal umum (Fase 7; Fase 5 hanya
scope lokal multi-instance, 5.6); cron di `timeCycle` (Fase 7, keputusan 3.1); signal event
(belum dijadwalkan, keputusan 3.1); user task native dan Tasklist (Fase 6); Operate penuh dengan
riwayat pohon instance (Fase 6); modifikasi instance, migrasi instance antar versi; compensation,
link event, conditional event (tidak direncanakan); `startInstructions`; message dengan
`tenant_id` (Fase 8); pembatasan jumlah instance aktif per message start.

## 2. Definition of Done Fase 5

- [ ] `ProfileExecutable` menerima semua elemen di matriks 4.1 bertanda "Fase 5"; file korpus v08–v17 dan v19 lolos deploy; v18 (signal) tetap ditolak `UNSUPPORTED_ELEMENT` dengan pesan yang menyebut "belum dijadwalkan"; aturan validasi baru 4.3 punya file `testdata/invalid` masing-masing.
- [ ] Embedded sub-process (v13, dua tingkat): record `ELEMENT_ACTIVATING/ACTIVATED` sub-process → none start di dalam → … → scope selesai saat tidak ada anak aktif dan counter join kosong → `ELEMENT_COMPLETED` sub-process → outgoing di scope induk; `GetProcessInstance` menampilkan pohon lewat `flow_scope_key`.
- [ ] Parallel join dan inclusive join **di dalam** sub-process bekerja dengan counter per `flow_scope_key` (skenario baru `subprocess-parallel-join`, `subprocess-inclusive-join`); property test gateway Fase 2 diperluas dengan blok sub-process acak dan tetap lolos 1.000 iterasi.
- [ ] Terminate subtree: setelah interupsi apa pun (boundary interrupting, event sub-process interrupting, terminate end, cancel instance, cancel call activity), **tidak ada** job `CREATED/ACTIVATED`, timer `CREATED/DUE`, atau subscription `OPEN/CORRELATING` yang `element_instance_key`-nya berada di subtree yang dihentikan; dibuktikan property test 12.3 (≥ 1.000 skenario acak) dan kueri verifikasi Fase 3 baru `VerifyOrphanTimers`, `VerifyOrphanSubscriptions`.
- [ ] Timer intermediate catch (`timeDuration`, `timeDate`) dengan jam palsu: tidak terpicu sebelum jatuh tempo, terpicu setelah jam dimajukan + satu sweep; record `TIMER CREATED` → `TIMER TRIGGERED` → `ELEMENT_COMPLETED` catch event; `TriggerTimer` yang diulang (at-least-once) untuk timer yang sudah terpicu atau dibatalkan menjadi no-op tanpa record (`ErrTimerNotActive` di-log, bukan error RPC).
- [ ] Timer boundary interrupting (v08 `timeout`): host `TERMINATED`, job host `CANCELED`, boundary → `timedOut`; non-interrupting `R3/PT10M` (v08 `reminder`): terpicu tepat 3 kali selama host aktif, host tetap `ACTIVATED`, setiap trigger membuat element instance boundary baru dan timer baru dengan `repetition + 1`; setelah 3 kali tidak ada timer tersisa; host selesai lebih dulu → timer sisa `CANCELED`.
- [ ] Timer start event: setelah deploy proses dengan timer start, baris `timer` dengan `process_instance_key NULL` ada untuk **versi terbaru saja** (versi lama `CANCELED` oleh consumer `timer.start_scheduler`); saat jatuh tempo instance baru dibuat dari start event tersebut (`PROCESS_INSTANCE_CREATION CREATED` dengan `start_element_id`); cycle `R/PT1H` dijadwalkan ulang setelah setiap trigger; `timeDate` lampau saat deploy → terpicu sekali segera (verifikasi perilaku Camunda 8.8 untuk timer start yang sudah lewat).
- [ ] Sweeper timer: dua replika engine pada satu DB memproses 10.000 timer jatuh tempo bersamaan tanpa trigger ganda yang menghasilkan record ganda (klaim `FOR UPDATE SKIP LOCKED` + command idempoten); timer `DUE` yang `claimed_until` lewat (replika mati setelah klaim) diklaim ulang dan terpicu ≤ 2 × `ENGINE_TIMER_SWEEP_INTERVAL` + `ENGINE_TIMER_CLAIM_LEASE`.
- [ ] Pemulihan: engine dimatikan saat 1.000 timer jatuh tempo dalam 5 detik berikutnya; setelah restart (participant `timer` + sweeper) semua terpicu tepat sekali; skenario chaos baru `timer-missed-during-restart` hijau.
- [ ] Message intermediate catch / receive task (v09): `PublishMessage` setelah subscription terbuka mengkorelasikan dalam ≤ 2 × `ENGINE_OUTBOX_POLL_INTERVAL` (jalur consumer) dan variabel message digabung ke scope instance; `PublishMessage` **sebelum** subscription terbuka dengan TTL > 0 di-buffer dan dikorelasikan **secara sinkron** saat subscription dibuat (dalam transaksi command yang sama); TTL 0 tanpa subscription → dibuang; satu message tidak pernah dikorelasikan dua kali ke instance yang sama; dua message buffered dengan key sama dikorelasikan berurutan sesuai `message_key` saat subscription terbuka berulang (loop).
- [ ] Message boundary interrupting (v09 `cancelled`) menghentikan receive task dan subscription-nya; message start (v09 `OrderPlaced`): publish tanpa correlation key membuat instance baru dari versi terbaru; message start **dengan** correlation key hanya membuat satu instance aktif per `(name, correlation_key, bpmn_process_id)` selama instance yang dibuatnya masih aktif (verifikasi perilaku persis Camunda 8.8); setelah deploy versi baru, subscription start versi lama dinonaktifkan.
- [ ] Event sub-process (v14): timer start interrupting menghentikan `task` lalu menjalankan `onTimeout`; message start non-interrupting menjalankan `onUpdate` sejajar dengan `task` dan bisa terpicu berulang; error start catch-all menangkap `ThrowError` dari job `work`; scope proses selesai hanya setelah semua anak (termasuk event sub-process) selesai; subscription event sub-process dibatalkan saat scope selesai.
- [ ] Error (v11): `ThrowError(error_code = "PAYMENT_FAILED")` dari job `charge` dan error end event `payFailed` keduanya menghentikan sub-process `pay` dan mengaktifkan boundary `onPaymentError` → `refund`; `ThrowError` dengan code lain → incident `UNHANDLED_ERROR_EVENT` pada `charge` (perilaku Fase 4 dipertahankan); error boundary tanpa `errorRef` (catch-all) menangkap code apa pun; urutan prioritas 8.1 dibuktikan skenario `error-nearest-wins` dan `error-exact-before-catch-all`.
- [ ] Escalation (v12): throw `raise` di dalam `handle` memicu boundary non-interrupting `onLate` → `lateEnd` sementara `handle` lanjut ke `hEnd`; escalation tidak tertangkap → **tanpa** incident, token lanjut (verifikasi terhadap dokumentasi Camunda 8.8); escalation code tidak cocok tidak memicu.
- [ ] Call activity (v15 + proses anak): instance anak dibuat dengan `parent_process_instance_key`/`parent_element_instance_key`, variabel induk disalin ke anak (`propagateAllParentVariables`, default diverifikasi), penyelesaian anak menyelesaikan call activity di induk dan menyalin variabel anak (`propagateAllChildVariables`), pembatalan induk menghentikan anak (`TERMINATED`), error tidak tertangkap di anak dipropagasikan ke boundary call activity di induk (verifikasi); `GetProcessInstance` induk menampilkan `called_process_instance_key` pada element instance call activity; proses anak yang tidak ada saat aktivasi → incident `CALLED_ELEMENT_ERROR` pada call activity.
- [ ] Multi-instance (v16): parallel `each` atas `items` membuat N element instance body sekaligus (N job `process-item`), `outputCollection results` berisi hasil urut indeks setelah semua selesai; sequential `seq` atas `approvers` menjalankan satu demi satu dan berhenti saat `completionCondition` `=approved` bernilai `true` (sisa iterasi tidak dibuat); koleksi kosong → body selesai segera dengan `results = []`; `inputCollection` bukan list → incident `EXTRACT_VALUE_ERROR`; variabel `inputElement` hanya terlihat di scope body (lookup berantai) dan tidak bocor ke scope proses.
- [ ] Terminate end event (v17): `kill` menghentikan `b` (job `CANCELED`) dan menyelesaikan proses `COMPLETED` (bukan `TERMINATED`); di dalam sub-process hanya menghentikan scope sub-process tersebut.
- [ ] Event-based gateway (v19): gateway `ACTIVATED` dengan satu timer dan satu subscription milik element instance gateway; message `Answer` datang lebih dulu → timer `CANCELED`, token ke `endAnswer`; timer dulu → subscription `DELETED`, token ke `endTimeout`; tidak pernah keduanya.
- [ ] Determinisme dan replay: untuk setiap skenario Fase 5, `Apply` ulang seluruh record menghasilkan state identik dengan proyeksi (termasuk timer, subscription, pohon scope); `engine rebuild` Fase 3 lolos untuk instance dengan sub-process dan call activity; snapshot `format_version = 2` dan snapshot versi 1 dibaca sebagai "mulai dari nol" dengan WARN.
- [ ] Verifikasi start Fase 3 diperluas: `VerifyOrphanTimers`, `VerifyOrphanSubscriptions`, `VerifyScopeTree` (anak non-final dengan scope final); `engine verify --all` bersih setelah semua skenario integrasi.
- [ ] `GetProcessInstanceResponse` memuat `timers` dan `message_subscriptions`; `ProcessInstance` memuat `parent_process_instance_key`, `parent_element_instance_key`; `MessageService.ListMessageSubscriptions`; `buf breaking` hijau; client TS dan SDK Go meneruskan field baru (tanpa helper baru).
- [ ] Modeler-web fitur `process-instance`: panel "Menunggu" (timer dengan jatuh tempo, subscription dengan nama + key), marker `token-waiting` pada boundary/catch event yang punya timer/subscription aktif, tautan ke instance anak dan induk; uji manual tercatat di PR.
- [ ] Uji paritas suite `events` dan `scopes` (12.4): semua skenario bertanda `parity` memberi hasil observable yang sama dengan Camunda 8.8 referensi; deviasi tercatat di `expectations.json` dengan alasan.
- [ ] Benchmark: 10.000 timer jatuh tempo serentak diproses < 10 s dengan 1 replika (angka awal, dikunci setelah pengukuran); korelasi message end-to-end (publish → token lanjut) p50 < 50 ms saat idle; `Processor` murni untuk sub-process 3 tingkat × 10 cabang tetap > 20.000 element instance/detik.
- [ ] ADR-0023 sampai ADR-0028 tersimpan di `docs/adr/`; `docs/plan/README.md` menautkan dokumen ini; README modul `execution`, `timer`, `message` diperbarui.

## 3. Keputusan teknis

| Area | Keputusan | Alasan singkat |
|---|---|---|
| Pohon scope | Scope adalah **element instance** (`PROCESS`, `SUB_PROCESS`, `EVENT_SUB_PROCESS`, `MULTI_INSTANCE_BODY`); anak menunjuk induk lewat `FlowScopeKey` (sudah ada sejak Fase 2); counter join dan penyelesaian flow scope sudah dikunci ke `flow_scope_key` (ADR-0023) | Fase 2 sengaja menyiapkan `FlowScopeKey`; sub-process hanya mengganti "flow scope = instance" menjadi "flow scope = element instance mana pun yang bertipe scope" |
| Interupsi | Satu algoritma **`terminateSubtree(ei)`** untuk semua interupsi; membatalkan job, timer, subscription, incident milik setiap element instance di subtree dalam command yang sama (ADR-0023) | Tidak ada jalur khusus per pemicu; invarian "tidak ada yatim" bisa diuji dengan property test satu kali untuk semua pemicu |
| Kepemilikan timer | **State timer milik `execution`**: record `TIMER CREATED/TRIGGERED/CANCELED`, tabel `timer` ditulis projector dalam transaksi command; modul **`timer`** = scheduler (sweeper, timer start, clock) di atas `execution/api` tanpa tabel sendiri (ADR-0024) | Pembatalan timer harus atomik dengan terminate subtree, sama alasannya dengan job (Fase 2 ADR-0011); scheduler adalah hal yang tidak berhubungan dengan token: menunggu, mengklaim, memicu |
| Firing timer | **Klaim dua langkah**: transaksi pendek `UPDATE timer SET state='DUE', claimed_until … WHERE key IN (SELECT … FOR UPDATE SKIP LOCKED)`, commit, lalu per timer command `TriggerTimer` yang mengunci instance dan memverifikasi ulang; timer `DUE` lewat `claimed_until` diklaim ulang (ADR-0024) | Sweeper tidak boleh memegang row lock timer sambil menunggu row lock instance (command instance mengubah baris timer di bawah lock instance, jadi bisa deadlock). Klaim + command idempoten memberi at-least-once tanpa record ganda |
| Timer start event | Baris `timer` dengan `process_instance_key NULL` + `process_definition_key`; consumer durable `timer.start_scheduler` atas `deployment.process_deployed` membatalkan timer start versi lama dan membuat yang baru; `TriggerTimer` untuk timer start = `CreateInstance{StartElementID}` | Hanya versi terbaru yang punya timer start (perilaku Zeebe; verifikasi); consumer durable Fase 3 menjamin tidak ada deploy yang terlewat |
| Evaluasi waktu | ISO 8601 **tanpa FEEL** di Fase 5: `timeDuration` (`P…T…`), `timeDate` (RFC 3339 + zone id `[Area/City]` opsional), `timeCycle` `R[n]/duration` dan `R[n]/start/duration`; helper `bpmn.ParseISODuration/ParseISODateTime/ParseTimerCycle` dipublikasikan dari `iso8601.go`; `timeCycle` **cron** dan ekspresi `=` ditolak saat deploy (`TIMER_EXPRESSION_NOT_SUPPORTED`, `TIMER_CRON_NOT_SUPPORTED`) sampai Fase 7 | Validator Fase 1 sudah memeriksa bentuknya; evaluasi ada di kernel agar scheduler dan domain memakai aturan yang sama; cron butuh parser sendiri dan semantik Quartz Zeebe yang belum diverifikasi |
| Presisi timer | Sweeper tiap `ENGINE_TIMER_SWEEP_INTERVAL` (default 500 ms), batch 500, klaim `due_at <= $now`; jaminan: terpicu **tidak sebelum** `due_at` dan biasanya ≤ interval + latensi command setelahnya; tidak ada jaminan sub-detik (didokumentasikan) | Timer proses bisnis berorde detik sampai hari; polling DB sederhana dan benar lintas replika |
| Clock | `domain.Clock` Fase 2 dipakai untuk `due_at`; modul `timer` menerima `clock.Clock` (`Now`, `NewTicker`) dengan implementasi `clock.Fake{Advance}` di `internal/platform/clock`; kueri sweeper menerima `$now` dari Go, bukan `now()` DB | Skenario timer tidak boleh tidur sungguhan; satu sumber waktu untuk engine dan kueri agar test dan replay konsisten |
| Kepemilikan subscription | **`message_subscription` milik `execution`** (record `MESSAGE_SUBSCRIPTION CREATED/CORRELATED/DELETED`); **`message_start_subscription`** dan **`message_correlation`** milik modul `message`; tabel `message` Fase 4 tetap milik `message` | Subscription per element instance hidup-mati bersama token (atomik dengan terminate subtree); subscription start hidup-mati bersama versi definition (urusan deploy, bukan instance) |
| Korelasi | **Lane lock** `pg_advisory_xact_lock(hashtext('msg:' || name || '/' || correlation_key))` di kedua sisi; **sisi subscribe** (transaksi command, sudah memegang lock instance) mencocokkan buffer dan mengkorelasikan **sinkron**; **sisi publish** hanya memegang lane lock, menandai subscription `CORRELATING` dan menulis outbox `message.correlated` per target; consumer durable `execution.message_correlation` menjalankan `CorrelateMessage` dengan lock instance (ADR-0025) | Urutan lock konsisten (instance lalu lane di subscribe; lane saja di publish) sehingga bebas deadlock; lane lock menutup race "publish tidak melihat subscription baru, subscribe tidak melihat message baru"; latensi jalur publish = satu interval relay (< 10 ms dengan LISTEN) |
| Keunikan korelasi | Satu message dikorelasikan ke **setiap** subscription non-start yang cocok (banyak instance) tetapi **maksimal satu kali per process instance** (tabel `message_correlation`); untuk message start **maksimal satu instance aktif per `(name, correlation_key, bpmn_process_id)`** selama instance yang dibuatnya aktif; publish tanpa correlation key selalu membuat instance baru. Verifikasi semua aturan terhadap dokumentasi Camunda 8.8 | Mengikuti semantik Zeebe sejauh yang penulis ketahui; deviasi dicatat di `expectations.json` |
| Lintas instance | Call activity, propagasi error/escalation lintas instance, dan pembatalan anak memakai **internal command** yang dikirim lewat outbox (`execution.cross_instance`, consumer durable), bukan lock bersarang induk–anak (ADR-0026) | Lock dua instance dalam satu transaksi memaksa urutan global dan bisa deadlock dengan cancel; outbox memberi at-least-once dengan idempotensi per `(child_key, intent)`; latensi satu interval relay |
| Propagasi error | Pencarian catch dari elemen pelempar ke luar: boundary pada elemen itu → event sub-process di flow scope-nya → boundary pada owner scope → … → root; di tiap tingkat exact `errorCode` menang atas catch-all; tidak ada → incident `UNHANDLED_ERROR_EVENT` (Fase 4); lintas call activity: tidak tertangkap di anak → anak `TERMINATED` + lempar ke boundary call activity induk (verifikasi) (ADR-0028) | Urutan "terdekat dulu" adalah semantik BPMN; verifikasi urutan persis Camunda 8.8 di paritas |
| Escalation | Sama seperti error untuk pencarian catch, tetapi pelempar **tidak** berhenti (throw event selesai normal), tidak tertangkap → tanpa incident; boundary escalation boleh interrupting (`cancelActivity` default `true` pada boundary BPMN) atau non-interrupting | Sesuai spesifikasi BPMN dan dokumentasi Camunda 8 sejauh diketahui; verifikasi |
| Multi-instance | **Dikerjakan di Fase 5** dengan subset FEEL Fase 2: `inputCollection`, `outputElement`, `completionCondition` wajib path/literal (ditolak saat deploy bila di luar subset); body = element instance `MULTI_INSTANCE_BODY` + satu element instance inner per iterasi dengan **scope variabel lokal** (`inputElement`, `loopCounter`) dan lookup berantai scope → induk (ADR-0027) | Korpus v16 hanya memakai path; menunda ke Fase 7 berarti dua fase tanpa pola "untuk setiap item" yang sangat umum. Lookup berantai adalah fondasi scope lokal Fase 7, bukan sampingan |
| Event-based gateway | **Dikerjakan di Fase 5**: gateway tetap `ACTIVATED`, timer/subscription dibuat atas nama element instance gateway dengan `target_element_id`; pemicu pertama membatalkan sisanya | Hanya komposisi timer + subscription + terminate; korpus v19 ada |
| Terminate end event | **Dikerjakan di Fase 5**: `terminateSubtree` setiap anak lain di flow scope lalu scope selesai normal | Trivial di atas ADR-0023 |
| Signal event | **Tidak dijadwalkan** (tetap `UNSUPPORTED_ELEMENT`); dicatat sebagai kandidat Fase 7/8 | Butuh broadcast lintas instance dan subscription signal start; tidak ada kebutuhan produk; v18 tetap sebagai bukti parser |
| Variabel call activity | Fase 5 tanpa I/O mapping: `propagateAllParentVariables` (default `true`, verifikasi) menyalin semua variabel scope induk ke root anak saat create; `propagateAllChildVariables` (default `true` bila tidak ada output mapping, verifikasi) menyalin semua variabel root anak ke scope induk saat selesai; `false` → tidak ada yang disalin | Semantik Zeebe tanpa mapping; Fase 7 menambah mapping di titik yang sama |
| Profil deploy | `ProfileExecutable` menerima elemen 4.1; aturan baru 4.3 ditambah di `libs/go/bpmn` dengan file invalid | Pola Fase 1–2 |
| Snapshot | `format_version = 2` (timer, subscription, scope lokal, `Parent`); versi 1 → mulai dari nol dengan WARN | Dijanjikan Fase 3 |

### 3.1 Penempatan elemen yang belum dijadwalkan (jawaban matriks Fase 1)

| Elemen | Keputusan | Alasan |
|---|---|---|
| Terminate end event | Fase 5 | Satu pemanggilan `terminateSubtree` pada flow scope; v17 ada |
| Event-based gateway | Fase 5 | Komposisi primitif Fase 5; v19 ada; dibutuhkan pola "timeout atau jawaban" yang umum |
| Signal event | Belum dijadwalkan; kandidat Fase 7 bersama FEEL (nama signal bisa ekspresi) | Broadcast lintas instance + signal start subscription adalah fitur sendiri |
| `timeCycle` cron | Fase 7 | Parser cron + semantik Quartz Zeebe perlu verifikasi; FEEL timer juga Fase 7 |
| Multi-instance | Fase 5 (subset) | 3.2 |
| Call activity | Fase 5 | Bagian dari "sub-process"; v15 ada |

### 3.2 Multi-instance: Fase 5 dengan subset, atau Fase 7 (ADR-0027)

| Opsi | Kelebihan | Kekurangan |
|---|---|---|
| A. Tunda seluruhnya ke Fase 7 | Tidak ada scope lokal sebelum FEEL penuh | Dua fase tanpa "untuk setiap item"; Fase 7 harus mengerjakan FEEL, mapping, **dan** semantik iterasi sekaligus |
| **B. Fase 5 dengan subset FEEL (path/literal) + scope lokal minimal** | Korpus v16 jalan; semantik iterasi (parallel/sequential, completion condition, output collection) diuji terpisah dari FEEL; lookup berantai menjadi fondasi Fase 7 | `inputCollection` hanya path (mis. `=items`), bukan ekspresi list; ditolak saat deploy di luar subset |
| C. Fase 5 dengan evaluator ad-hoc untuk list | Lebih banyak model jalan | Semantik menyimpang dari FEEL (Fase 2 sudah menolak opsi ini) |

**Dipilih B.** Aturan deploy: `inputCollection` wajib FEEL path; `outputElement` wajib FEEL path
atau literal; `completionCondition` wajib ekspresi boolean subset Fase 2; di luar itu →
`EXPRESSION_NOT_SUPPORTED` (mekanisme Fase 2, 3.4). Hasil `inputCollection` wajib list JSON,
selain itu incident `EXTRACT_VALUE_ERROR` pada body.

### 3.3 Panggilan antarmodul dalam satu transaksi

Sisi subscribe (7.3) berjalan di transaksi command `execution` dan harus membaca/menandai baris
`message` milik modul `message`. Aturan Fase 0 melarang tabel bersama, tetapi mengizinkan panggilan
sinkron lewat `api`. Keputusan: `internal/platform/db` menambah

```go
func WithTx(ctx context.Context, tx pgx.Tx) context.Context
func TxFromContext(ctx context.Context) (pgx.Tx, bool)
```

`UnitOfWork.Do` (Fase 3) memasang transaksi ke `ctx`; implementasi `message/infrastructure`
memakai `TxFromContext` bila ada, pool bila tidak. `execution/application` hanya memanggil
`message/api.Buffer.Claim(ctx, …)`. Pola yang sama dipakai `execution.cross_instance` dan handler
consumer Fase 3 (yang sudah menerima `pgx.Tx`). Dicatat di ADR-0025.

### 3.4 Tata letak modul (baru dan perluasan)

```
apps/engine/internal/modules/execution/            # perluasan Fase 5
├─ api/api.go                 # Timers (Trigger, ListByInstance), Subscriptions (Correlate, ListByInstance),
│                             # ProcessInstances += CreateFromStart{StartElementID, Parent}, CompleteCallActivity,
│                             # TerminateChild, ThrowErrorFromChild
├─ api/events.go              # + TopicTimerCreated, TopicTimerCanceled (sinyal scheduler), TopicChildInstanceCompleted,
│                             # TopicChildInstanceTerminated, TopicChildErrorUnhandled, TopicSubscriptionOpened
├─ domain/
│  ├─ scope.go                # isScope(type), completeScopeIfQuiescent, terminateSubtree
│  ├─ timer.go                # Timer{Key, ElementInstanceKey, ElementID, TargetElementID, DueAt, Repetition, Cycle}
│  ├─ subscription.go         # MessageSubscription{Key, ElementInstanceKey, ElementID, TargetElementID, MessageName, CorrelationKey, Interrupting, State}
│  ├─ variables.go            # + lookup berantai scope; VariableLookup.WithScope(key)
│  ├─ behavior_subprocess.go  # embedded sub-process, event sub-process
│  ├─ behavior_boundary.go    # pemasangan catch (timer/message) saat aktivasi host; pemicu interrupting/non-interrupting
│  ├─ behavior_timer.go       # timer catch/boundary/start di event sub-process/event-based gateway
│  ├─ behavior_message.go     # receive task, intermediate catch, boundary, event sub-process start, event-based gateway
│  ├─ behavior_error.go       # findErrorCatch, throwError (job + end event), escalation
│  ├─ behavior_call_activity.go
│  ├─ behavior_multi_instance.go
│  ├─ behavior_gateway.go     # + event-based gateway
│  ├─ command.go              # + TriggerTimer, CorrelateMessage, CompleteCallActivity, TerminateChild, ThrowErrorFromChild
│  ├─ record.go               # + ValueType TIMER, MESSAGE_SUBSCRIPTION; payload Parent di PROCESS_INSTANCE_CREATION
│  ├─ apply.go                # + timer, subscription, scope lokal, snapshot v2
│  └─ catch_index.go          # indeks statis per *bpmn.Process: catch event per scope/activity (cache per definition key)
├─ application/
│  ├─ trigger_timer.go, correlate_message.go, complete_call_activity.go, terminate_child.go
│  ├─ create_instance.go      # + Parent, StartElementID, propagasi variabel induk
│  ├─ outbox_events.go        # + pemetaan record Fase 5 → event
│  ├─ cross_instance_consumer.go       # consumer execution.cross_instance (internal command)
│  └─ message_correlation_consumer.go  # consumer execution.message_correlation
├─ infrastructure/
│  ├─ projector.go            # + timer, message_subscription, parent_*
│  ├─ queries/timer.sql, queries/subscription.sql
│  └─ postgres_repository.go  # LoadRuntimeState += timer, subscription, variabel semua scope
└─ interfaces/                # consumer outbox terdaftar di sini (adapter masuk)

apps/engine/internal/modules/timer/
├─ api/api.go                 # (kosong secara fungsional; dipertahankan demi pola lima paket)
├─ domain/
│  ├─ schedule.go             # NextDue(def, now, repetition) di atas bpmn.Parse*; Lease
│  └─ errors.go
├─ application/
│  ├─ sweep_due.go            # klaim batch → TriggerTimer per timer (lewat execution/api)
│  ├─ reschedule_start.go     # consumer deployment.process_deployed
│  └─ recovery_participant.go # rekonsiliasi timer start vs definition terbaru; log/metrik timer tertunda
├─ infrastructure/
│  ├─ queries/timer_claim.sql # ClaimDueTimers, ReleaseExpiredClaims
│  └─ sweeper_task.go
└─ interfaces/
   └─ consumers.go            # registrasi consumer outbox

apps/engine/internal/modules/message/              # perluasan Fase 4
├─ api/api.go                 # Buffer.Claim(ctx, name, key, instanceKey) (Message, bool); StartSubscriptions.Match(name)
├─ domain/                    # + StartSubscription, Correlation (keunikan), Lane
├─ application/
│  ├─ publish_message.go      # + korelasi sisi publish (lane lock, match, outbox)
│  ├─ sync_start_subscriptions.go  # consumer deployment.process_deployed
│  └─ recovery_participant.go
├─ infrastructure/            # + queries/start_subscription.sql, queries/correlation.sql, lane_lock.go
└─ interfaces/                # + ListMessageSubscriptions (membaca execution/api), consumers.go

apps/engine/internal/platform/clock/   # Clock, Real, Fake
libs/go/bpmn/iso8601.go                # + ParseISODuration, ParseISODateTime, ParseTimerCycle (publik)
```

## 4. Model dan cakupan

### 4.1 Matriks perilaku elemen Fase 5

| Elemen | Aktivasi | Wait state | Penyelesaian / pemicu | Catatan |
|---|---|---|---|---|
| Embedded sub-process | `ACTIVATING` → `ACTIVATED`; pasang boundary (5.3); aktifkan none start di `Inner()` | Ya, selama ada anak | Scope quiescent (5.2) → `COMPLETING` → `COMPLETED` → outgoing di scope induk | Join di dalam memakai counter `flow_scope_key = key sub-process` |
| Event sub-process (`triggeredByEvent`) | Tidak lewat sequence flow; subscription/timer start-nya dipasang saat **scope induk** aktif; dipicu → `ACTIVATING` dengan `FlowScopeKey` = scope induk, start event di dalamnya langsung aktif | Ya | Seperti sub-process; interrupting: `terminateSubtree` setiap saudara di scope induk **sebelum** aktivasi | Non-interrupting boleh terpicu berulang; interrupting hanya sekali (subscription lain milik scope dibatalkan) |
| Call activity | `ACTIVATING`: evaluasi `processId` (statis/path), resolusi versi terbaru lewat `deployment/api`; tidak ada → incident `CALLED_ELEMENT_ERROR`; `ACTIVATED` + record `PROCESS_INSTANCE_CREATION CREATED` **di instance anak** lewat internal command (5.5) | Ya | Anak selesai → `CompleteCallActivity` → merge variabel → `COMPLETED` → outgoing | Anak `TERMINATED` oleh pembatalan langsung → call activity `TERMINATED` → propagasi ke atas (verifikasi perilaku Zeebe saat anak dibatalkan langsung) |
| Multi-instance (parallel) | Body `MULTI_INSTANCE_BODY` `ACTIVATED`; evaluasi `inputCollection` → N; buat N inner (scope lokal per inner) sekaligus | Ya | Semua inner selesai (atau completion condition `true` → sisanya `TERMINATED`) → kumpulkan output → body `COMPLETED` | N = 0 → body selesai segera |
| Multi-instance (sequential) | Sama; inner pertama saja | Ya | Inner selesai → cek completion condition → inner berikutnya / selesai | |
| Timer start event (proses) | Dari sweeper: `CreateInstance{StartElementID}` | — | Start event aktif → selesai → outgoing | Hanya versi terbaru |
| Timer intermediate catch | `ACTIVATED` + `TIMER CREATED` (`timeDuration`/`timeDate`) | Ya | `TriggerTimer` → `TIMER TRIGGERED` → `COMPLETING` → `COMPLETED` → outgoing | `timeCycle` ditolak Fase 1 |
| Timer boundary | Dipasang saat host `ACTIVATED` (timer milik element instance host, `TargetElementID` = boundary) | — (bukan element instance sampai terpicu) | Interrupting: `terminateSubtree(host)` → boundary `ACTIVATING…COMPLETED` → outgoing; non-interrupting: boundary diaktifkan sebagai saudara host; `timeCycle` → timer baru `repetition+1` bila masih ada sisa | Timer dibatalkan saat host final |
| Message start event | Dari consumer korelasi: `CreateInstance{StartElementID, Variables}` | — | Seperti timer start | Keunikan per `(name, key, process)` |
| Message intermediate catch / receive task | `ACTIVATED` + evaluasi `correlationKey` (subset: path/literal) → `MESSAGE_SUBSCRIPTION CREATED`; cek buffer sinkron (7.3) | Ya | `CorrelateMessage` → merge variabel message → `CORRELATED` → `COMPLETED` → outgoing | `correlationKey` gagal/`null` → incident `EXTRACT_VALUE_ERROR` |
| Message boundary | Subscription milik host, `TargetElementID` = boundary | — | Interrupting / non-interrupting seperti timer boundary; non-interrupting: subscription **tetap terbuka** untuk message berikutnya | |
| Message throw / end / send task | Berbasis job (Fase 2/4): job `type` dari `TaskDefinition()` | Ya (job) | `CompleteJob` | Tidak ada korelasi internal (sama dengan Zeebe) |
| Error end event | `ACTIVATING` → lempar error (8.1) | Tidak | Tertangkap: end event `COMPLETED`, lalu interupsi oleh catcher; tidak tertangkap: incident pada end event | Verifikasi state end event persis Zeebe |
| Error boundary / error event sub-process start | Dipasang statis lewat `catch_index.go` (tidak ada subscription runtime) | — | Dipicu oleh 8.1 | Catch-all = tanpa `errorRef` atau `errorCode` kosong |
| Escalation throw (intermediate) / end | Lempar escalation (8.3) lalu selesai normal | Tidak | — | Tidak tertangkap → lanjut tanpa incident |
| Escalation boundary / event sub-process | Statis seperti error | — | Interrupting / non-interrupting | |
| Terminate end event | `terminateSubtree` setiap saudara non-final di flow scope → end event `COMPLETED` → scope selesai | Tidak | — | Scope selesai **normal** (`COMPLETED`) |
| Event-based gateway | `ACTIVATED`; untuk tiap outgoing target (timer/message catch, receive task) buat timer/subscription milik gateway dengan `TargetElementID` | Ya | Pemicu pertama: batalkan sisa, `SEQUENCE_FLOW_TAKEN` ke target, target `ACTIVATING…COMPLETED` (tanpa menunggu lagi) → outgoing target | Validator Fase 1 sudah membatasi target |
| Signal event | — | — | — | Ditolak `UNSUPPORTED_ELEMENT` |

### 4.2 Perluasan `ProfileExecutable`

`executableInPhase2` digantikan `executableInPhase5` (nama fungsi mengikuti fase terakhir yang
memperluas; profil tetap `ProfileExecutable`):

```go
func executableInPhase5(el FlowElement) bool {
	switch x := el.(type) {
	case *SequenceFlow:
		return true
	case *Gateway:
		return true // + EventBasedGatewayType
	case *Activity:
		switch x.typ {
		case TaskType, ManualTaskType, ServiceTaskType, SendTaskType, ReceiveTaskType,
			SubProcessType, EventSubProcessType, CallActivityType:
			return true
		case UserTaskType:
			return !x.userTask.ZeebeUserTask // Fase 4
		case ScriptTaskType, BusinessRuleTaskType:
			return x.IsJobBased() // Fase 7 membuka varian FEEL/DMN
		}
		return false
	case *Event:
		if x.definitionCount > 1 {
			return false
		}
		switch x.definition.kind {
		case NoneEventDefinition, TimerEventDefinition, MessageEventDefinition,
			ErrorEventDefinition, EscalationEventDefinition, TerminateEventDefinition:
			return true
		}
		return false // SignalEventDefinition
	}
	return false
}
```

Kombinasi event + definition yang tidak sah (error start di level proses, terminate di
intermediate, dsb.) tetap ditolak `UNSUPPORTED_EVENT_DEFINITION` oleh aturan Fase 1, bukan oleh
profil.

### 4.3 Aturan validasi baru di `libs/go/bpmn`

| Kode | Severity | Kondisi | Profil |
|---|---|---|---|
| `TIMER_EXPRESSION_NOT_SUPPORTED` | error | Timer definition dengan `IsFEEL` (`=…`) | executable (Fase 7 menghapus) |
| `TIMER_CRON_NOT_SUPPORTED` | error | `timeCycle` berbentuk cron 6 field | executable (Fase 7 menghapus) |
| `TIMER_DATE_IN_PAST` | warning | `timeDate` statis sudah lewat saat validasi | semua (hanya peringatan) |
| `MESSAGE_START_DUPLICATE_NAME` | error | Dua message start event di satu proses memakai `name` message yang sama | semua (verifikasi aturan Zeebe) |
| `MESSAGE_CORRELATION_KEY_NOT_SUPPORTED` | error | `correlationKey` di luar subset Fase 2 (bukan path/literal) | executable |
| `MULTI_INSTANCE_EXPRESSION_NOT_SUPPORTED` | error | `inputCollection`/`outputElement`/`completionCondition` di luar subset | executable |
| `CALLED_ELEMENT_EXPRESSION_NOT_SUPPORTED` | error | `processId` FEEL bukan path/literal | executable |
| `EVENT_SUBPROCESS_INTERRUPTING_DUPLICATE` | warning | Dua event sub-process interrupting dengan timer di satu scope (hanya satu yang bisa menang) | semua |
| `BOUNDARY_ON_EVENT_SUBPROCESS` | error | Boundary event menempel pada event sub-process | semua (verifikasi: BPMN melarang) |

Aturan `TIMER_CYCLE_NOT_ALLOWED` Fase 1 sudah membatasi `timeCycle` ke timer start dan boundary
non-interrupting (lihat `rules_events.go`); event sub-process timer start dengan `timeCycle`
non-interrupting diizinkan (verifikasi dukungan Camunda 8.8).

### 4.4 Record baru

| ValueType | Intent | Payload | Dipakai |
|---|---|---|---|
| `TIMER` | `CREATED` | `key, element_instance_key (0 untuk timer start), process_definition_key, element_id, target_element_id, due_at, repetition, remaining (-1 = tak terbatas), cycle (raw)` | Semua timer |
| `TIMER` | `TRIGGERED` | `key, repetition` | Sweeper → `TriggerTimer` |
| `TIMER` | `CANCELED` | `key, reason (SCOPE_TERMINATED / HOST_COMPLETED / GATEWAY_RESOLVED / NEW_VERSION)` | Terminate subtree, host selesai, deploy versi baru |
| `MESSAGE_SUBSCRIPTION` | `CREATED` | `key, element_instance_key, element_id, target_element_id, message_name, correlation_key, interrupting` | Catch message aktif |
| `MESSAGE_SUBSCRIPTION` | `CORRELATED` | `key, message_key, variables` | Korelasi (sinkron atau consumer) |
| `MESSAGE_SUBSCRIPTION` | `DELETED` | `key, reason` | Terminate subtree, host selesai, interrupting terpicu |
| `PROCESS_INSTANCE_CREATION` | `CREATED` | + `start_element_id`, `parent_process_instance_key`, `parent_element_instance_key`, `trigger (API/TIMER/MESSAGE/CALL_ACTIVITY)` | Create dari timer/message/call activity |
| `PROCESS_INSTANCE` | `ELEMENT_*` | + `element_type` baru: `SUB_PROCESS`, `EVENT_SUB_PROCESS`, `CALL_ACTIVITY`, `MULTI_INSTANCE_BODY`, `BOUNDARY_EVENT`, `INTERMEDIATE_CATCH_EVENT`, `INTERMEDIATE_THROW_EVENT`, `RECEIVE_TASK`, `SEND_TASK`, `EVENT_BASED_GATEWAY`; payload `called_process_instance_key` pada call activity `ACTIVATED` | Semua |
| `VARIABLE` | `CREATED/UPDATED` | `scope_key` kini bisa ≠ instance (scope lokal multi-instance) | 5.6 |
| `ERROR` | `THROWN` | `element_instance_key, error_code, message, source (JOB/END_EVENT/CHILD_INSTANCE)`; `CAUGHT` dengan `catch_element_id` | Observabilitas Operate; tidak mengubah state selain penanda |
| `ESCALATION` | `THROWN` / `CAUGHT` | analog | idem |

Timer start event tidak punya process instance; record-nya **tidak** masuk `execution_record`
(yang bernomor per instance). Timer start disimpan langsung di tabel `timer` oleh modul `timer`
dengan `process_instance_key NULL`; `TIMER TRIGGERED` untuk timer start dicatat sebagai record
pertama instance baru (`PROCESS_INSTANCE_CREATION CREATED` dengan `trigger = TIMER` dan
`timer_key`). Hal yang sama berlaku untuk subscription start (tabel `message_start_subscription`
milik `message`).

## 5. Desain domain: scope, interupsi, dan activity baru

### 5.1 Perluasan tipe

```go
package domain

type ElementInstance struct {
	Key, ProcessInstanceKey, FlowScopeKey int64
	ElementID   string
	ElementType bpmn.ElementType
	State       ElementState
	// Fase 5
	CalledInstanceKey int64 // call activity: instance anak
	MultiInstance     *MultiInstanceState // hanya pada MULTI_INSTANCE_BODY
	Interrupted       bool  // scope yang sedang diinterupsi event sub-process interrupting
}

type MultiInstanceState struct {
	Sequential  bool
	Items       []json.RawMessage // snapshot inputCollection saat aktivasi
	NextIndex   int               // sequential: iterasi berikutnya
	Outputs     []json.RawMessage // indeks = iterasi; nil = belum selesai
	Done        int
}

type Timer struct {
	Key, ElementInstanceKey int64
	ElementID, TargetElementID string
	DueAt      time.Time
	Repetition int32 // 0 untuk pemicu pertama
	Remaining  int32 // -1 tak terbatas
	Cycle      string // raw timeCycle untuk re-schedule; kosong bila sekali
	State      TimerState // Created, Due (klaim scheduler; tidak di record), Triggered, Canceled
}

type SubscriptionState uint8 // Open, Correlating, Correlated, Deleted
type MessageSubscription struct {
	Key, ElementInstanceKey int64
	ElementID, TargetElementID string
	MessageName, CorrelationKey string
	Interrupting bool
	State        SubscriptionState
}

type InstanceState struct {
	// ... Fase 2
	Parent        *ParentRef // nil untuk instance root
	Timers        map[int64]*Timer
	Subscriptions map[int64]*MessageSubscription
	// Variables kini dikelompokkan per scope: map[int64]map[string]json.RawMessage
}

type ParentRef struct{ ProcessInstanceKey, ElementInstanceKey int64 }

// Command baru (eksternal maupun internal).
type TriggerTimer struct{ TimerKey int64; Repetition int32 }
type CorrelateMessage struct{ SubscriptionKey, MessageKey int64; Variables json.RawMessage }
type CompleteCallActivity struct{ ElementInstanceKey, ChildKey int64; Variables json.RawMessage }
type TerminateChild struct{ Reason string }                       // dijalankan pada instance anak
type ThrowErrorFromChild struct{ ElementInstanceKey int64; ErrorCode, Message string }
type ThrowEscalationFromChild struct{ ElementInstanceKey int64; EscalationCode string }
```

`VariableLookup` menjadi berantai: `state.Lookup(scopeKey, name)` mencari di `scopeKey`, lalu
`FlowScopeKey` induknya, sampai root instance (`scope_key = process_instance_key`). Penulisan
variabel tanpa mapping (job, message) tetap ke **root** (semantik Fase 2) kecuali variabel yang
sudah ada di scope lokal multi-instance pada rantai (Zeebe menulis ke scope terdekat yang memiliki
variabel itu; verifikasi). Scope lokal lain menunggu Fase 7.

### 5.2 Penyelesaian flow scope (generalisasi 5.5 Fase 2)

```
onElementFinal(ei):                       # dipanggil setelah COMPLETED / TERMINATED
  cancelOwned(ei)                         # timer + subscription milik ei (boundary, gateway, event sub-process start)
  scope := state.Elements[ei.FlowScopeKey]
  if scope == nil: return                 # ei adalah PROCESS
  if scope.State == Terminating: return   # terminateSubtree yang akan menutupnya
  if ei.ElementType == MULTI_INSTANCE_INNER: onInnerDone(scope, ei)   # 5.6, bisa membuat iterasi baru
  if !hasActiveChildren(scope) && !hasTakenFlows(scope.Key):
      completeScope(scope)

completeScope(scope):
  switch scope.ElementType:
    PROCESS:               ELEMENT_COMPLETING/COMPLETED proses; instance COMPLETED; bila Parent != nil → event ChildInstanceCompleted
    SUB_PROCESS, EVENT_SUB_PROCESS: COMPLETING/COMPLETED → takeOutgoing(scope) di scope induknya → onElementFinal(scope)
    MULTI_INSTANCE_BODY:   collectOutputs → COMPLETED → takeOutgoing → onElementFinal
```

Pengecekan tetap dilakukan **saat quiescence** (antrean kosong) untuk inclusive join (5.6 Fase 2);
analisis `Upstream` per gateway kini dihitung per scope BPMN (`*bpmn.Scope`), dan posisi token aktif
yang dipertimbangkan hanya element instance dengan `FlowScopeKey` = scope yang sama (element
instance sub-process dihitung sebagai satu token di node sub-process, seperti dijanjikan Fase 2).
Boundary event tidak diperlakukan sebagai outgoing host untuk reachability (token boundary muncul
sebagai saudara host hanya setelah terpicu, dan saat itu ia terlihat sebagai token aktif biasa).

### 5.3 Pemasangan catch pada aktivasi host

Saat activity `A` menjadi `ACTIVATED` (setelah job/subscription/anak dibuat):

```
for b in A.BoundaryEvents() (urutan dokumen):
  switch b.Definition().Kind():
    Timer:   due := schedule.NextDue(def, now, 0); emit TIMER CREATED{owner: A.key, target: b.ID()}
    Message: key := evalCorrelationKey(b); emit MESSAGE_SUBSCRIPTION CREATED{owner: A.key, target: b.ID()}; tryBufferedMessage()
    Error, Escalation: tidak ada state runtime (catch_index)
```

Saat scope `S` (`PROCESS`/`SUB_PROCESS`) menjadi `ACTIVATED`, hal yang sama untuk setiap event
sub-process di `S.Inner().Children()` yang start event-nya timer/message (`owner: S.key`,
`target: startEvent.ID()`); error/escalation start statis.

Pemicu (`TriggerTimer` / `CorrelateMessage`) menemukan `owner` dan `target`:

```
fire(owner, target, payload):
  switch target.Type():
    BoundaryEventType:
      if target.CancelActivity():  terminateSubtree(owner); activate(target, owner.FlowScopeKey)
      else:                        activate(target, owner.FlowScopeKey)          # saudara host
                                   if timer cycle && remaining != 0: emit TIMER CREATED{repetition+1}
                                   if message: subscription tetap OPEN (non-interrupting)
    StartEventType (event sub-process ESP di scope owner):
      if target.IsInterrupting():  owner.Interrupted = true; for c in children(owner) except ESP: terminateSubtree(c)
                                   cancelOwned(owner) kecuali subscription ini; activate(ESP, owner.Key) → start aktif
      else:                        activate(ESP, owner.Key) → start aktif; subscription/timer tetap (cycle)
    IntermediateCatchEventType / ReceiveTaskType (owner = gateway event-based):
      cancelOwned(owner) kecuali yang terpicu; SEQUENCE_FLOW_TAKEN gateway→target; activate(target) lalu langsung complete
    (owner adalah catch event itu sendiri): complete(owner)
```

Variabel message digabung sebelum `activate(target)` sehingga kondisi di outgoing melihatnya.

### 5.4 `terminateSubtree`

```
terminateSubtree(ei):
  if ei.State in {Completed, Terminated}: return
  emit ELEMENT_TERMINATING(ei)
  for child in childrenOf(ei) sorted key DESC:   # anak dulu; call activity: lihat di bawah
      terminateSubtree(child)
  for job in jobsOf(ei):           emit JOB CANCELED
  for t in timersOwnedBy(ei):      emit TIMER CANCELED{SCOPE_TERMINATED}
  for s in subscriptionsOwnedBy(ei): emit MESSAGE_SUBSCRIPTION DELETED{SCOPE_TERMINATED}
  for inc in openIncidentsOf(ei):  emit INCIDENT RESOLVED
  if ei.ElementType == CALL_ACTIVITY && ei.CalledInstanceKey != 0:
      emit ELEMENT_TERMINATED(ei) setelah anak dikonfirmasi? → TIDAK: lihat 5.5 (terminasi anak asinkron)
  emit ELEMENT_TERMINATED(ei)
  Apply menghapus TakenFlows dengan flow_scope_key = ei.Key
```

Urutan record satu interupsi (boundary timer interrupting pada service task `work`, v08):

```
TIMER                TRIGGERED          t(timeout)
PROCESS_INSTANCE     ELEMENT_TERMINATING work
JOB                  CANCELED           job(work)
TIMER                CANCELED           t(reminder)        # timer lain milik work
PROCESS_INSTANCE     ELEMENT_TERMINATED  work
PROCESS_INSTANCE     ELEMENT_ACTIVATING  timeout
PROCESS_INSTANCE     ELEMENT_ACTIVATED   timeout
PROCESS_INSTANCE     ELEMENT_COMPLETING  timeout
PROCESS_INSTANCE     ELEMENT_COMPLETED   timeout
PROCESS_INSTANCE     SEQUENCE_FLOW_TAKEN f3
...
```

Invarian yang ditegakkan `Apply` (error bila dilanggar, sehingga bug terdeteksi di test
determinisme): tidak ada `JOB CREATED`, `TIMER CREATED`, `MESSAGE_SUBSCRIPTION CREATED` yang
`element_instance_key`-nya final; `ELEMENT_TERMINATED` pada scope hanya bila semua anak final.

### 5.5 Call activity dan internal command lintas instance (ADR-0026)

```
Induk (tx A, lock P)                        Outbox / consumer execution.cross_instance            Anak (tx B, lock C)
──────────────────────────                 ──────────────────────────────────────────            ───────────────────
ACTIVATING callActivity
resolve processId → def (deployment/api)
alokasi key C (keygen)                      
ELEMENT_ACTIVATED callActivity{called=C}
outbox: cross_instance.create_child{C, def, vars}
COMMIT ───────────────────────────────────► handler: CreateInstance{Key: C, Parent{P, ei}, Variables}
                                            (idempoten: instance C sudah ada → no-op)              PROCESS_INSTANCE_CREATION CREATED{parent}
                                                                                                   ... berjalan ...
                                                                                                   ELEMENT_COMPLETED process
                                                                                                   outbox: execution.process_instance_completed{C, parent}
                              ◄──────────── handler: CompleteCallActivity{ei, C, vars anak}
CompleteCallActivity: ei ACTIVATED? → merge vars → COMPLETING → COMPLETED → outgoing
(ei sudah final → no-op; C ≠ ei.CalledInstanceKey → no-op + WARN)
```

- Key anak dialokasikan di induk agar `called_process_instance_key` tercatat pada
  `ELEMENT_ACTIVATED` dan handler create idempoten berdasarkan key.
- Jendela antara commit A dan create C: `GetProcessInstance(C)` → `NotFound` selama ≤ satu
  interval relay; diterima dan didokumentasikan.
- **Pembatalan induk**: `terminateSubtree` pada call activity menulis `ELEMENT_TERMINATED` segera
  dan outbox `cross_instance.terminate_child{C}`; handler menjalankan `TerminateChild` (=
  `CancelInstance` dengan reason `PARENT_TERMINATED`) pada C; anak yang sudah final → no-op.
  Anak yang tidak dipantau sesaat tidak berbahaya: anak tidak bisa memengaruhi induk selain lewat
  internal command yang memeriksa state induk.
- **Pembatalan anak langsung** (`CancelProcessInstance(C)` dari API): record
  `process_instance_terminated{parent}` → handler `TerminateChild`? Tidak: handler menjalankan
  `ChildTerminated{ei}` pada induk → call activity `TERMINATED` → propagasi `onElementFinal`
  (scope induk melanjutkan seperti activity yang dihentikan; verifikasi apakah Zeebe membuat
  incident pada call activity dalam kasus ini — bila ya, ikuti).
- **Error tidak tertangkap di anak**: 8.2.
- Idempotensi handler: tabel `cross_instance_applied(child_key, intent)` milik `execution`
  dengan `ON CONFLICT DO NOTHING` dalam transaksi relay (Fase 3 sudah menjamin efek DB atomik
  dengan kursor; tabel ini hanya untuk pengiriman ulang setelah dead letter requeue).
- Kedalaman call activity dibatasi `ENGINE_MAX_CALL_DEPTH` (default 32); melebihi → incident
  `CALLED_ELEMENT_ERROR` pada call activity (mencegah rekursi tak hingga).

### 5.6 Multi-instance (ADR-0027)

```
activate(MI activity A):
  body := new ElementInstance{Type: MULTI_INSTANCE_BODY, ElementID: A.ID(), FlowScopeKey: scope}
  ELEMENT_ACTIVATING/ACTIVATED body
  items := eval(inputCollection) di scope induk; bukan list → INCIDENT EXTRACT_VALUE_ERROR (body ACTIVATED)
  if len(items) == 0: completeBody(body)
  parallel:   for i in items: startIteration(body, i)
  sequential: startIteration(body, 0)

startIteration(body, i):
  inner := new ElementInstance{Type: A.Type() (SERVICE_TASK/SUB_PROCESS/...), ElementID: A.ID(), FlowScopeKey: body.Key}
  VARIABLE CREATED{scope: inner.Key, name: inputElement, value: items[i]}     # bila inputElement diisi
  VARIABLE CREATED{scope: inner.Key, name: "loopCounter", value: i+1}
  activate(inner) dengan perilaku elemen aslinya (job, sub-process, call activity, ...)
  boundary event A dipasang pada body (sekali), bukan pada tiap inner (verifikasi semantik Zeebe)

onInnerDone(body, inner):
  if outputElement: Outputs[i] = eval(outputElement) di scope inner (lookup berantai)
  Done++
  if completionCondition && eval(cond) di scope inner == true:
      for other inner aktif: terminateSubtree(other)        # sisa iterasi sequential tidak dibuat
      completeBody(body); return
  sequential && NextIndex < len(items): startIteration(body, NextIndex++)
  else if Done == len(items): completeBody(body)

completeBody(body):
  if outputCollection: VARIABLE CREATED/UPDATED{scope: body.FlowScopeKey (root sampai Fase 7), name: outputCollection, value: Outputs (null untuk iterasi dihentikan)}
  ELEMENT_COMPLETING/COMPLETED body → takeOutgoing(A)
```

Element instance inner memakai `ElementID` yang sama dengan body; Operate membedakannya lewat
`bpmn_element_type` (`MULTI_INSTANCE_BODY` vs tipe asli), seperti Zeebe. Variabel lokal inner
(`inputElement`, `loopCounter`) disimpan dengan `scope_key = inner.Key`; `GetProcessInstance`
mengembalikannya dengan `scope_key` tersebut.

### 5.7 Terminate end event dan event-based gateway

- Terminate: `activate(endEvent)` → untuk setiap saudara non-final di `FlowScopeKey` (kecuali
  dirinya) `terminateSubtree`; counter `TakenFlows` scope dihapus; `ELEMENT_COMPLETED` end event →
  `onElementFinal` → scope selesai normal.
- Event-based gateway: `ACTIVATED`; untuk setiap outgoing `f` dengan target `t`: timer catch →
  `TIMER CREATED{owner: gw, target: t}`; message catch / receive task → `MESSAGE_SUBSCRIPTION
  CREATED{owner: gw, target: t}` + cek buffer. Pemicu: 5.3 cabang gateway. Gateway tidak pernah
  punya anak; ia selesai (`COMPLETED`) tepat sebelum `SEQUENCE_FLOW_TAKEN`.

## 6. Timer

### 6.1 Tabel `timer` (migrasi `timer.sql`, milik `execution`)

```sql
-- +goose Up
CREATE TABLE timer (
    key                     BIGINT PRIMARY KEY,
    tenant_id               TEXT NOT NULL DEFAULT '<default>',
    process_instance_key    BIGINT REFERENCES process_instance (key),   -- NULL untuk timer start event
    element_instance_key    BIGINT,                                     -- pemilik (host / scope / gateway / catch event); NULL untuk start
    process_definition_key  BIGINT NOT NULL REFERENCES process_definition (key),
    element_id              TEXT NOT NULL,                              -- elemen pemilik (untuk start: start event)
    target_element_id       TEXT NOT NULL,                              -- elemen yang diaktifkan saat terpicu
    due_at                  TIMESTAMPTZ NOT NULL,
    repetition              INT NOT NULL DEFAULT 0,                     -- pemicu ke-n (0 = pertama)
    remaining               INT NOT NULL DEFAULT 0,                     -- sisa pengulangan setelah ini; -1 = tak terbatas
    cycle                   TEXT,                                       -- raw timeCycle untuk re-schedule
    state                   TEXT NOT NULL CHECK (state IN ('CREATED', 'DUE', 'TRIGGERED', 'CANCELED')),
    claimed_until           TIMESTAMPTZ,                                -- lease klaim sweeper (state DUE)
    claimed_by              TEXT,                                       -- replika (hostname:pid)
    created_at              TIMESTAMPTZ NOT NULL,
    updated_at              TIMESTAMPTZ NOT NULL
);
CREATE INDEX timer_due_idx ON timer (due_at) WHERE state = 'CREATED';
CREATE INDEX timer_claimed_idx ON timer (claimed_until) WHERE state = 'DUE';
CREATE INDEX timer_by_instance_idx ON timer (process_instance_key) WHERE state IN ('CREATED', 'DUE');
CREATE INDEX timer_start_idx ON timer (process_definition_key) WHERE process_instance_key IS NULL AND state IN ('CREATED', 'DUE');

-- +goose Down
DROP TABLE timer;
```

`DUE` adalah state **klaim scheduler**, bukan record: projector tidak pernah menulisnya, dan
`LoadRuntimeState` memperlakukan `CREATED` dan `DUE` sama (timer aktif). Rebuild Fase 3 menulis
ulang `CREATED` untuk timer aktif; klaim yang hilang hanya berarti sweeper mengklaim lagi.

### 6.2 Sweeper: klaim dan pemicu (ADR-0024)

```sql
-- name: ClaimDueTimers :many
UPDATE timer SET state = 'DUE', claimed_until = $2, claimed_by = $3, updated_at = $1
WHERE key IN (
    SELECT key FROM timer
    WHERE (state = 'CREATED' AND due_at <= $1)
       OR (state = 'DUE' AND claimed_until < $1)        -- klaim kedaluwarsa (replika mati)
    ORDER BY due_at
    LIMIT $4
    FOR UPDATE SKIP LOCKED)
RETURNING key, process_instance_key, process_definition_key, element_id, target_element_id, repetition;
```

```
Sweeper (modul timer, platform/sweeper, tiap ENGINE_TIMER_SWEEP_INTERVAL):
  1. tx1: rows := ClaimDueTimers(now, now+ENGINE_TIMER_CLAIM_LEASE, replica, batch); COMMIT
  2. untuk setiap row (paralel maks ENGINE_TIMER_TRIGGER_WORKERS, default 8):
       if process_instance_key == NULL:
            execution/api.ProcessInstances.CreateFromStart{DefinitionKey, StartElementID, Trigger: TIMER, TimerKey, Repetition}
            lalu timer/application: reschedule start timer (6.5) atau tandai TRIGGERED — dalam SATU transaksi
       else:
            execution/api.Timers.Trigger(ctx, TriggerTimer{TimerKey, Repetition})
              → tx2: LockInstance; LoadRuntimeState; Processor:
                     timer tidak ada / state ≠ aktif / repetition ≠ → ErrTimerNotActive (no-op, log DEBUG, metrik timer_trigger_stale_total)
                     else: TIMER TRIGGERED → fire(owner, target) (5.3) → ...; COMMIT
  3. bila langkah 2 gagal (error DB), klaim dibiarkan; akan kedaluwarsa dan diklaim ulang
  4. ulangi sampai ClaimDueTimers mengembalikan 0 baris, lalu tunggu tick berikutnya
```

Mengapa dua transaksi: command instance (tx2) memegang lock instance dan mengubah baris `timer`
(projector menulis `TRIGGERED`/`CANCELED`). Jika sweeper memegang row lock `timer` (tx1) sambil
menunggu lock instance, dan command lain memegang lock instance sambil menunggu row lock `timer`
→ deadlock. Dengan klaim yang di-commit, tx2 hanya memegang lock instance; `UPDATE timer` di
projector bersaing hanya dengan `ClaimDueTimers` yang `SKIP LOCKED`.

At-least-once: replika mati setelah tx1 → `claimed_until` lewat → diklaim ulang. Replika mati
setelah tx2 commit tetapi sebelum selesai → baris sudah `TRIGGERED`, klaim ulang tidak
menemukannya. Dua replika tidak pernah memegang klaim yang sama (`FOR UPDATE SKIP LOCKED`), dan
kalaupun klaim kedaluwarsa lalu replika lama hidup lagi, `TriggerTimer` kedua menjadi no-op karena
state sudah `TRIGGERED` di bawah lock instance. Idempotensi command adalah jaminan utama; klaim
hanya menghindari pekerjaan ganda.

Record untuk timer instance ditulis oleh Processor; sweeper tidak menulis ke `execution_record`.

### 6.3 Evaluasi ISO 8601 (tanpa FEEL)

Helper publik di `libs/go/bpmn/iso8601.go` (stdlib saja, dipakai domain lewat allowlist kernel):

```go
// ParseISODuration mengurai "P1DT2H30M", "PT0.5S". Y/M/W/D disimpan terpisah karena bulan/tahun
// tidak punya panjang tetap; AddTo(t) memakai t.AddDate untuk Y/M/D lalu menambah bagian waktu.
type Duration struct{ Years, Months, Weeks, Days int; Time time.Duration }
func ParseISODuration(s string) (Duration, error)
func (d Duration) AddTo(t time.Time) time.Time
func (d Duration) IsZero() bool

// ParseISODateTime mengurai RFC 3339 dengan offset, opsional "[Area/City]" (zone id dipakai
// untuk time.LoadLocation; gagal → error TIMER_ZONE_UNKNOWN saat deploy/aktivasi).
func ParseISODateTime(s string) (time.Time, error)

// ParseTimerCycle mengurai "R/PT1H", "R3/PT10M", "R/2026-01-01T00:00:00Z/P1D", "R2/P1D/2026-12-31T00:00:00Z".
type Cycle struct{ Repetitions int /* -1 = tak terbatas */; Start *time.Time; Interval Duration; End *time.Time }
func ParseTimerCycle(s string) (Cycle, error)
func (c Cycle) IsCron() bool // 6 field; Fase 5 menolak saat deploy
```

`timer/domain.NextDue(def bpmn.Timer, now time.Time, repetition int32) (due time.Time, remaining int32, ok bool)`:

| Jenis | `due` pada repetition 0 | Pengulangan |
|---|---|---|
| `timeDuration` | `now + d` (`d.IsZero()` → `now`) | tidak ada |
| `timeDate` | tanggal; bila sudah lewat → `now` (terpicu sekali segera) | tidak ada |
| `timeCycle R[n]/D` | `now + D` | `remaining = n-1` (atau -1); berikutnya `due_prev + D` (bukan `now + D`, agar tidak melayang; verifikasi perilaku Zeebe) |
| `timeCycle R[n]/start/D` | `start` bila ≥ now, kalau tidak `start + k·D` pertama ≥ now (lompat yang terlewat; verifikasi) | sama |
| `timeCycle R[n]/D/end` | `now + D`, berhenti bila `> end` | sama |

Zone id: `time.LoadLocation` butuh tzdata; image distroless memakai `time/tzdata` yang di-embed
(`import _ "time/tzdata"` di `cmd/engine`, +450 KB).

### 6.4 Timer pada elemen (ringkasan perilaku)

| Tempat | Pemilik timer | `target_element_id` | Saat terpicu | Dibatalkan saat |
|---|---|---|---|---|
| Intermediate catch | catch event itu sendiri | = element_id | catch event selesai, outgoing | scope dihentikan |
| Boundary interrupting | host | boundary | `terminateSubtree(host)`, boundary aktif | host final, scope dihentikan |
| Boundary non-interrupting (`timeCycle`) | host | boundary | boundary aktif sebagai saudara; timer baru `repetition+1` bila `remaining ≠ 0` | host final |
| Event sub-process start (interrupting) | scope induk | start event | saudara di scope dihentikan, ESP aktif | scope final / diinterupsi |
| Event sub-process start (non-interrupting, boleh cycle) | scope induk | start event | ESP aktif sejajar | scope final |
| Event-based gateway | gateway | catch event target | lihat 5.7 | pemicu lain menang |
| Timer start (proses) | — (`process_instance_key NULL`) | start event | instance baru | versi baru di-deploy |

### 6.5 Timer start event

```
Consumer durable timer.start_scheduler atas deployment.process_deployed (Fase 3 relay, handler dalam tx relay):
  def := deployment/api.GetByKey(event.ProcessDefinitionKey)
  UPDATE timer SET state='CANCELED' WHERE process_instance_key IS NULL AND state IN ('CREATED','DUE')
     AND process_definition_key IN (SELECT key FROM process_definition WHERE bpmn_process_id = $1 AND tenant_id = $2 AND key <> $3)
  for s in def.Process.StartEvents() with Timer definition:
      due, remaining := NextDue(s, now, 0)
      INSERT timer (key=keygen, process_instance_key=NULL, process_definition_key, element_id=s.ID(), target_element_id=s.ID(), due_at, remaining, cycle, state='CREATED')
  (deploy duplikat tidak menghasilkan event → tidak ada perubahan)

Pemicu (sweeper, timer start):
  tx: CreateFromStart{Definition: key, StartElementID, Trigger: TIMER, TimerKey}   # instance baru; PROCESS_INSTANCE_CREATION CREATED{trigger: TIMER, timer_key}
      if remaining != 0: UPDATE timer SET state='CREATED', due_at = NextDue(..., repetition+1), repetition+1, remaining-1, claimed_until=NULL
      else:              UPDATE timer SET state='TRIGGERED'
  COMMIT
```

Participant `timer` (saat start engine, sebelum ready): untuk setiap `bpmn_process_id` dengan
versi terbaru yang punya timer start, pastikan ada tepat satu baris aktif per start event (buat
bila hilang, batalkan milik versi lama) — rekonsiliasi idempoten yang menutup celah bila consumer
pernah dead-letter; lalu log jumlah timer `CREATED` dengan `due_at < now` (tertunda selama engine
mati) dan metrik `engine_timer_overdue_at_start`.

Timer start pada versi yang bukan terbaru tidak pernah aktif (sama dengan Zeebe; verifikasi).
Instance yang sudah berjalan dari versi lama tidak terpengaruh.

### 6.6 Urutan interaksi: boundary timer non-interrupting dengan cycle (v08 `reminder`, `R3/PT10M`)

```
t=0     CompleteJob … → work ACTIVATED
        TIMER CREATED{key=T1, owner=work, target=reminder, due=t+10m, repetition=0, remaining=2}
        TIMER CREATED{key=T2, owner=work, target=timeout,  due=t+30m, repetition=0, remaining=0}
t=10m   sweeper: Claim(T1) → TriggerTimer(T1,0)
        TIMER TRIGGERED{T1,0}
        ELEMENT_ACTIVATING/ACTIVATED/COMPLETING/COMPLETED reminder   (flow scope = scope work)
        SEQUENCE_FLOW_TAKEN f4 → reminded …
        TIMER CREATED{key=T3, owner=work, target=reminder, due=t+20m, repetition=1, remaining=1}
t=20m   … T3 → T4 (repetition=2, remaining=0)
t=25m   CompleteJob(work) → work COMPLETING/COMPLETED
        TIMER CANCELED{T4, HOST_COMPLETED}; TIMER CANCELED{T2, HOST_COMPLETED}
```

Setiap pengulangan adalah timer baru dengan key baru (bukan baris yang di-update), sehingga
record dan replay sederhana, dan `TriggerTimer{T1,0}` yang diulang tidak bisa memicu pengulangan
berikutnya.

## 7. Message

### 7.1 Tabel

```sql
-- migrasi message_subscription.sql (milik execution)
CREATE TABLE message_subscription (
    key                     BIGINT PRIMARY KEY,
    tenant_id               TEXT NOT NULL DEFAULT '<default>',
    process_instance_key    BIGINT NOT NULL REFERENCES process_instance (key),
    element_instance_key    BIGINT NOT NULL,          -- pemilik (catch event / host / scope / gateway)
    element_id              TEXT NOT NULL,
    target_element_id       TEXT NOT NULL,
    message_name            TEXT NOT NULL,
    correlation_key         TEXT NOT NULL,
    interrupting            BOOLEAN NOT NULL,
    state                   TEXT NOT NULL CHECK (state IN ('OPEN', 'CORRELATING', 'CORRELATED', 'DELETED')),
    correlating_message_key BIGINT,                   -- diisi sisi publish; dikosongkan bila command gagal/no-op
    created_at              TIMESTAMPTZ NOT NULL,
    updated_at              TIMESTAMPTZ NOT NULL
);
CREATE INDEX message_subscription_match_idx ON message_subscription (tenant_id, message_name, correlation_key, key)
    WHERE state = 'OPEN';
CREATE INDEX message_subscription_by_instance_idx ON message_subscription (process_instance_key)
    WHERE state IN ('OPEN', 'CORRELATING');

-- milik modul message (perluasan tabel message Fase 4 + dua tabel baru)
CREATE TABLE message_start_subscription (
    process_definition_key  BIGINT NOT NULL REFERENCES process_definition (key),
    start_element_id        TEXT NOT NULL,
    bpmn_process_id         TEXT NOT NULL,
    tenant_id               TEXT NOT NULL DEFAULT '<default>',
    message_name            TEXT NOT NULL,
    active                  BOOLEAN NOT NULL DEFAULT true,              -- false setelah versi baru
    PRIMARY KEY (process_definition_key, start_element_id)
);
CREATE INDEX message_start_subscription_name_idx ON message_start_subscription (tenant_id, message_name) WHERE active;

CREATE TABLE message_correlation (                                     -- keunikan korelasi
    message_key             BIGINT NOT NULL,
    process_instance_key    BIGINT NOT NULL,
    bpmn_process_id         TEXT NOT NULL,
    message_name            TEXT NOT NULL,
    correlation_key         TEXT NOT NULL,
    kind                    TEXT NOT NULL CHECK (kind IN ('SUBSCRIPTION', 'START')),
    created_at              TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (message_key, process_instance_key)
);
-- Keunikan message start: satu instance aktif per (name, key, process). Baris dihapus oleh consumer
-- execution.process_instance_completed/terminated (modul message berlangganan).
CREATE UNIQUE INDEX message_correlation_start_uidx ON message_correlation (message_name, correlation_key, bpmn_process_id)
    WHERE kind = 'START' AND correlation_key <> '';
```

`message_correlation` menjawab dua pertanyaan: "sudahkah message M dikorelasikan ke instance I"
(PK) dan "adakah instance aktif yang dimulai message start `(name, key)` untuk proses P" (indeks
unik parsial). Keduanya diverifikasi terhadap dokumentasi Camunda 8.8; bila Zeebe tidak membatasi
yang kedua, indeks dihapus dan DoD disesuaikan.

### 7.2 Subscription per elemen

Saat elemen catch message aktif (5.3): `correlationKey` dievaluasi dengan subset Fase 2 di scope
elemen (lookup berantai) → harus string atau angka (dikonversi ke string kanonik; verifikasi
konversi Zeebe) → selain itu incident `EXTRACT_VALUE_ERROR` pada elemen pemilik. Message start
event **tidak** punya correlation key di model (`CorrelationKey()` false): korelasi start memakai
`correlation_key` dari `PublishMessage` apa adanya.

### 7.3 Dua titik korelasi (ADR-0025)

**Sisi subscribe** (di dalam transaksi command yang menghasilkan `MESSAGE_SUBSCRIPTION CREATED`,
dieksekusi `application` setelah `Processor` tetapi sebelum commit — bagian dari
`command_runner`):

```
for s in recordsOfType(MESSAGE_SUBSCRIPTION CREATED):
    pg_advisory_xact_lock(hashtext('msg:' || s.Name || '/' || s.Key))          # lane lock (instance lock sudah dipegang)
    m, ok := message/api.Buffer.Claim(ctx, s.Name, s.Key, instanceKey)
          # SELECT … FROM message WHERE name=$1 AND correlation_key=$2 AND expires_at > $now
          #   AND NOT EXISTS (SELECT 1 FROM message_correlation WHERE message_key = message.key AND process_instance_key = $3)
          # ORDER BY key LIMIT 1; lalu INSERT message_correlation{SUBSCRIPTION}
    if ok: records += Processor.Process(CorrelateMessage{s.Key, m.Key, m.Variables})   # masih dalam transaksi yang sama
```

Karena ini dijalankan di dalam loop command (bisa membuat subscription baru lagi, mis. loop
receive task), `command_runner` mengulang sampai tidak ada subscription baru atau tidak ada
message cocok; batas record Fase 2 tetap berlaku.

**Sisi publish** (`message/application/publish_message.go`, transaksi publish):

```
BEGIN
  pg_advisory_xact_lock(hashtext('msg:' || name || '/' || correlation_key))
  INSERT message (… expires_at = now + ttl)   -- unique (name, correlation_key, message_id) aktif → AlreadyExists (Fase 4)
  -- subscription non-start
  subs := SELECT key, process_instance_key FROM message_subscription
          WHERE tenant_id=$t AND message_name=$1 AND correlation_key=$2 AND state='OPEN'
            AND NOT EXISTS (SELECT 1 FROM message_correlation c WHERE c.message_key=$m AND c.process_instance_key = message_subscription.process_instance_key)
          ORDER BY key FOR UPDATE SKIP LOCKED          -- baris subscription milik execution; UPDATE di bawah lewat execution/api.Subscriptions.MarkCorrelating(ctx, keys, messageKey)
  untuk tiap instance berbeda di subs (satu subscription per instance; yang terkecil key-nya):
      UPDATE message_subscription SET state='CORRELATING', correlating_message_key=$m
      INSERT message_correlation{m, instance, SUBSCRIPTION}
      outbox.Enqueue(topic "message.correlated", aggregate = process_instance_key, payload {subscription_key, message_key, variables})
  -- message start
  starts := SELECT … FROM message_start_subscription WHERE tenant_id=$t AND message_name=$1 AND active
  untuk tiap start (per bpmn_process_id):
      if correlation_key <> '' AND EXISTS message_correlation START (name, key, bpmn_process_id): lewati (instance aktif sudah ada)
      INSERT message_correlation{m, instance_key=keygen.Next(), bpmn_process_id, START}   -- key instance dialokasikan sekarang
      outbox.Enqueue(topic "message.start_matched", aggregate = instance_key, payload {definition_key, start_element_id, message_key, instance_key, variables})
  if ttl == 0 AND tidak ada korelasi: DELETE message (tidak di-buffer)
COMMIT
```

Apakah baris `message_subscription` boleh di-`UPDATE` oleh modul `message`? Tidak langsung: modul
`message` memanggil `execution/api.Subscriptions.MatchAndMarkCorrelating(ctx, name, key,
messageKey)` yang menjalankan kueri di atas di dalam transaksi yang sama (3.3). Tabel tetap milik
`execution`; kode SQL-nya ada di `execution/infrastructure`.

**Consumer durable `execution.message_correlation`** (topik `message.correlated`,
`message.start_matched`; handler dalam transaksi relay):

```
message.correlated:  CorrelateMessage{subscription_key, message_key, variables} dengan LockInstance
                     subscription tidak OPEN/CORRELATING atau correlating_message_key ≠ message_key → no-op (ErrSubscriptionNotActive):
                         DELETE message_correlation{message_key, instance}  (message boleh dikorelasikan ke instance ini lain kali)
                     else: MESSAGE_SUBSCRIPTION CORRELATED{message_key} → VARIABLE * (merge) → fire(owner, target) (5.3)
                           non-interrupting boundary / non-interrupting ESP: subscription baru OPEN dibuat (key baru) agar message berikutnya bisa masuk
message.start_matched: CreateFromStart{Key: instance_key, Definition, StartElementID, Trigger: MESSAGE, MessageKey, Variables}
                     instance_key sudah ada → no-op
```

Mengapa publish tidak mengkorelasikan sinkron: ia butuh lock instance target, sedangkan sisi
subscribe memegang lock instance lalu lane; publish yang memegang lane lalu instance membentuk
siklus. Harga: latensi satu interval relay; keuntungan: `PublishMessage` tidak pernah menunggu
command instance yang panjang, dan satu publish yang cocok ke 1.000 instance tidak mengunci 1.000
baris.

Urutan: dua message buffered dengan `(name, key)` sama dikorelasikan menurut `message_key`
(`ORDER BY key`) saat subscription dibuka berulang (loop receive task) — sesuai FIFO Zeebe
(verifikasi).

### 7.4 Urutan interaksi: publish sebelum dan sesudah subscription (v09 `awaitPayment`)

```
Kasus A — message dulu:
  PublishMessage(PaymentReceived, key=o-1, ttl=1h)      tx: lane(o-1) → INSERT message M1 → tidak ada subscription OPEN → tidak ada start → COMMIT
  CreateInstance(orderId=o-1) → … awaitPayment ACTIVATING
     MESSAGE_SUBSCRIPTION CREATED{S1, name, key=o-1}
     [command_runner] lane(o-1) → Buffer.Claim → M1 → CorrelateMessage{S1, M1}
     MESSAGE_SUBSCRIPTION CORRELATED{S1, M1} → VARIABLE … → awaitPayment COMPLETING/COMPLETED → f2 …
     COMMIT (satu transaksi, satu RPC)

Kasus B — subscription dulu:
  CreateInstance → awaitPayment ACTIVATED; S1 OPEN; cek buffer kosong; COMMIT
  PublishMessage(…)  tx: lane(o-1) → INSERT M1 → S1 ditemukan → S1 CORRELATING → message_correlation(M1, I) → outbox message.correlated → COMMIT
  relay → consumer execution.message_correlation → CorrelateMessage{S1, M1}: LockInstance(I) → CORRELATED → … → COMMIT
```

### 7.5 Consumer `message.start_subscription` dan participant

Consumer durable atas `deployment.process_deployed`: `UPDATE message_start_subscription SET
active=false WHERE bpmn_process_id=$1 AND process_definition_key<>$2`; `INSERT` satu baris per
message start event versi baru. Participant `message` merekonsiliasi tabel terhadap versi terbaru
saat start (idempoten) dan membersihkan `message_correlation` kind `START` yang instance-nya sudah
final (cadangan bila consumer `process_instance_completed/terminated` tertinggal).

Sweeper TTL Fase 4 tetap: message kedaluwarsa dihapus (baris `message_correlation` ikut lewat
`ON DELETE CASCADE` pada `message_key` — ditambahkan sebagai FK).

## 8. Error dan escalation

### 8.1 Pencarian catch (`catch_index.go`, ADR-0028)

Indeks statis per `*bpmn.Process` (dihitung sekali, cache per `process_definition_key`, seperti
`Upstream` Fase 2):

```go
type catchIndex struct {
	// boundary[activityID] = daftar boundary event error/escalation (urutan dokumen)
	boundary map[string][]*bpmn.Event
	// eventSub[scopeOwnerID] = daftar start event error/escalation dari event sub-process di scope itu ("" untuk scope proses)
	eventSub map[string][]*bpmn.Event
}

type catchResult struct {
	Event    *bpmn.Event // boundary atau start event ESP
	OwnerKey int64       // element instance host (boundary) atau scope (ESP) yang akan di-fire
}

// findCatch berjalan dari elemen pelempar ke luar. kind = Error|Escalation.
func (p *Processor) findCatch(state *InstanceState, from *ElementInstance, kind, code string) (catchResult, bool) {
	cur := from
	for cur != nil {
		el := process.Element(cur.ElementID)
		if act, ok := el.(*bpmn.Activity); ok && cur.ElementType != MultiInstanceBodyType {
			if ev, ok := best(idx.boundary[act.ID()], kind, code); ok { return catchResult{ev, cur.Key}, true }
		}
		scope := state.Elements[cur.FlowScopeKey]            // nil bila cur adalah PROCESS
		scopeID := ""
		if scope != nil { scopeID = scope.ElementID }
		if ev, ok := best(idx.eventSub[scopeID], kind, code); ok {
			owner := scope; if owner == nil { owner = cur }
			return catchResult{ev, owner.Key}, true
		}
		cur = scope
	}
	return catchResult{}, false
}

// best: exact code menang; lalu catch-all (tanpa ref / code kosong); tidak ada → false.
```

Catatan:

- Untuk pelempar berupa **end event** (bukan activity), langkah boundary dilewati secara alami
  (end event bukan `*bpmn.Activity`), sehingga urutannya: ESP di scope-nya → boundary owner scope →
  ESP scope di atasnya → …
- Untuk **job** pada service task: boundary service task sendiri dulu.
- Untuk **multi-instance**: boundary dipasang pada body (5.6); inner melewatkan langkah boundary
  dan naik ke body, yang memeriksanya.
- Urutan "boundary elemen dulu, baru ESP scope" dan prioritas exact vs catch-all **harus
  diverifikasi terhadap dokumentasi Camunda 8.8**; skenario paritas `error-nearest-wins` dan
  `error-exact-before-catch-all` menjadi bukti dan menentukan implementasi final.

### 8.2 Melempar error

```
throwError(from, code, msg, source):
  emit ERROR THROWN{from.Key, code, msg, source}
  res, ok := findCatch(state, from, Error, code)
  if !ok:
      switch source:
        JOB:            (Fase 4) INCIDENT CREATED{UNHANDLED_ERROR_EVENT} pada from; job tetap ERROR_THROWN
        END_EVENT:      INCIDENT CREATED{UNHANDLED_ERROR_EVENT} pada end event (state ACTIVATING; verifikasi)
        CHILD_INSTANCE: (lihat di bawah)
      return
  emit ERROR CAUGHT{catch: res.Event.ID()}
  if source == END_EVENT: complete(from)                 # end event selesai dulu (verifikasi record Zeebe)
  fire(res.OwnerKey, res.Event, payload{variables dari ThrowError})   # 5.3: interrupting boundary → terminateSubtree(owner) termasuk `from`
```

**Lintas call activity** (verifikasi perilaku Camunda 8.8: penulis memahami bahwa error yang tidak
tertangkap di proses anak dipropagasikan ke call activity induk sejak Zeebe 8.x):

```
Anak: findCatch gagal pada instance anak dengan Parent != nil
      → instance anak TERMINATING/TERMINATED (terminateSubtree(PROCESS)), reason ERROR_PROPAGATED
      → outbox cross_instance.child_error{parent, element_instance_key, code, msg}
Induk (consumer): ThrowErrorFromChild{ei, code, msg}: ei ACTIVATED? → throwError(ei, code, msg, CHILD_INSTANCE)
      tidak tertangkap di induk → incident UNHANDLED_ERROR_EVENT pada call activity; bila induk juga anak → naik lagi
```

Bila verifikasi menunjukkan Zeebe membuat incident di anak dan **tidak** mempropagasikan, cabang
`CHILD_INSTANCE` dihapus dan incident dibuat di anak (seperti `END_EVENT`).

`ThrowError` dari `JobService` (Fase 4) tidak berubah kontraknya: `execution/api.Jobs.ThrowError`
→ command `ThrowError` → `JOB ERROR_THROWN` → `throwError(serviceTask, code, msg, JOB)`. Job yang
jadi `ERROR_THROWN` tetap final meskipun tertangkap (sama dengan Zeebe); elemen service task
dihentikan oleh `terminateSubtree(owner)` bila catcher interrupting, atau — bila boundary
non-interrupting error? Tidak ada: error boundary selalu interrupting (BPMN); validator Fase 1
menolak `cancelActivity="false"` pada error boundary (aturan ditambahkan ke 4.3 bila belum ada:
`ERROR_BOUNDARY_NOT_INTERRUPTING`).

### 8.3 Escalation

```
throwEscalation(from, code):
  emit ESCALATION THROWN
  res, ok := findCatch(state, from, Escalation, code)
  if ok: emit ESCALATION CAUGHT; fire(res.OwnerKey, res.Event, nil)   # boundary interrupting → terminateSubtree(owner); non-interrupting → sejajar
  complete(from)                                                      # throw event SELALU selesai normal (urutan: fire dulu agar interrupting menghentikan from juga? → verifikasi)
```

Urutan `fire` vs `complete(from)` menentukan apakah escalation end event di dalam sub-process
yang ditangkap boundary interrupting sub-process tersebut sempat `COMPLETED` (verifikasi record
Camunda 8.8; skenario `escalation-interrupting-end-event`). Escalation tidak tertangkap: tanpa
incident, tanpa log selain DEBUG. Lintas call activity: sama dengan error (`cross_instance.child_escalation`),
tetapi anak **tidak** dihentikan (verifikasi: Camunda 8 mendukung propagasi escalation ke call
activity induk).

### 8.4 Incident baru

| Tipe | Kondisi | Elemen |
|---|---|---|
| `UNHANDLED_ERROR_EVENT` (Fase 4) | Error tidak tertangkap | service task (job), error end event, call activity (dari anak) |
| `CALLED_ELEMENT_ERROR` | `processId` tidak ditemukan / tidak executable / kedalaman > batas | call activity `ACTIVATING` |
| `EXTRACT_VALUE_ERROR` (Fase 2) | `correlationKey`, `inputCollection`, `outputElement`, `completionCondition`, `processId` gagal dievaluasi atau tipe salah | elemen pemilik |
| `TIMER_ERROR` | Timer tidak bisa dijadwalkan (zone id tidak dikenal, cycle habis saat aktivasi) | catch/boundary host `ACTIVATING` |

`ResolveIncident` (Fase 4, "langkah ulang"): `CALLED_ELEMENT_ERROR` → evaluasi ulang `processId`
(mungkin proses anak sudah di-deploy); `EXTRACT_VALUE_ERROR` pada subscription → evaluasi ulang
key; `TIMER_ERROR` → jadwalkan ulang; `UNHANDLED_ERROR_EVENT` → tetap butuh perbaikan model (Zeebe:
resolve mencoba ulang pencarian catch? verifikasi; default kita: cari catch ulang, gagal → incident
baru).

## 9. Persistensi dan pemulihan

### 9.1 Migrasi `scope.sql`

```sql
-- +goose Up
ALTER TABLE process_instance
    ADD COLUMN parent_process_instance_key  BIGINT REFERENCES process_instance (key),
    ADD COLUMN parent_element_instance_key  BIGINT,
    ADD COLUMN start_element_id             TEXT,            -- NULL = none start (Fase 2)
    ADD COLUMN trigger                      TEXT NOT NULL DEFAULT 'API';  -- API, TIMER, MESSAGE, CALL_ACTIVITY
CREATE INDEX process_instance_parent_idx ON process_instance (parent_process_instance_key) WHERE parent_process_instance_key IS NOT NULL;

ALTER TABLE element_instance
    ADD COLUMN called_process_instance_key  BIGINT,          -- call activity
    ADD COLUMN interrupted                  BOOLEAN NOT NULL DEFAULT false;
CREATE INDEX element_instance_scope_idx ON element_instance (flow_scope_key)
    WHERE state NOT IN ('COMPLETED', 'TERMINATED');       -- anak aktif per scope (verify, terminateSubtree saat rebuild)

-- scope lokal: variable.scope_key kini bisa = element_instance.key (multi-instance inner)
-- tidak perlu perubahan skema; UNIQUE (scope_key, name) sudah ada.

CREATE TABLE cross_instance_applied (                       -- idempotensi internal command (5.5)
    child_key   BIGINT NOT NULL,
    intent      TEXT NOT NULL,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (child_key, intent)
);

-- +goose Down
DROP TABLE cross_instance_applied;
DROP INDEX element_instance_scope_idx;
ALTER TABLE element_instance DROP COLUMN interrupted, DROP COLUMN called_process_instance_key;
DROP INDEX process_instance_parent_idx;
ALTER TABLE process_instance DROP COLUMN trigger, DROP COLUMN start_element_id,
    DROP COLUMN parent_element_instance_key, DROP COLUMN parent_process_instance_key;
```

Semua kolom aditif (expand); tidak ada backfill (nilai default benar untuk instance Fase 2–4).
Retensi Fase 3 menghapus instance anak bersama induk? Tidak: anak adalah instance final sendiri
dengan `end_time`; FK `parent_process_instance_key` memakai `ON DELETE SET NULL` agar induk yang
dipangkas lebih dulu tidak menggagalkan janitor (ditambahkan ke DDL di atas saat implementasi).

### 9.2 `LoadRuntimeState` dan projector

- `LoadRuntimeState` memuat tambahan: timer `CREATED/DUE` instance, subscription
  `OPEN/CORRELATING`, variabel **semua scope** instance (bukan hanya root), `parent_*`,
  `called_process_instance_key`, `MultiInstanceState` (disimpan sebagai JSON di kolom baru
  `element_instance.multi_instance_state JSONB`, ditulis hanya untuk `MULTI_INSTANCE_BODY`).
- Projector: `TIMER *` → upsert `timer` (klaim `DUE` tidak disentuh kecuali `TRIGGERED/CANCELED`
  menimpanya), `MESSAGE_SUBSCRIPTION *` → upsert `message_subscription`,
  `PROCESS_INSTANCE_CREATION CREATED` → `parent_*`, `trigger`, `start_element_id`; `ELEMENT_*` →
  kolom baru; `VARIABLE` dengan `scope_key` lokal. Semua upsert (Fase 3 DoD idempotensi projector).
- Kueri verifikasi Fase 3 baru: `VerifyOrphanTimers` (timer aktif dengan element instance final
  atau instance final), `VerifyOrphanSubscriptions`, `VerifyScopeTree` (element instance non-final
  yang `flow_scope_key`-nya final), `VerifyCallActivityLinks` (call activity `ACTIVATED` tanpa anak
  aktif selama > `ENGINE_OUTBOX_RETENTION` → kemungkinan event hilang).

### 9.3 Snapshot `format_version = 2`

```go
type Snapshot struct {
	FormatVersion int // 2
	Sequence      int32
	Parent        *ParentRef
	Elements      []ElementInstance // termasuk MultiInstanceState, CalledInstanceKey, Interrupted
	TakenFlows    []TakenFlow
	Variables     []VariableEntry   // + ScopeKey
	Jobs          []Job
	Incidents     []Incident
	Timers        []Timer           // state CREATED (DUE dinormalkan ke CREATED)
	Subscriptions []MessageSubscription
}
```

`RestoreSnapshot` dengan `FormatVersion == 1` mengembalikan `ErrSnapshotFormat`; rebuild mulai
dari nol dengan WARN (perilaku Fase 3). Test: snapshot v1 yang dibuat test Fase 3 dibaca oleh kode
Fase 5 → mulai dari nol → state identik.

### 9.4 Participant pemulihan dan urutan start

Urutan `recovery.Runner` (Fase 3, 5.2) ditambah: `deployment` → `execution` → `message`
(rekonsiliasi start subscription, 7.5) → `timer` (rekonsiliasi timer start, 6.5; metrik timer
tertunda). Sweeper timer dan relay outbox baru berjalan setelah `readiness.Set(true)`; semua
timer yang jatuh tempo selama engine mati terpicu pada sweep pertama (batch 500 per tick, sehingga
10.000 timer tertunda selesai dalam ±20 tick × latensi command; DoD mencatat angka).

Konsumer durable baru dan nama stabilnya:

| Consumer | Topik | Modul | Efek |
|---|---|---|---|
| `timer.start_scheduler` | `deployment.process_deployed` | timer | timer start versi baru, batalkan versi lama |
| `message.start_subscription` | `deployment.process_deployed` | message | `message_start_subscription` |
| `message.instance_final` | `execution.process_instance_completed`, `…_terminated` | message | hapus `message_correlation` kind `START` instance itu |
| `execution.message_correlation` | `message.correlated`, `message.start_matched` | execution | `CorrelateMessage`, `CreateFromStart` |
| `execution.cross_instance` | `execution.cross_instance.*` (create_child, terminate_child, child_completed, child_terminated, child_error, child_escalation) | execution | internal command 5.5 / 8.2 |

Topik `execution.cross_instance.*` ditulis `execution/application/outbox_events.go` dari record
(`ELEMENT_ACTIVATED` call activity → `create_child`; `ELEMENT_TERMINATED` call activity dengan
anak aktif → `terminate_child`; `ELEMENT_COMPLETED/TERMINATED` PROCESS dengan `Parent` →
`child_completed`/`child_terminated`; `ERROR THROWN` tidak tertangkap pada instance anak →
`child_error`).

Skenario chaos Fase 3 baru (`tools/scripts/chaos`): `timer-missed-during-restart` (kill engine
saat 1.000 timer jatuh tempo; semua terpicu tepat sekali), `message-publish-during-restart`
(publish 500 message ke instance menunggu saat relay mati; semua terkorelasi setelah start),
`call-activity-parent-kill` (kill setelah `ELEMENT_ACTIVATED` call activity sebelum anak dibuat;
anak dibuat setelah restart).

## 10. Konfigurasi dan observability

| Variabel | Default | Keterangan |
|---|---|---|
| `ENGINE_TIMER_SWEEP_INTERVAL` | `500ms` | interval sweeper timer |
| `ENGINE_TIMER_SWEEP_BATCH` | `500` | timer per klaim |
| `ENGINE_TIMER_CLAIM_LEASE` | `30s` | masa klaim `DUE` |
| `ENGINE_TIMER_TRIGGER_WORKERS` | `8` | paralelisme `TriggerTimer` per sweep |
| `ENGINE_MAX_CALL_DEPTH` | `32` | kedalaman call activity |
| `ENGINE_MAX_MULTI_INSTANCE_ITEMS` | `10000` | batas `inputCollection`; lebih → incident `EXTRACT_VALUE_ERROR` |
| `ENGINE_MESSAGE_START_UNIQUE` | `true` | aktifkan indeks keunikan message start (dimatikan bila verifikasi Zeebe menyatakan tidak ada batasan) |

Log terstruktur: `timer claimed/triggered/stale`, `message correlated/buffered/start matched`,
`cross-instance command applied/no-op`, `error caught/uncaught`, `escalation uncaught`. Metrik
(`platform/metrics`): `engine_timer_due_total`, `engine_timer_trigger_latency_seconds` (due_at →
TRIGGERED), `engine_timer_trigger_stale_total`, `engine_timer_overdue_at_start`,
`engine_message_correlated_total{path=subscribe|publish|start}`, `engine_message_buffered`
(gauge), `engine_cross_instance_total{intent,outcome}`, `engine_error_total{outcome=caught|uncaught}`.

## 11. Kontrak proto dan UI

### 11.1 `workflow/process/v1` (aditif)

```proto
message ProcessInstance {
  // ... Fase 2–4
  int64 parent_process_instance_key = 9;   // 0 = root
  int64 parent_element_instance_key = 10;
  string start_element_id = 11;
  string trigger = 12;                     // API, TIMER, MESSAGE, CALL_ACTIVITY
}

message ElementInstance {
  // ... Fase 2
  int64 called_process_instance_key = 9;   // call activity
}

message Timer {
  int64 timer_key = 1;
  int64 element_instance_key = 2;
  string element_id = 3;
  string target_element_id = 4;
  google.protobuf.Timestamp due_at = 5;
  int32 repetition = 6;
  int32 remaining = 7;
  TimerState state = 8;
}
enum TimerState { TIMER_STATE_UNSPECIFIED = 0; TIMER_STATE_CREATED = 1; TIMER_STATE_DUE = 2; TIMER_STATE_TRIGGERED = 3; TIMER_STATE_CANCELED = 4; }

message MessageSubscription {
  int64 subscription_key = 1;
  int64 process_instance_key = 2;
  int64 element_instance_key = 3;
  string element_id = 4;
  string target_element_id = 5;
  string message_name = 6;
  string correlation_key = 7;
  bool interrupting = 8;
  MessageSubscriptionState state = 9;
}
enum MessageSubscriptionState { MESSAGE_SUBSCRIPTION_STATE_UNSPECIFIED = 0; MESSAGE_SUBSCRIPTION_STATE_OPEN = 1; MESSAGE_SUBSCRIPTION_STATE_CORRELATING = 2; MESSAGE_SUBSCRIPTION_STATE_CORRELATED = 3; MESSAGE_SUBSCRIPTION_STATE_DELETED = 4; }

message GetProcessInstanceResponse {
  // ... Fase 2–3
  repeated Timer timers = 7;                                 // aktif + 100 terakhir final
  repeated MessageSubscription message_subscriptions = 8;
  repeated ProcessInstance child_instances = 9;              // anak langsung (call activity), maks 100
}

message ListProcessInstancesRequest {
  // ... Fase 2
  int64 parent_process_instance_key = 7;                      // filter anak
  bool only_root = 8;
}

message CreateProcessInstanceRequest {
  // ... Fase 2–3; tidak ada start_instructions (di luar lingkup)
}
```

`Variable.scope_key` (Fase 2) kini bisa berisi key element instance inner multi-instance; tidak ada
perubahan tipe.

### 11.2 `workflow/message/v1` (aditif)

```proto
service MessageService {
  rpc PublishMessage(PublishMessageRequest) returns (PublishMessageResponse) {}   // Fase 4, tidak berubah
  rpc ListMessageSubscriptions(ListMessageSubscriptionsRequest) returns (ListMessageSubscriptionsResponse) {}
  rpc ListBufferedMessages(ListBufferedMessagesRequest) returns (ListBufferedMessagesResponse) {}
}
message ListMessageSubscriptionsRequest { string message_name = 1; string correlation_key = 2; int64 process_instance_key = 3; bool only_open = 4; workflow.common.v1.PageRequest page = 5; }
message ListMessageSubscriptionsResponse { repeated workflow.process.v1.MessageSubscription subscriptions = 1; workflow.common.v1.PageResponse page = 2; }
message ListBufferedMessagesRequest { string message_name = 1; string correlation_key = 2; workflow.common.v1.PageRequest page = 3; }
message BufferedMessage { int64 message_key = 1; string name = 2; string correlation_key = 3; string message_id = 4; google.protobuf.Timestamp published_at = 5; google.protobuf.Timestamp expires_at = 6; }
message ListBufferedMessagesResponse { repeated BufferedMessage messages = 1; workflow.common.v1.PageResponse page = 2; }

message PublishMessageResponse {
  int64 message_key = 1;
  string tenant_id = 2;
  int32 correlated_subscriptions = 3;   // baru: jumlah subscription yang ditandai CORRELATING + start yang dicocokkan (informasi, bukan jaminan)
}
```

`ErrorInfo.reason` baru (Fase 4, 8.4): `TIMER_NOT_ACTIVE`, `SUBSCRIPTION_NOT_ACTIVE` (hanya di
log consumer, tidak pernah ke klien), `CALLED_PROCESS_NOT_FOUND` (di pesan incident). Tidak ada
RPC untuk memicu timer secara manual di Fase 5 (pertanyaan terbuka 16.2 #9).

### 11.3 UI (modeler-web fitur `process-instance`, minimal; Operate penuh Fase 6)

- `domain/`: `Timer`, `MessageSubscription`, aturan marker: elemen dengan timer/subscription aktif
  → `token-waiting` (selain marker Fase 2); boundary event yang menunggu diberi overlay ikon jam /
  amplop dengan waktu jatuh tempo relatif.
- `application/`: `useProcessInstance` membaca `timers`, `message_subscriptions`,
  `child_instances`; `usePublishMessage` (mutation, dialog sederhana: name, key, variabel JSON)
  untuk uji manual korelasi.
- `presentation/`: panel "Menunggu" (tabel timer: elemen, jatuh tempo, sisa pengulangan; tabel
  subscription: elemen, nama, key, state), tautan "Instance induk" / daftar "Instance anak",
  `WaitingOverlay` pada viewer bpmn-js. Sub-process ditampilkan apa adanya oleh bpmn-js (expanded);
  marker pada elemen di dalam sub-process bekerja karena id unik.
- Fitur ini dipindah ke `operate-web` di Fase 6 bersama pohon instance React Flow.

## 12. Strategi uji

### 12.1 Lapisan

| Lapisan | Alat | Cakupan Fase 5 |
|---|---|---|
| Kernel `bpmn` | tabel + golden | `ParseISODuration/DateTime/TimerCycle` (≥ 60 kasus termasuk tepi: `P0D`, `PT0.5S`, zone id, `R0/…`), aturan validasi 4.3 dengan file invalid |
| Domain `execution` murni | skenario tabel + `clock.Fake` + evaluator subset | semua baris 4.1; determinisme (`Process` dua kali → record identik); replay (`Apply` ulang = state) |
| Property | `math/rand/v2` seed tercatat | 12.3 |
| Application + Postgres | `-tags=integration` | sweeper klaim (dua replika, lease kedaluwarsa), korelasi dua sisi (race publish vs subscribe dengan fault point `command.before_commit=sleep`), cross-instance, rebuild/verify, snapshot v2 |
| Handler + consumer | `httptest` + relay `Drain` | end-to-end: deploy → create → publish → `Drain` → get |
| Chaos | Compose (Fase 3) | tiga skenario 9.4 |
| Paritas | runner `parity` suite `events`, `scopes` | 12.4 |
| UI | Vitest | mapper, aturan marker `token-waiting` |

### 12.2 Skenario dan korpus

Fixture dari `libs/go/bpmn/testdata/valid` v08–v17, v19 dipakai langsung; file baru di
`execution/testdata/bpmn` hanya untuk pola yang tidak ada di korpus:

| File baru | Pola |
|---|---|
| `x01_subprocess_parallel_join.bpmn`, `x02_subprocess_inclusive_join.bpmn` | fork/join di dalam sub-process; join di induk setelah dua sub-process |
| `x03_nested_interrupt.bpmn` | boundary timer pada sub-process luar yang berisi sub-process dalam dengan job dan timer sendiri (interupsi bersarang) |
| `x04_event_subprocess_nested.bpmn` | event sub-process di dalam sub-process; interrupting vs non-interrupting; error catch-all di dua tingkat (`error-nearest-wins`, `error-exact-before-catch-all`) |
| `x05_call_activity_chain.bpmn` + `x05_child.bpmn`, `x05_grandchild.bpmn` | tiga tingkat; cancel di induk; error dari cucu |
| `x06_multi_instance_subprocess.bpmn` | multi-instance parallel atas sub-process dengan job dan boundary; completion condition |
| `x07_receive_loop.bpmn` | receive task dalam loop exclusive; dua message buffered dikonsumsi berurutan |
| `x08_message_start_unique.bpmn` | message start dengan correlation key; instance kedua ditolak selama yang pertama aktif |
| `x09_timer_start_cycle.bpmn` | timer start `R/PT1S` + versi kedua dengan `R/PT2S` |
| `x10_terminate_in_subprocess.bpmn` | terminate end di dalam sub-process tidak menghentikan proses |
| `x11_escalation_interrupting.bpmn` | escalation end event ditangkap boundary interrupting pada sub-process |

Skenario tabel (format Fase 2) ditambah langkah baru:

```go
type Step interface{ /* Fase 2: CompleteJob, FailJob, ThrowError, SetVariables, Cancel */ }
type AdvanceClock struct{ By time.Duration }          // clock.Fake.Advance + satu sweep (domain: panggil TriggerTimer untuk timer due)
type PublishMessage struct{ Name, Key, Vars string; TTL time.Duration }
type DrainOutbox struct{}                              // integration: relay.Drain; domain: jalankan internal command tertunda
type ExpectWaiting struct{ Timers []string; Subscriptions []string }   // element id yang menunggu
```

Di lapisan domain, "sweeper" dan "consumer" disimulasikan oleh harness: setelah setiap langkah,
harness menjalankan `TriggerTimer` untuk timer dengan `DueAt <= clock.Now()` dan menjalankan
internal command yang dihasilkan record (cross-instance, correlated) pada state instance target
in-memory. Harness memegang **banyak instance** (map key → `InstanceState`) untuk call activity.

### 12.3 Property test invarian interupsi

Generator: graf blok acak Fase 2 diperluas dengan blok **sub-process** (kedalaman ≤ 3), boundary
timer interrupting/non-interrupting acak pada activity, event sub-process (timer/message, interrupting
acak) di scope acak, call activity ke proses acak yang juga di-generate, multi-instance parallel
atas list acak 0–5 item, terminate end di cabang acak. Simulator: pada setiap langkah pilih acak
salah satu dari: selesaikan job acak, majukan jam ke timer berikutnya, publish message yang cocok
dengan subscription acak, cancel instance acak, drain internal command.

Properti setelah setiap langkah dan di akhir:

1. Tidak ada job/timer/subscription aktif yang `element_instance_key`-nya final, atau yang
   instance-nya final.
2. Setiap element instance non-final punya rantai `FlowScopeKey` non-final sampai `PROCESS`.
3. Scope `COMPLETED` tidak punya anak non-final dan tidak punya counter join.
4. Call activity `ACTIVATED` ⇔ anak `ACTIVE` (setelah drain); anak `TERMINATED` oleh induk ⇒ call
   activity `TERMINATED`.
5. Multi-instance body `COMPLETED` ⇒ `len(Outputs) == len(Items)` (null untuk yang dihentikan).
6. Replay semua record setiap instance = state in-memory; snapshot v2 di titik acak + sisa record =
   state.
7. Simulasi selesai (semua instance final) dalam ≤ N langkah bila tidak ada model macet (oracle
   graf memastikan generator hanya membuat model yang bisa selesai).

`go test -run TestInterruptionProperties` 1.000 iterasi < 60 s di CI; `make prop` 100.000.

### 12.4 Paritas Camunda 8.8

Runner `tools/scripts/parity` (Fase 2/4) ditambah suite `events` (timer, message, error,
escalation, event-based gateway) dan `scopes` (sub-process, event sub-process, call activity,
multi-instance, terminate). Perbandingan observable: selesai/incident, multiset tipe job yang
diaktifkan, variabel akhir, **dan** untuk Fase 5: himpunan element id yang `COMPLETED`/`TERMINATED`
bila API pencarian Camunda 8.8 tersedia di profil compose (verifikasi, catatan Fase 2 8.13).

- Timer: skenario memakai durasi pendek nyata (`PT2S`–`PT5S`) karena jam Camunda referensi tidak
  bisa dipalsukan dari luar secara stabil (Camunda 8 punya endpoint clock di actuator untuk
  pengujian — verifikasi ketersediaannya di image `camunda/zeebe:8.8.44`; bila ada, runner
  memakainya dan durasi di-set panjang).
- Message: publish lewat REST v2 `POST /v2/messages/publication` (verifikasi path), TTL, dedup
  `messageId`, keunikan message start.
- Error: `POST /v2/jobs/{key}/error` (verifikasi) dengan boundary cocok, tidak cocok, catch-all;
  error di proses anak call activity.
- Semua butir "verifikasi" di dokumen ini dipetakan ke minimal satu skenario paritas; hasil
  verifikasi dicatat di ADR terkait dan `expectations.json`.

## 13. Rencana kerja Fase 5 (urutan eksekusi)

Setiap langkah punya **hasil yang bisa diverifikasi**. 13.2 (kernel) bisa paralel dengan 13.3–13.5
(domain scope); 13.6 (timer) dan 13.8 (message) paralel setelah 13.5; 13.9–13.11 (error, call
activity, multi-instance) berurutan di atas 13.5.

### 13.1 Prasyarat dan branch

- Fase 4 merge (`ThrowError`, modul `message`, `platform/sweeper`, `platform/rpc`); Fase 3 merge
  (outbox, participant, snapshot). Bila `message` Fase 4 belum ada, langkah 13.8 membuat modul
  sesuai sketsa Fase 4 bagian 7 dan 8.3 lebih dulu.
- Branch `feat/phase-5-events-scopes` dari `main`; `make check`, `make test-integration` hijau.
- Verifikasi awal terhadap dokumentasi Camunda 8.8 untuk semua butir "verifikasi" bagian 3–8;
  hasil dicatat sebagai tabel di 0 sebelum kode ditulis (menghindari implementasi ganda).

Hasil: branch siap; tabel verifikasi terisi; bagian 0 diperbarui.

### 13.2 Kernel `bpmn`: ISO 8601 publik, profil, aturan baru

- `ParseISODuration`, `ParseISODateTime`, `ParseTimerCycle`, `Duration.AddTo` (6.3) + tabel uji ≥ 60
  kasus + fuzz `FuzzParseTimer` (tidak panic, hasil `AddTo` monoton untuk durasi positif).
- `executableInPhase5` (4.2); aturan 4.3 + file `testdata/invalid/i33…i41`; golden diperbarui.
- `go generate` README daftar kode issue.

Hasil: `nx run bpmn-go:test` hijau; v08–v17, v19 lolos `ProfileExecutable`; v18 ditolak.

### 13.3 Domain: pohon scope dan `terminateSubtree`

- `scope.go`: `isScope`, `onElementFinal`, `completeScope`, `terminateSubtree` (5.2, 5.4);
  `Apply` untuk element type baru dan invarian 5.4; `ChildrenByScope` dipertahankan untuk semua
  scope; `TakenFlows` per scope.
- Embedded sub-process (`behavior_subprocess.go`), terminate end event, `CancelInstance` memakai
  `terminateSubtree(PROCESS)` (menggantikan loop Fase 2).
- Skenario: v13, v17, x01, x02, x10; property test gateway Fase 2 + blok sub-process.

Hasil: skenario lolos di domain murni; determinisme dan replay hijau; property gateway 1.000 iterasi.

### 13.4 Domain: catch index, pemasangan boundary, `fire`

- `catch_index.go` (8.1, cache per definition key), `behavior_boundary.go` (5.3) dengan timer dan
  subscription sebagai **state abstrak** (record `TIMER`/`MESSAGE_SUBSCRIPTION`), `fire` untuk
  semua cabang (boundary interrupting/non-interrupting, ESP, gateway, catch sendiri).
- Command `TriggerTimer`, `CorrelateMessage` (domain saja; `ErrTimerNotActive`,
  `ErrSubscriptionNotActive`), event sub-process (`Interrupted`), event-based gateway.
- Skenario domain dengan `clock.Fake` + harness 12.2: v08, v09 (tanpa buffer), v14, v19, x03, x04.

Hasil: urutan record 5.4 dan 6.6 cocok dengan ekspektasi; invarian `Apply` tidak pernah terpicu.

### 13.5 Application dan persistensi scope: migrasi, projector, `LoadRuntimeState`, snapshot v2

- `scope.sql`, `timer.sql`, `message_subscription.sql`; sqlc; projector + `LoadRuntimeState` (9.2);
  `Snapshot` v2 (9.3); kueri verifikasi baru; `GetProcessInstance` dengan timer/subscription/anak.
- Integration test: proyeksi setiap skenario 13.3–13.4 = state; truncate → rebuild identik;
  snapshot v1 → mulai dari nol.

Hasil: `make test-integration` hijau; `engine verify --all` bersih; ADR-0023 draf.

### 13.6 Modul `timer`: clock, sweeper, trigger, timer start

- `platform/clock` (`Real`, `Fake`); modul `timer` lima lapisan (3.4); `ClaimDueTimers`,
  `TriggerTimer` lewat `execution/api.Timers.Trigger`; `CreateFromStart` untuk timer start;
  consumer `timer.start_scheduler`; participant; metrik dan log (10); `time/tzdata`.
- Integration test: dua sweeper paralel pada 10.000 timer → tidak ada record ganda; klaim
  kedaluwarsa diklaim ulang; timer start versi lama dibatalkan saat deploy; cycle start x09;
  `timeDate` lampau.

Hasil: DoD butir timer (intermediate, boundary, start, sweeper, pemulihan dasar) terbukti; ADR-0024 draf.

### 13.7 Chaos timer dan benchmark timer

- Skenario `timer-missed-during-restart`; `BenchmarkSweep10kTimers`; angka dicatat.

Hasil: `make chaos SCENARIO=timer-missed-during-restart` hijau tiga kali; angka benchmark di 0.

### 13.8 Modul `message`: subscription, lane lock, dua titik korelasi, start

- `message_start_subscription`, `message_correlation`, lane lock, `Buffer.Claim`,
  `MatchAndMarkCorrelating` (di `execution/infrastructure`), korelasi sisi publish, consumer
  `execution.message_correlation`, `message.start_subscription`, `message.instance_final`,
  participant; `db.WithTx/TxFromContext` (3.3); `command_runner` loop sisi subscribe.
- Integration test: kasus A dan B 7.4; race publish vs subscribe dengan fault point (100 ulangan,
  tidak ada message tertinggal di buffer saat ada subscription cocok); x07 urutan FIFO; x08
  keunikan start; non-interrupting boundary message berulang; TTL 0; dedup `message_id` (Fase 4)
  tetap.

Hasil: DoD butir message terbukti; ADR-0025 draf.

### 13.9 Error dan escalation

- `behavior_error.go` (8.2, 8.3) disisipkan ke `ThrowError` Fase 4 sebelum incident; error end
  event; escalation throw/end; incident 8.4; `ResolveIncident` langkah ulang.
- Skenario: v11, v12, v14 (`onError`), x04 (`error-nearest-wins`, `error-exact-before-catch-all`),
  x11.

Hasil: DoD butir error dan escalation; perilaku Fase 4 (`UNHANDLED_ERROR_EVENT` tanpa catch)
tetap lolos test Fase 4 tanpa perubahan ekspektasi.

### 13.10 Call activity dan internal command lintas instance

- `behavior_call_activity.go`, `CreateFromStart{Parent}`, `CompleteCallActivity`,
  `TerminateChild`, `ChildTerminated`, `ThrowErrorFromChild`, consumer `execution.cross_instance`,
  `cross_instance_applied`, `ENGINE_MAX_CALL_DEPTH`, propagasi variabel; `ListProcessInstances`
  filter induk.
- Skenario: v15 + anak, x05 (tiga tingkat, cancel, error dari cucu), harness multi-instance
  domain; integration dengan relay `Drain`; chaos `call-activity-parent-kill`.

Hasil: DoD butir call activity; ADR-0026 draf.

### 13.11 Multi-instance

- `behavior_multi_instance.go` (5.6), scope lokal + lookup berantai (`variables.go`), aturan
  deploy subset, batas item, output collection; `GetProcessInstance` variabel lokal.
- Skenario: v16 (parallel + output, sequential + completion), x06 (sub-process body + boundary),
  koleksi kosong, bukan list, 10.001 item.

Hasil: DoD butir multi-instance; ADR-0027 draf.

### 13.12 Property test interupsi

- Generator dan simulator 12.3 di atas harness multi-instance; 7 properti; `make prop` target
  `TestInterruptionProperties`.

Hasil: 1.000 iterasi < 60 s hijau; seed gagal (bila ada) menjadi skenario regresi bernama.

### 13.13 Proto, SDK, client TS, UI

- Proto 11.1–11.2, `buf breaking`, generate; SDK Go `Client` meneruskan RPC baru
  (`ListMessageSubscriptions`, `ListBufferedMessages`); `libs/web/api-client` di-generate ulang.
- Modeler-web 11.3; uji manual (timer boundary, publish message dari dialog, call activity) dengan
  screenshot di PR.

Hasil: `make gen` bersih; `nx affected -t typecheck test build` hijau; uji manual tercatat.

### 13.14 Paritas

- Suite `events` dan `scopes`; mode live terhadap `camunda/zeebe:8.8.44`; perbarui
  `expectations.json`; tutup semua butir "verifikasi" (catat hasil di ADR dan 16.1).

Hasil: `make parity SUITE=events,scopes` lolos offline; laporan deviasi di PR penutup fase.

### 13.15 Dokumentasi dan penutupan

- ADR-0023 (pohon scope dan terminate subtree), ADR-0024 (timer: kepemilikan, klaim dua langkah,
  presisi), ADR-0025 (korelasi: lane lock, dua titik, panggilan antarmodul dalam transaksi),
  ADR-0026 (internal command lintas instance lewat outbox), ADR-0027 (multi-instance dengan subset
  FEEL dan scope lokal), ADR-0028 (propagasi error/escalation; event-based gateway dan terminate di
  Fase 5; signal ditunda).
- README modul `execution` (daftar record/incident baru, pohon scope), `timer`, `message`;
  `docs/plan/README.md` menautkan dokumen ini; review DoD.

Hasil: semua kotak DoD tercentang dengan bukti.

## 14. Target Nx dan Makefile

| Proyek | Target | Perintah / perubahan |
|---|---|---|
| `bpmn-go` | `test`, `fuzz` | + `FuzzParseTimer` (`make fuzz TARGET=bpmn` menjalankan semua fuzz target paket) |
| `engine` | `test` | mencakup `execution` Fase 5, `timer`, `message`, `platform/clock`; input menyertakan `execution/testdata/bpmn/**` |
| `engine` | `prop` (cache off) | `go test -run 'TestGatewayProperties|TestInterruptionProperties' -count=1 ./apps/engine/internal/modules/execution/domain -prop.iter=$PROP_ITER` |
| `engine` | `test-integration` | + sweeper dua replika, korelasi race, cross-instance, rebuild v2 |
| `engine` | `bench` | + `BenchmarkSweep10kTimers`, `BenchmarkCorrelateE2E`, `BenchmarkProcessorNestedScopes` |
| `engine` | `sqlc` | + `execution/infrastructure/queries/{timer,subscription,scope}.sql`, `timer/infrastructure/queries`, `message/infrastructure/queries` |
| `proto` | `generate`, `lint`, `breaking` | mencakup perubahan 11.1–11.2 |
| `chaos` | `run` | skenario baru 9.4 |
| `modeler-web` | `test`, `build` | fitur `process-instance` 11.3 |

Target Makefile: `make prop [PROP_ITER=100000] [PROP_SEED=…]` (sudah ada, kini dua suite),
`make parity SUITE=events,scopes [LIVE=1]`, `make chaos SCENARIO=timer-missed-during-restart|message-publish-during-restart|call-activity-parent-kill`,
`make fuzz TARGET=bpmn`. Tidak ada target baru selain nilai parameter.

## 15. Risiko dan mitigasi

| Risiko | Dampak | Mitigasi |
|---|---|---|
| Semantik Zeebe yang diasumsikan (urutan catch, keunikan message start, propagasi error lintas call activity, default propagasi variabel, cycle re-schedule) ternyata berbeda | Paritas gagal; perilaku proses nyata salah | Verifikasi dokumentasi **sebelum** kode (13.1); setiap asumsi punya skenario paritas; implementasi dirancang agar cabang mudah diganti (catch index, flag `ENGINE_MESSAGE_START_UNIQUE`) |
| Deadlock antara sweeper/publish dan command instance | Engine macet, `lock_timeout` Fase 3 memicu error beruntun | Urutan lock didefinisikan (6.2, 7.3); sweeper dan publish tidak pernah memegang row lock milik `execution` sambil menunggu lock instance; test race 100 ulangan dengan fault point; `lock_timeout` 5 s sebagai jaring |
| Timer ganda / terlewat saat dua replika | Instance dibuat dua kali atau tidak sama sekali | Klaim `SKIP LOCKED` + command idempoten di bawah lock instance; timer start memakai `CreateFromStart` dalam transaksi yang menandai timer; chaos `timer-missed-during-restart` |
| Latensi korelasi lewat outbox terasa lambat untuk pola request-response | UX/paritas timing | LISTEN relay < 10 ms; sisi subscribe sinkron (kasus message-dulu tidak lewat outbox); metrik `engine_message_correlated_total{path}`; opsi korelasi sinkron dengan `pg_try_advisory_lock` instance dicatat sebagai perbaikan bila terukur |
| Internal command lintas instance tertunda/hilang | Call activity menggantung | Consumer durable Fase 3 (at-least-once, dead letter + `requeue`); `VerifyCallActivityLinks` menandai yang menggantung; `cross_instance_applied` mencegah ganda |
| `terminateSubtree` menghasilkan ribuan record untuk multi-instance besar | Batas record per command (`ENGINE_MAX_RECORDS_PER_COMMAND`) terlampaui → incident di tengah pembatalan | Batas item multi-instance (10.000) dan batas record dinaikkan khusus untuk terminate (`ENGINE_MAX_RECORDS_PER_TERMINATE`, default 100.000); record terminate ringkas |
| Scope lokal multi-instance menyimpang dari semantik scope Fase 7 | Fase 7 mengubah perilaku | Scope lokal hanya untuk variabel yang dibuat engine (`inputElement`, `loopCounter`); penulisan worker tetap ke root; semantik Zeebe (tulis ke scope terdekat yang punya variabel) diverifikasi dan dicatat di ADR-0027 |
| Presisi timer tidak cukup untuk kasus tertentu | Kebutuhan sub-detik | Didokumentasikan sebagai batas; interval bisa diturunkan ke 100 ms; LISTEN/NOTIFY pada `TIMER CREATED` dengan `due_at` dekat dicatat sebagai perbaikan opsional |
| Zone id butuh tzdata di image distroless | `time.LoadLocation` gagal di produksi | `import _ "time/tzdata"`; test integrasi `TIMER_ZONE_UNKNOWN`; CI menjalankan test timer di image |
| `message_subscription` milik `execution` tetapi di-update di transaksi publish | Pelanggaran batas modul | SQL dan kode ada di `execution/infrastructure`, dipanggil lewat `execution/api` dalam transaksi yang dibawa `ctx` (3.3); `archtest` tetap menegakkan import |
| Fase 4 belum merge saat Fase 5 mulai | 13.8, 13.9 tertahan | 13.2–13.7 tidak bergantung pada Fase 4; `message` dibuat sesuai sketsa Fase 4 bila perlu |
| Snapshot v2 memperbesar ukuran snapshot untuk instance dengan ribuan timer | Snapshot lambat | Snapshot hanya tiap N record (Fase 3); timer aktif per instance biasanya kecil; benchmark instance 10.000 inner multi-instance |

## 16. Pertanyaan terbuka

Setiap pertanyaan diberi rekomendasi. Keputusan final dicatat di 16.1 setelah review.

### 16.1 Keputusan

Belum ada. Diisi dengan tabel yang sama seperti 9.1 dokumen Fase 1.

### 16.2 Pertanyaan dan rekomendasi

| # | Pertanyaan | Rekomendasi | Alasan |
|---|---|---|---|
| 1 | Multi-instance di Fase 5 (subset) atau Fase 7? | **Fase 5 dengan subset** (3.2) | Pola sangat umum; semantik iterasi independen dari FEEL; v16 hanya memakai path |
| 2 | State timer/subscription di `execution` atau modul sendiri dengan tabel sendiri? | **`execution`**; modul `timer` hanya scheduler, `message` hanya buffer + start + korelasi | Atomisitas dengan terminate subtree, sama dengan keputusan job Fase 2 |
| 3 | Korelasi sisi publish sinkron (lock instance) atau lewat outbox? | **Outbox** (7.3) | Bebas deadlock tanpa urutan lock global; publish tidak menunggu command panjang |
| 4 | Keunikan message start per `(name, key, process)` ditegakkan? | **Ya**, di balik `ENGINE_MESSAGE_START_UNIQUE=true`; dimatikan bila verifikasi Zeebe menyatakan tidak ada aturan itu | Mencegah instance ganda dari publish ulang; flag memudahkan paritas |
| 5 | Cron di `timeCycle` | **Tolak saat deploy** sampai Fase 7 | Parser cron + semantik Quartz Zeebe belum diverifikasi |
| 6 | Signal event | **Belum dijadwalkan**; kandidat Fase 7 | Fitur broadcast sendiri; tidak ada kebutuhan |
| 7 | Pengulangan cycle: `due_prev + D` atau `now + D`? | **`due_prev + D`** (tanpa drift), lompati jatuh tempo yang terlewat saat engine mati (trigger sekali, lanjut ke jadwal berikutnya ≥ now) | Perilaku jam dinding yang diharapkan; verifikasi Zeebe |
| 8 | Error tidak tertangkap di proses anak: propagasi ke induk atau incident di anak? | **Propagasi** ke boundary call activity induk, anak `TERMINATED` | Pemahaman penulis atas Camunda 8.x; bila verifikasi berbeda, ganti ke incident di anak (satu cabang kode) |
| 9 | RPC untuk memicu timer manual / membatalkan subscription (operator)? | **Tidak di Fase 5**; Operate Fase 6 memutuskan (bersama "modifikasi instance") | Mengubah state instance dari luar butuh desain modifikasi instance yang utuh |
| 10 | Scope variabel: tulis ke root atau scope terdekat yang memiliki variabel? | **Scope terdekat yang sudah memiliki variabel, selain itu root** (semantik Zeebe sejauh diketahui) | Membuat `outputElement` multi-instance dan Fase 7 konsisten; verifikasi |
| 11 | Timer boundary pada multi-instance: body atau tiap inner? | **Body** (satu timer) | Semantik BPMN: boundary menempel pada activity, bukan iterasi; verifikasi Zeebe |
| 12 | `timeDate` lampau saat aktivasi/deploy | **Terpicu sekali segera** | Konsisten dengan "due_at <= now"; verifikasi |
| 13 | Lintas instance: outbox (at-least-once, latensi relay) atau lock bersarang dalam satu transaksi? | **Outbox** (ADR-0026) | Tanpa urutan lock global; relay sudah ada |
| 14 | Instance anak dibatalkan langsung lewat API: call activity `TERMINATED` lalu lanjut, atau incident di induk? | **`TERMINATED` + propagasi normal** kecuali verifikasi menunjukkan Zeebe membuat incident | Sederhana; mudah diganti |
| 15 | Default `propagateAllParentVariables` / `propagateAllChildVariables` | **Keduanya `true`** sampai verifikasi | Pemahaman penulis atas Modeler Camunda 8 |
| 16 | Event sub-process non-interrupting dengan timer `timeCycle` | **Diizinkan** (sesuai validator Fase 1) | Pola "pengingat berkala selama proses berjalan" |

## 17. Estimasi dan pembagian kerja

Perkiraan **24–28 hari kerja** untuk 1 engineer, atau 14–16 hari untuk 2 engineer paralel:

| Hari | Fokus |
|---|---|
| 1 | 13.1 prasyarat, verifikasi dokumentasi Camunda 8.8 (tabel di 0) |
| 2–3 | 13.2 kernel ISO 8601, profil, aturan baru |
| 3–6 | 13.3 pohon scope, terminate subtree, sub-process, terminate end, property gateway + sub-process |
| 7–9 | 13.4 catch index, boundary, `fire`, event sub-process, event-based gateway, `TriggerTimer`/`CorrelateMessage` domain |
| 10–11 | 13.5 migrasi, projector, `LoadRuntimeState`, snapshot v2, verifikasi baru |
| 12–13 | 13.6 modul `timer` + 13.7 chaos/benchmark timer |
| 14–16 | 13.8 modul `message`: lane lock, dua titik korelasi, start, consumer |
| 17 | 13.9 error dan escalation |
| 18–19 | 13.10 call activity + cross-instance |
| 20–21 | 13.11 multi-instance + scope lokal |
| 22 | 13.12 property test interupsi |
| 23–24 | 13.13 proto, SDK, TS, UI |
| 25–27 | 13.14 paritas live + perbaikan deviasi |
| 28 | 13.15 ADR, README, review DoD |

Pembagian jika 2 orang: **A** = domain (13.3, 13.4, 13.9–13.12) lalu paritas; **B** = kernel
(13.2), persistensi (13.5), modul `timer` (13.6–13.7), modul `message` (13.8), proto/UI (13.13).
Titik temu: 13.5 (B memproyeksikan record yang didefinisikan A di 13.3–13.4) dan 13.8 (loop sisi
subscribe di `command_runner` memakai command domain A).

## 18. Keluaran Fase 5

Setelah fase ini selesai, Fase 6 dan 7 mendapat:

- **Fase 6 (Operate, Tasklist)**: pohon scope lengkap di proyeksi (`flow_scope_key`,
  `called_process_instance_key`, `parent_*`) untuk pohon instance React Flow; `timers`,
  `message_subscriptions`, `child_instances` di `GetProcessInstance`; `ListMessageSubscriptions`,
  `ListBufferedMessages`; record `ERROR`/`ESCALATION THROWN/CAUGHT` dan `TIMER`/`MESSAGE_SUBSCRIPTION`
  di `ListProcessInstanceRecords` untuk timeline; fitur `process-instance` modeler-web dengan panel
  "Menunggu" yang dipindah utuh ke `operate-web`; pertanyaan 16.2 #9 (aksi operator pada timer /
  subscription) untuk desain modifikasi instance.
- **Fase 7 (FEEL, DMN)**: titik evaluasi yang sudah terisolasi untuk diganti FEEL penuh — timer
  definition (`timer/domain.NextDue` menerima `bpmn.Timer`, tinggal menambah cabang `IsFEEL`),
  `correlationKey`, `processId`, `inputCollection`/`outputElement`/`completionCondition`;
  aturan deploy `*_NOT_SUPPORTED` yang dihapus bertahap; **lookup variabel berantai per scope** dan
  `VARIABLE` dengan `scope_key` lokal sebagai fondasi I/O mapping dan scope lokal umum; cron
  `timeCycle` dan signal event sebagai kandidat.
- **Fase 8**: `tenant_id` sudah ada di `timer`, `message_subscription`,
  `message_start_subscription`, `message_correlation`; lane lock dan indeks sudah memuat tenant.
- **Semua fase**: ADR-0023 sampai ADR-0028; suite paritas `events` dan `scopes`; property test
  interupsi sebagai pengaman regresi untuk setiap perubahan `Processor` berikutnya; tiga skenario
  chaos baru.
