package bpmn

import "slices"

func (v *validator) flowRules(f *SequenceFlow) {
	if f.source == nil && !v.proc.unsupported[f.sourceRef] {
		v.errorAt(CodeFlowSourceNotFound, f, "sourceRef %q does not reference a flow node", f.sourceRef)
	}
	if f.target == nil && !v.proc.unsupported[f.targetRef] {
		v.errorAt(CodeFlowTargetNotFound, f, "targetRef %q does not reference a flow node", f.targetRef)
	}
	if (f.source != nil && f.source.Scope() != f.scope) || (f.target != nil && f.target.Scope() != f.scope) {
		v.errorAt(CodeFlowCrossesScope, f, "sequence flow must connect elements in the same scope")
	}
	if f.condition == nil || f.source == nil {
		return
	}
	if f.condition.IsEmpty() {
		v.errorAt(CodeExpressionEmpty, f, "condition expression is empty")
	}
	switch f.source.Type() {
	case ExclusiveGatewayType, InclusiveGatewayType:
	case ParallelGatewayType:
		v.errorAt(CodeParallelGatewayFlowCondition, f, "outgoing flows of a parallel gateway must not have conditions")
	default:
		v.errorAt(CodeConditionOnNonGatewayFlow, f, "conditional flows are only supported on exclusive and inclusive gateways")
	}
}

func (v *validator) gatewayRules(g *Gateway) {
	if g.defaultRef != "" {
		switch {
		case g.typ != ExclusiveGatewayType && g.typ != InclusiveGatewayType:
			v.errorAt(CodeGatewayDefaultFlowInvalid, g, "%s must not have a default flow", g.typ)
		case g.deflt == nil || g.deflt.source != FlowNode(g):
			v.errorAt(CodeGatewayDefaultFlowInvalid, g, "default flow %q is not an outgoing flow of this gateway", g.defaultRef)
		case g.deflt.condition != nil:
			v.add(CodeDefaultFlowHasCondition, SeverityError, v.proc.id, g.deflt.id, g.deflt.pos, "default flow must not have a condition")
		}
	}
	switch g.typ {
	case ExclusiveGatewayType, InclusiveGatewayType:
		if len(g.outgoing) > 1 {
			for _, f := range g.outgoing {
				if f.condition == nil && f != g.deflt {
					v.errorAt(CodeGatewayFlowMissingCondition, f, "flow leaving a diverging %s needs a condition or must be the default flow", g.typ)
				}
			}
		}
	case EventBasedGatewayType:
		if len(g.outgoing) < 2 {
			v.errorAt(CodeEventGatewayInvalidTarget, g, "event-based gateway must have at least two outgoing flows")
		}
		for _, f := range g.outgoing {
			if f.target == nil {
				continue
			}
			e, ok := f.target.(*Event)
			valid := ok && e.typ == IntermediateCatchEventType &&
				(e.definition.kind == TimerEventDefinition || e.definition.kind == MessageEventDefinition || e.definition.kind == SignalEventDefinition)
			if !valid {
				v.errorAt(CodeEventGatewayInvalidTarget, f, "event-based gateway may only lead to timer, message, or signal intermediate catch events")
			}
		}
	}
}

// nodeRules memeriksa aturan umum flow node: konsistensi incoming/outgoing dan I/O mapping.
func (v *validator) nodeRules(n FlowNode) {
	var declIn, declOut []string
	switch x := n.(type) {
	case *Event:
		declIn, declOut = x.declaredIncoming, x.declaredOutgoing
	case *Activity:
		declIn, declOut = x.declaredIncoming, x.declaredOutgoing
	case *Gateway:
		declIn, declOut = x.declaredIncoming, x.declaredOutgoing
	}
	if inconsistent(declIn, n.Incoming()) || inconsistent(declOut, n.Outgoing()) {
		v.warnAt(CodeFlowRefInconsistent, n, "incoming/outgoing references do not match sequence flow sourceRef/targetRef")
	}
	io := n.IOMapping()
	for _, m := range append(append([]Mapping(nil), io.Inputs...), io.Outputs...) {
		if m.Target == "" {
			v.errorAt(CodeIOMappingTargetEmpty, n, "I/O mapping has an empty target")
		}
		if m.Source.IsEmpty() {
			v.errorAt(CodeIOMappingSourceEmpty, n, "I/O mapping for target %q has an empty source", m.Target)
		}
	}
	v.implicitEndRule(n)
}

// inconsistent membandingkan referensi yang ditulis di XML dengan flow sebenarnya.
// Daftar kosong di XML tidak dianggap salah (file buatan tangan sering tidak menulisnya).
func inconsistent(declared []string, actual []*SequenceFlow) bool {
	if len(declared) == 0 {
		return false
	}
	ids := make([]string, 0, len(actual))
	for _, f := range actual {
		ids = append(ids, f.id)
	}
	a := slices.Clone(declared)
	slices.Sort(a)
	slices.Sort(ids)
	return !slices.Equal(a, ids)
}

func (v *validator) implicitEndRule(n FlowNode) {
	if len(n.Outgoing()) > 0 {
		return
	}
	switch n.Type() {
	case EndEventType, EventSubProcessType:
		return
	}
	v.warnAt(CodeImplicitEnd, n, "%s has no outgoing flow and ends its path implicitly", n.Type())
}

// reachabilityRules menandai flow node yang tidak bisa dicapai dari titik masuk scope.
func (v *validator) reachabilityRules(s *Scope) {
	reached := map[FlowNode]bool{}
	var queue []FlowNode
	visit := func(n FlowNode) {
		if n != nil && !reached[n] {
			reached[n] = true
			queue = append(queue, n)
		}
	}
	for _, e := range s.startEvents {
		visit(e)
	}
	for _, c := range s.children {
		if c.typ == EventSubProcessType {
			visit(c)
		}
	}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		for _, f := range n.Outgoing() {
			if f.target != nil && f.target.Scope() == s {
				visit(f.target)
			}
		}
		if a, ok := n.(*Activity); ok {
			for _, b := range a.boundaryEvents {
				visit(b)
			}
		}
	}
	if len(s.startEvents) == 0 {
		return // NO_START_EVENT sudah dilaporkan; semua node akan tampak tak terjangkau
	}
	for _, el := range s.elements {
		n, ok := el.(FlowNode)
		if !ok || reached[n] {
			continue
		}
		v.warnAt(CodeElementUnreachable, n, "%s cannot be reached from any start event", n.Type())
	}
}
