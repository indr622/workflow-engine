package sdk_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"

	commonv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/common/v1"
	jobv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1/jobv1connect"
	"github.com/indr622/workflow-engine/libs/go/sdk"
)

// scripted adalah JobService palsu yang gagal UNAVAILABLE beberapa kali, lalu merekam header.
type scripted struct {
	jobv1connect.UnimplementedJobServiceHandler
	unavailable atomic.Int32
	calls       atomic.Int32
	auth        atomic.Value
	agent       atomic.Value
	reason      string
}

func (s *scripted) record(h http.Header) error {
	s.calls.Add(1)
	s.auth.Store(h.Get("Authorization"))
	s.agent.Store(h.Get("User-Agent"))
	if s.unavailable.Add(-1) >= 0 {
		return connect.NewError(connect.CodeUnavailable, errors.New("shutting down"))
	}
	if s.reason != "" {
		cerr := connect.NewError(connect.CodeFailedPrecondition, errors.New("job is not active"))
		d, _ := connect.NewErrorDetail(&commonv1.ErrorInfo{Reason: s.reason, Domain: "workflow-engine", Metadata: map[string]string{"state": "FAILED"}})
		cerr.AddDetail(d)
		return cerr
	}
	return nil
}

func (s *scripted) ActivateJobs(_ context.Context, req *connect.Request[jobv1.ActivateJobsRequest]) (*connect.Response[jobv1.ActivateJobsResponse], error) {
	if err := s.record(req.Header()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&jobv1.ActivateJobsResponse{Jobs: []*jobv1.ActivatedJob{{JobKey: "k", Type: req.Msg.GetType(), Variables: `{}`}}}), nil
}

func (s *scripted) CompleteJob(_ context.Context, req *connect.Request[jobv1.CompleteJobRequest]) (*connect.Response[jobv1.CompleteJobResponse], error) {
	if err := s.record(req.Header()); err != nil {
		return nil, err
	}
	return connect.NewResponse(&jobv1.CompleteJobResponse{}), nil
}

func serve(t *testing.T, s *scripted) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle(jobv1connect.NewJobServiceHandler(s))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

var fastRetry = sdk.RetryPolicy{MaxAttempts: 4, Backoff: sdk.BackoffPolicy{Initial: time.Millisecond, Max: 5 * time.Millisecond, Multiplier: 2}}

func TestRetryOnlyForSafeCalls(t *testing.T) {
	s := &scripted{}
	c, err := sdk.NewClient(serve(t, s), sdk.WithRetry(fastRetry))
	if err != nil {
		t.Fatal(err)
	}
	s.unavailable.Store(2)
	jobs, err := c.ActivateJobs(context.Background(), &jobv1.ActivateJobsRequest{Type: "t"})
	if err != nil || len(jobs) != 1 || s.calls.Load() != 3 {
		t.Fatalf("activate after 2 unavailable: jobs=%d err=%v calls=%d", len(jobs), err, s.calls.Load())
	}
	// CompleteJob tanpa request_id tidak aman diulang.
	s.calls.Store(0)
	s.unavailable.Store(1)
	if err := c.CompleteJob(context.Background(), "k", nil); connect.CodeOf(err) != connect.CodeUnavailable || s.calls.Load() != 1 {
		t.Fatalf("complete: err=%v calls=%d", err, s.calls.Load())
	}
	// Dengan request_id, mutasi idempoten di server sehingga boleh diulang.
	s.calls.Store(0)
	s.unavailable.Store(1)
	_, err = c.Jobs().CompleteJob(context.Background(), connect.NewRequest(&jobv1.CompleteJobRequest{JobKey: "k", RequestId: "r-1"}))
	if err != nil || s.calls.Load() != 2 {
		t.Fatalf("complete with request_id: err=%v calls=%d", err, s.calls.Load())
	}
}

func TestTokenSourceAndUserAgent(t *testing.T) {
	s := &scripted{}
	c, _ := sdk.NewClient(serve(t, s), sdk.WithTokenSource(func(context.Context) (string, error) { return "secret", nil }),
		sdk.WithUserAgent("my-worker/1"), sdk.WithJSON())
	if err := c.CompleteJob(context.Background(), "k", map[string]int{"a": 1}); err != nil {
		t.Fatal(err)
	}
	if s.auth.Load() != "Bearer secret" || s.agent.Load() != "my-worker/1" {
		t.Fatalf("auth=%v agent=%v", s.auth.Load(), s.agent.Load())
	}
	failing, _ := sdk.NewClient(serve(t, &scripted{}), sdk.WithTokenSource(func(context.Context) (string, error) {
		return "", errors.New("no token")
	}))
	if err := failing.CompleteJob(context.Background(), "k", nil); connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("token error = %v", err)
	}
}

func TestErrorsCarryReason(t *testing.T) {
	s := &scripted{reason: "JOB_NOT_ACTIVE"}
	c, _ := sdk.NewClient(serve(t, s))
	err := c.CompleteJob(context.Background(), "k", nil)
	if !errors.Is(err, sdk.ErrJobNotActive) || errors.Is(err, sdk.ErrJobNotFound) {
		t.Fatalf("errors.Is mismatch: %v", err)
	}
	var e *sdk.Error
	if !errors.As(err, &e) || e.Code != connect.CodeFailedPrecondition || e.Metadata["state"] != "FAILED" {
		t.Fatalf("error = %#v", err)
	}
	if info := sdk.AsErrorInfo(err); info.GetReason() != "JOB_NOT_ACTIVE" {
		t.Fatalf("AsErrorInfo = %v", info)
	}
	if sdk.AsErrorInfo(errors.New("plain")) != nil {
		t.Fatal("plain error has no ErrorInfo")
	}
}

func TestMarshalVariables(t *testing.T) {
	type payload struct {
		A int `json:"a"`
	}
	for _, tc := range []struct {
		in   any
		want string
	}{
		{nil, ""}, {"", ""}, {`{"a":1}`, `{"a":1}`}, {[]byte(` {"a":1} `), `{"a":1}`},
		{json.RawMessage(`{"a":1}`), `{"a":1}`}, {map[string]int{"a": 1}, `{"a":1}`}, {payload{A: 1}, `{"a":1}`},
		{sdk.Variables(`{"a":1}`), `{"a":1}`},
	} {
		got, err := sdk.MarshalVariables(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("MarshalVariables(%#v) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	for _, bad := range []any{"[1]", 42, make(chan int), "{broken"} {
		if _, err := sdk.MarshalVariables(bad); !errors.Is(err, sdk.ErrInvalidVariables) {
			t.Errorf("MarshalVariables(%#v) err = %v", bad, err)
		}
	}
}

func TestVariablesHelpers(t *testing.T) {
	v := sdk.Variables(`{"s":"x","n":9007199254740993,"f":1.5,"b":true}`)
	if s, err := v.String("s"); err != nil || s != "x" {
		t.Fatal(s, err)
	}
	if n, err := v.Int64("n"); err != nil || n != 9007199254740993 {
		t.Fatal(n, err)
	}
	if f, err := v.Float64("f"); err != nil || f != 1.5 {
		t.Fatal(f, err)
	}
	if b, err := v.Bool("b"); err != nil || !b {
		t.Fatal(b, err)
	}
	if _, err := v.String("missing"); err == nil {
		t.Fatal("missing variable must fail")
	}
	if _, err := v.String("n"); err == nil {
		t.Fatal("type mismatch must fail")
	}
	var m map[string]any
	if err := v.Unmarshal(&m); err != nil || m["n"].(json.Number).String() != "9007199254740993" {
		t.Fatal(m, err)
	}
	if err := sdk.Variables(nil).Unmarshal(&m); err != nil {
		t.Fatal(err)
	}
}

func TestBackoffDelay(t *testing.T) {
	b := sdk.BackoffPolicy{Initial: 100 * time.Millisecond, Max: time.Second, Multiplier: 2}
	for attempt, want := range map[int]time.Duration{1: 100 * time.Millisecond, 2: 200 * time.Millisecond, 4: 800 * time.Millisecond, 10: time.Second} {
		if got := b.Delay(attempt); got != want {
			t.Errorf("Delay(%d) = %s, want %s", attempt, got, want)
		}
	}
	jittered := sdk.BackoffPolicy{Initial: 100 * time.Millisecond, Max: time.Second, Multiplier: 2, Jitter: 0.2}
	if d := jittered.Delay(1); d < 80*time.Millisecond || d > 120*time.Millisecond {
		t.Errorf("jittered delay = %s", d)
	}
	if (sdk.BackoffPolicy{}).Delay(3) != 0 {
		t.Error("zero policy must not wait")
	}
}
