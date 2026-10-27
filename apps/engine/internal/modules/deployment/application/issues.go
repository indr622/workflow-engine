// Package application berisi use case modul deployment.
package application

import (
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/domain"
	"github.com/indr622/workflow-engine/libs/go/bpmn"
)

// toViolations memetakan issue parser BPMN ke violation domain dengan nama resource.
func toViolations(resourceName string, issues bpmn.Issues) []domain.Violation {
	out := make([]domain.Violation, 0, len(issues))
	for _, i := range issues {
		sev := domain.SeverityError
		if i.Severity == bpmn.SeverityWarning {
			sev = domain.SeverityWarning
		}
		out = append(out, domain.Violation{
			Code: string(i.Code), Severity: sev, Message: i.Message, ResourceName: resourceName,
			ProcessID: i.ProcessID, ElementID: i.ElementID, Line: i.Position.Line, Column: i.Position.Column,
		})
	}
	return out
}

func hasErrors(vs []domain.Violation) bool {
	for _, v := range vs {
		if v.Severity == domain.SeverityError {
			return true
		}
	}
	return false
}

func warningsOnly(vs []domain.Violation) []domain.Violation {
	var out []domain.Violation
	for _, v := range vs {
		if v.Severity == domain.SeverityWarning {
			out = append(out, v)
		}
	}
	return out
}
