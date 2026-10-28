package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"

	commonv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/common/v1"
	jobv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1"
	processv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/process/v1"
)

// scenario adalah satu baris tabel 11.3 yang dijalankan suite Compose.
type scenario struct {
	number int
	name   string
	title  string
	run    func(ctx context.Context, s *session) error
}

// skippedScenario adalah baris tabel 11.3 yang sengaja tidak dijalankan suite ini.
type skippedScenario struct {
	number int
	name   string
	reason string
}

var scenarios = []scenario{
	{1, "kill-engine", "docker kill -s KILL engine mid-load", runKillEngine},
	{4, "restart-postgres", "docker compose restart postgres mid-load", runRestartPostgres},
	{5, "kill-postgres", "docker kill -s KILL postgres mid-load", runKillPostgres},
	{6, "network-partition", "disconnect postgres from the engine network for 10s", runNetworkPartition},
	{7, "idempotent-parallel", "20 identical requests in parallel", runIdempotentParallel},
	{8, "key-reuse", "request_id reused with a different payload", runKeyReuse},
	{9, "corrupt-projection", "corrupt projection rows, verify, rebuild --instance", runCorruptProjection},
	{10, "replicas", "two replicas under load (profile replicas)", runReplicas},
	{12, "concurrent-start", "two replicas start simultaneously on an empty database", runConcurrentStart},
}

const faultpointReason = "covered by in-process fault-point integration tests (-tags=integration,faultinject)"

var skippedScenarios = []skippedScenario{
	{2, "crash-after-commit", faultpointReason + ": command.after_commit=panic"},
	{3, "relay-crash-before-cursor", faultpointReason + ": outbox.before_cursor_commit=panic"},
	{11, "out-of-order-commit", faultpointReason + ": command.before_commit=sleep"},
	{13, "backup-restore", "covered by make db-backup / db-restore and the backup-restore runbook drill"},
	{14, "retention-under-load", faultpointReason + " (retention janitor with test-only age)"},
	{15, "long-loop-snapshot", faultpointReason + " (snapshot rebuild timing)"},
	{16, "poison-consumer", faultpointReason + "; needs a durable consumer, none is wired in production yet"},
}

// --- 1. kill -9 engine -----------------------------------------------------------------------

func runKillEngine(ctx context.Context, s *session) error {
	if err := s.fresh(ctx, false); err != nil {
		return err
	}
	return s.loadWithInjection(ctx, func(ctx context.Context) error {
		if err := s.dc.kill(ctx, serviceEngine); err != nil {
			return err
		}
		s.logf("engine killed with SIGKILL")
		if err := sleepCtx(ctx, 2*time.Second); err != nil {
			return err
		}
		if err := s.dc.start(ctx, serviceEngine); err != nil {
			return err
		}
		took, err := s.clients[0].waitReady(ctx, s.cfg.readyTimeout)
		s.logf("engine restarted, ready after %s", took.Round(time.Millisecond))
		return err
	})
}

// --- 4. restart postgres ---------------------------------------------------------------------

func runRestartPostgres(ctx context.Context, s *session) error {
	return postgresFault(ctx, s, func(ctx context.Context) error {
		if err := s.dc.restart(ctx, servicePostgres); err != nil {
			return err
		}
		s.logf("postgres restarted")
		return s.dc.waitHealthy(ctx, servicePostgres, 60*time.Second)
	})
}

// --- 5. kill -9 postgres ---------------------------------------------------------------------

func runKillPostgres(ctx context.Context, s *session) error {
	return postgresFault(ctx, s, func(ctx context.Context) error {
		if err := s.dc.kill(ctx, servicePostgres); err != nil {
			return err
		}
		s.logf("postgres killed with SIGKILL")
		if err := sleepCtx(ctx, 2*time.Second); err != nil {
			return err
		}
		if err := s.dc.start(ctx, servicePostgres); err != nil {
			return err
		}
		return s.dc.waitHealthy(ctx, servicePostgres, 60*time.Second)
	})
}

// --- 6. network partition engine <-> postgres ------------------------------------------------

func runNetworkPartition(ctx context.Context, s *session) error {
	return postgresFault(ctx, s, func(ctx context.Context) error {
		if err := s.dc.disconnect(ctx, servicePostgres); err != nil {
			return err
		}
		s.logf("postgres disconnected from %s", chaosNetwork)
		werr := sleepCtx(ctx, 10*time.Second)
		if err := s.dc.connect(ctx, servicePostgres); err != nil {
			return err
		}
		s.logf("postgres reconnected")
		if werr != nil {
			return werr
		}
		return s.dc.waitHealthy(ctx, servicePostgres, 60*time.Second)
	})
}

// postgresFault: gangguan di sisi Postgres; engine harus pulih tanpa restart.
func postgresFault(ctx context.Context, s *session, inject func(context.Context) error) error {
	if err := s.fresh(ctx, false); err != nil {
		return err
	}
	_, startedAt, _, err := s.dc.containerState(ctx, serviceEngine)
	if err != nil {
		return err
	}
	if err := s.loadWithInjection(ctx, inject); err != nil {
		return err
	}
	return s.engineNotRestarted(ctx, startedAt)
}

// --- 7. parallel identical requests ----------------------------------------------------------

const parallelRequests = 20

func runIdempotentParallel(ctx context.Context, s *session) error {
	if err := s.fresh(ctx, false); err != nil {
		return err
	}
	cl := s.clients[0]
	create := newCreate(processByID("chaos-linear"), 0).create
	type result struct {
		body     []byte
		key      string
		replayed bool
		err      error
	}
	results := make([]result, parallelRequests)
	fire := func(call func(context.Context) (proto.Message, string, bool, error)) {
		var wg sync.WaitGroup
		gate := make(chan struct{})
		for i := range results {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-gate
				cctx, cancel := context.WithTimeout(ctx, commandTimeout)
				defer cancel()
				msg, key, replayed, err := call(cctx)
				r := result{key: key, replayed: replayed, err: err}
				if err == nil {
					r.body, r.err = proto.MarshalOptions{Deterministic: true}.Marshal(msg)
				}
				results[i] = r
			}(i)
		}
		close(gate)
		wg.Wait()
	}
	check := func(what string) (string, error) {
		replayed := 0
		for i, r := range results {
			if r.err != nil {
				return "", fmt.Errorf("%s request %d failed: %s", what, i, errText(r.err))
			}
			if r.key != results[0].key || !bytes.Equal(r.body, results[0].body) {
				return "", fmt.Errorf("%s request %d returned a different response (%q vs %q)", what, i, r.key, results[0].key)
			}
			if r.replayed {
				replayed++
			}
		}
		s.logf("%s: %d responses identical, %d marked %s", what, len(results), replayed, replayedHeader)
		if replayed != parallelRequests-1 {
			return "", fmt.Errorf("%s: %d responses replayed, want %d", what, replayed, parallelRequests-1)
		}
		return results[0].key, nil
	}

	fire(func(ctx context.Context) (proto.Message, string, bool, error) {
		resp, err := cl.instances.CreateProcessInstance(ctx, connect.NewRequest(create))
		if err != nil {
			return nil, "", false, err
		}
		return resp.Msg, resp.Msg.GetProcessInstanceKey(), resp.Header().Get(replayedHeader) == "true", nil
	})
	key, err := check("CreateProcessInstance")
	if err != nil {
		return err
	}
	s.ledger.ack(&command{kind: kindCreate, create: create, instanceKey: key})

	jobs, err := cl.listOpenJobs(ctx, key)
	if err != nil || len(jobs) != 1 {
		return fmt.Errorf("expected one open job on %s, got %d (%v)", key, len(jobs), err)
	}
	complete := &jobv1.CompleteJobRequest{JobKey: jobs[0].GetJobKey(), Variables: `{"parallel": true}`, RequestId: newRequestID()}
	fire(func(ctx context.Context) (proto.Message, string, bool, error) {
		resp, err := cl.jobs.CompleteJob(ctx, connect.NewRequest(complete))
		if err != nil {
			return nil, "", false, err
		}
		return resp.Msg, complete.GetJobKey(), resp.Header().Get(replayedHeader) == "true", nil
	})
	if _, err := check("CompleteJob"); err != nil {
		return err
	}
	s.ledger.ack(&command{kind: kindComplete, complete: complete, instanceKey: key})
	return s.recoverAndVerify(ctx)
}

// --- 8. request_id reused with a different payload -------------------------------------------

func runKeyReuse(ctx context.Context, s *session) error {
	if err := s.fresh(ctx, false); err != nil {
		return err
	}
	cl := s.clients[0]
	first := newCreate(processByID("chaos-linear"), 0)
	if err := execute(ctx, s.clients, first); err != nil {
		return fmt.Errorf("first create: %w", err)
	}
	s.ledger.ack(first)
	conn, err := pgx.Connect(ctx, s.cfg.dbURL)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	before, err := dbFingerprint(ctx, conn)
	if err != nil {
		return err
	}

	second := proto.Clone(first.create).(*processv1.CreateProcessInstanceRequest)
	second.Variables = `{"i": 0, "orderId": "different-payload"}`
	cctx, cancel := context.WithTimeout(ctx, commandTimeout)
	_, err = cl.instances.CreateProcessInstance(cctx, connect.NewRequest(second))
	cancel()
	if err := expectKeyReused("CreateProcessInstance", err); err != nil {
		return err
	}
	mid, err := dbFingerprint(ctx, conn)
	if err != nil {
		return err
	}
	if mid != before {
		return fmt.Errorf("state changed after a rejected create reuse: before %s, after %s", before, mid)
	}

	jobs, err := cl.listOpenJobs(ctx, first.instanceKey)
	if err != nil || len(jobs) != 1 {
		return fmt.Errorf("expected one open job, got %d (%v)", len(jobs), err)
	}
	done := newComplete(jobs[0].GetJobKey(), first.instanceKey, `{"a": 1}`, 0)
	if err := execute(ctx, s.clients, done); err != nil {
		return fmt.Errorf("first complete: %w", err)
	}
	s.ledger.ack(done)
	afterComplete, err := dbFingerprint(ctx, conn)
	if err != nil {
		return err
	}
	// Job lain pada instance yang sama, request_id job pertama, payload berbeda.
	next, err := cl.listOpenJobs(ctx, first.instanceKey)
	if err != nil || len(next) != 1 {
		return fmt.Errorf("expected the next open job, got %d (%v)", len(next), err)
	}
	reuse := &jobv1.CompleteJobRequest{JobKey: next[0].GetJobKey(), Variables: `{"a": 2}`, RequestId: done.complete.GetRequestId()}
	cctx, cancel = context.WithTimeout(ctx, commandTimeout)
	_, err = cl.jobs.CompleteJob(cctx, connect.NewRequest(reuse))
	cancel()
	if err := expectKeyReused("CompleteJob", err); err != nil {
		return err
	}
	after, err := dbFingerprint(ctx, conn)
	if err != nil {
		return err
	}
	if after != afterComplete {
		return fmt.Errorf("state changed after a rejected reuse:\nbefore %s\nafter  %s", afterComplete, after)
	}
	s.logf("state unchanged by both rejected requests (%s)", after)

	// Payload asli dengan request_id yang sama tetap me-replay response lama.
	replay := &command{kind: kindCreate, create: first.create}
	if err := execute(ctx, s.clients, replay); err != nil {
		return fmt.Errorf("replay of the original create: %w", err)
	}
	if !replay.replayed || replay.instanceKey != first.instanceKey {
		return fmt.Errorf("original create not replayed: replayed=%v key=%s want %s", replay.replayed, replay.instanceKey, first.instanceKey)
	}
	return s.recoverAndVerify(ctx)
}

func expectKeyReused(rpc string, err error) error {
	if err == nil {
		return fmt.Errorf("%s with a reused request_id and a different payload succeeded", rpc)
	}
	var cerr *connect.Error
	if !errors.As(err, &cerr) || cerr.Code() != connect.CodeInvalidArgument {
		return fmt.Errorf("%s reuse: want InvalidArgument, got %s", rpc, errText(err))
	}
	for _, d := range cerr.Details() {
		if v, derr := d.Value(); derr == nil {
			if _, ok := v.(*commonv1.IdempotencyKeyReused); ok {
				return nil
			}
		}
	}
	return fmt.Errorf("%s reuse: InvalidArgument without an IdempotencyKeyReused detail: %s", rpc, errText(err))
}

// dbFingerprint meringkas jumlah baris state agar perubahan sekecil apa pun terlihat.
func dbFingerprint(ctx context.Context, conn *pgx.Conn) (string, error) {
	var instances, records, jobs, variables, events int
	err := conn.QueryRow(ctx, `SELECT (SELECT count(*) FROM process_instance), (SELECT count(*) FROM execution_record),
  (SELECT count(*) FROM job), (SELECT count(*) FROM variable), (SELECT count(*) FROM outbox_event)`).Scan(&instances, &records, &jobs, &variables, &events)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("instances=%d records=%d jobs=%d variables=%d events=%d", instances, records, jobs, variables, events), nil
}

// --- 9. corrupted projection -----------------------------------------------------------------

func runCorruptProjection(ctx context.Context, s *session) error {
	if err := s.fresh(ctx, false); err != nil {
		return err
	}
	if err := s.loadWithInjection(ctx, nil); err != nil {
		return fmt.Errorf("baseline load: %w", err)
	}
	// Instance khusus yang dibiarkan menunggu di fork: 10 job dan element instance aktif.
	target := newCreate(processByID("chaos-fork"), 0)
	if err := execute(ctx, s.clients, target); err != nil {
		return fmt.Errorf("create target instance: %w", err)
	}
	s.ledger.ack(target)
	key := target.instanceKey
	conn, err := pgx.Connect(ctx, s.cfg.dbURL)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	before, err := dumpProjection(ctx, conn, key)
	if err != nil {
		return err
	}
	var elementKey string
	if err := conn.QueryRow(ctx, `SELECT element_instance_key::text FROM job WHERE process_instance_key = $1 AND state = 'CREATED' ORDER BY key LIMIT 1`, key).Scan(&elementKey); err != nil {
		return fmt.Errorf("pick element instance: %w", err)
	}
	tag, err := conn.Exec(ctx, `DELETE FROM element_instance WHERE key = $1`, elementKey)
	if err != nil || tag.RowsAffected() != 1 {
		return fmt.Errorf("delete element_instance %s: %v (rows %d)", elementKey, err, tag.RowsAffected())
	}
	tag, err = conn.Exec(ctx, `UPDATE variable SET value = '"corrupted"' WHERE process_instance_key = $1 AND name = 'orderId'`, key)
	if err != nil || tag.RowsAffected() != 1 {
		return fmt.Errorf("corrupt variable: %v (rows %d)", err, tag.RowsAffected())
	}
	s.logf("corrupted instance %s: deleted element_instance %s, overwrote variable orderId", key, elementKey)

	code, out := s.engineVerify(ctx)
	if code != 3 {
		return fmt.Errorf("verify after corruption exited %d, want 3: %s", code, tail(out, 800))
	}
	if !strings.Contains(out, key) {
		return fmt.Errorf("verify report does not name the corrupted instance %s: %s", key, tail(out, 800))
	}
	res, err := s.dc.engineCLI(ctx, "rebuild", "--instance", key)
	if err != nil {
		return err
	}
	s.logf("engine rebuild --instance: exit %d: %s", res.ExitCode, strings.TrimSpace(tail(res.Output, 300)))
	if res.ExitCode != 0 {
		return fmt.Errorf("rebuild --instance %s exited %d", key, res.ExitCode)
	}
	after, err := dumpProjection(ctx, conn, key)
	if err != nil {
		return err
	}
	if after != before {
		return fmt.Errorf("projection after rebuild differs from before the corruption:\nbefore:\n%s\nafter:\n%s", tail(before, 1500), tail(after, 1500))
	}
	s.logf("projection after rebuild is identical to the dump before corruption")
	// checkInvariants menjalankan verify --all lagi dan mengharapkan exit 0.
	if v := s.checkInvariants(ctx); len(v) > 0 {
		return fmt.Errorf("%d invariant violations after rebuild; first: %s", len(v), v[0])
	}
	return nil
}

// --- 10. two replicas ------------------------------------------------------------------------

func runReplicas(ctx context.Context, s *session) error {
	if err := s.fresh(ctx, true); err != nil {
		return err
	}
	return s.loadWithInjection(ctx, nil)
}

// --- 12. concurrent start on an empty database -----------------------------------------------

func runConcurrentStart(ctx context.Context, s *session) error {
	s.replicas = true
	if err := s.dc.down(ctx); err != nil {
		return err
	}
	if err := s.dc.up(ctx, servicePostgres); err != nil {
		return err
	}
	start := time.Now()
	if err := s.dc.upNoWait(ctx, serviceEngine, serviceEngine2); err != nil {
		return err
	}
	s.clients = []*engineClient{newEngineClient(serviceEngine, s.cfg.engineURL), newEngineClient(serviceEngine2, s.cfg.engine2URL)}
	if err := s.waitReady(ctx); err != nil {
		return err
	}
	s.logf("both replicas ready %s after a simultaneous start", time.Since(start).Round(time.Millisecond))

	want, err := filepath.Glob(filepath.Join(s.cfg.migrationsDir, "*.sql"))
	if err != nil || len(want) == 0 {
		return fmt.Errorf("no migrations found in %s (%v)", s.cfg.migrationsDir, err)
	}
	conn, err := pgx.Connect(ctx, s.cfg.dbURL)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	var applied, distinct int
	if err := conn.QueryRow(ctx, `SELECT count(*), count(DISTINCT version_id) FROM goose_db_version WHERE version_id > 0`).Scan(&applied, &distinct); err != nil {
		return fmt.Errorf("read goose_db_version: %w", err)
	}
	s.logf("goose_db_version: %d rows, %d distinct versions, %d migration files", applied, distinct, len(want))
	if applied != len(want) || distinct != len(want) {
		return fmt.Errorf("migrations not applied exactly once: %d rows, %d distinct, want %d", applied, distinct, len(want))
	}
	total := 0
	for _, svc := range s.engineServices() {
		n, ok := migratedApplied(s.dc.logs(ctx, svc))
		if !ok {
			return fmt.Errorf("%s did not log \"database migrated\"", svc)
		}
		s.logf("%s applied %d migrations", svc, n)
		total += n
	}
	if total != len(want) {
		return fmt.Errorf("replicas applied %d migrations in total, want %d (each exactly once)", total, len(want))
	}
	if err := s.clients[0].deployCorpus(ctx, 30*time.Second); err != nil {
		return err
	}
	return s.loadWithInjection(ctx, nil)
}

// migratedApplied membaca field `applied` dari log JSON "database migrated".
func migratedApplied(logs string) (int, bool) {
	for _, line := range strings.Split(logs, "\n") {
		if !strings.Contains(line, `"database migrated"`) {
			continue
		}
		var n int
		if i := strings.Index(line, `"applied":`); i >= 0 {
			if _, err := fmt.Sscanf(line[i+len(`"applied":`):], "%d", &n); err == nil {
				return n, true
			}
		}
	}
	return 0, false
}

// writeFailureLog menyimpan log runner dan log compose skenario ke dir/<name>.log.
func writeFailureLog(ctx context.Context, dir string, s *session, name string, runErr error) (string, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name+".log")
	var b strings.Builder
	fmt.Fprintf(&b, "scenario %s failed: %v\n\n=== runner log ===\n%s\n=== docker compose logs ===\n", name, runErr, s.log.String())
	b.WriteString(s.dc.logs(ctx))
	return path, os.WriteFile(path, []byte(b.String()), 0o600)
}
