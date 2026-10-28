# worker-example

Demo end-to-end SDK Go (`libs/go/sdk`): men-deploy `processes/order-process.bpmn`
(validate-order → charge-payment → ship-order), menjalankan tiga job worker, membuat N instance
dengan `CreateInstanceWithResult`, lalu mencetak ringkasan. `charge-payment` sengaja gagal sekali
per instance sehingga demo memperlihatkan `FailJob` dengan retry.

## Quickstart

```bash
make up          # Postgres dev
make dev-engine  # engine di http://localhost:8080 (terminal terpisah)
make demo        # INSTANCES=10 ENGINE_URL=http://localhost:8080
```

Keluaran yang diharapkan (exit code 0):

```
10 instances completed, 10 retries, in 298ms
```

## Container

Profil Compose `demo` menjalankan worker sebagai container terhadap engine di host:

```bash
make up PROFILES=demo
```

Container men-deploy model, menjalankan 10 instance, lalu tetap melayani job (`--serve`).
`DEMO_ENGINE_URL` mengganti alamat engine (default `http://host.docker.internal:8080`).

## Flag

| Flag | Default | Arti |
|---|---|---|
| `--engine-url` | `$ENGINE_URL` atau `http://localhost:8080` | Base URL engine |
| `--instances` | 10 | Jumlah instance demo |
| `--timeout` | 30s | Batas waktu seluruh demo |
| `--serve` | false | Tetap melayani job setelah demo selesai |

Handler di `internal/handlers` diuji tanpa engine dengan `sdktest.RunHandler`.
