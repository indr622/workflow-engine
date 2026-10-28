//go:build integration

package infrastructure_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/application"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/recovery"
)

// dumpProjection mengembalikan semua baris proyeksi satu instance sebagai JSON terurut.
func dumpProjection(t *testing.T, pool *pgxpool.Pool, key string) string {
	t.Helper()
	var out strings.Builder
	for _, q := range []string{
		`SELECT row_to_json(x)::text FROM (SELECT * FROM process_instance WHERE key = $1) x`,
		`SELECT row_to_json(x)::text FROM (SELECT * FROM element_instance WHERE process_instance_key = $1 ORDER BY key) x`,
		`SELECT row_to_json(x)::text FROM (SELECT * FROM variable WHERE process_instance_key = $1 ORDER BY key) x`,
		`SELECT row_to_json(x)::text FROM (SELECT * FROM job WHERE process_instance_key = $1 ORDER BY key) x`,
		`SELECT row_to_json(x)::text FROM (SELECT * FROM incident WHERE process_instance_key = $1 ORDER BY key) x`,
		`SELECT row_to_json(x)::text FROM (SELECT * FROM taken_sequence_flow WHERE process_instance_key = $1 ORDER BY gateway_id, sequence_flow_id) x`,
	} {
		rows, err := pool.Query(context.Background(), q, key)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				t.Fatal(err)
			}
			out.WriteString(line + "\n")
		}
		rows.Close()
	}
	return out.String()
}

func newRecovery(f *fixture) *application.Recovery {
	return application.NewRecovery(f.repo, f.cache, slog.New(slog.NewTextHandler(io.Discard, nil)), application.RecoveryConfig{})
}

// recoveryInstances membuat instance dengan bentuk berbeda: selesai, aktif sebagian (fork),
// incident, dan dibatalkan.
func recoveryInstances(t *testing.T, f *fixture) []string {
	t.Helper()
	ctx := context.Background()
	completeSome := func(key string, limit int) {
		for i := 0; i < limit; i++ {
			jobs, _, err := f.queries.ListJobs(ctx, application.JobFilter{ProcessInstanceKey: key, OnlyOpen: true, Limit: 1})
			if err != nil {
				t.Fatal(err)
			}
			if len(jobs) == 0 {
				return
			}
			if err := f.commands.CompleteJob(ctx, jobs[0].Key, []byte(fmt.Sprintf(`{"step": %d}`, i))); err != nil {
				t.Fatal(err)
			}
		}
	}
	create := func(processID, vars string) string {
		res, err := f.commands.CreateProcessInstance(ctx, api.CreateInstanceRequest{BpmnProcessID: processID, Variables: []byte(vars)})
		if err != nil {
			t.Fatal(err)
		}
		return res.ProcessInstanceKey
	}
	linear := f.deployXML(t, "rec-linear", linearXML)
	fork := f.deployXML(t, "rec-fork", forkXML(5))
	incident := f.deployXML(t, "rec-incident", incidentXML)

	done := create(linear, `{"a": 1}`)
	completeSome(done, 5)
	partial := create(fork, `{"x": [1, 2]}`)
	completeSome(partial, 3)
	broken := create(incident, `{"amount": 1}`)
	canceled := create(fork, `{}`)
	completeSome(canceled, 2)
	if err := f.commands.CancelProcessInstance(ctx, canceled); err != nil {
		t.Fatal(err)
	}
	return []string{done, partial, broken, canceled}
}

// TestRebuildRestoresIdenticalProjection: rebuild dari record menghasilkan baris proyeksi identik.
func TestRebuildRestoresIdenticalProjection(t *testing.T) {
	f := setup(t)
	rec := newRecovery(f)
	ctx := context.Background()
	for _, key := range recoveryInstances(t, f) {
		before := dumpProjection(t, f.pool, key)
		if m, err := rec.DeepVerify(ctx, key); err != nil || len(m) != 0 {
			t.Fatalf("instance %s does not verify before rebuild: %v %+v", key, err, m)
		}
		res, err := rec.RebuildInstance(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if res.Records == 0 {
			t.Fatalf("rebuild of %s used no records", key)
		}
		if after := dumpProjection(t, f.pool, key); after != before {
			t.Fatalf("projection of %s differs after rebuild\nbefore:\n%s\nafter:\n%s", key, before, after)
		}
	}
}

// TestVerifyDetectsCorruptionAndRunnerModes: proyeksi yang dirusak terdeteksi; mode fail menolak
// start, mode warn hanya mencatat, mode rebuild memperbaiki.
func TestVerifyDetectsCorruptionAndRunnerModes(t *testing.T) {
	f := setup(t)
	rec := newRecovery(f)
	ctx := context.Background()
	keys := recoveryInstances(t, f)
	partial, broken := keys[1], keys[2]
	before := map[string]string{partial: dumpProjection(t, f.pool, partial), broken: dumpProjection(t, f.pool, broken)}

	for _, q := range []string{
		`DELETE FROM element_instance WHERE key = (SELECT key FROM element_instance WHERE process_instance_key = $1 AND state = 'ACTIVATED' AND bpmn_element_type = 'SERVICE_TASK' ORDER BY key LIMIT 1)`,
		`UPDATE variable SET value = '"tampered"' WHERE process_instance_key = $1`,
	} {
		if _, err := f.pool.Exec(ctx, q, partial); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.pool.Exec(ctx, `UPDATE process_instance SET has_incident = false WHERE key = $1`, broken); err != nil {
		t.Fatal(err)
	}

	deep, err := rec.DeepVerify(ctx, partial)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, m := range deep {
		kinds[m.Kind] = true
	}
	if !kinds["element_instances"] || !kinds["variables"] {
		t.Fatalf("deep verify missed corruption: %+v", deep)
	}
	quick, err := rec.QuickVerifyFor(ctx, keys)
	if err != nil {
		t.Fatal(err)
	}
	foundIncident, foundOrphan := false, false
	for _, m := range quick {
		if m.Subject == broken && m.Kind == "has_incident" {
			foundIncident = true
		}
		if m.Subject == partial && m.Kind == "orphan_job" {
			foundOrphan = true
		}
	}
	if !foundIncident || !foundOrphan {
		t.Fatalf("quick verify missed corruption (has_incident=%v orphan_job=%v): %+v", foundIncident, foundOrphan, quick)
	}

	// Verifier yang hanya melihat dua instance ini, agar data test lain di database tidak ikut.
	scoped := scopedVerifier{rec: rec, keys: []string{partial, broken}}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	run := func(mode recovery.OnMismatch) error {
		_, err := (&recovery.Runner{Verifiers: []recovery.Verifier{scoped}, OnMismatch: mode, Log: quiet}).Run(ctx)
		return err
	}
	if err := run(recovery.OnMismatchFail); !errors.Is(err, recovery.ErrMismatch) {
		t.Fatalf("fail mode error = %v, want ErrMismatch", err)
	}
	if err := run(recovery.OnMismatchWarn); err != nil {
		t.Fatalf("warn mode error = %v", err)
	}
	if err := run(recovery.OnMismatchRebuild); err != nil {
		t.Fatalf("rebuild mode error = %v", err)
	}
	for key, dump := range before {
		if after := dumpProjection(t, f.pool, key); after != dump {
			t.Fatalf("instance %s not restored by rebuild mode\nbefore:\n%s\nafter:\n%s", key, dump, after)
		}
	}
}

// scopedVerifier membatasi verifikasi ke instance tertentu.
type scopedVerifier struct {
	rec  *application.Recovery
	keys []string
}

func (s scopedVerifier) Name() string { return "execution-scoped" }

func (s scopedVerifier) VerifyOnStart(ctx context.Context) (recovery.Report, error) {
	rep := recovery.Report{Checked: len(s.keys)}
	quick, err := s.rec.QuickVerifyFor(ctx, s.keys)
	if err != nil {
		return rep, err
	}
	rep.Mismatches = append(rep.Mismatches, quick...)
	for _, k := range s.keys {
		m, err := s.rec.DeepVerify(ctx, k)
		if err != nil {
			return rep, err
		}
		rep.Mismatches = append(rep.Mismatches, m...)
	}
	return rep, nil
}

func (s scopedVerifier) Repair(ctx context.Context, subjects []string) error {
	return s.rec.Repair(ctx, subjects)
}
