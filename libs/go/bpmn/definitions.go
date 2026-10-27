package bpmn

// Message adalah root element bpmn:message.
type Message struct {
	id              string
	name            string
	correlationKey  Expression
	hasSubscription bool
	pos             Position
}

// ID adalah id message.
func (m *Message) ID() string { return m.id }

// Name adalah nama message untuk korelasi.
func (m *Message) Name() string { return m.name }

// CorrelationKey adalah zeebe:subscription correlationKey.
func (m *Message) CorrelationKey() (Expression, bool) { return m.correlationKey, m.hasSubscription }

// ErrorDef adalah root element bpmn:error.
type ErrorDef struct {
	id        string
	name      string
	errorCode string
	pos       Position
}

// ID adalah id error.
func (e *ErrorDef) ID() string { return e.id }

// Name adalah nama error.
func (e *ErrorDef) Name() string { return e.name }

// ErrorCode adalah kode error yang dilempar/ditangkap.
func (e *ErrorDef) ErrorCode() string { return e.errorCode }

// Escalation adalah root element bpmn:escalation.
type Escalation struct {
	id   string
	name string
	code string
	pos  Position
}

// ID adalah id escalation.
func (e *Escalation) ID() string { return e.id }

// Name adalah nama escalation.
func (e *Escalation) Name() string { return e.name }

// EscalationCode adalah kode escalation.
func (e *Escalation) EscalationCode() string { return e.code }

// Signal adalah root element bpmn:signal.
type Signal struct {
	id   string
	name string
	pos  Position
}

// ID adalah id signal.
func (s *Signal) ID() string { return s.id }

// Name adalah nama signal.
func (s *Signal) Name() string { return s.name }

// Definitions adalah hasil Parse satu resource BPMN.
type Definitions struct {
	id                       string
	targetNamespace          string
	executionPlatform        string
	executionPlatformVersion string
	processes                []*Process
	messages                 map[string]*Message
	errors                   map[string]*ErrorDef
	escalations              map[string]*Escalation
	signals                  map[string]*Signal
	messageList              []*Message

	// Catatan builder untuk Validate.
	notes buildNotes
}

// ID adalah id definitions.
func (d *Definitions) ID() string { return d.id }

// TargetNamespace adalah atribut targetNamespace.
func (d *Definitions) TargetNamespace() string { return d.targetNamespace }

// ExecutionPlatform adalah modeler:executionPlatform dan versinya.
func (d *Definitions) ExecutionPlatform() (name, version string) {
	return d.executionPlatform, d.executionPlatformVersion
}

// Processes adalah semua proses, urutan dokumen.
func (d *Definitions) Processes() []*Process { return d.processes }

// ExecutableProcesses adalah proses dengan isExecutable="true".
func (d *Definitions) ExecutableProcesses() []*Process {
	var out []*Process
	for _, p := range d.processes {
		if p.executable {
			out = append(out, p)
		}
	}
	return out
}

// Process mencari proses berdasarkan id.
func (d *Definitions) Process(id string) (*Process, bool) {
	for _, p := range d.processes {
		if p.id == id {
			return p, true
		}
	}
	return nil, false
}

// Message mencari message berdasarkan id.
func (d *Definitions) Message(id string) (*Message, bool) { m, ok := d.messages[id]; return m, ok }

// Messages adalah semua message, urutan dokumen.
func (d *Definitions) Messages() []*Message { return d.messageList }

// Error mencari error berdasarkan id.
func (d *Definitions) Error(id string) (*ErrorDef, bool) { e, ok := d.errors[id]; return e, ok }

// Escalation mencari escalation berdasarkan id.
func (d *Definitions) Escalation(id string) (*Escalation, bool) {
	e, ok := d.escalations[id]
	return e, ok
}

// Signal mencari signal berdasarkan id.
func (d *Definitions) Signal(id string) (*Signal, bool) { s, ok := d.signals[id]; return s, ok }

// buildNotes adalah temuan builder yang dilaporkan Validate.
type buildNotes struct {
	missingIDs      []noteAt
	invalidIDs      []noteAt
	duplicateIDs    []noteAt
	unknown         []unknownElement
	multiDefs       []noteAt
	camunda7        []noteAt
	boundaryNoHost  []noteAt
	boundaryBadHost []noteAt
}

type noteAt struct {
	id        string
	processID string
	pos       Position
	detail    string
}
