// Package infrastructure berisi adapter keluar modul deployment: repository PostgreSQL.
package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/domain"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/infrastructure/db"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/keygen"
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

// WithTx menjalankan fn dalam transaksi pgx.
func (r *PostgresRepository) WithTx(ctx context.Context, fn func(ctx context.Context, tx domain.TxRepository) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(ctx, &txRepository{q: r.q.WithTx(tx), keys: keygen.NewPostgres(tx)}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// GetByKey mengembalikan definition dan konten resource asli.
func (r *PostgresRepository) GetByKey(ctx context.Context, key int64) (domain.ProcessDefinition, []byte, error) {
	row, err := r.q.GetProcessDefinitionByKey(ctx, key)
	if err != nil {
		return domain.ProcessDefinition{}, nil, mapErr(err)
	}
	pd, err := toDomain(row.Key, row.TenantID, row.BpmnProcessID, row.Version, row.VersionTag, row.Name,
		row.Checksum, row.DeploymentKey, row.CreatedAt, row.ResourceName)
	return pd, row.Content, err
}

// GetLatest mengembalikan versi terbaru proses dan konten resource.
func (r *PostgresRepository) GetLatest(ctx context.Context, tenantID, bpmnProcessID string) (domain.ProcessDefinition, []byte, error) {
	row, err := r.q.GetLatestProcessDefinitionWithContent(ctx, db.GetLatestProcessDefinitionWithContentParams{
		TenantID: tenantID, BpmnProcessID: bpmnProcessID,
	})
	if err != nil {
		return domain.ProcessDefinition{}, nil, mapErr(err)
	}
	pd, err := toDomain(row.Key, row.TenantID, row.BpmnProcessID, row.Version, row.VersionTag, row.Name,
		row.Checksum, row.DeploymentKey, row.CreatedAt, row.ResourceName)
	return pd, row.Content, err
}

// List mengembalikan halaman definition dengan keyset pagination.
func (r *PostgresRepository) List(ctx context.Context, q domain.ListQuery) ([]domain.ProcessDefinition, error) {
	var out []domain.ProcessDefinition
	if q.LatestOnly {
		rows, err := r.q.ListLatestProcessDefinitions(ctx, db.ListLatestProcessDefinitionsParams{
			TenantID: q.TenantID, BpmnProcessID: q.BpmnProcessID, AfterProcessID: q.AfterProcessID, PageLimit: q.Limit,
		})
		if err != nil {
			return nil, fmt.Errorf("list latest: %w", err)
		}
		for _, row := range rows {
			pd, err := toDomain(row.Key, row.TenantID, row.BpmnProcessID, row.Version, row.VersionTag, row.Name,
				row.Checksum, row.DeploymentKey, row.CreatedAt, row.ResourceName)
			if err != nil {
				return nil, err
			}
			out = append(out, pd)
		}
		return out, nil
	}
	rows, err := r.q.ListProcessDefinitions(ctx, db.ListProcessDefinitionsParams{
		TenantID: q.TenantID, BpmnProcessID: q.BpmnProcessID, AfterProcessID: q.AfterProcessID,
		AfterVersion: q.AfterVersion, PageLimit: q.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	for _, row := range rows {
		pd, err := toDomain(row.Key, row.TenantID, row.BpmnProcessID, row.Version, row.VersionTag, row.Name,
			row.Checksum, row.DeploymentKey, row.CreatedAt, row.ResourceName)
		if err != nil {
			return nil, err
		}
		out = append(out, pd)
	}
	return out, nil
}

type txRepository struct {
	q    *db.Queries
	keys *keygen.Postgres
}

func (t *txRepository) LockProcessID(ctx context.Context, tenantID, bpmnProcessID string) error {
	if err := t.q.AdvisoryLockProcessID(ctx, db.AdvisoryLockProcessIDParams{TenantID: tenantID, BpmnProcessID: bpmnProcessID}); err != nil {
		return fmt.Errorf("lock process %q: %w", bpmnProcessID, err)
	}
	return nil
}

func (t *txRepository) LatestByProcessID(ctx context.Context, tenantID, bpmnProcessID string) (domain.ProcessDefinition, bool, error) {
	row, err := t.q.GetLatestProcessDefinition(ctx, db.GetLatestProcessDefinitionParams{TenantID: tenantID, BpmnProcessID: bpmnProcessID})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ProcessDefinition{}, false, nil
	}
	if err != nil {
		return domain.ProcessDefinition{}, false, fmt.Errorf("latest %q: %w", bpmnProcessID, err)
	}
	pd, err := toDomain(row.Key, row.TenantID, row.BpmnProcessID, row.Version, row.VersionTag, row.Name,
		row.Checksum, row.DeploymentKey, row.CreatedAt, row.ResourceName)
	return pd, err == nil, err
}

func (t *txRepository) NextKeys(ctx context.Context, n int) ([]int64, error) {
	return t.keys.NextKeys(ctx, n)
}

func (t *txRepository) SaveDeployment(ctx context.Context, d domain.Deployment) error {
	if err := t.q.InsertDeployment(ctx, db.InsertDeploymentParams{Key: d.Key, TenantID: d.TenantID}); err != nil {
		return fmt.Errorf("insert deployment: %w", err)
	}
	resourceIDs := make(map[string]int64, len(d.Resources))
	for _, r := range d.Resources {
		id, err := t.q.InsertResource(ctx, db.InsertResourceParams{
			DeploymentKey: d.Key, ResourceName: r.Name, Content: r.Content, Checksum: r.Checksum.Bytes(),
		})
		if err != nil {
			return fmt.Errorf("insert resource %q: %w", r.Name, err)
		}
		resourceIDs[r.Name] = id
	}
	for _, pd := range d.Definitions {
		resourceID, ok := resourceIDs[pd.ResourceName]
		if !ok {
			return fmt.Errorf("definition %q references unknown resource %q", pd.BpmnProcessID, pd.ResourceName)
		}
		if err := t.q.InsertProcessDefinition(ctx, db.InsertProcessDefinitionParams{
			Key: pd.Key, TenantID: pd.TenantID, BpmnProcessID: pd.BpmnProcessID, Version: pd.Version,
			VersionTag: pd.VersionTag, Name: pd.Name, ResourceID: resourceID, Checksum: pd.Checksum.Bytes(),
			DeploymentKey: pd.DeploymentKey,
		}); err != nil {
			return fmt.Errorf("insert process definition %q v%d: %w", pd.BpmnProcessID, pd.Version, err)
		}
	}
	return nil
}

func toDomain(key int64, tenantID, processID string, version int32, versionTag, name string, checksum []byte,
	deploymentKey int64, createdAt pgtype.Timestamptz, resourceName string) (domain.ProcessDefinition, error) {
	c, err := domain.ChecksumFromBytes(checksum)
	if err != nil {
		return domain.ProcessDefinition{}, fmt.Errorf("definition %d: %w", key, err)
	}
	return domain.ProcessDefinition{
		Key: key, TenantID: tenantID, BpmnProcessID: processID, Version: version, VersionTag: versionTag,
		Name: name, ResourceName: resourceName, Checksum: c, DeploymentKey: deploymentKey, CreatedAt: createdAt.Time,
	}, nil
}

func mapErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}
