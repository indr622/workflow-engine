package api

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
