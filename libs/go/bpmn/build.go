package bpmn

import (
	"strings"
	"unicode"

	"github.com/indr622/workflow-engine/libs/go/bpmn/internal/xmltree"
)

// build mengubah pohon XML menjadi model. Tidak pernah gagal: referensi gantung dan id
// bermasalah dicatat di Definitions.notes untuk dilaporkan Validate.
func build(root *xmltree.Node) *Definitions {
	b := &builder{
		defs: &Definitions{
			id:              root.AttrOr("", "id", ""),
			targetNamespace: root.AttrOr("", "targetNamespace", ""),
			messages:        map[string]*Message{},
			errors:          map[string]*ErrorDef{},
			escalations:     map[string]*Escalation{},
			signals:         map[string]*Signal{},
		},
		seen: map[string]bool{},
	}
	b.defs.executionPlatform = root.AttrOr(NamespaceModeler, "executionPlatform", "")
	b.defs.executionPlatformVersion = root.AttrOr(NamespaceModeler, "executionPlatformVersion", "")
	b.scanCamunda7(root, "")

	// Root element dibangun lebih dulu agar event bisa langsung me-resolve referensinya.
	for _, c := range bpmnChildren(root) {
		switch c.Local {
		case "message":
			b.buildMessage(c)
		case "error":
			id := b.registerID(c, "")
			e := &ErrorDef{id: id, name: c.AttrOr("", "name", ""), errorCode: c.AttrOr("", "errorCode", ""), pos: posOf(c)}
			b.defs.errors[id] = e
		case "escalation":
			id := b.registerID(c, "")
			b.defs.escalations[id] = &Escalation{id: id, name: c.AttrOr("", "name", ""), code: c.AttrOr("", "escalationCode", ""), pos: posOf(c)}
		case "signal":
			id := b.registerID(c, "")
			b.defs.signals[id] = &Signal{id: id, name: c.AttrOr("", "name", ""), pos: posOf(c)}
		}
	}
	for _, c := range bpmnChildren(root) {
		if c.Local == "process" {
			b.buildProcess(c)
		}
	}
	return b.defs
}

type builder struct {
	defs *Definitions
	seen map[string]bool
	// state per proses
	proc  *Process
	flows []*SequenceFlow
}

func bpmnChildren(n *xmltree.Node) []*xmltree.Node {
	var out []*xmltree.Node
	for _, c := range n.Children {
		if c.Space == NamespaceBPMN {
			out = append(out, c)
		}
	}
	return out
}

func posOf(n *xmltree.Node) Position { return Position{Line: n.Line, Column: n.Column} }

func (b *builder) processID() string {
	if b.proc == nil {
		return ""
	}
	return b.proc.id
}

// registerID mencatat id dan mendeteksi id kosong, bukan NCName, atau ganda.
func (b *builder) registerID(n *xmltree.Node, processID string) string {
	id := strings.TrimSpace(n.AttrOr("", "id", ""))
	at := noteAt{id: id, processID: processID, pos: posOf(n), detail: n.Local}
	switch {
	case id == "":
		b.defs.notes.missingIDs = append(b.defs.notes.missingIDs, at)
	case !isNCName(id):
		b.defs.notes.invalidIDs = append(b.defs.notes.invalidIDs, at)
	case b.seen[id]:
		b.defs.notes.duplicateIDs = append(b.defs.notes.duplicateIDs, at)
	}
	b.seen[id] = true
	return id
}

// isNCName memeriksa bentuk xsd:ID (NCName) secara praktis.
func isNCName(s string) bool {
	for i, r := range s {
		if i == 0 {
			if !unicode.IsLetter(r) && r != '_' {
				return false
			}
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' && r != '.' {
			return false
		}
	}
	return s != ""
}

func (b *builder) scanCamunda7(n *xmltree.Node, owner string) {
	id := n.AttrOr("", "id", owner)
	if n.Space == NamespaceCamunda7 || n.HasAttrInNamespace(NamespaceCamunda7) {
		b.defs.notes.camunda7 = append(b.defs.notes.camunda7, noteAt{id: id, pos: posOf(n), detail: n.Local})
		return // satu catatan per subtree cukup
	}
	for _, c := range n.Children {
		b.scanCamunda7(c, id)
	}
}

func (b *builder) buildMessage(n *xmltree.Node) {
	id := b.registerID(n, "")
	m := &Message{id: id, name: n.AttrOr("", "name", ""), pos: posOf(n)}
	if ext, ok := n.Child(NamespaceBPMN, "extensionElements"); ok {
		if sub, ok := ext.Child(NamespaceZeebe, "subscription"); ok {
			m.hasSubscription = true
			m.correlationKey = newExpression(sub.AttrOr("", "correlationKey", ""))
		}
	}
	b.defs.messages[id] = m
	b.defs.messageList = append(b.defs.messageList, m)
}

func (b *builder) buildProcess(n *xmltree.Node) {
	p := &Process{
		id:          strings.TrimSpace(n.AttrOr("", "id", "")),
		name:        n.AttrOr("", "name", ""),
		executable:  n.AttrOr("", "isExecutable", "false") == "true",
		pos:         posOf(n),
		index:       map[string]FlowElement{},
		unsupported: map[string]bool{},
		defs:        b.defs,
	}
	b.registerID(n, p.id)
	p.process = p
	if ext, ok := n.Child(NamespaceBPMN, "extensionElements"); ok {
		if vt, ok := ext.Child(NamespaceZeebe, "versionTag"); ok {
			p.versionTag = vt.AttrOr("", "value", "")
		}
	}
	b.proc = p
	b.flows = nil
	b.buildScope(n, &p.Scope)
	b.resolve()
	b.defs.processes = append(b.defs.processes, p)
	b.proc = nil
}

// unsupportedScopeElements adalah elemen alir yang dikenali tetapi tidak dimodelkan.
var unsupportedScopeElements = map[string]bool{
	"complexGateway": true, "transaction": true, "adHocSubProcess": true,
	"callChoreography": true, "subChoreography": true, "choreographyTask": true,
}

var activityTypes = map[string]ElementType{
	"task": TaskType, "manualTask": ManualTaskType, "serviceTask": ServiceTaskType,
	"sendTask": SendTaskType, "receiveTask": ReceiveTaskType, "userTask": UserTaskType,
	"scriptTask": ScriptTaskType, "businessRuleTask": BusinessRuleTaskType,
	"subProcess": SubProcessType, "callActivity": CallActivityType,
}

var eventTypes = map[string]ElementType{
	"startEvent": StartEventType, "endEvent": EndEventType,
	"intermediateCatchEvent": IntermediateCatchEventType,
	"intermediateThrowEvent": IntermediateThrowEventType,
	"boundaryEvent":          BoundaryEventType,
}

var gatewayTypes = map[string]ElementType{
	"exclusiveGateway": ExclusiveGatewayType, "parallelGateway": ParallelGatewayType,
	"inclusiveGateway": InclusiveGatewayType, "eventBasedGateway": EventBasedGatewayType,
}

func (b *builder) buildScope(n *xmltree.Node, scope *Scope) {
	for _, c := range bpmnChildren(n) {
		var el FlowElement
		switch {
		case c.Local == "sequenceFlow":
			el = b.buildFlow(c, scope)
		case eventTypes[c.Local] != UnknownType:
			ev := b.buildEvent(c, scope, eventTypes[c.Local])
			if ev.typ == StartEventType {
				scope.startEvents = append(scope.startEvents, ev)
			}
			el = ev
		case activityTypes[c.Local] != UnknownType:
			a := b.buildActivity(c, scope, activityTypes[c.Local])
			if a.inner != nil {
				scope.children = append(scope.children, a)
			}
			el = a
		case gatewayTypes[c.Local] != UnknownType:
			el = b.buildGateway(c, scope, gatewayTypes[c.Local])
		case unsupportedScopeElements[c.Local]:
			id := b.registerID(c, b.processID())
			b.proc.unsupported[id] = true
			b.defs.notes.unknown = append(b.defs.notes.unknown, unknownElement{name: c.Local, id: id, processID: b.processID(), pos: posOf(c)})
			continue
		default:
			continue // lane, annotation, data object, dll. tidak relevan untuk eksekusi
		}
		scope.elements = append(scope.elements, el)
		if _, exists := b.proc.index[el.ID()]; !exists && el.ID() != "" {
			b.proc.index[el.ID()] = el
		}
	}
}

func (b *builder) baseOf(n *xmltree.Node, scope *Scope, typ ElementType) base {
	return base{
		id:         b.registerID(n, b.processID()),
		name:       n.AttrOr("", "name", ""),
		typ:        typ,
		scope:      scope,
		pos:        posOf(n),
		properties: zeebeProperties(n),
	}
}

func (b *builder) nodeOf(n *xmltree.Node, scope *Scope, typ ElementType) node {
	nd := node{base: b.baseOf(n, scope, typ), ioMapping: zeebeIOMapping(n)}
	for _, c := range n.ChildrenNamed(NamespaceBPMN, "incoming") {
		nd.declaredIncoming = append(nd.declaredIncoming, c.Text)
	}
	for _, c := range n.ChildrenNamed(NamespaceBPMN, "outgoing") {
		nd.declaredOutgoing = append(nd.declaredOutgoing, c.Text)
	}
	return nd
}

func (b *builder) buildFlow(n *xmltree.Node, scope *Scope) *SequenceFlow {
	f := &SequenceFlow{
		base:      b.baseOf(n, scope, SequenceFlowType),
		sourceRef: strings.TrimSpace(n.AttrOr("", "sourceRef", "")),
		targetRef: strings.TrimSpace(n.AttrOr("", "targetRef", "")),
	}
	if ce, ok := n.Child(NamespaceBPMN, "conditionExpression"); ok {
		e := newExpression(ce.Text)
		f.condition = &e
	}
	b.flows = append(b.flows, f)
	return f
}

func (b *builder) buildGateway(n *xmltree.Node, scope *Scope, typ ElementType) *Gateway {
	return &Gateway{node: b.nodeOf(n, scope, typ), defaultRef: strings.TrimSpace(n.AttrOr("", "default", ""))}
}

var eventDefinitionKinds = map[string]EventDefinitionKind{
	"timerEventDefinition":      TimerEventDefinition,
	"messageEventDefinition":    MessageEventDefinition,
	"errorEventDefinition":      ErrorEventDefinition,
	"escalationEventDefinition": EscalationEventDefinition,
	"signalEventDefinition":     SignalEventDefinition,
	"terminateEventDefinition":  TerminateEventDefinition,
}

var unsupportedEventDefinitions = map[string]bool{
	"linkEventDefinition": true, "compensateEventDefinition": true,
	"conditionalEventDefinition": true, "cancelEventDefinition": true,
}

func (b *builder) buildEvent(n *xmltree.Node, scope *Scope, typ ElementType) *Event {
	e := &Event{
		node:           b.nodeOf(n, scope, typ),
		interrupting:   n.AttrOr("", "isInterrupting", "true") != "false",
		cancelActivity: n.AttrOr("", "cancelActivity", "true") != "false",
		attachedToRef:  strings.TrimSpace(n.AttrOr("", "attachedToRef", "")),
	}
	for _, c := range bpmnChildren(n) {
		if unsupportedEventDefinitions[c.Local] {
			e.definitionCount++
			b.defs.notes.unknown = append(b.defs.notes.unknown, unknownElement{
				name: c.Local, id: e.id, processID: b.processID(), pos: posOf(c),
			})
			continue
		}
		kind, ok := eventDefinitionKinds[c.Local]
		if !ok {
			continue
		}
		e.definitionCount++
		if e.definitionCount > 1 {
			continue // definition pertama dipakai; MULTIPLE_EVENT_DEFINITIONS dari definitionCount
		}
		e.definition = b.eventDefinition(c, kind)
	}
	if e.definitionCount > 1 {
		b.defs.notes.multiDefs = append(b.defs.notes.multiDefs, noteAt{id: e.id, processID: b.processID(), pos: e.pos})
	}
	if e.definition.kind == MessageEventDefinition && (typ == IntermediateThrowEventType || typ == EndEventType) {
		e.taskDefinition = zeebeTaskDefinition(n)
		e.headers = zeebeHeaders(n)
	}
	return e
}

func (b *builder) eventDefinition(n *xmltree.Node, kind EventDefinitionKind) EventDefinition {
	d := EventDefinition{kind: kind}
	switch kind {
	case TimerEventDefinition:
		for _, t := range []struct {
			local string
			kind  TimerKind
		}{{"timeDate", TimerDate}, {"timeDuration", TimerDuration}, {"timeCycle", TimerCycle}} {
			if c, ok := n.Child(NamespaceBPMN, t.local); ok {
				d.timer.count++
				if d.timer.Kind == TimerUnset {
					d.timer.Kind = t.kind
					d.timer.Value = newExpression(c.Text)
				}
			}
		}
	case MessageEventDefinition:
		d.ref = n.AttrOr("", "messageRef", "")
		d.message = b.defs.messages[d.ref]
	case ErrorEventDefinition:
		d.ref = n.AttrOr("", "errorRef", "")
		d.errorDef = b.defs.errors[d.ref]
	case EscalationEventDefinition:
		d.ref = n.AttrOr("", "escalationRef", "")
		d.escalation = b.defs.escalations[d.ref]
	case SignalEventDefinition:
		d.ref = n.AttrOr("", "signalRef", "")
		d.signal = b.defs.signals[d.ref]
	}
	return d
}

func (b *builder) buildActivity(n *xmltree.Node, scope *Scope, typ ElementType) *Activity {
	if typ == SubProcessType && n.AttrOr("", "triggeredByEvent", "false") == "true" {
		typ = EventSubProcessType
	}
	a := &Activity{node: b.nodeOf(n, scope, typ)}
	a.taskDefinition = zeebeTaskDefinition(n)
	a.headers = zeebeHeaders(n)
	if ext, ok := n.Child(NamespaceBPMN, "extensionElements"); ok {
		a.userTask = zeebeUserTask(ext)
		if ce, ok := ext.Child(NamespaceZeebe, "calledElement"); ok {
			a.calledElement = CalledElement{
				ProcessID:                   newExpression(ce.AttrOr("", "processId", "")),
				PropagateAllChildVariables:  ce.AttrOr("", "propagateAllChildVariables", "true") != "false",
				PropagateAllParentVariables: ce.AttrOr("", "propagateAllParentVariables", "true") != "false",
				present:                     true,
			}
		}
		if cd, ok := ext.Child(NamespaceZeebe, "calledDecision"); ok {
			a.calledDecision = &CalledDecision{DecisionID: cd.AttrOr("", "decisionId", ""), ResultVariable: cd.AttrOr("", "resultVariable", "")}
		}
		if sc, ok := ext.Child(NamespaceZeebe, "script"); ok {
			a.script = &Script{Expression: newExpression(sc.AttrOr("", "expression", "")), ResultVariable: sc.AttrOr("", "resultVariable", "")}
		}
		a.extensions = zeebeRawExtensions(ext)
	}
	if typ == ReceiveTaskType {
		a.messageRef = n.AttrOr("", "messageRef", "")
		a.message = b.defs.messages[a.messageRef]
	}
	if mi, ok := n.Child(NamespaceBPMN, "multiInstanceLoopCharacteristics"); ok {
		a.multiInstance = zeebeMultiInstance(mi)
	}
	if sl, ok := n.Child(NamespaceBPMN, "standardLoopCharacteristics"); ok {
		b.defs.notes.unknown = append(b.defs.notes.unknown, unknownElement{name: sl.Local, id: a.id, processID: b.processID(), pos: posOf(sl)})
	}
	if typ == SubProcessType || typ == EventSubProcessType {
		a.inner = &Scope{owner: a, process: b.proc}
		b.buildScope(n, a.inner)
	}
	return a
}

// resolve menyambungkan referensi di seluruh proses setelah semua elemen dibuat.
func (b *builder) resolve() {
	p := b.proc
	for _, f := range b.flows {
		if src, ok := p.index[f.sourceRef].(FlowNode); ok {
			f.source = src
			appendOutgoing(src, f)
		}
		if tgt, ok := p.index[f.targetRef].(FlowNode); ok {
			f.target = tgt
			appendIncoming(tgt, f)
		}
	}
	for _, el := range p.index {
		switch x := el.(type) {
		case *Gateway:
			if f, ok := p.index[x.defaultRef].(*SequenceFlow); ok {
				x.deflt = f
			}
		case *Event:
			if x.typ != BoundaryEventType {
				continue
			}
			host, exists := p.index[x.attachedToRef]
			act, isActivity := host.(*Activity)
			switch {
			case !exists:
				b.defs.notes.boundaryNoHost = append(b.defs.notes.boundaryNoHost, noteAt{id: x.id, processID: p.id, pos: x.pos, detail: x.attachedToRef})
			case !isActivity:
				b.defs.notes.boundaryBadHost = append(b.defs.notes.boundaryBadHost, noteAt{id: x.id, processID: p.id, pos: x.pos, detail: x.attachedToRef})
			default:
				x.attachedTo = act
				act.boundaryEvents = append(act.boundaryEvents, x)
			}
		}
	}
	// Urutan boundary event mengikuti dokumen agar deterministik.
	for _, el := range p.index {
		if a, ok := el.(*Activity); ok && len(a.boundaryEvents) > 1 {
			sortEventsByPosition(a.boundaryEvents)
		}
	}
}

func sortEventsByPosition(evs []*Event) {
	for i := 1; i < len(evs); i++ {
		for j := i; j > 0 && before(evs[j].pos, evs[j-1].pos); j-- {
			evs[j], evs[j-1] = evs[j-1], evs[j]
		}
	}
}

func before(a, b Position) bool {
	return a.Line < b.Line || (a.Line == b.Line && a.Column < b.Column)
}

func appendOutgoing(n FlowNode, f *SequenceFlow) {
	switch x := n.(type) {
	case *Event:
		x.outgoing = append(x.outgoing, f)
	case *Activity:
		x.outgoing = append(x.outgoing, f)
	case *Gateway:
		x.outgoing = append(x.outgoing, f)
	}
}

func appendIncoming(n FlowNode, f *SequenceFlow) {
	switch x := n.(type) {
	case *Event:
		x.incoming = append(x.incoming, f)
	case *Activity:
		x.incoming = append(x.incoming, f)
	case *Gateway:
		x.incoming = append(x.incoming, f)
	}
}

// --- ekstensi zeebe ---

func extension(n *xmltree.Node, local string) (*xmltree.Node, bool) {
	ext, ok := n.Child(NamespaceBPMN, "extensionElements")
	if !ok {
		return nil, false
	}
	return ext.Child(NamespaceZeebe, local)
}

func zeebeTaskDefinition(n *xmltree.Node) TaskDefinition {
	td, ok := extension(n, "taskDefinition")
	if !ok {
		return TaskDefinition{}
	}
	return TaskDefinition{
		Type:    newExpression(td.AttrOr("", "type", "")),
		Retries: newExpression(td.AttrOr("", "retries", "")),
		present: true,
	}
}

func zeebeHeaders(n *xmltree.Node) []Header {
	th, ok := extension(n, "taskHeaders")
	if !ok {
		return nil
	}
	var hs []Header
	for _, h := range th.ChildrenNamed(NamespaceZeebe, "header") {
		hs = append(hs, Header{Key: h.AttrOr("", "key", ""), Value: h.AttrOr("", "value", "")})
	}
	return hs
}

func zeebeIOMapping(n *xmltree.Node) IOMapping {
	io, ok := extension(n, "ioMapping")
	if !ok {
		return IOMapping{}
	}
	var m IOMapping
	for _, c := range io.ChildrenNamed(NamespaceZeebe, "input") {
		m.Inputs = append(m.Inputs, Mapping{Source: newExpression(c.AttrOr("", "source", "")), Target: c.AttrOr("", "target", "")})
	}
	for _, c := range io.ChildrenNamed(NamespaceZeebe, "output") {
		m.Outputs = append(m.Outputs, Mapping{Source: newExpression(c.AttrOr("", "source", "")), Target: c.AttrOr("", "target", "")})
	}
	return m
}

func zeebeProperties(n *xmltree.Node) map[string]string {
	ps, ok := extension(n, "properties")
	if !ok {
		return nil
	}
	out := map[string]string{}
	for _, p := range ps.ChildrenNamed(NamespaceZeebe, "property") {
		out[p.AttrOr("", "name", "")] = p.AttrOr("", "value", "")
	}
	return out
}

func zeebeUserTask(ext *xmltree.Node) UserTaskDefinition {
	var u UserTaskDefinition
	if _, ok := ext.Child(NamespaceZeebe, "userTask"); ok {
		u.ZeebeUserTask = true
	}
	if fd, ok := ext.Child(NamespaceZeebe, "formDefinition"); ok {
		u.FormKey = fd.AttrOr("", "formKey", "")
		u.FormID = fd.AttrOr("", "formId", "")
		u.ExternalReference = fd.AttrOr("", "externalReference", "")
	}
	if ad, ok := ext.Child(NamespaceZeebe, "assignmentDefinition"); ok {
		u.Assignee = newExpression(ad.AttrOr("", "assignee", ""))
		u.CandidateGroups = newExpression(ad.AttrOr("", "candidateGroups", ""))
		u.CandidateUsers = newExpression(ad.AttrOr("", "candidateUsers", ""))
	}
	if ts, ok := ext.Child(NamespaceZeebe, "taskSchedule"); ok {
		u.DueDate = newExpression(ts.AttrOr("", "dueDate", ""))
		u.FollowUpDate = newExpression(ts.AttrOr("", "followUpDate", ""))
	}
	return u
}

func zeebeMultiInstance(mi *xmltree.Node) *MultiInstance {
	m := &MultiInstance{Sequential: mi.AttrOr("", "isSequential", "false") == "true"}
	if ext, ok := mi.Child(NamespaceBPMN, "extensionElements"); ok {
		if lc, ok := ext.Child(NamespaceZeebe, "loopCharacteristics"); ok {
			m.InputCollection = newExpression(lc.AttrOr("", "inputCollection", ""))
			m.InputElement = lc.AttrOr("", "inputElement", "")
			m.OutputCollection = lc.AttrOr("", "outputCollection", "")
			m.OutputElement = newExpression(lc.AttrOr("", "outputElement", ""))
		}
	}
	if cc, ok := mi.Child(NamespaceBPMN, "completionCondition"); ok {
		m.CompletionCondition = newExpression(cc.Text)
	}
	return m
}

// Ekstensi zeebe yang sudah dimodelkan; selain ini disimpan sebagai RawExtension.
var modeledZeebeExtensions = map[string]bool{
	"taskDefinition": true, "taskHeaders": true, "ioMapping": true, "properties": true,
	"userTask": true, "formDefinition": true, "assignmentDefinition": true, "taskSchedule": true,
	"calledElement": true, "calledDecision": true, "script": true, "subscription": true,
	"loopCharacteristics": true, "versionTag": true,
}

func zeebeRawExtensions(ext *xmltree.Node) []RawExtension {
	var out []RawExtension
	for _, c := range ext.Children {
		if c.Space != NamespaceZeebe || modeledZeebeExtensions[c.Local] {
			continue
		}
		attrs := map[string]string{}
		for _, a := range c.Attrs {
			attrs[a.Name.Local] = a.Value
		}
		out = append(out, RawExtension{Name: c.Local, Attrs: attrs})
	}
	return out
}
