// Package infrastructure berisi adapter keluar modul execution: repository PostgreSQL yang menulis
// record append-only beserta proyeksinya, model baca, dan generator key.
//
// Satu command memakai sedikit round trip: kunci + muat state dalam satu query (agregasi JSON),
// lalu semua proyeksi dan record dalam satu pgx.Batch. Query baca memakai sqlc.
package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/application"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/domain"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/infrastructure/db"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/faultpoint"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/idempotency"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/keygen"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/outbox"
	"github.com/indr622/workflow-engine/libs/go/bpmn"
)

// PostgresRepository mengimplementasikan application.UnitOfWork di atas pgx dan sqlc.
type PostgresRepository struct {
	pool             *pgxpool.Pool
	q                *db.Queries
	outbox           *outbox.Writer
	snapshotInterval int32
	idemTTL          time.Duration
}

// WithIdempotencyTTL mengubah masa berlaku klaim request_id.
func (r *PostgresRepository) WithIdempotencyTTL(ttl time.Duration) *PostgresRepository {
	r.idemTTL = ttl
	return r
}

// DefaultSnapshotInterval adalah jumlah record antar snapshot instance aktif.
const DefaultSnapshotInterval = 5000

// NewPostgresRepository membuat repository untuk pool yang diberikan.
func NewPostgresRepository(pool *pgxpool.Pool, writer *outbox.Writer) *PostgresRepository {
	return &PostgresRepository{pool: pool, q: db.New(pool), outbox: writer, snapshotInterval: DefaultSnapshotInterval}
}

// WithSnapshotInterval mengubah interval snapshot (record); 0 atau negatif mematikan snapshot.
func (r *PostgresRepository) WithSnapshotInterval(n int) *PostgresRepository {
	r.snapshotInterval = int32(min(max(n, 0), 1<<30)) //nolint:gosec // dibatasi di atas
	return r
}

var _ application.UnitOfWork = (*PostgresRepository)(nil)

// Do menjalankan fn dalam satu transaksi. Event outbox yang belum ikut batch AppendAndProject
// dikirim sebelum commit.
func (r *PostgresRepository) Do(ctx context.Context, fn func(ctx context.Context, tx application.TxScope) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	scope := &txRepository{tx: tx, outbox: r.outbox, snapshotInterval: r.snapshotInterval, idemTTL: r.idemTTL}
	if err := fn(ctx, scope); err != nil {
		return err
	}
	if len(scope.stores) > 0 {
		b := &pgx.Batch{}
		scope.queueStores(b)
		if err := tx.SendBatch(ctx, b).Close(); err != nil {
			return fmt.Errorf("store idempotent response: %w", err)
		}
	}
	if len(scope.pending) > 0 {
		events := make([]outbox.Event, 0, len(scope.pending))
		for _, p := range scope.pending {
			events = append(events, p.Event)
		}
		if err := r.outbox.Enqueue(ctx, tx, events...); err != nil {
			return err
		}
	}
	if err := faultpoint.Hit(ctx, faultpoint.CommandBeforeCommit); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// JobRef memenuhi domain.Repository.
func (r *PostgresRepository) JobRef(ctx context.Context, jobKey string) (string, string, error) {
	row, err := r.q.GetJobRef(ctx, jobKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", domain.ErrJobNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("job %s: %w", jobKey, err)
	}
	return row.ProcessInstanceKey, row.ProcessDefinitionKey, nil
}

// DefinitionKeyOf memenuhi domain.Repository.
func (r *PostgresRepository) DefinitionKeyOf(ctx context.Context, instanceKey string) (string, error) {
	pi, err := r.q.GetProcessInstanceRow(ctx, instanceKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrInstanceNotFound
	}
	if err != nil {
		return "", fmt.Errorf("instance %s: %w", instanceKey, err)
	}
	return pi.ProcessDefinitionKey, nil
}

type txRepository struct {
	tx               pgx.Tx
	outbox           *outbox.Writer
	snapshotInterval int32
	// lastSequence adalah sequence record terakhir instance yang dimuat (0 untuk instance baru).
	lastSequence int32
	pending      []application.RecordEvent
	stores       []pendingStore
	idemTTL      time.Duration
}

type pendingStore struct {
	claim    idempotency.Claim
	response []byte
}

// BeginIdempotent memenuhi application.TxScope.
func (t *txRepository) BeginIdempotent(ctx context.Context, c idempotency.Claim) (idempotency.Result, error) {
	return idempotency.Begin(ctx, t.tx, c, t.idemTTL)
}

// StoreIdempotent memenuhi application.TxScope; dikirim bersama batch AppendAndProject atau saat commit.
func (t *txRepository) StoreIdempotent(c idempotency.Claim, response []byte) {
	t.stores = append(t.stores, pendingStore{claim: c, response: response})
}

func (t *txRepository) queueStores(b *pgx.Batch) {
	for _, s := range t.stores {
		idempotency.QueueStore(b, s.claim, s.response)
	}
	t.stores = nil
}

// EnqueueForRecords memenuhi application.TxScope.
func (t *txRepository) EnqueueForRecords(events []application.RecordEvent) {
	t.pending = append(t.pending, events...)
}

// lockSQL mengunci baris instance. Kunci dan muat sengaja dua statement: di READ COMMITTED
// setiap statement memakai snapshot baru, sehingga statement muat yang berjalan SETELAH kunci
// didapat melihat semua perubahan writer sebelumnya. Satu statement gabungan (SELECT ... FOR
// UPDATE dengan subquery) membaca subquery dari snapshot sebelum menunggu kunci dan menghasilkan
// state basi. Keduanya dikirim dalam satu pgx.Batch sehingga tetap satu round trip.
const lockSQL = `SELECT key FROM process_instance WHERE key = $1 FOR UPDATE`

// loadSQL mengumpulkan seluruh state runtime instance dalam satu statement (agregasi JSON).
const loadSQL = `
SELECT pi.process_definition_key::text, pi.bpmn_process_id, pi.process_definition_version, pi.state,
  COALESCE((SELECT max(r.sequence) FROM execution_record r WHERE r.process_instance_key = pi.key), 0)::int,
  COALESCE((SELECT json_agg(json_build_object(
      'key', e.key, 'flowScopeKey', e.flow_scope_key, 'elementId', e.element_id,
      'bpmnElementType', e.bpmn_element_type, 'state', e.state, 'incidentKey', e.incident_key) ORDER BY e.key)
    FROM element_instance e
    WHERE e.process_instance_key = pi.key AND e.state NOT IN ('COMPLETED', 'TERMINATED')), '[]'),
  COALESCE((SELECT json_agg(json_build_object(
      'flowScopeKey', f.flow_scope_key, 'gatewayId', f.gateway_id, 'sequenceFlowId', f.sequence_flow_id, 'count', f.count))
    FROM taken_sequence_flow f WHERE f.process_instance_key = pi.key), '[]'),
  COALESCE((SELECT json_agg(json_build_object(
      'key', v.key, 'scopeKey', v.scope_key, 'name', v.name, 'value', v.value))
    FROM variable v WHERE v.process_instance_key = pi.key), '[]'),
  COALESCE((SELECT json_agg(json_build_object(
      'key', j.key, 'elementInstanceKey', j.element_instance_key, 'elementId', j.element_id,
      'type', j.type, 'retries', j.retries, 'customHeaders', j.custom_headers, 'state', j.state) ORDER BY j.key)
    FROM job j WHERE j.process_instance_key = pi.key AND j.state IN ('CREATED', 'ACTIVATED', 'FAILED', 'ERROR_THROWN')), '[]'),
  COALESCE((SELECT json_agg(json_build_object(
      'key', i.key, 'elementInstanceKey', i.element_instance_key, 'elementId', i.element_id,
      'errorType', i.error_type, 'errorMessage', i.error_message, 'jobKey', i.job_key) ORDER BY i.key)
    FROM incident i WHERE i.process_instance_key = pi.key AND i.state = 'CREATED'), '[]')
FROM process_instance pi
WHERE pi.key = $1`

type loadedElement struct {
	Key             string  `json:"key"`
	FlowScopeKey    *string `json:"flowScopeKey"`
	ElementID       string  `json:"elementId"`
	BpmnElementType string  `json:"bpmnElementType"`
	State           string  `json:"state"`
	IncidentKey     *string `json:"incidentKey"`
}

type loadedFlow struct {
	FlowScopeKey   string `json:"flowScopeKey"`
	GatewayID      string `json:"gatewayId"`
	SequenceFlowID string `json:"sequenceFlowId"`
	Count          int32  `json:"count"`
}

type loadedVariable struct {
	Key      string `json:"key"`
	ScopeKey string `json:"scopeKey"`
	Name     string `json:"name"`
	Value    string `json:"value"`
}

type loadedJob struct {
	Key                string        `json:"key"`
	ElementInstanceKey string        `json:"elementInstanceKey"`
	ElementID          string        `json:"elementId"`
	Type               string        `json:"type"`
	Retries            int32         `json:"retries"`
	CustomHeaders      []bpmn.Header `json:"customHeaders"`
	State              string        `json:"state"`
}

type loadedIncident struct {
	Key                string  `json:"key"`
	ElementInstanceKey string  `json:"elementInstanceKey"`
	ElementID          string  `json:"elementId"`
	ErrorType          string  `json:"errorType"`
	ErrorMessage       string  `json:"errorMessage"`
	JobKey             *string `json:"jobKey"`
}

func (t *txRepository) LockAndLoad(ctx context.Context, key string) (*domain.InstanceSnapshot, error) {
	b := &pgx.Batch{}
	b.Queue(lockSQL, key)
	b.Queue(loadSQL, key)
	br := t.tx.SendBatch(ctx, b)
	var locked string
	err := br.QueryRow().Scan(&locked)
	var snap *domain.InstanceSnapshot
	if err == nil {
		snap, err = t.decodeLoaded(key, br.QueryRow())
	}
	if cerr := br.Close(); err == nil {
		err = cerr
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrInstanceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock and load instance %s: %w", key, err)
	}
	return snap, nil
}

// load memuat state runtime tanpa mengunci (untuk verifikasi dalam di transaksi read only).
func (t *txRepository) load(ctx context.Context, key string) (*domain.InstanceSnapshot, error) {
	snap, err := t.decodeLoaded(key, t.tx.QueryRow(ctx, loadSQL, key))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrInstanceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load instance %s: %w", key, err)
	}
	return snap, nil
}

func (t *txRepository) decodeLoaded(key string, row pgx.Row) (*domain.InstanceSnapshot, error) {
	var (
		defKey, processID, state                          string
		version, lastSeq                                  int32
		elementsJSON, flowsJSON, varsJSON, jobsJSON, incJ []byte
	)
	if err := row.Scan(&defKey, &processID, &version, &state, &lastSeq, &elementsJSON, &flowsJSON, &varsJSON, &jobsJSON, &incJ); err != nil {
		return nil, err
	}
	t.lastSequence = lastSeq
	snap := &domain.InstanceSnapshot{
		Key:        key,
		Definition: domain.DefinitionRef{Key: defKey, BpmnProcessID: processID, Version: version},
		Status:     domain.InstanceStatus(state),
	}
	var (
		elements  []loadedElement
		flows     []loadedFlow
		vars      []loadedVariable
		jobs      []loadedJob
		incidents []loadedIncident
	)
	for _, part := range []struct {
		raw []byte
		dst any
	}{{elementsJSON, &elements}, {flowsJSON, &flows}, {varsJSON, &vars}, {jobsJSON, &jobs}, {incJ, &incidents}} {
		if err := json.Unmarshal(part.raw, part.dst); err != nil {
			return nil, fmt.Errorf("decode runtime state of %s: %w", key, err)
		}
	}
	for _, e := range elements {
		snap.Elements = append(snap.Elements, domain.ElementInstance{
			Key: e.Key, ProcessInstanceKey: key, FlowScopeKey: deref(e.FlowScopeKey), ElementID: e.ElementID,
			BpmnElementType: e.BpmnElementType, State: domain.ElementState(e.State), IncidentKey: deref(e.IncidentKey),
		})
	}
	for _, f := range flows {
		snap.TakenFlows = append(snap.TakenFlows, domain.TakenFlow{
			JoinKey: domain.JoinKey{FlowScopeKey: f.FlowScopeKey, GatewayID: f.GatewayID, SequenceFlowID: f.SequenceFlowID},
			Count:   f.Count,
		})
	}
	for _, v := range vars {
		snap.Variables = append(snap.Variables, domain.Variable{Key: v.Key, ScopeKey: v.ScopeKey, Name: v.Name, Value: json.RawMessage(v.Value)})
	}
	for _, j := range jobs {
		// Lease (ACTIVATED) tidak tercatat di record (ADR-0020): bagi Processor job itu CREATED.
		state := domain.JobState(j.State)
		if state == domain.JobActivated {
			state = domain.JobCreated
		}
		snap.Jobs = append(snap.Jobs, domain.Job{
			Key: j.Key, ElementInstanceKey: j.ElementInstanceKey, ElementID: j.ElementID, Type: j.Type,
			Retries: j.Retries, CustomHeaders: j.CustomHeaders, State: state,
		})
	}
	for _, inc := range incidents {
		snap.Incidents = append(snap.Incidents, domain.Incident{
			Key: inc.Key, ElementInstanceKey: inc.ElementInstanceKey, ElementID: inc.ElementID,
			Type: domain.IncidentType(inc.ErrorType), Message: inc.ErrorMessage, JobKey: deref(inc.JobKey),
		})
	}
	return snap, nil
}

// Statement proyeksi. Ditulis tangan (bukan sqlc) karena dikirim bersama dalam satu pgx.Batch.
const (
	sqlInsertProcessInstance = `INSERT INTO process_instance
    (key, process_definition_key, bpmn_process_id, process_definition_version, state, start_time)
VALUES ($1, $2, $3, $4, 'ACTIVE', $5)
ON CONFLICT (key) DO UPDATE SET
    process_definition_key = EXCLUDED.process_definition_key, bpmn_process_id = EXCLUDED.bpmn_process_id,
    process_definition_version = EXCLUDED.process_definition_version, state = 'ACTIVE',
    start_time = EXCLUDED.start_time, end_time = NULL, has_incident = false`
	sqlFinishProcessInstance = `UPDATE process_instance SET state = $2, end_time = $3 WHERE key = $1`
	sqlInsertElement         = `INSERT INTO element_instance
    (key, process_instance_key, flow_scope_key, element_id, bpmn_element_type, state, start_time)
VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (key) DO NOTHING`
	sqlUpdateElement      = `UPDATE element_instance SET state = $2, end_time = $3 WHERE key = $1`
	sqlSetElementIncident = `UPDATE element_instance SET incident_key = $2 WHERE key = $1`
	sqlInsertVariable     = `INSERT INTO variable (key, process_instance_key, scope_key, name, value, updated_at)
VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT DO NOTHING`
	sqlUpdateVariable = `UPDATE variable SET value = $2, updated_at = $3 WHERE key = $1`
	sqlInsertJob      = `INSERT INTO job
    (key, process_instance_key, element_instance_key, element_id, bpmn_process_id, process_definition_key,
     process_definition_version, type, retries, custom_headers, state, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'CREATED', $11, $11) ON CONFLICT (key) DO NOTHING`
	// Setiap transisi job dari record membersihkan lease (worker, deadline).
	sqlFinishJob = `UPDATE job SET state = $2, updated_at = $3, worker = NULL, deadline = NULL WHERE key = $1`
	sqlFailJob   = `UPDATE job SET state = $2, retries = $3, error_message = $4, activatable_at = $5,
    updated_at = $6, worker = NULL, deadline = NULL WHERE key = $1`
	sqlThrowJobError = `UPDATE job SET state = 'ERROR_THROWN', error_code = $2, error_message = $3,
    updated_at = $4, worker = NULL, deadline = NULL WHERE key = $1`
	sqlUpdateJobRetries = `UPDATE job SET retries = $2, updated_at = $3 WHERE key = $1`
	// sqlRecurJob mengembalikan job ke antrean saat incident-nya diselesaikan; syaratnya sama dengan
	// domain.JobMadeActivatableOnResolve.
	sqlRecurJob = `UPDATE job SET state = 'CREATED', activatable_at = NULL, updated_at = $2
WHERE key = $1 AND (state = 'ERROR_THROWN' OR (state = 'FAILED' AND retries > 0))`
	sqlInsertIncident = `INSERT INTO incident
    (key, process_instance_key, element_instance_key, element_id, error_type, error_message, job_key, state, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, 'CREATED', $8) ON CONFLICT (key) DO NOTHING`
	sqlResolveIncident = `UPDATE incident SET state = 'RESOLVED', resolved_at = $2 WHERE key = $1`
	sqlDeleteFlows     = `DELETE FROM taken_sequence_flow WHERE process_instance_key = $1`
	sqlInsertFlows     = `INSERT INTO taken_sequence_flow (process_instance_key, flow_scope_key, gateway_id, sequence_flow_id, count)
SELECT $1, unnest($2::uuid[]), unnest($3::text[]), unnest($4::text[]), unnest($5::int[])`
	// sqlTouchInstance menulis penanda proyeksi dari record terakhir (deterministik untuk rebuild):
	// version = last_sequence, updated_at = waktu record terakhir.
	sqlTouchInstance = `UPDATE process_instance
SET last_sequence = $2::int, version = $2::int, updated_at = $3,
    has_incident = EXISTS (SELECT 1 FROM incident i WHERE i.process_instance_key = $1 AND i.state = 'CREATED')
WHERE key = $1`
	sqlUpsertSnapshot = `INSERT INTO execution_snapshot (process_instance_key, sequence, format_version, state)
VALUES ($1, $2, $3, $4::jsonb)
ON CONFLICT (process_instance_key) DO UPDATE SET sequence = EXCLUDED.sequence,
    format_version = EXCLUDED.format_version, state = EXCLUDED.state, created_at = now()`
	sqlInsertRecords = `INSERT INTO execution_record
    (process_instance_key, sequence, record_key, value_type, intent, element_id, payload, created_at)
SELECT $1, unnest($2::int[]), unnest($3::uuid[]), unnest($4::text[]), unnest($5::text[]),
       unnest($6::text[]), unnest($7::json[]), unnest($8::timestamptz[])`
)

// AppendAndProject menulis proyeksi per record, counter join dari state akhir, lalu record ke log,
// semuanya dalam satu pgx.Batch.
func (t *txRepository) AppendAndProject(ctx context.Context, records []domain.Record, st *domain.InstanceState) error {
	if len(records) == 0 {
		return nil
	}
	b := &pgx.Batch{}
	for i := range records {
		if err := project(b, &records[i]); err != nil {
			return err
		}
	}
	last := records[len(records)-1]
	queueFinalize(b, st, t.lastSequence+int32(len(records)), last.Timestamp) //nolint:gosec // dibatasi Limits

	n := len(records)
	seqs, keys, types, intents := make([]int32, n), make([]string, n), make([]string, n), make([]string, n)
	elementIDs, payloads, times := make([]*string, n), make([]string, n), make([]time.Time, n)
	for i := range records {
		r := &records[i]
		payload, err := json.Marshal(r.Value)
		if err != nil {
			return fmt.Errorf("marshal record: %w", err)
		}
		seqs[i] = t.lastSequence + int32(i) + 1 //nolint:gosec // record per command dibatasi Limits
		keys[i], types[i], intents[i], payloads[i], times[i] = r.Key, string(r.ValueType), string(r.Intent), string(payload), r.Timestamp
		if id := r.ElementID(); id != "" {
			elementIDs[i] = &id
		}
	}
	b.Queue(sqlInsertRecords, st.Key, seqs, keys, types, intents, elementIDs, payloads, times)

	t.queueStores(b)

	// Snapshot state runtime instance aktif setiap kali last_sequence melewati kelipatan interval.
	newLast := t.lastSequence + int32(n) //nolint:gosec // dibatasi Limits
	if t.snapshotInterval > 0 && st.Status == domain.InstanceActive && newLast/t.snapshotInterval > t.lastSequence/t.snapshotInterval {
		if err := faultpoint.Hit(ctx, faultpoint.SnapshotBeforeWrite); err != nil {
			return err
		}
		data, err := json.Marshal(st.Snapshot(newLast))
		if err != nil {
			return fmt.Errorf("encode snapshot: %w", err)
		}
		b.Queue(sqlUpsertSnapshot, st.Key, newLast, domain.SnapshotFormatVersion, string(data))
	}

	// Event outbox ikut batch yang sama; aggregate_seq = sequence record pemicu.
	if len(t.pending) > 0 {
		events := make([]outbox.Event, 0, len(t.pending))
		for _, p := range t.pending {
			ev := p.Event
			ev.AggregateSeq = t.lastSequence + int32(p.RecordIndex) + 1 //nolint:gosec // dibatasi Limits
			events = append(events, ev)
		}
		if err := t.outbox.Queue(ctx, b, events...); err != nil {
			return err
		}
		t.pending = nil
	}

	if err := t.tx.SendBatch(ctx, b).Close(); err != nil {
		return fmt.Errorf("append and project: %w", err)
	}
	t.lastSequence += int32(n) //nolint:gosec // record per command dibatasi Limits
	return nil
}

// queueFinalize mengantrekan nilai turunan state akhir: counter join dan penanda proyeksi.
func queueFinalize(b *pgx.Batch, st *domain.InstanceState, lastSequence int32, lastAt time.Time) {
	b.Queue(sqlDeleteFlows, st.Key)
	if flows := st.SortedTakenFlows(); len(flows) > 0 {
		scopes, gateways, ids, counts := make([]string, len(flows)), make([]string, len(flows)), make([]string, len(flows)), make([]int32, len(flows))
		for i, f := range flows {
			scopes[i], gateways[i], ids[i], counts[i] = f.FlowScopeKey, f.GatewayID, f.SequenceFlowID, f.Count
		}
		b.Queue(sqlInsertFlows, st.Key, scopes, gateways, ids, counts)
	}
	b.Queue(sqlTouchInstance, st.Key, lastSequence, lastAt)
}

// project mengantrekan statement proyeksi untuk satu record. Fungsi ini juga dipakai rebuild
// proyeksi di Fase 3.
func project(b *pgx.Batch, r *domain.Record) error {
	ts := r.Timestamp
	switch v := r.Value.(type) {
	case *domain.ProcessInstanceValue:
		projectElement(b, r, v)
	case *domain.VariableValue:
		if r.Intent == domain.IntentCreated {
			b.Queue(sqlInsertVariable, r.Key, r.ProcessInstanceKey, v.ScopeKey, v.Name, string(v.Value), ts)
		} else {
			b.Queue(sqlUpdateVariable, r.Key, string(v.Value), ts)
		}
	case *domain.JobValue:
		return projectJob(b, r, v)
	case *domain.IncidentValue:
		if r.Intent == domain.IntentCreated {
			b.Queue(sqlInsertIncident, r.Key, r.ProcessInstanceKey, v.ElementInstanceKey, v.ElementID,
				string(v.ErrorType), v.ErrorMessage, optString(v.JobKey), ts)
			b.Queue(sqlSetElementIncident, v.ElementInstanceKey, r.Key)
		} else {
			b.Queue(sqlResolveIncident, r.Key, ts)
			b.Queue(sqlSetElementIncident, v.ElementInstanceKey, nil)
			if v.JobKey != "" {
				b.Queue(sqlRecurJob, v.JobKey, ts)
			}
		}
	}
	return nil
}

func projectJob(b *pgx.Batch, r *domain.Record, v *domain.JobValue) error {
	ts := r.Timestamp
	switch r.Intent {
	case domain.IntentCreated:
		headers, err := json.Marshal(nonNilHeaders(v.CustomHeaders))
		if err != nil {
			return err
		}
		b.Queue(sqlInsertJob, r.Key, r.ProcessInstanceKey, v.ElementInstanceKey, v.ElementID, v.BpmnProcessID,
			v.ProcessDefinitionKey, v.Version, v.Type, v.Retries, headers, ts)
	case domain.IntentCompleted:
		b.Queue(sqlFinishJob, r.Key, string(domain.JobCompleted), ts)
	case domain.IntentCanceled:
		b.Queue(sqlFinishJob, r.Key, string(domain.JobCanceled), ts)
	case domain.IntentFailed:
		state := domain.JobCreated
		if v.Retries <= 0 {
			state = domain.JobFailed
		}
		// activatable_at diturunkan dari timestamp record agar rebuild menghasilkan nilai yang sama.
		var activatable *time.Time
		if v.Retries > 0 && v.RetryBackoffMs > 0 {
			at := ts.Add(time.Duration(v.RetryBackoffMs) * time.Millisecond)
			activatable = &at
		}
		b.Queue(sqlFailJob, r.Key, string(state), v.Retries, optString(v.ErrorMessage), activatable, ts)
	case domain.IntentErrorThrown:
		b.Queue(sqlThrowJobError, r.Key, v.ErrorCode, optString(v.ErrorMessage), ts)
	case domain.IntentRetriesUpdated:
		b.Queue(sqlUpdateJobRetries, r.Key, v.Retries, ts)
	default:
		return fmt.Errorf("%w: unknown job intent %s", domain.ErrInvalidRecord, r.Intent)
	}
	return nil
}

func optString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func projectElement(b *pgx.Batch, r *domain.Record, v *domain.ProcessInstanceValue) {
	if r.Intent == domain.IntentSequenceFlowTaken {
		return
	}
	state, _ := domain.ElementStateOf(r.Intent)
	isProcess := v.BpmnElementType == domain.ElementTypeProcess
	if state == domain.ElementActivating {
		if isProcess {
			b.Queue(sqlInsertProcessInstance, r.Key, v.ProcessDefinitionKey, v.BpmnProcessID, v.Version, r.Timestamp)
		}
		var scope *string
		if v.FlowScopeKey != "" {
			s := v.FlowScopeKey
			scope = &s
		}
		b.Queue(sqlInsertElement, r.Key, r.ProcessInstanceKey, scope, v.ElementID, v.BpmnElementType, string(state), r.Timestamp)
		return
	}
	var end *time.Time
	if state.IsFinal() {
		t := r.Timestamp
		end = &t
	}
	b.Queue(sqlUpdateElement, r.Key, string(state), end)
	if isProcess && state.IsFinal() {
		status := domain.InstanceCompleted
		if state == domain.ElementTerminated {
			status = domain.InstanceTerminated
		}
		b.Queue(sqlFinishProcessInstance, r.Key, string(status), end)
	}
}

func nonNilHeaders(h []bpmn.Header) []bpmn.Header {
	if h == nil {
		return []bpmn.Header{}
	}
	return h
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func optTime(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// KeyGenerator mengadaptasi generator UUIDv7 platform ke port domain.
type KeyGenerator struct {
	g *keygen.UUIDv7
}

// NewKeyGenerator membuat adapter.
func NewKeyGenerator(g *keygen.UUIDv7) *KeyGenerator { return &KeyGenerator{g: g} }

// Next memenuhi domain.KeyGenerator. Error crypto/rand dilaporkan sebagai key kosong, yang
// ditolak Processor sehingga command dibatalkan tanpa menulis apa pun.
func (k *KeyGenerator) Next() string {
	key, err := k.g.NewKey()
	if err != nil {
		return ""
	}
	return key
}
