# Fase 1 — Parser dan Model BPMN

**Proyek:** Workflow Engine (kompatibel konsep Camunda 8 / BPMN 2.0)
**Stack:** Go (engine utama) + React (UI) dalam monorepo Nx
**Status:** Draft rencana — belum dieksekusi
**Tanggal:** 2026-10-28

---

## 0. Status eksekusi

Belum ada langkah yang dieksekusi. Bagian ini diisi dengan tabel yang sama seperti Fase 0
(langkah, status, bukti) begitu pekerjaan dimulai.

**Kondisi repo yang menjadi titik awal (dicek 2026-10-28):**

| Area | Kondisi | Dampak ke Fase 1 |
|---|---|---|
| `apps/engine` | Platform (`config`, `logger`, `db` + goose, `httpserver`, `eventbus`) dan modul `health` lima lapisan sudah jalan | Modul `deployment` mengikuti pola `health` |
| `apps/engine/internal/archtest` | Test `go list` menegakkan aturan lapisan: `domain` hanya boleh stdlib, antarmodul hanya lewat `api` | `deployment/domain` **tidak boleh** meng-import `libs/go/bpmn`; parsing dilakukan di `application` (lihat 3.3) |
| Migrasi | Satu urutan goose di `apps/engine/internal/platform/db/migrations` (`00001_init.sql`), di-embed ke binary | Migrasi Fase 1 menjadi `00002_...sql` di folder yang sama |
| sqlc | Belum ada `sqlc.yaml` | Fase 1 membuat konfigurasi sqlc pertama |
| `libs/proto` | `buf` v2, `workflow/health/v1/health.proto`, generate ke `libs/go/api/gen` dan `libs/web/api-client/src/gen` | Proto baru di `workflow/deployment/v1` |
| `libs/go/bpmn` | Belum dibuat | Dibuat di Fase 1 |
| `apps/modeler-web` | Selesai di Fase 0: fitur `health` empat lapisan, shadcn/ui, TanStack Router, kanvas bpmn-js (`libs/web/bpmn-viewer`) dan React Flow (`libs/web/flow-canvas`) | Fitur `deployment` di 6.13 mengikuti pola fitur `health` |
| Makefile, CI, lint hook | Selesai di Fase 0: Makefile 23 target, GitHub Actions (check, integration, docker), husky + commitlint | Target baru (`fuzz`, `parity`) ditambahkan ke Makefile yang sudah ada |

## 1. Lingkup

Fase 1 membangun **fondasi definisi proses**: engine bisa menerima file BPMN 2.0 dari Camunda
Modeler, mengubahnya menjadi model in-memory yang immutable dan cepat ditelusuri, memvalidasinya
dengan pesan error per elemen (setara penolakan deployment di Camunda 8), lalu menyimpannya sebagai
process definition berversi di PostgreSQL. Fase 2 (modul `execution`) tinggal memakai model ini untuk
menjalankan token tanpa menyentuh XML lagi.

Termasuk dalam Fase 1:

1. **`libs/go/bpmn`** — kernel murni: parse XML → model, indeks graf, ekstensi `zeebe:`, validator
   dengan kode error stabil, profil dukungan elemen.
2. **Modul engine `deployment`** — lima lapisan: deploy resource, versi per `bpmnProcessId`, dedup
   checksum, simpan XML di PostgreSQL (goose + sqlc), publikasi event domain, cache model ter-parse
   untuk Fase 2.
3. **Kontrak API** `workflow/deployment/v1`: `DeployResource`, `GetProcessDefinition`,
   `ListProcessDefinitions` (Connect, Go + TS).
4. **Modeler-web fitur `deployment`** — tombol deploy dari kanvas bpmn-js, daftar definition
   (TanStack Query/Table), error validasi tampil di elemen diagram.
5. **Strategi uji** — golden file dari Camunda Modeler, fuzz parser, uji paritas terhadap Camunda 8
   referensi.

**Tidak termasuk** (fase lain): eksekusi token (Fase 2), evaluasi FEEL termasuk validasi sintaks
ekspresi (Fase 7), DMN dan form sebagai resource deploy (Fase 7 / Fase 6), multi-tenant dan otorisasi
(Fase 8), penghapusan resource (`DeleteResource`), migrasi instance antar versi.

## 2. Definition of Done Fase 1

- [ ] `libs/go/bpmn` adalah proyek Nx (`bpmn-go`) dengan target `build`, `test`, `lint`, `fuzz`, `bench`; hanya meng-import stdlib (ditegakkan test).
- [ ] Semua file di `libs/go/bpmn/testdata/valid` ter-parse tanpa issue error dan cocok dengan golden JSON-nya; semua file di `testdata/invalid` menghasilkan kode issue yang sama persis dengan golden-nya.
- [ ] Korpus berisi minimal **30 file** BPMN hasil ekspor Camunda Modeler (desktop atau web, execution platform Camunda 8) yang mencakup setiap baris matriks cakupan di 4.1.
- [ ] `go test -fuzz=FuzzParse` berjalan 10 menit tanpa panic, hang, atau alokasi tak terbatas; korpus crash (jika ada) masuk `testdata/fuzz` sebagai regresi.
- [ ] Benchmark: parse + validasi proses 500 flow node < 10 ms dan < 5 MB alokasi di mesin dev (angka awal, dikunci setelah pengukuran pertama).
- [ ] Modul `deployment` lima lapisan lolos `archtest`; use case punya unit test dengan repository fake; repository punya integration test (`-tags=integration`) terhadap Postgres 18.
- [ ] `DeployResource` dengan file valid mengembalikan `deployment_key` dan metadata tiap proses (`bpmn_process_id`, `version`, `process_definition_key`); deploy ulang file identik **tidak** menaikkan versi; file berubah menaikkan versi tepat satu.
- [ ] Deploy file tidak valid ditolak dengan Connect `InvalidArgument` + detail `DeploymentRejection` berisi daftar issue (kode, element id, posisi baris/kolom).
- [ ] Deploy paralel 20 request untuk `bpmnProcessId` yang sama menghasilkan versi berurutan tanpa duplikat (test integrasi).
- [ ] Event `deployment.process_deployed` terpublikasi di event bus untuk setiap versi baru (dibuktikan test dengan subscriber).
- [ ] Modeler-web: deploy dari kanvas, lihat daftar definition, klik issue memilih elemen bermasalah dan elemen diberi marker merah.
- [ ] Uji paritas: setiap file korpus memberi hasil terima/tolak yang sama dengan Camunda 8 referensi (profil `full`), perbedaan yang disengaja tercatat di `testdata/parity/expectations.json` dengan alasan.
- [ ] ADR-0008 (parser BPMN) dan ADR-0009 (versioning dan dedup deployment) tersimpan di `docs/adr/`.

## 3. Keputusan teknis

| Area | Keputusan | Alasan singkat |
|---|---|---|
| Lokasi parser | `libs/go/bpmn`, package `bpmn`, **hanya stdlib** | Kernel murni sesuai aturan 4.1 Fase 0 butir 6; dipakai engine, SDK worker, CLI lint, dan tooling tanpa menarik DB/transport |
| Library XML | **`encoding/xml` stdlib** dengan pola hibrid: `Decoder.Token()` untuk menelusuri container (`definitions`, `process`, `subProcess`) + `Decoder.DecodeElement()` ke struct DTO per elemen | Tanpa dependency dan tanpa CGO (image distroless `CGO_ENABLED=0`); tidak memproses DTD/entity eksternal sehingga aman dari XXE; namespace dicocokkan lewat URI (prefix `bpmn:` atau `bpmn2:` sama-sama diterima); `Decoder.InputPos()` memberi baris/kolom untuk setiap elemen. Alternatif `beevik/etree` (DOM, butuh dependency, posisi tidak tersedia) dan libxml2 (CGO) ditolak |
| Dua tahap | Tahap 1 XML → DTO (`internal/xmlmodel`, mutable, mirror skema); tahap 2 DTO → model domain immutable (`builder`) | Tag XML dan quirk skema tidak bocor ke model; model bisa dibentuk ulang tanpa mengubah decoding |
| Immutability | Field model tidak diekspor, akses lewat getter; model hanya dibentuk oleh `Parse`; slice dari getter berbagi backing array dan dinyatakan read-only di godoc | Aman dibaca konkuren oleh banyak instance di Fase 2 tanpa lock; menghindari salinan di hot path. Diuji dengan test `-race` baca paralel |
| Identitas elemen | Pointer langsung antar node (`SequenceFlow.Source() FlowNode`) + indeks `map[string]FlowElement` per `Process` | Traversal token O(1) tanpa lookup map di setiap langkah |
| Ekspresi | Disimpan sebagai `Expression{Raw string; IsFEEL bool}`; `IsFEEL` jika diawali `=` (konvensi Zeebe); Fase 1 hanya cek tidak kosong dan bentuk statis (mis. ISO 8601 untuk timer non-FEEL) | FEEL baru ada di Fase 7; model sudah siap menerima ekspresi terkompilasi tanpa mengubah API |
| Kesalahan | `Parse` hanya mengembalikan `error` untuk kegagalan fatal (XML rusak, bukan BPMN, melebihi batas); masalah semantik menjadi `Issue` dari `Validate` | Pengguna mendapat **semua** masalah sekaligus, bukan berhenti di error pertama |
| Kode issue | String konstan `SCREAMING_SNAKE` (mis. `SERVICE_TASK_MISSING_TASK_DEFINITION`), severity `error`/`warning`, plus element id dan posisi | Stabil untuk UI dan test; bebas dari perubahan teks pesan |
| Profil dukungan | `bpmn.Profile`: `ProfileFull` (semua elemen yang dimodelkan parser) dan `ProfileExecutable` (hanya elemen yang sudah bisa dieksekusi engine; tumbuh tiap fase) | Engine tidak menerima proses yang tidak bisa dijalankan, sementara uji paritas dan lint modeler memakai `ProfileFull` |
| Banyak proses | Satu `definitions` boleh berisi banyak `process` (collaboration/pool); setiap proses `isExecutable="true"` menjadi process definition sendiri; proses non-executable diabaikan dengan warning; minimal satu executable | Selaras perilaku Zeebe (verifikasi terhadap dokumentasi Camunda 8 untuk detail participant/pool) |
| BPMNDI | **Tidak dimodelkan**; elemen namespace DI dilewati saat decoding. XML asli disimpan byte-per-byte di DB sehingga diagram utuh untuk viewer | Engine tidak butuh koordinat; menyimpan XML asli menghindari serialisasi ulang yang bisa merusak kompatibilitas Modeler |
| Ekstensi tak dikenal | Elemen/atribut di namespace selain BPMN dan `zeebe:` diabaikan tanpa issue (mis. `modeler:`, `camunda:` C7 diberi warning) | Camunda Modeler menambah atribut `modeler:executionPlatform`; file C7 perlu peringatan, bukan crash |
| Batas input | Default maks 4 MiB per resource, maks 10.000 flow element per definitions, kedalaman nesting maks 64; dapat diatur lewat `ParseOption` | Melindungi engine dari input berbahaya; diuji fuzz |
| Versi proses | `version` per `bpmnProcessId`, mulai 1, naik satu per deploy yang berubah; dikunci dengan `pg_advisory_xact_lock(hashtext(bpmn_process_id))` + `UNIQUE (tenant_id, bpmn_process_id, version)` | Advisory lock mencegah race; unique constraint sebagai jaring pengaman |
| Dedup | SHA-256 atas byte resource mentah; jika checksum sama dengan versi **terbaru** proses tsb, tidak ada versi baru dan metadata versi lama dikembalikan | Meniru perilaku Zeebe (verifikasi terhadap dokumentasi Camunda 8: granularitas per proses vs per resource dan apakah dedup hanya terhadap versi terbaru) |
| Key | `int64` dari sequence Postgres milik platform (`internal/platform/keygen`), dipakai untuk `deployment_key` dan `process_definition_key` | Key unik global, sederhana; skema partisi ala Zeebe bisa menyusul di Fase 3 tanpa mengubah tipe |
| Tenant | Kolom `tenant_id TEXT NOT NULL DEFAULT '<default>'` sudah ada, belum diekspos di API | Menghindari migrasi berat di Fase 8; nilai default mengikuti konvensi Camunda (verifikasi) |
| Event domain | Topik `deployment.process_deployed`, tipe event didefinisikan di `deployment/api`, dipublikasi **setelah commit** | Modul lain (Fase 2+: timer start, message start) berlangganan tanpa meng-import internal `deployment`; outbox transaksional ditunda ke Fase 3 |
| Cache model | LRU in-memory `process_definition_key → *bpmn.Process` di modul `deployment`, diekspos lewat `api.ProcessDefinitions` | Fase 2 tidak mem-parse ulang XML untuk setiap instance |

### 3.1 Namespace yang dikenali

| Prefix umum | URI | Perlakuan |
|---|---|---|
| `bpmn` / `bpmn2` | `http://www.omg.org/spec/BPMN/20100524/MODEL` | Dimodelkan |
| `bpmndi`, `dc`, `di` | `http://www.omg.org/spec/BPMN/20100524/DI`, `.../DD/20100524/DC`, `.../DD/20100524/DI` | Dilewati |
| `zeebe` | `http://camunda.org/schema/zeebe/1.0` | Dimodelkan (lihat 4.2) |
| `modeler` | `http://camunda.org/schema/modeler/1.0` | Dibaca `executionPlatform` dan `executionPlatformVersion` untuk warning |
| `camunda` | `http://camunda.org/schema/1.0/bpmn` (Camunda 7) | Diabaikan + warning `CAMUNDA7_EXTENSION_IGNORED` |

### 3.2 Tata letak paket `libs/go/bpmn`

```
libs/go/bpmn/
├─ project.json                 # proyek Nx "bpmn-go"
├─ doc.go                       # gambaran paket (komentar Indonesia)
├─ parse.go                     # Parse, ParseOption, batas input, ParseError
├─ definitions.go               # Definitions, Message, Error, Signal, Escalation (root elements)
├─ process.go                   # Process, Scope, indeks graf
├─ element.go                   # ElementType, interface Element/FlowNode/FlowElement
├─ event.go                     # StartEvent, EndEvent, IntermediateCatch/ThrowEvent, BoundaryEvent, EventDefinition
├─ activity.go                  # ServiceTask, UserTask, SubProcess, CallActivity, MultiInstance, ...
├─ gateway.go                   # Exclusive/Parallel/Inclusive/EventBasedGateway
├─ flow.go                      # SequenceFlow, Expression
├─ zeebe.go                     # TaskDefinition, IOMapping, TaskHeaders, CalledElement, ...
├─ validate.go                  # Validate(d, profile) Issues; satu file per kelompok aturan di rules_*.go
├─ issue.go                     # Issue, Code, Severity, Position, Issues
├─ profile.go                   # Profile, ProfileFull, ProfileExecutable
├─ deps_test.go                 # memastikan paket hanya meng-import stdlib (go list)
├─ internal/xmlmodel/           # DTO encoding/xml + decoder hibrid (token + DecodeElement)
├─ internal/builder/            # DTO → model, resolusi referensi, pencatatan referensi gantung
└─ testdata/
   ├─ valid/*.bpmn  + *.golden.json
   ├─ invalid/*.bpmn + *.issues.golden.json
   ├─ parity/expectations.json
   └─ fuzz/FuzzParse/           # korpus regresi go test -fuzz
```

### 3.3 Posisi `libs/go/bpmn` terhadap aturan lapisan

`archtest` mengizinkan `domain` hanya meng-import stdlib. Karena itu di Fase 1:

- `deployment/domain` memodelkan **metadata** (`ProcessDefinition`, `Deployment`, `Checksum`,
  aturan versi dan dedup) tanpa meng-import `libs/go/bpmn`.
- `deployment/application` memanggil `bpmn.Parse` dan `bpmn.Validate` (kernel murni diperlakukan
  seperti stdlib bagi lapisan aplikasi), lalu memetakan hasilnya ke entity domain.
- `deployment/api` mengekspos `*bpmn.Process` ke modul lain lewat interface `ProcessDefinitions`.

Fase 2 kemungkinan butuh `execution/domain` yang bekerja langsung di atas model BPMN. Keputusan
apakah `archtest` diberi allowlist kernel murni (`libs/go/bpmn`, nanti `libs/go/feel`) untuk lapisan
`domain` dicatat sebagai pertanyaan terbuka (bagian 9) dan diputuskan di awal Fase 2.

## 4. Model dan cakupan

### 4.1 Matriks cakupan elemen

Kolom **Parse** dan **Validasi** dikerjakan di Fase 1 untuk semua baris, sehingga fase berikutnya
hampir tidak menyentuh parser. Kolom **Eksekusi** menunjuk fase yang menjalankannya; selama belum
dieksekusi, elemen tersebut ditolak oleh `ProfileExecutable` dengan `UNSUPPORTED_ELEMENT` tetapi
diterima `ProfileFull`.

| Elemen | Parse (F1) | Validasi struktural (F1) | Eksekusi |
|---|---|---|---|
| Start event none (proses) | ✅ | ✅ | Fase 2 |
| End event none | ✅ | ✅ | Fase 2 |
| Sequence flow + `conditionExpression` | ✅ | ✅ (ekspresi disimpan mentah) | Fase 2 (kondisi FEEL penuh: Fase 7) |
| Service task + `zeebe:taskDefinition` | ✅ | ✅ | Fase 2 (job dibuat), Fase 4 (worker) |
| Exclusive gateway (+ default flow) | ✅ | ✅ | Fase 2 |
| Parallel gateway | ✅ | ✅ | Fase 2 |
| Inclusive gateway (+ default flow) | ✅ | ✅ | Fase 2 |
| Undefined task, manual task (pass-through) | ✅ | ✅ | Fase 2 (verifikasi dukungan di Camunda 8) |
| User task (`zeebe:userTask`, `formDefinition`, `assignmentDefinition`, `taskSchedule`) | ✅ | ✅ | Fase 4/6 (ditetapkan di dokumen fasenya) |
| Timer start / intermediate catch / boundary | ✅ | ✅ (`timeDate`/`timeDuration`/`timeCycle`) | Fase 5 |
| Message start / intermediate catch / boundary / receive task + `zeebe:subscription` | ✅ | ✅ | Fase 5 |
| Message throw / end, send task (job-based) | ✅ | ✅ | Fase 5 |
| Error end event, error boundary | ✅ | ✅ | Fase 5 |
| Escalation event | ✅ | ✅ | Fase 5 |
| Embedded sub-process | ✅ | ✅ | Fase 5 |
| Event sub-process (`triggeredByEvent`) | ✅ | ✅ | Fase 5 |
| Call activity + `zeebe:calledElement` | ✅ | ✅ (tanpa cek keberadaan proses target) | Fase 5 (diusulkan; roadmap belum menyebut) |
| Multi-instance (parallel/sequential) + `zeebe:loopCharacteristics` | ✅ | ✅ | Fase 5 (diusulkan) |
| Terminate end event | ✅ | ✅ | Belum dijadwalkan |
| Signal event | ✅ | ✅ | Belum dijadwalkan |
| Event-based gateway | ✅ | ✅ | Belum dijadwalkan |
| Script task (`zeebe:script` / job), business rule task (`zeebe:calledDecision` / job) | ✅ | ✅ | Fase 7 |
| Link event, compensation, conditional event, complex gateway, transaction, ad-hoc sub-process | ❌ (dikenali, tidak dimodelkan) | `UNSUPPORTED_ELEMENT` di semua profil | Tidak direncanakan; verifikasi daftar elemen yang didukung Camunda 8 versi acuan |

### 4.2 Ekstensi `zeebe:` yang diparse

| Ekstensi | Tempat | Field model |
|---|---|---|
| `zeebe:taskDefinition` | service/send/script/business rule task, message throw/end | `Type Expression`, `Retries Expression` |
| `zeebe:ioMapping` (`input`, `output`) | activity, event | `[]Mapping{Source Expression; Target string}` |
| `zeebe:taskHeaders` (`header key value`) | task berbasis job | `[]Header{Key, Value string}` (urutan dipertahankan) |
| `zeebe:calledElement` | call activity | `ProcessID Expression`, `PropagateAllChildVariables`, `PropagateAllParentVariables bool` |
| `zeebe:subscription` | `bpmn:message` | `CorrelationKey Expression` |
| `zeebe:formDefinition` | user task | `FormKey`, `FormID`, `ExternalReference` (string/Expression) |
| `zeebe:assignmentDefinition` | user task | `Assignee`, `CandidateGroups`, `CandidateUsers Expression` |
| `zeebe:userTask` | user task | penanda user task native Zeebe (`bool`) |
| `zeebe:taskSchedule` | user task | `DueDate`, `FollowUpDate Expression` |
| `zeebe:loopCharacteristics` | multi-instance | `InputCollection`, `InputElement`, `OutputCollection`, `OutputElement` |
| `zeebe:calledDecision` | business rule task | `DecisionID`, `ResultVariable` |
| `zeebe:script` | script task | `Expression`, `ResultVariable` |
| `zeebe:properties` | elemen apa pun | `map[string]string` |
| `zeebe:versionTag` | process | `VersionTag string` |
| `zeebe:executionListeners`, `zeebe:priorityDefinition`, ekstensi lain | — | Diparse bila skemanya sudah diverifikasi; jika belum, disimpan sebagai `RawExtension{Name, Attrs}` tanpa validasi |

Nama atribut dan elemen persis (mis. `propagateAllChildVariables`) harus diverifikasi terhadap
dokumentasi Camunda 8 dan file contoh hasil Modeler sebelum DTO dikunci; korpus testdata menjadi
bukti.

### 4.3 Sketsa tipe Go

```go
package bpmn

// Parse membaca satu resource BPMN. Error hanya untuk kegagalan fatal (XML rusak, bukan BPMN,
// melebihi batas); masalah semantik dilaporkan Validate.
func Parse(r io.Reader, opts ...ParseOption) (*Definitions, error)
func Validate(d *Definitions, p Profile) Issues
func ParseAndValidate(r io.Reader, p Profile, opts ...ParseOption) (*Definitions, Issues, error)

func WithMaxBytes(n int64) ParseOption
func WithMaxElements(n int) ParseOption

type Definitions struct{ /* unexported */ }
func (d *Definitions) ID() string
func (d *Definitions) ExecutionPlatform() (name, version string)
func (d *Definitions) Processes() []*Process            // semua proses, urutan dokumen
func (d *Definitions) ExecutableProcesses() []*Process
func (d *Definitions) Message(id string) (*Message, bool)
func (d *Definitions) Error(id string) (*ErrorDef, bool)

type Process struct{ /* unexported; menanam Scope */ }
func (p *Process) ID() string                           // bpmnProcessId
func (p *Process) Name() string
func (p *Process) VersionTag() string
func (p *Process) Element(id string) (FlowElement, bool) // indeks seluruh scope bersarang
func (p *Process) NoneStartEvent() (*StartEvent, bool)
func (p *Process) StartEvents() []*StartEvent

type ElementType uint8 // StartEventType, ServiceTaskType, ExclusiveGatewayType, ...

type FlowElement interface {
	ID() string
	Name() string
	Type() ElementType
	Scope() *Scope       // proses atau sub-process yang menampung
	Position() Position  // baris/kolom di XML sumber
}

type FlowNode interface {
	FlowElement
	Incoming() []*SequenceFlow
	Outgoing() []*SequenceFlow
	IOMapping() IOMapping
}

type SequenceFlow struct{ /* unexported */ }
func (f *SequenceFlow) Source() FlowNode
func (f *SequenceFlow) Target() FlowNode
func (f *SequenceFlow) Condition() (Expression, bool)

type ExclusiveGateway struct{ /* unexported */ }
func (g *ExclusiveGateway) Default() (*SequenceFlow, bool)

type ServiceTask struct{ /* unexported */ }
func (t *ServiceTask) TaskDefinition() TaskDefinition
func (t *ServiceTask) Headers() []Header
func (t *ServiceTask) BoundaryEvents() []*BoundaryEvent
func (t *ServiceTask) MultiInstance() (*MultiInstance, bool)

type Expression struct {
	Raw    string // teks persis dari XML
	IsFEEL bool   // true jika diawali "="
}
```

Indeks yang dibangun builder (sekali, saat parse):

| Indeks | Struktur | Pemakai utama |
|---|---|---|
| Elemen per id (semua scope) | `map[string]FlowElement` di `Process` | API `GetProcessDefinition`, Operate, penanganan job/incident |
| Outgoing/incoming flow | slice di setiap `FlowNode` | Eksekusi token Fase 2 |
| Boundary event per activity | slice di activity | Fase 5 |
| Start event per jenis (none/timer/message) | slice di `Scope` | Create instance (Fase 2), timer/message start (Fase 5) |
| Child scope | slice di `Scope` | Sub-process, event sub-process |
| Message/error/signal/escalation root element | `map[string]*...` di `Definitions` | Validasi referensi, Fase 5 |

### 4.4 Daftar kode issue validasi

Semua issue membawa `Code`, `Severity`, `ProcessID`, `ElementID` (bisa kosong untuk issue level
dokumen), `Message` (bahasa Inggris, untuk log dan API), dan `Position{Line, Column}`.

| Kode | Severity | Kondisi |
|---|---|---|
| `XML_MALFORMED` | fatal (`ParseError`) | XML tidak well-formed |
| `NOT_BPMN` | fatal | Root bukan `definitions` di namespace BPMN MODEL |
| `RESOURCE_TOO_LARGE` / `TOO_MANY_ELEMENTS` / `NESTING_TOO_DEEP` | fatal | Melewati batas `ParseOption` |
| `NO_EXECUTABLE_PROCESS` | error | Tidak ada `process` dengan `isExecutable="true"` |
| `PROCESS_NOT_EXECUTABLE` | warning | Proses non-executable diabaikan |
| `MISSING_ID` / `INVALID_ID` / `DUPLICATE_ID` | error | Id kosong, bukan NCName, atau ganda dalam satu definitions |
| `DUPLICATE_PROCESS_ID` | error | Dua proses executable dengan id sama di satu deployment |
| `UNSUPPORTED_ELEMENT` | error | Elemen tidak dimodelkan, atau tidak ada di profil aktif |
| `UNSUPPORTED_EVENT_DEFINITION` | error | Kombinasi event + definition tidak didukung (mis. error start event di level proses) |
| `MULTIPLE_EVENT_DEFINITIONS` | error | Satu event dengan lebih dari satu event definition |
| `NO_START_EVENT` | error | Proses atau embedded sub-process tanpa start event |
| `MULTIPLE_NONE_START_EVENTS` | error | Lebih dari satu none start event di satu scope |
| `SUBPROCESS_INVALID_START_EVENT` | error | Embedded sub-process harus punya tepat satu none start event |
| `START_EVENT_HAS_INCOMING` / `BOUNDARY_EVENT_HAS_INCOMING` | error | Start/boundary event punya incoming flow |
| `END_EVENT_HAS_OUTGOING` | error | End event punya outgoing flow |
| `FLOW_SOURCE_NOT_FOUND` / `FLOW_TARGET_NOT_FOUND` | error | `sourceRef`/`targetRef` tidak menunjuk flow node |
| `FLOW_CROSSES_SCOPE` | error | Sequence flow menghubungkan node di scope berbeda |
| `FLOW_REF_INCONSISTENT` | warning | `<incoming>`/`<outgoing>` di node tidak cocok dengan `sourceRef`/`targetRef` (sumber kebenaran: `sourceRef`/`targetRef`) |
| `ELEMENT_UNREACHABLE` | warning | Flow node tidak bisa dicapai dari start event mana pun |
| `IMPLICIT_END` | warning | Activity/gateway tanpa outgoing flow (verifikasi apakah Camunda 8 menerimanya sebagai implicit end) |
| `TASK_DEFINITION_MISSING` | error | Task berbasis job tanpa `zeebe:taskDefinition` |
| `TASK_DEFINITION_TYPE_EMPTY` | error | Atribut `type` kosong |
| `TASK_DEFINITION_RETRIES_INVALID` | error | `retries` bukan bilangan bulat ≥ 0 dan bukan ekspresi FEEL |
| `TASK_HEADER_DUPLICATE_KEY` | error | Key header ganda |
| `IO_MAPPING_TARGET_EMPTY` / `IO_MAPPING_SOURCE_EMPTY` | error | Mapping tanpa target/source |
| `EXPRESSION_EMPTY` | error | Ekspresi wajib bernilai kosong (termasuk `=` saja) |
| `GATEWAY_DEFAULT_FLOW_INVALID` | error | `default` bukan outgoing flow gateway tersebut |
| `DEFAULT_FLOW_HAS_CONDITION` | error | Default flow punya `conditionExpression` |
| `GATEWAY_FLOW_MISSING_CONDITION` | error | Exclusive/inclusive gateway dengan > 1 outgoing: flow non-default tanpa kondisi |
| `PARALLEL_GATEWAY_FLOW_HAS_CONDITION` | error | Outgoing parallel gateway punya kondisi |
| `CONDITION_ON_NON_GATEWAY_FLOW` | error | Conditional flow dari activity (verifikasi: Camunda 8 tidak mendukung conditional flow dari task) |
| `EVENT_GATEWAY_INVALID_TARGET` | error | Target event-based gateway bukan intermediate catch timer/message (atau receive task) |
| `TIMER_DEFINITION_INVALID` | error | Bukan tepat satu dari `timeDate`/`timeDuration`/`timeCycle`, atau nilai non-FEEL bukan ISO 8601 valid |
| `TIMER_CYCLE_NOT_ALLOWED` | error | `timeCycle` di tempat yang tidak mengizinkan (mis. intermediate catch) |
| `MESSAGE_REF_NOT_FOUND` / `MESSAGE_NAME_MISSING` | error | `messageRef` gantung atau `bpmn:message` tanpa `name` |
| `MESSAGE_CORRELATION_KEY_MISSING` | error | Message catch non-start tanpa `zeebe:subscription correlationKey` |
| `ERROR_REF_NOT_FOUND` / `ERROR_CODE_MISSING` | error | Error event menunjuk error tidak ada; error end event tanpa `errorCode` |
| `ESCALATION_REF_NOT_FOUND` / `SIGNAL_REF_NOT_FOUND` | error | Referensi gantung |
| `BOUNDARY_ATTACHED_TO_NOT_FOUND` / `BOUNDARY_INVALID_HOST` | error | `attachedToRef` gantung atau bukan activity |
| `BOUNDARY_DUPLICATE_ERROR_CATCH` | error | Dua error boundary dengan `errorCode` sama di satu activity (verifikasi aturan Camunda 8) |
| `CALLED_ELEMENT_MISSING` | error | Call activity tanpa `zeebe:calledElement processId` |
| `MULTI_INSTANCE_INPUT_COLLECTION_MISSING` | error | Multi-instance tanpa `inputCollection` |
| `MULTI_INSTANCE_OUTPUT_INCOMPLETE` | error | Hanya salah satu dari `outputCollection`/`outputElement` yang diisi |
| `USER_TASK_FORM_CONFLICT` | error | Lebih dari satu sumber form (`formKey`, `formId`, `externalReference`) |
| `EVENT_SUBPROCESS_INVALID_START` | error | Event sub-process tanpa tepat satu start event bertipe yang diizinkan |
| `EXECUTION_PLATFORM_MISMATCH` | warning | `modeler:executionPlatform` bukan Camunda Cloud/8 |
| `CAMUNDA7_EXTENSION_IGNORED` | warning | Ada atribut/elemen `camunda:` |

Warning tidak menggagalkan deploy, tetapi dikembalikan di response (`warnings`) dan ditampilkan di
modeler. Setiap aturan punya minimal satu file di `testdata/invalid` yang memicunya.

## 5. Modul engine `deployment`

### 5.1 Struktur paket

```
apps/engine/internal/modules/deployment/
├─ api/
│  ├─ api.go            # interface ProcessDefinitions, DTO ProcessDefinition (metadata)
│  └─ events.go         # ProcessDeployed (Topic() = "deployment.process_deployed")
├─ domain/
│  ├─ process_definition.go  # entity ProcessDefinition, value object Key, Version, Checksum
│  ├─ deployment.go          # aggregate Deployment; aturan NextVersion, IsDuplicateOf
│  ├─ errors.go              # ErrNotFound, ErrRejected (membawa []Violation)
│  └─ repository.go          # port Repository, KeyGenerator, Clock
├─ application/
│  ├─ deploy_resource.go     # use case DeployResource (parse → validate → versi → simpan → publish)
│  ├─ get_definition.go      # query GetProcessDefinition
│  ├─ list_definitions.go    # query ListProcessDefinitions (paging)
│  └─ process_cache.go       # LRU model ter-parse; implementasi api.ProcessDefinitions
├─ infrastructure/
│  ├─ sqlc.yaml
│  ├─ queries/deployment.sql
│  ├─ db/                    # hasil sqlc (di-commit)
│  └─ postgres_repository.go
└─ interfaces/
   └─ connect_handler.go     # DeploymentService (Connect), pemetaan error ke connect.Code
```

Sketsa kontrak:

```go
// package api
type ProcessDefinition struct {
	Key            int64
	BpmnProcessID  string
	Version        int32
	VersionTag     string
	DeploymentKey  int64
	ResourceName   string
}

type ProcessDefinitions interface {
	GetByKey(ctx context.Context, key int64) (ProcessDefinition, *bpmn.Process, error)
	GetLatest(ctx context.Context, bpmnProcessID string) (ProcessDefinition, *bpmn.Process, error)
}

// package domain
type Repository interface {
	WithTx(ctx context.Context, fn func(tx TxRepository) error) error
}
type TxRepository interface {
	LockProcessID(ctx context.Context, bpmnProcessID string) error
	LatestByProcessID(ctx context.Context, bpmnProcessID string) (ProcessDefinition, bool, error)
	SaveDeployment(ctx context.Context, d Deployment) error
}
```

### 5.2 Alur `DeployResource`

1. Interfaces menerima `resources[]`; menolak request kosong, nama resource ganda, atau ekstensi
   selain `.bpmn` (Fase 1 hanya BPMN; `.dmn`/`.form` → `UNSUPPORTED_RESOURCE_TYPE`).
2. Application: `bpmn.ParseAndValidate(resource, ProfileExecutable)` per resource; kumpulkan semua
   issue lintas resource. Ada issue `error` → `domain.ErrRejected` (tidak ada yang tersimpan;
   deployment bersifat atomik untuk seluruh resource).
3. Hitung `SHA-256` tiap resource.
4. Transaksi: untuk setiap proses executable (urutan id terurut agar tidak deadlock)
   `LockProcessID` → `LatestByProcessID` → jika checksum sama: pakai metadata lama; jika tidak:
   `version = latest + 1`, key baru dari `KeyGenerator`.
5. Simpan `deployment`, `deployment_resource`, `process_definition` baru; commit.
6. Setelah commit: masukkan model ke cache, publikasi `ProcessDeployed` untuk setiap versi baru
   (tidak untuk duplikat). Kegagalan handler event dicatat di log, tidak membatalkan deploy
   (keterbatasan yang ditutup outbox di Fase 3).
7. Kembalikan `deployment_key`, metadata semua proses (baru maupun duplikat), dan warning.

Jika semua proses duplikat, Zeebe tetap mengembalikan deployment (verifikasi apakah membuat
`deployment_key` baru atau mengembalikan yang lama); perilaku kita mengikuti hasil verifikasi.

### 5.3 Skema database (`00002_deployment.sql`)

```sql
-- +goose Up
CREATE SEQUENCE engine_key_seq;  -- milik platform/keygen, bukan modul deployment

CREATE TABLE deployment (
    key         BIGINT PRIMARY KEY,
    tenant_id   TEXT NOT NULL DEFAULT '<default>',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE deployment_resource (
    id              BIGSERIAL PRIMARY KEY,
    deployment_key  BIGINT NOT NULL REFERENCES deployment (key),
    resource_name   TEXT NOT NULL,
    content         BYTEA NOT NULL,          -- byte asli, termasuk BPMNDI
    checksum        BYTEA NOT NULL,          -- SHA-256 (32 byte)
    UNIQUE (deployment_key, resource_name)
);

CREATE TABLE process_definition (
    key             BIGINT PRIMARY KEY,
    tenant_id       TEXT NOT NULL DEFAULT '<default>',
    bpmn_process_id TEXT NOT NULL,
    version         INT NOT NULL CHECK (version > 0),
    version_tag     TEXT,
    name            TEXT,
    resource_id     BIGINT NOT NULL REFERENCES deployment_resource (id),
    checksum        BYTEA NOT NULL,
    deployment_key  BIGINT NOT NULL REFERENCES deployment (key),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, bpmn_process_id, version)
);

CREATE INDEX process_definition_latest_idx
    ON process_definition (tenant_id, bpmn_process_id, version DESC);

-- +goose Down
DROP TABLE process_definition;
DROP TABLE deployment_resource;
DROP TABLE deployment;
DROP SEQUENCE engine_key_seq;
```

Kueri sqlc (`queries/deployment.sql`): `InsertDeployment`, `InsertResource`,
`InsertProcessDefinition`, `GetLatestProcessDefinition`, `GetProcessDefinitionByKey` (join resource
untuk XML), `ListProcessDefinitions` (keyset pagination `ORDER BY bpmn_process_id, version DESC`,
filter `bpmn_process_id`, opsi `latest_only` lewat `DISTINCT ON`), `AdvisoryLockProcessID`
(`SELECT pg_advisory_xact_lock(hashtext($1))`).

### 5.4 Kontrak proto `workflow/deployment/v1/deployment.proto`

```proto
syntax = "proto3";
package workflow.deployment.v1;
option go_package = "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/deployment/v1;deploymentv1";

import "google/protobuf/timestamp.proto";

service DeploymentService {
  rpc DeployResource(DeployResourceRequest) returns (DeployResourceResponse) {}
  rpc GetProcessDefinition(GetProcessDefinitionRequest) returns (GetProcessDefinitionResponse) {}
  rpc ListProcessDefinitions(ListProcessDefinitionsRequest) returns (ListProcessDefinitionsResponse) {}
}

message Resource {
  string name = 1;     // nama file, mis. "order-process.bpmn"
  bytes content = 2;
}

message DeployResourceRequest {
  repeated Resource resources = 1;
}

message DeployResourceResponse {
  int64 deployment_key = 1;
  repeated ProcessDefinition process_definitions = 2;
  repeated ValidationIssue warnings = 3;
}

message ProcessDefinition {
  int64 process_definition_key = 1;
  string bpmn_process_id = 2;
  int32 version = 3;
  string version_tag = 4;
  string name = 5;
  string resource_name = 6;
  int64 deployment_key = 7;
  google.protobuf.Timestamp created_at = 8;
  bool duplicate = 9;  // true jika deploy ini tidak membuat versi baru
}

message GetProcessDefinitionRequest {
  int64 process_definition_key = 1;
}

message GetProcessDefinitionResponse {
  ProcessDefinition process_definition = 1;
  string bpmn_xml = 2;  // resource asli, hanya di Get, tidak di List
}

message ListProcessDefinitionsRequest {
  string bpmn_process_id = 1;  // filter opsional
  bool latest_only = 2;
  int32 page_size = 3;         // default 50, maks 500
  string page_token = 4;
}

message ListProcessDefinitionsResponse {
  repeated ProcessDefinition process_definitions = 1;
  string next_page_token = 2;
}

// Dilampirkan sebagai detail error Connect (CodeInvalidArgument) saat deploy ditolak.
message DeploymentRejection {
  repeated ValidationIssue issues = 1;
}

message ValidationIssue {
  string code = 1;
  Severity severity = 2;
  string message = 3;
  string resource_name = 4;
  string bpmn_process_id = 5;
  string element_id = 6;
  int32 line = 7;
  int32 column = 8;
}

enum Severity {
  SEVERITY_UNSPECIFIED = 0;
  SEVERITY_ERROR = 1;
  SEVERITY_WARNING = 2;
}
```

Pemetaan error: `ErrRejected` → `InvalidArgument` + `DeploymentRejection`; `ErrNotFound` →
`NotFound`; resource > batas → `InvalidArgument` dengan issue `RESOURCE_TOO_LARGE`; handler memakai
`connect.WithReadMaxBytes` (default 16 MiB per request). `int64` di TS menjadi `bigint`
(protobuf-es); UI menampilkan key sebagai string.

## 6. Rencana kerja Fase 1 (urutan eksekusi)

Setiap langkah punya **hasil yang bisa diverifikasi**. 6.3–6.8 (lib) dan 6.9–6.10 (proto, skema)
bisa paralel; 6.13 menunggu sisa Fase 0 di sisi web.

### 6.1 Prasyarat dari Fase 0

- ✅ Terpenuhi per 2026-10-28: `Makefile` dengan `gen`, `test`, `test-integration`, `lint`; CI
  menjalankan `nx affected -t lint test typecheck build` dan job integrasi Postgres 18.
- ✅ Terpenuhi per 2026-10-28: shadcn/ui di `libs/web/ui`, TanStack Router, kanvas bpmn-js di
  `libs/web/bpmn-viewer`.

Hasil: `make check` hijau di CI pada branch `feat/bpmn-parser`.

### 6.2 Korpus BPMN dan hasil referensi Camunda

- Buat file di Camunda Modeler (execution platform Camunda 8, versi platform dicatat) untuk setiap
  baris matriks 4.1, plus file invalid untuk setiap kode issue yang bisa dibuat dari Modeler.
  Kasus yang tidak bisa dibuat lewat Modeler (id ganda, ref gantung) diedit tangan dan diberi
  komentar XML `<!-- hand-edited: ... -->`.
- Sertakan beberapa proses nyata kompleks (≥ 100 node), collaboration dengan dua pool, dan file
  yang memakai prefix `bpmn2:`.
- Deploy setiap file ke Camunda 8 referensi (`make up PROFILES=camunda`) dan catat hasil terima/tolak
  serta pesan penolakan ke `testdata/parity/expectations.json`.

Hasil: ≥ 30 file di `testdata/valid` + `testdata/invalid`, `expectations.json` terisi.

### 6.3 Kerangka `libs/go/bpmn` dan proyek Nx

- `project.json` dengan nama `bpmn-go`, tag `type:kernel`, `lang:go`; target di bagian 7.
- `deps_test.go`: `go list -deps` paket `libs/go/bpmn/...` tidak boleh berisi paket non-stdlib.
- `engine` menambah `bpmn-go` ke `implicitDependencies`.

Hasil: `pnpm nx run-many -t build test lint -p bpmn-go` lolos dengan paket kosong.

### 6.4 Lapisan XML (`internal/xmlmodel`)

- DTO untuk `definitions`, `process`, setiap flow element di 4.1, event definition, root element
  (`message`, `error`, `signal`, `escalation`), dan `extensionElements` zeebe.
- Decoder hibrid: loop `Token()` di container, `DecodeElement()` per elemen, rekam
  `InputPos()` sebelum setiap elemen; lewati subtree DI dengan `Skip()`.
- Batas ukuran (`io.LimitReader` + deteksi overflow), jumlah elemen, dan kedalaman.
- Elemen BPMN yang tidak dikenal dicatat sebagai `Unknown{Name, Pos}` untuk `UNSUPPORTED_ELEMENT`.

Hasil: unit test decoding semua file korpus menghasilkan DTO tanpa error; file bukan BPMN → `NOT_BPMN`.

### 6.5 Model domain, builder, dan indeks

- Tipe di 4.3; builder resolusi referensi (`sourceRef`, `targetRef`, `default`, `attachedToRef`,
  `messageRef`, `errorRef`) dua tahap: buat semua node per scope, lalu sambungkan.
- Referensi gantung tidak menggagalkan builder; dicatat untuk `Validate`.
- Test `-race`: 50 goroutine menelusuri model yang sama.

Hasil: golden JSON (dump kanonik model, terurut id) untuk semua file valid; flag `-update`
untuk regenerasi.

### 6.6 Ekstensi `zeebe:`

- Parse semua ekstensi di 4.2; ekspresi menjadi `Expression`.
- Verifikasi nama atribut terhadap file korpus dan dokumentasi Camunda 8; perbedaan dicatat di ADR-0008.

Hasil: golden JSON memuat task definition, header, I/O mapping, subscription, dsb.

### 6.7 Validator, kode issue, profil

- Aturan di 4.4, satu fungsi per aturan, dikelompokkan `rules_structure.go`, `rules_flow.go`,
  `rules_events.go`, `rules_zeebe.go`.
- `ProfileExecutable` awal = elemen bertanda "Fase 2" di matriks 4.1; `ProfileFull` = semua elemen
  yang dimodelkan.
- Issue diurutkan stabil (resource, posisi, kode) agar golden deterministik.

Hasil: setiap kode issue punya minimal satu test invalid; file valid tanpa issue error di `ProfileFull`.

### 6.8 Fuzz dan benchmark

- `FuzzParse(f *testing.F)`: seed dari semua file testdata; properti: tidak panic, selesai dalam
  batas waktu, dan jika `Parse` sukses maka invarian model terpenuhi (setiap flow punya source dan
  target di indeks yang sama, setiap id di indeks unik, `Validate` tidak panic).
- `BenchmarkParseLarge` (file 500 node yang di-generate di test) dan `BenchmarkValidate`.

Hasil: `nx run bpmn-go:fuzz` 10 menit bersih; angka benchmark dicatat di `libs/go/bpmn/README.md`.

### 6.9 Proto `workflow/deployment/v1`

- Tulis `deployment.proto` (5.4), `buf lint` lolos, `buf generate` ke Go dan TS, hasil di-commit.

Hasil: paket `deploymentv1` dan `deploymentv1connect` terkompilasi; stub TS ada di `libs/web/api-client`.

### 6.10 Migrasi, keygen, sqlc

- `00002_deployment.sql` (5.3) di `internal/platform/db/migrations`.
- `internal/platform/keygen`: interface `Generator{Next(ctx) (int64, error)}` + implementasi
  `nextval('engine_key_seq')`.
- `sqlc.yaml` pertama di `deployment/infrastructure`, membaca skema dari folder migrasi platform;
  `make gen` menjalankan `sqlc generate`; hasil di-commit dan CI memeriksa `git diff`.

Hasil: engine start → migrasi versi 2; `sqlc generate` bersih.

### 6.11 Modul `deployment` lima lapisan

- Domain + unit test aturan versi/dedup; application + unit test dengan repository fake dan
  `bpmn` sungguhan; infrastructure + integration test (deploy, dedup, versi, 20 deploy paralel);
  interfaces + test handler Connect (`httptest`) termasuk detail `DeploymentRejection`.

Hasil: `go test ./apps/engine/...` dan `make test-integration` lolos; `archtest` lolos.

### 6.12 Wiring dan event

- `cmd/engine/main.go` merakit `deployment` (repository, keygen, cache, event bus, handler).
- Bus in-process dibagi modul (`eventbus.NewInMemory()` yang kini benar-benar dipakai).
- Test: subscriber `deployment.process_deployed` menerima event dengan key dan versi yang benar.

Hasil: `buf curl` / `curl` ke `/workflow.deployment.v1.DeploymentService/DeployResource` dengan
file korpus mengembalikan key; deploy ulang mengembalikan `duplicate: true`.

### 6.13 Modeler-web fitur `deployment`

```
apps/modeler-web/src/features/deployment/
├─ domain/          # ProcessDefinition, ValidationIssue, aturan "issue error memblokir deploy"
├─ application/     # useDeployResource (mutation), useProcessDefinitions (query + key factory),
│                   # deployment.store.ts (Zustand: issue aktif, issue terpilih, panel terbuka)
├─ infrastructure/  # deployment.repository.ts: Connect client, mapper proto → domain,
│                   # ekstraksi DeploymentRejection dari ConnectError.findDetails
└─ presentation/    # DeployButton, DefinitionsTable (TanStack Table), IssuesPanel, DiagramIssueOverlay
```

- Tombol deploy mengambil XML via `modeler.saveXML()` dari `libs/web/bpmn-viewer`.
- Issue dengan `element_id` diberi marker (`canvas.addMarker(id, 'issue-error')`) dan overlay badge;
  klik issue di panel memilih dan men-scroll ke elemen (`selection.select`, `canvas.scrollToElement`).
- Halaman `/definitions` (TanStack Router, loader `ensureQueryData`) berisi tabel dengan filter
  `bpmn_process_id` dan toggle latest-only; klik baris membuka XML di viewer read-only.
- Sukses deploy: toast + invalidasi query `processDefinitions`.

Hasil: Vitest untuk mapper dan store; uji manual end-to-end deploy valid dan invalid tercatat di PR.

### 6.14 Uji paritas

- `tools/scripts/parity-deploy` (Go): untuk setiap file korpus, deploy ke engine kita (profil `full`
  lewat flag test) dan bandingkan dengan `expectations.json`; mode `--live` juga men-deploy ke
  Camunda 8 referensi (endpoint REST deployment Camunda 8; verifikasi path dan autentikasi untuk
  versi acuan) dan memperbarui ekspektasi.
- Pembandingan: terima/tolak harus sama; untuk penolakan, element id yang disebut Camunda harus ada
  di issue kita bila Camunda menyebutnya. Perbedaan disengaja diberi field `deviation` + alasan.
- CI menjalankan mode offline (tanpa Camunda); mode live dijalankan manual sebelum fase ditutup.

Hasil: `make parity` lolos; laporan perbedaan terlampir di PR penutup fase.

### 6.15 Dokumentasi

- ADR-0008 Parser BPMN di atas `encoding/xml` dengan model immutable dua tahap.
- ADR-0009 Versioning process definition dan dedup checksum.
- `libs/go/bpmn/README.md`: cara pakai, matriks cakupan, daftar kode issue (dihasilkan dari kode
  lewat `go generate` agar tidak basi).
- Perbarui `docs/plan/README.md`: Fase 1 menautkan dokumen ini.

## 7. Target Nx dan Makefile

| Proyek | Target | Perintah |
|---|---|---|
| `bpmn-go` | `build` | `go build ./libs/go/bpmn/...` |
| `bpmn-go` | `test` | `go test -race ./libs/go/bpmn/...` |
| `bpmn-go` | `lint` | `golangci-lint run ./libs/go/bpmn/...` |
| `bpmn-go` | `fuzz` (cache off) | `go test -run=^$ -fuzz=FuzzParse -fuzztime=$FUZZTIME ./libs/go/bpmn` (default 60s lokal, 10m terjadwal) |
| `bpmn-go` | `bench` (cache off) | `go test -run=^$ -bench=. -benchmem ./libs/go/bpmn` |
| `engine` | `sqlc` | `sqlc generate -f apps/engine/internal/modules/deployment/infrastructure/sqlc.yaml` |
| `engine` | `test` | sudah ada; kini mencakup modul `deployment` |
| `proto` | `generate`, `lint`, `breaking` | sudah ada; mencakup `deployment/v1` |
| `modeler-web` | `test`, `build` | sudah ada; mencakup fitur `deployment` |

Input `bpmn-go:test` menyertakan `{projectRoot}/testdata/**` agar perubahan golden membatalkan cache.

Target Makefile baru: `make fuzz [FUZZTIME=10m]`, `make bench`, `make parity [LIVE=1]`; `make gen`
diperluas dengan `sqlc generate`.

## 8. Risiko dan mitigasi

| Risiko | Dampak | Mitigasi |
|---|---|---|
| Aturan validasi berbeda dari Camunda 8 | Proses diterima di sini tapi ditolak Camunda (atau sebaliknya), paritas gagal | Korpus + `expectations.json` dari Camunda referensi sejak 6.2; aturan yang tidak pasti ditandai "verifikasi" dan baru dikunci setelah uji live |
| Skema `zeebe:` berubah antar versi Camunda | Atribut baru diabaikan diam-diam | Kunci versi Camunda acuan di ADR-0008; ekstensi tak dikenal disimpan sebagai `RawExtension`; test korpus per versi Modeler |
| `encoding/xml` lambat untuk file besar | Deploy lambat | Benchmark sejak 6.8; deploy bukan hot path; model di-cache sehingga parse hanya sekali per versi |
| `encoding/xml` kurang ketat (menerima XML yang tidak valid skema XSD) | Input aneh lolos | Validasi semantik eksplisit; tidak menjanjikan validasi XSD penuh (dicatat di ADR) |
| Input berbahaya (bom ukuran, nesting dalam) | Engine kehabisan memori | Batas ukuran/elemen/kedalaman + fuzz + `connect.WithReadMaxBytes` |
| Kondisi FEEL tidak tervalidasi sampai Fase 7 | Proses dengan ekspresi rusak lolos deploy | Issue `EXPRESSION_EMPTY` dan cek statis; warning `FEEL_NOT_VALIDATED` opsional; Fase 7 menambah validasi tanpa ubah API |
| Race versi saat deploy paralel | Versi ganda | Advisory lock per `bpmn_process_id` + unique constraint + test 20 deploy paralel |
| Event hilang jika handler gagal setelah commit | Fase 5 (timer start) tidak terjadwal | Didokumentasikan; outbox transaksional di Fase 3; subscriber Fase 2+ juga melakukan rekonsiliasi saat start |
| `archtest` menghalangi domain memakai model BPMN | Fase 2 terpaksa menaruh logika di application | Fase 1 menjaga domain bebas BPMN; keputusan allowlist kernel dibuat di awal Fase 2 (bagian 9) |
| Modeler-web tertahan sisa Fase 0 | Langkah 6.13 terlambat | Lib dan modul engine tidak bergantung pada web; 6.13 bisa menyusul tanpa memblokir Fase 2 |
| Golden file sering berubah karena format dump | Review PR berisik | Dump JSON kanonik terurut, hanya field semantik (tanpa posisi kecuali di file khusus) |

## 9. Pertanyaan terbuka

1. **Allowlist kernel di `archtest`**: bolehkah lapisan `domain` meng-import `libs/go/bpmn` (dan
   nanti `libs/go/feel`)? Fase 1 tidak membutuhkannya, Fase 2 hampir pasti membutuhkannya.
2. **Versi Camunda 8 acuan** untuk paritas (8.6, 8.7, atau 8.8) — menentukan endpoint REST, daftar
   ekstensi `zeebe:` (mis. `zeebe:userTask`, `executionListeners`), dan elemen yang didukung.
3. **Dry-run**: perlukah RPC `ValidateResource` (validasi tanpa simpan) agar modeler bisa lint
   langsung saat menggambar? Alternatif: menjalankan `libs/go/bpmn` sebagai WASM di browser.
4. **DMN dan form** sebagai resource deploy: tetap ditolak di Fase 1 (`UNSUPPORTED_RESOURCE_TYPE`)
   atau sudah disimpan mentah tanpa validasi?
5. **Granularitas dedup**: per proses (seperti rencana) atau per resource utuh — diputuskan setelah
   verifikasi perilaku Zeebe.

## 10. Estimasi dan pembagian kerja

Perkiraan **11–14 hari kerja** untuk 1 engineer, atau 7–8 hari untuk 2 engineer paralel (di luar
sisa Fase 0 di 6.1):

| Hari | Fokus |
|---|---|
| 1–2 | 6.2 korpus BPMN + rekam hasil Camunda referensi, 6.3 kerangka lib |
| 3–4 | 6.4 lapisan XML, 6.5 model + builder + indeks |
| 5 | 6.6 ekstensi `zeebe:` |
| 6–7 | 6.7 validator dan seluruh kode issue |
| 8 | 6.8 fuzz + benchmark, 6.9 proto, 6.10 migrasi + sqlc |
| 9–10 | 6.11 modul `deployment`, 6.12 wiring dan event |
| 11–12 | 6.13 modeler-web fitur `deployment` |
| 13–14 | 6.14 uji paritas, perbaikan perbedaan, 6.15 dokumentasi, review DoD |

Pembagian jika 2 orang: **A** = `libs/go/bpmn` (6.3–6.8) lalu paritas; **B** = proto, skema, modul
`deployment`, modeler-web (6.9–6.13), mulai dengan model palsu lalu beralih ke lib setelah 6.5.

## 11. Keluaran Fase 1

Setelah fase ini selesai, Fase 2 mendapat:

- `api.ProcessDefinitions` yang mengembalikan `*bpmn.Process` ter-cache berdasarkan key atau
  `bpmnProcessId` terbaru, dengan indeks outgoing/incoming flow dan elemen per id siap untuk
  eksekusi token.
- Event `deployment.process_deployed` untuk modul yang perlu bereaksi terhadap versi baru.
- `ProfileExecutable` sebagai satu tempat untuk "membuka" elemen baru: setiap fase yang
  mengimplementasikan eksekusi elemen menambahkannya ke profil beserta test korpusnya.
