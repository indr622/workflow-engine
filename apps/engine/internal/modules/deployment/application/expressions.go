package application

import (
	"errors"
	"fmt"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/domain"
	"github.com/indr622/workflow-engine/libs/go/bpmn"
	"github.com/indr622/workflow-engine/libs/go/feel"
)

// Kode violation pemeriksaan ekspresi saat deploy (ADR-0014).
const (
	CodeExpressionNotSupported = "EXPRESSION_NOT_SUPPORTED"
	CodeExpressionSyntaxError  = "EXPRESSION_SYNTAX_ERROR"
)

// checkExpressions mengompilasi setiap kondisi sequence flow, ekspresi task definition, dan atribut
// user task job-based di proses executable dengan subset FEEL engine. Ekspresi di luar subset
// ditolak saat deploy agar tidak menjadi incident saat runtime; Fase 7 memperluas subset tanpa
// mengubah pemanggil.
func checkExpressions(resourceName string, defs *bpmn.Definitions) []domain.Violation {
	var out []domain.Violation
	for _, proc := range defs.Processes() {
		if !proc.IsExecutable() {
			continue
		}
		add := func(code string, el bpmn.FlowElement, msg string, args ...any) {
			pos := el.Position()
			out = append(out, domain.Violation{
				Code: code, Severity: domain.SeverityError, Message: fmt.Sprintf(msg, args...), ResourceName: resourceName,
				ProcessID: proc.ID(), ElementID: el.ID(), Line: pos.Line, Column: pos.Column,
			})
		}
		check := func(el bpmn.FlowElement, what string, expr bpmn.Expression, literalOrPath bool) {
			src, isFEEL := feel.StripPrefix(expr.Raw)
			if !isFEEL {
				return
			}
			compiled, err := feel.Compile(src)
			var unsupported *feel.UnsupportedError
			switch {
			case errors.As(err, &unsupported):
				add(CodeExpressionNotSupported, el, "%s uses a FEEL construct the engine does not support yet: %v", what, err)
			case err != nil:
				add(CodeExpressionSyntaxError, el, "%s is not a valid FEEL expression: %v", what, err)
			case literalOrPath && !compiled.IsLiteralOrPath():
				add(CodeExpressionNotSupported, el, "%s may only be a literal or a variable path for now", what)
			}
		}
		var walk func(s *bpmn.Scope)
		walk = func(s *bpmn.Scope) {
			for _, el := range s.Elements() {
				switch x := el.(type) {
				case *bpmn.SequenceFlow:
					cond, has := x.Condition()
					if !has {
						continue
					}
					if !cond.IsFEEL {
						add(CodeExpressionNotSupported, x, "condition must be a FEEL expression starting with '='")
						continue
					}
					check(x, "condition", cond, false)
				case *bpmn.Activity:
					if x.IsJobBased() {
						def := x.TaskDefinition()
						check(x, "job type", def.Type, true)
						check(x, "job retries", def.Retries, true)
					}
					if x.Type() == bpmn.UserTaskType {
						ut := x.UserTask()
						check(x, "assignee", ut.Assignee, false)
						check(x, "candidate groups", ut.CandidateGroups, false)
						check(x, "candidate users", ut.CandidateUsers, false)
						check(x, "due date", ut.DueDate, false)
						check(x, "follow-up date", ut.FollowUpDate, false)
					}
					if inner := x.Inner(); inner != nil {
						walk(inner)
					}
				}
			}
		}
		walk(&proc.Scope)
	}
	return out
}
