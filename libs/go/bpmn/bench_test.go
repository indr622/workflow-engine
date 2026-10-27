package bpmn

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// generateChain membuat proses dengan n service task berurutan untuk benchmark.
func generateChain(n int) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><bpmn:definitions xmlns:bpmn="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:zeebe="http://camunda.org/schema/zeebe/1.0" id="bench"><bpmn:process id="bench" isExecutable="true"><bpmn:startEvent id="start"><bpmn:outgoing>f0</bpmn:outgoing></bpmn:startEvent>`)
	for i := range n {
		fmt.Fprintf(&b, `<bpmn:serviceTask id="t%d" name="Task %d"><bpmn:extensionElements><zeebe:taskDefinition type="work" retries="3"/><zeebe:ioMapping><zeebe:input source="=a" target="b"/></zeebe:ioMapping></bpmn:extensionElements><bpmn:incoming>f%d</bpmn:incoming><bpmn:outgoing>f%d</bpmn:outgoing></bpmn:serviceTask>`, i, i, i, i+1)
	}
	fmt.Fprintf(&b, `<bpmn:endEvent id="end"><bpmn:incoming>f%d</bpmn:incoming></bpmn:endEvent>`, n)
	prev := "start"
	for i := range n {
		fmt.Fprintf(&b, `<bpmn:sequenceFlow id="f%d" sourceRef="%s" targetRef="t%d"/>`, i, prev, i)
		prev = fmt.Sprintf("t%d", i)
	}
	fmt.Fprintf(&b, `<bpmn:sequenceFlow id="f%d" sourceRef="%s" targetRef="end"/></bpmn:process></bpmn:definitions>`, n, prev)
	return []byte(b.String())
}

func BenchmarkParse500(b *testing.B) {
	data := generateChain(500)
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	for b.Loop() {
		if _, err := Parse(bytes.NewReader(data)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseAndValidate500(b *testing.B) {
	data := generateChain(500)
	b.ReportAllocs()
	for b.Loop() {
		d, err := Parse(bytes.NewReader(data))
		if err != nil {
			b.Fatal(err)
		}
		if issues := Validate(d, ProfileExecutable); issues.HasErrors() {
			b.Fatal(issues.Errors())
		}
	}
}
