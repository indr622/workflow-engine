package application

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/domain"
)

// fakeRepository adalah repository in-memory untuk unit test use case.
type fakeRepository struct {
	mu          sync.Mutex
	nextKey     int64
	definitions []domain.ProcessDefinition
	content     map[string][]byte
	deployments int
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{nextKey: 1000, content: map[string][]byte{}}
}

func (r *fakeRepository) WithTx(ctx context.Context, fn func(ctx context.Context, tx domain.TxRepository) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	nextKey, deployments := r.nextKey, r.deployments
	snapshotDefs := append([]domain.ProcessDefinition(nil), r.definitions...)
	if err := fn(ctx, (*fakeTx)(r)); err != nil {
		// rollback
		r.nextKey, r.deployments, r.definitions = nextKey, deployments, snapshotDefs
		return err
	}
	return nil
}

func (r *fakeRepository) GetByKey(_ context.Context, key string) (domain.ProcessDefinition, []byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range r.definitions {
		if d.Key == key {
			return d, r.content[key], nil
		}
	}
	return domain.ProcessDefinition{}, nil, domain.ErrNotFound
}

func (r *fakeRepository) GetLatest(_ context.Context, tenantID, processID string) (domain.ProcessDefinition, []byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pd, found := r.latest(tenantID, processID)
	if !found {
		return domain.ProcessDefinition{}, nil, domain.ErrNotFound
	}
	return pd, r.content[pd.Key], nil
}

func (r *fakeRepository) List(_ context.Context, q domain.ListQuery) ([]domain.ProcessDefinition, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := append([]domain.ProcessDefinition(nil), r.definitions...)
	sort.Slice(items, func(a, b int) bool {
		if items[a].BpmnProcessID != items[b].BpmnProcessID {
			return items[a].BpmnProcessID < items[b].BpmnProcessID
		}
		return items[a].Version > items[b].Version
	})
	var out []domain.ProcessDefinition
	seen := map[string]bool{}
	for _, d := range items {
		if q.BpmnProcessID != "" && d.BpmnProcessID != q.BpmnProcessID {
			continue
		}
		if q.LatestOnly && seen[d.BpmnProcessID] {
			continue
		}
		if q.AfterProcessID != "" && (d.BpmnProcessID < q.AfterProcessID ||
			(d.BpmnProcessID == q.AfterProcessID && (q.LatestOnly || d.Version >= q.AfterVersion))) {
			continue
		}
		seen[d.BpmnProcessID] = true
		out = append(out, d)
		if int32(len(out)) == q.Limit {
			break
		}
	}
	return out, nil
}

func (r *fakeRepository) latest(tenantID, processID string) (domain.ProcessDefinition, bool) {
	var best domain.ProcessDefinition
	found := false
	for _, d := range r.definitions {
		if d.TenantID == tenantID && d.BpmnProcessID == processID && (!found || d.Version > best.Version) {
			best, found = d, true
		}
	}
	return best, found
}

type fakeTx fakeRepository

func (t *fakeTx) LockProcessID(context.Context, string, string) error { return nil }

func (t *fakeTx) LatestByProcessID(_ context.Context, tenantID, processID string) (domain.ProcessDefinition, bool, error) {
	pd, found := (*fakeRepository)(t).latest(tenantID, processID)
	return pd, found, nil
}

func (t *fakeTx) NextKeys(_ context.Context, n int) ([]string, error) {
	keys := make([]string, n)
	for i := range keys {
		t.nextKey++
		keys[i] = fmt.Sprintf("00000000-0000-7000-8000-%012d", t.nextKey)
	}
	return keys, nil
}

func (t *fakeTx) SaveDeployment(_ context.Context, d domain.Deployment) error {
	t.deployments++
	byName := map[string][]byte{}
	for _, r := range d.Resources {
		byName[r.Name] = r.Content
	}
	for _, pd := range d.Definitions {
		t.definitions = append(t.definitions, pd)
		t.content[pd.Key] = byName[pd.ResourceName]
	}
	return nil
}
