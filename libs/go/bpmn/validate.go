package bpmn

import "fmt"

// Validate memeriksa model terhadap aturan semantik dan profil. Semua masalah dikumpulkan
// sekaligus; urutannya stabil (posisi, kode, element id). Hanya proses executable yang
// diperiksa secara mendalam, sesuai perilaku Zeebe.
func Validate(d *Definitions, p Profile) Issues {
	v := &validator{defs: d, profile: p}
	v.documentRules()
	for _, proc := range d.processes {
		if !proc.executable {
			v.add(CodeProcessNotExecutable, SeverityWarning, proc.id, proc.id, proc.pos,
				"process is not executable and is ignored")
			continue
		}
		v.proc = proc
		v.scopeRules(&proc.Scope)
		v.walk(&proc.Scope)
	}
	v.issues.sort()
	return v.issues
}

type validator struct {
	defs    *Definitions
	profile Profile
	proc    *Process
	issues  Issues
}

func (v *validator) add(code Code, sev Severity, processID, elementID string, pos Position, msg string, args ...any) {
	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}
	v.issues = append(v.issues, Issue{
		Code: code, Severity: sev, ProcessID: processID, ElementID: elementID, Message: msg, Position: pos,
	})
}

func (v *validator) errorAt(code Code, el FlowElement, msg string, args ...any) {
	v.add(code, SeverityError, v.proc.id, el.ID(), el.Position(), msg, args...)
}

func (v *validator) warnAt(code Code, el FlowElement, msg string, args ...any) {
	v.add(code, SeverityWarning, v.proc.id, el.ID(), el.Position(), msg, args...)
}

// walk memeriksa setiap elemen di scope dan scope bersarang.
func (v *validator) walk(s *Scope) {
	for _, el := range s.elements {
		if !v.profile.accepts(el) {
			v.errorAt(CodeUnsupportedElement, el, "%s is not supported in profile %q", describe(el), v.profile.name)
		}
		switch x := el.(type) {
		case *SequenceFlow:
			v.flowRules(x)
		case *Gateway:
			v.gatewayRules(x)
			v.nodeRules(x)
		case *Event:
			v.eventRules(x)
			v.nodeRules(x)
		case *Activity:
			v.activityRules(x)
			v.nodeRules(x)
			if x.inner != nil {
				v.scopeRules(x.inner)
				v.walk(x.inner)
			}
		}
	}
	v.reachabilityRules(s)
}

func describe(el FlowElement) string {
	if e, ok := el.(*Event); ok && e.definition.kind != NoneEventDefinition {
		return e.definition.kind.String() + " " + e.typ.String()
	}
	if a, ok := el.(*Activity); ok && a.multiInstance != nil {
		return "multi-instance " + a.typ.String()
	}
	return el.Type().String()
}
