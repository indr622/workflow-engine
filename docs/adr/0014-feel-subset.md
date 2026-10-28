# ADR-0014: Subset FEEL untuk kondisi dan task definition, ditolak saat deploy di luar subset

- **Status:** Diterima
- **Tanggal:** 2026-10-28

## Konteks

Model dari Camunda Modeler hampir selalu memakai kondisi FEEL seperti `=amount > 100`. FEEL penuh
baru dibangun di Fase 7. Menolak semua kondisi membuat gateway tidak bisa diuji; evaluator ad-hoc
akan menyimpang dari FEEL dan harus dibuang.

## Keputusan

- Kernel `libs/go/feel` (stdlib saja) mengimplementasikan subset: literal (angka, string, boolean,
  null), path variabel bertitik, perbandingan `= != < <= > >=`, `and`, `or`, `not(…)`, dan kurung.
- Semantik identik dengan FEEL untuk konstruksi yang didukung: angka desimal eksak (`big.Rat`),
  variabel atau field yang tidak ada bernilai `null`, perbandingan lintas tipe menghasilkan
  `null`, logika tiga nilai untuk `and`/`or`.
- Konstruksi di luar subset (aritmetika, pemanggilan fungsi, `in`, `if`, list, context literal,
  dan lainnya) menghasilkan `*UnsupportedError` saat kompilasi, tidak pernah ditebak.
- Kondisi yang hasilnya bukan boolean, termasuk `null`, menghasilkan incident `CONDITION_ERROR`
  di gateway, bahkan bila ada default flow. Paritas dengan Camunda 8.8 mengonfirmasi perilaku ini.
- `zeebe:taskDefinition` type dan retries berbentuk FEEL hanya boleh literal atau path.
- Saat deploy (profil executable), setiap kondisi dan ekspresi task definition dikompilasi.
  Kondisi tanpa awalan `=` dan konstruksi di luar subset ditolak dengan
  `EXPRESSION_NOT_SUPPORTED`; sintaks rusak dengan `EXPRESSION_SYNTAX_ERROR`. I/O mapping ditolak
  `IO_MAPPING_NOT_SUPPORTED` sampai Fase 7.
- Domain eksekusi memakai port `ExpressionEvaluator`; implementasinya `domain.FeelEvaluator`
  langsung di atas kernel `feel` (diizinkan ADR-0010), dengan cache hasil kompilasi.

## Konsekuensi

- Fase 7 memperluas grammar dan menghapus `UnsupportedError` bertahap tanpa mengubah pemanggil
  atau perilaku model yang sudah berjalan.
- Kasus semantik yang belum pasti ditandai `verify: Camunda 8.8` di test kernel; yang relevan untuk
  gateway sudah dibandingkan langsung lewat paritas eksekusi (string, path bersarang, angka besar,
  kondisi null).
