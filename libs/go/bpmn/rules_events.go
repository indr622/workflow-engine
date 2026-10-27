package bpmn

// allowedDefinitions memetakan konteks event ke event definition yang diizinkan.
func allowedDefinitions(e *Event) map[EventDefinitionKind]bool {
	set := func(kinds ...EventDefinitionKind) map[EventDefinitionKind]bool {
		m := map[EventDefinitionKind]bool{}
		for _, k := range kinds {
			m[k] = true
		}
		return m
	}
	switch e.typ {
	case StartEventType:
		owner, isSub := e.scope.Owner()
		switch {
		case !isSub:
			return set(NoneEventDefinition, TimerEventDefinition, MessageEventDefinition, SignalEventDefinition)
		case owner.typ == EventSubProcessType:
			return set(TimerEventDefinition, MessageEventDefinition, ErrorEventDefinition, EscalationEventDefinition, SignalEventDefinition)
		default:
			return set(NoneEventDefinition)
		}
	case EndEventType:
		return set(NoneEventDefinition, ErrorEventDefinition, EscalationEventDefinition, MessageEventDefinition, SignalEventDefinition, TerminateEventDefinition)
	case IntermediateCatchEventType:
		return set(TimerEventDefinition, MessageEventDefinition, SignalEventDefinition)
	case IntermediateThrowEventType:
		return set(NoneEventDefinition, MessageEventDefinition, EscalationEventDefinition, SignalEventDefinition)
	case BoundaryEventType:
		return set(TimerEventDefinition, MessageEventDefinition, ErrorEventDefinition, EscalationEventDefinition, SignalEventDefinition)
	}
	return nil
}

func (v *validator) eventRules(e *Event) {
	switch {
	case e.typ == StartEventType && len(e.incoming) > 0:
		v.errorAt(CodeStartEventHasIncoming, e, "start event must not have incoming flows")
	case e.typ == BoundaryEventType && len(e.incoming) > 0:
		v.errorAt(CodeBoundaryEventHasIncoming, e, "boundary event must not have incoming flows")
	case e.typ == EndEventType && len(e.outgoing) > 0:
		v.errorAt(CodeEndEventHasOutgoing, e, "end event must not have outgoing flows")
	}

	// Definition tak didukung atau ganda sudah dilaporkan dari catatan builder.
	if e.definitionCount > 1 || (e.definitionCount == 1 && e.definition.kind == NoneEventDefinition) {
		return
	}
	if !allowedDefinitions(e)[e.definition.kind] && !v.startReportedByScope(e) {
		v.errorAt(CodeUnsupportedEventDefinition, e, "%s is not supported here", describe(e))
		return
	}

	switch e.definition.kind {
	case TimerEventDefinition:
		v.timerRules(e)
	case MessageEventDefinition:
		v.messageEventRules(e)
	case ErrorEventDefinition:
		v.errorEventRules(e)
	case EscalationEventDefinition:
		v.escalationEventRules(e)
	case SignalEventDefinition:
		v.signalEventRules(e)
	}
	if e.typ == BoundaryEventType {
		v.boundaryRules(e)
	}
}

// startReportedByScope: start event di sub-process sudah dilaporkan scopeRules.
func (v *validator) startReportedByScope(e *Event) bool {
	_, isSub := e.scope.Owner()
	return e.typ == StartEventType && isSub
}

func (v *validator) timerRules(e *Event) {
	t := e.definition.timer
	if t.count != 1 {
		v.errorAt(CodeTimerDefinitionInvalid, e, "timer must define exactly one of timeDate, timeDuration, or timeCycle")
		return
	}
	if t.Value.IsEmpty() {
		v.errorAt(CodeTimerDefinitionInvalid, e, "timer %s is empty", t.Kind)
		return
	}
	if t.Kind == TimerCycle {
		owner, isSub := e.scope.Owner()
		allowed := (e.typ == StartEventType && (!isSub || owner.typ == EventSubProcessType)) ||
			(e.typ == BoundaryEventType && !e.cancelActivity)
		if !allowed {
			v.errorAt(CodeTimerCycleNotAllowed, e, "timeCycle is only allowed on timer start events and non-interrupting boundary events")
			return
		}
	}
	if t.Value.IsFEEL {
		return
	}
	var ok bool
	switch t.Kind {
	case TimerDate:
		ok = isISODateTime(t.Value.Raw)
	case TimerDuration:
		ok = isISODuration(t.Value.Raw)
	case TimerCycle:
		ok = isTimerCycle(t.Value.Raw)
	}
	if !ok {
		v.errorAt(CodeTimerDefinitionInvalid, e, "timer %s %q is not a valid ISO 8601 value", t.Kind, t.Value.Raw)
	}
}

func (v *validator) messageEventRules(e *Event) {
	isThrow := e.typ == IntermediateThrowEventType || e.typ == EndEventType
	if isThrow {
		// Message throw/end di Zeebe dikerjakan job worker; messageRef opsional.
		v.taskDefinitionRules(e, e.taskDefinition)
		v.headerRules(e, e.headers)
		if e.definition.ref != "" && e.definition.message == nil {
			v.errorAt(CodeMessageRefNotFound, e, "messageRef %q does not reference a message", e.definition.ref)
		}
		return
	}
	m, ok := e.definition.Message()
	if !ok {
		v.errorAt(CodeMessageRefNotFound, e, "messageRef %q does not reference a message", e.definition.ref)
		return
	}
	v.messageRules(e, m, !v.isProcessLevelStart(e))
}

func (v *validator) isProcessLevelStart(e *Event) bool {
	_, isSub := e.scope.Owner()
	return e.typ == StartEventType && !isSub
}

// messageRules memeriksa message yang dirujuk catch event atau receive task.
func (v *validator) messageRules(el FlowElement, m *Message, needsCorrelationKey bool) {
	if m.name == "" {
		v.errorAt(CodeMessageNameMissing, el, "message %q has no name", m.id)
	}
	key, has := m.CorrelationKey()
	if needsCorrelationKey && (!has || key.IsEmpty()) {
		v.errorAt(CodeMessageCorrelationKeyMissing, el, "message %q needs a zeebe:subscription correlationKey", m.id)
	}
}

func (v *validator) errorEventRules(e *Event) {
	d := e.definition
	if e.typ == EndEventType {
		errDef, ok := d.Error()
		if !ok {
			v.errorAt(CodeErrorRefNotFound, e, "error end event must reference an error (errorRef %q)", d.ref)
			return
		}
		if errDef.errorCode == "" {
			v.errorAt(CodeErrorCodeMissing, e, "error %q has no errorCode", errDef.id)
		}
		return
	}
	if d.ref != "" && d.errorDef == nil {
		v.errorAt(CodeErrorRefNotFound, e, "errorRef %q does not reference an error", d.ref)
	}
}

func (v *validator) escalationEventRules(e *Event) {
	d := e.definition
	isThrow := e.typ == IntermediateThrowEventType || e.typ == EndEventType
	if !isThrow {
		if d.ref != "" && d.escalation == nil {
			v.errorAt(CodeEscalationRefNotFound, e, "escalationRef %q does not reference an escalation", d.ref)
		}
		return
	}
	esc, ok := d.Escalation()
	if !ok {
		v.errorAt(CodeEscalationRefNotFound, e, "escalation throw event must reference an escalation (escalationRef %q)", d.ref)
		return
	}
	if esc.code == "" {
		v.errorAt(CodeEscalationCodeMissing, e, "escalation %q has no escalationCode", esc.id)
	}
}

func (v *validator) signalEventRules(e *Event) {
	s, ok := e.definition.Signal()
	if !ok {
		v.errorAt(CodeSignalRefNotFound, e, "signalRef %q does not reference a signal", e.definition.ref)
		return
	}
	if s.name == "" {
		v.errorAt(CodeSignalNameMissing, e, "signal %q has no name", s.id)
	}
}

func (v *validator) boundaryRules(e *Event) {
	host, ok := e.AttachedTo()
	if !ok || e.definition.kind != ErrorEventDefinition {
		return
	}
	code := func(b *Event) string {
		if d, ok := b.definition.Error(); ok {
			return d.errorCode
		}
		return ""
	}
	mine := code(e)
	for _, other := range host.boundaryEvents {
		if other == e {
			return // hanya boundary kedua dan seterusnya yang dilaporkan
		}
		if other.definition.kind == ErrorEventDefinition && code(other) == mine {
			v.errorAt(CodeBoundaryDuplicateErrorCatch, e, "activity %q already has an error boundary event for error code %q", host.id, mine)
			return
		}
	}
}
