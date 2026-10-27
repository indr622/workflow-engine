package application

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/domain"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/eventbus"
	"github.com/indr622/workflow-engine/libs/go/bpmn"
)

func bpmnXML(processID, taskType string) []byte {
	return []byte(strings.ReplaceAll(strings.ReplaceAll(`<?xml version="1.0" encoding="UTF-8"?>
<bpmn:definitions xmlns:bpmn="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:zeebe="http://camunda.org/schema/zeebe/1.0" id="defs" targetNamespace="http://bpmn.io/schema/bpmn">
  <bpmn:process id="PID" name="Process PID" isExecutable="true">
    <bpmn:startEvent id="start"><bpmn:outgoing>f1</bpmn:outgoing></bpmn:startEvent>
    <bpmn:serviceTask id="task"><bpmn:extensionElements><zeebe:taskDefinition type="TTYPE" /></bpmn:extensionElements><bpmn:incoming>f1</bpmn:incoming><bpmn:outgoing>f2</bpmn:outgoing></bpmn:serviceTask>
    <bpmn:endEvent id="end"><bpmn:incoming>f2</bpmn:incoming></bpmn:endEvent>
    <bpmn:sequenceFlow id="f1" sourceRef="start" targetRef="task" />
    <bpmn:sequenceFlow id="f2" sourceRef="task" targetRef="end" />
  </bpmn:process>
</bpmn:definitions>`, "PID", processID), "TTYPE", taskType))
}

type harness struct {
	repo   *fakeRepository
	bus    *eventbus.InMemory
	cache  *ProcessCache
	deploy *DeployResource
	events []api.ProcessDeployed
}

func newHarness() *harness {
	h := &harness{repo: newFakeRepository(), bus: eventbus.NewInMemory()}
	h.cache = NewProcessCache(h.repo, 16)
	h.deploy = NewDeployResource(h.repo, h.bus, h.cache, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.bus.Subscribe(api.TopicProcessDeployed, func(_ context.Context, ev eventbus.Event) error {
		h.events = append(h.events, ev.(api.ProcessDeployed))
		return nil
	})
	return h
}

func TestDeployCreatesVersionsAndDedupsIdenticalResource(t *testing.T) {
	h := newHarness()
	ctx := context.Background()

	first, err := h.deploy.Execute(ctx, "", []domain.ResourceInput{{Name: "order.bpmn", Content: bpmnXML("order", "a")}})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Definitions) != 1 || first.Definitions[0].Version != 1 || first.Definitions[0].Duplicate {
		t.Fatalf("first deploy = %+v", first.Definitions)
	}

	again, err := h.deploy.Execute(ctx, "", []domain.ResourceInput{{Name: "order.bpmn", Content: bpmnXML("order", "a")}})
	if err != nil {
		t.Fatal(err)
	}
	if d := again.Definitions[0]; !d.Duplicate || d.Version != 1 || d.Key != first.Definitions[0].Key {
		t.Fatalf("identical redeploy must reuse version 1, got %+v", d)
	}
	if again.DeploymentKey == first.DeploymentKey {
		t.Fatal("each deploy request gets its own deployment key")
	}

	changed, err := h.deploy.Execute(ctx, "", []domain.ResourceInput{{Name: "order.bpmn", Content: bpmnXML("order", "b")}})
	if err != nil {
		t.Fatal(err)
	}
	if d := changed.Definitions[0]; d.Duplicate || d.Version != 2 {
		t.Fatalf("changed resource must create version 2, got %+v", d)
	}

	if len(h.events) != 2 {
		t.Fatalf("events = %d, want 2 (duplicates publish nothing)", len(h.events))
	}
	if h.events[1].Version != 2 || h.events[1].BpmnProcessID != "order" {
		t.Fatalf("second event = %+v", h.events[1])
	}
	if h.cache.Len() != 2 {
		t.Fatalf("cache holds %d models, want 2", h.cache.Len())
	}

	meta, proc, err := h.cache.GetLatest(ctx, "order")
	if err != nil || meta.Version != 2 || proc.ID() != "order" {
		t.Fatalf("GetLatest = %+v, %v, %v", meta, proc, err)
	}
}

func TestDeployRejectsInvalidResourceAtomically(t *testing.T) {
	h := newHarness()
	broken := []byte(strings.Replace(string(bpmnXML("broken", "x")), `<zeebe:taskDefinition type="x" />`, "", 1))

	_, err := h.deploy.Execute(context.Background(), "", []domain.ResourceInput{
		{Name: "ok.bpmn", Content: bpmnXML("ok", "a")},
		{Name: "broken.bpmn", Content: broken},
		{Name: "rules.dmn", Content: []byte("<definitions/>")},
	})
	var rejected *domain.RejectedError
	if !errors.As(err, &rejected) {
		t.Fatalf("err = %v, want RejectedError", err)
	}
	codes := map[string]string{}
	for _, v := range rejected.Errors() {
		codes[v.Code] = v.ResourceName
	}
	if codes[string(bpmn.CodeTaskDefinitionMissing)] != "broken.bpmn" {
		t.Errorf("missing TASK_DEFINITION_MISSING for broken.bpmn: %v", rejected.Violations)
	}
	if codes[domain.CodeUnsupportedResourceType] != "rules.dmn" {
		t.Errorf("missing UNSUPPORTED_RESOURCE_TYPE for rules.dmn: %v", rejected.Violations)
	}
	if h.repo.deployments != 0 || len(h.repo.definitions) != 0 {
		t.Fatal("nothing may be stored when the deployment is rejected")
	}
	if len(h.events) != 0 {
		t.Fatal("no events on rejection")
	}
}

func TestDeployRejectsDuplicateProcessIDAcrossResources(t *testing.T) {
	h := newHarness()
	_, err := h.deploy.Execute(context.Background(), "", []domain.ResourceInput{
		{Name: "a.bpmn", Content: bpmnXML("same", "a")},
		{Name: "b.bpmn", Content: bpmnXML("same", "b")},
	})
	var rejected *domain.RejectedError
	if !errors.As(err, &rejected) || rejected.Errors()[0].Code != domain.CodeDuplicateProcessID {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateResourceDoesNotStore(t *testing.T) {
	h := newHarness()
	res := ValidateResource{}.Execute([]domain.ResourceInput{{Name: "x.bpmn", Content: bpmnXML("x", "a")}}, bpmn.ProfileExecutable)
	if !res.Valid || len(res.Processes) != 1 || res.Processes[0].ElementCount != 5 {
		t.Fatalf("validate = %+v", res)
	}
	if h.repo.deployments != 0 {
		t.Fatal("validate must not store")
	}
}

func TestListProcessDefinitionsPagination(t *testing.T) {
	h := newHarness()
	ctx := context.Background()
	for _, id := range []string{"a", "b", "c"} {
		for _, tt := range []string{"1", "2"} {
			if _, err := h.deploy.Execute(ctx, "", []domain.ResourceInput{{Name: id + ".bpmn", Content: bpmnXML(id, tt)}}); err != nil {
				t.Fatal(err)
			}
		}
	}
	q := NewQueries(h.repo)
	var all []domain.ProcessDefinition
	token := ""
	for pages := 0; ; pages++ {
		items, next, err := q.ListProcessDefinitions(ctx, ListInput{PageSize: 4, PageToken: token})
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, items...)
		if next == "" {
			break
		}
		token = next
		if pages > 5 {
			t.Fatal("pagination does not terminate")
		}
	}
	if len(all) != 6 {
		t.Fatalf("paginated total = %d, want 6", len(all))
	}
	latest, _, err := q.ListProcessDefinitions(ctx, ListInput{LatestOnly: true})
	if err != nil || len(latest) != 3 || latest[0].Version != 2 {
		t.Fatalf("latest only = %+v, %v", latest, err)
	}
	if _, _, err := q.ListProcessDefinitions(ctx, ListInput{PageToken: "!!!"}); !errors.Is(err, ErrInvalidPageToken) {
		t.Fatalf("bad token err = %v", err)
	}
}
