// Package infrastructure berisi adapter keluar modul execution: repository PostgreSQL yang menulis
// record append-only beserta proyeksinya, model baca, dan generator key.
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

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/domain"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/infrastructure/db"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/keygen"
	"github.com/indr622/workflow-engine/libs/go/bpmn"
)

// PostgresRepository mengimplementasikan domain.Repository di atas pgx dan sqlc.
type PostgresRepository struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewPostgresRepository membuat repository untuk pool yang diberikan.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool, q: db.New(pool)}
}

var _ domain.Repository = (*PostgresRepository)(nil)

// WithTx menjalankan fn dalam satu transaksi.
func (r *PostgresRepository) WithTx(ctx context.Context, fn func(ctx context.Context, tx domain.TxRepository) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(ctx, &txRepository{q: r.q.WithTx(tx)}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
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
	q *db.Queries
}

func (t *txRepository) LockInstance(ctx context.Context, key string) error {
	if _, err := t.q.LockProcessInstance(ctx, key); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrInstanceNotFound
		}
		return fmt.Errorf("lock instance %s: %w", key, err)
	}
	return nil
}

// InstanceKeyOfJob memenuhi domain.Repository.
func (r *PostgresRepository) InstanceKeyOfJob(ctx context.Context, jobKey string) (string, error) {
	key, err := r.q.GetJobInstanceKey(ctx, jobKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrJobNotFound
	}
	if err != nil {
		return "", fmt.Errorf("job %s: %w", jobKey, err)
	}
	return key, nil
}

func (t *txRepository) LoadRuntimeState(ctx context.Context, key string) (*domain.InstanceSnapshot, error) {
	pi, err := t.q.GetProcessInstanceRow(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrInstanceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load instance %s: %w", key, err)
	}
	snap := &domain.InstanceSnapshot{
		Key:        pi.Key,
		Definition: domain.DefinitionRef{Key: pi.ProcessDefinitionKey, BpmnProcessID: pi.BpmnProcessID, Version: pi.ProcessDefinitionVersion},
		Status:     domain.InstanceStatus(pi.State),
	}
	elements, err := t.q.ListRuntimeElementInstances(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("load elements: %w", err)
	}
	for _, e := range elements {
		snap.Elements = append(snap.Elements, domain.ElementInstance{
			Key: e.Key, ProcessInstanceKey: e.ProcessInstanceKey, FlowScopeKey: deref(e.FlowScopeKey),
			ElementID: e.ElementID, BpmnElementType: e.BpmnElementType, State: domain.ElementState(e.State),
			IncidentKey: deref(e.IncidentKey),
		})
	}
	flows, err := t.q.ListTakenFlows(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("load taken flows: %w", err)
	}
	for _, f := range flows {
		snap.TakenFlows = append(snap.TakenFlows, domain.TakenFlow{
			JoinKey: domain.JoinKey{FlowScopeKey: f.FlowScopeKey, GatewayID: f.GatewayID, SequenceFlowID: f.SequenceFlowID},
			Count:   f.Count,
		})
	}
	vars, err := t.q.ListVariables(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("load variables: %w", err)
	}
	for _, v := range vars {
		snap.Variables = append(snap.Variables, domain.Variable{Key: v.Key, ScopeKey: v.ScopeKey, Name: v.Name, Value: json.RawMessage(v.Value)})
	}
	jobs, err := t.q.ListOpenJobs(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("load jobs: %w", err)
	}
	for _, j := range jobs {
		var headers []bpmn.Header
		if err := json.Unmarshal(j.CustomHeaders, &headers); err != nil {
			return nil, fmt.Errorf("job %s headers: %w", j.Key, err)
		}
		snap.Jobs = append(snap.Jobs, domain.Job{
			Key: j.Key, ElementInstanceKey: j.ElementInstanceKey, ElementID: j.ElementID, Type: j.Type,
			Retries: j.Retries, CustomHeaders: headers, State: domain.JobState(j.State),
		})
	}
	incidents, err := t.q.ListOpenIncidents(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("load incidents: %w", err)
	}
	for _, inc := range incidents {
		snap.Incidents = append(snap.Incidents, domain.Incident{
			Key: inc.Key, ElementInstanceKey: inc.ElementInstanceKey, ElementID: inc.ElementID,
			Type: domain.IncidentType(inc.ErrorType), Message: inc.ErrorMessage,
		})
	}
	return snap, nil
}

// AppendAndProject menulis proyeksi per record, counter join dari state akhir, lalu record ke log.
func (t *txRepository) AppendAndProject(ctx context.Context, records []domain.Record, st *domain.InstanceState) error {
	if len(records) == 0 {
		return nil
	}
	for i := range records {
		if err := project(ctx, t.q, &records[i]); err != nil {
			return err
		}
	}
	if err := t.q.DeleteTakenFlows(ctx, st.Key); err != nil {
		return fmt.Errorf("clear taken flows: %w", err)
	}
	for _, f := range st.SortedTakenFlows() {
		if err := t.q.InsertTakenFlow(ctx, db.InsertTakenFlowParams{
			ProcessInstanceKey: st.Key, FlowScopeKey: f.FlowScopeKey, GatewayID: f.GatewayID,
			SequenceFlowID: f.SequenceFlowID, Count: f.Count,
		}); err != nil {
			return fmt.Errorf("insert taken flow: %w", err)
		}
	}
	if err := t.q.TouchProcessInstance(ctx, st.Key); err != nil {
		return fmt.Errorf("touch instance: %w", err)
	}
	last, err := t.q.MaxRecordSequence(ctx, st.Key)
	if err != nil {
		return fmt.Errorf("max sequence: %w", err)
	}
	rows := make([]db.InsertRecordsParams, 0, len(records))
	for i := range records {
		r := &records[i]
		payload, err := json.Marshal(r.Value)
		if err != nil {
			return fmt.Errorf("marshal record: %w", err)
		}
		var elementID *string
		if id := r.ElementID(); id != "" {
			elementID = &id
		}
		rows = append(rows, db.InsertRecordsParams{
			ProcessInstanceKey: r.ProcessInstanceKey, Sequence: last + int32(i) + 1, //nolint:gosec // record per command dibatasi Limits
			RecordKey: r.Key, ValueType: string(r.ValueType), Intent: string(r.Intent), ElementID: elementID,
			Payload: payload, CreatedAt: ts(r.Timestamp),
		})
	}
	if _, err := t.q.InsertRecords(ctx, rows); err != nil {
		return fmt.Errorf("append records: %w", err)
	}
	return nil
}

// project memetakan satu record ke tabel state. Fungsi ini juga dipakai rebuild proyeksi di Fase 3.
func project(ctx context.Context, q *db.Queries, r *domain.Record) error {
	var err error
	switch v := r.Value.(type) {
	case *domain.ProcessInstanceValue:
		err = projectElement(ctx, q, r, v)
	case *domain.VariableValue:
		if r.Intent == domain.IntentCreated {
			err = q.InsertVariable(ctx, db.InsertVariableParams{
				Key: r.Key, ProcessInstanceKey: r.ProcessInstanceKey, ScopeKey: v.ScopeKey, Name: v.Name,
				Value: string(v.Value), UpdatedAt: ts(r.Timestamp),
			})
		} else {
			err = q.UpdateVariable(ctx, db.UpdateVariableParams{Key: r.Key, Value: string(v.Value), UpdatedAt: ts(r.Timestamp)})
		}
	case *domain.JobValue:
		if r.Intent == domain.IntentCreated {
			headers, merr := json.Marshal(nonNilHeaders(v.CustomHeaders))
			if merr != nil {
				return merr
			}
			err = q.InsertJob(ctx, db.InsertJobParams{
				Key: r.Key, ProcessInstanceKey: r.ProcessInstanceKey, ElementInstanceKey: v.ElementInstanceKey,
				ElementID: v.ElementID, BpmnProcessID: v.BpmnProcessID, ProcessDefinitionKey: v.ProcessDefinitionKey,
				ProcessDefinitionVersion: v.Version, Type: v.Type, Retries: v.Retries, CustomHeaders: headers,
				CreatedAt: ts(r.Timestamp),
			})
		} else {
			state := domain.JobCompleted
			if r.Intent == domain.IntentCanceled {
				state = domain.JobCanceled
			}
			err = q.UpdateJobState(ctx, db.UpdateJobStateParams{Key: r.Key, State: string(state), UpdatedAt: ts(r.Timestamp)})
		}
	case *domain.IncidentValue:
		if r.Intent == domain.IntentCreated {
			err = q.InsertIncident(ctx, db.InsertIncidentParams{
				Key: r.Key, ProcessInstanceKey: r.ProcessInstanceKey, ElementInstanceKey: v.ElementInstanceKey,
				ElementID: v.ElementID, ErrorType: string(v.ErrorType), ErrorMessage: v.ErrorMessage, CreatedAt: ts(r.Timestamp),
			})
			if err == nil {
				key := r.Key
				err = q.SetElementInstanceIncident(ctx, db.SetElementInstanceIncidentParams{Key: v.ElementInstanceKey, IncidentKey: &key})
			}
		} else {
			err = q.ResolveIncident(ctx, db.ResolveIncidentParams{Key: r.Key, ResolvedAt: ts(r.Timestamp)})
			if err == nil {
				err = q.SetElementInstanceIncident(ctx, db.SetElementInstanceIncidentParams{Key: v.ElementInstanceKey})
			}
		}
	}
	if err != nil {
		return fmt.Errorf("project %s %s %s: %w", r.ValueType, r.Intent, r.Key, err)
	}
	return nil
}

func projectElement(ctx context.Context, q *db.Queries, r *domain.Record, v *domain.ProcessInstanceValue) error {
	if r.Intent == domain.IntentSequenceFlowTaken {
		return nil
	}
	state, _ := domain.ElementStateOf(r.Intent)
	isProcess := v.BpmnElementType == domain.ElementTypeProcess
	if state == domain.ElementActivating {
		if isProcess {
			if err := q.InsertProcessInstance(ctx, db.InsertProcessInstanceParams{
				Key: r.Key, ProcessDefinitionKey: v.ProcessDefinitionKey, BpmnProcessID: v.BpmnProcessID,
				ProcessDefinitionVersion: v.Version, StartTime: ts(r.Timestamp),
			}); err != nil {
				return err
			}
		}
		var scope *string
		if v.FlowScopeKey != "" {
			s := v.FlowScopeKey
			scope = &s
		}
		return q.InsertElementInstance(ctx, db.InsertElementInstanceParams{
			Key: r.Key, ProcessInstanceKey: r.ProcessInstanceKey, FlowScopeKey: scope, ElementID: v.ElementID,
			BpmnElementType: v.BpmnElementType, State: string(state), StartTime: ts(r.Timestamp),
		})
	}
	var end pgtype.Timestamptz
	if state.IsFinal() {
		end = ts(r.Timestamp)
	}
	if err := q.UpdateElementInstanceState(ctx, db.UpdateElementInstanceStateParams{Key: r.Key, State: string(state), EndTime: end}); err != nil {
		return err
	}
	if isProcess && state.IsFinal() {
		status := domain.InstanceCompleted
		if state == domain.ElementTerminated {
			status = domain.InstanceTerminated
		}
		return q.FinishProcessInstance(ctx, db.FinishProcessInstanceParams{Key: r.Key, State: string(status), EndTime: end})
	}
	return nil
}

func nonNilHeaders(h []bpmn.Header) []bpmn.Header {
	if h == nil {
		return []bpmn.Header{}
	}
	return h
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
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
