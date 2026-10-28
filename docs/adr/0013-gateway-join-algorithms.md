# ADR-0013: Algoritma join parallel dan inclusive gateway

- **Status:** Diterima
- **Tanggal:** 2026-10-28

## Konteks

Join harus aktif tepat sekali per fork, mendukung token berulang (loop), dan tidak menyimpan
"token mati". Inclusive join adalah bagian tersulit: join harus menunggu cabang yang masih mungkin
datang, tetapi tidak menunggu cabang yang tidak diambil.

## Keputusan

- **Counter per `(flow_scope_key, gateway_id, sequence_flow_id)`.** Record `SEQUENCE_FLOW_TAKEN`
  ke gateway join (parallel atau inclusive dengan lebih dari satu incoming) menaikkan counter lewat
  `Apply`. Record `ELEMENT_ACTIVATING` gateway join mengurangi counter incoming yang > 0. Counter
  adalah turunan record, bukan record sendiri.
- **Parallel join** aktif segera saat semua incoming punya counter ≥ 1. Token kedua di flow yang
  sama sebelum join lengkap tetap tercatat dan memicu join berikutnya.
- **Inclusive join: reachability statis + cek token aktif saat quiescence.** Untuk setiap incoming
  flow `f` dari join `G`, `Upstream(G, f)` adalah node yang bisa mencapai `f` tanpa melewati `G`
  (dihitung sekali per model dan di-cache). Saat antrean command kosong, `G` aktif bila tidak ada
  token aktif (element instance non-final atau counter join lain) di upstream incoming yang belum
  punya token.
- **Fork inclusive** mengevaluasi semua kondisi; tidak ada yang true → default; tanpa default →
  incident `CONDITION_ERROR`.
- **Penyelesaian flow scope** juga diperiksa saat quiescence: scope selesai bila tidak ada anak
  non-final dan tidak ada counter tersisa. Counter yang tidak akan pernah lengkap (exclusive split
  ke parallel join) membuat instance tetap `ACTIVE` tanpa token aktif.

## Batas yang diketahui

| Kasus | Perilaku |
|---|---|
| Graf blok asiklik | Benar (property test 20.000 graf acak) |
| Inclusive join di dalam loop | Token yang akan kembali melewati join tidak dihitung upstream; join tidak menunggu putaran berikutnya |
| Dua inclusive join yang saling menjadi upstream | Bisa saling menunggu selamanya; deploy memberi warning `INCLUSIVE_JOIN_IN_CYCLE` |
| Sub-process dan boundary event | Ditinjau ulang di Fase 5 |

## Bukti

- `TestGatewayProperties`: graf blok acak (fork/join parallel dan inclusive bersarang, exclusive
  split/merge, kedalaman ≤ 4, lebar ≤ 5), job diselesaikan dalam urutan acak; instance selalu
  `COMPLETED`, aktivasi join = aktivasi fork, counter kosong, jumlah job cocok dengan oracle,
  replay identik. Lolos untuk 20.000 iterasi.
- Paritas dengan Camunda 8.8 untuk skenario inclusive (sebagian cabang, semua cabang, default,
  join menunggu job): hasil observable sama.
