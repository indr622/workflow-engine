# ADR-0009: Versioning process definition dan dedup checksum

- **Status:** Diterima
- **Tanggal:** 2026-10-28

## Konteks

Setiap deploy harus menghasilkan versi yang monoton per `bpmnProcessId`, aman terhadap deploy
paralel, dan tidak membuat versi baru bila file tidak berubah (perilaku Zeebe).

## Keputusan

- Modul `deployment` lima lapisan. `domain` tidak mengenal BPMN; parsing dan validasi
  (`bpmn.ParseAndValidate` dengan `ProfileExecutable`) ada di `application`.
- **Versi per proses**: `version = latest + 1`, mulai 1, di bawah
  `pg_advisory_xact_lock(hashtext(tenant || ':' || bpmnProcessId))` dalam transaksi deploy, dengan
  `UNIQUE (tenant_id, bpmn_process_id, version)` sebagai jaring pengaman. Kunci diambil dalam
  urutan id proses terurut agar deploy multi-proses tidak deadlock.
- **Dedup**: SHA-256 atas byte resource mentah dibandingkan dengan checksum **versi terbaru**
  proses tersebut. Sama berarti metadata versi lama dipakai ulang dan ditandai `duplicate`.
  Setiap request tetap mendapat `deployment_key` baru dan resource-nya disimpan.
- **Atomik**: satu issue error pada resource mana pun menolak seluruh request; tidak ada yang
  tersimpan. Penolakan dikirim sebagai Connect `InvalidArgument` dengan detail
  `DeploymentRejection` berisi semua issue.
- **Key** int64 dari sequence global `engine_key_seq` (`internal/platform/keygen`), diambil di
  dalam transaksi.
- XML asli disimpan byte-per-byte di `deployment_resource.content` agar diagram tetap utuh.
- Model ter-parse di-cache LRU per key dan diekspos lewat `api.ProcessDefinitions`; event
  `deployment.process_deployed` dipublikasikan **setelah commit** untuk versi baru saja. Kegagalan
  handler hanya dicatat; outbox transaksional menyusul di Fase 3.
- `ValidateResource` menjalankan use case validasi yang sama tanpa menyimpan, dengan pilihan profil.
- DMN dan form ditolak (`UNSUPPORTED_RESOURCE_TYPE`) sampai fase yang bersangkutan.

## Konsekuensi

- Dua deploy identik berturut-turut menghasilkan dua baris `deployment` tetapi satu
  `process_definition`. Penyimpanan resource duplikat diterima demi kesederhanaan.
- Perubahan sekecil apa pun pada byte file (termasuk koordinat diagram) menaikkan versi, sama
  seperti Zeebe.
- Diuji integrasi: 20 deploy paralel untuk proses yang sama menghasilkan versi 2 sampai 21 tanpa celah.
