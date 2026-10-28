package domain_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/domain"
)

func TestLinearPassThroughCompletesInOneCommand(t *testing.T) {
	proc := newModel("linear").
		node("start", "startEvent").node("task", "task").node("manual", "manualTask").node("end", "endEvent").
		flow("f1", "start", "task", "").flow("f2", "task", "manual", "").flow("f3", "manual", "end", "").
		build(t)
	h := startInstance(t, proc, `{"a": 1}`)
	h.expectStatus(domain.InstanceCompleted)
	equalStrings(t, "completed", h.completedElements(), []string{"start", "task", "manual", "end"})

	var got []string
	for _, r := range h.all[:12] {
		got = append(got, string(r.ValueType)+" "+string(r.Intent)+" "+r.ElementID())
	}
	want := []string{
		"PROCESS_INSTANCE ELEMENT_ACTIVATING linear",
		"VARIABLE CREATED ",
		"PROCESS_INSTANCE_CREATION CREATED ",
		"PROCESS_INSTANCE ELEMENT_ACTIVATED linear",
		"PROCESS_INSTANCE ELEMENT_ACTIVATING start",
		"PROCESS_INSTANCE ELEMENT_ACTIVATED start",
		"PROCESS_INSTANCE ELEMENT_COMPLETING start",
		"PROCESS_INSTANCE ELEMENT_COMPLETED start",
		"PROCESS_INSTANCE SEQUENCE_FLOW_TAKEN f1",
		"PROCESS_INSTANCE ELEMENT_ACTIVATING task",
		"PROCESS_INSTANCE ELEMENT_ACTIVATED task",
		"PROCESS_INSTANCE ELEMENT_COMPLETING task",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("record order:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	lastRec := h.all[len(h.all)-1]
	if lastRec.Intent != domain.IntentElementCompleted || lastRec.ElementID() != "linear" {
		t.Fatalf("last record = %s %s, want process completed", lastRec.Intent, lastRec.ElementID())
	}
	for _, r := range h.all {
		if r.ProcessInstanceKey != h.st.Key {
			t.Fatalf("record %s %s has process instance key %d, want %d", r.Intent, r.ElementID(), r.ProcessInstanceKey, h.st.Key)
		}
	}
	h.assertReplay()
}

func TestImplicitEndCompletesProcess(t *testing.T) {
	proc := newModel("implicit").node("start", "startEvent").node("task", "task").flow("f1", "start", "task", "").build(t)
	h := startInstance(t, proc, `{}`)
	h.expectStatus(domain.InstanceCompleted)
	equalStrings(t, "completed", h.completedElements(), []string{"start", "task"})
	h.assertReplay()
}

func TestServiceTaskCreatesJobAndContinuesAfterCompletion(t *testing.T) {
	proc := newModel("svc").
		node("start", "startEvent").service("pay", "payment", "", "region", "eu", "tier", "gold").node("end", "endEvent").
		flow("f1", "start", "pay", "").flow("f2", "pay", "end", "").
		build(t)
	h := startInstance(t, proc, `{"amount": 9007199254740993, "note": "x"}`)
	h.expectStatus(domain.InstanceActive)
	job := h.openJob("pay")
	if job.Type != "payment" || job.Retries != domain.DefaultJobRetries {
		t.Fatalf("job = %+v, want type payment retries 3", job)
	}
	if len(job.CustomHeaders) != 2 || job.CustomHeaders[0].Key != "region" || job.CustomHeaders[1].Value != "gold" {
		t.Fatalf("headers = %+v", job.CustomHeaders)
	}
	if el := h.st.Elements[job.ElementInstanceKey]; el == nil || el.State != domain.ElementActivated {
		t.Fatalf("service task element = %+v, want ACTIVATED", el)
	}

	h.complete("pay", `{"note": "x", "approved": true, "amount": 1}`)
	h.expectStatus(domain.InstanceCompleted)
	if got := h.variablesJSON(); got != `{"amount":1,"approved":true,"note":"x"}` {
		t.Fatalf("variables = %s", got)
	}
	// note tidak berubah → tanpa record; amount berubah → UPDATED; approved baru → CREATED.
	var intents []string
	for _, r := range h.last {
		if r.ValueType == domain.ValueVariable {
			intents = append(intents, r.Value.(*domain.VariableValue).Name+":"+string(r.Intent))
		}
	}
	if strings.Join(intents, ",") != "approved:CREATED,amount:UPDATED" && strings.Join(intents, ",") != "amount:UPDATED,approved:CREATED" {
		t.Fatalf("variable records = %v", intents)
	}
	h.assertReplay()

	expectErr(t, h.run(domain.CompleteJob{JobKey: job.Key}), domain.ErrInstanceNotActive)
}

func TestCompleteJobTwiceIsRejected(t *testing.T) {
	proc := newModel("twice").
		node("start", "startEvent").service("a", "a", "").service("b", "b", "").
		flow("f1", "start", "a", "").flow("f2", "a", "b", "").
		build(t)
	h := startInstance(t, proc, `{}`)
	job := h.openJob("a")
	h.complete("a", `{}`)
	expectErr(t, h.run(domain.CompleteJob{JobKey: job.Key}), domain.ErrJobNotActive)
	h.assertReplay()
}

func TestJobTypeAndRetriesFromExpressions(t *testing.T) {
	proc := newModel("expr").
		node("start", "startEvent").service("dyn", "=jobType", "=tries").
		flow("f1", "start", "dyn", "").
		build(t)
	h := startInstance(t, proc, `{"jobType": "dynamic", "tries": 7}`)
	job := h.openJob("dyn")
	if job.Type != "dynamic" || job.Retries != 7 {
		t.Fatalf("job = %+v", job)
	}
	h.assertReplay()
}

func TestJobTypeExpressionFailureRaisesExtractValueIncident(t *testing.T) {
	proc := newModel("bad").
		node("start", "startEvent").service("dyn", "=jobType", "").
		flow("f1", "start", "dyn", "").
		build(t)
	h := startInstance(t, proc, `{"jobType": 42}`)
	h.expectStatus(domain.InstanceActive)
	equalStrings(t, "incidents", h.incidentTypes(), []string{"EXTRACT_VALUE_ERROR@dyn"})
	equalStrings(t, "active", h.activeElements(), []string{"dyn:ACTIVATING"})
	if len(h.st.Jobs) != 0 {
		t.Fatalf("no job expected, got %v", h.openJobElements())
	}
	h.assertReplay()
}

func exclusiveModel(t testing.TB, withDefault bool) *harness {
	b := newModel("excl").
		node("start", "startEvent").node("big", "endEvent").node("small", "endEvent")
	def := ""
	if withDefault {
		def = "fSmall"
	}
	b.gateway("gw", "exclusiveGateway", def).
		flow("f1", "start", "gw", "").
		flow("fBig", "gw", "big", "=amount > 100")
	if withDefault {
		b.flow("fSmall", "gw", "small", "")
	} else {
		b.flow("fSmall", "gw", "small", "=amount <= 10")
	}
	return &harness{proc: b.build(t)}
}

func TestExclusiveGateway(t *testing.T) {
	tests := []struct {
		name        string
		withDefault bool
		vars        string
		status      domain.InstanceStatus
		completed   []string
		incident    string
	}{
		{"condition true", true, `{"amount": 500}`, domain.InstanceCompleted, []string{"start", "gw", "big"}, ""},
		{"default flow", true, `{"amount": 50}`, domain.InstanceCompleted, []string{"start", "gw", "small"}, ""},
		{"second condition", false, `{"amount": 5}`, domain.InstanceCompleted, []string{"start", "gw", "small"}, ""},
		{"no match without default", false, `{"amount": 50}`, domain.InstanceActive, []string{"start"}, "CONDITION_ERROR@gw"},
		{"null condition", false, `{}`, domain.InstanceActive, []string{"start"}, "CONDITION_ERROR@gw"},
		{"non-boolean comparison", true, `{"amount": "many"}`, domain.InstanceActive, []string{"start"}, "CONDITION_ERROR@gw"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := startInstance(t, exclusiveModel(t, tt.withDefault).proc, tt.vars)
			h.expectStatus(tt.status)
			equalStrings(t, "completed", h.completedElements(), tt.completed)
			if tt.incident != "" {
				equalStrings(t, "incidents", h.incidentTypes(), []string{tt.incident})
				equalStrings(t, "active", h.activeElements(), []string{"gw:ACTIVATING"})
			}
			h.assertReplay()
		})
	}
}

func TestExclusiveGatewayEvaluatesConditionsInDocumentOrder(t *testing.T) {
	proc := newModel("order").
		node("start", "startEvent").gateway("gw", "exclusiveGateway", "").node("a", "endEvent").node("b", "endEvent").
		flow("f1", "start", "gw", "").
		flow("fa", "gw", "a", "=x > 1").
		flow("fb", "gw", "b", "=x > 0").
		build(t)
	h := startInstance(t, proc, `{"x": 5}`)
	equalStrings(t, "completed", h.completedElements(), []string{"start", "gw", "a"})
}

func parallelModel(t testing.TB) *harness {
	return &harness{proc: newModel("par").
		node("start", "startEvent").gateway("fork", "parallelGateway", "").
		service("a", "a", "").service("b", "b", "").service("c", "c", "").
		gateway("join", "parallelGateway", "").node("end", "endEvent").
		flow("f1", "start", "fork", "").
		flow("fa", "fork", "a", "").flow("fb", "fork", "b", "").flow("fc", "fork", "c", "").
		flow("ja", "a", "join", "").flow("jb", "b", "join", "").flow("jc", "c", "join", "").
		flow("f2", "join", "end", "").
		build(t)}
}

func TestParallelForkJoinInAnyOrder(t *testing.T) {
	for _, order := range [][]string{{"a", "b", "c"}, {"c", "b", "a"}, {"b", "a", "c"}} {
		t.Run(strings.Join(order, ""), func(t *testing.T) {
			h := startInstance(t, parallelModel(t).proc, `{}`)
			equalStrings(t, "open jobs", h.openJobElements(), []string{"a", "b", "c"})
			for i, el := range order {
				h.complete(el, `{}`)
				if i < len(order)-1 {
					h.expectStatus(domain.InstanceActive)
					if h.countIntent("join", domain.IntentElementActivating) != 0 {
						t.Fatalf("join activated after %d of 3 branches", i+1)
					}
				}
			}
			h.expectStatus(domain.InstanceCompleted)
			equalStrings(t, "job types", h.createdJobTypes(), []string{"a", "b", "c"})
			if n := h.countIntent("join", domain.IntentElementActivating); n != 1 {
				t.Fatalf("join activated %d times, want 1", n)
			}
			if len(h.st.TakenFlows) != 0 {
				t.Fatalf("leftover counters: %v", h.st.TakenFlows)
			}
			h.assertReplay()
		})
	}
}

func TestParallelWithoutWaitStatesJoinsInSameCommand(t *testing.T) {
	proc := newModel("parfast").
		node("start", "startEvent").gateway("fork", "parallelGateway", "").
		node("a", "task").node("b", "manualTask").
		gateway("join", "parallelGateway", "").node("end", "endEvent").
		flow("f1", "start", "fork", "").flow("fa", "fork", "a", "").flow("fb", "fork", "b", "").
		flow("ja", "a", "join", "").flow("jb", "b", "join", "").flow("f2", "join", "end", "").
		build(t)
	h := startInstance(t, proc, `{}`)
	h.expectStatus(domain.InstanceCompleted)
	if n := h.countIntent("join", domain.IntentElementCompleted); n != 1 {
		t.Fatalf("join completed %d times", n)
	}
	h.assertReplay()
}

func TestLoopThroughExclusiveGatewayWithServiceTask(t *testing.T) {
	proc := newModel("loop").
		node("start", "startEvent").gateway("merge", "exclusiveGateway", "").service("work", "work", "").
		gateway("check", "exclusiveGateway", "").node("end", "endEvent").
		flow("f1", "start", "merge", "").flow("f2", "merge", "work", "").flow("f3", "work", "check", "").
		flow("again", "check", "merge", "=i < 3").flow("done", "check", "end", "=i >= 3").
		build(t)
	h := startInstance(t, proc, `{"i": 0}`)
	for i := 1; i <= 3; i++ {
		h.complete("work", `{"i": `+string(rune('0'+i))+`}`)
	}
	h.expectStatus(domain.InstanceCompleted)
	if n := h.countIntent("work", domain.IntentElementCompleted); n != 3 {
		t.Fatalf("work completed %d times, want 3", n)
	}
	h.assertReplay()
}

func TestParallelJoinCountsRepeatedTokens(t *testing.T) {
	// Token kedua lewat flow yang sama sebelum join lengkap tetap tercatat dan memicu join berikutnya.
	proc := newModel("repeat").
		node("start", "startEvent").gateway("fork", "parallelGateway", "").
		service("a", "a", "").service("b", "b", "").
		gateway("join", "parallelGateway", "").service("after", "after", "").
		flow("f1", "start", "fork", "").flow("fa", "fork", "a", "").flow("fb", "fork", "b", "").
		flow("ja", "a", "join", "").flow("jb", "b", "join", "").flow("f2", "join", "after", "").
		build(t)
	h := startInstance(t, proc, `{}`)
	h.complete("a", `{}`)
	h.complete("b", `{}`)
	if n := h.countIntent("join", domain.IntentElementActivating); n != 1 {
		t.Fatalf("join activated %d times, want 1", n)
	}
	h.openJob("after")
	h.assertReplay()
}

func TestExclusiveSplitIntoParallelJoinStaysActive(t *testing.T) {
	proc := newModel("stuck").
		node("start", "startEvent").gateway("split", "exclusiveGateway", "fb").
		node("a", "task").node("b", "task").
		gateway("join", "parallelGateway", "").node("end", "endEvent").
		flow("f1", "start", "split", "").flow("fa", "split", "a", "=go").flow("fb", "split", "b", "").
		flow("ja", "a", "join", "").flow("jb", "b", "join", "").flow("f2", "join", "end", "").
		build(t)
	h := startInstance(t, proc, `{"go": true}`)
	h.expectStatus(domain.InstanceActive)
	if len(h.activeElements()) != 0 {
		t.Fatalf("no active elements expected, got %v", h.activeElements())
	}
	if len(h.st.TakenFlows) != 1 {
		t.Fatalf("want one waiting counter, got %v", h.st.TakenFlows)
	}
	h.assertReplay()
}

func TestCancelTerminatesChildrenCancelsJobsAndResolvesIncidents(t *testing.T) {
	proc := newModel("cancel").
		node("start", "startEvent").gateway("fork", "parallelGateway", "").
		service("a", "a", "").service("b", "b", "").gateway("gw", "exclusiveGateway", "").node("x", "endEvent").
		flow("f1", "start", "fork", "").flow("fa", "fork", "a", "").flow("fb", "fork", "b", "").
		flow("fg", "fork", "gw", "").flow("fx", "gw", "x", "=missing > 1").
		build(t)
	h := startInstance(t, proc, `{}`)
	equalStrings(t, "incidents", h.incidentTypes(), []string{"CONDITION_ERROR@gw"})
	h.mustRun(domain.CancelInstance{})
	h.expectStatus(domain.InstanceTerminated)
	if len(h.st.Jobs) != 0 || len(h.st.Incidents) != 0 || len(h.st.Elements) != 0 {
		t.Fatalf("leftovers after cancel: jobs=%v incidents=%v elements=%v", h.openJobElements(), h.incidentTypes(), h.activeElements())
	}
	var canceled, resolved, terminated int
	for _, r := range h.last {
		switch {
		case r.ValueType == domain.ValueJob && r.Intent == domain.IntentCanceled:
			canceled++
		case r.ValueType == domain.ValueIncident && r.Intent == domain.IntentResolved:
			resolved++
		case r.Intent == domain.IntentElementTerminated:
			terminated++
		}
	}
	if canceled != 2 || resolved != 1 || terminated != 4 {
		t.Fatalf("canceled=%d resolved=%d terminated=%d, want 2/1/4", canceled, resolved, terminated)
	}
	h.assertReplay()
	expectErr(t, h.run(domain.CancelInstance{}), domain.ErrInstanceNotActive)
}

func TestExecutionLimitRaisesIncident(t *testing.T) {
	proc := newModel("spin").
		node("start", "startEvent").gateway("merge", "exclusiveGateway", "").node("task", "task").
		gateway("check", "exclusiveGateway", "").node("end", "endEvent").
		flow("f1", "start", "merge", "").flow("f2", "merge", "task", "").flow("f3", "task", "check", "").
		flow("again", "check", "merge", "=true").flow("done", "check", "end", "=false").
		build(t)
	h := startWith(t, proc, `{}`, newProcessor(200))
	h.expectStatus(domain.InstanceActive)
	incidents := h.incidentTypes()
	if len(incidents) != 1 || !strings.HasPrefix(incidents[0], "EXECUTION_LIMIT_EXCEEDED@") {
		t.Fatalf("incidents = %v", incidents)
	}
	if len(h.all) > 210 {
		t.Fatalf("wrote %d records, limit 200", len(h.all))
	}
	h.assertReplay()
}

func TestProcessIsDeterministic(t *testing.T) {
	run := func() []domain.Record {
		h := startInstance(t, parallelModel(t).proc, `{"z": 1, "a": [1, 2]}`)
		h.complete("b", `{"r": "b"}`)
		h.complete("a", `{}`)
		h.complete("c", `{"r": "c"}`)
		return h.all
	}
	first, second := run(), run()
	if !reflect.DeepEqual(first, second) {
		t.Fatal("two runs with the same keys and clock produced different records")
	}
}

func TestKeysExhausted(t *testing.T) {
	proc := parallelModel(t).proc
	def := testDefinition
	def.BpmnProcessID = proc.ID()
	st := domain.NewInstanceState(def, proc)
	_, err := newProcessor(0).Process(st, domain.CreateInstance{}, domain.NewSliceKeys([]int64{1, 2, 3}))
	expectErr(t, err, domain.ErrKeysExhausted)
}

func TestUnknownJobIsNotActive(t *testing.T) {
	h := startInstance(t, parallelModel(t).proc, `{}`)
	expectErr(t, h.run(domain.CompleteJob{JobKey: 1}), domain.ErrJobNotActive)
}
