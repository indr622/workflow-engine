package domain

import (
	"encoding/json"
	"sort"

	"github.com/indr622/workflow-engine/libs/go/bpmn"
)

// InstanceStatus adalah status process instance.
type InstanceStatus string

// Status process instance.
const (
	InstanceActive     InstanceStatus = "ACTIVE"
	InstanceCompleted  InstanceStatus = "COMPLETED"
	InstanceTerminated InstanceStatus = "TERMINATED"
)

// JobState adalah state job. Fase 4 menambah ACTIVATED, FAILED, TIMED_OUT, ERROR_THROWN.
type JobState string

// State job Fase 2.
const (
	JobCreated   JobState = "CREATED"
	JobCompleted JobState = "COMPLETED"
	JobCanceled  JobState = "CANCELED"
)

// DefinitionRef menunjuk versi process definition yang dijalankan instance.
type DefinitionRef struct {
	Key           string
	BpmnProcessID string
	Version       int32
}

// JoinKey mengidentifikasi counter sequence flow yang sudah sampai di gateway join.
type JoinKey struct {
	FlowScopeKey   string
	GatewayID      string
	SequenceFlowID string
}

// VariableKey mengidentifikasi variabel di sebuah scope.
type VariableKey struct {
	ScopeKey string
	Name     string
}

// Variable adalah satu variabel; Value adalah teks JSON ringkas (json.Compact) apa adanya.
type Variable struct {
	Key      string
	ScopeKey string
	Name     string
	Value    json.RawMessage
}

// Job adalah job terbuka milik sebuah service task.
type Job struct {
	Key                string
	ElementInstanceKey string
	ElementID          string
	Type               string
	Retries            int32
	CustomHeaders      []bpmn.Header
	State              JobState
}

// Incident adalah incident terbuka.
type Incident struct {
	Key                string
	ElementInstanceKey string
	ElementID          string
	Type               IncidentType
	Message            string
}

// InstanceState adalah state RUNTIME satu process instance: hanya yang dibutuhkan untuk memproses
// command berikutnya. Element instance final, job selesai, dan incident yang sudah diselesaikan
// tidak disimpan di sini; riwayatnya ada di proyeksi.
type InstanceState struct {
	Key        string
	Definition DefinitionRef
	Process    *bpmn.Process
	Status     InstanceStatus
	Elements   map[string]*ElementInstance
	TakenFlows map[JoinKey]int32
	Variables  map[VariableKey]*Variable
	Jobs       map[string]*Job
	Incidents  map[string]*Incident
}

// NewInstanceState membuat state kosong untuk instance baru dari model proses.
func NewInstanceState(def DefinitionRef, proc *bpmn.Process) *InstanceState {
	return &InstanceState{
		Definition: def,
		Process:    proc,
		Elements:   map[string]*ElementInstance{},
		TakenFlows: map[JoinKey]int32{},
		Variables:  map[VariableKey]*Variable{},
		Jobs:       map[string]*Job{},
		Incidents:  map[string]*Incident{},
	}
}

// TakenFlow adalah satu counter join tersimpan.
type TakenFlow struct {
	JoinKey
	Count int32
}

// InstanceSnapshot adalah state runtime yang dimuat repository dari proyeksi.
type InstanceSnapshot struct {
	Key        string
	Definition DefinitionRef
	Status     InstanceStatus
	Elements   []ElementInstance
	TakenFlows []TakenFlow
	Variables  []Variable
	Jobs       []Job
	Incidents  []Incident
}

// Restore membangun InstanceState dari snapshot dan model proses.
func Restore(s *InstanceSnapshot, proc *bpmn.Process) *InstanceState {
	st := NewInstanceState(s.Definition, proc)
	st.Key = s.Key
	st.Status = s.Status
	for i := range s.Elements {
		e := s.Elements[i]
		st.Elements[e.Key] = &e
	}
	for _, f := range s.TakenFlows {
		st.TakenFlows[f.JoinKey] = f.Count
	}
	for i := range s.Variables {
		v := s.Variables[i]
		st.Variables[VariableKey{ScopeKey: v.ScopeKey, Name: v.Name}] = &v
	}
	for i := range s.Jobs {
		j := s.Jobs[i]
		st.Jobs[j.Key] = &j
	}
	for i := range s.Incidents {
		inc := s.Incidents[i]
		st.Incidents[inc.Key] = &inc
	}
	return st
}

// HasIncident mengembalikan true jika ada incident terbuka.
func (s *InstanceState) HasIncident() bool { return len(s.Incidents) > 0 }

// ChildrenOf mengembalikan element instance non-final dengan flow scope tertentu, urut key naik.
func (s *InstanceState) ChildrenOf(scopeKey string) []*ElementInstance {
	var out []*ElementInstance
	for _, e := range s.Elements {
		if e.FlowScopeKey == scopeKey && e.Key != scopeKey {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// hasChildren mengembalikan true jika scope masih punya element instance non-final, tanpa alokasi.
func (s *InstanceState) hasChildren(scopeKey string) bool {
	for _, e := range s.Elements {
		if e.FlowScopeKey == scopeKey && e.Key != scopeKey {
			return true
		}
	}
	return false
}

// SortedTakenFlows mengembalikan counter join urut (scope, gateway, flow) untuk iterasi deterministik.
func (s *InstanceState) SortedTakenFlows() []TakenFlow {
	out := make([]TakenFlow, 0, len(s.TakenFlows))
	for k, c := range s.TakenFlows {
		out = append(out, TakenFlow{JoinKey: k, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.FlowScopeKey != b.FlowScopeKey {
			return a.FlowScopeKey < b.FlowScopeKey
		}
		if a.GatewayID != b.GatewayID {
			return a.GatewayID < b.GatewayID
		}
		return a.SequenceFlowID < b.SequenceFlowID
	})
	return out
}

// SortedVariables mengembalikan variabel urut (scope, nama).
func (s *InstanceState) SortedVariables() []Variable {
	out := make([]Variable, 0, len(s.Variables))
	for _, v := range s.Variables {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ScopeKey != out[j].ScopeKey {
			return out[i].ScopeKey < out[j].ScopeKey
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Lookup memenuhi VariableLookup untuk scope process instance.
func (s *InstanceState) Lookup(name string) (json.RawMessage, bool) {
	v, ok := s.Variables[VariableKey{ScopeKey: s.Key, Name: name}]
	if !ok {
		return nil, false
	}
	return v.Value, true
}

// isJoin mengembalikan true untuk gateway yang menyinkronkan token: parallel atau inclusive
// dengan lebih dari satu incoming flow.
func isJoin(n bpmn.FlowNode) bool {
	t := n.Type()
	return (t == bpmn.ParallelGatewayType || t == bpmn.InclusiveGatewayType) && len(n.Incoming()) > 1
}
