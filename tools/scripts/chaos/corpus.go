package main

import (
	"fmt"
	"strings"
)

// Ukuran korpus. Rencana 11.2 menyebut 20 task / 50 cabang / 1.000 iterasi; suite ini memakai
// ukuran lebih kecil agar satu skenario 15–60 detik tetap menyelesaikan banyak instance, dengan
// fork 10 cabang sesuai tugas 12.10. Loop panjang + snapshot diuji skenario 15 (fault point).
const (
	linearTasks    = 10
	forkBranches   = 10
	loopIterations = 20
)

// process adalah satu proses korpus beserta cara worker menyelesaikan job-nya.
type process struct {
	id   string
	xml  string
	loop bool // job loop diselesaikan dengan variabel {"i": n}
}

var corpus = []process{
	{id: "chaos-linear", xml: linearProcess(linearTasks)},
	{id: "chaos-fork", xml: forkProcess(forkBranches)},
	{id: "chaos-loop", xml: loopProcess(loopIterations), loop: true},
}

func processByID(id string) process {
	for _, p := range corpus {
		if p.id == id {
			return p
		}
	}
	panic("unknown corpus process " + id)
}

const bpmnHeader = `<?xml version="1.0" encoding="UTF-8"?>
<bpmn:definitions xmlns:bpmn="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:zeebe="http://camunda.org/schema/zeebe/1.0"
  xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" id="defs" targetNamespace="http://bpmn.io/schema/bpmn">
<bpmn:process id="%s" isExecutable="true">`

const bpmnFooter = `</bpmn:process></bpmn:definitions>`

func serviceTask(id, jobType string) string {
	return fmt.Sprintf(`<bpmn:serviceTask id=%q><bpmn:extensionElements><zeebe:taskDefinition type=%q/></bpmn:extensionElements></bpmn:serviceTask>`, id, jobType)
}

func sequenceFlow(id, from, to string) string {
	return fmt.Sprintf(`<bpmn:sequenceFlow id=%q sourceRef=%q targetRef=%q/>`, id, from, to)
}

func conditionalFlow(id, from, to, condition string) string {
	return fmt.Sprintf(`<bpmn:sequenceFlow id=%q sourceRef=%q targetRef=%q><bpmn:conditionExpression xsi:type="bpmn:tFormalExpression">%s</bpmn:conditionExpression></bpmn:sequenceFlow>`, id, from, to, condition)
}

// linearProcess: start → n service task berurutan → end.
func linearProcess(n int) string {
	var b strings.Builder
	fmt.Fprintf(&b, bpmnHeader, "chaos-linear")
	b.WriteString(`<bpmn:startEvent id="start"/><bpmn:endEvent id="end"/>`)
	prev := "start"
	for i := 0; i < n; i++ {
		task := fmt.Sprintf("step%d", i)
		b.WriteString(serviceTask(task, "chaos-linear"))
		b.WriteString(sequenceFlow(fmt.Sprintf("f%d", i), prev, task))
		prev = task
	}
	b.WriteString(sequenceFlow("fend", prev, "end"))
	b.WriteString(bpmnFooter)
	return b.String()
}

// forkProcess: start → parallel fork n cabang (service task) → join → end.
func forkProcess(n int) string {
	var b strings.Builder
	fmt.Fprintf(&b, bpmnHeader, "chaos-fork")
	b.WriteString(`<bpmn:startEvent id="start"/><bpmn:parallelGateway id="fork"/><bpmn:parallelGateway id="join"/><bpmn:endEvent id="end"/>`)
	b.WriteString(sequenceFlow("f0", "start", "fork") + sequenceFlow("fz", "join", "end"))
	for i := 0; i < n; i++ {
		task := fmt.Sprintf("branch%d", i)
		b.WriteString(serviceTask(task, "chaos-branch"))
		b.WriteString(sequenceFlow("a"+task, "fork", task) + sequenceFlow("b"+task, task, "join"))
	}
	b.WriteString(bpmnFooter)
	return b.String()
}

// loopProcess: start → merge → work → check; kembali ke merge selama i < n.
func loopProcess(n int) string {
	var b strings.Builder
	fmt.Fprintf(&b, bpmnHeader, "chaos-loop")
	b.WriteString(`<bpmn:startEvent id="start"/><bpmn:exclusiveGateway id="merge"/><bpmn:exclusiveGateway id="check"/><bpmn:endEvent id="end"/>`)
	b.WriteString(serviceTask("work", "chaos-loop"))
	b.WriteString(sequenceFlow("f1", "start", "merge") + sequenceFlow("f2", "merge", "work") + sequenceFlow("f3", "work", "check"))
	b.WriteString(conditionalFlow("again", "check", "merge", fmt.Sprintf("=i &lt; %d", n)))
	b.WriteString(conditionalFlow("done", "check", "end", fmt.Sprintf("=i &gt;= %d", n)))
	b.WriteString(bpmnFooter)
	return b.String()
}
