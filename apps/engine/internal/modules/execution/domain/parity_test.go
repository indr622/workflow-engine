package domain_test

import (
	"bytes"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/domain"
)

// Uji paritas eksekusi (langkah 8.13): setiap skenario dijalankan di engine kita dan dibandingkan
// dengan hasil observable Camunda 8.8 yang direkam di testdata/parity/expectations.json:
// selesai atau tidak, multiset tipe job yang dibuat, dan variabel akhir (bila selesai).
// Rekaman diperbarui dengan: make parity SUITE=execution LIVE=1 (butuh profil compose camunda).

const parityExpectationsPath = "testdata/parity/expectations.json"

// parityRunVar disisipkan ke variabel awal di kedua engine; worker Camunda memakainya untuk
// memilih job milik run ini saja.
const parityRunVar = "parityRun"

type parityStep struct {
	JobType   string
	Variables string
}

type parityScenario struct {
	Name      string
	Model     *modelBuilder
	Variables string
	Steps     []parityStep
	// JobTypes adalah semua tipe job di model, untuk menyapu job yang masih terbuka.
	JobTypes []string
}

type parityOutcome struct {
	Completed bool `json:"completed"`
	// Jobs adalah multiset tipe job yang dibuat selama skenario, terurut.
	Jobs []string `json:"jobs"`
	// OpenJobs adalah tipe job yang masih terbuka setelah semua langkah, terurut.
	OpenJobs []string `json:"openJobs"`
	// Variables hanya diisi bila instance selesai (Camunda hanya mengembalikannya lewat awaitCompletion).
	Variables map[string]json.RawMessage `json:"variables,omitempty"`
}

type parityEntry struct {
	Scenario  string        `json:"scenario"`
	Camunda   parityOutcome `json:"camunda"`
	Ours      parityOutcome `json:"ours"`
	Deviation string        `json:"deviation,omitempty"`
}

type parityFile struct {
	Reference string        `json:"reference"`
	UpdatedAt string        `json:"updatedAt"`
	Entries   []parityEntry `json:"entries"`
}

func svcTask(m *modelBuilder, id string) *modelBuilder { return m.service(id, id, "") }

func parityScenarios() []parityScenario {
	excl := func(id string, withDefault bool, cond string) *modelBuilder {
		m := newModel(id).node("start", "startEvent").node("end", "endEvent")
		svcTask(m, "big")
		svcTask(m, "small")
		def := ""
		if withDefault {
			def = "fSmall"
		}
		m.gateway("gw", "exclusiveGateway", def).
			flow("f1", "start", "gw", "").flow("fBig", "gw", "big", cond).
			flow("e1", "big", "end", "").flow("e2", "small", "end", "")
		if withDefault {
			m.flow("fSmall", "gw", "small", "")
		} else {
			m.flow("fSmall", "gw", "small", "=amount <= 10")
		}
		return m
	}
	incl := func(id string) *modelBuilder {
		m := newModel(id).node("start", "startEvent").node("end", "endEvent").
			gateway("fork", "inclusiveGateway", "fd").gateway("join", "inclusiveGateway", "")
		for _, b := range []string{"a", "b", "c", "d"} {
			svcTask(m, b)
			m.flow("j"+b, b, "join", "")
		}
		return m.flow("f1", "start", "fork", "").
			flow("fa", "fork", "a", "=x > 0").flow("fb", "fork", "b", "=x > 10").flow("fc", "fork", "c", "=x > 100").
			flow("fd", "fork", "d", "").flow("f2", "join", "end", "")
	}
	parallel := func() *modelBuilder {
		m := newModel("parity-parallel").node("start", "startEvent").node("end", "endEvent").
			gateway("fork", "parallelGateway", "").gateway("join", "parallelGateway", "")
		for _, b := range []string{"a", "b", "c"} {
			svcTask(m, b)
			m.flow("s"+b, "fork", b, "").flow("j"+b, b, "join", "")
		}
		svcTask(m, "after")
		return m.flow("f1", "start", "fork", "").flow("f2", "join", "after", "").flow("f3", "after", "end", "")
	}
	loop := func() *modelBuilder {
		m := newModel("parity-loop").node("start", "startEvent").node("end", "endEvent").
			gateway("merge", "exclusiveGateway", "").gateway("check", "exclusiveGateway", "")
		svcTask(m, "work")
		return m.flow("f1", "start", "merge", "").flow("f2", "merge", "work", "").flow("f3", "work", "check", "").
			flow("again", "check", "merge", "=i < 3").flow("done", "check", "end", "=i >= 3")
	}
	return []parityScenario{
		{
			Name:      "linear-service-task",
			Model:     svcTask(newModel("parity-linear").node("start", "startEvent").node("end", "endEvent"), "a").flow("f1", "start", "a", "").flow("f2", "a", "end", ""),
			Variables: `{"x": 1}`, Steps: []parityStep{{"a", `{"y": 2}`}}, JobTypes: []string{"a"},
		},
		{Name: "exclusive-condition", Model: excl("parity-excl-cond", true, "=amount > 100"), Variables: `{"amount": 500}`, Steps: []parityStep{{"big", `{}`}}, JobTypes: []string{"big", "small"}},
		{Name: "exclusive-default", Model: excl("parity-excl-default", true, "=amount > 100"), Variables: `{"amount": 50}`, Steps: []parityStep{{"small", `{}`}}, JobTypes: []string{"big", "small"}},
		{Name: "exclusive-second-condition", Model: excl("parity-excl-second", false, "=amount > 100"), Variables: `{"amount": 5}`, Steps: []parityStep{{"small", `{}`}}, JobTypes: []string{"big", "small"}},
		{Name: "exclusive-no-match", Model: excl("parity-excl-nomatch", false, "=amount > 100"), Variables: `{"amount": 50}`, JobTypes: []string{"big", "small"}},
		{Name: "exclusive-null-condition", Model: excl("parity-excl-null", false, "=amount > 100"), Variables: `{}`, JobTypes: []string{"big", "small"}},
		{Name: "exclusive-null-condition-with-default", Model: excl("parity-excl-null-default", true, "=amount > 100"), Variables: `{}`, Steps: []parityStep{{"small", `{}`}}, JobTypes: []string{"big", "small"}},
		{Name: "exclusive-string-equality", Model: excl("parity-excl-string", true, `=status = "vip"`), Variables: `{"status": "vip"}`, Steps: []parityStep{{"big", `{}`}}, JobTypes: []string{"big", "small"}},
		{Name: "exclusive-big-number", Model: excl("parity-excl-bignum", true, "=amount > 9007199254740992"), Variables: `{"amount": 9007199254740993}`, Steps: []parityStep{{"big", `{}`}}, JobTypes: []string{"big", "small"}},
		{Name: "exclusive-nested-path", Model: excl("parity-excl-path", true, "=order.total > 100 and order.paid"), Variables: `{"order": {"total": 150, "paid": true}}`, Steps: []parityStep{{"big", `{}`}}, JobTypes: []string{"big", "small"}},
		{
			Name: "parallel-fork-join", Model: parallel(), Variables: `{}`,
			Steps:    []parityStep{{"c", `{"c": 1}`}, {"a", `{"a": 1}`}, {"b", `{"b": 1}`}, {"after", `{}`}},
			JobTypes: []string{"a", "b", "c", "after"},
		},
		{Name: "inclusive-some-branches", Model: incl("parity-incl-some"), Variables: `{"x": 50}`, Steps: []parityStep{{"b", `{}`}, {"a", `{}`}}, JobTypes: []string{"a", "b", "c", "d"}},
		{Name: "inclusive-all-branches", Model: incl("parity-incl-all"), Variables: `{"x": 500}`, Steps: []parityStep{{"c", `{}`}, {"b", `{}`}, {"a", `{}`}}, JobTypes: []string{"a", "b", "c", "d"}},
		{Name: "inclusive-default", Model: incl("parity-incl-default"), Variables: `{"x": -1}`, Steps: []parityStep{{"d", `{}`}}, JobTypes: []string{"a", "b", "c", "d"}},
		{Name: "inclusive-join-waits", Model: incl("parity-incl-wait"), Variables: `{"x": 50}`, Steps: []parityStep{{"a", `{}`}}, JobTypes: []string{"a", "b", "c", "d"}},
		{
			Name: "loop-through-exclusive", Model: loop(), Variables: `{"i": 0}`,
			Steps:    []parityStep{{"work", `{"i": 1}`}, {"work", `{"i": 2}`}, {"work", `{"i": 3}`}},
			JobTypes: []string{"work"},
		},
		{
			Name:      "job-type-expression",
			Model:     newModel("parity-jobtype").node("start", "startEvent").node("end", "endEvent").service("dyn", "=jobType", "").flow("f1", "start", "dyn", "").flow("f2", "dyn", "end", ""),
			Variables: `{"jobType": "dynamic-type"}`, Steps: []parityStep{{"dynamic-type", `{}`}}, JobTypes: []string{"dynamic-type"},
		},
		{
			Name:      "implicit-end",
			Model:     svcTask(newModel("parity-implicit").node("start", "startEvent"), "a").flow("f1", "start", "a", ""),
			Variables: `{}`, Steps: []parityStep{{"a", `{}`}}, JobTypes: []string{"a"},
		},
		{
			Name:      "variables-top-level-replace",
			Model:     svcTask(newModel("parity-vars").node("start", "startEvent").node("end", "endEvent"), "a").flow("f1", "start", "a", "").flow("f2", "a", "end", ""),
			Variables: `{"keep": "x", "obj": {"c": 1}, "n": 1.50}`, Steps: []parityStep{{"a", `{"obj": {"d": 2}, "added": [1, 2]}`}}, JobTypes: []string{"a"},
		},
		{
			Name: "parallel-without-wait-states",
			Model: newModel("parity-parfast").node("start", "startEvent").node("end", "endEvent").node("t1", "task").node("t2", "manualTask").
				gateway("fork", "parallelGateway", "").gateway("join", "parallelGateway", "").
				flow("f1", "start", "fork", "").flow("a", "fork", "t1", "").flow("b", "fork", "t2", "").
				flow("c", "t1", "join", "").flow("d", "t2", "join", "").flow("f2", "join", "end", ""),
			Variables: `{}`,
		},
	}
}

// runOurs menjalankan skenario di Processor kita dengan langkah yang sama seperti worker Camunda.
func runOurs(t testing.TB, sc parityScenario, run string) parityOutcome {
	t.Helper()
	proc := sc.Model.build(t)
	h := startInstance(t, proc, withRunVar(t, sc.Variables, run))
	for _, step := range sc.Steps {
		job := firstOpenJob(h, step.JobType)
		if job == nil {
			// Sama dengan worker Camunda: langkah berhenti bila job yang ditunggu tidak muncul.
			break
		}
		h.mustRun(domain.CompleteJob{JobKey: job.Key, Variables: mustVars(t, step.Variables)})
	}
	out := parityOutcome{Completed: h.st.Status == domain.InstanceCompleted, Jobs: h.createdJobTypes(), OpenJobs: []string{}}
	for _, j := range h.st.Jobs {
		out.OpenJobs = append(out.OpenJobs, j.Type)
	}
	sort.Strings(out.OpenJobs)
	if out.Jobs == nil {
		out.Jobs = []string{}
	}
	if out.Completed {
		out.Variables = map[string]json.RawMessage{}
		for _, v := range h.st.Variables {
			if v.Name != parityRunVar {
				out.Variables[v.Name] = v.Value
			}
		}
	}
	return out
}

func firstOpenJob(h *harness, jobType string) *domain.Job {
	var found *domain.Job
	for _, j := range h.st.Jobs {
		if j.Type == jobType && (found == nil || j.Key < found.Key) {
			found = j
		}
	}
	return found
}

func withRunVar(t testing.TB, vars, run string) string {
	t.Helper()
	m := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(vars), &m); err != nil {
		t.Fatal(err)
	}
	m[parityRunVar] = json.RawMessage(`"` + run + `"`)
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// sameOutcome membandingkan hasil observable. Angka JSON dibandingkan sebagai desimal eksak, jadi
// 1.50 dan 1.5 sama (Camunda mengembalikan bentuk kanonik) tetapi presisi angka besar tetap diuji.
func sameOutcome(a, b parityOutcome) bool {
	if a.Completed != b.Completed || !reflect.DeepEqual(a.Jobs, b.Jobs) || !reflect.DeepEqual(a.OpenJobs, b.OpenJobs) {
		return false
	}
	return reflect.DeepEqual(canonicalVars(a.Variables), canonicalVars(b.Variables))
}

func canonicalVars(m map[string]json.RawMessage) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		dec := json.NewDecoder(bytes.NewReader(v))
		dec.UseNumber()
		var x any
		if err := dec.Decode(&x); err != nil {
			out[k] = string(v)
			continue
		}
		out[k] = exactNumbers(x)
	}
	return out
}

// exactNumbers mengganti json.Number dengan representasi pecahan eksak big.Rat.
func exactNumbers(x any) any {
	switch v := x.(type) {
	case json.Number:
		r, ok := new(big.Rat).SetString(v.String())
		if !ok {
			return v.String()
		}
		return r.RatString()
	case map[string]any:
		for k, e := range v {
			v[k] = exactNumbers(e)
		}
		return v
	case []any:
		for i, e := range v {
			v[i] = exactNumbers(e)
		}
		return v
	default:
		return v
	}
}

// TestExecutionParity membandingkan engine kita dengan rekaman Camunda 8.8.
func TestExecutionParity(t *testing.T) {
	data, err := os.ReadFile(filepath.FromSlash(parityExpectationsPath))
	if err != nil {
		t.Fatalf("read expectations (record them with make parity SUITE=execution LIVE=1): %v", err)
	}
	var file parityFile
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	recorded := map[string]parityEntry{}
	for _, e := range file.Entries {
		recorded[e.Scenario] = e
	}
	for _, sc := range parityScenarios() {
		t.Run(sc.Name, func(t *testing.T) {
			entry, ok := recorded[sc.Name]
			if !ok {
				t.Fatalf("scenario has no recorded Camunda result; run make parity SUITE=execution LIVE=1")
			}
			ours := runOurs(t, sc, "offline")
			if entry.Deviation != "" {
				if !sameOutcome(ours, entry.Ours) {
					t.Fatalf("documented deviation %q no longer matches our behaviour:\nours now: %+v\nrecorded: %+v", entry.Deviation, ours, entry.Ours)
				}
				return
			}
			if !sameOutcome(ours, entry.Camunda) {
				t.Fatalf("differs from Camunda 8.8:\nours:    %+v\ncamunda: %+v", ours, entry.Camunda)
			}
		})
	}
}
