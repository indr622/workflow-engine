package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/domain"
)

func singleTaskModel(t testing.TB, retries string) *harness {
	proc := newModel("single").
		node("start", "startEvent").service("work", "work", retries).node("end", "endEvent").
		flow("f1", "start", "work", "").flow("f2", "work", "end", "").
		build(t)
	return startInstance(t, proc, `{}`)
}

func (h *harness) incidentKey(t domain.IncidentType) string {
	h.t.Helper()
	for k, inc := range h.st.Incidents {
		if inc.Type == t {
			return k
		}
	}
	h.t.Fatalf("no open %s incident (incidents: %v)", t, h.incidentTypes())
	return ""
}

func TestFailJobWithRetriesKeepsJobActivatable(t *testing.T) {
	h := singleTaskModel(t, "3")
	job := h.openJob("work")
	h.mustRun(domain.FailJob{JobKey: job.Key, Retries: 2, RetryBackoff: 2 * time.Second, ErrorMessage: "upstream timeout",
		Variables: mustVars(t, `{"attempt": 1}`)})

	if job.State != domain.JobCreated || job.Retries != 2 {
		t.Fatalf("job after fail = %+v, want CREATED with retries 2", job)
	}
	if len(h.st.Incidents) != 0 {
		t.Fatalf("no incident expected, got %v", h.incidentTypes())
	}
	failed := h.last[0].Value.(*domain.JobValue)
	if h.last[0].Intent != domain.IntentFailed || failed.RetryBackoffMs != 2000 || failed.ErrorMessage != "upstream timeout" {
		t.Fatalf("first record = %s %+v", h.last[0].Intent, failed)
	}
	if got := h.variablesJSON(); got != `{"attempt":1}` {
		t.Fatalf("variables = %s", got)
	}
	h.complete("work", `{}`)
	h.expectStatus(domain.InstanceCompleted)
	h.assertReplay()
}

func TestFailJobWithoutRetriesRaisesIncidentAndResolveAfterRetriesUpdate(t *testing.T) {
	h := singleTaskModel(t, "1")
	job := h.openJob("work")
	h.mustRun(domain.FailJob{JobKey: job.Key, Retries: 0, ErrorMessage: "permanent"})

	if job.State != domain.JobFailed {
		t.Fatalf("job state = %s, want FAILED", job.State)
	}
	equalStrings(t, "incidents", h.incidentTypes(), []string{"JOB_NO_RETRIES@work"})
	equalStrings(t, "active", h.activeElements(), []string{"work:ACTIVATED"})
	inc := h.st.Incidents[h.incidentKey(domain.IncidentJobNoRetries)]
	if inc.Message != "permanent" || inc.JobKey != job.Key {
		t.Fatalf("incident = %+v", inc)
	}
	expectErr(t, h.run(domain.CompleteJob{JobKey: job.Key}), domain.ErrJobNotActive)
	expectErr(t, h.run(domain.FailJob{JobKey: job.Key, Retries: 1}), domain.ErrJobNotActive)

	// Resolve tanpa menaikkan retries: incident baru, job tetap FAILED.
	first := inc.Key
	h.mustRun(domain.ResolveIncident{IncidentKey: first})
	if job.State != domain.JobFailed || len(h.st.Incidents) != 1 || h.st.Incidents[first] != nil {
		t.Fatalf("resolve without retries: job=%s incidents=%v", job.State, h.incidentTypes())
	}

	h.mustRun(domain.UpdateJobRetries{JobKey: job.Key, Retries: 2})
	if job.Retries != 2 || job.State != domain.JobFailed || len(h.st.Incidents) != 1 {
		t.Fatalf("update retries must not resolve the incident: job=%+v incidents=%v", job, h.incidentTypes())
	}
	h.mustRun(domain.ResolveIncident{IncidentKey: h.incidentKey(domain.IncidentJobNoRetries)})
	if job.State != domain.JobCreated || len(h.st.Incidents) != 0 {
		t.Fatalf("after resolve: job=%s incidents=%v", job.State, h.incidentTypes())
	}
	h.complete("work", `{"done": true}`)
	h.expectStatus(domain.InstanceCompleted)
	h.assertReplay()
}

func TestFailJobDefaultIncidentMessage(t *testing.T) {
	h := singleTaskModel(t, "1")
	h.mustRun(domain.FailJob{JobKey: h.openJob("work").Key, Retries: 0})
	if msg := h.st.Incidents[h.incidentKey(domain.IncidentJobNoRetries)].Message; msg != "No more retries left." {
		t.Fatalf("incident message = %q", msg)
	}
}

func TestThrowErrorWithoutCatchRaisesUnhandledErrorIncident(t *testing.T) {
	h := singleTaskModel(t, "")
	job := h.openJob("work")
	h.mustRun(domain.ThrowError{JobKey: job.Key, ErrorCode: "NOT_FOUND", ErrorMessage: "no stock"})

	if job.State != domain.JobErrorThrown {
		t.Fatalf("job state = %s", job.State)
	}
	inc := h.st.Incidents[h.incidentKey(domain.IncidentUnhandledErrorEvent)]
	if !strings.Contains(inc.Message, "'NOT_FOUND'") || inc.JobKey != job.Key {
		t.Fatalf("incident = %+v", inc)
	}
	expectErr(t, h.run(domain.ThrowError{JobKey: job.Key, ErrorCode: "X"}), domain.ErrJobNotActive)

	// Resolve mengembalikan job ke antrean seperti Zeebe.
	h.mustRun(domain.ResolveIncident{IncidentKey: inc.Key})
	if job.State != domain.JobCreated || len(h.st.Incidents) != 0 {
		t.Fatalf("after resolve: job=%s incidents=%v", job.State, h.incidentTypes())
	}
	h.complete("work", `{}`)
	h.expectStatus(domain.InstanceCompleted)
	h.assertReplay()
}

func TestResolveConditionIncidentAfterSetVariables(t *testing.T) {
	h := exclusiveModel(t, false)
	h = startInstance(t, h.proc, `{"amount": 50}`)
	equalStrings(t, "incidents", h.incidentTypes(), []string{"CONDITION_ERROR@gw"})
	incKey := h.incidentKey(domain.IncidentConditionError)

	// Resolve tanpa perbaikan: incident baru, command sukses.
	h.mustRun(domain.ResolveIncident{IncidentKey: incKey})
	equalStrings(t, "incidents", h.incidentTypes(), []string{"CONDITION_ERROR@gw"})
	if h.st.Incidents[incKey] != nil {
		t.Fatal("resolved incident must be closed")
	}
	expectErr(t, h.run(domain.ResolveIncident{IncidentKey: incKey}), domain.ErrIncidentAlreadyResolved)

	h.mustRun(domain.SetVariables{ScopeKey: h.st.Key, Variables: mustVars(t, `{"amount": 500}`)})
	h.mustRun(domain.ResolveIncident{IncidentKey: h.incidentKey(domain.IncidentConditionError)})
	h.expectStatus(domain.InstanceCompleted)
	equalStrings(t, "completed", h.completedElements(), []string{"big", "gw", "start"})
	h.assertReplay()
}

func TestResolveExtractValueIncidentCreatesJob(t *testing.T) {
	proc := newModel("bad").
		node("start", "startEvent").service("dyn", "=jobType", "").
		flow("f1", "start", "dyn", "").
		build(t)
	h := startInstance(t, proc, `{"jobType": 42}`)
	h.mustRun(domain.SetVariables{ScopeKey: h.st.Key, Variables: mustVars(t, `{"jobType": "fixed"}`)})
	h.mustRun(domain.ResolveIncident{IncidentKey: h.incidentKey(domain.IncidentExtractValueError)})
	if job := h.openJob("dyn"); job.Type != "fixed" {
		t.Fatalf("job = %+v", job)
	}
	equalStrings(t, "active", h.activeElements(), []string{"dyn:ACTIVATED"})
	h.assertReplay()
}

func TestSetVariablesScopes(t *testing.T) {
	h := singleTaskModel(t, "")
	job := h.openJob("work")
	h.mustRun(domain.SetVariables{ScopeKey: job.ElementInstanceKey, Variables: mustVars(t, `{"a": 1}`)})
	h.mustRun(domain.SetVariables{ScopeKey: h.st.Key, Variables: mustVars(t, `{"b": 2}`), Local: true})
	if got := h.variablesJSON(); got != `{"a":1,"b":2}` {
		t.Fatalf("variables = %s", got)
	}
	expectErr(t, h.run(domain.SetVariables{ScopeKey: job.ElementInstanceKey, Variables: mustVars(t, `{"c": 3}`), Local: true}),
		domain.ErrLocalScopeNotSupported)
	expectErr(t, h.run(domain.SetVariables{ScopeKey: "00000000-0000-7000-8000-999999999999", Variables: mustVars(t, `{"c": 3}`)}),
		domain.ErrScopeNotFound)
	h.assertReplay()

	h.complete("work", `{}`)
	expectErr(t, h.run(domain.SetVariables{ScopeKey: h.st.Key, Variables: mustVars(t, `{"c": 3}`)}), domain.ErrInstanceNotActive)
}

func TestUpdateJobRetriesOnFinalJobIsRejected(t *testing.T) {
	h := singleTaskModel(t, "")
	job := h.openJob("work")
	h.complete("work", `{}`)
	expectErr(t, h.run(domain.UpdateJobRetries{JobKey: job.Key, Retries: 5}), domain.ErrJobNotOpen)
}

func TestCancelResolvesJobIncidentsWithoutReopeningJobs(t *testing.T) {
	h := singleTaskModel(t, "")
	job := h.openJob("work")
	h.mustRun(domain.ThrowError{JobKey: job.Key, ErrorCode: "E"})
	h.mustRun(domain.CancelInstance{})
	h.expectStatus(domain.InstanceTerminated)
	if len(h.st.Jobs) != 0 || len(h.st.Incidents) != 0 {
		t.Fatalf("jobs=%v incidents=%v", h.openJobElements(), h.incidentTypes())
	}
	h.assertReplay()
}

const userTaskXML = `<?xml version="1.0" encoding="UTF-8"?>
<bpmn:definitions xmlns:bpmn="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:zeebe="http://camunda.org/schema/zeebe/1.0" id="d" targetNamespace="t">
<bpmn:process id="ut" isExecutable="true">
<bpmn:startEvent id="start"/>
<bpmn:userTask id="approve"><bpmn:extensionElements>
  <zeebe:formDefinition formKey="camunda-forms:bpmn:form1"/>
  <zeebe:assignmentDefinition assignee="=initiator" candidateGroups="sales, managers" candidateUsers="=reviewers"/>
  <zeebe:taskSchedule dueDate="2026-12-01T10:00:00Z"/>
  <zeebe:taskHeaders><zeebe:header key="priority" value="high"/></zeebe:taskHeaders>
</bpmn:extensionElements></bpmn:userTask>
<bpmn:endEvent id="end"/>
<bpmn:sequenceFlow id="f1" sourceRef="start" targetRef="approve"/>
<bpmn:sequenceFlow id="f2" sourceRef="approve" targetRef="end"/>
</bpmn:process>
</bpmn:definitions>`

func TestJobBasedUserTask(t *testing.T) {
	proc := parseExecutable(t, userTaskXML, "ut")
	h := startInstance(t, proc, `{"initiator": "alice", "reviewers": ["bob", "carol"]}`)
	job := h.openJob("approve")
	if job.Type != domain.UserTaskJobType || job.Retries != 1 {
		t.Fatalf("job = %+v", job)
	}
	got := map[string]string{}
	for _, hd := range job.CustomHeaders {
		got[hd.Key] = hd.Value
	}
	want := map[string]string{
		domain.UserTaskHeaderAssignee:       "alice",
		domain.UserTaskHeaderCandidateGroup: `["sales","managers"]`,
		domain.UserTaskHeaderCandidateUsers: `["bob","carol"]`,
		domain.UserTaskHeaderDueDate:        "2026-12-01T10:00:00Z",
		domain.UserTaskHeaderFormKey:        "camunda-forms:bpmn:form1",
		"priority":                          "high",
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("header %s = %q, want %q (all: %v)", k, got[k], v, got)
		}
	}
	h.complete("approve", `{"approved": true}`)
	h.expectStatus(domain.InstanceCompleted)
	h.assertReplay()
}

func TestJobBasedUserTaskBadAssigneeRaisesIncident(t *testing.T) {
	proc := parseExecutable(t, userTaskXML, "ut")
	h := startInstance(t, proc, `{"initiator": 7, "reviewers": []}`)
	equalStrings(t, "incidents", h.incidentTypes(), []string{"EXTRACT_VALUE_ERROR@approve"})
	h.mustRun(domain.SetVariables{ScopeKey: h.st.Key, Variables: mustVars(t, `{"initiator": "dave"}`)})
	h.mustRun(domain.ResolveIncident{IncidentKey: h.incidentKey(domain.IncidentExtractValueError)})
	if job := h.openJob("approve"); job.Type != domain.UserTaskJobType {
		t.Fatalf("job = %+v", job)
	}
	h.assertReplay()
}

func TestSnapshotRoundTripKeepsJobStateAndIncidentJob(t *testing.T) {
	h := singleTaskModel(t, "1")
	job := h.openJob("work")
	h.mustRun(domain.FailJob{JobKey: job.Key, Retries: 0, ErrorMessage: "x"})
	snap := h.st.Snapshot(int32(len(h.all)))
	restored, err := domain.RestoreSnapshot(h.proc, &snap)
	if err != nil {
		t.Fatal(err)
	}
	r := restored.Jobs[job.Key]
	inc := restored.Incidents[h.incidentKey(domain.IncidentJobNoRetries)]
	if r == nil || r.State != domain.JobFailed || inc == nil || inc.JobKey != job.Key {
		t.Fatalf("restored job=%+v incident=%+v", r, inc)
	}
}
