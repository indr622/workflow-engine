//go:build integration

package infrastructure_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/application"
)

var loopXML = header + `<bpmn:startEvent id="start"/><bpmn:exclusiveGateway id="merge"/>` + serviceTask("work", "loop-work") +
	`<bpmn:exclusiveGateway id="check"/><bpmn:endEvent id="end"/>` +
	flow("f1", "start", "merge") + flow("f2", "merge", "work") + flow("f3", "work", "check") +
	`<bpmn:sequenceFlow id="again" sourceRef="check" targetRef="merge"><bpmn:conditionExpression xsi:type="bpmn:tFormalExpression">=i &lt; 1000</bpmn:conditionExpression></bpmn:sequenceFlow>` +
	`<bpmn:sequenceFlow id="done" sourceRef="check" targetRef="end"><bpmn:conditionExpression xsi:type="bpmn:tFormalExpression">=i &gt;= 1000</bpmn:conditionExpression></bpmn:sequenceFlow>` +
	footer

// TestRebuildFromSnapshotMatchesFullRebuild: instance loop dengan snapshot setiap 10 record;
// rebuild dari snapshot, rebuild penuh, dan fallback format snapshot tak dikenal memberi proyeksi
// yang identik dengan aslinya.
func TestRebuildFromSnapshotMatchesFullRebuild(t *testing.T) {
	f := setup(t)
	f.repo.WithSnapshotInterval(10)
	defer f.repo.WithSnapshotInterval(5000)
	ctx := context.Background()
	processID := f.deployXML(t, "snap-loop", loopXML)
	created, err := f.commands.CreateProcessInstance(ctx, api.CreateInstanceRequest{BpmnProcessID: processID, Variables: []byte(`{"i": 0}`)})
	if err != nil {
		t.Fatal(err)
	}
	key := created.ProcessInstanceKey
	for i := 1; i <= 25; i++ {
		jobs, _, err := f.queries.ListJobs(ctx, application.JobFilter{ProcessInstanceKey: key, OnlyOpen: true, Limit: 1})
		if err != nil || len(jobs) != 1 {
			t.Fatalf("iteration %d: %d jobs, %v", i, len(jobs), err)
		}
		if err := f.commands.CompleteJob(ctx, jobs[0].Key, []byte(fmt.Sprintf(`{"i": %d}`, i))); err != nil {
			t.Fatal(err)
		}
	}
	var snapSeq, lastSeq int32
	if err := f.pool.QueryRow(ctx, `SELECT s.sequence, pi.last_sequence FROM execution_snapshot s JOIN process_instance pi ON pi.key = s.process_instance_key WHERE s.process_instance_key = $1`, key).Scan(&snapSeq, &lastSeq); err != nil {
		t.Fatalf("no snapshot written: %v", err)
	}
	if snapSeq == 0 || snapSeq > lastSeq || lastSeq-snapSeq > 20 {
		t.Fatalf("snapshot at %d, last sequence %d", snapSeq, lastSeq)
	}
	before := dumpProjection(t, f.pool, key)
	rec := newRecovery(f)

	res, err := rec.RebuildInstance(ctx, key, false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.FromSnapshot || res.Records != int(lastSeq-snapSeq) {
		t.Fatalf("rebuild result %+v, want from snapshot with %d records", res, lastSeq-snapSeq)
	}
	if after := dumpProjection(t, f.pool, key); after != before {
		t.Fatalf("snapshot rebuild changed the projection\nbefore:\n%s\nafter:\n%s", before, after)
	}

	if res, err = rec.RebuildInstance(ctx, key, true); err != nil || res.FromSnapshot || res.Records != int(lastSeq) {
		t.Fatalf("full rebuild %+v, %v", res, err)
	}
	if after := dumpProjection(t, f.pool, key); after != before {
		t.Fatal("full rebuild changed the projection")
	}

	if _, err := f.pool.Exec(ctx, `UPDATE execution_snapshot SET format_version = 99 WHERE process_instance_key = $1`, key); err != nil {
		t.Fatal(err)
	}
	if res, err = rec.RebuildInstance(ctx, key, false); err != nil || res.FromSnapshot {
		t.Fatalf("unknown snapshot format must fall back to a full replay: %+v, %v", res, err)
	}
	if after := dumpProjection(t, f.pool, key); after != before {
		t.Fatal("fallback rebuild changed the projection")
	}
	if m, err := rec.DeepVerify(ctx, key); err != nil || len(m) != 0 {
		t.Fatalf("verify after rebuilds: %v %+v", err, m)
	}
}
