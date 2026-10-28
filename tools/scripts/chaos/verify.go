package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Query invarian outbox (dicek di level database; lihat README: belum ada consumer durable di
// wiring produksi, jadi "tepat sekali per consumer" diwakili "tepat satu event per record pemicu").
const (
	// Record yang menurut application.eventOf menghasilkan event outbox, tetapi jumlah event-nya
	// (aggregate_key, aggregate_seq) bukan tepat satu.
	sqlEventsNotExactlyOnce = `SELECT count(*) FROM execution_record r
WHERE ((r.value_type IN ('JOB', 'INCIDENT') AND r.intent = 'CREATED')
    OR (r.value_type = 'PROCESS_INSTANCE' AND r.intent IN ('ELEMENT_COMPLETED', 'ELEMENT_TERMINATED')
        AND r.record_key = r.process_instance_key))
  AND (SELECT count(*) FROM outbox_event e
       WHERE e.aggregate_type = 'process_instance' AND e.aggregate_key = r.process_instance_key
         AND e.aggregate_seq = r.sequence) <> 1`
	// Event instance tanpa record pemicu (event tanpa state).
	sqlOrphanEvents = `SELECT count(*) FROM outbox_event e
WHERE e.aggregate_type = 'process_instance'
  AND NOT EXISTS (SELECT 1 FROM execution_record r
                  WHERE r.process_instance_key = e.aggregate_key AND r.sequence = e.aggregate_seq)`
	// aggregate_seq harus naik tegas per instance dalam urutan baca relay (tx_id, event_id).
	sqlNonMonotonicSeq = `SELECT count(*) FROM (
  SELECT aggregate_seq, lag(aggregate_seq) OVER (PARTITION BY aggregate_key ORDER BY tx_id, event_id) AS prev
  FROM outbox_event WHERE aggregate_type = 'process_instance') x
WHERE prev IS NOT NULL AND aggregate_seq <= prev`
	sqlDeadLetters = `SELECT count(*) FROM outbox_dead_letter`
	// Penyelesaian ganda: lebih dari satu record JOB COMPLETED untuk job yang sama.
	sqlDuplicateCompletions = `SELECT record_key::text, count(*) FROM execution_record
WHERE value_type = 'JOB' AND intent = 'COMPLETED' GROUP BY record_key HAVING count(*) > 1`
	sqlCompletedJobs = `SELECT record_key::text FROM execution_record WHERE value_type = 'JOB' AND intent = 'COMPLETED'`
	sqlInstances     = `SELECT key::text FROM process_instance`
)

// logPatterns adalah teks di log engine yang selalu berarti bug, apa pun skenarionya.
var logPatterns = []struct{ pattern, meaning string }{
	{"23505", "unique violation (SQLSTATE 23505), e.g. UNIQUE (process_instance_key, sequence)"},
	{"duplicate key value", "unique violation"},
	{"panic:", "engine panic"},
	{"outbox relay stopped", "outbox relay died"},
}

// checkInvariants memeriksa invarian umum setelah pemulihan dan retry. Mengembalikan daftar
// pelanggaran (kosong bila lulus).
func (s *session) checkInvariants(ctx context.Context) []string {
	var v []string
	add := func(format string, args ...any) { v = append(v, fmt.Sprintf(format, args...)) }

	creates, completes, failed := s.ledger.snapshot()
	for _, f := range failed {
		add("%s", f)
	}
	conn, err := pgx.Connect(ctx, s.cfg.dbURL)
	if err != nil {
		add("connect to chaos postgres: %v", err)
		return v
	}
	defer func() { _ = conn.Close(ctx) }()

	// 1. Setiap create yang di-ack ada; tidak ada instance yang tidak pernah di-ack (instance ganda
	//    dari retry akan muncul di sini, karena retry dengan request_id sama harus me-replay key lama).
	dbInstances, err := querySet(ctx, conn, sqlInstances)
	if err != nil {
		add("list instances: %v", err)
		return v
	}
	missing, orphans := diff(creates, dbInstances)
	if len(missing) > 0 {
		add("%d acknowledged creates missing from process_instance: %s", len(missing), sample(missing))
	}
	if len(orphans) > 0 {
		add("%d instances in the database were never acknowledged (duplicate create?): %s", len(orphans), sample(orphans))
	}

	// 2. Setiap job yang di-ack tepat satu record JOB COMPLETED; tidak ada penyelesaian yang tidak
	//    pernah di-ack.
	rows, err := conn.Query(ctx, sqlDuplicateCompletions)
	if err == nil {
		var dups []string
		for rows.Next() {
			var key string
			var n int
			if err := rows.Scan(&key, &n); err == nil {
				dups = append(dups, fmt.Sprintf("%s×%d", key, n))
			}
		}
		rows.Close()
		if len(dups) > 0 {
			add("%d jobs have duplicate JOB COMPLETED records: %s", len(dups), sample(dups))
		}
	} else {
		add("query duplicate completions: %v", err)
	}
	dbCompleted, err := querySet(ctx, conn, sqlCompletedJobs)
	if err != nil {
		add("list completed jobs: %v", err)
	} else {
		missing, extra := diff(completes, dbCompleted)
		if len(missing) > 0 {
			add("%d acknowledged job completions have no JOB COMPLETED record: %s", len(missing), sample(missing))
		}
		if len(extra) > 0 {
			add("%d job completions in the database were never acknowledged: %s", len(extra), sample(extra))
		}
	}
	var notCompleted int
	keys := make([]string, 0, len(completes))
	for k := range completes {
		keys = append(keys, k)
	}
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM job WHERE key::text = ANY($1) AND state <> 'COMPLETED'`, keys).Scan(&notCompleted); err != nil {
		add("check job state: %v", err)
	} else if notCompleted > 0 {
		add("%d acknowledged job completions are not COMPLETED in the job projection", notCompleted)
	}

	// 3. Outbox.
	for _, q := range []struct{ sql, what string }{
		{sqlEventsNotExactlyOnce, "event-producing records without exactly one outbox event"},
		{sqlOrphanEvents, "outbox events without a triggering execution record"},
		{sqlNonMonotonicSeq, "outbox events with non-increasing aggregate_seq per instance"},
		{sqlDeadLetters, "outbox dead letters"},
	} {
		var n int
		if err := conn.QueryRow(ctx, q.sql).Scan(&n); err != nil {
			add("outbox check %q: %v", q.what, err)
		} else if n > 0 {
			add("%d %s", n, q.what)
		}
	}

	// 4. engine verify --all di image yang sama.
	if code, out := s.engineVerify(ctx); code != 0 {
		add("engine verify --all exited %d: %s", code, strings.TrimSpace(tail(out, 1500)))
	}

	// 5. Log engine.
	v = append(v, s.checkLogs(ctx)...)

	s.logf("invariants: %d acked creates (%d in db), %d acked completions, %d violations",
		len(creates), len(dbInstances), len(completes), len(v))
	return v
}

// engineVerify menjalankan `engine verify --all --json` dan mengembalikan exit code dan output.
func (s *session) engineVerify(ctx context.Context) (int, string) {
	res, err := s.dc.engineCLI(ctx, "verify", "--all", "--json")
	if err != nil {
		return -1, err.Error()
	}
	s.logf("engine verify --all: exit %d: %s", res.ExitCode, strings.TrimSpace(tail(summarizeVerify(res.Output), 600)))
	return res.ExitCode, res.Output
}

// summarizeVerify mengambil baris laporan JSON verify (tanpa log JSON engine).
func summarizeVerify(out string) string {
	for _, line := range strings.Split(out, "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(line), &m) == nil {
			if _, ok := m["checked"]; ok {
				return line
			}
		}
	}
	return out
}

// checkLogs mencari pola bug di log semua service engine.
func (s *session) checkLogs(ctx context.Context) []string {
	var v []string
	for _, svc := range s.engineServices() {
		logs := s.dc.logs(ctx, svc)
		for _, p := range logPatterns {
			if i := strings.Index(logs, p.pattern); i >= 0 {
				line := logs[strings.LastIndex(logs[:i], "\n")+1:]
				if j := strings.Index(line, "\n"); j >= 0 {
					line = line[:j]
				}
				v = append(v, fmt.Sprintf("%s log contains %q (%s): %s", svc, p.pattern, p.meaning, tail(line, 400)))
			}
		}
	}
	return v
}

func querySet(ctx context.Context, conn *pgx.Conn, sql string, args ...any) (map[string]bool, error) {
	rows, err := conn.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	keys, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(keys))
	for _, k := range keys {
		set[k] = true
	}
	return set, nil
}

// diff mengembalikan key yang ada di want tetapi tidak di got, dan sebaliknya.
func diff(want map[string]string, got map[string]bool) (missing, extra []string) {
	for k := range want {
		if !got[k] {
			missing = append(missing, k)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			extra = append(extra, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return missing, extra
}

func sample(keys []string) string {
	if len(keys) > 5 {
		return strings.Join(keys[:5], ", ") + fmt.Sprintf(" (+%d more)", len(keys)-5)
	}
	return strings.Join(keys, ", ")
}

// dumpProjection mengambil seluruh baris proyeksi satu instance sebagai teks deterministik (sama
// dengan helper integration test recovery).
func dumpProjection(ctx context.Context, conn *pgx.Conn, key string) (string, error) {
	var out strings.Builder
	for _, q := range []string{
		`SELECT row_to_json(x)::text FROM (SELECT * FROM process_instance WHERE key = $1) x`,
		`SELECT row_to_json(x)::text FROM (SELECT * FROM element_instance WHERE process_instance_key = $1 ORDER BY key) x`,
		`SELECT row_to_json(x)::text FROM (SELECT * FROM variable WHERE process_instance_key = $1 ORDER BY key) x`,
		`SELECT row_to_json(x)::text FROM (SELECT * FROM job WHERE process_instance_key = $1 ORDER BY key) x`,
		`SELECT row_to_json(x)::text FROM (SELECT * FROM incident WHERE process_instance_key = $1 ORDER BY key) x`,
		`SELECT row_to_json(x)::text FROM (SELECT * FROM taken_sequence_flow WHERE process_instance_key = $1 ORDER BY gateway_id, sequence_flow_id) x`,
	} {
		rows, err := conn.Query(ctx, q, key)
		if err != nil {
			return "", err
		}
		lines, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return "", err
		}
		for _, l := range lines {
			out.WriteString(l + "\n")
		}
	}
	return out.String(), nil
}
