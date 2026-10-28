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
	items map[string]*list.Element
	order *list.List // depan = paling baru dipakai
}

type cacheEntry struct {
	key  string
	proc *bpmn.Process
	// pd terisi setelah model pertama kali dimuat lewat repository; Put dari deploy hanya
	// membawa model, metadata dilengkapi saat GetByKey pertama.
	pd    domain.ProcessDefinition
	hasPD bool
}

// NewProcessCache membuat cache dengan kapasitas tertentu (default 256).
func NewProcessCache(repo domain.Repository, capacity int) *ProcessCache {
	if capacity <= 0 {
		capacity = 256
	}
	return &ProcessCache{repo: repo, capacity: capacity, items: map[string]*list.Element{}, order: list.New()}
}

// Put memasukkan model ke cache.
func (c *ProcessCache) Put(key string, proc *bpmn.Process) {
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

func (c *ProcessCache) get(key string) (*bpmn.Process, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(el)
	return el.Value.(*cacheEntry).proc, true
}

// getFull mengembalikan metadata dan model bila keduanya sudah di cache.
func (c *ProcessCache) getFull(key string) (domain.ProcessDefinition, *bpmn.Process, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok || !el.Value.(*cacheEntry).hasPD {
		return domain.ProcessDefinition{}, nil, false
	}
	c.order.MoveToFront(el)
	e := el.Value.(*cacheEntry)
	return e.pd, e.proc, true
}

func (c *ProcessCache) remember(pd domain.ProcessDefinition) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[pd.Key]; ok {
		e := el.Value.(*cacheEntry)
		e.pd, e.hasPD = pd, true
	}
}

// Len mengembalikan jumlah model di cache.
func (c *ProcessCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// GetByKey mengembalikan metadata dan model; parse dari XML tersimpan bila belum di cache.
// Saat cache hit (metadata dan model), tidak ada akses database: aman dipanggil dari dalam
// transaksi pemanggil tanpa meminta koneksi kedua dari pool.
func (c *ProcessCache) GetByKey(ctx context.Context, key string) (api.ProcessDefinition, *bpmn.Process, error) {
	if pd, proc, ok := c.getFull(key); ok {
		return toAPI(pd), proc, nil
	}
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

// GetByVersion mengembalikan versi tertentu proses beserta modelnya.
func (c *ProcessCache) GetByVersion(ctx context.Context, bpmnProcessID string, version int32) (api.ProcessDefinition, *bpmn.Process, error) {
	pd, content, err := c.repo.GetByVersion(ctx, domain.DefaultTenant, bpmnProcessID, version)
	if err != nil {
		return api.ProcessDefinition{}, nil, mapNotFound(err)
	}
	proc, err := c.load(pd, content)
	return toAPI(pd), proc, err
}

func (c *ProcessCache) load(pd domain.ProcessDefinition, content []byte) (*bpmn.Process, error) {
	if proc, ok := c.get(pd.Key); ok {
		c.remember(pd)
		return proc, nil
	}
	defs, err := bpmn.Parse(bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("parse stored definition %s: %w", pd.Key, err)
	}
	proc, ok := defs.Process(pd.BpmnProcessID)
	if !ok {
		return nil, fmt.Errorf("stored definition %s has no process %q", pd.Key, pd.BpmnProcessID)
	}
	c.Put(pd.Key, proc)
	c.remember(pd)
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
