package sdk_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"

	deploymentv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/deployment/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/deployment/v1/deploymentv1connect"
	incidentv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/incident/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/incident/v1/incidentv1connect"
	jobv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1/jobv1connect"
	messagev1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/message/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/message/v1/messagev1connect"
	processv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/process/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/process/v1/processv1connect"
	"github.com/indr622/workflow-engine/libs/go/sdk"
)

// stubEngine merekam request terakhir tiap RPC dan menjawab dengan respons tetap.
type stubEngine struct {
	deploymentv1connect.UnimplementedDeploymentServiceHandler
	processv1connect.UnimplementedProcessInstanceServiceHandler
	jobv1connect.UnimplementedJobServiceHandler
	incidentv1connect.UnimplementedIncidentServiceHandler
	messagev1connect.UnimplementedMessageServiceHandler
	last map[string]any
}

func (s *stubEngine) DeployResource(_ context.Context, r *connect.Request[deploymentv1.DeployResourceRequest]) (*connect.Response[deploymentv1.DeployResourceResponse], error) {
	s.last["deploy"] = r.Msg
	return connect.NewResponse(&deploymentv1.DeployResourceResponse{DeploymentKey: "d1", ProcessDefinitions: []*deploymentv1.ProcessDefinition{
		{ProcessDefinitionKey: "p1", BpmnProcessId: "order", Version: 2},
	}}), nil
}

func (s *stubEngine) CreateProcessInstance(_ context.Context, r *connect.Request[processv1.CreateProcessInstanceRequest]) (*connect.Response[processv1.CreateProcessInstanceResponse], error) {
	s.last["create"] = r.Msg
	return connect.NewResponse(&processv1.CreateProcessInstanceResponse{ProcessInstanceKey: "i1", BpmnProcessId: "order", Version: 2,
		State: processv1.ProcessInstanceState_PROCESS_INSTANCE_STATE_ACTIVE}), nil
}

func (s *stubEngine) CreateProcessInstanceWithResult(_ context.Context, r *connect.Request[processv1.CreateProcessInstanceWithResultRequest]) (*connect.Response[processv1.CreateProcessInstanceWithResultResponse], error) {
	s.last["result"] = r.Msg
	return connect.NewResponse(&processv1.CreateProcessInstanceWithResultResponse{ProcessInstanceKey: "i2", Variables: `{"ok":true}`,
		State: processv1.ProcessInstanceState_PROCESS_INSTANCE_STATE_COMPLETED}), nil
}

func (s *stubEngine) CancelProcessInstance(_ context.Context, r *connect.Request[processv1.CancelProcessInstanceRequest]) (*connect.Response[processv1.CancelProcessInstanceResponse], error) {
	s.last["cancel"] = r.Msg
	return connect.NewResponse(&processv1.CancelProcessInstanceResponse{}), nil
}

func (s *stubEngine) SetVariables(_ context.Context, r *connect.Request[processv1.SetVariablesRequest]) (*connect.Response[processv1.SetVariablesResponse], error) {
	s.last["set"] = r.Msg
	return connect.NewResponse(&processv1.SetVariablesResponse{}), nil
}

func (s *stubEngine) FailJob(_ context.Context, r *connect.Request[jobv1.FailJobRequest]) (*connect.Response[jobv1.FailJobResponse], error) {
	s.last["fail"] = r.Msg
	return connect.NewResponse(&jobv1.FailJobResponse{}), nil
}

func (s *stubEngine) ThrowError(_ context.Context, r *connect.Request[jobv1.ThrowErrorRequest]) (*connect.Response[jobv1.ThrowErrorResponse], error) {
	s.last["throw"] = r.Msg
	return connect.NewResponse(&jobv1.ThrowErrorResponse{}), nil
}

func (s *stubEngine) UpdateJobRetries(_ context.Context, r *connect.Request[jobv1.UpdateJobRetriesRequest]) (*connect.Response[jobv1.UpdateJobRetriesResponse], error) {
	s.last["retries"] = r.Msg
	return connect.NewResponse(&jobv1.UpdateJobRetriesResponse{}), nil
}

func (s *stubEngine) UpdateJobTimeout(_ context.Context, r *connect.Request[jobv1.UpdateJobTimeoutRequest]) (*connect.Response[jobv1.UpdateJobTimeoutResponse], error) {
	s.last["timeout"] = r.Msg
	return connect.NewResponse(&jobv1.UpdateJobTimeoutResponse{}), nil
}

func (s *stubEngine) ResolveIncident(_ context.Context, r *connect.Request[incidentv1.ResolveIncidentRequest]) (*connect.Response[incidentv1.ResolveIncidentResponse], error) {
	s.last["resolve"] = r.Msg
	return connect.NewResponse(&incidentv1.ResolveIncidentResponse{}), nil
}

func (s *stubEngine) PublishMessage(_ context.Context, r *connect.Request[messagev1.PublishMessageRequest]) (*connect.Response[messagev1.PublishMessageResponse], error) {
	s.last["publish"] = r.Msg
	return connect.NewResponse(&messagev1.PublishMessageResponse{MessageKey: "m1"}), nil
}

func newStub(t *testing.T) (*sdk.Client, *stubEngine) {
	t.Helper()
	s := &stubEngine{last: map[string]any{}}
	mux := http.NewServeMux()
	mux.Handle(deploymentv1connect.NewDeploymentServiceHandler(s))
	mux.Handle(processv1connect.NewProcessInstanceServiceHandler(s))
	mux.Handle(jobv1connect.NewJobServiceHandler(s))
	mux.Handle(incidentv1connect.NewIncidentServiceHandler(s))
	mux.Handle(messagev1connect.NewMessageServiceHandler(s))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := sdk.NewClient(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	return c, s
}

func TestHighLevelOperations(t *testing.T) {
	c, s := newStub(t)
	ctx := context.Background()

	dep, err := c.Deploy(ctx, sdk.Resource{Name: "order.bpmn", Content: []byte("<xml/>")})
	if err != nil || dep.DeploymentKey != "d1" || dep.Processes[0].Version != 2 {
		t.Fatalf("deploy = %+v %v", dep, err)
	}
	inst, err := c.CreateInstance(ctx, sdk.Latest("order"), map[string]int{"n": 1})
	if err != nil || inst.Key != "i1" || inst.State != "ACTIVE" {
		t.Fatalf("create = %+v %v", inst, err)
	}
	if got := s.last["create"].(*processv1.CreateProcessInstanceRequest); got.GetBpmnProcess().GetBpmnProcessId() != "order" || got.GetVariables() != `{"n":1}` {
		t.Fatalf("create request = %v", got)
	}
	if _, err := c.CreateInstance(ctx, sdk.ProcessRef{Key: "p1"}, nil); err != nil {
		t.Fatal(err)
	}
	if got := s.last["create"].(*processv1.CreateProcessInstanceRequest); got.GetProcessDefinitionKey() != "p1" {
		t.Fatalf("create by key = %v", got)
	}
	res, err := c.CreateInstanceWithResult(ctx, sdk.Latest("order"), nil, sdk.WithResultTimeout(3*time.Second), sdk.WithFetchVariables("ok"))
	if err != nil || res.State != "COMPLETED" || string(res.Variables) != `{"ok":true}` {
		t.Fatalf("result = %+v %v", res, err)
	}
	if got := s.last["result"].(*processv1.CreateProcessInstanceWithResultRequest); got.GetRequestTimeout().AsDuration() != 3*time.Second || got.GetFetchVariables()[0] != "ok" {
		t.Fatalf("result request = %v", got)
	}
	if err := c.CancelInstance(ctx, "i1"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetVariables(ctx, "i1", `{"x":1}`); err != nil || s.last["set"].(*processv1.SetVariablesRequest).GetScopeKey() != "i1" {
		t.Fatal(err)
	}
	if err := c.FailJob(ctx, "j1", sdk.Failure{Retries: 2, ErrorMessage: "e", RetryBackoff: time.Second}); err != nil {
		t.Fatal(err)
	}
	if got := s.last["fail"].(*jobv1.FailJobRequest); got.GetRetries() != 2 || got.GetRetryBackOff().AsDuration() != time.Second {
		t.Fatalf("fail request = %v", got)
	}
	if err := c.ThrowError(ctx, "j1", "CODE", "msg", map[string]bool{"x": true}); err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateJobRetries(ctx, "j1", 5); err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateJobTimeout(ctx, "j1", time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := c.ResolveIncident(ctx, "inc1"); err != nil {
		t.Fatal(err)
	}
	key, err := c.PublishMessage(ctx, sdk.Message{Name: "paid", CorrelationKey: "o-1", TTL: time.Minute, MessageID: "m"})
	if err != nil || key != "m1" {
		t.Fatalf("publish = %q %v", key, err)
	}
	for _, name := range []string{"deploy", "cancel", "throw", "retries", "timeout", "resolve", "publish"} {
		if s.last[name] == nil {
			t.Errorf("%s was not called", name)
		}
	}
	if c.ProcessInstances() == nil || c.Deployments() == nil || c.Incidents() == nil || c.Messages() == nil || c.Jobs() == nil {
		t.Fatal("raw clients must be available")
	}

	// Variabel yang tidak valid ditolak sebelum RPC.
	for name, call := range map[string]func() error{
		"create":   func() error { _, err := c.CreateInstance(ctx, sdk.Latest("order"), "[1]"); return err },
		"result":   func() error { _, err := c.CreateInstanceWithResult(ctx, sdk.Latest("order"), "[1]"); return err },
		"set":      func() error { return c.SetVariables(ctx, "i1", "[1]") },
		"complete": func() error { return c.CompleteJob(ctx, "j1", "[1]") },
		"fail":     func() error { return c.FailJob(ctx, "j1", sdk.Failure{Variables: "[1]"}) },
		"throw":    func() error { return c.ThrowError(ctx, "j1", "C", "", "[1]") },
		"publish":  func() error { _, err := c.PublishMessage(ctx, sdk.Message{Name: "x", Variables: "[1]"}); return err },
	} {
		if err := call(); !errors.Is(err, sdk.ErrInvalidVariables) {
			t.Errorf("%s with invalid variables = %v", name, err)
		}
	}
}

func TestUnimplementedRPCsReturnSDKErrors(t *testing.T) {
	c, _ := newStub(t)
	err := c.CompleteJob(context.Background(), "j1", nil)
	var e *sdk.Error
	if !errors.As(err, &e) || e.Code != connect.CodeUnimplemented || e.Reason != "" || e.Error() == "" {
		t.Fatalf("error = %#v", err)
	}
	if _, err := c.ActivateJobs(context.Background(), &jobv1.ActivateJobsRequest{Type: "t"}); err == nil {
		t.Fatal("unimplemented activate must fail")
	}
	if _, err := c.Deploy(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// ExampleNewClient menunjukkan pembuatan client dan pembuatan instance.
func ExampleNewClient() {
	c, err := sdk.NewClient("http://localhost:8080")
	if err != nil {
		panic(err)
	}
	_ = c // c.CreateInstance(ctx, sdk.Latest("order-process"), map[string]any{"orderId": 7})
	fmt.Println("client ready")
	// Output: client ready
}

// ExampleNewJobWorker menunjukkan worker yang menyelesaikan job atau melempar BPMN error.
func ExampleNewJobWorker() {
	c, _ := sdk.NewClient("http://localhost:8080")
	w, err := sdk.NewJobWorker(c).
		JobType("charge-payment").
		Concurrency(4).
		Handler(func(_ context.Context, job *sdk.Job) (sdk.Result, error) {
			amount, err := job.Variables.Float64("amount")
			if err != nil {
				return sdk.Result{}, err // FailJob dengan retries - 1
			}
			if amount > 1000 {
				return sdk.Result{}, sdk.BPMNError("LIMIT_EXCEEDED", "amount too high")
			}
			return sdk.Result{Variables: map[string]any{"charged": true}}, nil
		}).
		Build()
	fmt.Println(w != nil, err)
	// Output: true <nil>
}

// ExampleVariables_Unmarshal menunjukkan decode variabel job ke struct.
func ExampleVariables_Unmarshal() {
	var order struct {
		ID    int64   `json:"id"`
		Total float64 `json:"total"`
	}
	vars := sdk.Variables(`{"id": 9007199254740993, "total": 12.5}`)
	if err := vars.Unmarshal(&order); err != nil {
		panic(err)
	}
	fmt.Println(order.ID, order.Total)
	// Output: 9007199254740993 12.5
}
