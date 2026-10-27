package eventbus

import (
	"context"
	"testing"
)

type testEvent struct{}

func (testEvent) Topic() string { return "test.happened" }

func TestPublishCallsOnlySubscribedHandlers(t *testing.T) {
	bus := NewInMemory()
	calls := 0
	bus.Subscribe("test.happened", func(context.Context, Event) error { calls++; return nil })
	bus.Subscribe("other.topic", func(context.Context, Event) error { calls += 100; return nil })

	if err := bus.Publish(context.Background(), testEvent{}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}
