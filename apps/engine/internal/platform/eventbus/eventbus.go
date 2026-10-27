// Package eventbus menyediakan event bus in-process untuk komunikasi antar modul.
package eventbus

import (
	"context"
	"sync"
)

// Event adalah pesan domain yang dipublikasikan sebuah modul.
type Event interface {
	// Topic mengembalikan nama topik event, misalnya "deployment.process_deployed".
	Topic() string
}

// Handler memproses satu event.
type Handler func(ctx context.Context, event Event) error

// Bus adalah kontrak publish/subscribe yang dipakai modul.
type Bus interface {
	Publish(ctx context.Context, event Event) error
	Subscribe(topic string, handler Handler)
}

// InMemory adalah implementasi Bus sinkron di dalam proses.
type InMemory struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
}

// NewInMemory membuat event bus in-process.
func NewInMemory() *InMemory {
	return &InMemory{handlers: make(map[string][]Handler)}
}

// Publish memanggil semua handler untuk topik event secara berurutan.
// Error pertama menghentikan pemanggilan handler berikutnya.
func (b *InMemory) Publish(ctx context.Context, event Event) error {
	b.mu.RLock()
	handlers := append([]Handler(nil), b.handlers[event.Topic()]...)
	b.mu.RUnlock()
	for _, h := range handlers {
		if err := h(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

// Subscribe mendaftarkan handler untuk sebuah topik.
func (b *InMemory) Subscribe(topic string, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[topic] = append(b.handlers[topic], handler)
}
