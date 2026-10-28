package application

import (
	"bytes"
	"testing"

	"github.com/indr622/workflow-engine/libs/go/bpmn"
)

func expressionModel(condition, jobType string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<bpmn:definitions xmlns:bpmn="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:zeebe="http://camunda.org/schema/zeebe/1.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" id="d" targetNamespace="t">
<bpmn:process id="p" isExecutable="true">
<bpmn:startEvent id="s"/><bpmn:exclusiveGateway id="g" default="fd"/>
<bpmn:serviceTask id="t"><bpmn:extensionElements><zeebe:taskDefinition type="` + jobType + `"/></bpmn:extensionElements></bpmn:serviceTask>
<bpmn:endEvent id="e"/>
<bpmn:sequenceFlow id="f1" sourceRef="s" targetRef="g"/>
<bpmn:sequenceFlow id="fc" sourceRef="g" targetRef="t"><bpmn:conditionExpression xsi:type="bpmn:tFormalExpression">` + condition + `</bpmn:conditionExpression></bpmn:sequenceFlow>
<bpmn:sequenceFlow id="fd" sourceRef="g" targetRef="e"/>
<bpmn:sequenceFlow id="f2" sourceRef="t" targetRef="e"/>
</bpmn:process></bpmn:definitions>`
}

func TestCheckExpressions(t *testing.T) {
	tests := []struct {
		name      string
		condition string
		jobType   string
		want      string
		element   string
	}{
		{"supported", "=amount &gt; 10 and approved", "payment", "", ""},
		{"path job type", "=x", "=jobType", "", ""},
		{"no feel prefix", "amount &gt; 10", "payment", CodeExpressionNotSupported, "fc"},
		{"arithmetic", "=amount + 1 &gt; 10", "payment", CodeExpressionNotSupported, "fc"},
		{"function call", "=count(items) &gt; 0", "payment", CodeExpressionNotSupported, "fc"},
		{"syntax error", "=amount &gt;", "payment", CodeExpressionSyntaxError, "fc"},
		{"job type comparison", "=x", "=a = b", CodeExpressionNotSupported, "t"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defs, err := bpmn.Parse(bytes.NewReader([]byte(expressionModel(tt.condition, tt.jobType))))
			if err != nil {
				t.Fatal(err)
			}
			vs := checkExpressions("r.bpmn", defs)
			if tt.want == "" {
				if len(vs) != 0 {
					t.Fatalf("unexpected violations %+v", vs)
				}
				return
			}
			if len(vs) != 1 || vs[0].Code != tt.want || vs[0].ElementID != tt.element || vs[0].Line == 0 {
				t.Fatalf("violations = %+v, want one %s on %s", vs, tt.want, tt.element)
			}
		})
	}
}
