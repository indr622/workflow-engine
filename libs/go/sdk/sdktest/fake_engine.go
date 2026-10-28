// Package sdktest membantu menguji worker tanpa engine dan Postgres: FakeEngine menjalankan
// JobService in-memory dengan semantik lease, fail, dan error yang sama (versi disederhanakan),
// dan RunHandler memanggil handler secara langsung.
package sdktest

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	jobv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1/jobv1connect"
)

// JobState adalah state job di FakeEngine; nilainya sama dengan engine.
type JobState string

// State job.
const (
	Created     JobState = "CREATED"
	Activated   JobState = "ACTIVATED"
	Completed   JobState = "COMPLETED"
	Failed      JobState = "FAILED"
	ErrorThrown JobState = "ERROR_THROWN"
)

// FakeJob adalah satu job di FakeEngine beserta hasil terakhirnya.
type FakeJob struct {
	Key           string
	Type          string
	State         JobState
	Retries       int32
	Variables     string
	Headers       map[string]string
	Worker        string
	Deadline      time.Time
	ActivatableAt time.Time
	// Hasil yang dilaporkan worker.
	CompletedVariables string
	ErrorMessage       string
	ErrorCode          string
	LastBackoff        time.Duration
	Activations        int
}

// FakeEngine adalah JobService in-memory di server httptest.
type FakeEngine struct {
	jobv1connect.UnimplementedJobServiceHandler

	mu     sync.Mutex
	jobs   map[string]*FakeJob
	order  []string
	seq    int
	srv    *httptest.Server
	notify chan struct{}
	now    func() time.Time
}

// NewFakeEngine menjalankan FakeEngine dan menutupnya saat test selesai.
func NewFakeEngine(t testing.TB) *FakeEngine {
	t.Helper()
	f := &FakeEngine{jobs: map[string]*FakeJob{}, notify: make(chan struct{}, 1), now: time.Now}
	mux := http.NewServeMux()
	mux.Handle(jobv1connect.NewJobServiceHandler(f))
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// URL adalah base URL untuk sdk.NewClient.
func (f *FakeEngine) URL() string { return f.srv.URL }

// AddJob menambah job CREATED dan mengembalikan key-nya. vars adalah JSON object ("" = {}).
func (f *FakeEngine) AddJob(jobType, vars string, retries int32, headers map[string]string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	key := fmt.Sprintf("00000000-0000-7000-8000-%012d", f.seq)
	if vars == "" {
		vars = "{}"
	}
	f.jobs[key] = &FakeJob{Key: key, Type: jobType, State: Created, Retries: retries, Variables: vars, Headers: headers}
	f.order = append(f.order, key)
	f.wake()
	return key
}

// Job mengembalikan salinan job.
func (f *FakeEngine) Job(key string) FakeJob {
	f.mu.Lock()
	defer f.mu.Unlock()
	if j, ok := f.jobs[key]; ok {
		return *j
	}
	return FakeJob{}
}

// WaitFor menunggu sampai job mencapai state atau timeout; mengembalikan salinan terakhir.
func (f *FakeEngine) WaitFor(t testing.TB, key string, state JobState, timeout time.Duration) FakeJob {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		j := f.Job(key)
		if j.State == state {
			return j
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %s is %s after %s, want %s", key, j.State, timeout, state)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (f *FakeEngine) wake() {
	select {
	case f.notify <- struct{}{}:
	default:
	}
}

// ActivateJobs memenuhi JobService: mengembalikan job CREATED yang sudah boleh diaktifkan, dan
// menunggu sampai request_timeout bila tidak ada.
func (f *FakeEngine) ActivateJobs(ctx context.Context, req *connect.Request[jobv1.ActivateJobsRequest]) (*connect.Response[jobv1.ActivateJobsResponse], error) {
	wait := req.Msg.GetRequestTimeout().AsDuration()
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	for {
		jobs := f.activate(req.Msg)
		if len(jobs) > 0 || wait <= 0 {
			return connect.NewResponse(&jobv1.ActivateJobsResponse{Jobs: jobs}), nil
		}
		select {
		case <-ctx.Done():
			return connect.NewResponse(&jobv1.ActivateJobsResponse{}), nil
		case <-deadline.C:
			return connect.NewResponse(&jobv1.ActivateJobsResponse{}), nil
		case <-f.notify:
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func (f *FakeEngine) activate(m *jobv1.ActivateJobsRequest) []*jobv1.ActivatedJob {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := f.now()
	maxJobs := int(m.GetMaxJobsToActivate())
	if maxJobs <= 0 {
		maxJobs = 32
	}
	timeout := m.GetTimeout().AsDuration()
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	var out []*jobv1.ActivatedJob
	for _, key := range f.order {
		j := f.jobs[key]
		if j.State == Activated && now.After(j.Deadline) {
			j.State = Created // lease kedaluwarsa
		}
		if len(out) >= maxJobs || j.Type != m.GetType() || j.State != Created || now.Before(j.ActivatableAt) {
			continue
		}
		j.State, j.Worker, j.Deadline = Activated, m.GetWorker(), now.Add(timeout)
		j.Activations++
		out = append(out, &jobv1.ActivatedJob{
			JobKey: j.Key, Type: j.Type, Retries: j.Retries, Variables: j.Variables, CustomHeaders: j.Headers,
			Worker: j.Worker, Deadline: timestamppb.New(j.Deadline), TenantId: "<default>",
		})
	}
	return out
}

func (f *FakeEngine) open(key string) (*FakeJob, error) {
	j, ok := f.jobs[key]
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("job %s not found", key))
	}
	if j.State != Created && j.State != Activated {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("job %s is %s", key, j.State))
	}
	return j, nil
}

// CompleteJob memenuhi JobService.
func (f *FakeEngine) CompleteJob(_ context.Context, req *connect.Request[jobv1.CompleteJobRequest]) (*connect.Response[jobv1.CompleteJobResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	j, err := f.open(req.Msg.GetJobKey())
	if err != nil {
		return nil, err
	}
	j.State, j.CompletedVariables = Completed, req.Msg.GetVariables()
	return connect.NewResponse(&jobv1.CompleteJobResponse{}), nil
}

// FailJob memenuhi JobService.
func (f *FakeEngine) FailJob(_ context.Context, req *connect.Request[jobv1.FailJobRequest]) (*connect.Response[jobv1.FailJobResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	j, err := f.open(req.Msg.GetJobKey())
	if err != nil {
		return nil, err
	}
	j.Retries, j.ErrorMessage = req.Msg.GetRetries(), req.Msg.GetErrorMessage()
	j.LastBackoff = req.Msg.GetRetryBackOff().AsDuration()
	if j.Retries <= 0 {
		j.State = Failed
	} else {
		j.State, j.ActivatableAt = Created, f.now().Add(j.LastBackoff)
		f.wake()
	}
	return connect.NewResponse(&jobv1.FailJobResponse{}), nil
}

// ThrowError memenuhi JobService.
func (f *FakeEngine) ThrowError(_ context.Context, req *connect.Request[jobv1.ThrowErrorRequest]) (*connect.Response[jobv1.ThrowErrorResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	j, err := f.open(req.Msg.GetJobKey())
	if err != nil {
		return nil, err
	}
	j.State, j.ErrorCode, j.ErrorMessage = ErrorThrown, req.Msg.GetErrorCode(), req.Msg.GetErrorMessage()
	return connect.NewResponse(&jobv1.ThrowErrorResponse{}), nil
}

// UpdateJobTimeout memenuhi JobService.
func (f *FakeEngine) UpdateJobTimeout(_ context.Context, req *connect.Request[jobv1.UpdateJobTimeoutRequest]) (*connect.Response[jobv1.UpdateJobTimeoutResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	j, ok := f.jobs[req.Msg.GetJobKey()]
	if !ok || j.State != Activated {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("job %s is not activated", req.Msg.GetJobKey()))
	}
	j.Deadline = f.now().Add(req.Msg.GetTimeout().AsDuration())
	return connect.NewResponse(&jobv1.UpdateJobTimeoutResponse{}), nil
}

// UpdateJobRetries memenuhi JobService.
func (f *FakeEngine) UpdateJobRetries(_ context.Context, req *connect.Request[jobv1.UpdateJobRetriesRequest]) (*connect.Response[jobv1.UpdateJobRetriesResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	j, ok := f.jobs[req.Msg.GetJobKey()]
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("job %s not found", req.Msg.GetJobKey()))
	}
	j.Retries = req.Msg.GetRetries()
	return connect.NewResponse(&jobv1.UpdateJobRetriesResponse{}), nil
}

// ListJobs memenuhi JobService (urut key menurun, tanpa halaman).
func (f *FakeEngine) ListJobs(_ context.Context, req *connect.Request[jobv1.ListJobsRequest]) (*connect.Response[jobv1.ListJobsResponse], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := &jobv1.ListJobsResponse{}
	keys := append([]string(nil), f.order...)
	sort.Sort(sort.Reverse(sort.StringSlice(keys)))
	for _, k := range keys {
		j := f.jobs[k]
		if req.Msg.GetType() != "" && j.Type != req.Msg.GetType() {
			continue
		}
		out.Jobs = append(out.Jobs, &jobv1.JobInfo{JobKey: j.Key, Type: j.Type, Retries: j.Retries, State: string(j.State), Worker: j.Worker})
	}
	return connect.NewResponse(out), nil
}
