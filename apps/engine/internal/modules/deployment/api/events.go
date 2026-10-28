package api

import (
	"encoding/json"
	"fmt"
)

// TopicProcessDeployed adalah topik event bus untuk versi proses baru.
const TopicProcessDeployed = "deployment.process_deployed"

// ProcessDeployed dipublikasikan setelah commit untuk setiap versi baru (bukan duplikat).
type ProcessDeployed struct {
	ProcessDefinitionKey string
	BpmnProcessID        string
	Version              int32
	VersionTag           string
	DeploymentKey        string
	TenantID             string
}

// Topic memenuhi eventbus.Event.
func (ProcessDeployed) Topic() string { return TopicProcessDeployed }

// DecodeProcessDeployed mengubah payload outbox topik TopicProcessDeployed menjadi event api.
func DecodeProcessDeployed(payload []byte) (ProcessDeployed, error) {
	var ev ProcessDeployed
	if err := json.Unmarshal(payload, &ev); err != nil {
		return ev, fmt.Errorf("decode %s: %w", TopicProcessDeployed, err)
	}
	return ev, nil
}
