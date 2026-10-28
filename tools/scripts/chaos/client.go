package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"

	deploymentv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/deployment/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/deployment/v1/deploymentv1connect"
	jobv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1/jobv1connect"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/process/v1/processv1connect"
)

// replayedHeader ditulis engine pada response yang diambil dari klaim request_id sebelumnya.
const replayedHeader = "Idempotent-Replayed"

// commandTimeout membatasi satu RPC. Tanpa batas ini, RPC yang terjebak di koneksi mati (partisi
// jaringan) menahan worker selamanya; pembatalan di sisi klien juga membatalkan query di engine.
const commandTimeout = 10 * time.Second

// engineClient adalah klien Connect ke satu replika engine.
type engineClient struct {
	name      string
	base      string
	http      *http.Client
	instances processv1connect.ProcessInstanceServiceClient
	jobs      jobv1connect.JobServiceClient
	deploy    deploymentv1connect.DeploymentServiceClient
}

func newEngineClient(name, base string) *engineClient {
	base = strings.TrimRight(base, "/")
	hc := &http.Client{Transport: &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
		MaxIdleConns:        256,
		MaxIdleConnsPerHost: 128,
		IdleConnTimeout:     30 * time.Second,
	}}
	return &engineClient{
		name: name, base: base, http: hc,
		instances: processv1connect.NewProcessInstanceServiceClient(hc, base),
		jobs:      jobv1connect.NewJobServiceClient(hc, base),
		deploy:    deploymentv1connect.NewDeploymentServiceClient(hc, base),
	}
}

// ready mengembalikan true bila /readyz menjawab 200.
func (c *engineClient) ready(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/readyz", http.NoBody)
	if err != nil {
		return false
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// waitReady menunggu /readyz 200 sampai timeout.
func (c *engineClient) waitReady(ctx context.Context, timeout time.Duration) (time.Duration, error) {
	start := time.Now()
	for time.Since(start) < timeout {
		if c.ready(ctx) {
			return time.Since(start), nil
		}
		if err := sleepCtx(ctx, 250*time.Millisecond); err != nil {
			return time.Since(start), err
		}
	}
	return time.Since(start), fmt.Errorf("%s (%s) not ready after %s", c.name, c.base, timeout)
}

// listOpenJobs mengembalikan job terbuka satu instance.
func (c *engineClient) listOpenJobs(ctx context.Context, instanceKey string) ([]*jobv1.JobInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	resp, err := c.jobs.ListJobs(ctx, connect.NewRequest(&jobv1.ListJobsRequest{
		ProcessInstanceKey: instanceKey, OnlyOpen: true, PageSize: 100,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetJobs(), nil
}

// deployCorpus men-deploy korpus proses sampai berhasil (deploy ulang konten sama bersifat no-op).
func (c *engineClient) deployCorpus(ctx context.Context, timeout time.Duration) error {
	req := &deploymentv1.DeployResourceRequest{RequestId: newRequestID()}
	for _, p := range corpus {
		req.Resources = append(req.Resources, &deploymentv1.Resource{Name: p.id + ".bpmn", Content: []byte(p.xml)})
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		cctx, cancel := context.WithTimeout(ctx, commandTimeout)
		_, err := c.deploy.DeployResource(cctx, connect.NewRequest(req))
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		if definitive(err) {
			return fmt.Errorf("deploy corpus: %w", err)
		}
		if err := sleepCtx(ctx, 500*time.Millisecond); err != nil {
			return err
		}
	}
	return fmt.Errorf("deploy corpus: %w", lastErr)
}

// definitive bernilai true untuk error yang tidak boleh dicoba ulang: request ditolak karena
// isinya, bukan karena engine atau database sedang tidak tersedia.
func definitive(err error) bool {
	switch connect.CodeOf(err) {
	case connect.CodeInvalidArgument, connect.CodeNotFound, connect.CodeFailedPrecondition,
		connect.CodeAlreadyExists, connect.CodeUnimplemented, connect.CodePermissionDenied:
		return true
	}
	return false
}

// newRequestID membuat request_id acak (UUIDv4 kanonik).
func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // crypto/rand tidak gagal di platform yang didukung
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b[:])
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// errText meringkas error RPC untuk log.
func errText(err error) string {
	var cerr *connect.Error
	if errors.As(err, &cerr) {
		return cerr.Code().String() + ": " + tail(cerr.Message(), 200)
	}
	return tail(err.Error(), 200)
}
