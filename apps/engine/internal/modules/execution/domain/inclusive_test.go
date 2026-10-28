package domain_test

import (
	"testing"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/domain"
	"github.com/indr622/workflow-engine/libs/go/bpmn"
)

// inclusiveModel: fork inclusive ke tiga cabang service task (a, b, c) dengan kondisi, default ke d,
// lalu join inclusive.
func inclusiveModel(t testing.TB) *bpmn.Process {
	return newModel("incl").
		node("start", "startEvent").gateway("fork", "inclusiveGateway", "fd").
		service("a", "a", "").service("b", "b", "").service("c", "c", "").service("d", "d", "").
		gateway("join", "inclusiveGateway", "").node("end", "endEvent").
		flow("f1", "start", "fork", "").
		flow("fa", "fork", "a", "=x > 0").flow("fb", "fork", "b", "=x > 10").flow("fc", "fork", "c", "=x > 100").
		flow("fd", "fork", "d", "").
		flow("ja", "a", "join", "").flow("jb", "b", "join", "").flow("jc", "c", "join", "").flow("jd", "d", "join", "").
		flow("f2", "join", "end", "").
		build(t)
}

func TestInclusiveGateway(t *testing.T) {
	tests := []struct {
		name     string
		vars     string
		branches []string
	}{
		{"all branches", `{"x": 500}`, []string{"a", "b", "c"}},
		{"some branches", `{"x": 50}`, []string{"a", "b"}},
		{"one branch", `{"x": 5}`, []string{"a"}},
		{"default flow", `{"x": -1}`, []string{"d"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := startInstance(t, inclusiveModel(t), tt.vars)
			equalStrings(t, "open jobs", h.openJobElements(), tt.branches)
			for i, b := range tt.branches {
				h.complete(b, `{}`)
				if i < len(tt.branches)-1 && h.countIntent("join", domain.IntentElementActivating) != 0 {
					t.Fatalf("join activated before all taken branches finished")
				}
			}
			h.expectStatus(domain.InstanceCompleted)
			if n := h.countIntent("join", domain.IntentElementActivating); n != 1 {
				t.Fatalf("join activated %d times", n)
			}
			if len(h.st.TakenFlows) != 0 {
				t.Fatalf("leftover counters %v", h.st.TakenFlows)
			}
			h.assertReplay()
		})
	}
}

func TestInclusiveGatewayWithoutMatchOrDefaultRaisesIncident(t *testing.T) {
	proc := newModel("inclnone").
		node("start", "startEvent").gateway("fork", "inclusiveGateway", "").node("a", "endEvent").node("b", "endEvent").
		flow("f1", "start", "fork", "").flow("fa", "fork", "a", "=x > 0").flow("fb", "fork", "b", "=x > 10").
		build(t)
	h := startInstance(t, proc, `{"x": -5}`)
	equalStrings(t, "incidents", h.incidentTypes(), []string{"CONDITION_ERROR@fork"})
	h.assertReplay()
}

func TestInclusiveJoinWaitsForUpstreamWaitState(t *testing.T) {
	// Cabang a langsung sampai di join; cabang b masih menunggu job di upstream, jadi join menunggu.
	proc := newModel("inclwait").
		node("start", "startEvent").gateway("fork", "inclusiveGateway", "").
		node("a", "task").service("b", "b", "").
		gateway("join", "inclusiveGateway", "").service("after", "after", "").
		flow("f1", "start", "fork", "").flow("fa", "fork", "a", "=true").flow("fb", "fork", "b", "=true").
		flow("ja", "a", "join", "").flow("jb", "b", "join", "").flow("f2", "join", "after", "").
		build(t)
	h := startInstance(t, proc, `{}`)
	if h.countIntent("join", domain.IntentElementActivating) != 0 {
		t.Fatal("join must wait for branch b")
	}
	h.complete("b", `{}`)
	h.openJob("after")
	if n := h.countIntent("join", domain.IntentElementActivating); n != 1 {
		t.Fatalf("join activated %d times", n)
	}
	h.assertReplay()
}

func TestInclusiveJoinNestedInParallel(t *testing.T) {
	proc := newModel("nested").
		node("start", "startEvent").gateway("pfork", "parallelGateway", "").
		gateway("ifork", "inclusiveGateway", "").service("a", "a", "").service("b", "b", "").
		gateway("ijoin", "inclusiveGateway", "").service("side", "side", "").
		gateway("pjoin", "parallelGateway", "").node("end", "endEvent").
		flow("f1", "start", "pfork", "").flow("p1", "pfork", "ifork", "").flow("p2", "pfork", "side", "").
		flow("ia", "ifork", "a", "=x > 1").flow("ib", "ifork", "b", "=x > 2").
		flow("ja", "a", "ijoin", "").flow("jb", "b", "ijoin", "").
		flow("q1", "ijoin", "pjoin", "").flow("q2", "side", "pjoin", "").flow("f2", "pjoin", "end", "").
		build(t)
	h := startInstance(t, proc, `{"x": 3}`)
	h.complete("side", `{}`)
	h.complete("b", `{}`)
	if h.countIntent("ijoin", domain.IntentElementActivating) != 0 {
		t.Fatal("inclusive join must wait for a")
	}
	h.complete("a", `{}`)
	h.expectStatus(domain.InstanceCompleted)
	h.assertReplay()
}

func TestInclusiveJoinInsideLoop(t *testing.T) {
	// Token yang akan kembali melewati join tidak dihitung upstream (BFS berhenti di join, ADR-0012).
	proc := newModel("inclloop").
		node("start", "startEvent").gateway("merge", "exclusiveGateway", "").gateway("fork", "inclusiveGateway", "").
		service("a", "a", "").service("b", "b", "").gateway("join", "inclusiveGateway", "").
		gateway("check", "exclusiveGateway", "").node("end", "endEvent").
		flow("f1", "start", "merge", "").flow("f2", "merge", "fork", "").
		flow("fa", "fork", "a", "=true").flow("fb", "fork", "b", "=round = 1").
		flow("ja", "a", "join", "").flow("jb", "b", "join", "").flow("f3", "join", "check", "").
		flow("again", "check", "merge", "=round = 2").flow("done", "check", "end", "=round != 2").
		build(t)
	h := startInstance(t, proc, `{"round": 1}`)
	h.complete("b", `{"round": 2}`)
	if h.countIntent("join", domain.IntentElementActivating) != 0 {
		t.Fatal("join must wait for a in the first round")
	}
	h.complete("a", `{}`)
	// Putaran kedua: hanya cabang a yang diambil (round = 2), jadi join tidak menunggu b.
	equalStrings(t, "open jobs", h.openJobElements(), []string{"a"})
	h.complete("a", `{"round": 3}`)
	h.expectStatus(domain.InstanceCompleted)
	if n := h.countIntent("join", domain.IntentElementActivating); n != 2 {
		t.Fatalf("join activated %d times, want 2", n)
	}
	h.assertReplay()
}

func TestApplyRejectsIllegalTransitions(t *testing.T) {
	states := []domain.ElementState{
		domain.ElementActivating, domain.ElementActivated, domain.ElementCompleting,
		domain.ElementCompleted, domain.ElementTerminating, domain.ElementTerminated,
	}
	allowed := map[[2]domain.ElementState]bool{
		{domain.ElementActivating, domain.ElementActivated}:   true,
		{domain.ElementActivating, domain.ElementTerminating}: true,
		{domain.ElementActivated, domain.ElementCompleting}:   true,
		{domain.ElementActivated, domain.ElementTerminating}:  true,
		{domain.ElementCompleting, domain.ElementCompleted}:   true,
		{domain.ElementCompleting, domain.ElementTerminating}: true,
		{domain.ElementTerminating, domain.ElementTerminated}: true,
	}
	for _, from := range states {
		for _, to := range states {
			if got := domain.CanTransition(from, to); got != allowed[[2]domain.ElementState{from, to}] {
				t.Errorf("CanTransition(%s, %s) = %v", from, to, got)
			}
		}
	}

	proc := newModel("p").node("start", "startEvent").service("a", "a", "").flow("f1", "start", "a", "").build(t)
	h := startInstance(t, proc, `{}`)
	job := h.openJob("a")
	bad := domain.Record{
		Key: job.ElementInstanceKey, ProcessInstanceKey: h.st.Key, ValueType: domain.ValueProcessInstance,
		Intent: domain.IntentElementCompleted,
		Value:  &domain.ProcessInstanceValue{ElementID: "a", BpmnElementType: "SERVICE_TASK", FlowScopeKey: h.st.Key},
	}
	expectErr(t, domain.Apply(h.st, &bad), domain.ErrInvalidRecord)
	if h.st.Elements[job.ElementInstanceKey].State != domain.ElementActivated {
		t.Fatal("state changed after a rejected record")
	}
}

func TestParseVariables(t *testing.T) {
	tests := []struct {
		raw     string
		names   string
		wantErr bool
	}{
		{``, "", false},
		{`  `, "", false},
		{`{}`, "", false},
		{`{"b": 1, "a": {"x": [1, 2]}, "b": 2}`, "b=2,a={\"x\":[1,2]}", false},
		{`{"n": 9007199254740993}`, "n=9007199254740993", false},
		{`[]`, "", true},
		{`{"": 1}`, "", true},
		{`{"a": }`, "", true},
		{`{"a": 1} {}`, "", true},
		{`"text"`, "", true},
	}
	for _, tt := range tests {
		vars, err := domain.ParseVariables([]byte(tt.raw), 0)
		if tt.wantErr {
			if err == nil {
				t.Errorf("%q: want error", tt.raw)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", tt.raw, err)
			continue
		}
		got := ""
		for i, v := range vars {
			if i > 0 {
				got += ","
			}
			got += v.Name + "=" + string(v.Value)
		}
		if got != tt.names {
			t.Errorf("%q: got %s, want %s", tt.raw, got, tt.names)
		}
	}
	if _, err := domain.ParseVariables([]byte(`{"a": 1}`), 4); err == nil {
		t.Error("size limit not enforced")
	}
}
