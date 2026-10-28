//go:build integration

package infrastructure_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/application"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/domain"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/infrastructure"
	platformdb "github.com/indr622/workflow-engine/apps/engine/internal/platform/db"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/keygen"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/outbox"
)

// seedJobs menyisipkan n job CREATED bertipe jobType langsung ke tabel untuk menguji semantik kueri
// aktivasi tanpa biaya membuat n instance. Semua job menempel pada satu baris process_instance.
func seedJobs(t *testing.T, f *fixture, jobType string, n int) string {
	t.Helper()
	ctx := context.Background()
	pool := f.pool
	keys := keygen.New()
	pi, _ := keys.NewKey()
	processID := f.deployXML(t, "seed", linearXML)
	var def string
	if err := pool.QueryRow(ctx, `SELECT key::text FROM process_definition WHERE bpmn_process_id = $1`, processID).Scan(&def); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO process_instance
    (key, process_definition_key, bpmn_process_id, process_definition_version, state, start_time)
VALUES ($1, $2, 'seed', 1, 'ACTIVE', now())`, pi, def); err != nil {
		t.Fatal(err)
	}
	jobKeys := make([]string, n)
	for i := range jobKeys {
		jobKeys[i], _ = keys.NewKey()
	}
	if _, err := pool.Exec(ctx, `INSERT INTO job
    (key, process_instance_key, element_instance_key, element_id, bpmn_process_id, process_definition_key,
     process_definition_version, type, retries, custom_headers, state, created_at, updated_at)
SELECT k, $1, $1, 'task', 'seed', $2, 1, $3, 3, '[]', 'CREATED', now(), now() FROM unnest($4::uuid[]) k`,
		pi, def, jobType, jobKeys); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO variable (key, process_instance_key, scope_key, name, value, updated_at)
VALUES ($1, $3, $3, 'a', '1', now()), ($2, $3, $3, 'b', '"x"', now())`, mustKey(keys), mustKey(keys), pi); err != nil {
		t.Fatal(err)
	}
	return pi
}

func mustKey(g *keygen.UUIDv7) string {
	k, err := g.NewKey()
	if err != nil {
		panic(err)
	}
	return k
}

func cleanupInstance(t *testing.T, pool *pgxpool.Pool, pi string) {
	t.Cleanup(func() {
		ctx := context.Background()
		for _, table := range []string{"variable", "job"} {
			_, _ = pool.Exec(ctx, "DELETE FROM "+table+" WHERE process_instance_key = $1", pi)
		}
		_, _ = pool.Exec(ctx, "DELETE FROM process_instance WHERE key = $1", pi)
	})
}

// TestActivateJobsExactlyOnceUnderConcurrency: 20 worker dengan pool terpisah mengaktifkan 2.000 job
// bertipe sama; setiap job diaktifkan tepat satu kali dan tidak ada yang tertinggal.
func TestActivateJobsExactlyOnceUnderConcurrency(t *testing.T) {
	f := setup(t)
	jobType := "exactly-once-" + f.run
	pi := seedJobs(t, f, jobType, 2000)
	cleanupInstance(t, f.pool, pi)

	const workers = 20
	var (
		mu   sync.Mutex
		seen = map[string]int{}
		wg   sync.WaitGroup
		errs = make(chan error, workers)
	)
	for w := 0; w < workers; w++ {
		pool, err := platformdb.Connect(context.Background(), platformdb.ConfigFromURL(os.Getenv("DATABASE_URL")))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(pool.Close)
		repo := infrastructure.NewPostgresRepository(pool, outbox.NewWriter(keygen.New()))
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for {
				jobs, err := repo.ActivateJobs(context.Background(), api.ActivationQuery{
					Type: jobType, Worker: fmt.Sprintf("w%d", w), MaxJobs: 32, Timeout: time.Minute,
				})
				if err != nil {
					errs <- err
					return
				}
				if len(jobs) == 0 {
					return
				}
				mu.Lock()
				for _, j := range jobs {
					seen[j.Key]++
				}
				mu.Unlock()
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if len(seen) != 2000 {
		t.Fatalf("activated %d distinct jobs, want 2000", len(seen))
	}
	for k, n := range seen {
		if n != 1 {
			t.Fatalf("job %s activated %d times", k, n)
		}
	}
	var left int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM job WHERE type = $1 AND state = 'CREATED'`, jobType).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("%d jobs left CREATED", left)
	}
}

func TestActivationQuerySemantics(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	jobType := "semantics-" + f.run
	pi := seedJobs(t, f, jobType, 30)
	cleanupInstance(t, f.pool, pi)

	// (b) job dengan activatable_at di masa depan tidak terambil.
	if _, err := f.pool.Exec(ctx, `UPDATE job SET activatable_at = now() + interval '1 hour'
WHERE key IN (SELECT key FROM job WHERE type = $1 ORDER BY key LIMIT 5)`, jobType); err != nil {
		t.Fatal(err)
	}
	// (c) baris yang dikunci transaksi lain dilewati, bukan ditunggu.
	locker, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = locker.Rollback(ctx) }()
	rows, err := locker.Query(ctx, `SELECT key::text FROM job WHERE type = $1 AND activatable_at IS NULL ORDER BY key LIMIT 10 FOR UPDATE`, jobType)
	if err != nil {
		t.Fatal(err)
	}
	locked := map[string]bool{}
	for rows.Next() {
		var k string
		_ = rows.Scan(&k)
		locked[k] = true
	}
	rows.Close()

	start := time.Now()
	jobs, err := f.repo.ActivateJobs(ctx, api.ActivationQuery{Type: jobType, Worker: "w", MaxJobs: 100, Timeout: 90 * time.Second,
		FetchVariables: []string{"b", "missing"}})
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("activation waited for locked rows")
	}
	if len(jobs) != 15 {
		t.Fatalf("activated %d jobs, want 15 (30 - 5 future - 10 locked)", len(jobs))
	}
	for _, j := range jobs {
		if locked[j.Key] {
			t.Fatalf("locked job %s was activated", j.Key)
		}
		// (e) subset variabel; (f) deadline = now + timeout menurut jam Postgres.
		if string(j.Variables) != `{"b":"x"}` {
			t.Fatalf("variables = %s", j.Variables)
		}
		if d := time.Until(j.Deadline); d < 80*time.Second || d > 100*time.Second {
			t.Fatalf("deadline in %s, want ~90s", d)
		}
		if j.Worker != "w" || j.Retries != 3 {
			t.Fatalf("job = %+v", j)
		}
	}
	_ = locker.Rollback(ctx)

	// (d) kueri aktivasi bisa memakai indeks parsial job_activatable_idx.
	conn, err := f.pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SET enable_seqscan = off"); err != nil {
		t.Fatal(err)
	}
	var plan string
	if err := conn.QueryRow(ctx, `EXPLAIN (FORMAT JSON) SELECT key FROM job
WHERE type = $1 AND state = 'CREATED' AND (activatable_at IS NULL OR activatable_at <= now())
ORDER BY activatable_at NULLS FIRST, key LIMIT 32 FOR UPDATE SKIP LOCKED`, jobType).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	_, _ = conn.Exec(ctx, "RESET enable_seqscan")
	if !strings.Contains(plan, "job_activatable_idx") {
		t.Fatalf("activation plan does not use job_activatable_idx: %s", plan)
	}
}

func TestLeaseExtendAndExpiry(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	jobType := "lease-" + f.run
	pi := seedJobs(t, f, jobType, 2)
	cleanupInstance(t, f.pool, pi)

	jobs, err := f.repo.ActivateJobs(ctx, api.ActivationQuery{Type: jobType, Worker: "w", MaxJobs: 2, Timeout: time.Second})
	if err != nil || len(jobs) != 2 {
		t.Fatalf("activate: %v %d", err, len(jobs))
	}
	if err := f.repo.ExtendLease(ctx, jobs[0].Key, time.Hour); err != nil {
		t.Fatal(err)
	}
	// Lease job kedua habis: dikembalikan ke CREATED.
	if _, err := f.pool.Exec(ctx, `UPDATE job SET deadline = now() - interval '1 second' WHERE key = $1`, jobs[1].Key); err != nil {
		t.Fatal(err)
	}
	released, err := f.repo.ReleaseExpiredLeases(ctx, 500)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range released {
		if r.Key == jobs[0].Key {
			t.Fatal("extended lease must not be released")
		}
	}
	// Sweeper engine lain (mis. paket e2e yang berjalan paralel) boleh melepas lebih dulu; yang
	// diperiksa adalah state job, bukan siapa yang melepasnya.
	var state string
	if err := f.pool.QueryRow(ctx, `SELECT state FROM job WHERE key = $1`, jobs[1].Key).Scan(&state); err != nil || state != "CREATED" {
		t.Fatalf("expired lease state = %q %v, want CREATED", state, err)
	}
	again, err := f.repo.ActivateJobs(ctx, api.ActivationQuery{Type: jobType, Worker: "w2", MaxJobs: 5, Timeout: time.Minute})
	if err != nil || len(again) != 1 || again[0].Key != jobs[1].Key {
		t.Fatalf("reactivation = %v %v", again, err)
	}
	var count, timeouts int32
	if err := f.pool.QueryRow(ctx, `SELECT activation_count, timeout_count FROM job WHERE key = $1`, jobs[1].Key).Scan(&count, &timeouts); err != nil {
		t.Fatal(err)
	}
	if count != 2 || timeouts != 1 {
		t.Fatalf("activation_count=%d timeout_count=%d, want 2 and 1", count, timeouts)
	}

	if _, err := f.pool.Exec(ctx, `UPDATE job SET state = 'CREATED', worker = NULL, deadline = NULL WHERE key = $1`, jobs[0].Key); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.ExtendLease(ctx, jobs[0].Key, time.Minute); !errors.Is(err, domain.ErrJobNotActivated) {
		t.Fatalf("extend on CREATED = %v, want ErrJobNotActivated", err)
	}
	if err := f.repo.ExtendLease(ctx, mustKey(keygen.New()), time.Minute); !errors.Is(err, domain.ErrJobNotFound) {
		t.Fatalf("extend on missing job = %v, want ErrJobNotFound", err)
	}
}

var retryXML = header + `<bpmn:startEvent id="start"/>` +
	`<bpmn:serviceTask id="charge"><bpmn:extensionElements><zeebe:taskDefinition type="charge-PID" retries="2"/></bpmn:extensionElements></bpmn:serviceTask>` +
	`<bpmn:endEvent id="end"/>` + flow("f1", "start", "charge") + flow("f2", "charge", "end") + footer

func activateOne(t *testing.T, f *fixture, jobType string) api.ActivatedJob {
	t.Helper()
	jobs, err := f.commands.ActivateJobs(context.Background(), api.ActivationQuery{Type: jobType, Worker: "it", MaxJobs: 1, Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("want one activatable %s job, got %d", jobType, len(jobs))
	}
	return jobs[0]
}

// TestWorkerLifecycleKeepsProjectionReplayable menjalankan fail dengan backoff, fail tanpa retries,
// update retries, resolve, throw error, dan complete, lalu memastikan proyeksi sama dengan replay
// dan rebuild menghasilkan kolom turunan record yang sama.
func TestWorkerLifecycleKeepsProjectionReplayable(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	id := f.deployXML(t, "retry", retryXML)
	jobType := "charge-" + id
	res, err := f.commands.CreateProcessInstance(ctx, api.CreateInstanceRequest{BpmnProcessID: id, Variables: []byte(`{"order":7}`)})
	if err != nil {
		t.Fatal(err)
	}
	pi := res.ProcessInstanceKey

	job := activateOne(t, f, jobType)
	if string(job.Variables) != `{"order":7}` || job.Retries != 2 {
		t.Fatalf("activated job = %+v vars=%s", job, job.Variables)
	}
	if _, err := f.commands.FailJob(ctx, api.JobFailure{JobKey: job.Key, Retries: 1, RetryBackoff: time.Hour, ErrorMessage: "later"}); err != nil {
		t.Fatal(err)
	}
	if jobs, _ := f.commands.ActivateJobs(ctx, api.ActivationQuery{Type: jobType, Worker: "it", MaxJobs: 1, Timeout: time.Minute}); len(jobs) != 0 {
		t.Fatal("job in backoff must not be activatable")
	}
	assertReplay(t, f, pi, true)
	if _, err := f.pool.Exec(ctx, `UPDATE job SET activatable_at = now() - interval '1 second' WHERE key = $1`, job.Key); err != nil {
		t.Fatal(err)
	}
	job = activateOne(t, f, jobType)
	if job.Retries != 1 {
		t.Fatalf("retries after fail = %d", job.Retries)
	}
	if _, err := f.commands.FailJob(ctx, api.JobFailure{JobKey: job.Key, Retries: 0, ErrorMessage: "permanent"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.commands.CompleteJob(ctx, job.Key, nil, nil); !errors.Is(err, domain.ErrJobNotActive) {
		t.Fatalf("complete on FAILED = %v", err)
	}
	incidents, _, err := f.queries.ListIncidents(ctx, application.IncidentFilter{ProcessInstanceKey: pi, OnlyOpen: true})
	if err != nil || len(incidents) != 1 || incidents[0].ErrorType != "JOB_NO_RETRIES" || incidents[0].JobKey != job.Key {
		t.Fatalf("incidents = %+v %v", incidents, err)
	}
	assertReplay(t, f, pi, true)

	if _, err := f.commands.UpdateJobRetries(ctx, job.Key, 3, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.commands.ResolveIncident(ctx, incidents[0].Key, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.commands.ResolveIncident(ctx, incidents[0].Key, nil); !errors.Is(err, domain.ErrIncidentAlreadyResolved) {
		t.Fatalf("second resolve = %v", err)
	}
	job = activateOne(t, f, jobType)
	if _, err := f.commands.ThrowError(ctx, api.JobError{JobKey: job.Key, ErrorCode: "DECLINED", ErrorMessage: "card"}); err != nil {
		t.Fatal(err)
	}
	incidents, _, _ = f.queries.ListIncidents(ctx, application.IncidentFilter{ProcessInstanceKey: pi, OnlyOpen: true})
	if len(incidents) != 1 || incidents[0].ErrorType != "UNHANDLED_ERROR_EVENT" {
		t.Fatalf("incidents after throw = %+v", incidents)
	}
	assertReplay(t, f, pi, true)

	before := projectionWithoutLease(t, f.pool, pi)
	if _, err := newRecovery(f).RebuildInstance(ctx, pi, true); err != nil {
		t.Fatal(err)
	}
	if after := projectionWithoutLease(t, f.pool, pi); after != before {
		t.Fatalf("rebuild changed record-derived projection\nbefore: %s\nafter:  %s", before, after)
	}

	if _, err := f.commands.ResolveIncident(ctx, incidents[0].Key, nil); err != nil {
		t.Fatal(err)
	}
	job = activateOne(t, f, jobType)
	if _, err := f.commands.CompleteJob(ctx, job.Key, []byte(`{"paid":true}`), nil); err != nil {
		t.Fatal(err)
	}
	assertReplay(t, f, pi, false)
	mismatches, err := newRecovery(f).QuickVerifyFor(ctx, []string{pi})
	if err != nil || len(mismatches) != 0 {
		t.Fatalf("quick verify = %v %v", mismatches, err)
	}
}

// projectionWithoutLease membaca baris job dan incident tanpa kolom lease (yang sengaja hilang
// saat rebuild, ADR-0020).
func projectionWithoutLease(t *testing.T, pool *pgxpool.Pool, pi string) string {
	t.Helper()
	var out string
	err := pool.QueryRow(context.Background(), `SELECT json_build_object(
  'jobs', (SELECT json_agg(json_build_object('key', key, 'state', CASE WHEN state = 'ACTIVATED' THEN 'CREATED' ELSE state END,
            'retries', retries, 'error_message', error_message, 'error_code', error_code, 'activatable_at', activatable_at) ORDER BY key)
           FROM job WHERE process_instance_key = $1),
  'incidents', (SELECT json_agg(json_build_object('key', key, 'type', error_type, 'state', state, 'job', job_key) ORDER BY key)
                FROM incident WHERE process_instance_key = $1))::text`, pi).Scan(&out)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

var conditionXML = header + `<bpmn:startEvent id="start"/><bpmn:exclusiveGateway id="gw"/>` +
	`<bpmn:endEvent id="big"/><bpmn:endEvent id="small"/>` + flow("f1", "start", "gw") +
	`<bpmn:sequenceFlow id="fb" sourceRef="gw" targetRef="big"><bpmn:conditionExpression xsi:type="bpmn:tFormalExpression">=amount &gt; 100</bpmn:conditionExpression></bpmn:sequenceFlow>` +
	`<bpmn:sequenceFlow id="fs" sourceRef="gw" targetRef="small"><bpmn:conditionExpression xsi:type="bpmn:tFormalExpression">=amount &lt; 10</bpmn:conditionExpression></bpmn:sequenceFlow>` +
	footer

func TestSetVariablesAndResolveConditionIncident(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	id := f.deployXML(t, "cond", conditionXML)
	res, err := f.commands.CreateProcessInstance(ctx, api.CreateInstanceRequest{BpmnProcessID: id, Variables: []byte(`{"amount":50}`)})
	if err != nil {
		t.Fatal(err)
	}
	incidents, _, _ := f.queries.ListIncidents(ctx, application.IncidentFilter{ProcessInstanceKey: res.ProcessInstanceKey, OnlyOpen: true})
	if len(incidents) != 1 || incidents[0].ErrorType != "CONDITION_ERROR" {
		t.Fatalf("incidents = %+v", incidents)
	}
	if _, err := f.commands.SetVariables(ctx, res.ProcessInstanceKey, []byte(`{"amount":500}`), false, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.commands.SetVariables(ctx, mustKey(keygen.New()), []byte(`{"x":1}`), false, nil); !errors.Is(err, domain.ErrScopeNotFound) {
		t.Fatalf("unknown scope = %v", err)
	}
	if _, err := f.commands.ResolveIncident(ctx, incidents[0].Key, nil); err != nil {
		t.Fatal(err)
	}
	assertReplay(t, f, res.ProcessInstanceKey, false)
	d, err := f.queries.GetInstance(ctx, res.ProcessInstanceKey)
	if err != nil || d.Instance.State != "COMPLETED" || d.Instance.HasIncident {
		t.Fatalf("instance = %+v %v", d.Instance, err)
	}
}

func TestCreateProcessInstanceWithResult(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	// Instance tanpa wait state: hasil langsung.
	direct := f.deployXML(t, "direct", header+`<bpmn:startEvent id="s"/><bpmn:endEvent id="e"/>`+flow("f", "s", "e")+footer)
	got, err := f.commands.CreateProcessInstanceWithResult(ctx, api.CreateInstanceRequest{BpmnProcessID: direct, Variables: []byte(`{"a":1,"b":2}`)},
		[]string{"b"}, time.Second)
	if err != nil || got.State != "COMPLETED" || string(got.Variables) != `{"b":2}` {
		t.Fatalf("direct = %+v vars=%s err=%v", got, got.Variables, err)
	}

	// Instance yang diselesaikan goroutine lain.
	id := f.deployXML(t, "await", header+`<bpmn:startEvent id="s"/>`+serviceTask("pay", "pay-PID")+`<bpmn:endEvent id="e"/>`+
		flow("f1", "s", "pay")+flow("f2", "pay", "e")+footer)
	go func() {
		for i := 0; i < 100; i++ {
			jobs, _ := f.commands.ActivateJobs(ctx, api.ActivationQuery{Type: "pay-" + id, Worker: "bg", MaxJobs: 1, Timeout: time.Minute})
			for _, j := range jobs {
				var vars map[string]any
				_ = json.Unmarshal(j.Variables, &vars)
				if vars["marker"] == f.run {
					_, _ = f.commands.CompleteJob(ctx, j.Key, []byte(`{"paid":true}`), nil)
					return
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()
	got, err = f.commands.CreateProcessInstanceWithResult(ctx, api.CreateInstanceRequest{BpmnProcessID: id,
		Variables: []byte(fmt.Sprintf(`{"marker":%q}`, f.run))}, nil, 10*time.Second)
	if err != nil || got.State != "COMPLETED" || !strings.Contains(string(got.Variables), `"paid":true`) {
		t.Fatalf("awaited = %+v vars=%s err=%v", got, got.Variables, err)
	}

	// Timeout: instance tetap berjalan.
	_, err = f.commands.CreateProcessInstanceWithResult(ctx, api.CreateInstanceRequest{BpmnProcessID: f.deployXML(t, "stuck", retryXML)},
		nil, 300*time.Millisecond)
	var timeout *api.ResultTimeoutError
	if !errors.As(err, &timeout) || timeout.ProcessInstanceKey == "" {
		t.Fatalf("timeout err = %v", err)
	}
	d, err := f.queries.GetInstance(ctx, timeout.ProcessInstanceKey)
	if err != nil || d.Instance.State != "ACTIVE" {
		t.Fatalf("timed out instance = %+v %v", d.Instance, err)
	}
}
