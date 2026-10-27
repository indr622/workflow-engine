package domain

import (
	"path"
	"strings"
	"time"
)

// Resource adalah satu file yang di-deploy. Fase 1 hanya menerima BPMN.
type Resource struct {
	Name     string
	Content  []byte
	Checksum Checksum
}

// Kode violation level resource (di luar kode validasi BPMN).
const (
	CodeNoResources             = "NO_RESOURCES"
	CodeEmptyResource           = "EMPTY_RESOURCE"
	CodeDuplicateResourceName   = "DUPLICATE_RESOURCE_NAME"
	CodeUnsupportedResourceType = "UNSUPPORTED_RESOURCE_TYPE"
	CodeDuplicateProcessID      = "DUPLICATE_PROCESS_ID"
)

// NewResources memeriksa daftar resource mentah dan menghitung checksum-nya.
// Semua pelanggaran dikumpulkan sekaligus.
func NewResources(inputs []ResourceInput) ([]Resource, []Violation) {
	var violations []Violation
	if len(inputs) == 0 {
		violations = append(violations, Violation{Code: CodeNoResources, Severity: SeverityError, Message: "at least one resource is required"})
		return nil, violations
	}
	seen := map[string]bool{}
	resources := make([]Resource, 0, len(inputs))
	for _, in := range inputs {
		name := strings.TrimSpace(in.Name)
		v := Violation{Severity: SeverityError, ResourceName: name}
		switch {
		case name == "":
			v.Code, v.Message = CodeUnsupportedResourceType, "resource name is required"
		case strings.ToLower(path.Ext(name)) != ".bpmn":
			v.Code, v.Message = CodeUnsupportedResourceType, "only .bpmn resources are supported in this phase"
		case seen[name]:
			v.Code, v.Message = CodeDuplicateResourceName, "resource name is used more than once in the request"
		case len(in.Content) == 0:
			v.Code, v.Message = CodeEmptyResource, "resource content is empty"
		}
		seen[name] = true
		if v.Code != "" {
			violations = append(violations, v)
			continue
		}
		resources = append(resources, Resource{Name: name, Content: in.Content, Checksum: ChecksumOf(in.Content)})
	}
	return resources, violations
}

// ResourceInput adalah resource mentah dari request.
type ResourceInput struct {
	Name    string
	Content []byte
}

// ProcessCandidate adalah proses executable di dalam resource sebelum diberi versi.
type ProcessCandidate struct {
	ResourceName  string
	BpmnProcessID string
	Name          string
	VersionTag    string
	Checksum      Checksum
}

// Deployment adalah aggregate hasil satu request deploy.
type Deployment struct {
	Key       int64
	TenantID  string
	CreatedAt time.Time
	Resources []Resource
	// Definitions berisi versi baru yang dibuat deployment ini (bukan duplikat).
	Definitions []ProcessDefinition
}

// CheckUniqueProcessIDs menolak dua proses executable dengan id sama di satu deployment.
func CheckUniqueProcessIDs(candidates []ProcessCandidate) []Violation {
	seen := map[string]string{}
	var out []Violation
	for _, c := range candidates {
		if other, ok := seen[c.BpmnProcessID]; ok {
			out = append(out, Violation{
				Code: CodeDuplicateProcessID, Severity: SeverityError, ResourceName: c.ResourceName,
				ProcessID: c.BpmnProcessID,
				Message:   "process id is also defined in resource " + other,
			})
			continue
		}
		seen[c.BpmnProcessID] = c.ResourceName
	}
	return out
}
