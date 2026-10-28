package domain_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/domain"
)

// TestSnapshotAtEveryRecordMatchesFullReplay: snapshot yang diambil setelah record ke-k, melewati
// JSON, lalu ditambah record sisanya, selalu sama dengan replay dari nol.
func TestSnapshotAtEveryRecordMatchesFullReplay(t *testing.T) {
	proc := newModel("snap").
		node("start", "startEvent").gateway("merge", "exclusiveGateway", "").gateway("fork", "parallelGateway", "").
		service("a", "a", "").service("b", "b", "").gateway("join", "parallelGateway", "").
		gateway("check", "exclusiveGateway", "").node("end", "endEvent").
		flow("f1", "start", "merge", "").flow("f2", "merge", "fork", "").flow("fa", "fork", "a", "").flow("fb", "fork", "b", "").
		flow("ja", "a", "join", "").flow("jb", "b", "join", "").flow("f3", "join", "check", "").
		flow("again", "check", "merge", "=i < 3").flow("done", "check", "end", "=i >= 3").
		build(t)
	h := startInstance(t, proc, `{"i": 0, "doc": {"z": 1, "a": [1, 2]}}`)
	for i := 1; i <= 3; i++ {
		h.complete("a", `{}`)
		h.complete("b", `{"i": `+string(rune('0'+i))+`}`)
	}
	h.expectStatus(domain.InstanceCompleted)

	full := domain.NewInstanceState(domain.DefinitionRef{}, proc)
	for k := 0; k <= len(h.all); k++ {
		if k > 0 {
			if err := domain.Apply(full, &h.all[k-1]); err != nil {
				t.Fatal(err)
			}
		}
		data, err := json.Marshal(full.Snapshot(int32(k)))
		if err != nil {
			t.Fatal(err)
		}
		var snap domain.Snapshot
		if err := json.Unmarshal(data, &snap); err != nil {
			t.Fatal(err)
		}
		restored, err := domain.RestoreSnapshot(proc, &snap)
		if err != nil {
			t.Fatal(err)
		}
		for i := k; i < len(h.all); i++ {
			if err := domain.Apply(restored, &h.all[i]); err != nil {
				t.Fatalf("snapshot at %d, record %d: %v", k, i, err)
			}
		}
		if !reflect.DeepEqual(normalize(restored), normalize(h.st)) {
			t.Fatalf("snapshot at %d then tail differs from the live state", k)
		}
	}
}

func TestRestoreSnapshotRejectsUnknownFormat(t *testing.T) {
	proc := newModel("v").node("start", "startEvent").node("end", "endEvent").flow("f", "start", "end", "").build(t)
	_, err := domain.RestoreSnapshot(proc, &domain.Snapshot{FormatVersion: 99})
	if !errors.Is(err, domain.ErrSnapshotFormat) {
		t.Fatalf("error = %v", err)
	}
}
