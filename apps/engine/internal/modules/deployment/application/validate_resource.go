package application

import (
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/domain"
	"github.com/indr622/workflow-engine/libs/go/bpmn"
)

// ProcessSummary adalah ringkasan proses executable yang ditemukan saat validasi.
type ProcessSummary struct {
	ResourceName  string
	BpmnProcessID string
	Name          string
	VersionTag    string
	ElementCount  int
}

// ValidateResult adalah hasil dry run validasi.
type ValidateResult struct {
	Valid      bool
	Violations []domain.Violation
	Processes  []ProcessSummary
}

// ValidateResource menjalankan validasi yang sama dengan deploy tanpa menyimpan apa pun.
type ValidateResource struct{}

// Execute memvalidasi resource dengan profil yang diminta.
func (ValidateResource) Execute(inputs []domain.ResourceInput, profile bpmn.Profile) ValidateResult {
	parsed, violations := parseAndValidate(inputs, profile)
	res := ValidateResult{Valid: !hasErrors(violations), Violations: violations}
	for _, p := range parsed {
		for _, c := range p.candidates {
			proc, _ := p.defs.Process(c.BpmnProcessID)
			res.Processes = append(res.Processes, ProcessSummary{
				ResourceName: c.ResourceName, BpmnProcessID: c.BpmnProcessID, Name: c.Name,
				VersionTag: c.VersionTag, ElementCount: proc.ElementCount(),
			})
		}
	}
	return res
}
