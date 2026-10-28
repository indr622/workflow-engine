# Modul execution

Menjalankan process instance: membuat instance dari process definition yang sudah di-deploy,
menggerakkan token lewat sequence flow dan gateway, berhenti di service task sebagai job, dan
melanjutkan setelah job diselesaikan. Keputusan desain: ADR-0012 (model eksekusi), ADR-0013
(algoritma join), ADR-0014 (subset FEEL), ADR-0011 (key UUIDv7). Fase 3: ADR-0015 (outbox),
ADR-0016 (verify dan rebuild), ADR-0017 (idempotensi), ADR-0018 (contention), ADR-0019 (snapshot
dan retensi).

## Lapisan

| Paket | Isi |
|---|---|
| `api` | `ProcessInstances`, `Jobs`, event setelah commit (`execution.*`) |
| `domain` | `Processor` (run-to-completion), `Apply`, state runtime, record, `FeelEvaluator`, port |
| `application` | `Commands` (transaksi per command, publikasi event), `Queries` atas proyeksi |
| `infrastructure` | Repository Postgres (kunci + muat satu batch, tulis satu batch), model baca, adapter key |
| `interfaces` | Handler Connect `ProcessInstanceService` dan `JobService` |

## Elemen yang dieksekusi (Fase 2)

| Elemen | Perilaku |
|---|---|
| None start event, none end event, undefined task, manual task | Pass-through |
| Service task | Membuat job (`type`, `retries` default 3, header); menunggu `CompleteJob` |
| Exclusive gateway | Flow pertama yang true (urutan dokumen), lalu default, lalu incident |
| Parallel gateway | Fork semua outgoing; join menunggu semua incoming |
| Inclusive gateway | Fork semua yang true, lalu default, lalu incident; join dengan reachability |
| Implicit end | Elemen tanpa outgoing menyelesaikan cabangnya |

## Siklus hidup dan record

Element instance: `ACTIVATING` → `ACTIVATED` → `COMPLETING` → `COMPLETED`, atau `TERMINATING` →
`TERMINATED`. Record yang ditulis:

| Value type | Intent |
|---|---|
| `PROCESS_INSTANCE` | `ELEMENT_ACTIVATING`, `ELEMENT_ACTIVATED`, `ELEMENT_COMPLETING`, `ELEMENT_COMPLETED`, `ELEMENT_TERMINATING`, `ELEMENT_TERMINATED`, `SEQUENCE_FLOW_TAKEN` |
| `PROCESS_INSTANCE_CREATION` | `CREATED` |
| `VARIABLE` | `CREATED`, `UPDATED` (tanpa record bila nilai identik) |
| `JOB` | `CREATED`, `COMPLETED`, `CANCELED` |
| `INCIDENT` | `CREATED`, `RESOLVED` |

## Incident

| Tipe | Kapan | Elemen berhenti di |
|---|---|---|
| `CONDITION_ERROR` | Tidak ada flow cocok tanpa default, kondisi bukan boolean (termasuk null), evaluasi gagal | Gateway, `ACTIVATING` |
| `EXTRACT_VALUE_ERROR` | `type`/`retries` task definition gagal dievaluasi atau tipe salah | Service task, `ACTIVATING` |
| `EXECUTION_LIMIT_EXCEEDED` | Lebih dari `ENGINE_MAX_RECORDS_PER_COMMAND` record dalam satu command | Elemen yang sedang diaktifkan |

## Test

| Test | Isi |
|---|---|
| `domain/processor_test.go`, `inclusive_test.go` | Skenario per baris matriks perilaku dan tiap tipe incident, dengan replay di setiap skenario |
| `domain/property_test.go` | Graf blok acak (`make prop PROP_ITER=…`, `PROP_SEED=…`) |
| `domain/parity_test.go` | 20 skenario dibandingkan dengan rekaman Camunda 8.8 (`make parity SUITE=execution [LIVE=1]`) |
| `domain/bench_test.go` | Throughput `Processor` (`make bench`) |
| `infrastructure/execution_integration_test.go` | End-to-end lewat Connect, 50 `CompleteJob` paralel, 200 create paralel, replay vs proyeksi, latensi (`make test-integration`) |

## Persistensi dan pemulihan (Fase 3)

| Fitur | Ringkasan |
|---|---|
| Outbox | Event `execution.*` ditulis dalam batch command; `aggregate_seq` = sequence record pemicu |
| Verify | Saat start: SQL cepat atas semua instance + replay dalam atas instance yang baru berubah; `engine verify --all` |
| Rebuild | `engine rebuild --instance K` (dari snapshot bila ada) atau `--full`; perbaikan otomatis saat start memakai mode penuh |
| Snapshot | State runtime setiap `ENGINE_SNAPSHOT_INTERVAL_RECORDS` record untuk instance aktif |
| Idempotensi | `request_id` pada create, complete job, cancel; klaim setelah row lock |
| Riwayat | `ListProcessInstanceRecords`, `ListElementInstances` berhalaman; detail dipotong di 1.000 elemen |
| Retensi | `ENGINE_HISTORY_RETENTION_DAYS` (default mati), `engine prune` |
| Fault point | Tag `faultinject`: `command.*`, `outbox.*`, `idempotency.after_claim`, `rebuild.mid_stream`, `snapshot.before_write` |
