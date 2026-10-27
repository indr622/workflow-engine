package bpmn

// EventDefinitionKind adalah jenis event definition.
type EventDefinitionKind uint8

// Jenis event definition yang dimodelkan.
const (
	NoneEventDefinition EventDefinitionKind = iota
	TimerEventDefinition
	MessageEventDefinition
	ErrorEventDefinition
	EscalationEventDefinition
	SignalEventDefinition
	TerminateEventDefinition
)

func (k EventDefinitionKind) String() string {
	return [...]string{"none", "timer", "message", "error", "escalation", "signal", "terminate"}[k]
}

// TimerKind membedakan timeDate, timeDuration, dan timeCycle.
type TimerKind uint8

// Jenis timer.
const (
	TimerUnset TimerKind = iota
	TimerDate
	TimerDuration
	TimerCycle
)

func (k TimerKind) String() string {
	return [...]string{"unset", "date", "duration", "cycle"}[k]
}

// Timer adalah isi timerEventDefinition.
type Timer struct {
	Kind  TimerKind
	Value Expression
	// count adalah jumlah dari timeDate/timeDuration/timeCycle yang ditulis (harus tepat 1).
	count int
}

// EventDefinition adalah definisi event (timer, message, error, dst.).
type EventDefinition struct {
	kind       EventDefinitionKind
	timer      Timer
	ref        string // messageRef, errorRef, escalationRef, atau signalRef mentah
	message    *Message
	errorDef   *ErrorDef
	escalation *Escalation
	signal     *Signal
}

// Kind adalah jenis definition.
func (d EventDefinition) Kind() EventDefinitionKind { return d.kind }

// Timer mengembalikan definisi timer bila jenisnya timer.
func (d EventDefinition) Timer() (Timer, bool) { return d.timer, d.kind == TimerEventDefinition }

// Ref adalah referensi mentah (messageRef, errorRef, escalationRef, signalRef).
func (d EventDefinition) Ref() string { return d.ref }

// Message mengembalikan message yang dirujuk.
func (d EventDefinition) Message() (*Message, bool) { return d.message, d.message != nil }

// Error mengembalikan error yang dirujuk.
func (d EventDefinition) Error() (*ErrorDef, bool) { return d.errorDef, d.errorDef != nil }

// Escalation mengembalikan escalation yang dirujuk.
func (d EventDefinition) Escalation() (*Escalation, bool) { return d.escalation, d.escalation != nil }

// Signal mengembalikan signal yang dirujuk.
func (d EventDefinition) Signal() (*Signal, bool) { return d.signal, d.signal != nil }

// Event adalah start, end, intermediate catch/throw, atau boundary event.
type Event struct {
	node
	definition      EventDefinition
	definitionCount int
	interrupting    bool // start event di event sub-process
	cancelActivity  bool // boundary event
	attachedToRef   string
	attachedTo      *Activity
	taskDefinition  TaskDefinition
	headers         []Header
}

// Definition adalah event definition; Kind() == NoneEventDefinition untuk event polos.
func (e *Event) Definition() EventDefinition { return e.definition }

// IsInterrupting berlaku untuk start event di event sub-process.
func (e *Event) IsInterrupting() bool { return e.interrupting }

// CancelActivity berlaku untuk boundary event.
func (e *Event) CancelActivity() bool { return e.cancelActivity }

// AttachedTo adalah activity tempat boundary event menempel.
func (e *Event) AttachedTo() (*Activity, bool) { return e.attachedTo, e.attachedTo != nil }

// AttachedToRef adalah id activity mentah dari XML.
func (e *Event) AttachedToRef() string { return e.attachedToRef }

// TaskDefinition berlaku untuk message throw/end event berbasis job.
func (e *Event) TaskDefinition() TaskDefinition { return e.taskDefinition }

// Headers adalah zeebe:taskHeaders untuk message throw/end event berbasis job.
func (e *Event) Headers() []Header { return e.headers }
