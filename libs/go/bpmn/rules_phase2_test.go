package bpmn

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestExecutableProfileRejectsIOMapping(t *testing.T) {
	data := readFile(t, filepath.Join("testdata", "valid", "v02_service_task.bpmn"))
	_, full, err := ParseAndValidate(strings.NewReader(string(data)), ProfileFull)
	if err != nil {
		t.Fatal(err)
	}
	if hasCode(full, CodeIOMappingNotSupported) {
		t.Fatal("full profile must accept I/O mappings")
	}
	_, exec, err := ParseAndValidate(strings.NewReader(string(data)), ProfileExecutable)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(exec.Errors(), CodeIOMappingNotSupported) {
		t.Fatalf("executable profile must reject I/O mappings, got %v", exec)
	}
}

const cycleHeader = `<?xml version="1.0" encoding="UTF-8"?>
<bpmn:definitions xmlns:bpmn="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" id="d" targetNamespace="t">
<bpmn:process id="p" isExecutable="true">`

func TestInclusiveJoinInCycleWarning(t *testing.T) {
	cond := func(id, from, to string) string {
		return `<bpmn:sequenceFlow id="` + id + `" sourceRef="` + from + `" targetRef="` + to + `"><bpmn:conditionExpression xsi:type="bpmn:tFormalExpression">=true</bpmn:conditionExpression></bpmn:sequenceFlow>`
	}
	plain := func(id, from, to string) string {
		return `<bpmn:sequenceFlow id="` + id + `" sourceRef="` + from + `" targetRef="` + to + `"/>`
	}
	// j1 dan j2 saling menjadi upstream lewat j1 → j2 → j1.
	cyclic := cycleHeader + `<bpmn:startEvent id="s"/><bpmn:inclusiveGateway id="j1"/><bpmn:inclusiveGateway id="j2"/><bpmn:endEvent id="e"/>` +
		plain("f1", "s", "j1") + plain("f2", "s", "j2") + cond("f3", "j1", "j2") + cond("f4", "j2", "j1") + cond("f5", "j2", "e") +
		`</bpmn:process></bpmn:definitions>`
	_, issues, err := ParseAndValidate(strings.NewReader(cyclic), ProfileFull)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(issues.Warnings(), CodeInclusiveJoinInCycle) {
		t.Fatalf("want INCLUSIVE_JOIN_IN_CYCLE warning, got %v", issues)
	}

	// Satu inclusive join di dalam loop tanpa join lain: bukan siklus antar join.
	single := cycleHeader + `<bpmn:startEvent id="s"/><bpmn:exclusiveGateway id="m"/><bpmn:inclusiveGateway id="f"/><bpmn:task id="a"/><bpmn:task id="b"/>` +
		`<bpmn:inclusiveGateway id="j"/><bpmn:exclusiveGateway id="c" default="back"/><bpmn:endEvent id="e"/>` +
		plain("f1", "s", "m") + plain("f2", "m", "f") + cond("fa", "f", "a") + cond("fb", "f", "b") +
		plain("ja", "a", "j") + plain("jb", "b", "j") + plain("f3", "j", "c") + cond("out", "c", "e") + plain("back", "c", "m") +
		`</bpmn:process></bpmn:definitions>`
	_, issues, err = ParseAndValidate(strings.NewReader(single), ProfileFull)
	if err != nil {
		t.Fatal(err)
	}
	if hasCode(issues, CodeInclusiveJoinInCycle) {
		t.Fatalf("unexpected INCLUSIVE_JOIN_IN_CYCLE: %v", issues)
	}
}

func hasCode(issues Issues, code Code) bool {
	for _, i := range issues {
		if i.Code == code {
			return true
		}
	}
	return false
}

func TestExecutableProfileExplainsNativeUserTask(t *testing.T) {
	data := readFile(t, filepath.Join("testdata", "valid", "v07_user_task.bpmn"))
	_, issues, err := ParseAndValidate(strings.NewReader(string(data)), ProfileExecutable)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range issues.Errors() {
		if i.Code == CodeUnsupportedElement && i.ElementID == "task" {
			if !strings.Contains(i.Message, "phase 6") {
				t.Fatalf("native user task rejection should point to phase 6, got %q", i.Message)
			}
			return
		}
	}
	t.Fatalf("native user task must be rejected, got %v", issues)
}

func TestJobBasedUserTaskHeaders(t *testing.T) {
	data := readFile(t, filepath.Join("testdata", "valid", "v28_user_task_job_based.bpmn"))
	d, issues, err := ParseAndValidate(strings.NewReader(string(data)), ProfileExecutable)
	if err != nil || issues.HasErrors() {
		t.Fatalf("job-based user task must be executable: %v %v", err, issues)
	}
	el, _ := d.Processes()[0].Element("approve")
	a := el.(*Activity)
	if a.UserTask().ZeebeUserTask || a.UserTask().FormKey == "" || len(a.Headers()) != 1 {
		t.Fatalf("unexpected user task model: %+v headers=%v", a.UserTask(), a.Headers())
	}
}
