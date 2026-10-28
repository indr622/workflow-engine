package infrastructure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/application"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/domain"
	"github.com/indr622/workflow-engine/libs/go/bpmn"
)

// Lease job (ADR-0020). Aktivasi tidak mengunci baris process_instance: FOR UPDATE SKIP LOCKED
// melewati job yang sedang dipegang command instance (urutan lock command = process_instance →
// job; aktivasi = job saja tanpa menunggu), sehingga tidak ada siklus lock. Deadline memakai now()
// Postgres sebagai jam tunggal semua replika.

var _ application.LeaseStore = (*PostgresRepository)(nil)

// sqlActivateJobs memilih job FIFO per tipe menurut (activatable_at, key); NULL berarti segera.
const sqlActivateJobs = `
WITH picked AS (
    SELECT key FROM job
    WHERE type = $1 AND state = 'CREATED' AND (activatable_at IS NULL OR activatable_at <= now())
    ORDER BY activatable_at NULLS FIRST, key
    LIMIT $2
    FOR UPDATE SKIP LOCKED
)
UPDATE job j
SET state = 'ACTIVATED', worker = $3, deadline = now() + make_interval(secs => $4),
    activated_at = now(), activation_count = j.activation_count + 1
FROM picked
WHERE j.key = picked.key
RETURNING j.key::text, j.type, j.process_instance_key::text, j.element_instance_key::text, j.element_id,
          j.bpmn_process_id, j.process_definition_key::text, j.process_definition_version, j.retries,
          j.custom_headers, j.deadline`

// sqlActivatedVariables membaca variabel scope process instance untuk job yang diaktifkan.
const sqlActivatedVariables = `
SELECT process_instance_key::text, name, value
FROM variable
WHERE process_instance_key = ANY($1::uuid[]) AND scope_key = process_instance_key
  AND (cardinality($2::text[]) = 0 OR name = ANY($2::text[]))
ORDER BY process_instance_key, name`

// ActivateJobs memenuhi application.LeaseStore.
func (r *PostgresRepository) ActivateJobs(ctx context.Context, q api.ActivationQuery) ([]api.ActivatedJob, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, sqlActivateJobs, q.Type, q.MaxJobs, q.Worker, q.Timeout.Seconds())
	if err != nil {
		return nil, fmt.Errorf("activate %s: %w", q.Type, err)
	}
	var jobs []api.ActivatedJob
	for rows.Next() {
		var (
			j       api.ActivatedJob
			headers []byte
		)
		if err := rows.Scan(&j.Key, &j.Type, &j.ProcessInstanceKey, &j.ElementInstanceKey, &j.ElementID, &j.BpmnProcessID,
			&j.ProcessDefinitionKey, &j.ProcessDefinitionVersion, &j.Retries, &headers, &j.Deadline); err != nil {
			rows.Close()
			return nil, err
		}
		var hs []bpmn.Header
		if err := json.Unmarshal(headers, &hs); err != nil {
			rows.Close()
			return nil, fmt.Errorf("job %s headers: %w", j.Key, err)
		}
		for _, h := range hs {
			j.CustomHeaders = append(j.CustomHeaders, api.Header{Key: h.Key, Value: h.Value})
		}
		j.Worker = q.Worker
		jobs = append(jobs, j)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("activate %s: %w", q.Type, err)
	}
	if len(jobs) == 0 {
		return nil, tx.Commit(ctx)
	}
	vars, err := activatedVariables(ctx, tx, jobs, q.FetchVariables)
	if err != nil {
		return nil, err
	}
	for i := range jobs {
		jobs[i].Variables = vars[jobs[i].ProcessInstanceKey]
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit activation: %w", err)
	}
	return jobs, nil
}

// activatedVariables mengembalikan dokumen JSON object variabel per process instance; instance
// tanpa variabel mendapat "{}".
func activatedVariables(ctx context.Context, tx pgx.Tx, jobs []api.ActivatedJob, fetch []string) (map[string][]byte, error) {
	seen := map[string]bool{}
	var keys []string
	for _, j := range jobs {
		if !seen[j.ProcessInstanceKey] {
			seen[j.ProcessInstanceKey] = true
			keys = append(keys, j.ProcessInstanceKey)
		}
	}
	if fetch == nil {
		fetch = []string{}
	}
	rows, err := tx.Query(ctx, sqlActivatedVariables, keys, fetch)
	if err != nil {
		return nil, fmt.Errorf("activated job variables: %w", err)
	}
	defer rows.Close()
	byInstance := map[string]map[string]json.RawMessage{}
	for rows.Next() {
		var pi, name, value string
		if err := rows.Scan(&pi, &name, &value); err != nil {
			return nil, err
		}
		if byInstance[pi] == nil {
			byInstance[pi] = map[string]json.RawMessage{}
		}
		byInstance[pi][name] = json.RawMessage(value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(keys))
	for _, k := range keys {
		doc, err := json.Marshal(nonNilVars(byInstance[k]))
		if err != nil {
			return nil, err
		}
		out[k] = doc
	}
	return out, nil
}

func nonNilVars(m map[string]json.RawMessage) map[string]json.RawMessage {
	if m == nil {
		return map[string]json.RawMessage{}
	}
	return m
}

// ExtendLease memenuhi application.LeaseStore.
func (r *PostgresRepository) ExtendLease(ctx context.Context, jobKey string, timeout time.Duration) error {
	tag, err := r.pool.Exec(ctx, `UPDATE job SET deadline = now() + make_interval(secs => $2)
WHERE key = $1 AND state = 'ACTIVATED'`, jobKey, timeout.Seconds())
	if err != nil {
		return fmt.Errorf("extend lease of %s: %w", jobKey, err)
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	var state string
	err = r.pool.QueryRow(ctx, `SELECT state FROM job WHERE key = $1`, jobKey).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrJobNotFound
	}
	if err != nil {
		return fmt.Errorf("job %s: %w", jobKey, err)
	}
	return fmt.Errorf("%w: job is %s", domain.ErrJobNotActivated, state)
}

// sqlReleaseExpiredLeases mengembalikan lease yang lewat deadline-nya; aman di banyak replika.
const sqlReleaseExpiredLeases = `
WITH expired AS (
    SELECT key FROM job
    WHERE state = 'ACTIVATED' AND deadline < now()
    ORDER BY deadline
    LIMIT $1
    FOR UPDATE SKIP LOCKED
)
UPDATE job j
SET state = 'CREATED', worker = NULL, deadline = NULL, timeout_count = j.timeout_count + 1
FROM expired
WHERE j.key = expired.key
RETURNING j.key::text, j.type`

// ReleaseExpiredLeases memenuhi application.LeaseStore.
func (r *PostgresRepository) ReleaseExpiredLeases(ctx context.Context, limit int) ([]api.ReleasedJob, error) {
	rows, err := r.pool.Query(ctx, sqlReleaseExpiredLeases, limit)
	if err != nil {
		return nil, fmt.Errorf("release expired leases: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (api.ReleasedJob, error) {
		var j api.ReleasedJob
		return j, row.Scan(&j.Key, &j.Type)
	})
	if err != nil {
		return nil, fmt.Errorf("release expired leases: %w", err)
	}
	return out, nil
}

// IncidentRef memenuhi application.UnitOfWork.
func (r *PostgresRepository) IncidentRef(ctx context.Context, incidentKey string) (string, string, error) {
	row, err := r.q.GetIncidentRef(ctx, incidentKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", domain.ErrIncidentNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("incident %s: %w", incidentKey, err)
	}
	return row.ProcessInstanceKey, row.ProcessDefinitionKey, nil
}

// ScopeRef memenuhi application.UnitOfWork. Element instance PROCESS memakai key process instance,
// sehingga satu tabel cukup untuk kedua jenis scope.
func (r *PostgresRepository) ScopeRef(ctx context.Context, scopeKey string) (string, string, error) {
	var piKey, defKey string
	err := r.pool.QueryRow(ctx, `SELECT e.process_instance_key::text, pi.process_definition_key::text
FROM element_instance e JOIN process_instance pi ON pi.key = e.process_instance_key
WHERE e.key = $1`, scopeKey).Scan(&piKey, &defKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", domain.ErrScopeNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("scope %s: %w", scopeKey, err)
	}
	return piKey, defKey, nil
}

// InstanceResult memenuhi application.ResultReader.
func (r *PostgresRepository) InstanceResult(ctx context.Context, instanceKey string, fetch []string) (string, []byte, error) {
	if fetch == nil {
		fetch = []string{}
	}
	var state string
	var doc []byte
	err := r.pool.QueryRow(ctx, `SELECT pi.state,
  COALESCE((SELECT json_object_agg(v.name, v.value::json ORDER BY v.name) FROM variable v
            WHERE v.process_instance_key = pi.key AND v.scope_key = pi.key
              AND (cardinality($2::text[]) = 0 OR v.name = ANY($2::text[]))), '{}'::json)
FROM process_instance pi WHERE pi.key = $1`, instanceKey, fetch).Scan(&state, &doc)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, domain.ErrInstanceNotFound
	}
	if err != nil {
		return "", nil, fmt.Errorf("instance result %s: %w", instanceKey, err)
	}
	// json_object_agg menyisipkan spasi; ringkaskan agar sama dengan dokumen variabel lain.
	var buf bytes.Buffer
	if err := json.Compact(&buf, doc); err != nil {
		return "", nil, fmt.Errorf("instance result %s: %w", instanceKey, err)
	}
	return state, buf.Bytes(), nil
}
