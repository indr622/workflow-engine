package bpmn

import "strconv"

func (v *validator) activityRules(a *Activity) {
	if a.IsJobBased() {
		v.taskDefinitionRules(a, a.taskDefinition)
		v.headerRules(a, a.headers)
	}
	switch a.typ {
	case CallActivityType:
		if !a.calledElement.IsSet() || a.calledElement.ProcessID.IsEmpty() {
			v.errorAt(CodeCalledElementMissing, a, "call activity needs zeebe:calledElement with a processId")
		}
	case UserTaskType:
		if a.userTask.formSources() > 1 {
			v.errorAt(CodeUserTaskFormConflict, a, "user task may use only one of formKey, formId, or externalReference")
		}
	case ScriptTaskType:
		if a.script != nil && a.script.Expression.IsEmpty() {
			v.errorAt(CodeExpressionEmpty, a, "zeebe:script expression is empty")
		}
	case ReceiveTaskType:
		m, ok := a.Message()
		if !ok {
			v.errorAt(CodeMessageRefNotFound, a, "messageRef %q does not reference a message", a.messageRef)
		} else {
			v.messageRules(a, m, true)
		}
	}
	if mi, ok := a.MultiInstance(); ok {
		if mi.InputCollection.IsEmpty() {
			v.errorAt(CodeMultiInstanceInputMissing, a, "multi-instance needs zeebe:loopCharacteristics inputCollection")
		}
		if (mi.OutputCollection == "") != mi.OutputElement.IsEmpty() {
			v.errorAt(CodeMultiInstanceOutputPartial, a, "multi-instance outputCollection and outputElement must be set together")
		}
	}
}

func (v *validator) taskDefinitionRules(el FlowElement, td TaskDefinition) {
	if !td.IsSet() {
		v.errorAt(CodeTaskDefinitionMissing, el, "%s needs zeebe:taskDefinition", el.Type())
		return
	}
	if td.Type.IsEmpty() {
		v.errorAt(CodeTaskDefinitionTypeEmpty, el, "zeebe:taskDefinition type is empty")
	}
	if td.Retries.Raw == "" {
		return
	}
	if td.Retries.IsFEEL {
		if td.Retries.IsEmpty() {
			v.errorAt(CodeExpressionEmpty, el, "zeebe:taskDefinition retries expression is empty")
		}
		return
	}
	if n, err := strconv.Atoi(td.Retries.Raw); err != nil || n < 0 {
		v.errorAt(CodeTaskDefinitionRetriesInvalid, el, "retries %q must be a non-negative integer or a FEEL expression", td.Retries.Raw)
	}
}

func (v *validator) headerRules(el FlowElement, headers []Header) {
	seen := map[string]bool{}
	for _, h := range headers {
		if seen[h.Key] {
			v.errorAt(CodeTaskHeaderDuplicateKey, el, "task header key %q is used more than once", h.Key)
		}
		seen[h.Key] = true
	}
}
