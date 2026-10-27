package bpmn

import "strings"

// Expression adalah nilai atribut yang bisa berupa literal atau ekspresi FEEL.
// Mengikuti konvensi Zeebe: diawali "=" berarti FEEL. FEEL belum dievaluasi di Fase 1.
type Expression struct {
	Raw    string
	IsFEEL bool
}

func newExpression(raw string) Expression {
	trimmed := strings.TrimSpace(raw)
	return Expression{Raw: raw, IsFEEL: strings.HasPrefix(trimmed, "=")}
}

// IsEmpty mengembalikan true jika ekspresi kosong, termasuk "=" tanpa isi.
func (e Expression) IsEmpty() bool {
	t := strings.TrimSpace(e.Raw)
	return t == "" || (e.IsFEEL && strings.TrimSpace(strings.TrimPrefix(t, "=")) == "")
}

// TaskDefinition adalah zeebe:taskDefinition untuk task berbasis job.
type TaskDefinition struct {
	Type    Expression
	Retries Expression
	present bool
}

// IsSet mengembalikan true jika elemen zeebe:taskDefinition ada di XML.
func (t TaskDefinition) IsSet() bool { return t.present }

// Header adalah satu zeebe:header.
type Header struct {
	Key   string
	Value string
}

// Mapping adalah satu zeebe:input atau zeebe:output.
type Mapping struct {
	Source Expression
	Target string
}

// IOMapping adalah zeebe:ioMapping.
type IOMapping struct {
	Inputs  []Mapping
	Outputs []Mapping
}

// CalledElement adalah zeebe:calledElement untuk call activity.
type CalledElement struct {
	ProcessID                   Expression
	PropagateAllChildVariables  bool
	PropagateAllParentVariables bool
	present                     bool
}

// IsSet mengembalikan true jika elemen zeebe:calledElement ada di XML.
func (c CalledElement) IsSet() bool { return c.present }

// UserTaskDefinition mengumpulkan ekstensi zeebe untuk user task.
type UserTaskDefinition struct {
	ZeebeUserTask     bool
	FormKey           string
	FormID            string
	ExternalReference string
	Assignee          Expression
	CandidateGroups   Expression
	CandidateUsers    Expression
	DueDate           Expression
	FollowUpDate      Expression
}

func (u UserTaskDefinition) formSources() int {
	n := 0
	for _, s := range []string{u.FormKey, u.FormID, u.ExternalReference} {
		if s != "" {
			n++
		}
	}
	return n
}

// MultiInstance adalah multiInstanceLoopCharacteristics beserta zeebe:loopCharacteristics.
type MultiInstance struct {
	Sequential          bool
	InputCollection     Expression
	InputElement        string
	OutputCollection    string
	OutputElement       Expression
	CompletionCondition Expression
}

// CalledDecision adalah zeebe:calledDecision untuk business rule task.
type CalledDecision struct {
	DecisionID     string
	ResultVariable string
}

// Script adalah zeebe:script untuk script task berbasis FEEL.
type Script struct {
	Expression     Expression
	ResultVariable string
}

// RawExtension menyimpan ekstensi zeebe yang belum dimodelkan (mis. executionListeners).
type RawExtension struct {
	Name  string
	Attrs map[string]string
}
