package bpmn

// Profile menentukan elemen mana yang diterima Validate.
type Profile struct {
	name    string
	accepts func(FlowElement) bool
	// rejectIOMapping menolak zeebe:ioMapping karena engine belum mengevaluasinya (Fase 7).
	rejectIOMapping bool
	// explain memberi pesan penolakan yang lebih spesifik; kosong berarti pesan umum.
	explain func(FlowElement) string
}

func (p Profile) explainRejection(el FlowElement) string {
	if p.explain == nil {
		return ""
	}
	return p.explain(el)
}

// Name adalah nama profil ("full" atau "executable").
func (p Profile) Name() string { return p.name }

// ProfileFull menerima semua elemen yang dimodelkan parser. Dipakai lint modeler dan uji paritas.
var ProfileFull = Profile{name: "full", accepts: func(FlowElement) bool { return true }}

// ProfileExecutable hanya menerima elemen yang sudah bisa dieksekusi engine.
// Daftarnya tumbuh setiap fase; Fase 2: none start/end, sequence flow, service task,
// undefined/manual task, exclusive/parallel/inclusive gateway. Fase 4: user task berbasis job
// worker (tanpa zeebe:userTask).
var ProfileExecutable = Profile{name: "executable", accepts: executableInPhase4, rejectIOMapping: true, explain: explainExecutable}

func explainExecutable(el FlowElement) string {
	if a, ok := el.(*Activity); ok && a.typ == UserTaskType && a.userTask.ZeebeUserTask {
		return "native user task (zeebe:userTask) is supported from phase 6; remove zeebe:userTask to run it as a job-based user task"
	}
	return ""
}

func executableInPhase4(el FlowElement) bool {
	switch x := el.(type) {
	case *SequenceFlow:
		return true
	case *Gateway:
		return x.typ == ExclusiveGatewayType || x.typ == ParallelGatewayType || x.typ == InclusiveGatewayType
	case *Activity:
		if x.multiInstance != nil {
			return false
		}
		if x.typ == UserTaskType {
			return !x.userTask.ZeebeUserTask
		}
		return x.typ == TaskType || x.typ == ManualTaskType || x.typ == ServiceTaskType
	case *Event:
		if x.definition.kind != NoneEventDefinition || x.definitionCount > 0 {
			return false
		}
		if x.typ == EndEventType {
			return true
		}
		_, inSubProcess := x.scope.Owner()
		return x.typ == StartEventType && !inSubProcess
	default:
		return false
	}
}
