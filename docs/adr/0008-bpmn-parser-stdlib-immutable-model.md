# ADR-0008: Parser BPMN di atas encoding/xml dengan model immutable dua tahap

- **Status:** Diterima
- **Tanggal:** 2026-10-28

## Konteks

Engine harus menerima BPMN 2.0 hasil Camunda Modeler (ekstensi `zeebe:`), memvalidasinya dengan
pesan per elemen seperti penolakan deployment Camunda 8, dan menyediakan model yang cepat ditelusuri
untuk eksekusi token. Parser dipakai engine, SDK, dan tooling, sehingga tidak boleh menarik
database maupun transport.

## Keputusan

- Paket `libs/go/bpmn` hanya memakai **standard library**; ditegakkan `TestOnlyStdlibDependencies`.
- **Dua tahap**: `internal/xmltree` membaca XML menjadi pohon node ringan dengan namespace
  ter-resolve dan posisi baris/kolom (DOCTYPE ditolak, subtree BPMNDI dilewati, batas ukuran,
  jumlah elemen, dan kedalaman), lalu `build` mengubah pohon menjadi model.
- **Model immutable**: field tidak diekspor, hanya getter; referensi antar node berupa pointer
  (`SequenceFlow.Source()`, `Activity.BoundaryEvents()`), dengan indeks `Process.Element(id)`.
  Aman dibaca paralel tanpa lock.
- Elemen dimodelkan per **kategori** (`Event`, `Activity`, `Gateway`, `SequenceFlow`) dengan
  `Type()` sebagai pembeda, bukan satu tipe Go per elemen BPMN. Lebih sedikit tipe untuk Fase 2.
- `Parse` hanya gagal untuk kegagalan fatal (`*ParseError`). Semua masalah semantik dikumpulkan
  `Validate` sebagai `Issues` dengan kode stabil, severity, element id, dan posisi, diurutkan stabil.
- **Profil**: `ProfileFull` untuk lint dan paritas, `ProfileExecutable` untuk apa yang bisa
  dieksekusi engine; daftar executable tumbuh tiap fase.
- Ekspresi disimpan mentah sebagai `Expression{Raw, IsFEEL}`; FEEL baru dievaluasi di Fase 7.
- Versi acuan paritas: **Camunda 8.8** (`camunda/zeebe:8.8.44`). Korpus 59 file diuji live;
  satu penyimpangan disengaja tercatat di `testdata/parity/expectations.json` (flow yang keluar
  dari sub-process ditolak oleh kita, diterima Camunda).

## Konsekuensi

- Tidak ada validasi XSD penuh; aturan semantik eksplisit. Input aneh yang lolos XML well-formed
  diperlakukan sebagai issue, bukan crash (diuji fuzz).
- Parse 500 service task memakan sekitar 8 ms dan 3 MB; deploy bukan hot path dan model di-cache.
- Elemen yang belum dimodelkan (link, compensation, conditional, complex gateway, transaction,
  ad-hoc) ditolak dengan `UNSUPPORTED_ELEMENT` di semua profil.
