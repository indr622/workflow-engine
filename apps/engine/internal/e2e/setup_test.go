//go:build integration

// Package e2e menjalankan engine in-process (perakitan internal/app yang sama dengan cmd/engine)
// di atas Postgres nyata dan memanggilnya lewat Connect, termasuk dengan SDK Go.
package e2e

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/indr622/workflow-engine/apps/engine/internal/app"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/config"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/rpc"
	deploymentv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/deployment/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/deployment/v1/deploymentv1connect"
)

type engine struct {
	app    *app.App
	url    string
	run    string
	client *http.Client
}

func startEngine(t testing.TB) *engine {
	t.Helper()
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.SweepInterval = 100 * time.Millisecond
	cfg.Job.LongPollInterval = 200 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	a, err := app.Build(ctx, cfg, log, "e2e")
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	go a.Sweeper.Run(ctx)
	srv := httptest.NewUnstartedServer(a.Mux)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(func() {
		a.BeginShutdown()
		cancel()
		srv.Close()
		a.Pool.Close()
	})
	return &engine{app: a, url: srv.URL, run: fmt.Sprintf("%d", time.Now().UnixNano()), client: srv.Client()}
}

// deploy men-deploy model dengan "PID" diganti id unik dan mengembalikan id itu.
func (e *engine) deploy(t testing.TB, name, xml string) string {
	t.Helper()
	id := name + "-" + e.run
	client := deploymentv1connect.NewDeploymentServiceClient(e.client, e.url)
	_, err := client.DeployResource(context.Background(), connect.NewRequest(&deploymentv1.DeployResourceRequest{
		Resources: []*deploymentv1.Resource{{Name: id + ".bpmn", Content: []byte(strings.ReplaceAll(xml, "PID", id))}},
	}))
	if err != nil {
		t.Fatalf("deploy %s: %v", name, err)
	}
	return id
}

func expectReason(t testing.TB, what string, err error, code connect.Code, reason string) {
	t.Helper()
	if connect.CodeOf(err) != code {
		t.Fatalf("%s: code = %v (%v), want %v", what, connect.CodeOf(err), err, code)
	}
	if info := rpc.ErrorInfoOf(err); info.GetReason() != reason {
		t.Fatalf("%s: reason = %q, want %q", what, info.GetReason(), reason)
	}
}

const header = `<?xml version="1.0" encoding="UTF-8"?>
<bpmn:definitions xmlns:bpmn="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:zeebe="http://camunda.org/schema/zeebe/1.0"
  xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" id="defs" targetNamespace="http://bpmn.io/schema/bpmn">
<bpmn:process id="PID" isExecutable="true">`

const footer = `</bpmn:process></bpmn:definitions>`

func serviceTask(id, jobType, retries string) string {
	r := ""
	if retries != "" {
		r = fmt.Sprintf(` retries=%q`, retries)
	}
	return fmt.Sprintf(`<bpmn:serviceTask id=%q><bpmn:extensionElements><zeebe:taskDefinition type=%q%s/></bpmn:extensionElements></bpmn:serviceTask>`, id, jobType, r)
}

func flow(id, from, to string) string {
	return fmt.Sprintf(`<bpmn:sequenceFlow id=%q sourceRef=%q targetRef=%q/>`, id, from, to)
}
