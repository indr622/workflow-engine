package domain

import (
	"encoding/json"
	"time"

	"github.com/indr622/workflow-engine/libs/go/bpmn"
)

// ValueType adalah jenis entitas yang dicatat sebuah record.
type ValueType string

// Jenis record.
const (
	ValueProcessInstance         ValueType = "PROCESS_INSTANCE"
	ValueProcessInstanceCreation ValueType = "PROCESS_INSTANCE_CREATION"
	ValueVariable                ValueType = "VARIABLE"
	ValueJob                     ValueType = "JOB"
	ValueIncident                ValueType = "INCIDENT"
)

// Intent adalah apa yang terjadi pada entitas; nama mengikuti Zeebe.
type Intent string

// Intent record.
const (
	IntentElementActivating  Intent = "ELEMENT_ACTIVATING"
	IntentElementActivated   Intent = "ELEMENT_ACTIVATED"
	IntentElementCompleting  Intent = "ELEMENT_COMPLETING"
	IntentElementCompleted   Intent = "ELEMENT_COMPLETED"
	IntentElementTerminating Intent = "ELEMENT_TERMINATING"
	IntentElementTerminated  Intent = "ELEMENT_TERMINATED"
	IntentSequenceFlowTaken  Intent = "SEQUENCE_FLOW_TAKEN"
	IntentCreated            Intent = "CREATED"
	IntentUpdated            Intent = "UPDATED"
	IntentCompleted          Intent = "COMPLETED"
	IntentCanceled           Intent = "CANCELED"
	IntentResolved           Intent = "RESOLVED"
	IntentFailed             Intent = "FAILED"
	IntentErrorThrown        Intent = "ERROR_THROWN"
	IntentRetriesUpdated     Intent = "RETRIES_UPDATED"
)

// elementIntentState memetakan intent element ke state yang dihasilkannya.
var elementIntentState = map[Intent]ElementState{
	IntentElementActivating:  ElementActivating,
	IntentElementActivated:   ElementActivated,
	IntentElementCompleting:  ElementCompleting,
	IntentElementCompleted:   ElementCompleted,
	IntentElementTerminating: ElementTerminating,
	IntentElementTerminated:  ElementTerminated,
}

// ElementStateOf mengembalikan state yang dihasilkan intent element.
func ElementStateOf(i Intent) (ElementState, bool) {
	s, ok := elementIntentState[i]
	return s, ok
}

// Record adalah event append-only. State instance hanya berubah lewat Apply(record), sehingga
// menerapkan ulang seluruh record sebuah instance ke state kosong menghasilkan state yang sama.
type Record struct {
	// Key adalah key entitas: element instance, variable, job, atau incident.
	Key                string
	ProcessInstanceKey string
	ValueType          ValueType
	Intent             Intent
	Timestamp          time.Time
	// Value bertipe *ProcessInstanceValue, *ProcessInstanceCreationValue, *VariableValue,
	// *JobValue, atau *IncidentValue sesuai ValueType.
	Value any
}

// ElementID mengembalikan id elemen BPMN yang dirujuk record, kosong bila tidak ada.
func (r *Record) ElementID() string {
	switch v := r.Value.(type) {
	case *ProcessInstanceValue:
		return v.ElementID
	case *JobValue:
		return v.ElementID
	case *IncidentValue:
		return v.ElementID
	default:
		return ""
	}
}

// ProcessInstanceValue adalah payload record PROCESS_INSTANCE (element instance dan sequence flow).
type ProcessInstanceValue struct {
	BpmnProcessID        string `json:"bpmnProcessId"`
	ProcessDefinitionKey string `json:"processDefinitionKey"`
	Version              int32  `json:"version"`
	ElementID            string `json:"elementId"`
	BpmnElementType      string `json:"bpmnElementType"`
	FlowScopeKey         string `json:"flowScopeKey"`
}

// ProcessInstanceCreationValue adalah payload record PROCESS_INSTANCE_CREATION.
type ProcessInstanceCreationValue struct {
	BpmnProcessID        string   `json:"bpmnProcessId"`
	ProcessDefinitionKey string   `json:"processDefinitionKey"`
	Version              int32    `json:"version"`
	ProcessInstanceKey   string   `json:"processInstanceKey"`
	VariableNames        []string `json:"variableNames"`
}

// VariableValue adalah payload record VARIABLE.
type VariableValue struct {
	Name     string          `json:"name"`
	Value    json.RawMessage `json:"value"`
	ScopeKey string          `json:"scopeKey"`
}

// JobValue adalah payload record JOB.
type JobValue struct {
	Type                 string        `json:"type"`
	Retries              int32         `json:"retries"`
	CustomHeaders        []bpmn.Header `json:"customHeaders"`
	ElementInstanceKey   string        `json:"elementInstanceKey"`
	ElementID            string        `json:"elementId"`
	BpmnProcessID        string        `json:"bpmnProcessId"`
	ProcessDefinitionKey string        `json:"processDefinitionKey"`
	Version              int32         `json:"version"`
	// ErrorMessage diisi FAILED dan ERROR_THROWN.
	ErrorMessage string `json:"errorMessage,omitempty"`
	// ErrorCode diisi ERROR_THROWN.
	ErrorCode string `json:"errorCode,omitempty"`
	// RetryBackoffMs diisi FAILED: job dapat diaktifkan lagi pada timestamp record + backoff.
	RetryBackoffMs int64 `json:"retryBackoff,omitempty"`
}

// IncidentValue adalah payload record INCIDENT.
type IncidentValue struct {
	ErrorType          IncidentType `json:"errorType"`
	ErrorMessage       string       `json:"errorMessage"`
	ElementInstanceKey string       `json:"elementInstanceKey"`
	ElementID          string       `json:"elementId"`
	JobKey             string       `json:"jobKey,omitempty"`
}

// NewRecordValue mengembalikan pointer payload kosong untuk ValueType, untuk deserialisasi.
func NewRecordValue(t ValueType) (any, bool) {
	switch t {
	case ValueProcessInstance:
		return &ProcessInstanceValue{}, true
	case ValueProcessInstanceCreation:
		return &ProcessInstanceCreationValue{}, true
	case ValueVariable:
		return &VariableValue{}, true
	case ValueJob:
		return &JobValue{}, true
	case ValueIncident:
		return &IncidentValue{}, true
	default:
		return nil, false
	}
}
