package interfaces_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"

	deploymentv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/deployment/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/deployment/v1/deploymentv1connect"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/application"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/domain"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/interfaces"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/eventbus"
)

// memoryRepository adalah repository in-memory minimal untuk test handler.
type memoryRepository struct {
	nextKey int64
	defs    []domain.ProcessDefinition
	content map[string][]byte
}

func (r *memoryRepository) WithTx(ctx context.Context, fn func(context.Context, domain.TxRepository) error) error {
	return fn(ctx, r)
}
func (r *memoryRepository) GetByKey(_ context.Context, key string) (domain.ProcessDefinition, []byte, error) {
	for _, d := range r.defs {
		if d.Key == key {
			return d, r.content[key], nil
		}
	}
	return domain.ProcessDefinition{}, nil, domain.ErrNotFound
}
func (r *memoryRepository) GetLatest(_ context.Context, _, id string) (domain.ProcessDefinition, []byte, error) {
	pd, ok, _ := r.LatestByProcessID(context.Background(), "", id)
	if !ok {
		return domain.ProcessDefinition{}, nil, domain.ErrNotFound
	}
	return pd, r.content[pd.Key], nil
}
func (r *memoryRepository) GetByVersion(_ context.Context, _, id string, version int32) (domain.ProcessDefinition, []byte, error) {
	for _, d := range r.defs {
		if d.BpmnProcessID == id && d.Version == version {
			return d, r.content[d.Key], nil
		}
	}
	return domain.ProcessDefinition{}, nil, domain.ErrNotFound
}
func (r *memoryRepository) List(context.Context, domain.ListQuery) ([]domain.ProcessDefinition, error) {
	return r.defs, nil
}
func (r *memoryRepository) LockProcessID(context.Context, string, string) error { return nil }
func (r *memoryRepository) LatestByProcessID(_ context.Context, _, id string) (domain.ProcessDefinition, bool, error) {
	var best domain.ProcessDefinition
	found := false
	for _, d := range r.defs {
		if d.BpmnProcessID == id && (!found || d.Version > best.Version) {
			best, found = d, true
		}
	}
	return best, found, nil
}
func (r *memoryRepository) NextKeys(_ context.Context, n int) ([]string, error) {
	keys := make([]string, n)
	for i := range keys {
		r.nextKey++
		keys[i] = fmt.Sprintf("00000000-0000-7000-8000-%012d", r.nextKey)
	}
	return keys, nil
}
func (r *memoryRepository) SaveDeployment(_ context.Context, d domain.Deployment) error {
	for _, pd := range d.Definitions {
		r.defs = append(r.defs, pd)
		for _, res := range d.Resources {
			if res.Name == pd.ResourceName {
				r.content[pd.Key] = res.Content
			}
		}
	}
	return nil
}

func newClient(t *testing.T) deploymentv1connect.DeploymentServiceClient {
	t.Helper()
	repo := &memoryRepository{content: map[string][]byte{}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	deploy := application.NewDeployResource(repo, eventbus.NewInMemory(), application.NewProcessCache(repo, 8), nil, log)
	mux := http.NewServeMux()
	interfaces.NewHandler(deploy, application.NewQueries(repo)).Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return deploymentv1connect.NewDeploymentServiceClient(http.DefaultClient, srv.URL)
}

const validXML = `<?xml version="1.0" encoding="UTF-8"?>
<bpmn:definitions xmlns:bpmn="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:zeebe="http://camunda.org/schema/zeebe/1.0" id="d" targetNamespace="http://bpmn.io/schema/bpmn">
  <bpmn:process id="order" name="Order" isExecutable="true">
    <bpmn:startEvent id="start" />
    <bpmn:serviceTask id="task"><bpmn:extensionElements><zeebe:taskDefinition type="work" /></bpmn:extensionElements></bpmn:serviceTask>
    <bpmn:endEvent id="end" />
    <bpmn:sequenceFlow id="f1" sourceRef="start" targetRef="task" />
    <bpmn:sequenceFlow id="f2" sourceRef="task" targetRef="end" />
  </bpmn:process>
</bpmn:definitions>`

func TestDeployThenGetAndList(t *testing.T) {
	client := newClient(t)
	ctx := context.Background()

	res, err := client.DeployResource(ctx, connect.NewRequest(&deploymentv1.DeployResourceRequest{
		Resources: []*deploymentv1.Resource{{Name: "order.bpmn", Content: []byte(validXML)}},
	}))
	if err != nil {
		t.Fatal(err)
	}
	pd := res.Msg.GetProcessDefinitions()[0]
	if pd.GetVersion() != 1 || pd.GetBpmnProcessId() != "order" || pd.GetDuplicate() {
		t.Fatalf("deploy response = %v", pd)
	}

	got, err := client.GetProcessDefinition(ctx, connect.NewRequest(&deploymentv1.GetProcessDefinitionRequest{ProcessDefinitionKey: pd.GetProcessDefinitionKey()}))
	if err != nil || got.Msg.GetBpmnXml() != validXML {
		t.Fatalf("get = %v, err %v", got, err)
	}
	list, err := client.ListProcessDefinitions(ctx, connect.NewRequest(&deploymentv1.ListProcessDefinitionsRequest{}))
	if err != nil || len(list.Msg.GetProcessDefinitions()) != 1 {
		t.Fatalf("list = %v, err %v", list, err)
	}
	_, err = client.GetProcessDefinition(ctx, connect.NewRequest(&deploymentv1.GetProcessDefinitionRequest{ProcessDefinitionKey: "00000000-0000-7000-8000-000000000999"}))
	if connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing key code = %v", connect.CodeOf(err))
	}
}

func TestRejectionCarriesDeploymentRejectionDetail(t *testing.T) {
	client := newClient(t)
	broken := strings.Replace(validXML, `<zeebe:taskDefinition type="work" />`, "", 1)

	_, err := client.DeployResource(context.Background(), connect.NewRequest(&deploymentv1.DeployResourceRequest{
		Resources: []*deploymentv1.Resource{{Name: "order.bpmn", Content: []byte(broken)}},
	}))
	var cerr *connect.Error
	if !errors.As(err, &cerr) || cerr.Code() != connect.CodeInvalidArgument {
		t.Fatalf("err = %v", err)
	}
	var found bool
	for _, d := range cerr.Details() {
		msg, verr := d.Value()
		if verr != nil {
			continue
		}
		if rej, ok := msg.(*deploymentv1.DeploymentRejection); ok {
			found = true
			if len(rej.GetIssues()) == 0 || rej.GetIssues()[0].GetCode() != "TASK_DEFINITION_MISSING" || rej.GetIssues()[0].GetElementId() != "task" {
				t.Fatalf("issues = %v", rej.GetIssues())
			}
		}
	}
	if !found {
		t.Fatal("DeploymentRejection detail missing")
	}

	val, err := client.ValidateResource(context.Background(), connect.NewRequest(&deploymentv1.ValidateResourceRequest{
		Resources: []*deploymentv1.Resource{{Name: "order.bpmn", Content: []byte(broken)}},
	}))
	if err != nil || val.Msg.GetValid() || len(val.Msg.GetIssues()) == 0 {
		t.Fatalf("validate = %v, err %v", val, err)
	}
}

func (r *memoryRepository) BeginIdempotent(context.Context, domain.IdempotencyClaim) ([]byte, bool, error) {
	return nil, false, nil
}

func (r *memoryRepository) StoreIdempotent(context.Context, domain.IdempotencyClaim, []byte) error {
	return nil
}
