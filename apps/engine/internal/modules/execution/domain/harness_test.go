package domain_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/domain"
	"github.com/indr622/workflow-engine/libs/go/bpmn"
)

// modelBuilder menyusun XML BPMN executable tanpa diagram untuk test.
type modelBuilder struct {
	id    string
	nodes []string
	flows []string
}

func newModel(id string) *modelBuilder { return &modelBuilder{id: id} }

func (b *modelBuilder) node(id, kind string) *modelBuilder {
	b.nodes = append(b.nodes, fmt.Sprintf(`<bpmn:%s id=%q/>`, kind, id))
	return b
}

func (b *modelBuilder) gateway(id, kind, defaultFlow string) *modelBuilder {
	attr := ""
	if defaultFlow != "" {
		attr = fmt.Sprintf(` default=%q`, defaultFlow)
	}
	b.nodes = append(b.nodes, fmt.Sprintf(`<bpmn:%s id=%q%s/>`, kind, id, attr))
	return b
}

// service menambah service task dengan type dan retries (retries kosong = default).
func (b *modelBuilder) service(id, jobType, retries string, headers ...string) *modelBuilder {
	attrs := fmt.Sprintf(`type=%q`, xmlEscape(jobType))
	if retries != "" {
		attrs += fmt.Sprintf(` retries=%q`, xmlEscape(retries))
	}
	var hdr string
	if len(headers) > 0 {
		var sb strings.Builder
		sb.WriteString("<zeebe:taskHeaders>")
		for i := 0; i+1 < len(headers); i += 2 {
			fmt.Fprintf(&sb, `<zeebe:header key=%q value=%q/>`, headers[i], headers[i+1])
		}
		sb.WriteString("</zeebe:taskHeaders>")
		hdr = sb.String()
	}
	b.nodes = append(b.nodes, fmt.Sprintf(
		`<bpmn:serviceTask id=%q><bpmn:extensionElements><zeebe:taskDefinition %s/>%s</bpmn:extensionElements></bpmn:serviceTask>`,
		id, attrs, hdr))
	return b
}

// flow menambah sequence flow; cond kosong berarti tanpa kondisi.
func (b *modelBuilder) flow(id, from, to, cond string) *modelBuilder {
	if cond == "" {
		b.flows = append(b.flows, fmt.Sprintf(`<bpmn:sequenceFlow id=%q sourceRef=%q targetRef=%q/>`, id, from, to))
		return b
	}
	b.flows = append(b.flows, fmt.Sprintf(
		`<bpmn:sequenceFlow id=%q sourceRef=%q targetRef=%q><bpmn:conditionExpression xsi:type="bpmn:tFormalExpression">%s</bpmn:conditionExpression></bpmn:sequenceFlow>`,
		id, from, to, xmlEscape(cond)))
	return b
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

func (b *modelBuilder) xml() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<bpmn:definitions xmlns:bpmn="http://www.omg.org/spec/BPMN/20100524/MODEL"
  xmlns:zeebe="http://camunda.org/schema/zeebe/1.0"
  xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
  id="defs" targetNamespace="http://bpmn.io/schema/bpmn">
<bpmn:process id="` + b.id + `" isExecutable="true">
` + strings.Join(b.nodes, "\n") + "\n" + strings.Join(b.flows, "\n") + `
</bpmn:process>
</bpmn:definitions>`
}

func (b *modelBuilder) build(t testing.TB) *bpmn.Process {
	t.Helper()
	return parseExecutable(t, b.xml(), b.id)
}

func parseExecutable(t testing.TB, xml, id string) *bpmn.Process {
	t.Helper()
	defs, issues, err := bpmn.ParseAndValidate(strings.NewReader(xml), bpmn.ProfileExecutable)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if issues.HasErrors() {
		t.Fatalf("model is not executable: %v\n%s", issues, xml)
	}
	proc, ok := defs.Process(id)
	if !ok {
		t.Fatalf("process %q not found", id)
	}
	return proc
}

// counterKeys memberi key naik tanpa batas.
type counterKeys struct{ next int64 }

func (k *counterKeys) Next() (int64, bool) {
	k.next++
	return k.next, true
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

var testTime = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

func newProcessor(limit int) *domain.Processor {
	return domain.NewProcessor(domain.NewFeelEvaluator(), fixedClock{t: testTime}, domain.Limits{MaxRecordsPerCommand: limit})
}

// harness menjalankan command berurutan pada satu instance dan menyimpan semua record.
type harness struct {
	t    testing.TB
	proc *bpmn.Process
	p    *domain.Processor
	keys *counterKeys
	st   *domain.InstanceState
	all  []domain.Record
	last []domain.Record
}

var testDefinition = domain.DefinitionRef{Key: 900, Version: 1}

func startInstance(t testing.TB, proc *bpmn.Process, vars string) *harness {
	t.Helper()
	return startWith(t, proc, vars, newProcessor(0))
}

func startWith(t testing.TB, proc *bpmn.Process, vars string, p *domain.Processor) *harness {
	t.Helper()
	def := testDefinition
	def.BpmnProcessID = proc.ID()
	h := &harness{t: t, proc: proc, p: p, keys: &counterKeys{next: 1000}, st: domain.NewInstanceState(def, proc)}
	h.mustRun(domain.CreateInstance{Variables: mustVars(t, vars)})
	return h
}

func mustVars(t testing.TB, raw string) []domain.NamedValue {
	t.Helper()
	vars, err := domain.ParseVariables([]byte(raw), 0)
	if err != nil {
		t.Fatalf("variables %q: %v", raw, err)
	}
	return vars
}

func (h *harness) run(cmd domain.Command) error {
	recs, err := h.p.Process(h.st, cmd, h.keys)
	if err != nil {
		return err
	}
	h.last = recs
	h.all = append(h.all, recs...)
	return nil
}

func (h *harness) mustRun(cmd domain.Command) {
	h.t.Helper()
	if err := h.run(cmd); err != nil {
		h.t.Fatalf("process %T: %v", cmd, err)
	}
}

// openJob mengembalikan job terbuka untuk element id; gagal bila tidak tepat satu.
func (h *harness) openJob(elementID string) *domain.Job {
	h.t.Helper()
	var found []*domain.Job
	for _, j := range h.st.Jobs {
		if j.ElementID == elementID {
			found = append(found, j)
		}
	}
	if len(found) != 1 {
		h.t.Fatalf("want exactly one open job for %q, got %d (open jobs: %v)", elementID, len(found), h.openJobElements())
	}
	return found[0]
}

func (h *harness) openJobElements() []string {
	var out []string
	for _, j := range h.st.Jobs {
		out = append(out, j.ElementID)
	}
	sort.Strings(out)
	return out
}

func (h *harness) complete(elementID, vars string) {
	h.t.Helper()
	h.mustRun(domain.CompleteJob{JobKey: h.openJob(elementID).Key, Variables: mustVars(h.t, vars)})
}

// completedElements mengembalikan multiset (terurut) element id yang mencapai COMPLETED, tanpa PROCESS.
func (h *harness) completedElements() []string {
	var out []string
	for _, r := range h.all {
		v, ok := r.Value.(*domain.ProcessInstanceValue)
		if ok && r.Intent == domain.IntentElementCompleted && v.BpmnElementType != domain.ElementTypeProcess {
			out = append(out, v.ElementID)
		}
	}
	sort.Strings(out)
	return out
}

func (h *harness) countIntent(elementID string, intent domain.Intent) int {
	n := 0
	for i := range h.all {
		if h.all[i].Intent == intent && h.all[i].ElementID() == elementID && h.all[i].ValueType == domain.ValueProcessInstance {
			n++
		}
	}
	return n
}

func (h *harness) createdJobTypes() []string {
	var out []string
	for _, r := range h.all {
		if r.ValueType == domain.ValueJob && r.Intent == domain.IntentCreated {
			out = append(out, r.Value.(*domain.JobValue).Type)
		}
	}
	sort.Strings(out)
	return out
}

func (h *harness) variablesJSON() string {
	m := map[string]json.RawMessage{}
	for _, v := range h.st.Variables {
		m[v.Name] = v.Value
	}
	b, err := json.Marshal(m)
	if err != nil {
		h.t.Fatal(err)
	}
	return string(b)
}

func (h *harness) expectStatus(want domain.InstanceStatus) {
	h.t.Helper()
	if h.st.Status != want {
		h.t.Fatalf("status = %s, want %s; active elements: %v; open jobs: %v; incidents: %v",
			h.st.Status, want, h.activeElements(), h.openJobElements(), h.incidentTypes())
	}
}

func (h *harness) activeElements() []string {
	var out []string
	for _, e := range h.st.Elements {
		if e.BpmnElementType != domain.ElementTypeProcess {
			out = append(out, e.ElementID+":"+string(e.State))
		}
	}
	sort.Strings(out)
	return out
}

func (h *harness) incidentTypes() []string {
	var out []string
	for _, inc := range h.st.Incidents {
		out = append(out, string(inc.Type)+"@"+inc.ElementID)
	}
	sort.Strings(out)
	return out
}

// assertReplay menerapkan ulang semua record ke state kosong dan membandingkannya dengan state
// hasil pemrosesan: fondasi replay Fase 3.
func (h *harness) assertReplay() {
	h.t.Helper()
	replayed := domain.NewInstanceState(domain.DefinitionRef{}, h.proc)
	for i := range h.all {
		if err := domain.Apply(replayed, &h.all[i]); err != nil {
			h.t.Fatalf("replay record %d: %v", i, err)
		}
	}
	if !reflect.DeepEqual(normalize(replayed), normalize(h.st)) {
		h.t.Fatalf("replayed state differs\nreplayed: %+v\nlive:     %+v", normalize(replayed), normalize(h.st))
	}
}

type stateView struct {
	Key        int64
	Definition domain.DefinitionRef
	Status     domain.InstanceStatus
	Elements   []domain.ElementInstance
	TakenFlows []domain.TakenFlow
	Variables  []domain.Variable
	Jobs       []domain.Job
	Incidents  []domain.Incident
}

func normalize(s *domain.InstanceState) stateView {
	v := stateView{Key: s.Key, Definition: s.Definition, Status: s.Status, TakenFlows: s.SortedTakenFlows(), Variables: s.SortedVariables()}
	for _, e := range s.Elements {
		v.Elements = append(v.Elements, *e)
	}
	sort.Slice(v.Elements, func(i, j int) bool { return v.Elements[i].Key < v.Elements[j].Key })
	for _, j := range s.Jobs {
		v.Jobs = append(v.Jobs, *j)
	}
	sort.Slice(v.Jobs, func(i, j int) bool { return v.Jobs[i].Key < v.Jobs[j].Key })
	for _, inc := range s.Incidents {
		v.Incidents = append(v.Incidents, *inc)
	}
	sort.Slice(v.Incidents, func(i, j int) bool { return v.Incidents[i].Key < v.Incidents[j].Key })
	return v
}

func expectErr(t testing.TB, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func equalStrings(t testing.TB, what string, got, want []string) {
	t.Helper()
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
}
