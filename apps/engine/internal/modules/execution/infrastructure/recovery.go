package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/application"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/domain"
)

// Operasi pemulihan (ADR-0016): execution_record adalah sumber kebenaran; tabel state adalah
// proyeksi yang bisa diverifikasi dan dibangun ulang.

const listRecordsSQL = `SELECT sequence, record_key::text, value_type, intent, payload, created_at
FROM execution_record WHERE process_instance_key = $1 AND sequence > $2 ORDER BY sequence`

func loadRecords(ctx context.Context, q interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}, key string, after int32) ([]domain.Record, error) {
	rows, err := q.Query(ctx, listRecordsSQL, key, after)
	if err != nil {
		return nil, fmt.Errorf("load records of %s: %w", key, err)
	}
	defer rows.Close()
	var out []domain.Record
	for rows.Next() {
		var (
			seq                         int32
			recordKey, valueType, inten string
			payload                     []byte
			at                          time.Time
		)
		if err := rows.Scan(&seq, &recordKey, &valueType, &inten, &payload, &at); err != nil {
			return nil, err
		}
		value, ok := domain.NewRecordValue(domain.ValueType(valueType))
		if !ok {
			return nil, fmt.Errorf("record %s/%d has unknown value type %q", key, seq, valueType)
		}
		if err := json.Unmarshal(payload, value); err != nil {
			return nil, fmt.Errorf("decode record %s/%d: %w", key, seq, err)
		}
		out = append(out, domain.Record{
			Key: recordKey, ProcessInstanceKey: key, ValueType: domain.ValueType(valueType),
			Intent: domain.Intent(inten), Timestamp: at, Value: value,
		})
	}
	return out, rows.Err()
}

// LoadRecords memenuhi application.TxScope.
func (t *txRepository) LoadRecords(ctx context.Context, key string) ([]domain.Record, error) {
	return loadRecords(ctx, t.tx, key, 0)
}

// ResetProjection menghapus proyeksi turunan instance (bukan baris process_instance, yang menjadi
// jangkar lock dan di-upsert ulang oleh record pertama).
func (t *txRepository) ResetProjection(ctx context.Context, key string) error {
	b := &pgx.Batch{}
	for _, table := range []string{"incident", "job", "variable", "taken_sequence_flow", "element_instance", "execution_snapshot"} {
		b.Queue("DELETE FROM "+table+" WHERE process_instance_key = $1", key)
	}
	if err := t.tx.SendBatch(ctx, b).Close(); err != nil {
		return fmt.Errorf("reset projection of %s: %w", key, err)
	}
	return nil
}

// Reproject memproyeksikan ulang semua record (tanpa menulis record) lalu nilai turunan state akhir.
func (t *txRepository) Reproject(ctx context.Context, records []domain.Record, st *domain.InstanceState) error {
	if len(records) == 0 {
		return nil
	}
	b := &pgx.Batch{}
	for i := range records {
		if err := project(b, &records[i]); err != nil {
			return err
		}
	}
	queueFinalize(b, st, int32(len(records)), records[len(records)-1].Timestamp) //nolint:gosec // jumlah record per instance < 2^31
	if err := t.tx.SendBatch(ctx, b).Close(); err != nil {
		return fmt.Errorf("reproject %s: %w", st.Key, err)
	}
	return nil
}

// ReadInstance membaca record dan state runtime proyeksi instance dari satu snapshot konsisten
// (REPEATABLE READ, read only, tanpa lock) untuk verifikasi dalam.
func (r *PostgresRepository) ReadInstance(ctx context.Context, key string) (application.InstanceReading, error) {
	var out application.InstanceReading
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	scope := &txRepository{tx: tx}
	snap, err := scope.load(ctx, key)
	if err != nil {
		return out, err
	}
	records, err := loadRecords(ctx, tx, key, 0)
	if err != nil {
		return out, err
	}
	return application.InstanceReading{Snapshot: snap, Records: records, LastSequence: scope.lastSequence}, tx.Commit(ctx)
}

// QuickVerify menjalankan pemeriksaan SQL agregat atas semua instance.
func (r *PostgresRepository) QuickVerify(ctx context.Context) ([]application.Mismatch, error) {
	checks := []struct{ kind, sql string }{
		{"sequence_gap", `SELECT pi.key::text, format('last_sequence=%s max=%s count=%s', pi.last_sequence, r.max_seq, r.n)
FROM process_instance pi
JOIN (SELECT process_instance_key, MAX(sequence) AS max_seq, COUNT(*) AS n FROM execution_record GROUP BY 1) r
  ON r.process_instance_key = pi.key
WHERE pi.last_sequence <> r.max_seq OR r.n <> r.max_seq`},
		{"instance_without_records", `SELECT pi.key::text, 'no execution records'
FROM process_instance pi
WHERE NOT EXISTS (SELECT 1 FROM execution_record r WHERE r.process_instance_key = pi.key)`},
		{"final_state", `SELECT pi.key::text, format('state=%s last process intent=%s', pi.state, last.intent)
FROM process_instance pi
JOIN LATERAL (SELECT r.intent FROM execution_record r
              WHERE r.process_instance_key = pi.key AND r.value_type = 'PROCESS_INSTANCE' AND r.record_key = pi.key
              ORDER BY r.sequence DESC LIMIT 1) last ON true
WHERE (pi.state = 'ACTIVE' AND last.intent IN ('ELEMENT_COMPLETED', 'ELEMENT_TERMINATED'))
   OR (pi.state = 'COMPLETED' AND last.intent <> 'ELEMENT_COMPLETED')
   OR (pi.state = 'TERMINATED' AND last.intent <> 'ELEMENT_TERMINATED')`},
		{"orphan_job", `SELECT j.process_instance_key::text, format('open job %s on element instance %s in state %s', j.key, j.element_instance_key, COALESCE(e.state, 'missing'))
FROM job j LEFT JOIN element_instance e ON e.key = j.element_instance_key
WHERE j.state = 'CREATED' AND (e.key IS NULL OR e.state <> 'ACTIVATED')`},
		{"has_incident", `SELECT pi.key::text, format('has_incident=%s open incidents=%s', pi.has_incident, COALESCE(i.n, 0))
FROM process_instance pi
LEFT JOIN (SELECT process_instance_key, COUNT(*) AS n FROM incident WHERE state = 'CREATED' GROUP BY 1) i
  ON i.process_instance_key = pi.key
WHERE pi.has_incident <> (COALESCE(i.n, 0) > 0)`},
	}
	var out []application.Mismatch
	for _, c := range checks {
		rows, err := r.pool.Query(ctx, c.sql)
		if err != nil {
			return nil, fmt.Errorf("verify %s: %w", c.kind, err)
		}
		for rows.Next() {
			m := application.Mismatch{Kind: c.kind}
			if err := rows.Scan(&m.Subject, &m.Detail); err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, m)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// DeepVerifyCandidates mengembalikan instance aktif yang berubah dalam jendela terakhir ditambah
// sampel acak instance aktif lain.
func (r *PostgresRepository) DeepVerifyCandidates(ctx context.Context, window time.Duration, sample int) ([]string, error) {
	rows, err := r.pool.Query(ctx, `(SELECT key::text FROM process_instance WHERE state = 'ACTIVE' AND updated_at >= now() - make_interval(secs => $1))
UNION
(SELECT key::text FROM process_instance WHERE state = 'ACTIVE' ORDER BY random() LIMIT $2)`, window.Seconds(), sample)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// InstanceKeys mengembalikan satu halaman key semua instance (urut naik) untuk verify/rebuild --all.
func (r *PostgresRepository) InstanceKeys(ctx context.Context, afterKey string, limit int) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT key::text FROM process_instance
WHERE NULLIF($1::text, '')::uuid IS NULL OR key > NULLIF($1::text, '')::uuid ORDER BY key LIMIT $2`, afterKey, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

var (
	_ application.RecoveryStore = (*PostgresRepository)(nil)
	_ application.RebuildScope  = (*txRepository)(nil)
)
