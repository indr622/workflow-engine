//go:build integration

package e2e

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"

	incidentv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/incident/v1"
	processv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/process/v1"
	"github.com/indr622/workflow-engine/libs/go/sdk"
)

func (e *engine) sdk(t testing.TB) *sdk.Client {
	t.Helper()
	c, err := sdk.NewClient(e.url, sdk.WithHTTPClient(e.client))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func startWorker(t testing.TB, c *sdk.Client, jobType string, h sdk.Handler, tune func(*sdk.WorkerBuilder)) *sdk.JobWorker {
	t.Helper()
	b := sdk.NewJobWorker(c).JobType(jobType).Handler(h).RequestTimeout(2 * time.Second).
		FailureBackoff(sdk.BackoffPolicy{Initial: 50 * time.Millisecond, Max: 200 * time.Millisecond, Multiplier: 2})
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

// TestSDKWorkerLifecycle: deploy → create → worker fail (retry) → complete → worker BPMN error →
// incident → resolve → selesai, semuanya lewat SDK terhadap engine in-process.
func TestSDKWorkerLifecycle(t *testing.T) {
	e := startEngine(t)
	c := e.sdk(t)
	ctx := context.Background()
	xml := strings.ReplaceAll(header+`<bpmn:startEvent id="s"/>`+serviceTask("charge", "charge-PID", "3")+
		serviceTask("ship", "ship-PID", "1")+`<bpmn:endEvent id="e"/>`+
		flow("f1", "s", "charge")+flow("f2", "charge", "ship")+flow("f3", "ship", "e")+footer, "PID", "sdk-"+e.run)
	dep, err := c.Deploy(ctx, sdk.Resource{Name: "sdk-" + e.run + ".bpmn", Content: []byte(xml)})
	if err != nil || len(dep.Processes) != 1 {
		t.Fatalf("deploy = %+v %v", dep, err)
	}
	processID := dep.Processes[0].BpmnProcessID

	var chargeCalls atomic.Int32
	startWorker(t, c, "charge-"+processID, func(_ context.Context, job *sdk.Job) (sdk.Result, error) {
		if chargeCalls.Add(1) == 1 {
			return sdk.Result{}, errors.New("gateway timeout") // retry otomatis
		}
		amount, err := job.Variables.Int64("amount")
		if err != nil {
			return sdk.Result{}, err
		}
		return sdk.Result{Variables: map[string]any{"charged": amount}}, nil
	}, nil)
	var shipCalls atomic.Int32
	startWorker(t, c, "ship-"+processID, func(context.Context, *sdk.Job) (sdk.Result, error) {
		if shipCalls.Add(1) == 1 {
			return sdk.Result{}, sdk.BPMNError("NO_CARRIER", "no carrier available")
		}
		return sdk.Result{Variables: map[string]string{"carrier": "dhl"}}, nil
	}, nil)

	inst, err := c.CreateInstance(ctx, sdk.Latest(processID), map[string]any{"amount": 120})
	if err != nil {
		t.Fatal(err)
	}
	// Tunggu incident UNHANDLED_ERROR_EVENT dari worker ship, lalu selesaikan.
	var incidentKey string
	deadline := time.Now().Add(15 * time.Second)
	for incidentKey == "" {
		res, err := c.Incidents().ListIncidents(ctx, connect.NewRequest(&incidentv1.ListIncidentsRequest{
			ProcessInstanceKey: inst.Key, OnlyOpen: true,
		}))
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Msg.GetIncidents()) > 0 {
			incidentKey = res.Msg.GetIncidents()[0].GetIncidentKey()
			if res.Msg.GetIncidents()[0].GetErrorType() != "UNHANDLED_ERROR_EVENT" {
				t.Fatalf("incident = %v", res.Msg.GetIncidents()[0])
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no incident from the BPMN error")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := c.ResolveIncident(ctx, incidentKey); err != nil {
		t.Fatal(err)
	}
	if err := c.ResolveIncident(ctx, incidentKey); !errors.Is(err, sdk.ErrIncidentAlreadyResolved) {
		t.Fatalf("second resolve = %v", err)
	}
	for {
		got, err := c.ProcessInstances().GetProcessInstance(ctx, connect.NewRequest(processGet(inst.Key)))
		if err != nil {
			t.Fatal(err)
		}
		if got.Msg.GetProcessInstance().GetState().String() == "PROCESS_INSTANCE_STATE_COMPLETED" {
			vars := map[string]string{}
			for _, v := range got.Msg.GetVariables() {
				vars[v.GetName()] = v.GetValue()
			}
			if vars["charged"] != "120" || vars["carrier"] != `"dhl"` {
				t.Fatalf("final variables = %v", vars)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("instance not completed: %v", got.Msg.GetProcessInstance())
		}
		time.Sleep(100 * time.Millisecond)
	}
	if chargeCalls.Load() != 2 || shipCalls.Load() != 2 {
		t.Fatalf("charge calls=%d ship calls=%d, want 2 and 2", chargeCalls.Load(), shipCalls.Load())
	}

	// CreateInstanceWithResult lewat SDK dengan worker yang sudah berjalan (incident pertama sudah
	// lewat, jadi ship langsung sukses).
	res, err := c.CreateInstanceWithResult(ctx, sdk.Latest(processID), map[string]any{"amount": 5}, sdk.WithResultTimeout(15*time.Second))
	if err != nil || res.State != "COMPLETED" {
		t.Fatalf("with result = %+v %v", res, err)
	}
	if charged, err := res.Variables.Int64("charged"); err != nil || charged != 5 {
		t.Fatalf("charged = %d %v", charged, err)
	}
}

// TestConcurrentWorkersActivateEachJobOnce: 40 instance × 50 service task paralel, 8 worker SDK;
// semua instance selesai dan setiap job diaktifkan tepat satu kali.
func TestConcurrentWorkersActivateEachJobOnce(t *testing.T) {
	if testing.Short() {
		t.Skip("long concurrency test")
	}
	e := startEngine(t)
	c := e.sdk(t)
	ctx := context.Background()
	const branches, instances, workers = 50, 40, 8
	var b strings.Builder
	b.WriteString(header + `<bpmn:startEvent id="s"/><bpmn:parallelGateway id="fork"/><bpmn:parallelGateway id="join"/><bpmn:endEvent id="e"/>`)
	b.WriteString(flow("f0", "s", "fork") + flow("fe", "join", "e"))
	for i := 0; i < branches; i++ {
		b.WriteString(serviceTask(fmt.Sprintf("t%d", i), "branch-PID", "3"))
		b.WriteString(flow(fmt.Sprintf("a%d", i), "fork", fmt.Sprintf("t%d", i)) + flow(fmt.Sprintf("b%d", i), fmt.Sprintf("t%d", i), "join"))
	}
	b.WriteString(footer)
	id := e.deploy(t, "fanout", b.String())

	var mu sync.Mutex
	seen := map[string]int{}
	for i := 0; i < workers; i++ {
		startWorker(t, c, "branch-"+id, func(_ context.Context, job *sdk.Job) (sdk.Result, error) {
			mu.Lock()
			seen[job.Key]++
			mu.Unlock()
			return sdk.Result{}, nil
		}, func(b *sdk.WorkerBuilder) { b.Name(fmt.Sprintf("w%d", i)).Concurrency(8).MaxJobsActive(32) })
	}
	start := time.Now()
	keys := make([]string, 0, instances)
	for i := 0; i < instances; i++ {
		inst, err := c.CreateInstance(ctx, sdk.Latest(id), nil)
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, inst.Key)
	}
	deadline := time.Now().Add(90 * time.Second)
	for _, k := range keys {
		for {
			var state string
			if err := e.app.Pool.QueryRow(ctx, `SELECT state FROM process_instance WHERE key = $1`, k).Scan(&state); err != nil {
				t.Fatal(err)
			}
			if state == "COMPLETED" {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("instance %s still %s after %s", k, state, time.Since(start))
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	elapsed := time.Since(start)
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != branches*instances {
		t.Fatalf("handled %d distinct jobs, want %d", len(seen), branches*instances)
	}
	var reactivated int
	if err := e.app.Pool.QueryRow(ctx, `SELECT count(*) FROM job WHERE process_instance_key = ANY($1::uuid[]) AND activation_count <> 1`, keys).Scan(&reactivated); err != nil {
		t.Fatal(err)
	}
	if reactivated != 0 {
		t.Fatalf("%d jobs were activated more than once without a timeout", reactivated)
	}
	t.Logf("%d instances x %d branches with %d SDK workers completed in %s", instances, branches, workers, elapsed)
}

func processGet(key string) *processv1.GetProcessInstanceRequest {
	return &processv1.GetProcessInstanceRequest{ProcessInstanceKey: key}
}
