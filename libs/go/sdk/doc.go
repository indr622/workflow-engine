// Package sdk adalah client Go untuk workflow engine: operasi deploy, process instance, job,
// incident, dan message, serta JobWorker yang menyembunyikan protokol worker (long polling,
// concurrency, retry, graceful shutdown).
//
// SDK hanya bergantung pada libs/go/api, connectrpc.com/connect, google.golang.org/protobuf, dan
// stdlib (dijaga deps_test.go) agar kelak bisa dipisah menjadi module sendiri. API masih v0
// sampai Fase 6.
//
// Handler worker sebaiknya idempoten: lease job bersifat at-least-once, sehingga job yang lease-nya
// kedaluwarsa bisa diaktifkan worker lain (ADR-0020).
package sdk
