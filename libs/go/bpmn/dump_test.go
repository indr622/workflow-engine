package bpmn

import (
	"sort"
)

// dump mengubah model menjadi struktur kanonik untuk golden file: hanya field semantik,
// tanpa posisi, elemen terurut id. Dipakai test agar diff golden stabil.
func dump(d *Definitions) map[string]any {
	out := map[string]any{"id": d.ID()}
	if p, v := d.ExecutionPlatform(); p != "" {
		out["executionPlatform"] = p + " " + v
	}
	if ms := d.Messages(); len(ms) > 0 {
		var list []map[string]any
		for _, m := range ms {
			e := map[string]any{"id": m.ID(), "name": m.Name()}
			if key, ok := m.CorrelationKey(); ok {
				e["correlationKey"] = dumpExpr(key)
			}
			list = append(list, e)
		}
		out["messages"] = list
	}
	addRoots := func(name string, items map[string]any) {
		if len(items) > 0 {
			out[name] = items
		}
	}
	errs := map[string]any{}
	for id, e := range d.errors {
		errs[id] = map[string]any{"name": e.Name(), "errorCode": e.ErrorCode()}
	}
	addRoots("errors", errs)
	escs := map[string]any{}
	for id, e := range d.escalations {
		escs[id] = map[string]any{"name": e.Name(), "escalationCode": e.EscalationCode()}
	}
	addRoots("escalations", escs)
	sigs := map[string]any{}
	for id, s := range d.signals {
		sigs[id] = map[string]any{"name": s.Name()}
	}
	addRoots("signals", sigs)

	var procs []map[string]any
	for _, p := range d.Processes() {
		procs = append(procs, dumpProcess(p))
	}
	out["processes"] = procs
	return out
}

func dumpProcess(p *Process) map[string]any {
	out := map[string]any{
		"id":         p.ID(),
		"name":       p.Name(),
		"executable": p.IsExecutable(),
	}
	if p.VersionTag() != "" {
		out["versionTag"] = p.VersionTag()
	}
	ids := make([]string, 0, len(p.index))
	for id := range p.index {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	elements := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		elements = append(elements, dumpElement(p.index[id]))
	}
	out["elements"] = elements
	return out
}

func dumpElement(el FlowElement) map[string]any {
	out := map[string]any{"id": el.ID(), "type": el.Type().String()}
	if el.Name() != "" {
		out["name"] = el.Name()
	}
	if owner, ok := el.Scope().Owner(); ok {
		out["scope"] = owner.ID()
	}
	if props := el.Properties(); len(props) > 0 {
		out["properties"] = props
	}
	if n, ok := el.(FlowNode); ok {
		if in := flowIDs(n.Incoming()); len(in) > 0 {
			out["incoming"] = in
		}
		if o := flowIDs(n.Outgoing()); len(o) > 0 {
			out["outgoing"] = o
		}
		if io := n.IOMapping(); len(io.Inputs)+len(io.Outputs) > 0 {
			out["ioMapping"] = map[string]any{"inputs": dumpMappings(io.Inputs), "outputs": dumpMappings(io.Outputs)}
		}
	}
	switch x := el.(type) {
	case *SequenceFlow:
		out["source"], out["target"] = x.SourceRef(), x.TargetRef()
		if c, ok := x.Condition(); ok {
			out["condition"] = dumpExpr(c)
		}
	case *Gateway:
		if d, ok := x.Default(); ok {
			out["default"] = d.ID()
		}
	case *Event:
		if x.Definition().Kind() != NoneEventDefinition {
			out["definition"] = dumpDefinition(x.Definition())
		}
		if x.Type() == BoundaryEventType {
			out["attachedTo"] = x.AttachedToRef()
			out["cancelActivity"] = x.CancelActivity()
		}
		if x.Type() == StartEventType && !x.IsInterrupting() {
			out["interrupting"] = false
		}
		if td := x.TaskDefinition(); td.IsSet() {
			out["taskDefinition"] = dumpTaskDefinition(td)
		}
		if hs := x.Headers(); len(hs) > 0 {
			out["headers"] = dumpHeaders(hs)
		}
	case *Activity:
		if td := x.TaskDefinition(); td.IsSet() {
			out["taskDefinition"] = dumpTaskDefinition(td)
		}
		if hs := x.Headers(); len(hs) > 0 {
			out["headers"] = dumpHeaders(hs)
		}
		if ce := x.CalledElement(); ce.IsSet() {
			out["calledElement"] = map[string]any{
				"processId":                   dumpExpr(ce.ProcessID),
				"propagateAllChildVariables":  ce.PropagateAllChildVariables,
				"propagateAllParentVariables": ce.PropagateAllParentVariables,
			}
		}
		if cd, ok := x.CalledDecision(); ok {
			out["calledDecision"] = map[string]any{"decisionId": cd.DecisionID, "resultVariable": cd.ResultVariable}
		}
		if s, ok := x.Script(); ok {
			out["script"] = map[string]any{"expression": dumpExpr(s.Expression), "resultVariable": s.ResultVariable}
		}
		if x.Type() == UserTaskType {
			u := x.UserTask()
			ut := map[string]any{"zeebeUserTask": u.ZeebeUserTask}
			for k, v := range map[string]string{"formKey": u.FormKey, "formId": u.FormID, "externalReference": u.ExternalReference} {
				if v != "" {
					ut[k] = v
				}
			}
			for k, v := range map[string]Expression{"assignee": u.Assignee, "candidateGroups": u.CandidateGroups, "candidateUsers": u.CandidateUsers, "dueDate": u.DueDate, "followUpDate": u.FollowUpDate} {
				if v.Raw != "" {
					ut[k] = dumpExpr(v)
				}
			}
			out["userTask"] = ut
		}
		if mi, ok := x.MultiInstance(); ok {
			m := map[string]any{"sequential": mi.Sequential, "inputCollection": dumpExpr(mi.InputCollection)}
			if mi.InputElement != "" {
				m["inputElement"] = mi.InputElement
			}
			if mi.OutputCollection != "" {
				m["outputCollection"] = mi.OutputCollection
				m["outputElement"] = dumpExpr(mi.OutputElement)
			}
			if mi.CompletionCondition.Raw != "" {
				m["completionCondition"] = dumpExpr(mi.CompletionCondition)
			}
			out["multiInstance"] = m
		}
		if x.MessageRef() != "" {
			out["messageRef"] = x.MessageRef()
		}
		if bs := x.BoundaryEvents(); len(bs) > 0 {
			ids := make([]string, 0, len(bs))
			for _, b := range bs {
				ids = append(ids, b.ID())
			}
			out["boundaryEvents"] = ids
		}
		if inner := x.Inner(); inner != nil {
			ids := make([]string, 0, len(inner.Elements()))
			for _, e := range inner.Elements() {
				ids = append(ids, e.ID())
			}
			sort.Strings(ids)
			out["inner"] = ids
		}
		if ex := x.Extensions(); len(ex) > 0 {
			names := make([]string, 0, len(ex))
			for _, e := range ex {
				names = append(names, e.Name)
			}
			out["rawExtensions"] = names
		}
	}
	return out
}

func dumpDefinition(d EventDefinition) map[string]any {
	out := map[string]any{"kind": d.Kind().String()}
	if t, ok := d.Timer(); ok {
		out["timer"] = map[string]any{"kind": t.Kind.String(), "value": dumpExpr(t.Value)}
	}
	if d.Ref() != "" {
		out["ref"] = d.Ref()
	}
	return out
}

func dumpTaskDefinition(td TaskDefinition) map[string]any {
	out := map[string]any{"type": dumpExpr(td.Type)}
	if td.Retries.Raw != "" {
		out["retries"] = dumpExpr(td.Retries)
	}
	return out
}

func dumpHeaders(hs []Header) []map[string]string {
	out := make([]map[string]string, 0, len(hs))
	for _, h := range hs {
		out = append(out, map[string]string{"key": h.Key, "value": h.Value})
	}
	return out
}

func dumpMappings(ms []Mapping) []map[string]any {
	out := make([]map[string]any, 0, len(ms))
	for _, m := range ms {
		out = append(out, map[string]any{"source": dumpExpr(m.Source), "target": m.Target})
	}
	return out
}

func dumpExpr(e Expression) string {
	if e.IsFEEL {
		return "feel:" + e.Raw
	}
	return e.Raw
}

func flowIDs(fs []*SequenceFlow) []string {
	ids := make([]string, 0, len(fs))
	for _, f := range fs {
		ids = append(ids, f.ID())
	}
	return ids
}
