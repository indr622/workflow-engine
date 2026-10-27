package bpmn

// Activity adalah task, sub-process, event sub-process, atau call activity.
type Activity struct {
	node
	taskDefinition TaskDefinition
	headers        []Header
	userTask       UserTaskDefinition
	calledElement  CalledElement
	calledDecision *CalledDecision
	script         *Script
	multiInstance  *MultiInstance
	messageRef     string
	message        *Message
	boundaryEvents []*Event
	inner          *Scope
	extensions     []RawExtension
}

// TaskDefinition adalah zeebe:taskDefinition (task berbasis job).
func (a *Activity) TaskDefinition() TaskDefinition { return a.taskDefinition }

// Headers adalah zeebe:taskHeaders, urutan dokumen dipertahankan.
func (a *Activity) Headers() []Header { return a.headers }

// UserTask adalah ekstensi zeebe untuk user task.
func (a *Activity) UserTask() UserTaskDefinition { return a.userTask }

// CalledElement adalah zeebe:calledElement untuk call activity.
func (a *Activity) CalledElement() CalledElement { return a.calledElement }

// CalledDecision adalah zeebe:calledDecision untuk business rule task.
func (a *Activity) CalledDecision() (*CalledDecision, bool) {
	return a.calledDecision, a.calledDecision != nil
}

// Script adalah zeebe:script untuk script task berbasis FEEL.
func (a *Activity) Script() (*Script, bool) { return a.script, a.script != nil }

// MultiInstance mengembalikan karakteristik multi-instance bila ada.
func (a *Activity) MultiInstance() (*MultiInstance, bool) {
	return a.multiInstance, a.multiInstance != nil
}

// MessageRef adalah messageRef mentah untuk receive task.
func (a *Activity) MessageRef() string { return a.messageRef }

// Message adalah message yang dirujuk receive task.
func (a *Activity) Message() (*Message, bool) { return a.message, a.message != nil }

// BoundaryEvents adalah boundary event yang menempel di activity ini.
func (a *Activity) BoundaryEvents() []*Event { return a.boundaryEvents }

// Inner adalah scope di dalam sub-process atau event sub-process; nil untuk task.
func (a *Activity) Inner() *Scope { return a.inner }

// Extensions adalah ekstensi zeebe yang belum dimodelkan.
func (a *Activity) Extensions() []RawExtension { return a.extensions }

// IsJobBased mengembalikan true jika activity dikerjakan job worker.
func (a *Activity) IsJobBased() bool {
	switch a.typ {
	case ServiceTaskType, SendTaskType:
		return true
	case ScriptTaskType:
		return a.script == nil
	case BusinessRuleTaskType:
		return a.calledDecision == nil
	default:
		return false
	}
}
