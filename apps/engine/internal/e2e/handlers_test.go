//go:build integration

package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/indr622/workflow-engine/apps/engine/internal/platform/rpc"
	incidentv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/incident/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/incident/v1/incidentv1connect"
	jobv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1/jobv1connect"
	messagev1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/message/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/message/v1/messagev1connect"
	processv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/process/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/process/v1/processv1connect"
)

const unknownKey = "01a18350-a6b3-7458-8513-000000000000"

type clients struct {
	jobs      jobv1connect.JobServiceClient
	instances processv1connect.ProcessInstanceServiceClient
	incidents incidentv1connect.IncidentServiceClient
	messages  messagev1connect.MessageServiceClient
}

func (e *engine) clients() clients {
	return clients{
		jobs:      jobv1connect.NewJobServiceClient(e.client, e.url),
		instances: processv1connect.NewProcessInstanceServiceClient(e.client, e.url),
		incidents: incidentv1connect.NewIncidentServiceClient(e.client, e.url),
		messages:  messagev1connect.NewMessageServiceClient(e.client, e.url),
	}
}

func (c clients) create(t *testing.T, processID, vars string) string {
	t.Helper()
	res, err := c.instances.CreateProcessInstance(context.Background(), connect.NewRequest(&processv1.CreateProcessInstanceRequest{
		Definition: &processv1.CreateProcessInstanceRequest_BpmnProcess{BpmnProcess: &processv1.ProcessDefinitionRef{BpmnProcessId: processID}},
		Variables:  vars,
	}))
	if err != nil {
		t.Fatal(err)
	}
	return res.Msg.GetProcessInstanceKey()
}

func (c clients) activate(t *testing.T, jobType string, max int32, wait time.Duration) []*jobv1.ActivatedJob {
	t.Helper()
	res, err := c.jobs.ActivateJobs(context.Background(), connect.NewRequest(&jobv1.ActivateJobsRequest{
		Type: jobType, Worker: "e2e", MaxJobsToActivate: max, Timeout: durationpb.New(time.Minute),
		RequestTimeout: durationpb.New(wait),
	}))
	if err != nil {
		t.Fatal(err)
	}
	return res.Msg.GetJobs()
}

// TestWorkerProtocolOverConnect menjalankan alur 4.5 rencana Fase 4 lewat Connect: fail dengan
// backoff, fail tanpa retries, update retries, resolve, complete; plus penolakan beralasan.
func TestWorkerProtocolOverConnect(t *testing.T) {
	e := startEngine(t)
	c := e.clients()
	ctx := context.Background()
	id := e.deploy(t, "proto", header+`<bpmn:startEvent id="s"/>`+serviceTask("charge", "charge-PID", "3")+`<bpmn:endEvent id="e"/>`+
		flow("f1", "s", "charge")+flow("f2", "charge", "e")+footer)
	jobType := "charge-" + id
	pi := c.create(t, id, `{"order":1}`)

	jobs := c.activate(t, jobType, 10, 0)
	if len(jobs) != 1 || jobs[0].GetRetries() != 3 || jobs[0].GetVariables() != `{"order":1}` || jobs[0].GetTenantId() != rpc.DefaultTenant {
		t.Fatalf("activated = %v", jobs)
	}
	job := jobs[0]
	if _, err := c.jobs.FailJob(ctx, connect.NewRequest(&jobv1.FailJobRequest{
		JobKey: job.GetJobKey(), Retries: 2, ErrorMessage: "timeout upstream", RetryBackOff: durationpb.New(time.Second),
	})); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	again := c.activate(t, jobType, 10, 10*time.Second)
	if len(again) != 1 || again[0].GetRetries() != 2 || time.Since(start) < 500*time.Millisecond {
		t.Fatalf("reactivation after backoff = %v after %s", again, time.Since(start))
	}
	failReq := &jobv1.FailJobRequest{JobKey: job.GetJobKey(), Retries: 0, ErrorMessage: "permanent", RequestId: "fail-" + e.run}
	if _, err := c.jobs.FailJob(ctx, connect.NewRequest(failReq)); err != nil {
		t.Fatal(err)
	}
	// Ulangan dengan request_id sama: replay, bukan JOB_NOT_ACTIVE.
	replay, err := c.jobs.FailJob(ctx, connect.NewRequest(failReq))
	if err != nil || replay.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatalf("idempotent fail replay = %v, header %q", err, replay.Header().Get("Idempotent-Replayed"))
	}
	_, err = c.jobs.CompleteJob(ctx, connect.NewRequest(&jobv1.CompleteJobRequest{JobKey: job.GetJobKey()}))
	expectReason(t, "complete failed job", err, connect.CodeFailedPrecondition, rpc.ReasonJobNotActive)

	incidents, err := c.incidents.ListIncidents(ctx, connect.NewRequest(&incidentv1.ListIncidentsRequest{ProcessInstanceKey: pi, OnlyOpen: true}))
	if err != nil || len(incidents.Msg.GetIncidents()) != 1 {
		t.Fatalf("incidents = %v %v", incidents, err)
	}
	inc := incidents.Msg.GetIncidents()[0]
	if inc.GetErrorType() != "JOB_NO_RETRIES" || inc.GetJobKey() != job.GetJobKey() || inc.GetErrorMessage() != "permanent" {
		t.Fatalf("incident = %v", inc)
	}
	listed, err := c.jobs.ListJobs(ctx, connect.NewRequest(&jobv1.ListJobsRequest{ProcessInstanceKey: pi, State: jobv1.JobState_JOB_STATE_FAILED}))
	if err != nil || len(listed.Msg.GetJobs()) != 1 || listed.Msg.GetJobs()[0].GetStateEnum() != jobv1.JobState_JOB_STATE_FAILED ||
		listed.Msg.GetJobs()[0].GetActivationCount() != 2 {
		t.Fatalf("list failed jobs = %v %v", listed, err)
	}

	if _, err := c.jobs.UpdateJobRetries(ctx, connect.NewRequest(&jobv1.UpdateJobRetriesRequest{JobKey: job.GetJobKey(), Retries: 1})); err != nil {
		t.Fatal(err)
	}
	if _, err := c.incidents.ResolveIncident(ctx, connect.NewRequest(&incidentv1.ResolveIncidentRequest{IncidentKey: inc.GetIncidentKey()})); err != nil {
		t.Fatal(err)
	}
	_, err = c.incidents.ResolveIncident(ctx, connect.NewRequest(&incidentv1.ResolveIncidentRequest{IncidentKey: inc.GetIncidentKey()}))
	expectReason(t, "resolve twice", err, connect.CodeFailedPrecondition, rpc.ReasonIncidentAlreadyResolved)

	final := c.activate(t, jobType, 10, 5*time.Second)
	if len(final) != 1 {
		t.Fatalf("job not activatable after resolve: %v", final)
	}
	_, err = c.jobs.UpdateJobTimeout(ctx, connect.NewRequest(&jobv1.UpdateJobTimeoutRequest{JobKey: job.GetJobKey(), Timeout: durationpb.New(time.Hour)}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.jobs.CompleteJob(ctx, connect.NewRequest(&jobv1.CompleteJobRequest{JobKey: job.GetJobKey(), Variables: `{"paid":true}`})); err != nil {
		t.Fatal(err)
	}
	_, err = c.jobs.UpdateJobTimeout(ctx, connect.NewRequest(&jobv1.UpdateJobTimeoutRequest{JobKey: job.GetJobKey(), Timeout: durationpb.New(time.Hour)}))
	expectReason(t, "timeout on completed job", err, connect.CodeFailedPrecondition, rpc.ReasonJobNotActivated)

	got, err := c.instances.GetProcessInstance(ctx, connect.NewRequest(&processv1.GetProcessInstanceRequest{ProcessInstanceKey: pi}))
	if err != nil || got.Msg.GetProcessInstance().GetState() != processv1.ProcessInstanceState_PROCESS_INSTANCE_STATE_COMPLETED {
		t.Fatalf("instance = %v %v", got, err)
	}

	// Penolakan beralasan lainnya.
	_, err = c.jobs.CompleteJob(ctx, connect.NewRequest(&jobv1.CompleteJobRequest{JobKey: unknownKey}))
	expectReason(t, "unknown job", err, connect.CodeNotFound, rpc.ReasonJobNotFound)
	_, err = c.jobs.FailJob(ctx, connect.NewRequest(&jobv1.FailJobRequest{JobKey: "nope"}))
	expectReason(t, "invalid job key", err, connect.CodeInvalidArgument, rpc.ReasonRequestInvalid)
	_, err = c.jobs.ThrowError(ctx, connect.NewRequest(&jobv1.ThrowErrorRequest{JobKey: job.GetJobKey()}))
	expectReason(t, "empty error code", err, connect.CodeInvalidArgument, rpc.ReasonRequestInvalid)
	_, err = c.incidents.ResolveIncident(ctx, connect.NewRequest(&incidentv1.ResolveIncidentRequest{IncidentKey: unknownKey}))
	expectReason(t, "unknown incident", err, connect.CodeNotFound, rpc.ReasonIncidentNotFound)
	_, err = c.instances.SetVariables(ctx, connect.NewRequest(&processv1.SetVariablesRequest{ScopeKey: pi, Variables: `{"x":1}`}))
	expectReason(t, "set variables on completed instance", err, connect.CodeFailedPrecondition, rpc.ReasonProcessInstanceNotActive)
	_, err = c.instances.SetVariables(ctx, connect.NewRequest(&processv1.SetVariablesRequest{ScopeKey: unknownKey, Variables: `{"x":1}`}))
	expectReason(t, "unknown scope", err, connect.CodeNotFound, rpc.ReasonScopeNotFound)
	_, err = c.instances.SetVariables(ctx, connect.NewRequest(&processv1.SetVariablesRequest{ScopeKey: pi, Variables: `[1]`}))
	expectReason(t, "array variables", err, connect.CodeInvalidArgument, rpc.ReasonVariablesInvalid)
}

func TestThrowErrorAndLocalVariablesOverConnect(t *testing.T) {
	e := startEngine(t)
	c := e.clients()
	ctx := context.Background()
	id := e.deploy(t, "throw", header+`<bpmn:startEvent id="s"/>`+serviceTask("ship", "ship-PID", "")+`<bpmn:endEvent id="e"/>`+
		flow("f1", "s", "ship")+flow("f2", "ship", "e")+footer)
	pi := c.create(t, id, `{}`)
	job := c.activate(t, "ship-"+id, 1, 0)[0]

	_, err := c.instances.SetVariables(ctx, connect.NewRequest(&processv1.SetVariablesRequest{
		ScopeKey: job.GetElementInstanceKey(), Variables: `{"x":1}`, Local: true,
	}))
	expectReason(t, "local on element scope", err, connect.CodeUnimplemented, rpc.ReasonLocalScopeNotSupported)
	if _, err := c.instances.SetVariables(ctx, connect.NewRequest(&processv1.SetVariablesRequest{
		ScopeKey: job.GetElementInstanceKey(), Variables: `{"x":1}`,
	})); err != nil {
		t.Fatal(err)
	}

	if _, err := c.jobs.ThrowError(ctx, connect.NewRequest(&jobv1.ThrowErrorRequest{JobKey: job.GetJobKey(), ErrorCode: "NO_STOCK"})); err != nil {
		t.Fatal(err)
	}
	incidents, _ := c.incidents.ListIncidents(ctx, connect.NewRequest(&incidentv1.ListIncidentsRequest{
		ProcessInstanceKey: pi, ErrorType: "UNHANDLED_ERROR_EVENT", OnlyOpen: true,
	}))
	if len(incidents.Msg.GetIncidents()) != 1 || !strings.Contains(incidents.Msg.GetIncidents()[0].GetErrorMessage(), "NO_STOCK") {
		t.Fatalf("incidents = %v", incidents.Msg)
	}
	if got := c.activate(t, "ship-"+id, 1, 0); len(got) != 0 {
		t.Fatal("job with thrown error must not be activatable")
	}
}

func TestLongPollingWakesOnJobCreation(t *testing.T) {
	e := startEngine(t)
	c := e.clients()
	id := e.deploy(t, "wake", header+`<bpmn:startEvent id="s"/>`+serviceTask("t", "wake-PID", "")+flow("f", "s", "t")+footer)
	type result struct {
		jobs []*jobv1.ActivatedJob
		at   time.Time
	}
	done := make(chan result, 1)
	go func() {
		jobs := c.activate(t, "wake-"+id, 1, 20*time.Second)
		done <- result{jobs, time.Now()}
	}()
	time.Sleep(300 * time.Millisecond)
	created := time.Now()
	c.create(t, id, `{}`)
	r := <-done
	if len(r.jobs) != 1 {
		t.Fatalf("long poll returned %d jobs", len(r.jobs))
	}
	// Notifier in-process membangunkan jauh sebelum polling cadangan (200 ms di test ini).
	if d := r.at.Sub(created); d > time.Second {
		t.Fatalf("long poll woke %s after creation", d)
	}

	// Tanpa job: request_timeout habis → daftar kosong, bukan error.
	start := time.Now()
	if jobs := c.activate(t, "nothing-"+e.run, 1, 500*time.Millisecond); len(jobs) != 0 || time.Since(start) < 500*time.Millisecond {
		t.Fatalf("empty long poll = %v after %s", jobs, time.Since(start))
	}
}

func TestLeaseTimeoutReactivates(t *testing.T) {
	e := startEngine(t)
	c := e.clients()
	ctx := context.Background()
	id := e.deploy(t, "lease", header+`<bpmn:startEvent id="s"/>`+serviceTask("t", "lease-PID", "")+flow("f", "s", "t")+footer)
	c.create(t, id, `{}`)
	res, err := c.jobs.ActivateJobs(ctx, connect.NewRequest(&jobv1.ActivateJobsRequest{
		Type: "lease-" + id, MaxJobsToActivate: 1, Timeout: durationpb.New(time.Second),
	}))
	if err != nil || len(res.Msg.GetJobs()) != 1 {
		t.Fatalf("activate = %v %v", res, err)
	}
	first := res.Msg.GetJobs()[0]
	start := time.Now()
	again := c.activate(t, "lease-"+id, 1, 10*time.Second)
	if len(again) != 1 || again[0].GetJobKey() != first.GetJobKey() {
		t.Fatalf("reactivation = %v", again)
	}
	// Sweeper 100 ms: kembali dalam ≤ 2 × interval setelah deadline 1 s.
	if d := time.Since(start); d < 800*time.Millisecond || d > 3*time.Second {
		t.Fatalf("reactivated after %s", d)
	}
	// Worker lama masih boleh menyelesaikan job yang belum final (semantik Zeebe).
	if _, err := c.jobs.CompleteJob(ctx, connect.NewRequest(&jobv1.CompleteJobRequest{JobKey: first.GetJobKey()})); err != nil {
		t.Fatal(err)
	}
}

func TestCreateWithResultOverConnect(t *testing.T) {
	e := startEngine(t)
	c := e.clients()
	ctx := context.Background()
	id := e.deploy(t, "result", header+`<bpmn:startEvent id="s"/>`+serviceTask("t", "result-PID", "")+`<bpmn:endEvent id="e"/>`+
		flow("f1", "s", "t")+flow("f2", "t", "e")+footer)
	go func() {
		jobs := c.activate(t, "result-"+id, 1, 10*time.Second)
		for _, j := range jobs {
			_, _ = c.jobs.CompleteJob(ctx, connect.NewRequest(&jobv1.CompleteJobRequest{JobKey: j.GetJobKey(), Variables: `{"total":42}`}))
		}
	}()
	res, err := c.instances.CreateProcessInstanceWithResult(ctx, connect.NewRequest(&processv1.CreateProcessInstanceWithResultRequest{
		Request: &processv1.CreateProcessInstanceRequest{
			Definition: &processv1.CreateProcessInstanceRequest_BpmnProcess{BpmnProcess: &processv1.ProcessDefinitionRef{BpmnProcessId: id}},
			Variables:  `{"a":1}`,
		},
		RequestTimeout: durationpb.New(10 * time.Second), FetchVariables: []string{"total"},
	}))
	if err != nil || res.Msg.GetState() != processv1.ProcessInstanceState_PROCESS_INSTANCE_STATE_COMPLETED || res.Msg.GetVariables() != `{"total":42}` {
		t.Fatalf("result = %v %v", res, err)
	}

	_, err = c.instances.CreateProcessInstanceWithResult(ctx, connect.NewRequest(&processv1.CreateProcessInstanceWithResultRequest{
		Request: &processv1.CreateProcessInstanceRequest{
			Definition: &processv1.CreateProcessInstanceRequest_BpmnProcess{BpmnProcess: &processv1.ProcessDefinitionRef{BpmnProcessId: id}},
		},
		RequestTimeout: durationpb.New(300 * time.Millisecond),
	}))
	expectReason(t, "result timeout", err, connect.CodeDeadlineExceeded, rpc.ReasonResultTimeout)
	if rpc.ErrorInfoOf(err).GetMetadata()["process_instance_key"] == "" {
		t.Fatal("RESULT_TIMEOUT must carry process_instance_key")
	}
}

func TestPublishMessageOverConnect(t *testing.T) {
	e := startEngine(t)
	c := e.clients()
	ctx := context.Background()
	name := "order-paid-" + e.run
	pub := func(ttl time.Duration, id string) (*connect.Response[messagev1.PublishMessageResponse], error) {
		return c.messages.PublishMessage(ctx, connect.NewRequest(&messagev1.PublishMessageRequest{
			Name: name, CorrelationKey: "order-1", TimeToLive: durationpb.New(ttl), MessageId: id, Variables: `{"amount": 10}`,
		}))
	}
	first, err := pub(time.Second, "m-1")
	if err != nil || first.Msg.GetMessageKey() == "" || first.Msg.GetTenantId() != rpc.DefaultTenant {
		t.Fatalf("publish = %v %v", first, err)
	}
	_, err = pub(time.Second, "m-1")
	expectReason(t, "duplicate message id", err, connect.CodeAlreadyExists, rpc.ReasonMessageDuplicate)
	if rpc.ErrorInfoOf(err).GetMetadata()["message_key"] != first.Msg.GetMessageKey() {
		t.Fatalf("duplicate metadata = %v", rpc.ErrorInfoOf(err).GetMetadata())
	}
	// TTL 0: diterima, tidak disimpan, key tetap dikembalikan.
	zero, err := pub(0, "m-zero")
	if err != nil || zero.Msg.GetMessageKey() == "" {
		t.Fatalf("ttl 0 = %v %v", zero, err)
	}
	var stored int
	if err := e.app.Pool.QueryRow(ctx, `SELECT count(*) FROM message WHERE name = $1`, name).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 1 {
		t.Fatalf("stored messages = %d, want 1", stored)
	}
	// Setelah TTL habis (dan sweeper berjalan) message_id yang sama diterima lagi.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := e.app.Pool.QueryRow(ctx, `SELECT count(*) FROM message WHERE name = $1`, name).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if stored == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("expired message was not swept")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if _, err := pub(time.Second, "m-1"); err != nil {
		t.Fatalf("republish after ttl = %v", err)
	}
	_, err = c.messages.PublishMessage(ctx, connect.NewRequest(&messagev1.PublishMessageRequest{Name: ""}))
	expectReason(t, "empty name", err, connect.CodeInvalidArgument, rpc.ReasonRequestInvalid)
}
