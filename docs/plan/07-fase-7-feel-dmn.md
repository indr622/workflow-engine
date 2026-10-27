# Fase 7 — FEEL dan DMN

**Proyek:** Workflow Engine (kompatibel konsep Camunda 8 / BPMN 2.0)
**Stack:** Go (engine utama) + React (UI) dalam monorepo Nx
**Status:** Draft rencana — belum dieksekusi
**Tanggal:** 2026-10-28

---

## 0. Status eksekusi

Belum ada langkah yang dieksekusi. Bagian ini diisi dengan tabel yang sama seperti Fase 0
(langkah, status, bukti) begitu pekerjaan dimulai.

**Kondisi repo yang menjadi titik awal (dicek 2026-10-28, Fase 1 sedang dieksekusi; Fase 2–6 masih
draft):**

| Area | Kondisi / asumsi dari fase sebelumnya | Dampak ke Fase 7 |
|---|---|---|
| `libs/go/feel` | Folder penanda (README "Dibuat di Fase 7"). Fase 2 (3.4, ADR-0013) melahirkan **subset** kondisi: `Compile`/`Evaluate`, `Context.Lookup`, `Value` opaque, `FromJSON`, angka `big.Rat`, `UnsupportedError` untuk konstruksi di luar subset | Fase 7 mengganti isi paket dengan interpreter FEEL penuh **di balik API yang sama**; `UnsupportedError` hilang; representasi angka diganti desimal (3.1) tanpa mengubah hasil subset |
| `libs/go/bpmn` (Fase 1, sedang dibangun) | Semua ekspresi disimpan `Expression{Raw, IsFEEL}` (`IsFEEL` jika diawali `=`); `Mapping{Source Expression; Target string}`, `IOMapping{Inputs, Outputs}`; `Script{Expression, ResultVariable}`; `CalledDecision{DecisionID, ResultVariable string}`; `Timer{Kind, Value Expression}`; `MultiInstance{InputCollection Expression, InputElement string, OutputCollection string, OutputElement Expression, CompletionCondition Expression}`; `UserTaskDefinition{Assignee, CandidateGroups, CandidateUsers, DueDate, FollowUpDate Expression}`; `CalledElement{ProcessID Expression}`; `Code` adalah `string` | Fase 7 **tidak mengubah API `bpmn`**: validasi sintaks FEEL hidup di paket baru `libs/go/feel/bpmncheck` yang berjalan di atas `*bpmn.Definitions` dan menghasilkan `bpmn.Issues` dengan kode baru (3.4). Deploy `.dmn` ditolak `UNSUPPORTED_RESOURCE_TYPE` sejak Fase 1; dibuka di 7b |
| Modul `deployment` (Fase 1) | `DeployResource` → `bpmn.ParseAndValidate(ProfileExecutable)`; Fase 2 menambah langkah `feel.Compile` yang menolak `EXPRESSION_NOT_SUPPORTED` / `EXPRESSION_SYNTAX_ERROR`; cache LRU `*bpmn.Process` di `process_cache.go` | Langkah kompilasi diganti `bpmncheck`; `EXPRESSION_NOT_SUPPORTED` dan `IO_MAPPING_NOT_SUPPORTED` dihapus; kode sintaks diseragamkan menjadi `FEEL_SYNTAX_INVALID` (6.7) |
| Modul `execution` (Fase 2–5) | Port `domain.ExpressionEvaluator` (`EvaluateBool/String/Int`) dengan adapter `infrastructure/feel_evaluator.go`; variabel per `(scope_key, name)` dengan skema yang sudah siap scope lokal tapi hanya scope root yang dipakai; incident `CONDITION_ERROR`, `EXTRACT_VALUE_ERROR`; Fase 4 `SetVariables local=true` → `Unimplemented`; Fase 5 menambah sub-process, multi-instance, timer, message (ekspresi dievaluasi lewat port yang sama, dibatasi subset) | Fase 7 memperluas port (`EvaluateValue`, `EvaluateUnaryTests`), **menghidupkan scope lokal** (6.1), I/O mapping (6.2), script task, dan membuka `SetVariables local=true` |
| Modul `usertask` (Fase 6) | Native user task memakai `assignmentDefinition`/`taskSchedule`; diasumsikan memakai subset Fase 2 bila Fase 7 belum ada | Fase 7 menyediakan FEEL penuh (list literal untuk `candidateGroups`, fungsi tanggal untuk `dueDate`); lihat pertanyaan terbuka 13.2 #1 tentang urutan |
| `libs/proto` | `health/v1`, `deployment/v1`, `process/v1`, `job/v1`, `common/v1`, `incident/v1`, `message/v1` (Fase 1–4), `usertask/v1` (Fase 6) | Paket baru `workflow/expression/v1` (kecil, 6.8) dan, di 7b, `workflow/decision/v1`; `deployment/v1` ditambah field aditif untuk decision |
| Compose | Profil `camunda` dengan `camunda/zeebe:8.8.44` (Fase 1) | Paritas ekspresi memakai `CreateProcessInstanceWithResult` (9.1) sehingga tidak butuh secondary storage |
| `apps/modeler-web` | Fitur `deployment` (Fase 1) menampilkan issue per elemen; properties panel bpmn.io (Fase 6) | Issue `FEEL_SYNTAX_INVALID` tampil di elemen dengan posisi kolom dalam ekspresi; editor FEEL (8.1) |
| `apps/operate-web` (Fase 6) | Viewer instance, variabel, incident | 7b menambah fitur `decision-instance` |
| Image engine | distroless, `CGO_ENABLED=0` | **Tidak ada zoneinfo** di image: `cmd/engine` wajib `import _ "time/tzdata"` agar zona waktu FEEL (`@"...@Asia/Jakarta"`) bisa dimuat (3.2) |
| ADR | ADR-0001–0007 (Fase 0); Fase 1–6 merencanakan ADR-0008 sampai ±0026 (catatan: draft Fase 3 memakai 0014–0018 dan draft Fase 4 memakai 0017–0020, tumpang tindih yang harus dirapikan saat fase itu dieksekusi) | Fase 7 memakai **ADR-0027** (interpreter FEEL dan representasi angka), **ADR-0028** (scope variabel lokal dan semantik I/O mapping), **ADR-0029** (lingkup dan penempatan DMN). Jika saat eksekusi nomor di bawah 0027 ternyata bebas, nomor Fase 7 digeser turun dan dicatat di 13.1 |

## 1. Lingkup

Fase 7 membuat engine **memahami FEEL seutuhnya**: setiap atribut yang di Camunda Modeler diisi
dengan ekspresi `=...` (kondisi, input/output mapping, script, timer, koleksi multi-instance,
correlation key, `processId` call activity, assignment dan jadwal user task) dievaluasi dengan
interpreter FEEL 1.x (DMN 1.4/1.5) murni Go dan hasilnya sama dengan Camunda 8.8 untuk kasus yang
bisa diamati. Dengan itu, I/O mapping dan scope variabel lokal — yang sejak Fase 2 ditolak saat
deploy — menjadi hidup, dan model dari Modeler yang memakai mapping berhenti ditolak.

DMN (opsional di roadmap) direncanakan sebagai **Fase 7b**: dikerjakan setelah DoD 7a tercapai,
dengan dokumen rencana yang sama (bagian 7) dan estimasi terpisah (bagian 14). Alasan pemisahan ada
di 3.5.

Termasuk dalam **Fase 7 (7a)**:

1. **`libs/go/feel` penuh** — lexer, parser dengan presedensi sesuai spesifikasi, AST, evaluator
   dengan semantik yang mengikuti perilaku feel-scala (engine FEEL Zeebe) bila berbeda dari
   spesifikasi; tipe `null`, boolean, number (desimal), string, date, time, date and time,
   days-and-time duration, years-and-months duration, list, context, range, function; unary tests;
   katalog fungsi bawaan dengan matriks cakupan "wajib Fase 7" / "nanti".
2. **Kompilasi dan cache** — parse sekali per teks ekspresi, cache di adapter evaluator, pemanasan
   saat deploy; konversi JSON ↔ nilai FEEL tanpa kehilangan presisi; model error yang menghasilkan
   incident dengan pesan mirip Zeebe.
3. **Integrasi engine** — kondisi gateway memakai FEEL penuh; **scope variabel lokal** per element
   instance; **input/output mapping** dengan aturan propagasi Zeebe; **script task** `zeebe:script`;
   ekspresi timer, koleksi multi-instance, correlation key, `processId`, assignment/jadwal user task
   lewat port yang diperluas (Fase 5/6 adalah pemakai, Fase 7 pemasok); `SetVariables local=true`.
4. **Validasi deploy** — `FEEL_SYNTAX_INVALID` (error) untuk setiap ekspresi `=`; penghapusan
   penolakan subset Fase 2; pemeriksaan statis tambahan (target mapping, tipe literal).
5. **`ExpressionService.EvaluateExpression`** (kecil) untuk modeler-web, harness paritas, dan
   debugging.
6. **Modeler-web** — error sintaks FEEL per elemen dari `ValidateResource` dengan posisi dalam
   ekspresi; editor FEEL bpmn.io bila lisensinya terverifikasi; panel "coba ekspresi".
7. **Strategi uji** — tabel golden ≥ 600 kasus, fuzz parser dan evaluator, benchmark, korpus
   konformansi (porting kasus DMN TCK level 3 yang berbentuk ekspresi literal, lisensi diverifikasi),
   paritas ekspresi terhadap Camunda 8.8 lewat script task.

Termasuk dalam **Fase 7b (DMN, opsional)**: `libs/go/dmn` (parse DMN 1.3–1.5 XML: decision table
dengan semua hit policy dan agregasi, literal expression, decision requirements, BKM sederhana),
deploy `.dmn`, business rule task `zeebe:calledDecision`, `DecisionService.EvaluateDecision`,
decision instance untuk Operate, DMN modeler (dmn-js) di modeler-web.

**Tidak termasuk** (fase lain atau tidak direncanakan): fungsi FEEL eksternal (`external: true`,
Java) — tidak ada padanannya di Zeebe; DMN boxed expression selain literal expression, context, dan
decision table (invocation, relation, function definition, conditional/iterator DMN 1.4) — ditinjau
setelah 7b; decision service DMN; multi-tenant pada decision (Fase 8); FEEL di dalam form (Fase 6
memakai form-js); migrasi semantik untuk instance berjalan (tidak perlu: subset Fase 2 adalah subset
yang semantiknya identik).

## 2. Definition of Done

Fase 7 (7a):

- [ ] `libs/go/feel` tetap proyek Nx `feel-go` (`type:kernel`), hanya stdlib (`deps_test.go` + `TestKernelsArePure` di `archtest`); API Fase 2 (`Compile`, `Evaluate`, `Context`, `Value.Bool/String`, `FromJSON`) tidak berubah tanda tangannya; seluruh tabel uji subset Fase 2 (≥ 80 kasus) tetap lolos tanpa perubahan ekspektasi.
- [ ] Grammar FEEL di 4.1 terimplementasi: literal semua tipe (termasuk `@"..."` temporal dan string escape), aritmetika dengan presedensi spesifikasi, perbandingan, `between`, `in`, `and`/`or` tiga nilai, `if`, `for` (termasuk iterasi rentang dan multi-iterator), `some`/`every`, filter `[...]` (indeks positif/negatif dan predikat dengan `item`), path, context literal dengan referensi antar-entri, list literal, range literal, `instance of`, definisi dan pemanggilan fungsi (positional dan named parameter), komentar `//` dan `/* */`, nama dengan backtick.
- [ ] Katalog fungsi bawaan bertanda **wajib** di 4.4 (≈ 95 fungsi) terimplementasi dengan minimal 3 kasus uji per fungsi (nilai normal, `null`, tipe salah).
- [ ] Angka: desimal eksak untuk `+ - *`, pembagian dan fungsi transendental dengan presisi 34 digit signifikan pembulatan half-even (DECIMAL128); `9007199254740993 + 1` dan `0.1 + 0.2` eksak; konversi JSON ↔ FEEL bolak-balik tanpa kehilangan presisi.
- [ ] Temporal: date/time/date and time (lokal, offset, zona IANA), dua jenis durasi, aritmetika dan perbandingan sesuai 4.3; `now()`/`today()` memakai `Clock` dan zona yang di-inject; engine memuat `time/tzdata`.
- [ ] Semantik `null` dan tipe campuran sesuai tabel 3.2; setiap baris yang ditandai `verify` punya kasus paritas (bagian 9) dan hasil akhirnya dicatat di ADR-0027.
- [ ] Tabel golden `testdata/golden/*.json` ≥ 600 kasus; korpus konformansi 5.6 dijalankan (`nx run feel-go:conformance`) dengan daftar pengecualian beralasan; `FuzzParse` dan `FuzzEvaluate` 10 menit bersih; evaluasi selalu selesai di bawah batas langkah dan memori (`ErrLimitExceeded`, bukan hang).
- [ ] Benchmark 5.5: `x > 10` dievaluasi < 300 ns tanpa alokasi di hot path; `Compile` ekspresi 1 KB < 50 µs; cache ekspresi membuat evaluasi berulang tidak mem-parse ulang (dibuktikan benchmark dan counter).
- [ ] Engine: input mapping membuat variabel **lokal** di scope element instance; output mapping mempropagasi ke scope induk sesuai aturan 6.2; job variables pada task dengan output mapping tidak bocor ke scope induk; tanpa output mapping job variables dipropagasi seperti Fase 2. Dibuktikan skenario tabel per aturan dan paritas.
- [ ] Script task dengan `zeebe:script` mengevaluasi ekspresi dan menulis `resultVariable`; script task dengan `zeebe:taskDefinition` menjadi job (sudah ada); gagal evaluasi → incident sesuai 6.6.
- [ ] Ekspresi kondisi, timer, `inputCollection`/`outputElement`/`completionCondition`, correlation key, `processId`, assignment/jadwal user task, `type`/`retries` task definition dievaluasi FEEL penuh dengan pemeriksaan tipe hasil dan incident sesuai 6.6 (perilaku elemennya tetap milik Fase 2/4/5/6; Fase 7 hanya mengganti evaluator dan menambah skenario regresi).
- [ ] `SetVariables(local = true)` pada scope element instance aktif menulis variabel lokal (`Unimplemented` Fase 4 dihapus).
- [ ] Deploy: semua ekspresi `=` di proses executable diperiksa; sintaks rusak → `FEEL_SYNTAX_INVALID` (error) dengan element id, atribut, dan posisi kolom dalam ekspresi; `EXPRESSION_NOT_SUPPORTED`, `IO_MAPPING_NOT_SUPPORTED`, dan warning `FEEL_NOT_VALIDATED` tidak ada lagi; definisi yang lolos Fase 2 tetap lolos.
- [ ] `ExpressionService.EvaluateExpression` berjalan (Connect, Go + TS) dengan batas ukuran dan waktu.
- [ ] Modeler-web: issue FEEL tampil di elemen dan di field properties panel; panel "coba ekspresi" memanggil `EvaluateExpression`; uji manual tercatat di PR.
- [ ] Paritas: seluruh kasus di `libs/go/feel/testdata/parity/*.json` (≥ 150, termasuk daftar 9.2) memberi hasil yang sama dengan Camunda 8.8 (nilai variabel kanonik atau incident); deviasi disengaja tercatat di `expectations.json` dengan alasan dan ditulis ke ADR-0027.
- [ ] `TestKernelsArePure` mencakup `libs/go/feel`; `archtest` lolos; ADR-0027 dan ADR-0028 tersimpan; `docs/plan/README.md` menautkan dokumen ini.

Fase 7b (DMN), bila dikerjakan:

- [ ] `libs/go/dmn` proyek Nx `dmn-go` (kernel, hanya stdlib + `libs/go/feel`), parse DMN 1.3/1.4/1.5 dari Camunda Modeler, decision table dengan hit policy `UNIQUE`, `FIRST`, `PRIORITY`, `ANY`, `COLLECT` (+ `SUM`/`MIN`/`MAX`/`COUNT`), `RULE ORDER`, `OUTPUT ORDER`; literal expression; `informationRequirement`; BKM dengan `knowledgeRequirement`; validasi dengan kode issue stabil.
- [ ] Korpus DMN TCK yang relevan (compliance level 2 dan 3 untuk decision table dan ekspresi literal) lolos dengan daftar pengecualian beralasan.
- [ ] `DeployResource` menerima `.dmn`; versi per `decisionRequirementsId` dan per `decisionId`; dedup checksum seperti BPMN; cache DRG ter-parse lewat `deployment/api.Decisions`.
- [ ] Business rule task `zeebe:calledDecision` mengevaluasi decision (binding `latest`, `deployment`, `versionTag` — verifikasi atribut 8.8), menulis `resultVariable`, menghasilkan record `DECISION_EVALUATION`; decision tidak ditemukan → incident `CALLED_DECISION_ERROR`; gagal evaluasi → `DECISION_EVALUATION_ERROR`.
- [ ] `DecisionService.EvaluateDecision` (standalone) mengembalikan output, decision yang dievaluasi, rule yang cocok, dan `decision_instance_key`; `decision_instance` tersimpan untuk kedua jalur; `GetDecisionInstance`/`ListDecisionInstances` untuk Operate.
- [ ] Modeler-web: fitur `decision-modeler` (dmn-js) dengan deploy; operate-web: fitur `decision-instance` (tabel dengan rule yang cocok ditandai).
- [ ] Paritas: ≥ 20 decision table (semua hit policy, agregasi, input/output bertipe, unary test campuran) memberi output yang sama dengan Camunda 8.8 lewat `EvaluateDecision`.
- [ ] ADR-0029 tersimpan.

## 3. Keputusan teknis

| Area | Keputusan | Alasan singkat |
|---|---|---|
| Implementasi FEEL | **Interpreter tree-walking di Go murni, stdlib saja**, di `libs/go/feel` (ADR-0027). Tidak memakai feel-scala lewat JVM, tidak memakai library FEEL Go pihak ketiga | Kernel harus bisa diimpor `domain` (allowlist ADR-0010) dan masuk image distroless tanpa JVM; library Go yang ada tidak lengkap/tidak mengikuti perilaku Zeebe dan menarik dependensi. Tree-walking cukup: ekspresi proses pendek, hot path adalah lookup variabel |
| Acuan semantik | **Perilaku feel-scala yang teramati** (engine FEEL Zeebe 8.8) bila berbeda dari spesifikasi DMN; spesifikasi untuk yang tidak teramati; tabel perbedaan di 3.2 dengan bukti paritas | Tujuan proyek adalah paritas Camunda 8; pengguna menulis ekspresi mengikuti dokumentasi Camunda, bukan spesifikasi OMG |
| Angka | **Desimal kustom di `libs/go/feel/internal/decimal`**: koefisien `big.Int` + eksponen `int32`, jalur cepat `int64` untuk koefisien kecil; `+ - *` eksak; `/`, `**` non-bulat, `sqrt`, `log`, `exp` dibulatkan ke **34 digit signifikan, half-even** (padanan `MathContext.DECIMAL128` yang dipakai feel-scala, verifikasi) (ADR-0027, menggantikan `big.Rat` Fase 2) | `float64` merusak `0.1 + 0.2` dan integer besar; `big.Rat` tidak bisa merepresentasikan hasil `1/3` sebagai desimal dan tidak punya fungsi transendental; `big.Float` biner sehingga `0.1` tidak eksak. Hasil subset Fase 2 tidak berubah karena `+ - *` dan perbandingan tetap eksak. `shopspring/decimal` tidak dipakai karena kernel harus stdlib |
| Temporal | Lima tipe FEEL di atas `time.Time` + `time.Location` (date = `time.Time` tengah malam UTC dengan flag; time = detik + nano + offset/zona opsional) dan dua tipe durasi terpisah (`DaysTimeDuration{nanos int64}`, `YearsMonthsDuration{months int32}`) | Spesifikasi membedakan kedua durasi dan melarang pencampurannya; `time.Duration` saja tidak cukup untuk `P1M` |
| Zona waktu | Zona IANA dimuat lewat `time.LoadLocation`; **`cmd/engine` meng-embed `time/tzdata`**; `now()`/`today()` memakai port `Clock` dan zona dari konfigurasi `ENGINE_FEEL_TIMEZONE` (default `UTC`; verifikasi zona yang dipakai Zeebe: zona JVM broker) | Image distroless tidak punya `/usr/share/zoneinfo`; determinisme `Processor` butuh jam yang di-inject |
| Nilai | `feel.Value` tetap **opaque** (struct kecil: `kind` + payload); list `[]Value`, context berupa **urutan entri** (`[]Entry{Key, Value}` + indeks map saat > 8 entri) | Urutan key JSON dipertahankan (keputusan Fase 2 4.2); perbandingan context tidak peduli urutan (spesifikasi) |
| JSON → FEEL | `json.Decoder.UseNumber`; number literal diparse langsung ke desimal; object → context; array → list | Tidak lewat `float64`; aman untuk `int64` dan desimal panjang |
| FEEL → JSON | number → literal desimal kanonik (tanpa trailing zero, tanpa eksponen bila ≤ 34 digit); temporal → string ISO 8601 (zona IANA ditulis `@Zone` seperti feel-scala; verifikasi); durasi → string ISO 8601; range → **error konversi** (tidak bisa disimpan sebagai variabel; verifikasi perilaku Zeebe); function → error | Variabel adalah JSON (Fase 2); representasi harus bolak-balik untuk tipe yang didukung JSON. Deviasi yang diketahui: Zeebe mengonversi BigDecimal non-bulat ke double saat menyimpan (verifikasi lewat paritas `=1/3`); kita menyimpan desimal penuh dan mencatatnya sebagai deviasi bila terbukti |
| Variabel tidak ada | Path ke variabel/entri yang tidak ada → `null` **tanpa error** (perilaku Zeebe, verifikasi), sesuai subset Fase 2 | Spesifikasi menyatakan error, tetapi Zeebe mengembalikan `null` sehingga `=x = null` dipakai luas untuk "belum diisi" |
| Kompilasi dan cache | `Compile` dipanggil oleh adapter `execution/infrastructure/feel_evaluator.go` lewat **cache keyed teks ekspresi** (LRU 16.384 entri, sharded, hit rate dilaporkan metrik); **pemanasan saat deploy**: `bpmncheck` mengompilasi setiap ekspresi dan hasilnya dimasukkan ke cache yang sama lewat `deployment/api` → tidak perlu; cukup cache mengompilasi saat pertama dipakai (5.2) | Port `domain.ExpressionEvaluator` menerima `bpmn.Expression` (teks), jadi key teks tidak mengubah domain maupun `deployment/api`; teks ekspresi yang sama di banyak definisi berbagi AST (immutable, aman konkuren) |
| Scope variabel lokal | **Setiap element instance activity (task, sub-process, call activity, multi-instance body/inner) adalah scope variabel**; lookup berjalan dari scope terdekat ke root; variabel lokal lahir dari input mapping, `inputElement` multi-instance, `resultVariable`, job variables saat ada output mapping, dan `SetVariables local=true` (ADR-0028) | Sama dengan model scope Zeebe; skema `variable(scope_key, name)` Fase 2 sudah menampungnya; Fase 5 memakai scope untuk sub-process dan multi-instance |
| Propagasi | Menulis variabel "propagasi": cari scope terdekat yang sudah punya variabel bernama sama (naik ke atas), tulis di sana; bila tidak ada di mana pun, buat di **scope root process instance** (verifikasi dokumentasi Camunda 8.8 "variable propagation") | Perilaku Zeebe yang terdokumentasi; tanpa mapping dan tanpa scope lokal identik dengan Fase 2 |
| Validasi FEEL saat deploy | Paket baru **`libs/go/feel/bpmncheck`**: `Check(d *bpmn.Definitions) bpmn.Issues` berjalan setelah `bpmn.Validate`; kode `FEEL_SYNTAX_INVALID` (error), `FEEL_STATIC_TYPE_MISMATCH` (error, hanya untuk literal yang tipenya pasti salah), `IO_MAPPING_TARGET_INVALID` (error), `FEEL_UNKNOWN_FUNCTION` (warning) | `bpmn` tetap tidak bergantung pada `feel` (arah dependensi `feel → bpmn`), API `bpmn` tidak berubah; `bpmn.Code` adalah `string` sehingga kode baru bisa didefinisikan di luar paket `bpmn` |
| I/O mapping `=` | `source` **wajib** ekspresi FEEL (diawali `=`); `source` statis → `IO_MAPPING_SOURCE_NOT_EXPRESSION` (error) sampai diverifikasi; bila Camunda 8.8 menerima nilai statis sebagai literal string, aturan dilonggarkan mengikuti Zeebe dan dicatat di 13.1 | Modeler selalu menulis `=`; menolak lebih aman daripada menebak |
| Script task | `zeebe:script` dievaluasi **di dalam `Processor`** (sinkron, tanpa job), hasil ke `resultVariable` sebagai variabel **lokal** task lalu dipropagasi saat selesai (default: tanpa output mapping → ke root); verifikasi dokumentasi Camunda 8.8 tentang scope `resultVariable` | Perilaku Zeebe (verifikasi); script task dengan `taskDefinition` tetap job (Fase 2) |
| Batas evaluasi | `EvalLimits{MaxSteps: 1_000_000, MaxListLen: 1_000_000, MaxStringBytes: 16 MiB, MaxDepth: 256}`; melewati batas → `*feel.LimitError` → incident | `for x in 1..10^9` atau rekursi fungsi tidak boleh menggantung `Processor` |
| Model error | `*SyntaxError{Line, Column, Offset, Message}` dari `Compile`; `*EvalError{Message, Expr, Cause}` dari `Evaluate`; `*LimitError`; `*ResultTypeError{Expected, Actual}` dari adapter saat tipe hasil salah. Pesan incident: `failed to evaluate expression '<expr>': <message>` dan `Expected result of the expression '<expr>' to be '<TYPE>', but was '<TYPE>'.` (format mengikuti Zeebe; verifikasi teks persis) | Operate menampilkan pesan yang familiar bagi pengguna Camunda; test membandingkan prefiks, bukan teks utuh |
| `ExpressionService` | RPC kecil `EvaluateExpression(expression, variables JSON) → (result JSON | error)` di `workflow/expression/v1`, modul `expression` tipis (tanpa DB) | Dipakai modeler-web (coba ekspresi), harness paritas, dan dukungan; biaya < 0,5 hari |
| DMN | **Fase 7b** terpisah; `libs/go/dmn` kernel (allowlist `archtest` diperluas); evaluasi decision dari business rule task dilakukan **di dalam `execution/domain`** (kernel murni, record `DECISION_EVALUATION` dalam transaksi yang sama); modul baru `decision` memiliki `decision_instance` dan `DecisionService`; penyimpanan definisi DMN milik `deployment` (ADR-0029) | Atomisitas record instance tanpa transaksi lintas modul (prinsip Fase 2); `decision_instance` untuk jalur proses diproyeksikan dari outbox durable Fase 3 |
| Modeler FEEL | Editor FEEL bpmn.io (`@bpmn-io/feel-editor`, bagian dari properties panel Camunda) bila lisensi MIT terverifikasi; parsing sisi klien (`lezer-feel`) hanya untuk highlight; **sumber kebenaran validasi tetap `ValidateResource`** | Dua parser (JS dan Go) boleh berbeda di tepi; yang menentukan deploy adalah Go |
| DMN modeler | `dmn-js` memakai lisensi bpmn.io (watermark "Powered by bpmn.io" wajib dipertahankan; verifikasi teks lisensi saat 7b) — sama dengan bpmn-js yang sudah dipakai | Konsisten dengan keputusan Fase 0 untuk bpmn-js |

### 3.1 Representasi angka: perbandingan opsi

| Opsi | Eksak `+ - *` | `0.1 + 0.2 = 0.3` | `1/3` | `sqrt`, `log` | Alokasi | Catatan |
|---|---|---|---|---|---|---|
| `float64` | tidak | tidak | ya (biner) | ya | nol | Menyimpang dari Zeebe untuk desimal dan `int64` > 2^53 |
| `math/big.Rat` (Fase 2) | ya | ya | eksak rasional, **tidak bisa dicetak sebagai desimal** tanpa aturan pembulatan tambahan | tidak | tinggi | Cukup untuk subset, tidak untuk FEEL penuh |
| `math/big.Float` | tidak (biner) | tidak | ya | sebagian | sedang | Bukan desimal |
| **Desimal kustom (`big.Int` koefisien + eksponen, fast path `int64`)** | ya | ya | 34 digit half-even | lewat `big.Float` 128-bit lalu dibulatkan ke 34 digit (verifikasi akurasi 1 ulp terhadap `java.math.BigDecimal`/feel-scala) | rendah untuk angka kecil | Dipilih; ±600 baris + test |

Aturan tambahan: perbandingan angka eksak; `=` antara `1` dan `1.0` → `true`; pencetakan kanonik
tanpa trailing zero (`1.10` → `1.1`, `1.0` → `1`); `decimal(x, n)` membulatkan half-even
(verifikasi: feel-scala memakai `HALF_EVEN` untuk `decimal()`); `round half up/down/even` sesuai DMN 1.4.

### 3.2 Semantik: spesifikasi DMN FEEL vs feel-scala (Zeebe), dan yang kita ikuti

Setiap baris bertanda `verify` menjadi kasus paritas (bagian 9) dan hasil finalnya dikunci di
ADR-0027. Kolom "Kita" adalah rekomendasi awal.

| Konstruksi | Spesifikasi DMN 1.4/1.5 | feel-scala (Zeebe 8.8) | Kita |
|---|---|---|---|
| Variabel/entri tidak ada | error | `null` (dengan peringatan di log) | `null` (sesuai Fase 2); `verify` |
| `null = null`, `x = null` (x tidak ada) | `true` | `true` | `true` |
| `1 = "1"` | `null`? (tipe berbeda) | `false` (diduga) | ikuti hasil `verify`; sementara `false` |
| `1 < "a"`, `1 < null` | `null` | `null` | `null`; kondisi gateway → incident `CONDITION_ERROR` (keputusan Fase 2 #13, `verify`) |
| `true and null` / `false and null` / `null or true` | `null` / `false` / `true` | sama | sama |
| `and`/`or` dengan non-boolean | `null` | `null` | `null` |
| `if c then a else b`, `c` bukan boolean | cabang `else` | cabang `else` | cabang `else` |
| `"a" + 1` | error/`null` | `null` | `null` |
| `10 / 0` | `null` | `null` | `null`; `verify` apakah menghasilkan incident saat dipakai sebagai hasil script |
| `[1,2,3][0]`, `[1,2,3][4]` | `null` | `null` | `null` |
| `x[1]` dengan `x` bukan list | `x` (singleton) | `x` | `x` |
| `[1,[2]] = [1,[2]]`, `{a:1,b:2} = {b:2,a:1}` | `true` (deep, urutan context diabaikan) | `true` | `true` |
| `[{a:1},{a:2}].a` | `[1,2]` (proyeksi) | `[1,2]` | `[1,2]` |
| `date("2024-01-31") + duration("P1M")` | `2024-02-29` (clamp) | sama (java.time) | sama (`time.AddDate` **tidak** clamp — implementasi sendiri) |
| `duration("P1M") = duration("P30D")` | `null`/`false` (tipe berbeda) | `verify` | ikuti `verify` |
| `now()` | date and time dengan zona | zona JVM | zona `ENGINE_FEEL_TIMEZONE` (default UTC); `verify` representasi JSON |
| Nama dengan spasi (`first name`) | diizinkan (nama = token-token) | didukung bila ada di konteks; backtick `` `first name` `` sebagai ekstensi | backtick didukung; nama tanpa backtick yang mengandung spasi: **nanti** (parser dengan lookahead ke konteks ditinjau di 13.2 #4) |
| Komentar `//`, `/* */` | tidak dalam spesifikasi | didukung | didukung |
| String escape `\u` 4 hex, `\U` 6 hex | `\u` saja (DMN 1.4), `\U` ditambahkan 1.5? | keduanya (`verify`) | keduanya |
| Literal temporal `@"2024-01-01"` | ya (DMN 1.3+) | ya | ya |
| Range sebagai nilai `[1..10]` | ya (DMN 1.3+) | ya | ya; tidak bisa disimpan ke variabel (konversi JSON error) |
| `instance of` dengan tipe generik (`list<number>`, `context<a: number>`) | ya | tipe dasar saja (`verify`) | tipe dasar + `Any` + `list` + `context` + `function`; generik **nanti** |
| Fungsi ekstensi feel-scala (`is defined`, `get or else`, `assert`, `uuid`, `random number`, `trim`, `is blank`, `to base64`, `partition`, `last day of month` …) | tidak ada / sebagian di 1.5 | ada | ada yang dipakai umum: lihat 4.4 (`is defined`, `get or else`, `assert`, `uuid`, `random number`, `trim`, `is blank`, `last day of month`); `random number` memakai sumber acak yang di-inject agar `Processor` deterministik di replay (dicatat di record sebagai nilai) |
| Unary test dengan ekspresi boolean dan input non-boolean | aturan DMN 1.3 (hasil boolean dipakai bila `?` muncul atau input boolean; selain itu dibandingkan `=`) | `verify` | ikuti spesifikasi, `verify` di 7b |
| Hasil BigDecimal non-bulat disimpan ke variabel | — | dikonversi ke double saat ke MessagePack (`verify`) | desimal penuh; deviasi dicatat bila terbukti |

### 3.3 Tata letak paket `libs/go/feel`

```
libs/go/feel/
├─ project.json                 # feel-go: build, test, lint, fuzz, bench, conformance
├─ doc.go
├─ feel.go                      # Compile, CompileUnaryTests, Expression, UnaryTests, EvalOption
├─ value.go                     # Value, Kind, konstruktor, Entry, accessor
├─ json.go                      # FromJSON, ToJSON (kanonik)
├─ context.go                   # Context, MapContext, ChainContext, FuncContext
├─ errors.go                    # SyntaxError, EvalError, LimitError, ResultTypeError
├─ limits.go                    # EvalLimits, DefaultLimits
├─ internal/decimal/            # desimal kustom (3.1) + fuzz terhadap big.Rat/big.Float
├─ internal/temporal/           # parsing ISO 8601, aritmetika tanggal (clamp), zona
├─ internal/lexer/              # token, posisi, nama multi-kata via backtick
├─ internal/parser/             # Pratt parser → ast; presedensi 4.1
├─ internal/ast/                # node AST immutable, Walk, FreeNames
├─ internal/eval/               # evaluator, operator, filter/path, fungsi, limit
├─ internal/builtin/            # katalog fungsi bawaan per kategori (string.go, number.go, list.go, ...)
├─ bpmncheck/                   # Check(*bpmn.Definitions) bpmn.Issues  (meng-import libs/go/bpmn)
├─ feeltest/                    # helper tabel golden untuk pemakai (engine, dmn)
├─ cmd/feel/                    # REPL/CLI kecil: `go run ./libs/go/feel/cmd/feel -e '1+1' -v vars.json`
├─ deps_test.go                 # hanya stdlib (bpmncheck boleh libs/go/bpmn)
└─ testdata/
   ├─ golden/*.json             # {expr, vars, want | wantError}
   ├─ conformance/              # kasus hasil porting (5.6) + skip-list.json
   ├─ parity/*.json + expectations.json
   └─ fuzz/FuzzParse, FuzzEvaluate
```

`cmd/feel` adalah paket `main` di dalam kernel; `deps_test.go` memeriksa paket `feel` dan
subpaket `internal/*` saja (CLI boleh `flag`/`os`, tetap stdlib).

### 3.4 Validasi FEEL saat deploy tanpa mengubah API `bpmn`

Pilihan yang dipertimbangkan:

| Opsi | Kelebihan | Kekurangan |
|---|---|---|
| A. `bpmn.Validate` memanggil `feel.Compile` langsung | Satu tempat validasi | `bpmn` bergantung pada `feel`; mengubah API/dependensi kernel Fase 1 |
| B. `bpmn.Validate` menerima hook `WithExpressionChecker(func(string) error)` | `bpmn` tetap bebas `feel` | Mengubah tanda tangan `Validate`/menambah opsi; hook kehilangan informasi atribut |
| **C. Paket `feel/bpmncheck` menelusuri `*bpmn.Definitions` dan menghasilkan `bpmn.Issues`** | `bpmn` tidak berubah; tahu konteks atribut (kondisi vs mapping vs timer) sehingga bisa memeriksa tipe literal dan target | Dua penelusuran model saat deploy (murah: deploy bukan hot path) |

**Dipilih C.** `deployment/application` memanggil `bpmn.ParseAndValidate` lalu
`bpmncheck.Check(defs)`; issue digabung. `bpmncheck` juga dipakai `ValidateResource` (modeler) dan
CLI lint. Daftar atribut yang diperiksa ada di 6.7.

### 3.5 DMN: Fase 7 atau 7b?

| Opsi | Kelebihan | Kekurangan |
|---|---|---|
| A. DMN di dalam Fase 7 | Satu fase "ekspresi dan keputusan" | Fase 7 menjadi ±40 hari; Fase 8 (keamanan, deploy produksi) tertunda; DMN jarang dipakai di awal adopsi |
| **B. Fase 7b terpisah, dokumen yang sama** | Fase 7a (±25 hari) langsung membuka I/O mapping yang menghalangi banyak model; 7b bisa dijadwalkan setelah Fase 8 bila produksi lebih mendesak | Business rule task tetap job-based sampai 7b (sudah bisa dipakai lewat worker) |
| C. Tidak ada DMN | Hemat | Roadmap menjanjikan "opsional"; kehilangan paritas fitur yang terlihat di Modeler |

**Dipilih B.** Business rule task dengan `zeebe:taskDefinition` sudah berjalan sebagai job sejak
Fase 2; `zeebe:calledDecision` ditolak `UNSUPPORTED_ELEMENT` dengan pesan "DMN tersedia di Fase 7b"
sampai 7b selesai.

## 4. Bahasa FEEL yang diimplementasikan

### 4.1 Grammar (ringkas, EBNF) dan presedensi

Grammar mengikuti klausul 10.3 spesifikasi DMN 1.4/1.5 (verifikasi nomor aturan saat implementasi).
Parser Pratt dengan presedensi dari rendah ke tinggi:

```
1. for … return, if … then … else, some/every … satisfies   (meluas sejauh mungkin ke kanan)
2. or
3. and
4. comparison: = != < <= > >=, between … and …, in (…)       (non-asosiatif)
5. + -
6. * /
7. unary -                                                   (verifikasi posisi relatif terhadap **)
8. **                                                        (asosiatif kanan)
9. instance of                                               (verifikasi: spesifikasi menaruhnya di atas comparison)
10. postfix: path (.name), filter ([…]), invocation ((…))
```

```
expression      = textualExpr
textualExpr     = forExpr | ifExpr | quantifiedExpr | disjunction
forExpr         = "for" name "in" iterCtx { "," name "in" iterCtx } "return" expression
iterCtx         = expression [ ".." expression ]          // list, atau rentang bilangan bulat
ifExpr          = "if" expression "then" expression "else" expression
quantifiedExpr  = ("some" | "every") name "in" expression { "," name "in" expression } "satisfies" expression
disjunction     = conjunction { "or" conjunction }
conjunction     = comparison { "and" comparison }
comparison      = arith [ ( "=" | "!=" | "<" | "<=" | ">" | ">=" ) arith
                        | "between" arith "and" arith
                        | "in" positiveUnaryTest
                        | "in" "(" positiveUnaryTests ")" ]
arith           = term { ("+" | "-") term }
term            = factor { ("*" | "/") factor }
factor          = [ "-" ] power
power           = instanceOf [ "**" factor ]
instanceOf      = postfix [ "instance" "of" type ]
postfix         = primary { "." name | "[" expression "]" | "(" [ args ] ")" }
args            = positionalArgs | namedArgs
namedArgs       = name ":" expression { "," name ":" expression }
primary         = literal | name | "(" expression ")" | listLiteral | contextLiteral
                | rangeLiteral | functionDef | simplePositiveUnaryTest
literal         = number | string | boolean | "null" | temporalLiteral   // @"..."
listLiteral     = "[" [ expression { "," expression } ] "]"
contextLiteral  = "{" [ ctxEntry { "," ctxEntry } ] "}"
ctxEntry        = (name | string) ":" expression
rangeLiteral    = ("(" | "]" | "[") expression ".." expression (")" | "[" | "]")
functionDef     = "function" "(" [ name { "," name } ] ")" expression   // "external" ditolak
type            = "Any" | "null" | "number" | "string" | "boolean" | "date" | "time"
                | "date and time" | "day-time-duration" | "year-month-duration"
                | "list" | "context" | "function" | qualifiedName
                | "list" "<" type ">" | "context" "<" … ">" | "function" "<" … ">" "->" type   // nanti
name            = identifierToken | "`" chars "`"            // nama multi-kata: 13.2 #4
simplePositiveUnaryTest = ("<" | "<=" | ">" | ">=") endpoint | rangeLiteral
positiveUnaryTests = positiveUnaryTest { "," positiveUnaryTest }
unaryTests      = "-" | positiveUnaryTests | "not" "(" positiveUnaryTests ")"
```

Catatan lexer: angka `12`, `1.5`, `.5`, `1e3` (verifikasi apakah notasi eksponen diterima
feel-scala); string `"..."` dengan escape `\" \\ \n \r \t \uXXXX \UXXXXXX`; kata kunci
(`for`, `if`, `then`, `else`, `some`, `every`, `in`, `return`, `satisfies`, `and`, `or`, `not`,
`between`, `instance`, `of`, `function`, `true`, `false`, `null`) bukan nama kecuali di dalam
backtick; `?` hanya valid di unary test; komentar `//` sampai akhir baris dan `/* … */`; nama
fungsi bawaan multi-kata (`string length`, `date and time`) dikenali oleh parser sebagai satu nama
lewat **tabel nama dikenal** (daftar nama bawaan + key konteks yang diketahui saat compile tidak
tersedia → parser mencoba gabungan token terpanjang yang cocok dengan tabel bawaan; nama variabel
multi-kata pengguna butuh backtick di Fase 7).

### 4.2 Tipe nilai

| Kind | Representasi Go | JSON | Keterangan |
|---|---|---|---|
| `null` | — | `null` | |
| `boolean` | `bool` | `true/false` | |
| `number` | `decimal.Decimal` | number | 3.1 |
| `string` | `string` (UTF-8; panjang dihitung per code point) | string | `string length("é")` = 1 |
| `date` | `temporal.Date{y,m,d}` | `"2024-01-31"` | |
| `time` | `temporal.Time{nanos, offset *int32, zone string}` | `"10:00:00"`, `"10:00:00+07:00"`, `"10:00:00@Asia/Jakarta"` | zona opsional |
| `date and time` | `time.Time` + `zone string` + flag lokal | `"2024-01-31T10:00:00"`, `…+07:00`, `…@Asia/Jakarta` | |
| `days and time duration` | `int64` nanodetik | `"PT1H30M"`, `"P2DT3H"`, `"-PT1S"` | |
| `years and months duration` | `int32` bulan | `"P1Y2M"` | |
| `list` | `[]Value` | array | |
| `context` | `[]Entry` + indeks | object | urutan dipertahankan |
| `range` | `{start, end Value; startIncl, endIncl bool}` | **tidak bisa** | endpoint number/temporal/string |
| `function` | `*Function{params, body ast | builtin}` | **tidak bisa** | closure atas konteks definisi |

### 4.3 Aturan operator ringkas

- Aritmetika: number×number; `date and time ± duration`, `date ± duration`, `time ± days-time
  duration`, `date − date → days-time duration`, `date and time − date and time → days-time
  duration`, `duration ± duration` (jenis sama), `duration × number`, `duration / number`,
  `duration / duration → number` (jenis sama); `string + string` → konkatenasi (spesifikasi FEEL;
  verifikasi feel-scala); kombinasi lain → `null`.
- Perbandingan: hanya antar tipe sama (number, string, boolean untuk `=`/`!=` saja, temporal
  sejenis, durasi sejenis, list, context, range untuk `=`); `date` vs `date and time` → `null`;
  `date and time` lokal vs berzona → `null` (verifikasi); `time` berzona dibandingkan setelah
  normalisasi ke UTC.
- `between a and b` ≡ `a <= x and x <= b`; `x in (…)` mengevaluasi unary tests dengan `x` sebagai
  input.
- Filter: indeks (number bulat: positif 1-based, negatif dari belakang), predikat (boolean) dengan
  `item` dan — bila elemen context — entri context langsung sebagai nama; `item` juga tersedia untuk
  non-context.
- Path pada context → nilai entri; pada list → proyeksi; pada `null` → `null`; pada tipe lain →
  `null`. Path pada temporal: `.year .month .day .hour .minute .second .time offset .timezone
  .weekday .days .hours .minutes .seconds .years .months` sesuai spesifikasi.
- Definisi fungsi: closure; rekursi lewat context (`{ f: function(n) if n < 2 then n else f(n-1) + f(n-2) }`)
  dibatasi `MaxDepth`.
- Pemanggilan: positional atau named (nama parameter bawaan mengikuti spesifikasi, mis.
  `substring(string: "abc", start position: 2)`); jumlah argumen salah → `null` dan peringatan
  (feel-scala) — `verify`, sementara `null`.

### 4.4 Katalog fungsi bawaan dan matriks cakupan

Legenda: **W** = wajib Fase 7; **N** = nanti (ditambahkan saat dibutuhkan, dicatat `FEEL_UNKNOWN_FUNCTION` warning saat deploy); **Z** = ekstensi feel-scala (bukan spesifikasi).

| Kategori | Fungsi | Cakupan |
|---|---|---|
| Konversi | `date`, `time`, `date and time`, `duration`, `years and months duration`, `number`, `string` | W |
| Boolean | `not`, `is defined` (Z), `get or else` (Z), `assert` (Z) | W |
| String | `substring`, `string length`, `upper case`, `lower case`, `substring before`, `substring after`, `contains`, `starts with`, `ends with`, `matches`, `replace`, `split`, `string join` | W |
| String (lanjutan) | `trim` (Z), `is blank` (Z), `uuid` (Z), `to base64` (Z), `from base64` (Z, verifikasi nama) | W kecuali base64: N |
| Angka | `decimal`, `floor`, `ceiling`, `round up`, `round down`, `round half up`, `round half down`, `round half even`, `abs`, `modulo`, `sqrt`, `log`, `exp`, `odd`, `even`, `random number` (Z) | W |
| List | `list contains`, `count`, `min`, `max`, `sum`, `mean`, `median`, `stddev`, `mode`, `all`, `any`, `and` (alias), `or` (alias), `sublist`, `append`, `concatenate`, `insert before`, `remove`, `reverse`, `index of`, `union`, `distinct values`, `duplicate values`, `flatten`, `product`, `sort`, `list replace` | W (`list replace`, `duplicate values`: DMN 1.5, W karena murah) |
| List (lanjutan) | `partition` (Z) | N |
| Context | `get value`, `get entries`, `context put`, `context merge`, `context` | W |
| Temporal | `now`, `today`, `day of week`, `day of year`, `week of year`, `month of year`, `abs` (durasi), `last day of month` (Z/1.5) | W |
| Range | `before`, `after`, `meets`, `met by`, `overlaps`, `overlaps before`, `overlaps after`, `finishes`, `finished by`, `includes`, `during`, `starts`, `started by`, `coincides` | W (banyak, tetapi mekanis; satu tabel uji dari spesifikasi) |
| Lain | `is` (DMN 1.5, kesetaraan tipe), fungsi `external` | N / tidak akan |

`matches`/`replace`/`split` memakai `regexp` stdlib (sintaks RE2), **bukan** regex Java/XPath yang
dipakai feel-scala: perbedaan (lookahead, backreference) dicatat sebagai deviasi di ADR-0027 dan
terdeteksi saat compile pola (`EvalError` dengan pesan jelas). Flag `replace(…, "i")` dipetakan ke
`(?i)`; flag `s`, `m`, `x` dipetakan ke padanan RE2 (`x` tidak ada → error).

### 4.5 Unary tests

`CompileUnaryTests(src)` menghasilkan `*UnaryTests` dengan `Evaluate(input Value, ctx Context)
(bool, error)`:

- `-` → selalu `true`; daftar `a, b, c` → `or`; `not(a, b)` → negasi `or`.
- Tes positif sederhana (`< 10`, `[1..5]`, `"a"`, `date("…")`) → perbandingan dengan input.
- Ekspresi umum: dievaluasi dengan `?` terikat ke input; bila hasilnya boolean → hasil itu; bila
  bukan boolean → `input = hasil` (aturan DMN 1.3; `verify` feel-scala untuk input boolean).
- Dipakai oleh `x in (…)` (7a) dan sel decision table (7b).

## 5. API paket, kompilasi, dan kualitas

### 5.1 Sketsa API (perluasan kompatibel dari Fase 2)

```go
package feel

// Dipertahankan dari Fase 2.
func Compile(src string) (*Expression, error)                       // error: *SyntaxError
func (e *Expression) Evaluate(ctx Context) (Value, error)           // = EvaluateWith(ctx) dengan DefaultLimits, UTC, clock sistem
func FromJSON(raw []byte) (Value, error)
type Context interface{ Lookup(name string) (Value, bool) }

// Baru di Fase 7.
func (e *Expression) EvaluateWith(ctx Context, opts ...EvalOption) (Value, error)
func (e *Expression) FreeNames() []string      // nama variabel yang dirujuk (untuk UI dan filter variabel job)
func (e *Expression) Source() string

func CompileUnaryTests(src string) (*UnaryTests, error)
func (u *UnaryTests) Evaluate(input Value, ctx Context, opts ...EvalOption) (bool, error)

type EvalOption func(*evalConfig)
func WithClock(now func() time.Time) EvalOption
func WithLocation(loc *time.Location) EvalOption
func WithLimits(l EvalLimits) EvalOption
func WithRandom(src func() float64) EvalOption   // untuk random number(); engine memasok sumber deterministik per record

type Kind uint8 // KindNull, KindBoolean, KindNumber, KindString, KindDate, KindTime, KindDateTime,
                // KindDaysTimeDuration, KindYearsMonthsDuration, KindList, KindContext, KindRange, KindFunction

type Value struct{ /* unexported */ }
func (v Value) Kind() Kind
func (v Value) IsNull() bool
func (v Value) Bool() (bool, bool)
func (v Value) String() (string, bool)
func (v Value) Number() (Number, bool)         // Number: Int64() (int64, bool), Text() string, Cmp
func (v Value) List() ([]Value, bool)
func (v Value) Entries() ([]Entry, bool)       // context, urutan dipertahankan
func (v Value) Get(key string) (Value, bool)   // context lookup O(1) untuk > 8 entri
func ToJSON(v Value) ([]byte, error)           // kanonik; range/function → *ConversionError

func Null() Value
func Boolean(b bool) Value
func NumberFromInt(i int64) Value
func NumberFromString(s string) (Value, error)
func String(s string) Value
func List(items ...Value) Value
func NewContext(entries ...Entry) Value

type MapContext map[string]Value                           // Lookup dari map
func ChainContext(inner, outer Context) Context            // scope lokal → induk
type Entry struct{ Key string; Value Value }

type SyntaxError struct{ Line, Column, Offset int; Message string }
type EvalError struct{ Message string; Expr string; Cause error }
type LimitError struct{ Limit string; Max int64 }
type ConversionError struct{ Kind Kind }
```

### 5.2 Kompilasi, cache, dan konteks evaluasi di engine

```go
// execution/infrastructure/feel_evaluator.go
type FeelEvaluator struct {
	cache   *exprCache          // LRU sharded 16 shard × 1024, key: teks ekspresi tanpa "="
	clock   domain.Clock
	loc     *time.Location
	limits  feel.EvalLimits
	metrics Metrics             // feel_cache_hits_total, feel_cache_misses_total, feel_eval_duration_seconds
}

func (f *FeelEvaluator) compile(expr bpmn.Expression) (*feel.Expression, error) {
	src := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(expr.Raw), "="))
	if e, ok := f.cache.Get(src); ok { return e, nil }
	e, err := feel.Compile(src)
	if err != nil { return nil, err }        // tidak terjadi untuk definisi yang lolos deploy; tetap ditangani → incident
	f.cache.Add(src, e)
	return e, nil
}
```

Port domain diperluas (kompatibel):

```go
// execution/domain/ports.go
type ExpressionEvaluator interface {
	EvaluateBool(expr bpmn.Expression, vars VariableLookup) (bool, error)
	EvaluateString(expr bpmn.Expression, vars VariableLookup) (string, error)
	EvaluateInt(expr bpmn.Expression, vars VariableLookup) (int64, error)
	// Fase 7:
	EvaluateValue(expr bpmn.Expression, vars VariableLookup) (feel.Value, error)   // untuk mapping, script, koleksi
	EvaluateList(expr bpmn.Expression, vars VariableLookup) ([]feel.Value, error)
	EvaluateTemporal(expr bpmn.Expression, vars VariableLookup) (TemporalResult, error) // timer, dueDate
}

// VariableLookup: Fase 2 membaca satu scope; Fase 7 menerima rantai scope.
type VariableLookup interface {
	Lookup(name string) (json.RawMessage, bool)  // mencari dari scope terdekat ke root
}
```

Ekspresi statis (`IsFEEL == false`) tetap dipakai apa adanya (type job statis, timer ISO 8601
statis) — tidak lewat FEEL, sama seperti Fase 2.

Konteks evaluasi: adapter membuat `scopeContext{state, scopeKey}` yang mengimplementasikan
`feel.Context` dengan lookup lazy: `FromJSON` hanya untuk variabel yang benar-benar dirujuk (nama
diambil saat lookup, bukan seluruh dokumen), hasil konversi di-cache per command. Ini menjaga hot
path `x > 10` tanpa mem-parse seluruh variabel instance.

### 5.3 Model error dan incident

| Sumber | Error Go | Incident (tipe) | Pesan (format Zeebe, verifikasi teks) |
|---|---|---|---|
| Sintaks (hanya jika definisi lama lolos tanpa cek) | `*feel.SyntaxError` | `EXTRACT_VALUE_ERROR` | `failed to parse expression '<expr>': <msg>` |
| Evaluasi gagal (fungsi error, batas) | `*feel.EvalError`, `*feel.LimitError` | sesuai konteks (6.6) | `failed to evaluate expression '<expr>': <msg>` |
| Tipe hasil salah | `*ResultTypeError` | sesuai konteks | `Expected result of the expression '<expr>' to be '<TYPE>', but was '<TYPE>'.` |
| Konversi hasil ke JSON (range/function) | `*feel.ConversionError` | `IO_MAPPING_ERROR` / `EXTRACT_VALUE_ERROR` | `failed to convert result of expression '<expr>' to variable: <kind> is not supported` |

Pesan dipotong 2.000 karakter; teks ekspresi di pesan dipotong 200 karakter.

### 5.4 Fuzzing

- `FuzzParse`: input acak + korpus seed dari golden; invariant: tidak panic, waktu < 50 ms, `Compile`
  sukses ⇒ `Source()` di-`Compile` ulang menghasilkan AST yang sama (round-trip printer).
- `FuzzEvaluate`: pasangan (ekspresi dari korpus yang dimutasi, konteks JSON acak); invariant: tidak
  panic, berhenti di `DefaultLimits` (langkah ≤ 1e6, memori ≤ 64 MiB diukur `runtime.MemStats` di
  test), hasil `ToJSON` ⇒ `FromJSON` ⇒ sama (untuk kind yang bisa dikonversi).
- `FuzzDecimal`: operasi acak dibandingkan dengan `big.Rat` (untuk `+ - *`) dan dengan `big.Float`
  256-bit dibulatkan (untuk `/`), toleransi 1 ulp pada digit ke-34.
- `FuzzTemporal`: parsing ISO 8601 dibandingkan dengan `time.Parse` untuk bentuk yang didukung
  keduanya; aritmetika bulan dibandingkan dengan oracle sederhana.

### 5.5 Target performa dan benchmark (`nx run feel-go:bench`)

| Benchmark | Target awal (dikunci setelah pengukuran pertama) |
|---|---|
| `BenchmarkCompile/condition` (`x > 10 and status = "OK"`) | < 3 µs, < 2 KB alokasi |
| `BenchmarkCompile/1KB` | < 50 µs |
| `BenchmarkEvaluate/condition` (konteks 5 variabel, nilai sudah dikonversi) | < 300 ns, 0 alokasi |
| `BenchmarkEvaluate/mapping-20` (20 input mapping dengan path dan fungsi string) | < 20 µs |
| `BenchmarkEvaluate/list-1000` (`sum(for x in list return x * 2)`) | < 400 µs |
| `BenchmarkFromJSON/16KB` | < 150 µs |
| Engine (`execution`): proses linear Fase 2 dengan 20 service task + 2 mapping per task | p50 naik < 10% dibanding Fase 2 |

### 5.6 Korpus konformansi

- **DMN TCK** (`github.com/dmn-tck/tck`): lisensi harus diverifikasi (diduga Apache 2.0). Kasus
  FEEL berada di `TestCases/compliance-level-3/` sebagai model DMN dengan literal expression. Di 7a
  (belum ada parser DMN) dibuat `tools/scripts/tck-extract` (Go) yang membaca `.dmn` TCK dengan
  `encoding/xml` minimal (hanya `decision/literalExpression/text` dan `inputData`) plus
  `*-test-01.xml` (input/expected) dan menghasilkan `testdata/conformance/tck/*.json`. Kasus yang
  memakai boxed expression selain literal dilewati. `skip-list.json` berisi id kasus + alasan
  (spesifikasi vs feel-scala, fitur N). Di 7b runner memakai `libs/go/dmn` langsung.
- **feel-scala test suite** (Apache 2.0): kasus yang menggambarkan perilaku Zeebe (null, variabel
  tidak ada, fungsi ekstensi) di-port **manual** ke golden dengan atribusi di header file; bukan
  salinan massal.
- Kedua sumber hanya dipakai sebagai data uji; tidak ada kode yang disalin.

## 6. Integrasi di engine

### 6.1 Scope variabel lokal (ADR-0028)

- Setiap element instance bertipe activity (task apa pun, sub-process, call activity, multi-instance
  body dan inner instance) dan event sub-process menjadi **scope**: `scope_key` = key element
  instance tersebut. Gateway dan event biasa bukan scope (variabel mapping pada event
  (`zeebe:ioMapping` di catch event) ditulis ke scope induk — verifikasi perilaku Zeebe untuk
  output mapping pada intermediate catch event).
- Lookup: `scopeChain(elementInstanceKey)` = [scope sendiri, induk, …, root]; nama pertama yang
  ditemukan menang.
- Record baru/penyesuaian: `VARIABLE CREATED/UPDATED` membawa `scope_key`; saat scope `COMPLETED`/
  `TERMINATED`, variabel lokalnya dihapus dari state runtime (`Apply`) dan proyeksi (`variable`
  dengan `scope_key` itu dihapus; riwayat tetap di `execution_record`). Verifikasi: Zeebe
  menghapus variabel lokal saat scope selesai (ya, variabel hidup bersama scope).
- `GetProcessInstance` mengembalikan variabel dengan `scope_key` (field proto `scope_key` sudah ada
  sejak Fase 2? bila belum: tambahan aditif `int64 scope_key`); Operate menampilkan variabel per
  scope.
- `SetVariables(local = true, scope_key = element instance aktif)` menulis ke scope itu;
  `local = false` memakai aturan propagasi 6.2.

### 6.2 Input/output mapping

Algoritma (mengikuti dokumentasi Camunda 8.8 "Input/output variable mappings" dan "Variables";
setiap butir yang bertanda `verify` masuk paritas):

**Saat `ELEMENT_ACTIVATING` activity dengan `Inputs`:**

1. Buat scope untuk element instance (sudah terjadi untuk semua activity).
2. Untuk setiap `Mapping{Source, Target}` **urut dokumen**: evaluasi `Source` dengan rantai scope
   yang dimulai dari **scope induk** (variabel lokal hasil mapping sebelumnya **ikut terlihat**
   untuk mapping berikutnya — `verify`; feel-scala/Zeebe mengevaluasi semua input mapping terhadap
   konteks yang sama lalu menulis sekaligus, diduga; pilih yang terbukti).
3. Tulis hasil ke `Target` di scope lokal. `Target` boleh path `a.b.c`: entri context dibuat/digabung
   (`a` lokal baru berisi `{b: {c: v}}`; jika `a` sudah ada lokal dan context, digabung; jika `a`
   ada di scope induk, **tidak** diubah — dibuat salinan lokal dengan entri baru? `verify`).
4. Gagal (evaluasi, konversi) → incident `IO_MAPPING_ERROR` pada element instance di state
   `ACTIVATING`; resolusi incident (Fase 4) mengulang langkah aktivasi.

**Saat `ELEMENT_COMPLETING` activity:**

1. Jika elemen punya `Outputs` **dan** penyelesaiannya membawa variabel (job completion, message
   payload, hasil call activity, `resultVariable` script/decision): variabel itu ditulis sebagai
   **lokal** di scope elemen (bukan dipropagasi).
2. Untuk setiap `Mapping` urut dokumen: evaluasi `Source` dengan rantai scope dari **scope lokal
   elemen**; hasil ditulis ke `Target` di **scope induk** dengan aturan **propagasi** (cari scope
   yang sudah punya nama itu dari induk ke atas; bila tidak ada, buat di root — `verify`).
3. Jika elemen **tidak** punya `Outputs`: variabel penyelesaian dipropagasi langsung (aturan yang
   sama dengan butir 2), identik dengan Fase 2 saat hanya ada scope root.
4. Gagal → incident `IO_MAPPING_ERROR` di state `COMPLETING`.

**Sub-process dan call activity:** input mapping sub-process mengisi scope sub-process; output
mapping sub-process dievaluasi atas scope sub-process saat selesai. Call activity: variabel ke
child mengikuti `propagateAllParentVariables` (default `true`: semua variabel yang terlihat dari
scope call activity disalin ke root child) + input mapping (lokal call activity → ikut tersalin);
hasil child ke parent mengikuti `propagateAllChildVariables` (default `true` bila tanpa output
mapping; `false` bila ada output mapping — `verify` default persis 8.8) + output mapping.

**Multi-instance (Fase 5 pemilik perilaku):** `inputCollection` dievaluasi di scope body
(aktivasi); `inputElement` ditulis lokal di tiap inner instance; `outputElement` dievaluasi di scope
inner saat selesai dan dikumpulkan ke `outputCollection` lokal di scope body, lalu dipropagasi saat
body selesai; `completionCondition` dievaluasi di scope inner setelah tiap penyelesaian (`verify`
scope persis). Input/output mapping pada activity multi-instance berlaku untuk **inner instance**
(Zeebe: mapping ada pada activity, diterapkan tiap instance — `verify`).

Skema tidak berubah (`variable.scope_key` sudah ada). Kueri sqlc baru: `DeleteVariablesByScope`,
`ListVariablesByScopes`.

### 6.3 Script task

- `Activity.Script()` ada dan `TaskDefinition().IsSet()` tidak → perilaku **script FEEL**:
  `ELEMENT_ACTIVATING` → input mapping → `ELEMENT_ACTIVATED` → evaluasi `Script.Expression`
  (wajib `IsFEEL`; `bpmncheck` menolak yang tidak) dengan rantai scope task → hasil ke
  `ResultVariable` sebagai variabel **lokal** task → `ELEMENT_COMPLETING` (aturan 6.2 butir 1–3:
  `resultVariable` dipropagasi bila tanpa output mapping) → `ELEMENT_COMPLETED`.
- Gagal evaluasi → incident `EXTRACT_VALUE_ERROR`? Zeebe memakai tipe khusus untuk script? —
  `verify` daftar `ErrorType` 8.8; sementara `EXTRACT_VALUE_ERROR` di state `ACTIVATED`.
- Keduanya ada (`Script` dan `TaskDefinition`) → `bpmncheck`/`bpmn` error `SCRIPT_TASK_AMBIGUOUS`
  (verifikasi aturan Modeler: Modeler memilih salah satu "implementation").
- `ProfileExecutable`: script task dibuka (Fase 2 membuka hanya yang berbasis job? — menyesuaikan).

### 6.4 Ekspresi lain yang dievaluasi FEEL penuh (pemilik perilaku: fase masing-masing)

| Tempat | Ekspresi | Tipe hasil yang diterima | Gagal → incident |
|---|---|---|---|
| Sequence flow | `conditionExpression` (`=` wajib; tanpa `=` ditolak deploy, keputusan Fase 2 #5 tetap) | boolean; `null`/lainnya → incident (keputusan Fase 2 #13, `verify`) | `CONDITION_ERROR` di gateway |
| Task definition | `type`, `retries` | string non-kosong; number bulat ≥ 0 | `EXTRACT_VALUE_ERROR` |
| Timer (Fase 5) | `timeDuration`: durasi FEEL atau string ISO 8601; `timeCycle`: string (`R3/PT10S`, ISO berulang, atau cron — `verify` dukungan cron 8.8 untuk timer start/boundary); `timeDate`: date and time atau string ISO 8601 dengan zona | sesuai kolom | `EXTRACT_VALUE_ERROR` |
| Multi-instance (Fase 5) | `inputCollection` → list; `outputElement` → nilai apa pun yang bisa ke JSON; `completionCondition` → boolean | | `EXTRACT_VALUE_ERROR` |
| Message (Fase 5) | `correlationKey` → string atau number (dikonversi ke string kanonik); `name` message bila ekspresi (`verify` 8.8) → string | | `EXTRACT_VALUE_ERROR` |
| Call activity (Fase 5) | `processId` → string | | `EXTRACT_VALUE_ERROR` (`CALLED_ELEMENT_ERROR` bila proses tidak ada) |
| User task (Fase 6) | `assignee` → string; `candidateGroups`/`candidateUsers` → list of string **atau** string dipisah koma; `dueDate`/`followUpDate` → date and time atau string ISO 8601 | | `EXTRACT_VALUE_ERROR` |
| Form (Fase 6) | `externalReference`, `formId` bila ekspresi (`verify`) | string | `EXTRACT_VALUE_ERROR` |
| Business rule (7b) | `decisionId` bila ekspresi (`verify`) | string | `CALLED_DECISION_ERROR` |

Fase 5 dan 6 menulis perilaku elemen memakai port Fase 2 (`EvaluateString`, `EvaluateInt`,
`EvaluateBool`) ditambah `EvaluateList`/`EvaluateTemporal` yang bisa dipalsukan sampai Fase 7 ada;
Fase 7 hanya mengganti implementasi dan menambah skenario.

### 6.5 Urutan record untuk activity dengan mapping

```
ELEMENT_ACTIVATING   task          (scope task dibuat)
VARIABLE CREATED     scope=task    (hasil input mapping, satu record per target)
ELEMENT_ACTIVATED    task
JOB CREATED
… CompleteJob {a: 1} …
VARIABLE CREATED     scope=task    a=1        (hanya bila ada output mapping)
ELEMENT_COMPLETING   task
VARIABLE CREATED/UPDATED scope=<propagasi>   (hasil output mapping)
ELEMENT_COMPLETED    task          (Apply: variabel scope task dihapus dari state runtime)
```

Urutan persis relatif `ELEMENT_COMPLETING` vs `VARIABLE` diverifikasi terhadap ekspor record
Camunda 8.8 bila tersedia; bukan bagian kontrak paritas.

### 6.6 Tabel incident Fase 7

| Tipe | Kondisi | Elemen berhenti di |
|---|---|---|
| `CONDITION_ERROR` | (Fase 2) kondisi gagal/bukan boolean/tidak ada flow | gateway |
| `EXTRACT_VALUE_ERROR` | ekspresi atribut (type, retries, timer, koleksi, correlation key, processId, assignment, script) gagal atau tipe salah | elemen pada state saat evaluasi |
| `IO_MAPPING_ERROR` | input/output mapping gagal | `ACTIVATING` / `COMPLETING` |
| `DECISION_EVALUATION_ERROR` (7b) | decision gagal dievaluasi | business rule task `ACTIVATED` |
| `CALLED_DECISION_ERROR` (7b) | decision tidak ditemukan | business rule task `ACTIVATING` |

Nama mengikuti `ErrorType` Zeebe (verifikasi daftar 8.8). Resolusi (Fase 4) mengulang langkah yang
gagal; untuk `IO_MAPPING_ERROR` saat `COMPLETING`, pengulangan mengevaluasi ulang output mapping.

### 6.7 Validasi deploy: `bpmncheck`

Atribut yang diperiksa (semua `Expression` dengan `IsFEEL`): `conditionExpression`,
`taskDefinition.type/retries`, `ioMapping.input/output.source`, `script.expression` (wajib FEEL),
`timer.value`, `loopCharacteristics.inputCollection/outputElement/completionCondition`,
`subscription.correlationKey` (wajib FEEL), `calledElement.processId`,
`assignmentDefinition.*`, `taskSchedule.*`, `formDefinition.externalReference` (bila FEEL),
`calledDecision.decisionId` (bila FEEL, 7b).

| Kode | Severity | Kondisi |
|---|---|---|
| `FEEL_SYNTAX_INVALID` | error | `feel.Compile` gagal; `Message` memuat posisi kolom dalam ekspresi dan atribut |
| `FEEL_STATIC_TYPE_MISMATCH` | error | ekspresi adalah literal tunggal yang tipenya pasti salah (`retries="=\"abc\""`, `inputCollection="=1"`) |
| `FEEL_UNKNOWN_FUNCTION` | warning | pemanggilan fungsi dengan nama yang bukan bawaan dan bukan nama yang didefinisikan di ekspresi itu (bisa fungsi dari variabel context — karena itu warning) |
| `IO_MAPPING_SOURCE_NOT_EXPRESSION` | error | `source` tidak diawali `=` (lihat keputusan di bagian 3; bisa berubah setelah verifikasi) |
| `IO_MAPPING_TARGET_INVALID` | error | `target` bukan nama atau path nama (`a`, `a.b`); nama tidak boleh kata kunci FEEL (`verify` aturan Zeebe) |
| `SCRIPT_TASK_AMBIGUOUS` | error | `zeebe:script` dan `zeebe:taskDefinition` sekaligus |
| `SCRIPT_EXPRESSION_NOT_FEEL` | error | `zeebe:script expression` tanpa `=` |
| `CORRELATION_KEY_NOT_FEEL` | error | `correlationKey` tanpa `=` (`verify`: Zeebe menuntut ekspresi) |

Dihapus: `EXPRESSION_NOT_SUPPORTED`, `EXPRESSION_SYNTAX_ERROR` (→ `FEEL_SYNTAX_INVALID`),
`IO_MAPPING_NOT_SUPPORTED`, warning `FEEL_NOT_VALIDATED` (bila pernah ditambahkan).
`TIMER_DEFINITION_INVALID` tetap untuk nilai statis. Golden `testdata/invalid` Fase 1/2 yang
memakai kode lama diperbarui; satu file invalid per kode baru.

### 6.8 Proto `workflow/expression/v1/expression.proto`

```proto
syntax = "proto3";
package workflow.expression.v1;
option go_package = "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/expression/v1;expressionv1";

import "buf/validate/validate.proto";

service ExpressionService {
  // Mengevaluasi satu ekspresi FEEL terhadap variabel JSON. Tanpa efek samping; untuk modeler,
  // dukungan, dan harness uji. Dibatasi ENGINE_FEEL_EVAL_TIMEOUT (default 2 s) dan EvalLimits.
  rpc EvaluateExpression(EvaluateExpressionRequest) returns (EvaluateExpressionResponse) {}
}

message EvaluateExpressionRequest {
  string expression = 1 [(buf.validate.field).string = {min_len: 1, max_len: 65536}];  // dengan atau tanpa "="
  string variables = 2 [(buf.validate.field).string.max_len = 1048576];                // JSON object; kosong = {}
  bool unary_tests = 3;      // evaluasi sebagai unary tests terhadap `input`
  string input = 4;          // JSON, hanya bila unary_tests
}

message EvaluateExpressionResponse {
  string result = 1;         // JSON kanonik; kosong bila error
  string result_type = 2;    // "number", "date and time", ...
  string error_message = 3;  // kosong bila sukses; sintaks → InvalidArgument, evaluasi → di sini (bukan error RPC)
  repeated string warnings = 4;
}
```

Modul `expression` (`apps/engine/internal/modules/expression`) hanya `application` + `interfaces`
(tidak ada domain/infrastructure karena tanpa state; generator modul diberi opsi `--stateless`, atau
paket kosong dibiarkan dengan `doc.go` agar `archtest` tetap konsisten — pilih yang kedua).

## 7. Fase 7b — DMN

### 7.1 `libs/go/dmn` (kernel, proyek Nx `dmn-go`)

```
libs/go/dmn/
├─ parse.go            # Parse(r, opts) (*Definitions, error); namespace DMN 1.3 (20191111), 1.4 (20211108), 1.5 (20230324)
├─ definitions.go      # Definitions{ID, Name, Decisions, InputData, BKMs}
├─ decision.go         # Decision{ID, Name, ResultName, ResultType, Requirements, Logic}
├─ table.go            # DecisionTable{HitPolicy, Aggregation, Inputs, Outputs, Rules}; Input{Expression *feel.Expression, TypeRef}; Rule{InputEntries []*feel.UnaryTests, OutputEntries []*feel.Expression, Annotations}
├─ literal.go          # LiteralExpression{Expr *feel.Expression}
├─ evaluate.go         # Evaluate(d *Definitions, decisionID string, ctx feel.Context, opts) (Result, error)
├─ result.go           # Result{Output feel.Value, Evaluated []EvaluatedDecision{ID, Output, Inputs, MatchedRules}}
├─ validate.go         # Issues (kode stabil: DMN_DUPLICATE_ID, DECISION_TABLE_NO_INPUT, HIT_POLICY_INVALID, UNARY_TEST_INVALID, OUTPUT_ENTRY_INVALID, REQUIRED_DECISION_NOT_FOUND, BKM_UNSUPPORTED_KIND, ...)
├─ internal/xmlmodel/  # DTO encoding/xml, pola hibrid yang sama dengan bpmn
└─ testdata/{valid,invalid,tck,parity}
```

Evaluasi decision table (klausul 8 DMN; verifikasi detail):

1. Evaluasi semua `Input.Expression` di konteks (variabel + hasil required decisions sebagai entri
   bernama `ResultName`/nama decision).
2. Untuk tiap rule: semua sel `InputEntries[i].Evaluate(inputValue[i])` harus `true` (sel `-` selalu
   `true`).
3. Hit policy: `UNIQUE` (> 1 cocok → error evaluasi), `FIRST` (rule pertama), `PRIORITY` (output
   values list menentukan urutan; tanpa daftar → error), `ANY` (semua output identik, selain itu
   error), `COLLECT` tanpa agregasi (list), `COLLECT` + `SUM/MIN/MAX/COUNT` (satu output numerik),
   `RULE ORDER` (urutan rule), `OUTPUT ORDER` (urutan priority output values).
4. Bentuk hasil: satu output tanpa nama → nilai; satu output → nilai (bukan context) — `verify`
   dmn-scala: decision table dengan satu output menghasilkan nilai langsung, > 1 output → context
   `{name: value}`; `COLLECT` → list dari bentuk itu.
5. `typeRef` memengaruhi konversi hasil (`verify` apakah dmn-scala memaksa tipe).

### 7.2 Deploy `.dmn` dan penyimpanan (modul `deployment`)

- `DeployResource` menerima `.dmn`: `dmn.ParseAndValidate` → issue dengan kode DMN; atomik bersama
  BPMN dalam satu deployment.
- Versi per `decisionRequirementsId` (DRG) **dan** per `decisionId` (Zeebe memberi `decisionKey` dan
  versi per decision; `verify` apakah versi decision naik bersama DRG meski decision itu tidak
  berubah — diduga ya: setiap deploy DRG yang berubah menaikkan semua decision di dalamnya).
- Dedup checksum per resource seperti BPMN.

```sql
-- 000NN_decision.sql (nomor final saat merge)
CREATE TABLE decision_requirements (
    key                       BIGINT PRIMARY KEY,
    tenant_id                 TEXT NOT NULL DEFAULT '<default>',
    decision_requirements_id  TEXT NOT NULL,
    name                      TEXT,
    version                   INT NOT NULL CHECK (version > 0),
    resource_id               BIGINT NOT NULL REFERENCES deployment_resource (id),
    checksum                  BYTEA NOT NULL,
    deployment_key            BIGINT NOT NULL REFERENCES deployment (key),
    created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, decision_requirements_id, version)
);

CREATE TABLE decision_definition (
    key                        BIGINT PRIMARY KEY,
    tenant_id                  TEXT NOT NULL DEFAULT '<default>',
    decision_id                TEXT NOT NULL,
    name                       TEXT,
    version                    INT NOT NULL CHECK (version > 0),
    version_tag                TEXT,
    decision_requirements_key  BIGINT NOT NULL REFERENCES decision_requirements (key),
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, decision_id, version)
);
CREATE INDEX decision_definition_latest_idx ON decision_definition (tenant_id, decision_id, version DESC);
```

`deployment/api` menambah `Decisions interface { GetByKey(ctx, key) (DecisionDefinition,
*dmn.Definitions, error); GetLatest(ctx, decisionID string) (...) ; GetByVersionTag(...) }` dengan
cache LRU DRG ter-parse.

### 7.3 Business rule task (modul `execution`)

- `Activity.CalledDecision()` ada → perilaku: `ACTIVATING` (input mapping) → resolusi decision lewat
  port `DecisionResolver` (adapter ke `deployment/api.Decisions`; `bindingType` `latest` default,
  `deployment`, `versionTag` — verifikasi atribut `zeebe:calledDecision bindingType`/`versionTag`
  di 8.8) → `dmn.Evaluate` **di domain** (kernel) dengan rantai scope task → record
  `DECISION_EVALUATION EVALUATED` (payload: decision key/id/version, DRG key, output, evaluated
  decisions, matched rules) atau `FAILED` → `resultVariable` lokal → `COMPLETING` (6.2).
- Decision tidak ada → `CALLED_DECISION_ERROR` di `ACTIVATING`; gagal evaluasi →
  `DECISION_EVALUATION_ERROR` di `ACTIVATED`; record `FAILED` tetap ditulis (`verify` Zeebe menulis
  `DECISION_EVALUATION FAILED` plus incident).
- `libs/go/dmn` masuk `kernelAllowlist` `archtest` (perluasan ADR-0010, dicatat di ADR-0029).

### 7.4 Modul `decision` dan proto `workflow/decision/v1`

Modul `decision` (lima lapisan): `DecisionService.EvaluateDecision` standalone (memakai
`deployment/api.Decisions` + `dmn.Evaluate` di `application`), tabel `decision_instance`
(ditulis langsung untuk standalone; untuk jalur proses diisi oleh consumer outbox durable
`execution.decision_evaluated` dari Fase 3), query `GetDecisionInstance`, `ListDecisionInstances`.

```sql
CREATE TABLE decision_instance (
    key                        BIGINT PRIMARY KEY,
    tenant_id                  TEXT NOT NULL DEFAULT '<default>',
    decision_key               BIGINT NOT NULL,
    decision_id                TEXT NOT NULL,
    decision_version           INT NOT NULL,
    decision_requirements_key  BIGINT NOT NULL,
    process_instance_key       BIGINT,            -- NULL untuk standalone
    process_definition_key     BIGINT,
    element_instance_key       BIGINT,
    element_id                 TEXT,
    state                      TEXT NOT NULL CHECK (state IN ('EVALUATED', 'FAILED')),
    result                     JSONB,
    evaluated_decisions        JSONB NOT NULL,    -- [{decisionId, output, inputs:[{id,name,value}], matchedRules:[{ruleId, ruleIndex, outputs:[...]}]}]
    failure_message            TEXT,
    failed_decision_id         TEXT,
    evaluated_at               TIMESTAMPTZ NOT NULL
);
CREATE INDEX decision_instance_by_pi_idx ON decision_instance (process_instance_key, key);
CREATE INDEX decision_instance_list_idx ON decision_instance (tenant_id, decision_id, key DESC);
```

```proto
syntax = "proto3";
package workflow.decision.v1;
option go_package = "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/decision/v1;decisionv1";

import "google/protobuf/timestamp.proto";

service DecisionService {
  rpc EvaluateDecision(EvaluateDecisionRequest) returns (EvaluateDecisionResponse) {}
  rpc GetDecisionDefinition(GetDecisionDefinitionRequest) returns (GetDecisionDefinitionResponse) {}
  rpc ListDecisionDefinitions(ListDecisionDefinitionsRequest) returns (ListDecisionDefinitionsResponse) {}
  rpc GetDecisionInstance(GetDecisionInstanceRequest) returns (GetDecisionInstanceResponse) {}
  rpc ListDecisionInstances(ListDecisionInstancesRequest) returns (ListDecisionInstancesResponse) {}
}

message EvaluateDecisionRequest {
  oneof decision {
    int64 decision_key = 1;
    string decision_id = 2;   // versi terbaru
  }
  string variables = 3;       // JSON object
  string request_id = 4;      // idempotensi Fase 3 (opsional)
}

message EvaluateDecisionResponse {
  int64 decision_instance_key = 1;
  int64 decision_key = 2;
  string decision_id = 3;
  string decision_name = 4;
  int32 decision_version = 5;
  int64 decision_requirements_key = 6;
  string decision_requirements_id = 7;
  string decision_output = 8;                      // JSON; kosong bila gagal
  repeated EvaluatedDecision evaluated_decisions = 9;
  string failed_decision_id = 10;
  string failure_message = 11;
}

message EvaluatedDecision {
  int64 decision_key = 1;
  string decision_id = 2;
  string decision_name = 3;
  int32 decision_version = 4;
  string decision_type = 5;                        // "DECISION_TABLE", "LITERAL_EXPRESSION"
  string decision_output = 6;                      // JSON
  repeated EvaluatedInput evaluated_inputs = 7;
  repeated MatchedRule matched_rules = 8;
}

message EvaluatedInput { string input_id = 1; string input_name = 2; string input_value = 3; }
message MatchedRule { string rule_id = 1; int32 rule_index = 2; repeated EvaluatedOutput evaluated_outputs = 3; }
message EvaluatedOutput { string output_id = 1; string output_name = 2; string output_value = 3; }
```

Pesan definisi/instance dan request `Get`/`List` mengikuti pola `deployment/v1` dan `process/v1`
(keyset pagination, filter `decision_id`, `process_instance_key`, `state`). `DeployResourceResponse`
ditambah `repeated DecisionDefinition decisions = 4; repeated DecisionRequirements
decision_requirements = 5;` (aditif; `buf breaking` lolos). Bentuk field disesuaikan dengan
`EvaluateDecision` gRPC/REST v2 Camunda 8.8 (verifikasi nama persis) agar SDK terasa familiar.

## 8. Modeler-web dan Operate

### 8.1 Fase 7 (FEEL)

- `ValidationIssue` dari `ValidateResource`/`DeployResource` dengan kode `FEEL_*` membawa `element_id`
  dan pesan `attribute 'conditionExpression' column 7: unexpected token ')'`; fitur `deployment`
  (Fase 1) menandai elemen; properties panel (Fase 6, `bpmn-js-properties-panel` + Camunda
  Platform 8 provider) menampilkan error di field terkait lewat `errors` API panel (verifikasi API
  versi yang dipakai).
- Editor FEEL: properties panel Camunda sudah memakai `@bpmn-io/feel-editor` (CodeMirror 6 +
  `lezer-feel`); **verifikasi lisensi** (diduga MIT) sebelum dipakai; bila bukan MIT/kompatibel,
  pakai `<textarea>` biasa + highlight error dari server.
- Panel "Coba ekspresi" (fitur baru `expression-playground`, empat lapisan): input ekspresi,
  variabel JSON, tombol evaluasi → `ExpressionService.EvaluateExpression`; hasil + tipe; shortcut
  dari field FEEL di properties panel (ambil ekspresi yang sedang diedit). Tidak memakai `feelin`
  (interpreter JS) agar hasil selalu dari engine Go.
- Daftar `FreeNames()` dari server (lewat `EvaluateExpression` dengan flag `analyze`? — sederhana:
  tidak di DoD; cukup dicatat sebagai ide).

### 8.2 Fase 7b (DMN)

- modeler-web fitur `decision-modeler`: `dmn-js` (lisensi bpmn.io dengan watermark; verifikasi
  teks lisensi dan versi), `dmn-js-properties-panel`, `camunda-dmn-moddle`; `libs/web/dmn-viewer`
  sebagai pembungkus (pola `libs/web/bpmn-viewer`); deploy lewat `DeployResource` dengan `.dmn`;
  daftar decision definition.
- operate-web fitur `decision-instance`: daftar (TanStack Table), detail: viewer dmn-js dengan
  rule yang cocok disorot (overlay), input/output yang dievaluasi, tautan ke process instance.
- Graf DRG (React Flow) opsional — hanya bila murah.

## 9. Paritas terhadap Camunda 8.8

### 9.1 Harness `tools/scripts/parity-feel` (Go)

- Korpus `libs/go/feel/testdata/parity/*.json`: `{id, expr, vars, tags}`; `expectations.json`:
  hasil kanonik (`result` JSON atau `incident: {type, messagePrefix}`) + `deviation` beralasan.
- Untuk setiap kasus, harness **menghasilkan** BPMN `parity-feel.bpmn` satu proses: none start →
  script task (`zeebe:script expression="=<expr>" resultVariable="result"`) → end, dengan
  `bpmnProcessId` = `feel_<id>`; satu deployment untuk semua kasus (batas 100 proses per
  deployment, dipecah bila perlu).
- Engine kita: `CreateProcessInstanceWithResult` (Fase 4) → variabel akhir `result`; incident
  terdeteksi dari timeout + `ListIncidents`.
- Camunda 8.8 (profil `camunda`, mode `--live`): REST v2 `POST /v2/process-instances` dengan
  `awaitCompletion: true` (verifikasi nama field 8.8) → `variables.result`; incident: timeout +
  (bila secondary storage tersedia) `POST /v2/incidents/search`; tanpa secondary storage cukup
  "timeout = incident" dengan tipe tidak dibandingkan (dicatat sebagai keterbatasan).
- Pembandingan: JSON kanonik (angka dibandingkan sebagai desimal; `"1.0"` vs `"1"` dianggap sama
  **hanya** jika `tags` memuat `numeric-format`, agar perbedaan format tetap terlihat di kasus yang
  memang mengujinya).
- CI: mode offline terhadap `expectations.json`; mode live manual sebelum fase ditutup, hasil
  dilampirkan di PR penutup.

### 9.2 Daftar kasus yang wajib ada (minimum; setiap baris ≥ 1 kasus)

1. `=x` dengan `x` tidak ada → `null` tanpa incident; `=x.y` dengan `x = null`.
2. `=null = null`; `=x = null` (x tidak ada) → `true`; `=x != null`.
3. `=1 = "1"`, `=true = 1`, `=[1] = 1` → `false`/`null` (menentukan baris 3.2).
4. `=1 < "a"`, `=1 < null` → `null`; dipakai sebagai kondisi gateway → incident `CONDITION_ERROR`
   dan teks pesannya.
5. `=true and null`, `=false and null`, `=null or true`, `=null or false`, `=not(null)`.
6. `=if null then 1 else 2` → `2`; `=if "x" then 1 else 2`.
7. `=0.1 + 0.2` → `0.3`; `=0.1 + 0.2 = 0.3` → `true`; `=1.0` → format JSON (`1` atau `1.0`).
8. `=1/3`, `=2/3 * 3`, `=10 ** 20`, `=2 ** 0.5`, `=sqrt(2)`, `=decimal(2/3, 2)` (half-even vs
   half-up), `=round half up(2.5, 0)`.
9. `=10/0`, `=0/0`, `=modulo(10, 0)` → `null` dan apakah menjadi incident di script task.
10. `=9007199254740993 + 1`, `=12345678901234567890 * 10`, `=-9223372036854775808 - 1`.
11. `=string(1.0)`, `=string(1.10)`, `=string(1e21)`, `=number("1e3")`, `=number("1,000.5", ",", ".")`.
12. `="a" + 1`, `="a" + "b"`, `=1 + "1"`.
13. `=substring("hello", 2, 3)`, `=substring("hello", -2)`, `=string length("héllo")`,
    `=split("a,b,,c", ",")`, `=replace("abc", "(b)", "[$1]")`, `=matches("a", "A", "i")`.
14. `=date("2024-01-31") + duration("P1M")`, `=date("2024-02-29") + duration("P1Y")`,
    `=date("2024-03-01") - date("2024-02-01")`, `=date and time("2024-03-31T02:30:00@Europe/Berlin")`
    (celah DST), `=date and time("2024-01-01T00:00:00+07:00") = date and time("2023-12-31T17:00:00Z")`.
15. `=now()`, `=today()` → tipe dan bentuk JSON (zona); `=now() > date and time("2020-01-01T00:00:00Z")`.
16. `=duration("PT1H") > duration("PT30M")`, `=duration("P1M") = duration("P30D")`,
    `=duration("P1Y") + duration("P1M")`, `=duration("PT1H") / duration("PT15M")`.
17. `=[1,2,3] = [1,2,3]`, `=[1,[2]] = [1,[2]]`, `={a:1,b:2} = {b:2,a:1}`, `={a:[1]} = {a:[1.0]}`.
18. `=[1,2,3][-1]`, `=[1,2,3][0]`, `=[1,2,3][4]`, `=5[1]`, `=[][1]`, `=[1,2,3][item > 1]`,
    `=[{a:1},{a:2}][a = 2]`, `=[{a:1},{a:2}].a`.
19. `={a: 1, b: a + 1}.b` → `2`; `={"first name": "x"}` sebagai variabel lalu `` =`first name` ``.
20. `=1 in [1..3]`, `=4 in (1..4]`, `=3 in (<2, >5)`, `=x between 1 and 3` dengan `x = null`.
21. `=some x in [1,2] satisfies x > 1`, `=every x in [] satisfies false`, `=some x in null satisfies true`.
22. `=for x in 1..3 return x * 2`, `=for x in 3..1 return x`, `=for x in [1,2], y in [3] return x + y`,
    `=for i in 1..3 return if i = 1 then 1 else partial[-1] * i` (variabel `partial`).
23. `="abc" instance of string`, `=null instance of Any`, `=1 instance of number`, `=[1] instance of list`.
24. `="A\U01F600"`, `="a\nb"` → nilai dan bentuk JSON.
25. `=x` dengan variabel bernama `date`, `time`, `string` (tabrakan nama fungsi).
26. `=is defined(x)`, `=is defined(x.y)`, `=get or else(x, 1)`, `=assert(x, x != null)` → incident.
27. `=count([])`, `=sum([])`, `=min([])`, `=mean([])`, `=max(1, "a")`, `=sum([1, null])`.
28. `=string join(["a", null, "b"], ",")`, `=context put({}, "a", 1)`, `=context merge({a:1}, {a:2})`,
    `=get value({a:1}, "b")`, `=get entries({a:1})`.
29. `=sort([3,1,2], function(a, b) a < b)`, `=index of([1,2,1], 1)`, `=distinct values([1,1.0])`.
30. Komentar: `=1 + /* dua */ 2 // tiga`.
31. I/O mapping: input `=a` → `b.c` (target path) lalu output `=b` → `d`; job variables bocor atau
    tidak saat ada output mapping; `=x` di output mapping menunjuk variabel lokal input mapping.
32. Propagasi: variabel `v` sudah ada di scope sub-process dan root; output mapping sub-process
    menulis `v` → scope mana yang berubah.
33. Hasil berupa range/function disimpan ke variabel → incident dan pesannya.
34. Ekspresi dengan `1e3`, `.5`, `1.`, `+1` (unary plus) → terima/tolak saat deploy (sintaks).

## 10. Rencana kerja Fase 7 (urutan eksekusi)

Setiap langkah punya **hasil yang bisa diverifikasi**. 10.2–10.7 (kernel) tidak bergantung pada
engine dan bisa dikerjakan sebelum/selama Fase 5–6 (lihat 13.2 #1); 10.8–10.11 butuh Fase 5
(sub-process, multi-instance) di `main` untuk skenario lengkap, tetapi scope lokal dan mapping
pada task bisa dikerjakan dengan Fase 2–4 saja.

### 10.1 Prasyarat dan kerangka

- Branch `feat/phase-7-feel` dari `main`; `make check` hijau.
- `libs/go/feel/project.json` ditambah target `bench`, `conformance`; `deps_test.go` mencakup
  subpaket; `cmd/engine` meng-import `time/tzdata`; konfigurasi `ENGINE_FEEL_TIMEZONE`,
  `ENGINE_FEEL_MAX_STEPS`, `ENGINE_FEEL_EVAL_TIMEOUT`.
- Draf ADR-0027 (keputusan 3.1 dan 3.2 awal).

Hasil: `pnpm nx run-many -t build test lint -p feel-go` lolos dengan subset Fase 2 masih utuh.

### 10.2 Desimal dan model nilai

- `internal/decimal`: parsing, pencetakan kanonik, `+ - * /`, `Cmp`, `Pow` bulat, `Sqrt`/`Log`/`Exp`
  lewat `big.Float` 128-bit + pembulatan ke 34 digit, `Round*` sesuai DMN 1.4, fast path `int64`.
- `value.go`, `json.go`: semua kind, `FromJSON`/`ToJSON` kanonik, kesetaraan dalam (deep equal).
- `FuzzDecimal`.

Hasil: tabel uji desimal ≥ 150 kasus (termasuk perbandingan terhadap nilai yang dihitung
`java.math.BigDecimal` dan dicatat sebagai fixture); round-trip JSON untuk korpus variabel Fase 2.

### 10.3 Lexer dan parser

- `internal/lexer`, `internal/parser` (Pratt, presedensi 4.1), `internal/ast` dengan printer
  (untuk round-trip di fuzz) dan `FreeNames`.
- `Compile` mengganti parser subset; `CompileUnaryTests`.
- `FuzzParse`; golden `testdata/golden/syntax/*.json` (≥ 120 kasus: presedensi, asosiativitas, nama
  multi-kata bawaan, komentar, escape, literal temporal, error dengan posisi).

Hasil: subset Fase 2 lolos lewat parser baru; `nx run feel-go:fuzz` 10 menit bersih untuk
`FuzzParse`.

### 10.4 Evaluator inti

- Operator aritmetika/perbandingan/logika tiga nilai, `if`, `for` (list dan rentang, multi-iterator,
  `partial`), `some`/`every`, filter, path (termasuk proyeksi dan properti temporal), context literal
  dengan referensi antar-entri, list, range, `instance of`, definisi dan pemanggilan fungsi
  (positional/named), `in`/`between`, batas `EvalLimits`, `EvalError` dengan konteks ekspresi.
- Golden `testdata/golden/eval/*.json` ≥ 250 kasus.

Hasil: semua golden lolos; `FuzzEvaluate` 10 menit bersih; benchmark `condition` di bawah target 5.5.

### 10.5 Temporal

- `internal/temporal`: parsing/pencetakan ISO 8601 (date, time dengan offset/zona, date and time,
  kedua durasi), aritmetika dengan clamp hari akhir bulan, perbandingan, properti; zona IANA.
- Fungsi konversi temporal, `now`/`today` lewat opsi.

Hasil: golden temporal ≥ 120 kasus (termasuk DST, zona, tahun kabisat, durasi negatif); test
berjalan dengan `TZ` acak untuk memastikan tidak bergantung zona proses.

### 10.6 Fungsi bawaan

- `internal/builtin` per kategori sesuai 4.4 (W); registri nama → implementasi + nama parameter;
  pemetaan regex ke RE2 dengan pesan deviasi.
- Golden per fungsi ≥ 3 kasus (≈ 300 kasus).

Hasil: matriks 4.4 kolom W seluruhnya `✅` di README `libs/go/feel`; `FEEL_UNKNOWN_FUNCTION`
memakai registri yang sama.

### 10.7 Konformansi, CLI, dokumentasi kernel

- `tools/scripts/tck-extract` + `testdata/conformance/tck` + `skip-list.json`; porting manual kasus
  feel-scala yang menggambarkan perilaku Zeebe; `nx run feel-go:conformance`.
- `cmd/feel` REPL; README: tabel semantik `null`, deviasi dari spesifikasi dan dari feel-scala,
  matriks fungsi.

Hasil: laporan konformansi (lolos/skip/gagal dengan alasan) tersimpan di `docs/reports/feel-conformance.md`
(dihasilkan target, di-commit saat fase ditutup).

### 10.8 Engine: scope lokal dan propagasi (ADR-0028)

- `VariableLookup` rantai scope; `Apply` untuk `VARIABLE` dengan `scope_key` lokal dan penghapusan
  saat scope final; proyektor + kueri sqlc baru; `SetVariables local=true`.
- `GetProcessInstance` mengembalikan `scope_key` per variabel (proto aditif bila belum ada).

Hasil: skenario tabel: variabel lokal terlihat dari dalam, tidak dari luar; hilang setelah scope
selesai; propagasi ke scope yang memiliki nama; `SetVariables local` di task aktif; test
determinisme/replay Fase 3 tetap lolos.

### 10.9 Engine: I/O mapping dan script task

- `behavior_mapping.go` (input/output, 6.2) dipanggil dari aktivasi/penyelesaian semua activity;
  job variables lokal saat ada output mapping; `behavior_task.go` script FEEL (6.3); incident
  `IO_MAPPING_ERROR`; resolusi incident mengulang mapping.
- `ProfileExecutable`: hapus penolakan I/O mapping; buka script task FEEL.
- Evaluator adapter dengan cache dan metrik (5.2); port diperluas.

Hasil: skenario tabel per aturan 6.2 (≥ 25) dan urutan record 6.5; benchmark engine naik < 10%.

### 10.10 Engine: ekspresi atribut lain dan regresi Fase 5/6

- Ganti pemanggilan subset di timer, multi-instance, message, call activity, user task, task
  definition dengan port baru; pemeriksaan tipe hasil per 6.4; skenario regresi untuk tiap baris
  (ekspresi non-trivial: `=duration("PT" + string(n) + "M")`, `=orders[status = "NEW"]`,
  `=candidate groups` list, `=now() + duration("P2D")`).

Hasil: skenario Fase 2/4/5/6 bertanda `parity` tetap lolos; skenario baru lolos.

### 10.11 Validasi deploy dan `ExpressionService`

- `feel/bpmncheck` (6.7) + golden invalid baru; `deployment/application` memakai `bpmncheck`,
  menghapus langkah Fase 2; `ValidateResource` ikut.
- Proto `expression/v1`, modul `expression`, wiring, test handler.

Hasil: korpus `testdata/valid` Fase 1 yang memakai mapping lolos deploy; file invalid per kode baru;
`EvaluateExpression` diuji lewat `httptest` termasuk batas waktu.

### 10.12 Modeler-web

- Tampilkan issue `FEEL_*` di field properties panel; fitur `expression-playground`; verifikasi
  lisensi `@bpmn-io/feel-editor` dan pasang bila sesuai.

Hasil: uji manual tercatat (screenshot) di PR; unit test fitur (adapter Connect, store).

### 10.13 Paritas

- Harness 9.1 + korpus 9.2 (≥ 150 kasus); mode live terhadap Camunda 8.8; perbaikan deviasi atau
  pencatatan di `expectations.json`; kunci baris `verify` 3.2 di ADR-0027.

Hasil: `make parity SUITE=feel` lolos offline; laporan live di PR penutup.

### 10.14 Dokumentasi dan penutupan

- Final ADR-0027, ADR-0028; README `libs/go/feel`, README `execution` (scope, mapping, incident
  baru); `docs/plan/README.md` menautkan dokumen ini; review DoD.

Hasil: DoD 7a tercentang.

### 10.15–10.22 Fase 7b (ringkas)

| Langkah | Isi | Hasil |
|---|---|---|
| 10.15 | `libs/go/dmn` parse + model + validasi; korpus `.dmn` dari Camunda Modeler (≥ 15) + golden | `dmn-go` build/test/lint/fuzz lolos |
| 10.16 | Evaluasi decision table semua hit policy + literal expression + requirements + BKM; runner TCK level 2/3 | laporan konformansi DMN |
| 10.17 | `deployment`: `.dmn`, tabel, versi, cache, `api.Decisions`; proto `deployment/v1` aditif | deploy `.dmn` lewat API dan modeler |
| 10.18 | `execution`: business rule task, record `DECISION_EVALUATION`, incident, `kernelAllowlist` + `dmn` | skenario tabel ≥ 10 |
| 10.19 | Modul `decision`: `EvaluateDecision`, `decision_instance`, consumer outbox, query | integration test |
| 10.20 | modeler-web `decision-modeler`, operate-web `decision-instance` | uji manual tercatat |
| 10.21 | Paritas decision (≥ 20 tabel) | `make parity SUITE=dmn` |
| 10.22 | ADR-0029, README, DoD 7b | DoD 7b tercentang |

## 11. Target Nx dan Makefile

| Proyek | Target | Perintah |
|---|---|---|
| `feel-go` | `build`, `test`, `lint` | seperti Fase 2 (`go test -race ./libs/go/feel/...`) |
| `feel-go` | `fuzz` (cache off) | `go test -run=^$ -fuzz=$FUZZ -fuzztime=$FUZZTIME ./libs/go/feel/...` dengan `FUZZ` ∈ {`FuzzParse`, `FuzzEvaluate`, `FuzzDecimal`, `FuzzTemporal`}; target menjalankan keempatnya berurutan bila `FUZZ` kosong (masing-masing `FUZZTIME`, default 60s lokal, 10m terjadwal) |
| `feel-go` | `bench` (cache off) | `go test -run=^$ -bench=. -benchmem ./libs/go/feel/...` |
| `feel-go` | `conformance` | `go test -run=TestConformance -tags=conformance ./libs/go/feel` + tulis `docs/reports/feel-conformance.md` |
| `feel-go` | `repl` (cache off, continuous) | `go run ./libs/go/feel/cmd/feel` |
| `dmn-go` (7b) | `build`, `test`, `lint`, `fuzz`, `conformance` | pola yang sama |
| `engine` | `test`, `bench`, `sqlc` | mencakup `expression`, (7b) `decision`; `implicitDependencies` += `feel-go`, (7b) `dmn-go` |
| `proto` | `generate`, `lint`, `breaking` | mencakup `expression/v1`, (7b) `decision/v1` |
| `modeler-web`, `operate-web` | `test`, `build` | fitur baru |

Makefile: `make fuzz TARGET=feel [FUZZ=FuzzEvaluate] [FUZZTIME=10m]`, `make conformance
[TARGET=feel|dmn]`, `make parity SUITE=feel|dmn|all [LIVE=1]`, `make feel-repl`. CI: `conformance`
masuk `check` (offline, cepat); fuzz 10 menit di jadwal mingguan yang sudah ada.

## 12. Risiko dan mitigasi

| Risiko | Dampak | Mitigasi |
|---|---|---|
| Semantik berbeda dari feel-scala di kasus tepi (null, tipe campuran, format angka) | Proses yang dipindah dari Camunda berperilaku beda | Tabel 3.2 dengan `verify`; korpus paritas 9.2 sejak awal; deviasi eksplisit di ADR-0027 dan README; `EvaluateExpression` memudahkan pengguna memeriksa |
| Cakupan fungsi bawaan tidak lengkap saat pengguna memakainya | Deploy gagal `FEEL_UNKNOWN_FUNCTION` hanya warning → incident saat runtime | Registri fungsi tunggal; warning saat deploy menyebut nama; daftar N kecil dan terdokumentasi; menambah fungsi = satu file + golden |
| Regex RE2 vs Java | `matches`/`replace` dengan lookahead/backreference gagal | Deteksi saat compile pola → `EvalError` jelas; dicatat sebagai deviasi; pola umum Modeler (tanpa lookaround) tidak terpengaruh |
| Presisi desimal/pembulatan tidak identik dengan `BigDecimal` | Hasil `1/3`, `sqrt` beda di digit terakhir | Fixture dihitung dari BigDecimal (nilai dicatat manual dari JVM saat verifikasi); toleransi dijelaskan; kebanyakan proses memakai `decimal(x, 2)` |
| Zona waktu: `now()` di Zeebe memakai zona JVM, kita UTC | `today()` berbeda di sekitar tengah malam | `ENGINE_FEEL_TIMEZONE` dapat diatur; dokumentasi; paritas membandingkan setelah normalisasi |
| Image tanpa zoneinfo | `date and time("…@Asia/Jakarta")` gagal di produksi tapi sukses di dev | `time/tzdata` di-embed; test integrasi menjalankan binary hasil build dengan `ZONEINFO=` kosong |
| Hot path melambat karena konversi JSON → FEEL seluruh variabel | Latensi command naik | Lookup lazy per nama + cache per command; benchmark engine di DoD (< 10%) |
| Perubahan semantik untuk instance berjalan | Instance lama berperilaku beda setelah upgrade | Subset Fase 2 adalah subset semantik; tabel uji Fase 2 dipertahankan tanpa perubahan ekspektasi sebagai bukti |
| `bpmncheck` menolak model yang sebelumnya lolos (ekspresi `=` rusak yang tidak pernah dievaluasi) | Deploy ulang gagal setelah upgrade | Dicatat di catatan rilis; `ValidateResource` memberi daftar lengkap sebelum deploy; tidak ada mode lunak (menerima ekspresi rusak lebih berbahaya) |
| Nama variabel dengan spasi tanpa backtick | Pengguna Camunda yang memakai `first name` di ekspresi mendapat `FEEL_SYNTAX_INVALID` | Pesan error menyarankan backtick; keputusan lanjutan di 13.2 #4 |
| Ketidakpastian aturan mapping (urutan evaluasi input, target path atas variabel induk, default `propagateAllChildVariables`) | Hasil variabel berbeda | Setiap aturan `verify` punya kasus paritas 9.2 #31–32; algoritma dibuat parametrik agar hasil verifikasi hanya mengubah satu flag |
| DMN TCK lisensi/format tidak cocok | Korpus konformansi kosong | Golden sendiri ≥ 600 kasus tetap DoD; TCK hanya tambahan dengan skip-list |
| Fase 7b memakan waktu produksi (Fase 8) | Deploy produksi tertunda | 7b terpisah dan bisa dijadwalkan setelah Fase 8 (13.2 #2) |
| `random number()`/`uuid()`/`now()` merusak determinisme replay | Replay Fase 3 menghasilkan state berbeda | Nilai dievaluasi sekali di `Processor` dan hasilnya tersimpan di record (variabel); replay memakai record, bukan evaluasi ulang; sumber acak di-inject |

## 13. Pertanyaan terbuka

Setiap pertanyaan diberi rekomendasi. Keputusan final dicatat di 13.1 setelah review.

### 13.1 Keputusan

Belum ada. Diisi dengan tabel yang sama seperti 9.1 dokumen Fase 1.

### 13.2 Pertanyaan dan rekomendasi

| # | Pertanyaan | Rekomendasi | Alasan |
|---|---|---|---|
| 1 | Urutan: kernel FEEL (10.2–10.7) dikerjakan lebih awal, paralel dengan Fase 5–6? | **Ya.** Kernel tidak bergantung pada engine; Fase 6 (assignment, dueDate) langsung memakai FEEL penuh; Fase 5 (timer/multi-instance) tidak terhambat subset | Mengurangi pekerjaan "sementara" di Fase 5/6 dan skenario regresi Fase 7 |
| 2 | DMN di Fase 7, 7b sebelum Fase 8, atau setelah Fase 8? | **7b setelah DoD 7a; urutan terhadap Fase 8 diputuskan saat 7a selesai** (default: 7b sebelum 8 bila ada 2 engineer, setelah 8 bila 1) | 3.5 |
| 3 | Representasi angka: desimal kustom vs tetap `big.Rat` + pembulatan saat cetak | **Desimal kustom** (3.1) | `big.Rat` tidak punya fungsi transendental dan pembulatan 34 digit; hasil pembagian harus desimal seperti feel-scala |
| 4 | Nama variabel multi-kata tanpa backtick (`first name`) | **Nanti**: Fase 7 backtick saja + pesan error yang menyarankannya; evaluasi parser dengan lookahead ke daftar nama konteks (gaya feel-scala) setelah paritas menunjukkan kebutuhan nyata | Parser bergantung konteks memperumit `Compile` tanpa konteks (deploy time) |
| 5 | `source` I/O mapping tanpa `=` | **Tolak saat deploy** sampai verifikasi; ikuti Zeebe bila ternyata menerima literal | Bagian 3 |
| 6 | Kondisi bernilai `null` (lanjutan Fase 2 #13) | **Incident** kecuali paritas membuktikan Zeebe memperlakukannya `false` | Konsisten dengan Fase 2 |
| 7 | Regex: RE2 (stdlib) atau menulis engine regex kompatibel Java? | **RE2** dengan deteksi fitur tak didukung; deviasi terdokumentasi | Engine regex sendiri di luar proporsi; kernel harus stdlib |
| 8 | `now()` zona default: UTC atau zona sistem? | **UTC**, dapat diatur `ENGINE_FEEL_TIMEZONE` | Deterministik di container; Zeebe mengikuti JVM yang biasanya UTC di container juga |
| 9 | Hasil desimal non-bulat ke variabel: desimal penuh atau tiru pembulatan double Zeebe? | **Desimal penuh**; catat deviasi bila terbukti | Tidak membuang presisi; klien yang membaca sebagai double mendapat nilai yang sama |
| 10 | Perlukah `ExpressionService` di API publik? | **Ya**, tanpa state, dibatasi ukuran/waktu; Fase 8 memberi izin terpisah | Dipakai modeler dan harness; biaya kecil |
| 11 | Scope untuk gateway/event: apakah intermediate catch event dengan output mapping butuh scope sendiri? | **Tidak**; output mapping event ditulis langsung ke scope induk dengan aturan propagasi; `verify` | Sama dengan Zeebe (event bukan scope) sejauh diketahui |
| 12 | Variabel lokal setelah scope selesai: hapus dari proyeksi atau simpan untuk Operate? | **Hapus dari `variable`** (runtime); riwayat ada di `execution_record`; Operate (Fase 6) membaca riwayat variabel dari record bila perlu | Tabel runtime tetap ramping; sama dengan Zeebe (Operate menampilkan dari exporter) |
| 13 | Korpus konformansi: DMN TCK atau golden sendiri saja? | **Keduanya**: golden sendiri sebagai DoD, TCK sebagai tambahan dengan skip-list setelah lisensi diverifikasi | 5.6 |
| 14 | Editor FEEL: `@bpmn-io/feel-editor` atau textarea? | **feel-editor** bila MIT terverifikasi; fallback textarea | 8.1 |
| 15 | Evaluasi decision (7b) di `execution/domain` (kernel) atau lewat `decision/api`? | **Di domain** lewat kernel `libs/go/dmn`; modul `decision` untuk standalone dan proyeksi instance | Atomisitas record tanpa transaksi lintas modul (prinsip Fase 2) |
| 16 | `instance of` dengan tipe generik dan fungsi `is` (DMN 1.5) | **Nanti**; warning `FEEL_UNKNOWN_FUNCTION`/`FEEL_SYNTAX_INVALID` dengan pesan jelas | Jarang dipakai di proses; feel-scala juga terbatas (`verify`) |

## 14. Estimasi dan pembagian kerja

**Fase 7 (7a): 23–27 hari kerja** untuk 1 engineer, atau 14–16 hari untuk 2 engineer paralel:

| Hari | Fokus |
|---|---|
| 1 | 10.1 prasyarat, kerangka, draf ADR-0027 |
| 2–3 | 10.2 desimal + model nilai + JSON |
| 4–6 | 10.3 lexer, parser, AST, fuzz parse |
| 7–9 | 10.4 evaluator inti |
| 10–11 | 10.5 temporal |
| 12–14 | 10.6 fungsi bawaan |
| 15 | 10.7 konformansi, CLI, README kernel |
| 16–17 | 10.8 scope lokal + propagasi (ADR-0028) |
| 18–19 | 10.9 I/O mapping + script task |
| 20 | 10.10 ekspresi atribut lain + regresi |
| 21 | 10.11 `bpmncheck` + `ExpressionService` |
| 22 | 10.12 modeler-web |
| 23–25 | 10.13 paritas live + perbaikan |
| 26–27 | 10.14 dokumentasi, ADR final, review DoD |

Pembagian jika 2 orang: **A** = kernel (10.2–10.7) lalu paritas; **B** = engine (10.8–10.11, mulai
dengan evaluator palsu yang membungkus subset Fase 2) lalu modeler-web. Titik temu: 10.9 (adapter
evaluator memakai kernel A).

**Fase 7b (DMN): 13–16 hari kerja** untuk 1 engineer (10.15–10.22: kernel 5–6 hari, deployment +
execution + modul `decision` 4–5 hari, UI 2–3 hari, paritas + dokumentasi 2 hari).

## 15. Keluaran Fase 7

Setelah fase ini selesai, Fase 8 dan seterusnya mendapat:

- `libs/go/feel` penuh dengan API stabil (`Compile`, `Evaluate`, `EvaluateWith`, unary tests,
  `Value`/JSON kanonik), batas evaluasi, dan korpus golden/konformansi/paritas yang dijalankan
  ulang setiap fase; dipakai juga oleh `libs/go/dmn`, CLI, dan tooling tanpa engine.
- Scope variabel lokal dan I/O mapping dengan aturan propagasi yang terdokumentasi (ADR-0028);
  Fase 8 (multi-tenant) tidak menyentuh model variabel.
- `bpmncheck` sebagai satu tempat validasi ekspresi untuk deploy, `ValidateResource`, dan lint
  modeler; `ProfileExecutable` tanpa penolakan ekspresi/mapping.
- `ExpressionService` untuk modeler, dukungan operasional, dan harness; Fase 8 menambah otorisasi
  (izin `expression:evaluate`) dan batas laju.
- Metrik `feel_*` (cache, durasi evaluasi) siap diekspor OpenTelemetry di Fase 8.
- (7b) `libs/go/dmn`, modul `decision`, `DecisionService`, `decision_instance`, DMN modeler, dan
  korpus paritas decision; Fase 8 menambah tenant pada `decision_*` dan izin `decision:evaluate`.
- Daftar deviasi dari feel-scala/spesifikasi (ADR-0027) sebagai acuan dukungan dan catatan rilis.
