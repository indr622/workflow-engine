package application

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/domain"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/eventbus"
)

// Batas CreateProcessInstanceWithResult (bagian 6.3 rencana Fase 4).
const (
	DefaultResultTimeout = 10 * time.Second
	MaxResultTimeout     = 55 * time.Second
	// resultPollInterval menutup instance yang diselesaikan replika lain (event bus hanya in-process).
	resultPollInterval = 500 * time.Millisecond
)

// ResultReader membaca state akhir instance untuk CreateProcessInstanceWithResult.
type ResultReader interface {
	// InstanceResult mengembalikan state instance dan variabel scope process instance sebagai JSON
	// object; fetch kosong berarti semua variabel.
	InstanceResult(ctx context.Context, instanceKey string, fetch []string) (state string, variables []byte, err error)
}

// CreateProcessInstanceWithResult membuat instance lalu menunggu sampai COMPLETED atau TERMINATED.
func (c *Commands) CreateProcessInstanceWithResult(ctx context.Context, req api.CreateInstanceRequest, fetch []string,
	timeout time.Duration) (api.InstanceResult, error) {
	switch {
	case timeout <= 0:
		timeout = DefaultResultTimeout
	case timeout > MaxResultTimeout:
		timeout = MaxResultTimeout
	}
	created, err := c.CreateProcessInstance(ctx, req)
	if err != nil {
		return api.InstanceResult{}, err
	}
	key := created.ProcessInstanceKey
	// Daftar sebelum membaca state pertama agar penyelesaian di antara keduanya tidak terlewat.
	done, cancel := c.waiter.watch(key)
	defer cancel()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	poll := time.NewTicker(resultPollInterval)
	defer poll.Stop()
	for {
		state, vars, err := c.uow.InstanceResult(ctx, key, fetch)
		if err != nil {
			return api.InstanceResult{}, fmt.Errorf("read instance result: %w", err)
		}
		if state != string(domain.InstanceActive) {
			created.State = state
			return api.InstanceResult{CreateInstanceResult: created, Variables: vars}, nil
		}
		select {
		case <-ctx.Done():
			return api.InstanceResult{}, ctx.Err()
		case <-deadline.C:
			return api.InstanceResult{}, &api.ResultTimeoutError{ProcessInstanceKey: key}
		case <-done:
		case <-poll.C:
		}
	}
}

// waiter membangunkan CreateProcessInstanceWithResult saat instance yang ditunggu selesai. Event
// bus tidak punya Unsubscribe, jadi satu subscriber permanen membagi sinyal ke channel per key.
type waiter struct {
	mu      sync.Mutex
	waiting map[string]map[chan struct{}]struct{}
}

func newWaiter(bus eventbus.Bus) *waiter {
	w := &waiter{waiting: map[string]map[chan struct{}]struct{}{}}
	if bus == nil {
		return w
	}
	bus.Subscribe(api.TopicProcessInstanceCompleted, func(_ context.Context, ev eventbus.Event) error {
		if e, ok := ev.(api.ProcessInstanceCompleted); ok {
			w.notify(e.ProcessInstanceKey)
		}
		return nil
	})
	bus.Subscribe(api.TopicProcessInstanceTerminated, func(_ context.Context, ev eventbus.Event) error {
		if e, ok := ev.(api.ProcessInstanceTerminated); ok {
			w.notify(e.ProcessInstanceKey)
		}
		return nil
	})
	return w
}

func (w *waiter) watch(key string) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	w.mu.Lock()
	if w.waiting[key] == nil {
		w.waiting[key] = map[chan struct{}]struct{}{}
	}
	w.waiting[key][ch] = struct{}{}
	w.mu.Unlock()
	return ch, func() {
		w.mu.Lock()
		delete(w.waiting[key], ch)
		if len(w.waiting[key]) == 0 {
			delete(w.waiting, key)
		}
		w.mu.Unlock()
	}
}

func (w *waiter) notify(key string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for ch := range w.waiting[key] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
