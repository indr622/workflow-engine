package sdk

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	jobv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1"
)

// WorkerBuilder menyusun JobWorker. Buat dengan NewJobWorker.
type WorkerBuilder struct {
	client         *Client
	jobType        string
	handler        Handler
	name           string
	concurrency    int
	maxJobsActive  int
	timeout        time.Duration
	requestTimeout time.Duration
	pollInterval   time.Duration
	fetch          []string
	failureBackoff BackoffPolicy
	pollBackoff    BackoffPolicy
	log            *slog.Logger
}

// NewJobWorker memulai builder worker dengan default: concurrency 8, MaxJobsActive 32, lease 5 menit,
// long polling 10 s, jeda 100 ms saat batch kosong.
func NewJobWorker(c *Client) *WorkerBuilder {
	host, _ := os.Hostname()
	return &WorkerBuilder{
		client: c, name: fmt.Sprintf("%s-%d", host, os.Getpid()), concurrency: 8, maxJobsActive: 32,
		timeout: 5 * time.Minute, requestTimeout: 10 * time.Second, pollInterval: 100 * time.Millisecond,
		failureBackoff: BackoffPolicy{Initial: time.Second, Max: time.Minute, Multiplier: 2},
		pollBackoff:    BackoffPolicy{Initial: 100 * time.Millisecond, Max: 30 * time.Second, Multiplier: 2, Jitter: 0.2},
		log:            slog.Default(),
	}
}

// JobType menetapkan tipe job yang diaktifkan (wajib).
func (b *WorkerBuilder) JobType(t string) *WorkerBuilder { b.jobType = t; return b }

// Handler menetapkan handler job (wajib).
func (b *WorkerBuilder) Handler(h Handler) *WorkerBuilder { b.handler = h; return b }

// Name menetapkan nama worker (default hostname-pid).
func (b *WorkerBuilder) Name(n string) *WorkerBuilder { b.name = n; return b }

// Concurrency menetapkan jumlah goroutine handler.
func (b *WorkerBuilder) Concurrency(n int) *WorkerBuilder { b.concurrency = n; return b }

// MaxJobsActive membatasi job yang dipegang worker sekaligus (diaktifkan tetapi belum selesai).
func (b *WorkerBuilder) MaxJobsActive(n int) *WorkerBuilder { b.maxJobsActive = n; return b }

// Timeout menetapkan durasi lease; job kembali ke engine bila belum selesai sebelum habis.
func (b *WorkerBuilder) Timeout(d time.Duration) *WorkerBuilder { b.timeout = d; return b }

// RequestTimeout menetapkan batas long polling ActivateJobs; 0 mematikan long polling.
func (b *WorkerBuilder) RequestTimeout(d time.Duration) *WorkerBuilder {
	b.requestTimeout = d
	return b
}

// PollInterval menetapkan jeda setelah batch kosong.
func (b *WorkerBuilder) PollInterval(d time.Duration) *WorkerBuilder { b.pollInterval = d; return b }

// FetchVariables membatasi variabel yang dikirim engine.
func (b *WorkerBuilder) FetchVariables(names ...string) *WorkerBuilder { b.fetch = names; return b }

// FailureBackoff menetapkan backoff retry untuk error biasa dari handler, per sisa percobaan.
func (b *WorkerBuilder) FailureBackoff(p BackoffPolicy) *WorkerBuilder {
	b.failureBackoff = p
	return b
}

// PollBackoff menetapkan backoff saat ActivateJobs gagal (direset saat sukses).
func (b *WorkerBuilder) PollBackoff(p BackoffPolicy) *WorkerBuilder { b.pollBackoff = p; return b }

// Logger menetapkan logger worker.
func (b *WorkerBuilder) Logger(l *slog.Logger) *WorkerBuilder { b.log = l; return b }

// Build memvalidasi konfigurasi dan membuat JobWorker.
func (b *WorkerBuilder) Build() (*JobWorker, error) {
	switch {
	case b.client == nil:
		return nil, errors.New("sdk: worker needs a client")
	case b.jobType == "":
		return nil, errors.New("sdk: worker needs a job type")
	case b.handler == nil:
		return nil, errors.New("sdk: worker needs a handler")
	case b.concurrency <= 0 || b.maxJobsActive <= 0:
		return nil, errors.New("sdk: concurrency and max jobs active must be positive")
	}
	cfg := *b
	return &JobWorker{cfg: &cfg, slots: make(chan struct{}, cfg.maxJobsActive), jobs: make(chan *Job, cfg.maxJobsActive)}, nil
}

// JobWorker mengaktifkan job dengan long polling dan menjalankan handler secara konkuren.
type JobWorker struct {
	cfg *WorkerBuilder
	// slots berisi satu token per job yang sedang dipegang worker.
	slots chan struct{}
	jobs  chan *Job

	started    atomic.Bool
	stopPoll   context.CancelFunc
	cancelJobs context.CancelFunc
	pollDone   chan struct{}
	handlers   sync.WaitGroup
	inflight   sync.WaitGroup
	handled    atomic.Int64
}

// Start memulai polling dan pool handler; tidak blocking. ctx membatasi umur worker.
func (w *JobWorker) Start(ctx context.Context) error {
	if !w.started.CompareAndSwap(false, true) {
		return errors.New("sdk: worker already started")
	}
	pollCtx, stopPoll := context.WithCancel(ctx)
	jobCtx, cancelJobs := context.WithCancel(context.WithoutCancel(ctx))
	w.stopPoll, w.cancelJobs, w.pollDone = stopPoll, cancelJobs, make(chan struct{})
	for i := 0; i < w.cfg.concurrency; i++ {
		w.handlers.Add(1)
		go func() {
			defer w.handlers.Done()
			for job := range w.jobs {
				w.handle(jobCtx, job)
				<-w.slots
				w.inflight.Done()
			}
		}()
	}
	go w.poll(pollCtx)
	return nil
}

// Close menghentikan polling, menunggu handler aktif sampai ctx habis, lalu membatalkan context
// handler yang tersisa. Job yang tidak selesai kembali ke engine lewat timeout lease.
func (w *JobWorker) Close(ctx context.Context) error {
	if !w.started.Load() {
		return nil
	}
	w.stopPoll()
	<-w.pollDone
	done := make(chan struct{})
	go func() {
		w.inflight.Wait()
		close(done)
	}()
	var err error
	select {
	case <-done:
	case <-ctx.Done():
		err = ctx.Err()
	}
	w.cancelJobs()
	close(w.jobs)
	w.handlers.Wait()
	return err
}

// Handled mengembalikan jumlah job yang sudah diproses handler (sukses maupun gagal).
func (w *JobWorker) Handled() int64 { return w.handled.Load() }

func (w *JobWorker) poll(ctx context.Context) {
	defer close(w.pollDone)
	failures := 0
	for ctx.Err() == nil {
		free := cap(w.slots) - len(w.slots)
		if free == 0 {
			// Semua slot terpakai: tunggu satu selesai.
			select {
			case <-ctx.Done():
				return
			case w.slots <- struct{}{}:
				<-w.slots
			}
			continue
		}
		jobs, err := w.cfg.client.ActivateJobs(ctx, &jobv1.ActivateJobsRequest{
			Type: w.cfg.jobType, Worker: w.cfg.name, MaxJobsToActivate: int32(free), //nolint:gosec // dibatasi MaxJobsActive
			Timeout: durationpb.New(w.cfg.timeout), FetchVariable: w.cfg.fetch, RequestTimeout: durationpb.New(w.cfg.requestTimeout),
		})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			failures++
			w.cfg.log.Warn("activate jobs failed", slog.String("job_type", w.cfg.jobType), slog.Any("error", err))
			sleep(ctx, w.cfg.pollBackoff.Delay(failures))
			continue
		}
		failures = 0
		for _, job := range jobs {
			w.slots <- struct{}{}
			w.inflight.Add(1)
			w.jobs <- job
		}
		if len(jobs) == 0 && w.cfg.requestTimeout <= 0 {
			sleep(ctx, w.cfg.pollInterval)
		}
	}
}

func sleep(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// handle menjalankan handler lalu mengirim RPC sesuai hasilnya.
func (w *JobWorker) handle(ctx context.Context, job *Job) {
	defer w.handled.Add(1)
	res, herr := w.runHandler(ctx, job)
	if ctx.Err() != nil {
		return
	}
	// RPC hasil dikirim dengan context yang tidak ikut batal saat shutdown dimulai.
	rpcCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	var err error
	var failErr *FailError
	var bpmnErr *BPMNErrorResult
	switch {
	case herr == nil:
		err = w.cfg.client.CompleteJob(rpcCtx, job.Key, res.Variables)
	case errors.As(herr, &bpmnErr):
		err = w.cfg.client.ThrowError(rpcCtx, job.Key, bpmnErr.Code, bpmnErr.Message, bpmnErr.Variables)
	case errors.As(herr, &failErr):
		retries := max(job.Retries-1, 0)
		if failErr.Retries != nil {
			retries = *failErr.Retries
		}
		backoff := w.cfg.failureBackoff.Delay(int(max(job.Retries-retries, 1)))
		if failErr.Backoff != nil {
			backoff = *failErr.Backoff
		}
		err = w.cfg.client.FailJob(rpcCtx, job.Key, Failure{Retries: retries, ErrorMessage: failErr.Message, RetryBackoff: backoff, Variables: failErr.Variables})
	default:
		retries := max(job.Retries-1, 0)
		err = w.cfg.client.FailJob(rpcCtx, job.Key, Failure{
			Retries: retries, ErrorMessage: herr.Error(), RetryBackoff: w.cfg.failureBackoff.Delay(1),
		})
	}
	if err != nil {
		w.cfg.log.Warn("report job result failed", slog.String("job_key", job.Key), slog.String("job_type", job.Type), slog.Any("error", err))
	}
}

// runHandler menjalankan handler dan mengubah panic menjadi error biasa (job digagalkan).
func (w *JobWorker) runHandler(ctx context.Context, job *Job) (res Result, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("handler panic: %v", p)
		}
	}()
	return w.cfg.handler(ctx, job)
}
