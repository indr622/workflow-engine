package sdk_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/indr622/workflow-engine/libs/go/sdk"
	"github.com/indr622/workflow-engine/libs/go/sdk/sdktest"
)

func newWorker(t *testing.T, f *sdktest.FakeEngine, jobType string, h sdk.Handler, tune func(*sdk.WorkerBuilder)) *sdk.JobWorker {
	t.Helper()
	c, err := sdk.NewClient(f.URL())
	if err != nil {
		t.Fatal(err)
	}
	b := sdk.NewJobWorker(c).JobType(jobType).Handler(h).Name("test").RequestTimeout(200 * time.Millisecond).
		FailureBackoff(sdk.BackoffPolicy{Initial: 10 * time.Millisecond, Max: 100 * time.Millisecond, Multiplier: 2})
	if tune != nil {
		tune(b)
	}
	w, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = w.Close(ctx)
	})
	return w
}

type order struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
}

func TestWorkerCompletesJobsWithVariables(t *testing.T) {
	f := sdktest.NewFakeEngine(t)
	newWorker(t, f, "ship", func(_ context.Context, job *sdk.Job) (sdk.Result, error) {
		var o order
		if err := job.Variables.Unmarshal(&o); err != nil {
			return sdk.Result{}, err
		}
		o.Status = "shipped-" + job.CustomHeaders["carrier"]
		return sdk.Result{Variables: o}, nil
	}, nil)
	var keys []string
	for i := 0; i < 20; i++ {
		keys = append(keys, f.AddJob("ship", `{"id": 9007199254740993, "status": "new"}`, 3, map[string]string{"carrier": "dhl"}))
	}
	for _, k := range keys {
		j := f.WaitFor(t, k, sdktest.Completed, 5*time.Second)
		if j.CompletedVariables != `{"id":9007199254740993,"status":"shipped-dhl"}` {
			t.Fatalf("completed variables = %s", j.CompletedVariables)
		}
	}
}

func TestWorkerFailsWithDefaultsOnPlainError(t *testing.T) {
	f := sdktest.NewFakeEngine(t)
	var calls atomic.Int32
	newWorker(t, f, "flaky", func(context.Context, *sdk.Job) (sdk.Result, error) {
		if calls.Add(1) == 1 {
			return sdk.Result{}, errors.New("upstream down")
		}
		return sdk.Result{}, nil
	}, nil)
	k := f.AddJob("flaky", "", 3, nil)
	j := f.WaitFor(t, k, sdktest.Completed, 5*time.Second)
	if j.Retries != 2 || j.ErrorMessage != "upstream down" || j.LastBackoff <= 0 || j.Activations != 2 {
		t.Fatalf("job = %+v", j)
	}
}

func TestWorkerExplicitFailAndBPMNError(t *testing.T) {
	f := sdktest.NewFakeEngine(t)
	newWorker(t, f, "pay", func(_ context.Context, job *sdk.Job) (sdk.Result, error) {
		mode, _ := job.Variables.String("mode")
		switch mode {
		case "fail":
			return sdk.Result{}, sdk.Fail("card declined", sdk.WithRetries(0), sdk.WithBackoff(time.Second))
		case "bpmn":
			return sdk.Result{}, sdk.BPMNError("NO_FUNDS", "balance too low").WithVariables(map[string]any{"balance": 1})
		default:
			panic("unexpected mode")
		}
	}, nil)
	failed := f.AddJob("pay", `{"mode":"fail"}`, 3, nil)
	thrown := f.AddJob("pay", `{"mode":"bpmn"}`, 3, nil)
	panicked := f.AddJob("pay", `{"mode":"other"}`, 1, nil)
	if j := f.WaitFor(t, failed, sdktest.Failed, 5*time.Second); j.ErrorMessage != "card declined" || j.Retries != 0 {
		t.Fatalf("failed job = %+v", j)
	}
	if j := f.WaitFor(t, thrown, sdktest.ErrorThrown, 5*time.Second); j.ErrorCode != "NO_FUNDS" {
		t.Fatalf("thrown job = %+v", j)
	}
	if j := f.WaitFor(t, panicked, sdktest.Failed, 5*time.Second); !strings.Contains(j.ErrorMessage, "panic") {
		t.Fatalf("panicked job = %+v", j)
	}
}

func TestWorkerRespectsMaxJobsActiveAndConcurrency(t *testing.T) {
	f := sdktest.NewFakeEngine(t)
	var active, peak atomic.Int32
	release := make(chan struct{})
	newWorker(t, f, "slow", func(context.Context, *sdk.Job) (sdk.Result, error) {
		n := active.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		<-release
		active.Add(-1)
		return sdk.Result{}, nil
	}, func(b *sdk.WorkerBuilder) { b.Concurrency(3).MaxJobsActive(4) })
	var keys []string
	for i := 0; i < 10; i++ {
		keys = append(keys, f.AddJob("slow", "", 1, nil))
	}
	time.Sleep(300 * time.Millisecond)
	activated := 0
	for _, k := range keys {
		if f.Job(k).State == sdktest.Activated {
			activated++
		}
	}
	if activated > 4 || peak.Load() > 3 {
		t.Fatalf("activated=%d peak handlers=%d, want ≤ 4 and ≤ 3", activated, peak.Load())
	}
	close(release)
	for _, k := range keys {
		f.WaitFor(t, k, sdktest.Completed, 5*time.Second)
	}
}

func TestCloseWaitsForHandlersThenCancels(t *testing.T) {
	f := sdktest.NewFakeEngine(t)
	c, _ := sdk.NewClient(f.URL())
	started := make(chan struct{}, 2)
	w, err := sdk.NewJobWorker(c).JobType("long").RequestTimeout(100 * time.Millisecond).
		Handler(func(ctx context.Context, job *sdk.Job) (sdk.Result, error) {
			started <- struct{}{}
			if v, _ := job.Variables.String("kind"); v == "quick" {
				time.Sleep(100 * time.Millisecond)
				return sdk.Result{}, nil
			}
			<-ctx.Done()
			return sdk.Result{}, ctx.Err()
		}).Build()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	quick := f.AddJob("long", `{"kind":"quick"}`, 1, nil)
	stuck := f.AddJob("long", `{"kind":"stuck"}`, 1, nil)
	<-started
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := w.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("close = %v, want deadline exceeded for the stuck handler", err)
	}
	if f.Job(quick).State != sdktest.Completed {
		t.Fatalf("quick job = %s, want COMPLETED before close returned", f.Job(quick).State)
	}
	// Handler yang dibatalkan saat shutdown tidak mengirim RPC: job tetap dipegang sampai lease habis.
	if f.Job(stuck).State != sdktest.Activated {
		t.Fatalf("stuck job = %s, want ACTIVATED", f.Job(stuck).State)
	}
	if w.Handled() != 2 {
		t.Fatalf("handled = %d", w.Handled())
	}
}

func TestBuilderValidation(t *testing.T) {
	c, _ := sdk.NewClient("http://localhost:1")
	h := func(context.Context, *sdk.Job) (sdk.Result, error) { return sdk.Result{}, nil }
	for name, b := range map[string]*sdk.WorkerBuilder{
		"no type":     sdk.NewJobWorker(c).Handler(h),
		"no handler":  sdk.NewJobWorker(c).JobType("t"),
		"no client":   sdk.NewJobWorker(nil).JobType("t").Handler(h),
		"bad workers": sdk.NewJobWorker(c).JobType("t").Handler(h).Concurrency(0),
	} {
		if _, err := b.Build(); err == nil {
			t.Errorf("%s: build must fail", name)
		}
	}
	if _, err := sdk.NewClient("localhost:8080"); err == nil {
		t.Error("base URL without scheme must be rejected")
	}
}

func TestPollBackoffWhenEngineIsDown(t *testing.T) {
	c, _ := sdk.NewClient("http://127.0.0.1:1", sdk.WithRetry(sdk.RetryPolicy{}))
	w, err := sdk.NewJobWorker(c).JobType("t").Handler(func(context.Context, *sdk.Job) (sdk.Result, error) {
		return sdk.Result{}, nil
	}).PollBackoff(sdk.BackoffPolicy{Initial: 20 * time.Millisecond, Max: 50 * time.Millisecond, Multiplier: 2}).Build()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := w.Start(context.Background()); err == nil {
		t.Fatal("second Start must fail")
	}
	time.Sleep(200 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := w.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestRunHandler(t *testing.T) {
	h := func(_ context.Context, job *sdk.Job) (sdk.Result, error) {
		n, err := job.Variables.Int64("n")
		if err != nil {
			return sdk.Result{}, err
		}
		if n < 0 {
			return sdk.Result{}, sdk.Fail("negative", sdk.WithRetries(0))
		}
		return sdk.Result{Variables: map[string]int64{"double": n * 2}}, nil
	}
	if out := sdktest.RunHandler(t, h, &sdk.Job{Variables: sdk.Variables(`{"n": 21}`)}); !out.Completed ||
		out.Variables.(map[string]int64)["double"] != 42 {
		t.Fatalf("outcome = %+v", out)
	}
	if out := sdktest.RunHandler(t, h, &sdk.Job{Variables: sdk.Variables(`{"n": -1}`)}); !out.Failed || out.Retries == nil || *out.Retries != 0 {
		t.Fatalf("outcome = %+v", out)
	}
	if out := sdktest.RunHandler(t, h, &sdk.Job{}); !out.Failed || out.Err == nil {
		t.Fatalf("outcome = %+v", out)
	}
}
