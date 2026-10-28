# sdk

SDK Go untuk workflow engine: client Connect untuk semua service dan `JobWorker` yang
menyembunyikan protokol job worker (long polling, concurrency, retry, graceful shutdown).

- Import path: `github.com/indr622/workflow-engine/libs/go/sdk` (module root repo, ADR-0002).
- Dependensi hanya `libs/go/api`, `connectrpc.com/connect`, `google.golang.org/protobuf`, dan
  stdlib (dijaga `deps_test.go`), sehingga kelak bisa dipisah menjadi module sendiri.
- API masih `v0` sampai Fase 6; perubahan dicatat di bawah.

## Quickstart

```go
c, err := sdk.NewClient("http://localhost:8080")
if err != nil {
	log.Fatal(err)
}

w, err := sdk.NewJobWorker(c).
	JobType("charge-payment").
	Handler(func(ctx context.Context, job *sdk.Job) (sdk.Result, error) {
		var order struct{ Amount float64 `json:"amount"` }
		if err := job.Variables.Unmarshal(&order); err != nil {
			return sdk.Result{}, err // FailJob dengan retries - 1
		}
		if order.Amount > 1000 {
			return sdk.Result{}, sdk.BPMNError("LIMIT_EXCEEDED", "amount too high")
		}
		return sdk.Result{Variables: map[string]any{"charged": true}}, nil
	}).
	Build()
if err != nil {
	log.Fatal(err)
}
_ = w.Start(ctx)
defer w.Close(context.Background())

inst, err := c.CreateInstance(ctx, sdk.Latest("order-process"), map[string]any{"amount": 120})
```

## Hasil handler

| Handler mengembalikan | RPC yang dikirim |
|---|---|
| `(sdk.Result{Variables: v}, nil)` | `CompleteJob(v)` |
| `sdk.Fail(msg, sdk.WithRetries(n), sdk.WithBackoff(d))` | `FailJob(n, d, msg)`; tanpa opsi: retries `job.Retries - 1`, backoff kebijakan worker |
| `sdk.BPMNError(code, msg)` | `ThrowError(code, msg)` |
| error lain atau panic | `FailJob(job.Retries - 1, FailureBackoff, err.Error())` |
| context dibatalkan saat `Close` | tidak ada RPC; job kembali lewat timeout lease engine |

Handler sebaiknya idempoten: lease bersifat at-least-once (ADR-0020), jadi job yang lease-nya
habis bisa diaktifkan worker lain.

## Opsi worker

| Opsi | Default | Arti |
|---|---|---|
| `JobType` | wajib | Tipe job yang diaktifkan |
| `Handler` | wajib | Fungsi pemroses job |
| `Name` | `hostname-pid` | Nama worker di lease dan log engine |
| `Concurrency` | 8 | Goroutine handler |
| `MaxJobsActive` | 32 | Job yang dipegang sekaligus (batas `max_jobs_to_activate`) |
| `Timeout` | 5 menit | Durasi lease |
| `RequestTimeout` | 10 s | Long polling `ActivateJobs`; 0 mematikan long polling |
| `PollInterval` | 100 ms | Jeda setelah batch kosong tanpa long polling |
| `FetchVariables` | semua | Variabel yang dikirim engine |
| `FailureBackoff` | 1 s → 1 menit | Backoff retry untuk error biasa |
| `PollBackoff` | 100 ms → 30 s, jitter | Backoff saat `ActivateJobs` gagal |

## Opsi client

`WithHTTPClient`, `WithTokenSource` (header `Authorization: Bearer`, diverifikasi mulai Fase 8),
`WithRetry` (default 5 percobaan untuk `UNAVAILABLE`, hanya pada panggilan yang aman diulang:
query, `ActivateJobs`, `PublishMessage` dengan `message_id`, dan mutasi yang membawa `request_id`),
`WithUserAgent`, `WithJSON`.

## Error

Error engine menjadi `*sdk.Error` dengan `Reason` stabil dari `workflow.common.v1.ErrorInfo`:

```go
if errors.Is(err, sdk.ErrJobNotActive) { /* job sudah selesai atau gagal */ }
```

## Test

`sdktest.NewFakeEngine(t)` menjalankan `JobService` in-memory untuk menguji worker tanpa engine;
`sdktest.RunHandler(t, h, job)` memanggil handler langsung. Test e2e dengan engine nyata ada di
`apps/engine/internal/e2e` (`-tags=integration`).

## Perubahan

- 0.4.0 (Fase 4): rilis pertama.
