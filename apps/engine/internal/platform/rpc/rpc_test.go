package rpc_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/indr622/workflow-engine/apps/engine/internal/platform/rpc"
	jobv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1/jobv1connect"
)

const validKey = "01a18350-a6b3-7458-8513-c530e8458d56"

// stubJobs memakai JobService sebagai service uji interceptor.
type stubJobs struct {
	jobv1connect.UnimplementedJobServiceHandler
	principal   chan rpc.Principal
	deadlineIn  chan time.Duration
	failWithErr error
}

func (s *stubJobs) CompleteJob(ctx context.Context, req *connect.Request[jobv1.CompleteJobRequest]) (*connect.Response[jobv1.CompleteJobResponse], error) {
	switch req.Msg.GetVariables() {
	case "panic":
		panic("boom")
	case "block":
		<-ctx.Done()
		return nil, connect.NewError(connect.CodeDeadlineExceeded, ctx.Err())
	}
	s.principal <- rpc.FromContext(ctx)
	return connect.NewResponse(&jobv1.CompleteJobResponse{}), s.failWithErr
}

func (s *stubJobs) ActivateJobs(ctx context.Context, _ *connect.Request[jobv1.ActivateJobsRequest]) (*connect.Response[jobv1.ActivateJobsResponse], error) {
	d, _ := ctx.Deadline()
	s.deadlineIn <- time.Until(d)
	return connect.NewResponse(&jobv1.ActivateJobsResponse{}), nil
}

func newServer(t *testing.T, defaultTimeout time.Duration) (jobv1connect.JobServiceClient, *stubJobs) {
	t.Helper()
	opts, err := rpc.HandlerOptions(rpc.Config{Log: slog.New(slog.NewTextHandler(io.Discard, nil)), DefaultTimeout: defaultTimeout})
	if err != nil {
		t.Fatal(err)
	}
	stub := &stubJobs{principal: make(chan rpc.Principal, 1), deadlineIn: make(chan time.Duration, 1)}
	mux := http.NewServeMux()
	mux.Handle(jobv1connect.NewJobServiceHandler(stub, opts...))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return jobv1connect.NewJobServiceClient(srv.Client(), srv.URL), stub
}

func TestValidationRejectsWithErrorInfo(t *testing.T) {
	client, _ := newServer(t, 0)
	_, err := client.CompleteJob(context.Background(), connect.NewRequest(&jobv1.CompleteJobRequest{JobKey: "not-a-uuid"}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
	info := rpc.ErrorInfoOf(err)
	if info == nil || info.GetReason() != rpc.ReasonRequestInvalid || info.GetDomain() != rpc.Domain ||
		!strings.Contains(info.GetMetadata()["violations"], "job_key") {
		t.Fatalf("error info = %v", info)
	}
}

func TestAnonymousPrincipalWithAndWithoutToken(t *testing.T) {
	client, stub := newServer(t, 0)
	for _, token := range []string{"", "Bearer anything"} {
		req := connect.NewRequest(&jobv1.CompleteJobRequest{JobKey: validKey})
		if token != "" {
			req.Header().Set("Authorization", token)
		}
		if _, err := client.CompleteJob(context.Background(), req); err != nil {
			t.Fatalf("token %q rejected: %v", token, err)
		}
		if p := <-stub.principal; p != rpc.AnonymousPrincipal {
			t.Fatalf("principal = %+v", p)
		}
	}
}

func TestPanicBecomesInternal(t *testing.T) {
	client, _ := newServer(t, 0)
	_, err := client.CompleteJob(context.Background(), connect.NewRequest(&jobv1.CompleteJobRequest{JobKey: validKey, Variables: "panic"}))
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Fatalf("code = %v, want Internal", connect.CodeOf(err))
	}
}

func TestDefaultTimeoutCutsBlockedHandler(t *testing.T) {
	client, _ := newServer(t, 200*time.Millisecond)
	start := time.Now()
	_, err := client.CompleteJob(context.Background(), connect.NewRequest(&jobv1.CompleteJobRequest{JobKey: validKey, Variables: "block"}))
	if connect.CodeOf(err) != connect.CodeDeadlineExceeded || time.Since(start) > 5*time.Second {
		t.Fatalf("code = %v after %s", connect.CodeOf(err), time.Since(start))
	}
}

func TestLongPollTimeoutFollowsRequestTimeout(t *testing.T) {
	client, stub := newServer(t, time.Second)
	_, err := client.ActivateJobs(context.Background(), connect.NewRequest(&jobv1.ActivateJobsRequest{
		Type: "t", RequestTimeout: durationpb.New(20 * time.Second),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if d := <-stub.deadlineIn; d < 20*time.Second || d > 26*time.Second {
		t.Fatalf("handler deadline in %s, want request_timeout + 5s", d)
	}
	_, _ = client.ActivateJobs(context.Background(), connect.NewRequest(&jobv1.ActivateJobsRequest{
		Type: "t", RequestTimeout: durationpb.New(10 * time.Minute),
	}))
	if d := <-stub.deadlineIn; d > rpc.MaxTimeout {
		t.Fatalf("handler deadline in %s, want at most %s", d, rpc.MaxTimeout)
	}
}

func TestReadMaxBytes(t *testing.T) {
	client, _ := newServer(t, 0)
	big := strings.Repeat("x", rpc.MaxRequestBytes+1)
	_, err := client.CompleteJob(context.Background(), connect.NewRequest(&jobv1.CompleteJobRequest{JobKey: validKey, Variables: big}))
	if connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Fatalf("code = %v, want ResourceExhausted", connect.CodeOf(err))
	}
}

func TestRejectAndErrorInfoOf(t *testing.T) {
	err := rpc.Reject(connect.CodeFailedPrecondition, rpc.ReasonJobNotActive, errors.New("job is not active"), map[string]string{"state": "FAILED"})
	info := rpc.ErrorInfoOf(err)
	if info.GetReason() != rpc.ReasonJobNotActive || info.GetMetadata()["state"] != "FAILED" {
		t.Fatalf("info = %v", info)
	}
	if rpc.ErrorInfoOf(errors.New("plain")) != nil {
		t.Fatal("plain error has no ErrorInfo")
	}
}
