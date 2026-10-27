# bpmn

Parser, model, dan validator BPMN 2.0 untuk engine. Kernel murni: hanya standard library,
tanpa database maupun transport. Rencana lengkap ada di
[docs/plan/01-fase-1-parser-model-bpmn.md](../../../docs/plan/01-fase-1-parser-model-bpmn.md).

## Pemakaian

```go
defs, issues, err := bpmn.ParseAndValidate(r, bpmn.ProfileExecutable)
if err != nil {
    // *bpmn.ParseError: XML rusak, bukan BPMN, atau melewati batas input
}
if issues.HasErrors() {
    // resource ditolak; setiap Issue membawa Code, ElementID, dan posisi baris/kolom
}
for _, p := range defs.ExecutableProcesses() {
    start, _ := p.NoneStartEvent()
    for _, f := range start.Outgoing() {
        _ = f.Target()
    }
}
```

- `Parse` hanya gagal untuk kegagalan fatal. Masalah semantik dikumpulkan `Validate` sekaligus.
- Model immutable: field tidak diekspor, baca lewat getter, aman untuk pembacaan paralel.
- Diagram interchange (BPMNDI) dilewati. Simpan XML asli bila diagram dibutuhkan.
- Batas default: 4 MiB, 50.000 elemen, kedalaman 64. Ubah lewat `ParseOption`.

## Profil

| Profil | Isi |
|---|---|
| `ProfileFull` | Semua elemen yang dimodelkan. Dipakai lint modeler dan uji paritas |
| `ProfileExecutable` | Hanya elemen yang sudah dieksekusi engine. Fase 2: none start/end, sequence flow, service task, undefined/manual task, exclusive/parallel/inclusive gateway |

## Korpus test

`testdata/valid` berisi 27 file yang harus bebas error di `ProfileFull`; `testdata/invalid` berisi
32 file yang masing-masing mendeklarasikan kode error yang diharapkan di komentar
`<!-- expect: ... -->`. Golden file dibuat ulang dengan:

```bash
go test ./libs/go/bpmn -run 'TestValidCorpus|TestInvalidCorpus' -update
```

Korpus dihasilkan dari generator agar konsisten dengan ekspor Camunda Modeler 5.40 untuk
Camunda 8.8; file hasil ekspor nyata ditambahkan saat uji paritas.

## Target Nx

```bash
pnpm nx run bpmn-go:test
pnpm nx run bpmn-go:lint
FUZZTIME=10m pnpm nx run bpmn-go:fuzz
pnpm nx run bpmn-go:bench
```

## Benchmark awal (2026-10-28, laptop dev Windows, Go 1.27)

| Benchmark | Waktu | Alokasi |
|---|---|---|
| `BenchmarkParse500` (500 service task) | 7,7 ms | 2,9 MB, 52 ribu alokasi |
| `BenchmarkParseAndValidate500` | 11,7 ms | 3,0 MB, 54 ribu alokasi |

Deploy bukan hot path dan model di-cache per versi, sehingga angka ini cukup untuk Fase 1.
