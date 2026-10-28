package sdk

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"connectrpc.com/connect"

	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/deployment/v1/deploymentv1connect"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/incident/v1/incidentv1connect"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1/jobv1connect"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/message/v1/messagev1connect"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/process/v1/processv1connect"
)

// Version adalah versi SDK yang dikirim di User-Agent. API SDK masih v0 sampai Fase 6.
const Version = "0.4.0"

// Client memanggil semua service engine lewat Connect. Aman dipakai banyak goroutine.
type Client struct {
	deployments deploymentv1connect.DeploymentServiceClient
	instances   processv1connect.ProcessInstanceServiceClient
	jobs        jobv1connect.JobServiceClient
	incidents   incidentv1connect.IncidentServiceClient
	messages    messagev1connect.MessageServiceClient
}

type options struct {
	httpClient  connect.HTTPClient
	tokenSource TokenSource
	retry       RetryPolicy
	userAgent   string
	json        bool
}

// Option mengubah konfigurasi Client.
type Option func(*options)

// WithHTTPClient memakai HTTP client sendiri (mis. dengan TLS atau proxy). Default: http.Client
// tanpa timeout; batas waktu diatur per panggilan lewat context.
func WithHTTPClient(c connect.HTTPClient) Option { return func(o *options) { o.httpClient = c } }

// WithTokenSource mengirim header Authorization: Bearer <token> pada setiap request (Fase 8).
func WithTokenSource(ts TokenSource) Option { return func(o *options) { o.tokenSource = ts } }

// WithRetry mengganti kebijakan retry transport. RetryPolicy{} mematikan retry.
func WithRetry(p RetryPolicy) Option { return func(o *options) { o.retry = p } }

// WithUserAgent mengganti User-Agent.
func WithUserAgent(ua string) Option { return func(o *options) { o.userAgent = ua } }

// WithJSON memakai protokol Connect JSON (untuk debug); default proto biner.
func WithJSON() Option { return func(o *options) { o.json = true } }

// NewClient membuat client untuk engine di baseURL, misalnya "http://localhost:8080".
func NewClient(baseURL string, opts ...Option) (*Client, error) {
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		return nil, fmt.Errorf("sdk: base URL %q must start with http:// or https://", baseURL)
	}
	o := options{httpClient: &http.Client{}, retry: DefaultRetryPolicy(), userAgent: "workflow-sdk-go/" + Version}
	for _, opt := range opts {
		opt(&o)
	}
	interceptors := []connect.Interceptor{headerInterceptor(o.userAgent, o.tokenSource)}
	if o.retry.MaxAttempts > 1 {
		interceptors = append(interceptors, retryInterceptor(o.retry))
	}
	clientOpts := []connect.ClientOption{connect.WithInterceptors(interceptors...)}
	if o.json {
		clientOpts = append(clientOpts, connect.WithProtoJSON())
	}
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		deployments: deploymentv1connect.NewDeploymentServiceClient(o.httpClient, baseURL, clientOpts...),
		instances:   processv1connect.NewProcessInstanceServiceClient(o.httpClient, baseURL, clientOpts...),
		jobs:        jobv1connect.NewJobServiceClient(o.httpClient, baseURL, clientOpts...),
		incidents:   incidentv1connect.NewIncidentServiceClient(o.httpClient, baseURL, clientOpts...),
		messages:    messagev1connect.NewMessageServiceClient(o.httpClient, baseURL, clientOpts...),
	}, nil
}

// Jobs mengembalikan client JobService mentah.
func (c *Client) Jobs() jobv1connect.JobServiceClient { return c.jobs }

// ProcessInstances mengembalikan client ProcessInstanceService mentah.
func (c *Client) ProcessInstances() processv1connect.ProcessInstanceServiceClient { return c.instances }

// Deployments mengembalikan client DeploymentService mentah.
func (c *Client) Deployments() deploymentv1connect.DeploymentServiceClient { return c.deployments }

// Incidents mengembalikan client IncidentService mentah.
func (c *Client) Incidents() incidentv1connect.IncidentServiceClient { return c.incidents }

// Messages mengembalikan client MessageService mentah.
func (c *Client) Messages() messagev1connect.MessageServiceClient { return c.messages }

// TokenSource memberi bearer token untuk setiap request; error membatalkan request.
type TokenSource func(ctx context.Context) (string, error)

func headerInterceptor(userAgent string, ts TokenSource) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			req.Header().Set("User-Agent", userAgent)
			if ts != nil {
				token, err := ts(ctx)
				if err != nil {
					return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("sdk: token source: %w", err))
				}
				if token != "" {
					req.Header().Set("Authorization", "Bearer "+token)
				}
			}
			return next(ctx, req)
		}
	}
}
