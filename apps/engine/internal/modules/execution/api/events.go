package api

import (
	"encoding/json"
	"fmt"
)

// Topik event bus modul execution. Semua dipublikasikan setelah commit; outbox transaksional
// menyusul di Fase 3.
const (
	TopicProcessInstanceCompleted  = "execution.process_instance_completed"
	TopicProcessInstanceTerminated = "execution.process_instance_terminated"
	TopicJobCreated                = "execution.job_created"
	TopicIncidentCreated           = "execution.incident_created"
)

// ProcessInstanceCompleted: instance mencapai akhir.
type ProcessInstanceCompleted struct {
	ProcessInstanceKey   string
	ProcessDefinitionKey string
	BpmnProcessID        string
}

// Topic memenuhi eventbus.Event.
func (ProcessInstanceCompleted) Topic() string { return TopicProcessInstanceCompleted }

// ProcessInstanceTerminated: instance dibatalkan.
type ProcessInstanceTerminated struct {
	ProcessInstanceKey   string
	ProcessDefinitionKey string
	BpmnProcessID        string
}

// Topic memenuhi eventbus.Event.
func (ProcessInstanceTerminated) Topic() string { return TopicProcessInstanceTerminated }

// JobCreated: service task membuat job baru. Fase 4 memakainya untuk membangunkan long polling.
type JobCreated struct {
	JobKey             string
	JobType            string
	ProcessInstanceKey string
	ElementInstanceKey string
	ElementID          string
}

// Topic memenuhi eventbus.Event.
func (JobCreated) Topic() string { return TopicJobCreated }

// IncidentCreated: eksekusi berhenti di sebuah elemen karena incident.
type IncidentCreated struct {
	IncidentKey        string
	ErrorType          string
	ProcessInstanceKey string
	ElementInstanceKey string
	ElementID          string
}

// Topic memenuhi eventbus.Event.
func (IncidentCreated) Topic() string { return TopicIncidentCreated }

// Event adalah event modul execution yang bisa dikirim lewat outbox atau bus in-process.
type Event interface{ Topic() string }

// DecodeEvent mengubah payload outbox menjadi tipe event api; dipakai consumer modul lain.
func DecodeEvent(topic string, payload []byte) (Event, error) {
	var ev Event
	switch topic {
	case TopicProcessInstanceCompleted:
		ev = &ProcessInstanceCompleted{}
	case TopicProcessInstanceTerminated:
		ev = &ProcessInstanceTerminated{}
	case TopicJobCreated:
		ev = &JobCreated{}
	case TopicIncidentCreated:
		ev = &IncidentCreated{}
	default:
		return nil, fmt.Errorf("unknown execution topic %q", topic)
	}
	if err := json.Unmarshal(payload, ev); err != nil {
		return nil, fmt.Errorf("decode %s: %w", topic, err)
	}
	return ev, nil
}
