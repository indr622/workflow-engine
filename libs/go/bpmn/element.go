package bpmn

// ElementType adalah jenis elemen BPMN.
type ElementType uint8

// Jenis elemen yang dimodelkan.
const (
	UnknownType ElementType = iota
	StartEventType
	EndEventType
	IntermediateCatchEventType
	IntermediateThrowEventType
	BoundaryEventType
	TaskType
	ManualTaskType
	ServiceTaskType
	SendTaskType
	ReceiveTaskType
	UserTaskType
	ScriptTaskType
	BusinessRuleTaskType
	SubProcessType
	EventSubProcessType
	CallActivityType
	ExclusiveGatewayType
	ParallelGatewayType
	InclusiveGatewayType
	EventBasedGatewayType
	SequenceFlowType
)

var elementTypeNames = map[ElementType]string{
	StartEventType:             "startEvent",
	EndEventType:               "endEvent",
	IntermediateCatchEventType: "intermediateCatchEvent",
	IntermediateThrowEventType: "intermediateThrowEvent",
	BoundaryEventType:          "boundaryEvent",
	TaskType:                   "task",
	ManualTaskType:             "manualTask",
	ServiceTaskType:            "serviceTask",
	SendTaskType:               "sendTask",
	ReceiveTaskType:            "receiveTask",
	UserTaskType:               "userTask",
	ScriptTaskType:             "scriptTask",
	BusinessRuleTaskType:       "businessRuleTask",
	SubProcessType:             "subProcess",
	EventSubProcessType:        "eventSubProcess",
	CallActivityType:           "callActivity",
	ExclusiveGatewayType:       "exclusiveGateway",
	ParallelGatewayType:        "parallelGateway",
	InclusiveGatewayType:       "inclusiveGateway",
	EventBasedGatewayType:      "eventBasedGateway",
	SequenceFlowType:           "sequenceFlow",
}

// String mengembalikan nama lokal BPMN, mis. "serviceTask".
func (t ElementType) String() string {
	if s, ok := elementTypeNames[t]; ok {
		return s
	}
	return "unknown"
}

// IsEvent mengembalikan true untuk semua jenis event.
func (t ElementType) IsEvent() bool { return t >= StartEventType && t <= BoundaryEventType }

// IsActivity mengembalikan true untuk task, sub-process, dan call activity.
func (t ElementType) IsActivity() bool { return t >= TaskType && t <= CallActivityType }

// IsGateway mengembalikan true untuk semua gateway.
func (t ElementType) IsGateway() bool { return t >= ExclusiveGatewayType && t <= EventBasedGatewayType }

// FlowElement adalah elemen apa pun di dalam proses: flow node atau sequence flow.
type FlowElement interface {
	ID() string
	Name() string
	Type() ElementType
	// Scope adalah proses atau sub-process yang menampung elemen ini.
	Scope() *Scope
	Position() Position
	// Properties adalah zeebe:properties.
	Properties() map[string]string
}

// FlowNode adalah event, activity, atau gateway.
type FlowNode interface {
	FlowElement
	Incoming() []*SequenceFlow
	Outgoing() []*SequenceFlow
	IOMapping() IOMapping
}

// base menampung field bersama semua elemen.
type base struct {
	id         string
	name       string
	typ        ElementType
	scope      *Scope
	pos        Position
	properties map[string]string
}

func (b *base) ID() string                    { return b.id }
func (b *base) Name() string                  { return b.name }
func (b *base) Type() ElementType             { return b.typ }
func (b *base) Scope() *Scope                 { return b.scope }
func (b *base) Position() Position            { return b.pos }
func (b *base) Properties() map[string]string { return b.properties }

// node menampung field bersama flow node.
type node struct {
	base
	incoming  []*SequenceFlow
	outgoing  []*SequenceFlow
	ioMapping IOMapping
	// referensi yang ditulis di XML lewat <incoming>/<outgoing>, untuk cek konsistensi.
	declaredIncoming []string
	declaredOutgoing []string
}

func (n *node) Incoming() []*SequenceFlow { return n.incoming }
func (n *node) Outgoing() []*SequenceFlow { return n.outgoing }
func (n *node) IOMapping() IOMapping      { return n.ioMapping }

// SequenceFlow menghubungkan dua flow node di scope yang sama.
type SequenceFlow struct {
	base
	source    FlowNode
	target    FlowNode
	sourceRef string
	targetRef string
	condition *Expression
}

// Source adalah node asal. Nil jika sourceRef tidak ditemukan (lihat Validate).
func (f *SequenceFlow) Source() FlowNode { return f.source }

// Target adalah node tujuan. Nil jika targetRef tidak ditemukan (lihat Validate).
func (f *SequenceFlow) Target() FlowNode { return f.target }

// SourceRef dan TargetRef adalah id mentah dari XML.
func (f *SequenceFlow) SourceRef() string { return f.sourceRef }

// TargetRef adalah id node tujuan mentah dari XML.
func (f *SequenceFlow) TargetRef() string { return f.targetRef }

// Condition mengembalikan conditionExpression bila ada.
func (f *SequenceFlow) Condition() (Expression, bool) {
	if f.condition == nil {
		return Expression{}, false
	}
	return *f.condition, true
}

// Gateway adalah exclusive, parallel, inclusive, atau event-based gateway.
type Gateway struct {
	node
	defaultRef string
	deflt      *SequenceFlow
}

// Default mengembalikan default flow untuk exclusive/inclusive gateway.
func (g *Gateway) Default() (*SequenceFlow, bool) { return g.deflt, g.deflt != nil }

// DefaultRef adalah id default flow mentah dari XML.
func (g *Gateway) DefaultRef() string { return g.defaultRef }

// unknownElement adalah elemen BPMN yang dikenali tetapi tidak dimodelkan.
type unknownElement struct {
	name      string
	id        string
	processID string
	pos       Position
}
