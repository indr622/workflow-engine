//go:build integration

package infrastructure_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"github.com/indr622/workflow-engine/apps/engine/internal/platform/keygen"
	commonv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/common/v1"
	deploymentv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/deployment/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/deployment/v1/deploymentv1connect"
	jobv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1/jobv1connect"
	processv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/process/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/process/v1/processv1connect"
)

func newRequestID(t *testing.T) string {
	t.Helper()
	k, err := keygen.New().NewKey()
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// TestIdempotentCreateAndCompleteOverConnect: request_id yang sama dijalankan sekali, ulangan
// mendapat response identik dengan header Idempotent-Replayed, termasuk 20 request paralel.
func TestIdempotentCreateAndCompleteOverConnect(t *testing.T) {
	f := setup(t)
	processID := f.deployXML(t, "idem", linearXML)
	srv := httptest.NewServer(f.mux)
	defer srv.Close()
	ctx := context.Background()
	instances := processv1connect.NewProcessInstanceServiceClient(srv.Client(), srv.URL)
	jobs := jobv1connect.NewJobServiceClient(srv.Client(), srv.URL)

	rid := newRequestID(t)
	req := &processv1.CreateProcessInstanceRequest{
		Definition: &processv1.CreateProcessInstanceRequest_BpmnProcess{BpmnProcess: &processv1.ProcessDefinitionRef{BpmnProcessId: processID}},
		Variables:  `{"order": 1}`, RequestId: rid,
	}

	// 20 request paralel dengan request_id yang sama: satu instance, response identik.
	var wg sync.WaitGroup
	responses := make([]*connect.Response[processv1.CreateProcessInstanceResponse], 20)
	errs := make([]error, 20)
	for i := range responses {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			responses[i], errs[i] = instances.CreateProcessInstance(ctx, connect.NewRequest(proto.Clone(req).(*processv1.CreateProcessInstanceRequest)))
		}(i)
	}
	wg.Wait()
	replayed := 0
	for i, r := range responses {
		if errs[i] != nil {
			t.Fatalf("request %d: %v", i, errs[i])
		}
		if !proto.Equal(r.Msg, responses[0].Msg) {
			t.Fatalf("response %d differs: %v vs %v", i, r.Msg, responses[0].Msg)
		}
		if r.Header().Get("Idempotent-Replayed") == "true" {
			replayed++
		}
	}
	if replayed != 19 {
		t.Fatalf("%d replayed responses, want 19", replayed)
	}
	listed, err := instances.ListProcessInstances(ctx, connect.NewRequest(&processv1.ListProcessInstancesRequest{BpmnProcessId: processID}))
	if err != nil || len(listed.Msg.GetProcessInstances()) != 1 {
		t.Fatalf("instances = %v, %v; want exactly one", listed, err)
	}
	piKey := responses[0].Msg.GetProcessInstanceKey()

	// Request_id sama, payload berbeda: ditolak dengan detail IdempotencyKeyReused, tanpa efek.
	changed := proto.Clone(req).(*processv1.CreateProcessInstanceRequest)
	changed.Variables = `{"order": 2}`
	_, err = instances.CreateProcessInstance(ctx, connect.NewRequest(changed))
	var cerr *connect.Error
	if connect.CodeOf(err) != connect.CodeInvalidArgument || !errorsAs(err, &cerr) || !hasReusedDetail(cerr) {
		t.Fatalf("reused key error = %v", err)
	}

	// CompleteJob: ulangan setelah sukses dengan request_id yang sama tidak gagal FailedPrecondition.
	open, err := jobs.ListJobs(ctx, connect.NewRequest(&jobv1.ListJobsRequest{ProcessInstanceKey: piKey, OnlyOpen: true}))
	if err != nil || len(open.Msg.GetJobs()) != 1 {
		t.Fatalf("open jobs %v %v", open, err)
	}
	complete := &jobv1.CompleteJobRequest{JobKey: open.Msg.GetJobs()[0].GetJobKey(), Variables: `{"paid": true}`, RequestId: newRequestID(t)}
	first, err := jobs.CompleteJob(ctx, connect.NewRequest(complete))
	if err != nil || first.Header().Get("Idempotent-Replayed") != "" {
		t.Fatalf("first complete: %v replayed=%q", err, first.Header().Get("Idempotent-Replayed"))
	}
	again, err := jobs.CompleteJob(ctx, connect.NewRequest(complete))
	if err != nil || again.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatalf("retried complete: %v replayed=%q", err, again.Header().Get("Idempotent-Replayed"))
	}
	// Tanpa request_id, perilaku Fase 2 tetap: job yang sudah selesai ditolak.
	if _, err := jobs.CompleteJob(ctx, connect.NewRequest(&jobv1.CompleteJobRequest{JobKey: complete.GetJobKey()})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("complete without request_id: %v", err)
	}

	// Error domain tidak diklaim: cancel instance yang sudah selesai gagal, dan ulangan gagal sama.
	cancelReq := &processv1.CancelProcessInstanceRequest{ProcessInstanceKey: piKey, RequestId: newRequestID(t)}
	for i := 0; i < 2; i++ {
		if _, err := instances.CancelProcessInstance(ctx, connect.NewRequest(cancelReq)); connect.CodeOf(err) != connect.CodeFailedPrecondition {
			t.Fatalf("cancel attempt %d: %v", i, err)
		}
	}

	if _, err := instances.CreateProcessInstance(ctx, connect.NewRequest(&processv1.CreateProcessInstanceRequest{
		Definition: req.Definition, RequestId: string(make([]byte, 200)),
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("oversized request_id: %v", err)
	}
}

// TestIdempotentDeploy: DeployResource dengan request_id yang sama dijalankan sekali.
func TestIdempotentDeploy(t *testing.T) {
	f := setup(t)
	srv := httptest.NewServer(f.mux)
	defer srv.Close()
	defs := deploymentv1connect.NewDeploymentServiceClient(srv.Client(), srv.URL)
	req := &deploymentv1.DeployResourceRequest{
		Resources: []*deploymentv1.Resource{{Name: "idem-deploy.bpmn", Content: []byte(strings.ReplaceAll(linearXML, "PID", "idem-deploy-"+f.run))}},
		RequestId: newRequestID(t),
	}
	first, err := defs.DeployResource(context.Background(), connect.NewRequest(req))
	if err != nil {
		t.Fatal(err)
	}
	again, err := defs.DeployResource(context.Background(), connect.NewRequest(req))
	if err != nil {
		t.Fatal(err)
	}
	if again.Header().Get("Idempotent-Replayed") != "true" || !proto.Equal(first.Msg, again.Msg) {
		t.Fatalf("replay = %q, equal = %v", again.Header().Get("Idempotent-Replayed"), proto.Equal(first.Msg, again.Msg))
	}
	if first.Msg.GetDeploymentKey() != again.Msg.GetDeploymentKey() {
		t.Fatal("a second deployment was created")
	}
}

// TestExpiredClaimIsTreatedAsNew: klaim kedaluwarsa diperlakukan sebagai request baru.
func TestExpiredClaimIsTreatedAsNew(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	processID := f.deployXML(t, "idem-ttl", linearXML)
	srv := httptest.NewServer(f.mux)
	defer srv.Close()
	instances := processv1connect.NewProcessInstanceServiceClient(srv.Client(), srv.URL)
	req := &processv1.CreateProcessInstanceRequest{
		Definition: &processv1.CreateProcessInstanceRequest_BpmnProcess{BpmnProcess: &processv1.ProcessDefinitionRef{BpmnProcessId: processID}},
		RequestId:  newRequestID(t),
	}
	first, err := instances.CreateProcessInstance(ctx, connect.NewRequest(req))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE idempotency_key SET expires_at = now() - interval '1 second' WHERE key = $1`, req.GetRequestId()); err != nil {
		t.Fatal(err)
	}
	second, err := instances.CreateProcessInstance(ctx, connect.NewRequest(req))
	if err != nil {
		t.Fatal(err)
	}
	if second.Header().Get("Idempotent-Replayed") != "" || second.Msg.GetProcessInstanceKey() == first.Msg.GetProcessInstanceKey() {
		t.Fatal("expired claim must run the command again")
	}
}

func errorsAs(err error, target **connect.Error) bool {
	ce, ok := err.(*connect.Error)
	if ok {
		*target = ce
	}
	return ok
}

func hasReusedDetail(err *connect.Error) bool {
	for _, d := range err.Details() {
		msg, derr := d.Value()
		if derr != nil {
			continue
		}
		if _, ok := msg.(*commonv1.IdempotencyKeyReused); ok {
			return true
		}
	}
	return false
}
