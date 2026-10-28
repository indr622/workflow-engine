# ADR-0011: Key entity berupa UUIDv7, bukan auto-increment

- **Status:** Diterima
- **Tanggal:** 2026-10-28
- **Menggantikan sebagian:** ADR-0009 (bagian key dari sequence `engine_key_seq`)

## Konteks

Fase 1 memakai sequence PostgreSQL `engine_key_seq` (dan `BIGSERIAL` untuk
`deployment_resource.id`) sebagai sumber key `int64`. User meminta agar id tidak memakai
auto-increment dan diganti UUID. Selain itu, Fase 2 membutuhkan puluhan key per command; key dari
sequence memaksa round trip ke database atau alokasi blok yang rumit.

## Keputusan

- Semua key entity (deployment, deployment resource, process definition, process instance,
  element instance, sequence flow taken, variabel, job, incident) adalah **UUIDv7** (RFC 9562).
- Key dibuat di proses engine oleh `internal/platform/keygen.UUIDv7`: stdlib saja, aman untuk
  banyak goroutine, dan monotonik di dalam satu proses memakai counter 12-bit (RFC 9562 bagian
  6.2). Bila jam mundur atau counter penuh, generator meminjam milidetik berikutnya agar urutan
  tetap naik.
- Kolom database bertipe `UUID`. Kode Go memakai `string` kanonik huruf kecil
  (`xxxxxxxx-xxxx-7xxx-yxxx-xxxxxxxxxxxx`); sqlc memetakan `uuid` ke `string`.
- Proto memakai `string` untuk setiap field `*_key`. Lapisan interfaces menolak key yang bukan UUID
  kanonik dengan `INVALID_ARGUMENT`.
- Migrasi `00003_uuid_keys.sql` mengonversi data Fase 1 secara deterministik (BIGINT menjadi UUID
  32 digit heksadesimal berawalan nol), mempertahankan relasi, lalu menghapus `engine_key_seq` dan
  sequence `BIGSERIAL`. Migrasi ini sengaja satu arah.
- Domain eksekusi menerima `KeyGenerator` tanpa error dan tanpa I/O. Mekanisme blok key dan
  `ErrKeysExhausted` dari rencana Fase 2 tidak diperlukan.
- Urutan per instance di `execution_record` tetap memakai kolom `sequence` yang dihitung engine;
  itu nomor urut, bukan id, dan tidak memakai auto-increment database.

## Konsekuensi

- Key bisa dibuat tanpa koordinasi di replika engine mana pun, tanpa round trip ke database.
- Karena UUIDv7 berawalan timestamp, indeks B-tree tetap terisi berurutan dan paginasi keyset
  menurut key tetap berarti "terbaru dahulu" atau "terlama dahulu".
- Key lebih panjang daripada `int64` (16 byte di database, 36 karakter di API).
- Antar replika engine, urutan key hanya seakurat jam masing-masing mesin. Urutan yang benar-benar
  penting (record per instance) memakai `sequence`, bukan key.
- Perubahan proto deployment dari `int64` ke `string` adalah breaking change. Belum ada klien
  eksternal selain modeler-web di repo ini, yang diperbarui bersamaan.
