package domain

import (
	"fmt"

	"github.com/indr622/workflow-engine/libs/go/bpmn"
)

// Apply menerapkan satu record ke state. Ini satu-satunya tempat InstanceState berubah, dipakai
// Processor saat menulis record dan oleh replay (Fase 3). Record yang tidak sah untuk state
// sekarang menghasilkan error berbalut ErrInvalidRecord dan state tidak berubah.
func Apply(st *InstanceState, r *Record) error {
	switch r.ValueType {
	case ValueProcessInstance:
		v, ok := r.Value.(*ProcessInstanceValue)
		if !ok {
			return invalid(r, "value is not a process instance value")
		}
		if r.Intent == IntentSequenceFlowTaken {
			return applyFlowTaken(st, r, v)
		}
		return applyElement(st, r, v)
	case ValueProcessInstanceCreation:
		return nil
	case ValueVariable:
		v, ok := r.Value.(*VariableValue)
		if !ok {
			return invalid(r, "value is not a variable value")
		}
		vk := VariableKey{ScopeKey: v.ScopeKey, Name: v.Name}
		existing, exists := st.Variables[vk]
		switch r.Intent {
		case IntentCreated:
			if exists {
				return invalid(r, "variable %q already exists in scope %s", v.Name, v.ScopeKey)
			}
			st.Variables[vk] = &Variable{Key: r.Key, ScopeKey: v.ScopeKey, Name: v.Name, Value: v.Value}
		case IntentUpdated:
			if !exists || existing.Key != r.Key {
				return invalid(r, "variable %q does not exist in scope %s", v.Name, v.ScopeKey)
			}
			existing.Value = v.Value
		default:
			return invalid(r, "unknown variable intent")
		}
		return nil
	case ValueJob:
		v, ok := r.Value.(*JobValue)
		if !ok {
			return invalid(r, "value is not a job value")
		}
		switch r.Intent {
		case IntentCreated:
			if _, exists := st.Jobs[r.Key]; exists {
				return invalid(r, "job already exists")
			}
			st.Jobs[r.Key] = &Job{
				Key: r.Key, ElementInstanceKey: v.ElementInstanceKey, ElementID: v.ElementID,
				Type: v.Type, Retries: v.Retries, CustomHeaders: v.CustomHeaders, State: JobCreated,
			}
		case IntentCompleted, IntentCanceled:
			if _, exists := st.Jobs[r.Key]; !exists {
				return invalid(r, "job is not open")
			}
			delete(st.Jobs, r.Key)
		case IntentFailed:
			job, exists := st.Jobs[r.Key]
			if !exists || job.State != JobCreated {
				return invalid(r, "job is not activatable")
			}
			job.Retries = v.Retries
			if v.Retries <= 0 {
				job.State = JobFailed
			}
		case IntentErrorThrown:
			job, exists := st.Jobs[r.Key]
			if !exists || job.State != JobCreated {
				return invalid(r, "job is not activatable")
			}
			job.State = JobErrorThrown
		case IntentRetriesUpdated:
			job, exists := st.Jobs[r.Key]
			if !exists {
				return invalid(r, "job is not open")
			}
			job.Retries = v.Retries
		default:
			return invalid(r, "unknown job intent")
		}
		return nil
	case ValueIncident:
		v, ok := r.Value.(*IncidentValue)
		if !ok {
			return invalid(r, "value is not an incident value")
		}
		switch r.Intent {
		case IntentCreated:
			el, exists := st.Elements[v.ElementInstanceKey]
			if !exists {
				return invalid(r, "incident element instance %s is not active", v.ElementInstanceKey)
			}
			st.Incidents[r.Key] = &Incident{
				Key: r.Key, ElementInstanceKey: v.ElementInstanceKey, ElementID: v.ElementID,
				Type: v.ErrorType, Message: v.ErrorMessage, JobKey: v.JobKey,
			}
			el.IncidentKey = r.Key
		case IntentResolved:
			if _, exists := st.Incidents[r.Key]; !exists {
				return invalid(r, "incident is not open")
			}
			delete(st.Incidents, r.Key)
			if el, ok := st.Elements[v.ElementInstanceKey]; ok && el.IncidentKey == r.Key {
				el.IncidentKey = ""
			}
			// Seperti Zeebe, menyelesaikan incident job mengembalikan job ke antrean aktivasi:
			// job ERROR_THROWN selalu, job FAILED hanya bila retries sudah dinaikkan.
			if job, ok := st.Jobs[v.JobKey]; ok && v.JobKey != "" && JobMadeActivatableOnResolve(job.State, job.Retries) {
				job.State = JobCreated
			}
		default:
			return invalid(r, "unknown incident intent")
		}
		return nil
	default:
		return invalid(r, "unknown value type")
	}
}

// JobMadeActivatableOnResolve mengembalikan true bila menyelesaikan incident milik job dengan
// state dan retries ini membuat job dapat diaktifkan lagi. Proyektor memakai aturan yang sama.
func JobMadeActivatableOnResolve(state JobState, retries int32) bool {
	return state == JobErrorThrown || (state == JobFailed && retries > 0)
}

func applyElement(st *InstanceState, r *Record, v *ProcessInstanceValue) error {
	next, ok := ElementStateOf(r.Intent)
	if !ok {
		return invalid(r, "unknown element intent")
	}
	isProcess := v.BpmnElementType == ElementTypeProcess
	if next == ElementActivating {
		if _, exists := st.Elements[r.Key]; exists {
			return invalid(r, "element instance already exists")
		}
		if isProcess {
			if st.Key != "" {
				return invalid(r, "instance already has a process element instance")
			}
			st.Key = r.Key
			st.Definition = DefinitionRef{Key: v.ProcessDefinitionKey, BpmnProcessID: v.BpmnProcessID, Version: v.Version}
			st.Status = InstanceActive
		} else if err := consumeJoin(st, r, v); err != nil {
			return err
		}
		st.Elements[r.Key] = &ElementInstance{
			Key: r.Key, ProcessInstanceKey: r.ProcessInstanceKey, FlowScopeKey: v.FlowScopeKey,
			ElementID: v.ElementID, BpmnElementType: v.BpmnElementType, State: ElementActivating,
		}
		return nil
	}
	el, exists := st.Elements[r.Key]
	if !exists {
		return invalid(r, "element instance is not active")
	}
	if !CanTransition(el.State, next) {
		return invalid(r, "transition %s -> %s is not allowed", el.State, next)
	}
	el.State = next
	if !next.IsFinal() {
		return nil
	}
	delete(st.Elements, r.Key)
	if isProcess {
		if next == ElementCompleted {
			st.Status = InstanceCompleted
		} else {
			st.Status = InstanceTerminated
		}
		// Counter join yang tersisa di scope yang selesai tidak punya record sendiri.
		for k := range st.TakenFlows {
			if k.FlowScopeKey == r.Key {
				delete(st.TakenFlows, k)
			}
		}
	}
	return nil
}

// consumeJoin mengurangi counter incoming gateway join saat gateway diaktifkan: parallel butuh
// semua incoming ≥ 1, inclusive mengurangi incoming yang > 0.
func consumeJoin(st *InstanceState, r *Record, v *ProcessInstanceValue) error {
	if st.Process == nil {
		return nil
	}
	el, ok := st.Process.Element(v.ElementID)
	if !ok {
		return invalid(r, "element %q is not in the process", v.ElementID)
	}
	node, ok := el.(bpmn.FlowNode)
	if !ok || !isJoin(node) {
		return nil
	}
	consumed := 0
	for _, in := range node.Incoming() {
		k := JoinKey{FlowScopeKey: v.FlowScopeKey, GatewayID: node.ID(), SequenceFlowID: in.ID()}
		c := st.TakenFlows[k]
		if c == 0 {
			if node.Type() == bpmn.ParallelGatewayType {
				return invalid(r, "parallel join %q activated before flow %q arrived", node.ID(), in.ID())
			}
			continue
		}
		consumed++
		if c == 1 {
			delete(st.TakenFlows, k)
		} else {
			st.TakenFlows[k] = c - 1
		}
	}
	if consumed == 0 {
		return invalid(r, "join %q activated without any arrived flow", node.ID())
	}
	return nil
}

func applyFlowTaken(st *InstanceState, r *Record, v *ProcessInstanceValue) error {
	if st.Process == nil {
		return nil
	}
	el, ok := st.Process.Element(v.ElementID)
	if !ok {
		return invalid(r, "sequence flow %q is not in the process", v.ElementID)
	}
	flow, ok := el.(*bpmn.SequenceFlow)
	if !ok || flow.Target() == nil {
		return invalid(r, "%q is not a connected sequence flow", v.ElementID)
	}
	if isJoin(flow.Target()) {
		st.TakenFlows[JoinKey{FlowScopeKey: v.FlowScopeKey, GatewayID: flow.Target().ID(), SequenceFlowID: flow.ID()}]++
	}
	return nil
}

func invalid(r *Record, format string, args ...any) error {
	return fmt.Errorf("%w: %s %s key %s: %s", ErrInvalidRecord, r.ValueType, r.Intent, r.Key, fmt.Sprintf(format, args...))
}
