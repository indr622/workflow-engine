package bpmn

// Profile menentukan elemen mana yang diterima Validate.
type Profile struct {
	name    string
	accepts func(FlowElement) bool
	// rejectIOMapping menolak zeebe:ioMapping karena engine belum mengevaluasinya (Fase 7).
	rejectIOMapping bool
}

// Name adalah nama profil ("full" atau "executable").
func (p Profile) Name() string { return p.name }

// ProfileFull menerima semua elemen yang dimodelkan parser. Dipakai lint modeler dan uji paritas.
var ProfileFull = Profile{name: "full", accepts: func(FlowElement) bool { return true }}

// ProfileExecutable hanya menerima elemen yang sudah bisa dieksekusi engine.
// Daftarnya tumbuh setiap fase; Fase 2: none start/end, sequence flow, service task,
// undefined/manual task, exclusive/parallel/inclusive gateway.
var ProfileExecutable = Profile{name: "executable", accepts: executableInPhase2, rejectIOMapping: true}

func executableInPhase2(el FlowElement) bool {
	switch x := el.(type) {
	case *SequenceFlow:
		return true
	case *Gateway:
		return x.typ == ExclusiveGatewayType || x.typ == ParallelGatewayType || x.typ == InclusiveGatewayType
	case *Activity:
		if x.multiInstance != nil {
			return false
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
