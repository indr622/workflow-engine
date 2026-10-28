package application_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	executionapi "github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/job/application"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/job/domain"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/job/infrastructure"
)

// fakeJobs mengembalikan job dari antrean yang diisi test; hanya ActivateJobs yang dipakai.
type fakeJobs struct {
	executionapi.Jobs
	mu      sync.Mutex
	queue   []executionapi.ActivatedJob
	calls   atomic.Int32
	lastQ   executionapi.ActivationQuery
	onQuery func()
}

func (f *fakeJobs) ActivateJobs(_ context.Context, q executionapi.ActivationQuery) ([]executionapi.ActivatedJob, error) {
	f.calls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastQ = q
	n := min(int(q.MaxJobs), len(f.queue))
	out := f.queue[:n]
	f.queue = f.queue[n:]
	// onQuery berjalan setelah hasil kueri ditentukan: job yang ditambahkan di sini baru terlihat
	// pada kueri berikutnya, meniru job yang dibuat di antara kueri dan tunggu.
	if f.onQuery != nil {
		f.onQuery()
	}
	return out, nil
}

func (f *fakeJobs) add(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := 0; i < n; i++ {
		f.queue = append(f.queue, executionapi.ActivatedJob{Key: "k", Type: "t"})
	}
}

func newActivate(jobs *fakeJobs, poll time.Duration, stopping chan struct{}) (*application.ActivateJobs, *infrastructure.Notifier) {
	n := infrastructure.NewNotifier(nil)
	return application.NewActivateJobs(jobs, n, domain.Limits{PollInterval: poll, MaxLongPolls: 2}, stopping), n
}

func TestImmediateWhenNoRequestTimeout(t *testing.T) {
	jobs := &fakeJobs{}
	a, _ := newActivate(jobs, time.Hour, nil)
	got, err := a.Execute(context.Background(), domain.ActivationRequest{Type: "t"})
	if err != nil || len(got) != 0 || jobs.calls.Load() != 1 {
		t.Fatalf("got=%d err=%v calls=%d", len(got), err, jobs.calls.Load())
	}
	if jobs.lastQ.MaxJobs != 32 || jobs.lastQ.Timeout != 5*time.Minute || jobs.lastQ.Worker != "default" {
		t.Fatalf("defaults not applied: %+v", jobs.lastQ)
	}
}

func TestWokenByNotifier(t *testing.T) {
	jobs := &fakeJobs{}
	a, n := newActivate(jobs, time.Hour, nil)
	go func() {
		for jobs.calls.Load() == 0 {
			time.Sleep(5 * time.Millisecond)
		}
		jobs.add(3)
		n.Notify("t")
	}()
	start := time.Now()
	got, err := a.Execute(context.Background(), domain.ActivationRequest{Type: "t", RequestTimeout: 10 * time.Second})
	if err != nil || len(got) != 3 {
		t.Fatalf("got=%d err=%v", len(got), err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("notifier wake took %s", d)
	}
}

func TestNotificationBetweenQueryAndWaitIsNotLost(t *testing.T) {
	jobs := &fakeJobs{}
	a, n := newActivate(jobs, time.Hour, nil)
	// Job dibuat dan notifikasi dikirim setelah kueri pertama kosong tetapi sebelum menunggu; polling
	// cadangan sengaja satu jam agar hanya notifikasi yang bisa membangunkan.
	var once sync.Once
	jobs.onQuery = func() {
		once.Do(func() {
			jobs.queue = append(jobs.queue, executionapi.ActivatedJob{Key: "late"})
			n.Notify("t")
		})
	}
	got, err := a.Execute(context.Background(), domain.ActivationRequest{Type: "t", RequestTimeout: 5 * time.Second})
	if err != nil || len(got) != 1 {
		t.Fatalf("got=%d err=%v", len(got), err)
	}
}

func TestWokenByPollTicker(t *testing.T) {
	jobs := &fakeJobs{}
	a, _ := newActivate(jobs, 50*time.Millisecond, nil)
	time.AfterFunc(100*time.Millisecond, func() { jobs.add(1) })
	got, err := a.Execute(context.Background(), domain.ActivationRequest{Type: "t", RequestTimeout: 5 * time.Second})
	if err != nil || len(got) != 1 {
		t.Fatalf("got=%d err=%v", len(got), err)
	}
}

func TestRequestTimeoutReturnsEmpty(t *testing.T) {
	jobs := &fakeJobs{}
	a, _ := newActivate(jobs, time.Hour, nil)
	start := time.Now()
	got, err := a.Execute(context.Background(), domain.ActivationRequest{Type: "t", RequestTimeout: 150 * time.Millisecond})
	if err != nil || len(got) != 0 {
		t.Fatalf("got=%d err=%v", len(got), err)
	}
	if d := time.Since(start); d < 150*time.Millisecond || d > 2*time.Second {
		t.Fatalf("returned after %s", d)
	}
}

func TestCanceledContextStopsWithoutMoreQueries(t *testing.T) {
	jobs := &fakeJobs{}
	a, _ := newActivate(jobs, time.Hour, nil)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	got, err := a.Execute(ctx, domain.ActivationRequest{Type: "t", RequestTimeout: 10 * time.Second})
	if err != nil || len(got) != 0 || jobs.calls.Load() != 1 {
		t.Fatalf("got=%d err=%v calls=%d", len(got), err, jobs.calls.Load())
	}
}

func TestShutdownAnswersLongPollsImmediately(t *testing.T) {
	jobs := &fakeJobs{}
	stopping := make(chan struct{})
	a, _ := newActivate(jobs, time.Hour, stopping)
	time.AfterFunc(50*time.Millisecond, func() { close(stopping) })
	start := time.Now()
	if _, err := a.Execute(context.Background(), domain.ActivationRequest{Type: "t", RequestTimeout: 30 * time.Second}); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("shutdown took %s", d)
	}
}

func TestLongPollLimitFallsBackToSingleQuery(t *testing.T) {
	jobs := &fakeJobs{}
	a, _ := newActivate(jobs, time.Hour, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = a.Execute(ctx, domain.ActivationRequest{Type: "t", RequestTimeout: 30 * time.Second})
		}()
	}
	for a.LongPollsActive() < 2 {
		time.Sleep(5 * time.Millisecond)
	}
	start := time.Now()
	if _, err := a.Execute(context.Background(), domain.ActivationRequest{Type: "t", RequestTimeout: 30 * time.Second}); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("third request waited %s although the long poll limit is 2", d)
	}
	cancel()
	wg.Wait()
}

func TestClamp(t *testing.T) {
	l := domain.DefaultLimits()
	r := domain.ActivationRequest{MaxJobs: 5000, Timeout: time.Millisecond, RequestTimeout: time.Hour}.Clamp(l)
	if r.MaxJobs != 1000 || r.Timeout != time.Second || r.RequestTimeout != 55*time.Second || r.Worker != "default" {
		t.Fatalf("clamp = %+v", r)
	}
	if got := l.ClampTimeout(48 * time.Hour); got != 24*time.Hour {
		t.Fatalf("ClampTimeout = %s", got)
	}
	if got := l.ClampBackoff(-time.Second); got != 0 {
		t.Fatalf("ClampBackoff = %s", got)
	}
}
