package infrastructure

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/application"
)

var _ application.HistoryReader = (*PostgresRepository)(nil)

// ListRecords memenuhi application.HistoryReader (keyset atas sequence).
func (r *PostgresRepository) ListRecords(ctx context.Context, key string, afterSequence, limit int32) ([]application.RecordView, error) {
	rows, err := r.pool.Query(ctx, `SELECT sequence, record_key::text, value_type, intent, COALESCE(element_id, ''), payload::text, created_at
FROM execution_record WHERE process_instance_key = $1 AND sequence > $2 ORDER BY sequence LIMIT $3`, key, afterSequence, limit)
	if err != nil {
		return nil, fmt.Errorf("list records of %s: %w", key, err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (application.RecordView, error) {
		var v application.RecordView
		var payload string
		err := row.Scan(&v.Sequence, &v.RecordKey, &v.ValueType, &v.Intent, &v.ElementID, &payload, &v.Timestamp)
		v.Payload = []byte(payload)
		return v, err
	})
}

// ListElementInstances memenuhi application.HistoryReader (keyset atas key).
func (r *PostgresRepository) ListElementInstances(ctx context.Context, key string, onlyActive bool, afterKey string, limit int32) ([]application.ElementInstanceView, error) {
	rows, err := r.pool.Query(ctx, `SELECT key::text, COALESCE(flow_scope_key::text, ''), element_id, bpmn_element_type, state,
       COALESCE(incident_key::text, ''), start_time, end_time
FROM element_instance
WHERE process_instance_key = $1
  AND (NOT $2::bool OR state NOT IN ('COMPLETED', 'TERMINATED'))
  AND (NULLIF($3::text, '')::uuid IS NULL OR key > NULLIF($3::text, '')::uuid)
ORDER BY key LIMIT $4`, key, onlyActive, afterKey, limit)
	if err != nil {
		return nil, fmt.Errorf("list element instances of %s: %w", key, err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (application.ElementInstanceView, error) {
		var v application.ElementInstanceView
		var end *time.Time
		err := row.Scan(&v.Key, &v.FlowScopeKey, &v.ElementID, &v.BpmnElementType, &v.State, &v.IncidentKey, &v.StartTime, &end)
		v.EndTime = end
		return v, err
	})
}

// InstanceExists memenuhi application.HistoryReader.
func (r *PostgresRepository) InstanceExists(ctx context.Context, key string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM process_instance WHERE key = $1)`, key).Scan(&ok)
	return ok, err
}
