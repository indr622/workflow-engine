// Package application berisi use case modul health.
package application

import (
	"context"
	"time"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/health/domain"
)

// DependencyProbe adalah port keluar untuk memeriksa satu dependensi (database, broker, dll.).
type DependencyProbe interface {
	Name() string
	Probe(ctx context.Context) error
}

// CheckHealth adalah use case untuk menyusun laporan kesehatan engine.
type CheckHealth struct {
	version string
	probes  []DependencyProbe
	timeout time.Duration
}

// NewCheckHealth membuat use case dengan daftar probe yang akan dijalankan.
func NewCheckHealth(version string, timeout time.Duration, probes ...DependencyProbe) *CheckHealth {
	return &CheckHealth{version: version, probes: probes, timeout: timeout}
}

// Execute menjalankan semua probe dan mengembalikan laporan agregat.
func (uc *CheckHealth) Execute(ctx context.Context) domain.Report {
	ctx, cancel := context.WithTimeout(ctx, uc.timeout)
	defer cancel()

	results := make([]domain.DependencyResult, 0, len(uc.probes))
	for _, p := range uc.probes {
		result := domain.DependencyResult{Name: p.Name(), Status: domain.StatusServing}
		if err := p.Probe(ctx); err != nil {
			result.Status = domain.StatusNotServing
			result.Message = err.Error()
		}
		results = append(results, result)
	}
	return domain.NewReport(uc.version, results)
}

// IsHealthy memenuhi kontrak publik api.Checker untuk dipakai modul lain.
func (uc *CheckHealth) IsHealthy(ctx context.Context) bool {
	return uc.Execute(ctx).IsServing()
}
