package domain

import (
	"errors"
	"fmt"
	"sort"

	"github.com/indr622/workflow-engine/libs/go/bpmn"
)

// SnapshotFormatVersion adalah versi format Snapshot. Naikkan setiap kali InstanceState berubah
// bentuk (mis. Fase 5 menambah timer); snapshot versi lain diabaikan dan rebuild mulai dari nol.
const SnapshotFormatVersion = 1

// ErrSnapshotFormat menandai snapshot dengan versi format yang tidak dikenal.
var ErrSnapshotFormat = errors.New("unsupported snapshot format version")

// Snapshot adalah state runtime instance setelah record ke-Sequence diterapkan (ADR-0019).
type Snapshot struct {
	FormatVersion int               `json:"formatVersion"`
	Sequence      int32             `json:"sequence"`
	Key           string            `json:"key"`
	Definition    DefinitionRef     `json:"definition"`
	Status        InstanceStatus    `json:"status"`
	Elements      []ElementInstance `json:"elements"`
	TakenFlows    []TakenFlow       `json:"takenFlows"`
	Variables     []Variable        `json:"variables"`
	Jobs          []Job             `json:"jobs"`
	Incidents     []Incident        `json:"incidents"`
}

// Snapshot mengambil state runtime dalam urutan deterministik.
func (s *InstanceState) Snapshot(seq int32) Snapshot {
	snap := Snapshot{
		FormatVersion: SnapshotFormatVersion, Sequence: seq, Key: s.Key, Definition: s.Definition, Status: s.Status,
		TakenFlows: s.SortedTakenFlows(), Variables: s.SortedVariables(),
	}
	for _, e := range s.Elements {
		snap.Elements = append(snap.Elements, *e)
	}
	sort.Slice(snap.Elements, func(i, j int) bool { return snap.Elements[i].Key < snap.Elements[j].Key })
	for _, j := range s.Jobs {
		snap.Jobs = append(snap.Jobs, *j)
	}
	sort.Slice(snap.Jobs, func(i, j int) bool { return snap.Jobs[i].Key < snap.Jobs[j].Key })
	for _, inc := range s.Incidents {
		snap.Incidents = append(snap.Incidents, *inc)
	}
	sort.Slice(snap.Incidents, func(i, j int) bool { return snap.Incidents[i].Key < snap.Incidents[j].Key })
	return snap
}

// RestoreSnapshot membangun InstanceState dari snapshot; versi format lain → ErrSnapshotFormat.
func RestoreSnapshot(proc *bpmn.Process, snap *Snapshot) (*InstanceState, error) {
	if snap.FormatVersion != SnapshotFormatVersion {
		return nil, fmt.Errorf("%w: %d (want %d)", ErrSnapshotFormat, snap.FormatVersion, SnapshotFormatVersion)
	}
	return Restore(&InstanceSnapshot{
		Key: snap.Key, Definition: snap.Definition, Status: snap.Status, Elements: snap.Elements,
		TakenFlows: snap.TakenFlows, Variables: snap.Variables, Jobs: snap.Jobs, Incidents: snap.Incidents,
	}, proc), nil
}
