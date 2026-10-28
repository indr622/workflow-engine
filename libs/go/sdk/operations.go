package sdk

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/durationpb"

	deploymentv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/deployment/v1"
	incidentv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/incident/v1"
	jobv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1"
	messagev1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/message/v1"
	processv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/process/v1"
)

// Resource adalah satu file BPMN yang di-deploy.
type Resource struct {
	Name    string
	Content []byte
}

// ProcessDefinition adalah satu versi proses hasil deploy.
type ProcessDefinition struct {
	Key           string
	BpmnProcessID string
	Version       int32
}

// DeployResult adalah hasil Deploy.
type DeployResult struct {
	DeploymentKey string
	Processes     []ProcessDefinition
}

// Deploy men-deploy resource BPMN secara atomik.
func (c *Client) Deploy(ctx context.Context, resources ...Resource) (*DeployResult, error) {
	req := &deploymentv1.DeployResourceRequest{}
	for _, r := range resources {
		req.Resources = append(req.Resources, &deploymentv1.Resource{Name: r.Name, Content: r.Content})
	}
	res, err := c.deployments.DeployResource(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, wrap(err)
	}
	out := &DeployResult{DeploymentKey: res.Msg.GetDeploymentKey()}
	for _, p := range res.Msg.GetProcessDefinitions() {
		out.Processes = append(out.Processes, ProcessDefinition{Key: p.GetProcessDefinitionKey(), BpmnProcessID: p.GetBpmnProcessId(), Version: p.GetVersion()})
	}
	return out, nil
}

// ProcessRef memilih versi proses: Key, atau BpmnProcessID dengan Version (0 = terbaru).
type ProcessRef struct {
	Key           string
	BpmnProcessID string
	Version       int32
}

// Latest memilih versi terbaru sebuah bpmnProcessId.
func Latest(bpmnProcessID string) ProcessRef { return ProcessRef{BpmnProcessID: bpmnProcessID} }

func (r ProcessRef) request(vars string) *processv1.CreateProcessInstanceRequest {
	req := &processv1.CreateProcessInstanceRequest{Variables: vars}
	if r.Key != "" {
		req.Definition = &processv1.CreateProcessInstanceRequest_ProcessDefinitionKey{ProcessDefinitionKey: r.Key}
	} else {
		req.Definition = &processv1.CreateProcessInstanceRequest_BpmnProcess{
			BpmnProcess: &processv1.ProcessDefinitionRef{BpmnProcessId: r.BpmnProcessID, Version: r.Version},
		}
	}
	return req
}

// InstanceRef adalah process instance yang baru dibuat.
type InstanceRef struct {
	Key                  string
	ProcessDefinitionKey string
	BpmnProcessID        string
	Version              int32
	State                string
}

// CreateInstance membuat process instance.
func (c *Client) CreateInstance(ctx context.Context, ref ProcessRef, vars any) (*InstanceRef, error) {
	doc, err := MarshalVariables(vars)
	if err != nil {
		return nil, err
	}
	res, err := c.instances.CreateProcessInstance(ctx, connect.NewRequest(ref.request(doc)))
	if err != nil {
		return nil, wrap(err)
	}
	m := res.Msg
	return &InstanceRef{
		Key: m.GetProcessInstanceKey(), ProcessDefinitionKey: m.GetProcessDefinitionKey(), BpmnProcessID: m.GetBpmnProcessId(),
		Version: m.GetVersion(), State: stateName(m.GetState().String()),
	}, nil
}

// InstanceResult adalah hasil CreateInstanceWithResult.
type InstanceResult struct {
	InstanceRef
	Variables Variables
}

// ResultOption mengatur CreateInstanceWithResult.
type ResultOption func(*processv1.CreateProcessInstanceWithResultRequest)

// WithResultTimeout mengubah batas tunggu (default server 10 s, maksimum 55 s).
func WithResultTimeout(d time.Duration) ResultOption {
	return func(r *processv1.CreateProcessInstanceWithResultRequest) { r.RequestTimeout = durationpb.New(d) }
}

// WithFetchVariables membatasi variabel yang dikembalikan.
func WithFetchVariables(names ...string) ResultOption {
	return func(r *processv1.CreateProcessInstanceWithResultRequest) { r.FetchVariables = names }
}

// CreateInstanceWithResult membuat instance lalu menunggu sampai selesai. Timeout menghasilkan
// error dengan reason RESULT_TIMEOUT (errors.Is(err, ErrResultTimeout)); instance tetap berjalan.
func (c *Client) CreateInstanceWithResult(ctx context.Context, ref ProcessRef, vars any, opts ...ResultOption) (*InstanceResult, error) {
	doc, err := MarshalVariables(vars)
	if err != nil {
		return nil, err
	}
	req := &processv1.CreateProcessInstanceWithResultRequest{Request: ref.request(doc)}
	for _, o := range opts {
		o(req)
	}
	res, err := c.instances.CreateProcessInstanceWithResult(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, wrap(err)
	}
	m := res.Msg
	return &InstanceResult{
		InstanceRef: InstanceRef{
			Key: m.GetProcessInstanceKey(), ProcessDefinitionKey: m.GetProcessDefinitionKey(), BpmnProcessID: m.GetBpmnProcessId(),
			Version: m.GetVersion(), State: stateName(m.GetState().String()),
		},
		Variables: Variables(m.GetVariables()),
	}, nil
}

// CancelInstance membatalkan process instance.
func (c *Client) CancelInstance(ctx context.Context, instanceKey string) error {
	_, err := c.instances.CancelProcessInstance(ctx, connect.NewRequest(&processv1.CancelProcessInstanceRequest{ProcessInstanceKey: instanceKey}))
	return wrap(err)
}

// SetVariables menulis variabel ke scope process instance pemilik scopeKey.
func (c *Client) SetVariables(ctx context.Context, scopeKey string, vars any) error {
	doc, err := MarshalVariables(vars)
	if err != nil {
		return err
	}
	_, err = c.instances.SetVariables(ctx, connect.NewRequest(&processv1.SetVariablesRequest{ScopeKey: scopeKey, Variables: doc}))
	return wrap(err)
}

// ActivateJobs mengaktifkan job secara langsung (tanpa JobWorker).
func (c *Client) ActivateJobs(ctx context.Context, req *jobv1.ActivateJobsRequest) ([]*Job, error) {
	res, err := c.jobs.ActivateJobs(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, wrap(err)
	}
	out := make([]*Job, 0, len(res.Msg.GetJobs()))
	for _, j := range res.Msg.GetJobs() {
		out = append(out, jobFromProto(j))
	}
	return out, nil
}

// CompleteJob menyelesaikan job dan menggabungkan variabel ke scope process instance.
func (c *Client) CompleteJob(ctx context.Context, jobKey string, vars any) error {
	doc, err := MarshalVariables(vars)
	if err != nil {
		return err
	}
	_, err = c.jobs.CompleteJob(ctx, connect.NewRequest(&jobv1.CompleteJobRequest{JobKey: jobKey, Variables: doc}))
	return wrap(err)
}

// Failure adalah parameter FailJob.
type Failure struct {
	// Retries tersisa yang ditetapkan worker; 0 membuat incident JOB_NO_RETRIES.
	Retries      int32
	ErrorMessage string
	RetryBackoff time.Duration
	Variables    any
}

// FailJob menggagalkan job.
func (c *Client) FailJob(ctx context.Context, jobKey string, f Failure) error {
	doc, err := MarshalVariables(f.Variables)
	if err != nil {
		return err
	}
	_, err = c.jobs.FailJob(ctx, connect.NewRequest(&jobv1.FailJobRequest{
		JobKey: jobKey, Retries: f.Retries, ErrorMessage: f.ErrorMessage, RetryBackOff: durationpb.New(f.RetryBackoff), Variables: doc,
	}))
	return wrap(err)
}

// ThrowError melempar BPMN error dari job.
func (c *Client) ThrowError(ctx context.Context, jobKey, code, message string, vars any) error {
	doc, err := MarshalVariables(vars)
	if err != nil {
		return err
	}
	_, err = c.jobs.ThrowError(ctx, connect.NewRequest(&jobv1.ThrowErrorRequest{JobKey: jobKey, ErrorCode: code, ErrorMessage: message, Variables: doc}))
	return wrap(err)
}

// UpdateJobRetries mengubah retries job; incident tetap perlu ResolveIncident.
func (c *Client) UpdateJobRetries(ctx context.Context, jobKey string, retries int32) error {
	_, err := c.jobs.UpdateJobRetries(ctx, connect.NewRequest(&jobv1.UpdateJobRetriesRequest{JobKey: jobKey, Retries: retries}))
	return wrap(err)
}

// UpdateJobTimeout mengubah deadline lease job menjadi sekarang + timeout.
func (c *Client) UpdateJobTimeout(ctx context.Context, jobKey string, timeout time.Duration) error {
	_, err := c.jobs.UpdateJobTimeout(ctx, connect.NewRequest(&jobv1.UpdateJobTimeoutRequest{JobKey: jobKey, Timeout: durationpb.New(timeout)}))
	return wrap(err)
}

// ResolveIncident menyelesaikan incident dan menjalankan ulang langkah yang gagal.
func (c *Client) ResolveIncident(ctx context.Context, incidentKey string) error {
	_, err := c.incidents.ResolveIncident(ctx, connect.NewRequest(&incidentv1.ResolveIncidentRequest{IncidentKey: incidentKey}))
	return wrap(err)
}

// Message adalah parameter PublishMessage.
type Message struct {
	Name           string
	CorrelationKey string
	// TTL 0 berarti message diterima tetapi tidak disimpan.
	TTL       time.Duration
	MessageID string
	Variables any
}

// PublishMessage mempublikasikan message dan mengembalikan key-nya.
func (c *Client) PublishMessage(ctx context.Context, m Message) (string, error) {
	doc, err := MarshalVariables(m.Variables)
	if err != nil {
		return "", err
	}
	res, err := c.messages.PublishMessage(ctx, connect.NewRequest(&messagev1.PublishMessageRequest{
		Name: m.Name, CorrelationKey: m.CorrelationKey, TimeToLive: durationpb.New(m.TTL), MessageId: m.MessageID, Variables: doc,
	}))
	if err != nil {
		return "", wrap(err)
	}
	return res.Msg.GetMessageKey(), nil
}

// stateName membuang awalan enum, mis. PROCESS_INSTANCE_STATE_COMPLETED → COMPLETED.
func stateName(enum string) string {
	const prefix = "PROCESS_INSTANCE_STATE_"
	if len(enum) > len(prefix) && enum[:len(prefix)] == prefix {
		return enum[len(prefix):]
	}
	return enum
}
