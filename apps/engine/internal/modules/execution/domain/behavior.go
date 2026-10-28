package domain

import (
	"fmt"
	"math"

	"github.com/indr622/workflow-engine/libs/go/bpmn"
)

// DefaultJobRetries dipakai bila zeebe:taskDefinition retries kosong.
const DefaultJobRetries = 3

// chooseExclusive memilih satu outgoing: flow pertama (urutan dokumen) yang kondisinya true, lalu
// default, lalu gagal. Mengembalikan pesan incident bila tidak ada flow yang bisa diambil.
func (r *run) chooseExclusive(node bpmn.FlowNode) ([]*bpmn.SequenceFlow, string) {
	out := node.Outgoing()
	if len(out) == 0 {
		return nil, ""
	}
	deflt := defaultFlow(node)
	if len(out) == 1 {
		if _, has := out[0].Condition(); !has {
			return out, ""
		}
	}
	for _, f := range out {
		if f == deflt {
			continue
		}
		ok, msg := r.evalCondition(node, f)
		if msg != "" {
			return nil, msg
		}
		if ok {
			return []*bpmn.SequenceFlow{f}, ""
		}
	}
	if deflt != nil {
		return []*bpmn.SequenceFlow{deflt}, ""
	}
	return nil, fmt.Sprintf("expected at least one condition of exclusive gateway %q to evaluate to true, or a default flow, but none did", node.ID())
}

// chooseInclusive mengambil semua outgoing yang kondisinya true (semua dievaluasi), default bila
// tidak ada, atau gagal.
func (r *run) chooseInclusive(node bpmn.FlowNode) ([]*bpmn.SequenceFlow, string) {
	out := node.Outgoing()
	if len(out) == 0 {
		return nil, ""
	}
	deflt := defaultFlow(node)
	var taken []*bpmn.SequenceFlow
	for _, f := range out {
		if f == deflt {
			continue
		}
		ok, msg := r.evalCondition(node, f)
		if msg != "" {
			return nil, msg
		}
		if ok {
			taken = append(taken, f)
		}
	}
	if len(taken) > 0 {
		return taken, ""
	}
	if deflt != nil {
		return []*bpmn.SequenceFlow{deflt}, ""
	}
	return nil, fmt.Sprintf("expected at least one condition of inclusive gateway %q to evaluate to true, or a default flow, but none did", node.ID())
}

// evalCondition mengevaluasi kondisi flow; flow tanpa kondisi dianggap true.
func (r *run) evalCondition(node bpmn.FlowNode, f *bpmn.SequenceFlow) (bool, string) {
	cond, has := f.Condition()
	if !has {
		return true, ""
	}
	ok, err := r.p.evaluator.EvaluateBool(cond, r.st)
	if err != nil {
		return false, fmt.Sprintf("failed to evaluate condition of sequence flow %q leaving gateway %q: %v", f.ID(), node.ID(), err)
	}
	return ok, ""
}

func defaultFlow(node bpmn.FlowNode) *bpmn.SequenceFlow {
	if g, ok := node.(*bpmn.Gateway); ok {
		if d, has := g.Default(); has {
			return d
		}
	}
	return nil
}

// activateServiceTask mengevaluasi task definition lalu membuat job; elemen menunggu di ACTIVATED.
func (r *run) activateServiceTask(el *ElementInstance, node bpmn.FlowNode) error {
	act, ok := node.(*bpmn.Activity)
	if !ok {
		return fmt.Errorf("%w: service task %q is not an activity", ErrInvalidRecord, node.ID())
	}
	def := act.TaskDefinition()
	jobType, err := r.p.evaluator.EvaluateString(def.Type, r.st)
	if err == nil && jobType == "" {
		err = fmt.Errorf("job type is empty")
	}
	if err != nil {
		return r.raiseIncident(el, IncidentExtractValueError, fmt.Sprintf("failed to evaluate job type of %q: %v", node.ID(), err))
	}
	retries := int64(DefaultJobRetries)
	if !def.Retries.IsEmpty() {
		retries, err = r.p.evaluator.EvaluateInt(def.Retries, r.st)
		if err == nil && (retries < 0 || retries > math.MaxInt32) {
			err = fmt.Errorf("retries %d is out of range", retries)
		}
		if err != nil {
			return r.raiseIncident(el, IncidentExtractValueError, fmt.Sprintf("failed to evaluate job retries of %q: %v", node.ID(), err))
		}
	}
	if err := r.emitElement(el, IntentElementActivated); err != nil {
		return err
	}
	jobKey, err := r.nextKey()
	if err != nil {
		return err
	}
	d := r.st.Definition
	return r.emit(jobKey, ValueJob, IntentCreated, &JobValue{
		Type: jobType, Retries: int32(retries), CustomHeaders: act.Headers(),
		ElementInstanceKey: el.Key, ElementID: el.ElementID,
		BpmnProcessID: d.BpmnProcessID, ProcessDefinitionKey: d.Key, Version: d.Version,
	})
}
