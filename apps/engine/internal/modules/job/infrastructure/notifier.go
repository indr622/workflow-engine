// Package infrastructure berisi adapter modul job: notifier in-process yang dibangunkan event
// execution.job_created.
package infrastructure

import (
	"context"
	"sync"

	executionapi "github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/job/application"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/eventbus"
)

// Notifier membagi sinyal per tipe job ke long polling yang menunggu. Sinyal hanya menjangkau
// replika yang sama; replika lain tertangkap polling cadangan (keputusan 16.1 #8).
type Notifier struct {
	mu      sync.Mutex
	waiting map[string]map[chan struct{}]struct{}
}

var _ application.Notifier = (*Notifier)(nil)

// NewNotifier membuat notifier dan, bila bus tidak nil, berlangganan execution.job_created.
func NewNotifier(bus eventbus.Bus) *Notifier {
	n := &Notifier{waiting: map[string]map[chan struct{}]struct{}{}}
	if bus != nil {
		bus.Subscribe(executionapi.TopicJobCreated, func(_ context.Context, ev eventbus.Event) error {
			switch e := ev.(type) {
			case executionapi.JobCreated:
				n.Notify(e.JobType)
			case *executionapi.JobCreated:
				n.Notify(e.JobType)
			}
			return nil
		})
	}
	return n
}

// Subscribe memenuhi application.Notifier.
func (n *Notifier) Subscribe(jobType string) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	n.mu.Lock()
	if n.waiting[jobType] == nil {
		n.waiting[jobType] = map[chan struct{}]struct{}{}
	}
	n.waiting[jobType][ch] = struct{}{}
	n.mu.Unlock()
	return ch, func() {
		n.mu.Lock()
		delete(n.waiting[jobType], ch)
		if len(n.waiting[jobType]) == 0 {
			delete(n.waiting, jobType)
		}
		n.mu.Unlock()
	}
}

// Notify memenuhi application.Notifier; sinyal yang belum dibaca digabung.
func (n *Notifier) Notify(jobType string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for ch := range n.waiting[jobType] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
