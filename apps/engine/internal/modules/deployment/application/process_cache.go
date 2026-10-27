package application

import (
	"bytes"
	"container/list"
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/domain"
	"github.com/indr622/workflow-engine/libs/go/bpmn"
)

// ProcessCache menyimpan model BPMN ter-parse per process definition key (LRU) dan
// mengimplementasikan api.ProcessDefinitions untuk modul lain.
type ProcessCache struct {
	repo     domain.Repository
	capacity int

	mu    sync.Mutex
	items map[int64]*list.Element
	order *list.List // depan = paling baru dipakai
}

type cacheEntry struct {
	key  int64
	proc *bpmn.Process
}

// NewProcessCache membuat cache dengan kapasitas tertentu (default 256).
func NewProcessCache(repo domain.Repository, capacity int) *ProcessCache {
	if capacity <= 0 {
		capacity = 256
	}
	return &ProcessCache{repo: repo, capacity: capacity, items: map[int64]*list.Element{}, order: list.New()}
}

// Put memasukkan model ke cache.
func (c *ProcessCache) Put(key int64, proc *bpmn.Process) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		el.Value.(*cacheEntry).proc = proc
		c.order.MoveToFront(el)
		return
	}
	c.items[key] = c.order.PushFront(&cacheEntry{key: key, proc: proc})
	for c.order.Len() > c.capacity {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.items, oldest.Value.(*cacheEntry).key)
	}
}

func (c *ProcessCache) get(key int64) (*bpmn.Process, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(el)
	return el.Value.(*cacheEntry).proc, true
}

// Len mengembalikan jumlah model di cache.
func (c *ProcessCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// GetByKey mengembalikan metadata dan model; parse dari XML tersimpan bila belum di cache.
func (c *ProcessCache) GetByKey(ctx context.Context, key int64) (api.ProcessDefinition, *bpmn.Process, error) {
	pd, content, err := c.repo.GetByKey(ctx, key)
	if err != nil {
		return api.ProcessDefinition{}, nil, mapNotFound(err)
	}
	proc, err := c.load(pd, content)
	return toAPI(pd), proc, err
}

// GetLatest mengembalikan versi terbaru proses beserta modelnya.
func (c *ProcessCache) GetLatest(ctx context.Context, bpmnProcessID string) (api.ProcessDefinition, *bpmn.Process, error) {
	pd, content, err := c.repo.GetLatest(ctx, domain.DefaultTenant, bpmnProcessID)
	if err != nil {
		return api.ProcessDefinition{}, nil, mapNotFound(err)
	}
	proc, err := c.load(pd, content)
	return toAPI(pd), proc, err
}

func (c *ProcessCache) load(pd domain.ProcessDefinition, content []byte) (*bpmn.Process, error) {
	if proc, ok := c.get(pd.Key); ok {
		return proc, nil
	}
	defs, err := bpmn.Parse(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("parse stored definition %d: %w", pd.Key, err)
	}
	proc, ok := defs.Process(pd.BpmnProcessID)
	if !ok {
		return nil, fmt.Errorf("stored definition %d has no process %q", pd.Key, pd.BpmnProcessID)
	}
	c.Put(pd.Key, proc)
	return proc, nil
}

func mapNotFound(err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return api.ErrNotFound
	}
	return err
}

func toAPI(pd domain.ProcessDefinition) api.ProcessDefinition {
	return api.ProcessDefinition{
		Key: pd.Key, TenantID: pd.TenantID, BpmnProcessID: pd.BpmnProcessID, Version: pd.Version,
		VersionTag: pd.VersionTag, Name: pd.Name, ResourceName: pd.ResourceName,
		DeploymentKey: pd.DeploymentKey, CreatedAt: pd.CreatedAt,
	}
}
