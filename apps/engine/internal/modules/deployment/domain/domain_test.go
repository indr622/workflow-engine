package domain

import "testing"

func TestNextVersion(t *testing.T) {
	if got := NextVersion(nil); got != 1 {
		t.Fatalf("first version = %d, want 1", got)
	}
	if got := NextVersion(&ProcessDefinition{Version: 4}); got != 5 {
		t.Fatalf("next version = %d, want 5", got)
	}
}

func TestChecksumRoundTrip(t *testing.T) {
	c := ChecksumOf([]byte("<bpmn/>"))
	back, err := ChecksumFromBytes(c.Bytes())
	if err != nil || back != c {
		t.Fatalf("round trip failed: %v", err)
	}
	if _, err := ChecksumFromBytes([]byte{1, 2}); err == nil {
		t.Fatal("short checksum must be rejected")
	}
	if !(ProcessDefinition{Checksum: c}).IsDuplicateOf(c) {
		t.Fatal("same checksum must be duplicate")
	}
}

func TestNewResources(t *testing.T) {
	resources, violations := NewResources([]ResourceInput{
		{Name: "ok.bpmn", Content: []byte("x")},
		{Name: "rules.dmn", Content: []byte("x")},
		{Name: "ok.bpmn", Content: []byte("y")},
		{Name: "empty.bpmn"},
	})
	if len(resources) != 1 || resources[0].Name != "ok.bpmn" {
		t.Fatalf("resources = %+v", resources)
	}
	codes := map[string]bool{}
	for _, v := range violations {
		codes[v.Code] = true
	}
	for _, want := range []string{CodeUnsupportedResourceType, CodeDuplicateResourceName, CodeEmptyResource} {
		if !codes[want] {
			t.Errorf("missing violation %s in %v", want, violations)
		}
	}
	if _, v := NewResources(nil); len(v) != 1 || v[0].Code != CodeNoResources {
		t.Fatalf("empty input violations = %v", v)
	}
}

func TestCheckUniqueProcessIDs(t *testing.T) {
	v := CheckUniqueProcessIDs([]ProcessCandidate{
		{ResourceName: "a.bpmn", BpmnProcessID: "order"},
		{ResourceName: "b.bpmn", BpmnProcessID: "order"},
		{ResourceName: "b.bpmn", BpmnProcessID: "payment"},
	})
	if len(v) != 1 || v[0].Code != CodeDuplicateProcessID || v[0].ResourceName != "b.bpmn" {
		t.Fatalf("violations = %v", v)
	}
}
