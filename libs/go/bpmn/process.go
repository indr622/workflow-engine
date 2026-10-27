package bpmn

// Scope adalah wadah flow element: proses, sub-process, atau event sub-process.
type Scope struct {
	owner       *Activity // nil untuk scope proses
	process     *Process
	elements    []FlowElement
	startEvents []*Event
	children    []*Activity // sub-process dan event sub-process langsung
}

// Owner mengembalikan sub-process pemilik scope; false untuk scope proses.
func (s *Scope) Owner() (*Activity, bool) { return s.owner, s.owner != nil }

// Process mengembalikan proses yang menampung scope ini.
func (s *Scope) Process() *Process { return s.process }

// Elements adalah elemen langsung di scope ini, urutan dokumen.
func (s *Scope) Elements() []FlowElement { return s.elements }

// StartEvents adalah start event langsung di scope ini.
func (s *Scope) StartEvents() []*Event { return s.startEvents }

// Children adalah sub-process dan event sub-process langsung di scope ini.
func (s *Scope) Children() []*Activity { return s.children }

// NoneStartEvent mengembalikan start event tanpa definition di scope ini.
func (s *Scope) NoneStartEvent() (*Event, bool) {
	for _, e := range s.startEvents {
		if e.definition.kind == NoneEventDefinition {
			return e, true
		}
	}
	return nil, false
}

// Process adalah satu bpmn:process.
type Process struct {
	Scope
	id         string
	name       string
	versionTag string
	executable bool
	pos        Position
	index      map[string]FlowElement
	// unsupported adalah id elemen yang dikenali tetapi tidak dimodelkan; flow yang
	// menunjuknya tidak dilaporkan gantung karena UNSUPPORTED_ELEMENT sudah cukup.
	unsupported map[string]bool
	defs        *Definitions
}

// ID adalah bpmnProcessId.
func (p *Process) ID() string { return p.id }

// Name adalah nama proses.
func (p *Process) Name() string { return p.name }

// VersionTag adalah zeebe:versionTag.
func (p *Process) VersionTag() string { return p.versionTag }

// IsExecutable mengembalikan nilai isExecutable.
func (p *Process) IsExecutable() bool { return p.executable }

// Position adalah lokasi elemen process di XML.
func (p *Process) Position() Position { return p.pos }

// Definitions mengembalikan dokumen induk.
func (p *Process) Definitions() *Definitions { return p.defs }

// Element mencari elemen di seluruh scope bersarang berdasarkan id.
func (p *Process) Element(id string) (FlowElement, bool) {
	e, ok := p.index[id]
	return e, ok
}

// ElementCount adalah jumlah flow element di seluruh scope bersarang.
func (p *Process) ElementCount() int { return len(p.index) }
