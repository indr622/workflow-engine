package bpmn

// executionPlatformCamunda8 adalah nilai modeler:executionPlatform untuk Camunda 8.
const executionPlatformCamunda8 = "Camunda Cloud"

// documentRules memeriksa aturan level dokumen dan catatan builder.
func (v *validator) documentRules() {
	n := v.defs.notes
	for _, x := range n.missingIDs {
		v.add(CodeMissingID, SeverityError, x.processID, "", x.pos, "%s element has no id", x.detail)
	}
	for _, x := range n.invalidIDs {
		v.add(CodeInvalidID, SeverityError, x.processID, x.id, x.pos, "id %q is not a valid XML NCName", x.id)
	}
	for _, x := range n.duplicateIDs {
		v.add(CodeDuplicateID, SeverityError, x.processID, x.id, x.pos, "id %q is used more than once", x.id)
	}
	for _, x := range n.unknown {
		v.add(CodeUnsupportedElement, SeverityError, x.processID, x.id, x.pos, "%s is not supported", x.name)
	}
	for _, x := range n.multiDefs {
		v.add(CodeMultipleEventDefinitions, SeverityError, x.processID, x.id, x.pos, "event has more than one event definition")
	}
	for _, x := range n.camunda7 {
		v.add(CodeCamunda7ExtensionIgnored, SeverityWarning, x.processID, x.id, x.pos,
			"Camunda 7 extension on %s is ignored", x.detail)
	}
	for _, x := range n.boundaryNoHost {
		v.add(CodeBoundaryAttachedToNotFound, SeverityError, x.processID, x.id, x.pos,
			"attachedToRef %q does not reference an element", x.detail)
	}
	for _, x := range n.boundaryBadHost {
		v.add(CodeBoundaryInvalidHost, SeverityError, x.processID, x.id, x.pos,
			"attachedToRef %q must reference an activity", x.detail)
	}
	if platform, _ := v.defs.ExecutionPlatform(); platform != "" && platform != executionPlatformCamunda8 {
		v.add(CodeExecutionPlatformMismatch, SeverityWarning, "", v.defs.id, Position{},
			"diagram targets execution platform %q, expected %q", platform, executionPlatformCamunda8)
	}
	if len(v.defs.ExecutableProcesses()) == 0 {
		v.add(CodeNoExecutableProcess, SeverityError, "", v.defs.id, Position{}, "definitions contain no executable process")
	}
}

// scopeRules memeriksa start event per scope.
func (v *validator) scopeRules(s *Scope) {
	owner, isSub := s.Owner()
	var ownerEl FlowElement = owner
	ownerID, ownerPos := v.proc.id, v.proc.pos
	if isSub {
		ownerID, ownerPos = owner.id, owner.pos
	}

	none := 0
	for _, e := range s.startEvents {
		if e.definition.kind == NoneEventDefinition && e.definitionCount == 0 {
			none++
			if none > 1 && !isSub {
				v.warnOrErr(CodeMultipleNoneStartEvents, e, "scope has more than one none start event")
			}
		}
	}

	switch {
	case !isSub:
		if len(s.startEvents) == 0 {
			v.add(CodeNoStartEvent, SeverityError, v.proc.id, ownerID, ownerPos, "process has no start event")
		}
	case owner.typ == SubProcessType:
		switch {
		case len(s.startEvents) == 0:
			v.errorAt(CodeNoStartEvent, ownerEl, "embedded sub-process has no start event")
		case len(s.startEvents) > 1 || none != 1:
			v.errorAt(CodeSubprocessInvalidStartEvent, ownerEl, "embedded sub-process must have exactly one none start event")
		}
	case owner.typ == EventSubProcessType:
		if len(s.startEvents) != 1 || none > 0 {
			v.errorAt(CodeEventSubprocessInvalidStart, ownerEl,
				"event sub-process must have exactly one start event with an event definition")
		}
	}
}

func (v *validator) warnOrErr(code Code, el FlowElement, msg string) {
	v.errorAt(code, el, msg)
}
