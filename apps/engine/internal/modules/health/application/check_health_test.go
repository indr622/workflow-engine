package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/health/domain"
)

type fakeProbe struct {
	name string
	err  error
}

func (f fakeProbe) Name() string                  { return f.name }
func (f fakeProbe) Probe(_ context.Context) error { return f.err }

func TestCheckHealthReportsFailingDependency(t *testing.T) {
	uc := NewCheckHealth("v-test", time.Second,
		fakeProbe{name: "postgres"},
		fakeProbe{name: "broker", err: errors.New("connection refused")},
	)

	report := uc.Execute(context.Background())

	if report.Status != domain.StatusNotServing {
		t.Fatalf("status = %s, want not_serving", report.Status)
	}
	if got := report.Dependencies[1].Message; got != "connection refused" {
		t.Fatalf("message = %q", got)
	}
	if uc.IsHealthy(context.Background()) {
		t.Fatal("IsHealthy should be false")
	}
}
