//go:build integration

package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/application"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/domain"
	platformdb "github.com/indr622/workflow-engine/apps/engine/internal/platform/db"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/eventbus"
)

func bpmnXML(processID, taskType string) []byte {
	return []byte(strings.ReplaceAll(strings.ReplaceAll(`<?xml version="1.0" encoding="UTF-8"?>
<bpmn:definitions xmlns:bpmn="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:zeebe="http://camunda.org/schema/zeebe/1.0" id="defs" targetNamespace="http://bpmn.io/schema/bpmn">
  <bpmn:process id="PID" isExecutable="true">
    <bpmn:startEvent id="start" />
    <bpmn:serviceTask id="task"><bpmn:extensionElements><zeebe:taskDefinition type="TTYPE" /></bpmn:extensionElements></bpmn:serviceTask>
    <bpmn:endEvent id="end" />
    <bpmn:sequenceFlow id="f1" sourceRef="start" targetRef="task" />
    <bpmn:sequenceFlow id="f2" sourceRef="task" targetRef="end" />
  </bpmn:process>
</bpmn:definitions>`, "PID", processID), "TTYPE", taskType))
}

// TestDeployAgainstPostgres: versi, dedup, dan 20 deploy paralel tanpa versi ganda.
func TestDeployAgainstPostgres(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := platformdb.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := platformdb.Migrate(ctx, pool, log); err != nil {
		t.Fatal(err)
	}

	repo := NewPostgresRepository(pool)
	cache := application.NewProcessCache(repo, 64)
	deploy := application.NewDeployResource(repo, eventbus.NewInMemory(), cache, nil, log)
	// Id unik per run agar test bisa diulang di database yang sama.
	pid := fmt.Sprintf("it-%d", time.Now().UnixNano())

	first, err := deploy.Execute(ctx, "", []domain.ResourceInput{{Name: "p.bpmn", Content: bpmnXML(pid, "a")}})
	if err != nil {
		t.Fatal(err)
	}
	dup, err := deploy.Execute(ctx, "", []domain.ResourceInput{{Name: "p.bpmn", Content: bpmnXML(pid, "a")}})
	if err != nil {
		t.Fatal(err)
	}
	if !dup.Definitions[0].Duplicate || dup.Definitions[0].Key != first.Definitions[0].Key {
		t.Fatalf("duplicate not detected: %+v", dup.Definitions[0])
	}

	// 20 deploy paralel dengan konten berbeda: versi 2..21 harus unik dan berurutan.
	var wg sync.WaitGroup
	versions := make([]int32, 20)
	errs := make([]error, 20)
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := deploy.Execute(ctx, "", []domain.ResourceInput{{Name: "p.bpmn", Content: bpmnXML(pid, fmt.Sprintf("t%d", i))}})
			if err != nil {
				errs[i] = err
				return
			}
			versions[i] = res.Definitions[0].Version
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.Slice(versions, func(a, b int) bool { return versions[a] < versions[b] })
	for i, v := range versions {
		if v != int32(i+2) {
			t.Fatalf("versions = %v, want 2..21 without gaps", versions)
		}
	}

	meta, proc, err := cache.GetLatest(ctx, pid)
	if err != nil || meta.Version != 21 || proc.ID() != pid {
		t.Fatalf("latest = %+v %v %v", meta, proc, err)
	}
	if _, _, err := repo.GetByKey(ctx, -1); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing key err = %v", err)
	}
	items, err := repo.List(ctx, domain.ListQuery{TenantID: domain.DefaultTenant, BpmnProcessID: pid, Limit: 100})
	if err != nil || len(items) != 21 || items[0].Version != 21 {
		t.Fatalf("list = %d items, err %v", len(items), err)
	}
	latest, err := repo.List(ctx, domain.ListQuery{TenantID: domain.DefaultTenant, BpmnProcessID: pid, LatestOnly: true, Limit: 100})
	if err != nil || len(latest) != 1 || latest[0].Version != 21 {
		t.Fatalf("latest list = %+v, err %v", latest, err)
	}
}
