//go:build integration

package infrastructure_test

import (
	"context"
	"testing"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/application"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/domain"
)

// TestProjectorIsIdempotent: memproyeksikan ulang seluruh record di atas proyeksi yang sudah ada
// (dua kali) tidak mengubah satu baris pun. Instance uji mencakup semua (ValueType, Intent): elemen,
// sequence flow, variabel CREATED/UPDATED, job CREATED/COMPLETED/CANCELED, incident CREATED/RESOLVED.
func TestProjectorIsIdempotent(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	keys := recoveryInstances(t, f)
	if err := cancelInstance(ctx, f, keys[2]); err != nil { // incident RESOLVED lewat cancel
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, key := range keys {
		before := dumpProjection(t, f.pool, key)
		for pass := 0; pass < 2; pass++ {
			err := f.repo.Do(ctx, func(ctx context.Context, tx application.TxScope) error {
				scope := tx.(application.RebuildScope)
				snap, err := scope.LockAndLoad(ctx, key)
				if err != nil {
					return err
				}
				_, proc, err := f.cache.GetByKey(ctx, snap.Definition.Key)
				if err != nil {
					return err
				}
				records, err := scope.LoadRecords(ctx, key, 0)
				if err != nil {
					return err
				}
				st := domain.NewInstanceState(domain.DefinitionRef{}, proc)
				for i := range records {
					seen[string(records[i].ValueType)+" "+string(records[i].Intent)] = true
					if err := domain.Apply(st, &records[i]); err != nil {
						return err
					}
				}
				return scope.Reproject(ctx, records, st, int32(len(records)), records[len(records)-1].Timestamp)
			})
			if err != nil {
				t.Fatal(err)
			}
			if after := dumpProjection(t, f.pool, key); after != before {
				t.Fatalf("pass %d changed the projection of %s\nbefore:\n%s\nafter:\n%s", pass, key, before, after)
			}
		}
	}
	for _, want := range []string{
		"PROCESS_INSTANCE ELEMENT_ACTIVATING", "PROCESS_INSTANCE ELEMENT_COMPLETED", "PROCESS_INSTANCE ELEMENT_TERMINATED",
		"PROCESS_INSTANCE SEQUENCE_FLOW_TAKEN", "PROCESS_INSTANCE_CREATION CREATED", "VARIABLE CREATED", "VARIABLE UPDATED",
		"JOB CREATED", "JOB COMPLETED", "JOB CANCELED", "INCIDENT CREATED", "INCIDENT RESOLVED",
	} {
		if !seen[want] {
			t.Errorf("test instances never produced %s", want)
		}
	}
}
