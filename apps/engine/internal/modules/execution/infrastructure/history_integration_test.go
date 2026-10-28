//go:build integration

package infrastructure_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/application"
	processv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/process/v1"
	"github.com/indr622/workflow-engine/libs/go/api/gen/workflow/process/v1/processv1connect"
)

// runLoop membuat instance loopXML dan menyelesaikan n job.
func runLoop(t *testing.T, f *fixture, processID string, n int) string {
	t.Helper()
	ctx := context.Background()
	created, err := f.commands.CreateProcessInstance(ctx, api.CreateInstanceRequest{BpmnProcessID: processID, Variables: []byte(`{"i": 0}`)})
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= n; i++ {
		jobs, _, err := f.queries.ListJobs(ctx, application.JobFilter{ProcessInstanceKey: created.ProcessInstanceKey, OnlyOpen: true, Limit: 1})
		if err != nil || len(jobs) != 1 {
			t.Fatalf("iteration %d: %d jobs, %v", i, len(jobs), err)
		}
		if err := completeJob(ctx, f, jobs[0].Key, []byte(fmt.Sprintf(`{"i": %d}`, i))); err != nil {
			t.Fatal(err)
		}
	}
	return created.ProcessInstanceKey
}

// TestHistoryPagination: record dan element instance berhalaman menyambung tanpa celah; detail
// instance dipotong di 1.000 element instance dengan flag.
func TestHistoryPagination(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	processID := f.deployXML(t, "history-loop", loopXML)
	key := runLoop(t, f, processID, 350) // 3 element instance per putaran → > 1.000
	srv := httptest.NewServer(f.mux)
	defer srv.Close()
	client := processv1connect.NewProcessInstanceServiceClient(srv.Client(), srv.URL)

	var lastSeq int32
	if err := f.pool.QueryRow(ctx, `SELECT last_sequence FROM process_instance WHERE key = $1`, key).Scan(&lastSeq); err != nil {
		t.Fatal(err)
	}
	want, token, pages := int32(1), "", 0
	for {
		res, err := client.ListProcessInstanceRecords(ctx, connect.NewRequest(&processv1.ListProcessInstanceRecordsRequest{
			ProcessInstanceKey: key, PageSize: 137, PageToken: token,
		}))
		if err != nil {
			t.Fatal(err)
		}
		pages++
		for _, r := range res.Msg.GetRecords() {
			if r.GetSequence() != want {
				t.Fatalf("record sequence %d, want %d", r.GetSequence(), want)
			}
			want++
		}
		token = res.Msg.GetNextPageToken()
		if token == "" {
			break
		}
	}
	if want-1 != lastSeq || pages < 2 {
		t.Fatalf("read %d records in %d pages, last sequence %d", want-1, pages, lastSeq)
	}

	detail, err := client.GetProcessInstance(ctx, connect.NewRequest(&processv1.GetProcessInstanceRequest{ProcessInstanceKey: key}))
	if err != nil {
		t.Fatal(err)
	}
	if !detail.Msg.GetElementInstancesTruncated() || len(detail.Msg.GetElementInstances()) != application.MaxElementsInDetail {
		t.Fatalf("detail has %d elements, truncated=%v", len(detail.Msg.GetElementInstances()), detail.Msg.GetElementInstancesTruncated())
	}

	seen, token, prev := 0, "", ""
	for {
		res, err := client.ListElementInstances(ctx, connect.NewRequest(&processv1.ListElementInstancesRequest{
			ProcessInstanceKey: key, PageSize: 500, PageToken: token,
		}))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range res.Msg.GetElementInstances() {
			if e.GetElementInstanceKey() <= prev {
				t.Fatal("element instances are not in key order")
			}
			prev = e.GetElementInstanceKey()
			seen++
		}
		token = res.Msg.GetNextPageToken()
		if token == "" {
			break
		}
	}
	var total int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM element_instance WHERE process_instance_key = $1`, key).Scan(&total); err != nil || seen != total || total <= 1000 {
		t.Fatalf("listed %d of %d element instances, %v", seen, total, err)
	}
	active, err := client.ListElementInstances(ctx, connect.NewRequest(&processv1.ListElementInstancesRequest{ProcessInstanceKey: key, OnlyActive: true}))
	if err != nil || len(active.Msg.GetElementInstances()) != 2 { // PROCESS + service task yang menunggu
		t.Fatalf("active element instances = %d, %v", len(active.Msg.GetElementInstances()), err)
	}

	if _, err := client.ListProcessInstanceRecords(ctx, connect.NewRequest(&processv1.ListProcessInstanceRecordsRequest{
		ProcessInstanceKey: key, PageToken: "not-a-token",
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("bad token: %v", err)
	}
	if _, err := client.ListProcessInstanceRecords(ctx, connect.NewRequest(&processv1.ListProcessInstanceRecordsRequest{
		ProcessInstanceKey: newRequestID(t),
	})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("unknown instance: %v", err)
	}
}

var instantXML = header + `<bpmn:startEvent id="start"/><bpmn:endEvent id="end"/>` + flow("f1", "start", "end") + footer

// TestRetentionPrunesFinishedInstancesUnderLoad: 10.000 instance selesai dihapus per batch sementara
// 50 instance aktif terus diproses; tanpa error dan tanpa menyentuh instance aktif.
func TestRetentionPrunesFinishedInstancesUnderLoad(t *testing.T) {
	f := setup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	instant := f.deployXML(t, "retention-instant", instantXML)
	looping := f.deployXML(t, "retention-loop", loopXML)

	const finished = 10000
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for w := 0; w < 32; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := w; i < finished; i += 32 {
				if _, err := f.commands.CreateProcessInstance(ctx, api.CreateInstanceRequest{BpmnProcessID: instant}); err != nil {
					errs <- err
					return
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE process_instance SET end_time = now() - interval '2 days' WHERE bpmn_process_id = $1`, instant); err != nil {
		t.Fatal(err)
	}

	var active []string
	for i := 0; i < 50; i++ {
		active = append(active, runLoop(t, f, looping, 0))
	}
	retention := application.NewRetention(f.repo, slog.New(slog.NewTextHandler(io.Discard, nil)), 500, 10*time.Millisecond)

	var load sync.WaitGroup
	loadErr := make(chan error, len(active))
	stop := make(chan struct{})
	for _, key := range active {
		load.Add(1)
		go func(key string) {
			defer load.Done()
			for i := 1; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				jobs, _, err := f.queries.ListJobs(ctx, application.JobFilter{ProcessInstanceKey: key, OnlyOpen: true, Limit: 1})
				if err != nil {
					loadErr <- err
					return
				}
				if len(jobs) == 0 {
					return
				}
				if err := completeJob(ctx, f, jobs[0].Key, []byte(fmt.Sprintf(`{"i": %d}`, i))); err != nil {
					loadErr <- err
					return
				}
			}
		}(key)
	}

	dry, err := retention.Prune(ctx, 24*time.Hour, true)
	if err != nil || dry == 0 {
		t.Fatalf("dry run found %d candidates, %v", dry, err)
	}
	pruned, err := retention.Prune(ctx, 24*time.Hour, false)
	close(stop)
	load.Wait()
	close(loadErr)
	for err := range loadErr {
		t.Fatalf("active instance failed during pruning: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if pruned < finished {
		t.Fatalf("pruned %d instances, want at least %d", pruned, finished)
	}
	var left, records int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM process_instance WHERE bpmn_process_id = $1`, instant).Scan(&left); err != nil || left != 0 {
		t.Fatalf("%d finished instances left, %v", left, err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM execution_record r WHERE NOT EXISTS (SELECT 1 FROM process_instance pi WHERE pi.key = r.process_instance_key)`).Scan(&records); err != nil || records != 0 {
		t.Fatalf("%d orphan records, %v", records, err)
	}
	rec := newRecovery(f)
	for _, key := range active {
		if m, err := rec.DeepVerify(ctx, key); err != nil || len(m) != 0 {
			t.Fatalf("active instance %s damaged: %v %+v", key, err, m)
		}
	}
}
