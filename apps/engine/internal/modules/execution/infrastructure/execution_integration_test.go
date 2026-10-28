//go:build integration

package infrastructure_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgxpool"

	deploymentapp "github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/application"
	deploymentdomain "github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/domain"
	deploymentinfra "github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/infrastructure"
	deploymentifc "github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/interfaces"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/application"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/domain"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/infrastructure"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/infrastructure/db"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/interfaces"
	platformdb "github.com/indr622/workflow-engine/apps/engine/internal/platform/db"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/eventbus"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/keygen"
	deploymentv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/deployment/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/deployment/v1/deploymentv1connect"
	jobv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1/jobv1connect"
	processv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/process/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/process/v1/processv1connect"
)

type fixture struct {
	pool     *pgxpool.Pool
	repo     *infrastructure.PostgresRepository
	deploy   *deploymentapp.DeployResource
	cache    *deploymentapp.ProcessCache
	commands *application.Commands
	queries  *application.Queries
	bus      *eventbus.InMemory
	mux      *http.ServeMux
	run      string
}

type clock struct{}

func (clock) Now() time.Time { return time.Now().UTC() }

func setup(t *testing.T) *fixture {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := platformdb.Connect(ctx, platformdb.ConfigFromURL(url))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := platformdb.Migrate(ctx, pool, log); err != nil {
		t.Fatal(err)
	}
	keys := keygen.New()
	bus := eventbus.NewInMemory()
	deployRepo := deploymentinfra.NewPostgresRepository(pool, keys)
	cache := deploymentapp.NewProcessCache(deployRepo, 64)
	deploy := deploymentapp.NewDeployResource(deployRepo, bus, cache, nil, log)
	repo := infrastructure.NewPostgresRepository(pool)
	processor := domain.NewProcessor(domain.NewFeelEvaluator(), clock{}, domain.Limits{})
	commands := application.NewCommands(repo, cache, processor, infrastructure.NewKeyGenerator(keys), bus, log, application.Config{})
	queries := application.NewQueries(repo)
	mux := http.NewServeMux()
	deploymentifc.NewHandler(deploy, deploymentapp.NewQueries(deployRepo)).Register(mux)
	interfaces.NewHandler(commands, commands, queries).Register(mux)
	return &fixture{
		pool: pool, repo: repo, deploy: deploy, cache: cache, commands: commands, queries: queries, bus: bus, mux: mux,
		run: fmt.Sprintf("%d", time.Now().UnixNano()),
	}
}

// deployXML men-deploy satu model dan mengembalikan bpmnProcessId uniknya.
func (f *fixture) deployXML(t *testing.T, name, xml string) string {
	t.Helper()
	id := name + "-" + f.run
	_, err := f.deploy.Execute(context.Background(), deploymentdomain.DefaultTenant, []deploymentdomain.ResourceInput{
		{Name: id + ".bpmn", Content: []byte(strings.ReplaceAll(xml, "PID", id))},
	})
	if err != nil {
		t.Fatalf("deploy %s: %v", name, err)
	}
	return id
}

const header = `<?xml version="1.0" encoding="UTF-8"?>
<bpmn:definitions xmlns:bpmn="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:zeebe="http://camunda.org/schema/zeebe/1.0"
  xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" id="defs" targetNamespace="http://bpmn.io/schema/bpmn">
<bpmn:process id="PID" isExecutable="true">`

const footer = `</bpmn:process></bpmn:definitions>`

func serviceTask(id, jobType string) string {
	return fmt.Sprintf(`<bpmn:serviceTask id=%q><bpmn:extensionElements><zeebe:taskDefinition type=%q/><zeebe:taskHeaders><zeebe:header key="k" value="v"/></zeebe:taskHeaders></bpmn:extensionElements></bpmn:serviceTask>`, id, jobType)
}

func flow(id, from, to string) string {
	return fmt.Sprintf(`<bpmn:sequenceFlow id=%q sourceRef=%q targetRef=%q/>`, id, from, to)
}

var linearXML = header + `<bpmn:startEvent id="start"/>` + serviceTask("pay", "payment") + `<bpmn:endEvent id="end"/>` +
	flow("f1", "start", "pay") + flow("f2", "pay", "end") + footer

func forkXML(n int) string {
	var b strings.Builder
	b.WriteString(header + `<bpmn:startEvent id="start"/><bpmn:parallelGateway id="fork"/><bpmn:parallelGateway id="join"/><bpmn:endEvent id="end"/>`)
	b.WriteString(flow("f0", "start", "fork") + flow("fz", "join", "end"))
	for i := 0; i < n; i++ {
		task := fmt.Sprintf("t%d", i)
		b.WriteString(serviceTask(task, "branch"))
		b.WriteString(flow("a"+task, "fork", task) + flow("b"+task, task, "join"))
	}
	b.WriteString(footer)
	return b.String()
}

var incidentXML = header + `<bpmn:startEvent id="start"/><bpmn:parallelGateway id="fork"/>` + serviceTask("wait", "wait") +
	`<bpmn:exclusiveGateway id="gw"/><bpmn:endEvent id="end"/>` +
	flow("f1", "start", "fork") + flow("f2", "fork", "wait") + flow("f3", "fork", "gw") +
	`<bpmn:sequenceFlow id="f4" sourceRef="gw" targetRef="end"><bpmn:conditionExpression xsi:type="bpmn:tFormalExpression">=amount &gt; 10</bpmn:conditionExpression></bpmn:sequenceFlow>` +
	footer

func TestEndToEndOverConnect(t *testing.T) {
	f := setup(t)
	processID := f.deployXML(t, "e2e", linearXML)
	srv := httptest.NewServer(f.mux)
	defer srv.Close()
	ctx := context.Background()
	instances := processv1connect.NewProcessInstanceServiceClient(srv.Client(), srv.URL)
	jobs := jobv1connect.NewJobServiceClient(srv.Client(), srv.URL)
	defs := deploymentv1connect.NewDeploymentServiceClient(srv.Client(), srv.URL)

	created, err := instances.CreateProcessInstance(ctx, connect.NewRequest(&processv1.CreateProcessInstanceRequest{
		Definition: &processv1.CreateProcessInstanceRequest_BpmnProcess{BpmnProcess: &processv1.ProcessDefinitionRef{BpmnProcessId: processID}},
		Variables:  `{"orderId": "A-1", "amount": 9007199254740993}`,
	}))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	piKey := created.Msg.GetProcessInstanceKey()
	if !keygen.Valid(piKey) || created.Msg.GetState() != processv1.ProcessInstanceState_PROCESS_INSTANCE_STATE_ACTIVE {
		t.Fatalf("create response = %+v", created.Msg)
	}
	if _, err := defs.GetProcessDefinition(ctx, connect.NewRequest(&deploymentv1.GetProcessDefinitionRequest{ProcessDefinitionKey: created.Msg.GetProcessDefinitionKey()})); err != nil {
		t.Fatalf("definition of created instance: %v", err)
	}

	open, err := jobs.ListJobs(ctx, connect.NewRequest(&jobv1.ListJobsRequest{ProcessInstanceKey: piKey, OnlyOpen: true}))
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(open.Msg.GetJobs()) != 1 || open.Msg.GetJobs()[0].GetType() != "payment" || open.Msg.GetJobs()[0].GetCustomHeaders()["k"] != "v" {
		t.Fatalf("open jobs = %+v", open.Msg.GetJobs())
	}
	jobKey := open.Msg.GetJobs()[0].GetJobKey()

	if _, err := jobs.CompleteJob(ctx, connect.NewRequest(&jobv1.CompleteJobRequest{JobKey: jobKey, Variables: `{"paid": true}`})); err != nil {
		t.Fatalf("complete: %v", err)
	}
	got, err := instances.GetProcessInstance(ctx, connect.NewRequest(&processv1.GetProcessInstanceRequest{ProcessInstanceKey: piKey}))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Msg.GetProcessInstance().GetState() != processv1.ProcessInstanceState_PROCESS_INSTANCE_STATE_COMPLETED || got.Msg.GetProcessInstance().GetEndTime() == nil {
		t.Fatalf("instance = %+v", got.Msg.GetProcessInstance())
	}
	vars := map[string]string{}
	for _, v := range got.Msg.GetVariables() {
		vars[v.GetName()] = v.GetValue()
	}
	if vars["amount"] != "9007199254740993" || vars["paid"] != "true" || vars["orderId"] != `"A-1"` {
		t.Fatalf("variables = %v", vars)
	}
	var completed []string
	for _, e := range got.Msg.GetElementInstances() {
		if e.GetState() == processv1.ElementInstanceState_ELEMENT_INSTANCE_STATE_COMPLETED {
			completed = append(completed, e.GetElementId())
		}
	}
	sort.Strings(completed)
	want := []string{processID, "start", "pay", "end"}
	sort.Strings(want)
	if strings.Join(completed, ",") != strings.Join(want, ",") {
		t.Fatalf("completed elements = %v", completed)
	}
	if len(got.Msg.GetJobs()) != 1 || got.Msg.GetJobs()[0].GetState() != "COMPLETED" {
		t.Fatalf("jobs = %+v", got.Msg.GetJobs())
	}

	listed, err := instances.ListProcessInstances(ctx, connect.NewRequest(&processv1.ListProcessInstancesRequest{BpmnProcessId: processID}))
	if err != nil || len(listed.Msg.GetProcessInstances()) != 1 {
		t.Fatalf("list instances = %v, %v", listed, err)
	}

	expectCode(t, "complete twice", connect.CodeFailedPrecondition, func() error {
		_, err := jobs.CompleteJob(ctx, connect.NewRequest(&jobv1.CompleteJobRequest{JobKey: jobKey}))
		return err
	})
	expectCode(t, "cancel completed", connect.CodeFailedPrecondition, func() error {
		_, err := instances.CancelProcessInstance(ctx, connect.NewRequest(&processv1.CancelProcessInstanceRequest{ProcessInstanceKey: piKey}))
		return err
	})
	expectCode(t, "unknown instance", connect.CodeNotFound, func() error {
		k, _ := keygen.New().NewKey()
		_, err := instances.GetProcessInstance(ctx, connect.NewRequest(&processv1.GetProcessInstanceRequest{ProcessInstanceKey: k}))
		return err
	})
	expectCode(t, "unknown job", connect.CodeNotFound, func() error {
		k, _ := keygen.New().NewKey()
		_, err := jobs.CompleteJob(ctx, connect.NewRequest(&jobv1.CompleteJobRequest{JobKey: k}))
		return err
	})
	expectCode(t, "malformed key", connect.CodeInvalidArgument, func() error {
		_, err := instances.GetProcessInstance(ctx, connect.NewRequest(&processv1.GetProcessInstanceRequest{ProcessInstanceKey: "42"}))
		return err
	})
	expectCode(t, "invalid variables", connect.CodeInvalidArgument, func() error {
		_, err := instances.CreateProcessInstance(ctx, connect.NewRequest(&processv1.CreateProcessInstanceRequest{
			Definition: &processv1.CreateProcessInstanceRequest_BpmnProcess{BpmnProcess: &processv1.ProcessDefinitionRef{BpmnProcessId: processID}},
			Variables:  `[1, 2]`,
		}))
		return err
	})
	expectCode(t, "unknown process", connect.CodeNotFound, func() error {
		_, err := instances.CreateProcessInstance(ctx, connect.NewRequest(&processv1.CreateProcessInstanceRequest{
			Definition: &processv1.CreateProcessInstanceRequest_BpmnProcess{BpmnProcess: &processv1.ProcessDefinitionRef{BpmnProcessId: "missing-" + f.run}},
		}))
		return err
	})
	expectCode(t, "missing definition", connect.CodeInvalidArgument, func() error {
		_, err := instances.CreateProcessInstance(ctx, connect.NewRequest(&processv1.CreateProcessInstanceRequest{}))
		return err
	})
}

func expectCode(t *testing.T, name string, want connect.Code, call func() error) {
	t.Helper()
	err := call()
	if connect.CodeOf(err) != want {
		t.Fatalf("%s: code = %v (%v), want %v", name, connect.CodeOf(err), err, want)
	}
}

// TestParallelCompleteJobsJoinOnce: 50 CompleteJob paralel pada satu instance (fork 50 cabang)
// diserialkan row lock, join aktif tepat sekali, tanpa deadlock dan tanpa record ganda.
func TestParallelCompleteJobsJoinOnce(t *testing.T) {
	f := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	processID := f.deployXML(t, "fork50", forkXML(50))
	created, err := f.commands.CreateProcessInstance(ctx, api.CreateInstanceRequest{BpmnProcessID: processID})
	if err != nil {
		t.Fatal(err)
	}
	jobs, _, err := f.queries.ListJobs(ctx, application.JobFilter{ProcessInstanceKey: created.ProcessInstanceKey, OnlyOpen: true, Limit: 100})
	if err != nil || len(jobs) != 50 {
		t.Fatalf("open jobs = %d, %v", len(jobs), err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, len(jobs))
	for _, j := range jobs {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			errs <- f.commands.CompleteJob(ctx, key, []byte(`{"last": "`+key+`"}`))
		}(j.Key)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("complete: %v", err)
		}
	}
	detail, err := f.queries.GetInstance(ctx, created.ProcessInstanceKey)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Instance.State != "COMPLETED" {
		t.Fatalf("state = %s", detail.Instance.State)
	}
	joins := 0
	for _, e := range detail.Elements {
		if e.ElementID == "join" {
			joins++
		}
	}
	if joins != 1 {
		t.Fatalf("join element instances = %d, want 1", joins)
	}
	records := loadRecords(t, f.pool, created.ProcessInstanceKey)
	for i, r := range records {
		if r.seq != int32(i+1) {
			t.Fatalf("record %d has sequence %d: gap or duplicate", i, r.seq)
		}
	}
}

func TestCreateManyInstancesConcurrently(t *testing.T) {
	f := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	processID := f.deployXML(t, "many", linearXML)
	var mu sync.Mutex
	seen := map[string]bool{}
	var wg sync.WaitGroup
	errs := make(chan error, 200)
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res, err := f.commands.CreateProcessInstance(ctx, api.CreateInstanceRequest{BpmnProcessID: processID, Variables: []byte(fmt.Sprintf(`{"i": %d}`, i))})
			if err != nil {
				errs <- err
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if seen[res.ProcessInstanceKey] {
				errs <- fmt.Errorf("duplicate key %s", res.ProcessInstanceKey)
			}
			seen[res.ProcessInstanceKey] = true
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if len(seen) != 200 {
		t.Fatalf("created %d instances", len(seen))
	}
}

// TestReplayMatchesProjection: Apply ulang record dari database menghasilkan state yang sama dengan
// state runtime yang dimuat dari proyeksi (fondasi replay Fase 3), termasuk incident dan cancel.
func TestReplayMatchesProjection(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	processID := f.deployXML(t, "replay", incidentXML)
	var incidents []api.IncidentCreated
	var mu sync.Mutex
	f.bus.Subscribe(api.TopicIncidentCreated, func(_ context.Context, ev eventbus.Event) error {
		mu.Lock()
		defer mu.Unlock()
		incidents = append(incidents, ev.(api.IncidentCreated))
		return nil
	})
	created, err := f.commands.CreateProcessInstance(ctx, api.CreateInstanceRequest{BpmnProcessID: processID, Variables: []byte(`{"amount": 5}`)})
	if err != nil {
		t.Fatal(err)
	}
	assertReplay(t, f, created.ProcessInstanceKey, true)
	if len(incidents) != 1 || incidents[0].ElementID != "gw" {
		t.Fatalf("incident events = %+v", incidents)
	}
	detail, err := f.queries.GetInstance(ctx, created.ProcessInstanceKey)
	if err != nil || !detail.Instance.HasIncident {
		t.Fatalf("has_incident = %v, %v", detail.Instance.HasIncident, err)
	}

	if err := f.commands.CancelProcessInstance(ctx, created.ProcessInstanceKey); err != nil {
		t.Fatal(err)
	}
	assertReplay(t, f, created.ProcessInstanceKey, false)
	detail, err = f.queries.GetInstance(ctx, created.ProcessInstanceKey)
	if err != nil || detail.Instance.State != "TERMINATED" || detail.Instance.HasIncident {
		t.Fatalf("after cancel = %+v, %v", detail.Instance, err)
	}
	for _, j := range detail.Jobs {
		if j.State != "CANCELED" {
			t.Fatalf("job %s state %s", j.ElementID, j.State)
		}
	}
	for _, inc := range detail.Incidents {
		if !inc.Resolved {
			t.Fatalf("incident %s not resolved", inc.Key)
		}
	}
}

type storedRecord struct {
	seq int32
	rec domain.Record
}

func loadRecords(t *testing.T, pool *pgxpool.Pool, piKey string) []storedRecord {
	t.Helper()
	rows, err := db.New(pool).ListRecords(context.Background(), piKey)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]storedRecord, 0, len(rows))
	for _, row := range rows {
		value, ok := domain.NewRecordValue(domain.ValueType(row.ValueType))
		if !ok {
			t.Fatalf("unknown value type %s", row.ValueType)
		}
		if err := json.Unmarshal(row.Payload, value); err != nil {
			t.Fatal(err)
		}
		out = append(out, storedRecord{seq: row.Sequence, rec: domain.Record{
			Key: row.RecordKey, ProcessInstanceKey: row.ProcessInstanceKey, ValueType: domain.ValueType(row.ValueType),
			Intent: domain.Intent(row.Intent), Timestamp: row.CreatedAt.Time, Value: value,
		}})
	}
	return out
}

func assertReplay(t *testing.T, f *fixture, piKey string, active bool) {
	t.Helper()
	ctx := context.Background()
	var snap *domain.InstanceSnapshot
	err := f.repo.WithTx(ctx, func(ctx context.Context, tx domain.TxRepository) error {
		var err error
		snap, err = tx.LockAndLoad(ctx, piKey)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	_, proc, err := f.cache.GetByKey(ctx, snap.Definition.Key)
	if err != nil {
		t.Fatal(err)
	}
	projected := domain.Restore(snap, proc)
	replayed := domain.NewInstanceState(domain.DefinitionRef{}, proc)
	for _, r := range loadRecords(t, f.pool, piKey) {
		if err := domain.Apply(replayed, &r.rec); err != nil {
			t.Fatalf("replay seq %d: %v", r.seq, err)
		}
	}
	if (projected.Status == domain.InstanceActive) != active {
		t.Fatalf("status = %s, active want %v", projected.Status, active)
	}
	if !reflect.DeepEqual(view(replayed), view(projected)) {
		t.Fatalf("replay differs from projection\nreplayed:  %+v\nprojected: %+v", view(replayed), view(projected))
	}
}

func view(s *domain.InstanceState) map[string]any {
	elements := []domain.ElementInstance{}
	for _, e := range s.Elements {
		elements = append(elements, *e)
	}
	sort.Slice(elements, func(i, j int) bool { return elements[i].Key < elements[j].Key })
	jobs := []domain.Job{}
	for _, j := range s.Jobs {
		c := *j
		if c.CustomHeaders == nil {
			c.CustomHeaders = nil
		}
		jobs = append(jobs, c)
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].Key < jobs[j].Key })
	incidents := []domain.Incident{}
	for _, inc := range s.Incidents {
		incidents = append(incidents, *inc)
	}
	sort.Slice(incidents, func(i, j int) bool { return incidents[i].Key < incidents[j].Key })
	vars := map[string]string{}
	for _, v := range s.SortedVariables() {
		vars[v.Name] = string(v.Value)
	}
	return map[string]any{
		"key": s.Key, "definition": s.Definition, "status": s.Status, "elements": elements,
		"flows": s.SortedTakenFlows(), "variables": vars, "jobs": jobs, "incidents": incidents,
	}
}

func linear20XML() string {
	var b strings.Builder
	b.WriteString(header + `<bpmn:startEvent id="start"/><bpmn:endEvent id="end"/>`)
	prev := "start"
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("t%d", i)
		b.WriteString(serviceTask(id, "work") + flow("f"+id, prev, id))
		prev = id
	}
	b.WriteString(flow("fend", prev, "end") + footer)
	return b.String()
}

// TestLinear20Latency mengukur create + 20 CompleteJob berurutan terhadap Postgres lokal dan
// mencetak p50/p95 (target DoD: p50 < 50 ms total).
func TestLinear20Latency(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	processID := f.deployXML(t, "linear20", linear20XML())
	const runs = 30
	durations := make([]time.Duration, 0, runs)
	for i := 0; i < runs; i++ {
		// Hanya waktu command yang diukur; ListJobs untuk mencari job berikutnya tidak dihitung.
		var spent time.Duration
		start := time.Now()
		created, err := f.commands.CreateProcessInstance(ctx, api.CreateInstanceRequest{BpmnProcessID: processID})
		if err != nil {
			t.Fatal(err)
		}
		spent += time.Since(start)
		for j := 0; j < 20; j++ {
			jobs, _, err := f.queries.ListJobs(ctx, application.JobFilter{ProcessInstanceKey: created.ProcessInstanceKey, OnlyOpen: true, Limit: 1})
			if err != nil || len(jobs) != 1 {
				t.Fatalf("step %d: jobs %d, %v", j, len(jobs), err)
			}
			start = time.Now()
			if err := f.commands.CompleteJob(ctx, jobs[0].Key, nil); err != nil {
				t.Fatal(err)
			}
			spent += time.Since(start)
		}
		durations = append(durations, spent)
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p50, p95 := durations[runs/2], durations[runs*95/100]
	t.Logf("create + 20 CompleteJob: p50=%v p95=%v", p50, p95)
	// Target 50 ms berlaku untuk Postgres native (round trip ~0,1 ms). Di Docker Desktop Windows
	// satu round trip ~1,2 ms, jadi di sini hanya sanity check agar regresi besar tetap tertangkap.
	if p50 > time.Second {
		t.Fatalf("p50 %v: regression, a command should take a few round trips", p50)
	}
}
