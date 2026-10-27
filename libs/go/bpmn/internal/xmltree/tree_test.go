package xmltree

import (
	"errors"
	"strings"
	"testing"
)

var lim = Limits{MaxBytes: 1 << 20, MaxElements: 1000, MaxDepth: 16}

func TestParseResolvesNamespacesPositionsAndSkipsDI(t *testing.T) {
	doc := `<?xml version="1.0"?>
<bpmn:definitions xmlns:bpmn="http://www.omg.org/spec/BPMN/20100524/MODEL"
  xmlns:bpmndi="http://www.omg.org/spec/BPMN/20100524/DI" id="d">
  <bpmn:process id="p" isExecutable="true">
    <bpmn:conditionExpression> =x &gt; 1 </bpmn:conditionExpression>
  </bpmn:process>
  <bpmndi:BPMNDiagram id="di"><bpmndi:BPMNPlane id="pl"/></bpmndi:BPMNDiagram>
</bpmn:definitions>`
	root, err := Parse(strings.NewReader(doc), lim)
	if err != nil {
		t.Fatal(err)
	}
	if root.Local != "definitions" || root.Space != "http://www.omg.org/spec/BPMN/20100524/MODEL" {
		t.Fatalf("root = %s %s", root.Space, root.Local)
	}
	if len(root.Children) != 1 {
		t.Fatalf("DI not skipped: %d children", len(root.Children))
	}
	proc := root.Children[0]
	if proc.Line != 4 {
		t.Fatalf("process line = %d, want 4", proc.Line)
	}
	if got := proc.Children[0].Text; got != "=x > 1" {
		t.Fatalf("text = %q", got)
	}
}

func TestParseFailures(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		lim  Limits
		kind ErrorKind
	}{
		{"malformed", "<a><b></a>", lim, Malformed},
		{"empty", "", lim, Malformed},
		{"doctype", `<!DOCTYPE a [<!ENTITY x "y">]><a>&x;</a>`, lim, Malformed},
		{"too large", "<a>" + strings.Repeat("x", 100) + "</a>", Limits{MaxBytes: 50}, TooLarge},
		{"too many", "<a><b/><b/><b/></a>", Limits{MaxBytes: 1000, MaxElements: 3}, TooManyElements},
		{"too deep", "<a><a><a><a/></a></a></a>", Limits{MaxBytes: 1000, MaxDepth: 3}, TooDeep},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tt.doc), tt.lim)
			var xe *Error
			if !errors.As(err, &xe) {
				t.Fatalf("err = %v, want *Error", err)
			}
			if xe.Kind != tt.kind {
				t.Fatalf("kind = %d, want %d (%v)", xe.Kind, tt.kind, err)
			}
		})
	}
}
