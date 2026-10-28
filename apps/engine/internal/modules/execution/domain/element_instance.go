// Package domain berisi state machine eksekusi process instance: tipe state runtime, record
// (event) append-only, Apply sebagai satu-satunya tempat state berubah, dan Processor yang
// menggerakkan token secara run-to-completion di atas model *bpmn.Process.
package domain

import (
	"strings"
	"unicode"

	"github.com/indr622/workflow-engine/libs/go/bpmn"
)

// ElementState adalah siklus hidup element instance, mengikuti nama Zeebe.
type ElementState string

// State element instance.
const (
	ElementActivating  ElementState = "ACTIVATING"
	ElementActivated   ElementState = "ACTIVATED"
	ElementCompleting  ElementState = "COMPLETING"
	ElementCompleted   ElementState = "COMPLETED"
	ElementTerminating ElementState = "TERMINATING"
	ElementTerminated  ElementState = "TERMINATED"
)

// IsFinal mengembalikan true untuk COMPLETED dan TERMINATED.
func (s ElementState) IsFinal() bool { return s == ElementCompleted || s == ElementTerminated }

// validTransitions adalah transisi state yang sah; state awal selalu ACTIVATING.
var validTransitions = map[ElementState][]ElementState{
	ElementActivating:  {ElementActivated, ElementTerminating},
	ElementActivated:   {ElementCompleting, ElementTerminating},
	ElementCompleting:  {ElementCompleted, ElementTerminating},
	ElementTerminating: {ElementTerminated},
}

// CanTransition mengembalikan true jika from → to sah.
func CanTransition(from, to ElementState) bool {
	for _, s := range validTransitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

// Tipe elemen Zeebe yang tidak punya padanan bpmn.ElementType.
const (
	ElementTypeProcess      = "PROCESS"
	ElementTypeSequenceFlow = "SEQUENCE_FLOW"
)

// BpmnElementType mengubah jenis elemen BPMN ke nama Zeebe, mis. serviceTask → SERVICE_TASK.
func BpmnElementType(t bpmn.ElementType) string {
	name := t.String()
	var b strings.Builder
	for i, r := range name {
		if unicode.IsUpper(r) && i > 0 {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToUpper(r))
	}
	return b.String()
}

// ElementInstance adalah satu aktivasi elemen di dalam process instance.
type ElementInstance struct {
	Key                int64
	ProcessInstanceKey int64
	// FlowScopeKey adalah element instance pembungkus; 0 untuk element instance PROCESS.
	FlowScopeKey    int64
	ElementID       string
	BpmnElementType string
	State           ElementState
	// IncidentKey adalah incident terbuka yang menahan elemen ini, 0 jika tidak ada.
	IncidentKey int64
}
