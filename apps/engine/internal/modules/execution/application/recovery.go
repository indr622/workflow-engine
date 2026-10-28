package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"sort"
	"strings"
	"time"

	deploymentapi "github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/domain"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/recovery"
)

// Mismatch adalah ketidakcocokan proyeksi satu instance (Subject = process instance key).
type Mismatch = recovery.Mismatch

// InstanceReading adalah record dan state runtime proyeksi satu instance dari snapshot konsisten.
type InstanceReading struct {
	Snapshot     *domain.InstanceSnapshot
	Records      []domain.Record
	LastSequence int32
}

// RebuildScope adalah operasi transaksional untuk rebuild proyeksi.
type RebuildScope interface {
	TxScope
	// LoadRecords mengembalikan record dengan sequence > after, urut sequence.
	LoadRecords(ctx context.Context, key string, after int32) ([]domain.Record, error)
	LoadSnapshot(ctx context.Context, key string) (*domain.Snapshot, bool, error)
	ResetProjection(ctx context.Context, key string) error
	Reproject(ctx context.Context, records []domain.Record, st *domain.InstanceState, lastSequence int32, lastAt time.Time) error
}

// RecoveryStore adalah port baca dan tulis untuk verify dan rebuild (ADR-0016).
type RecoveryStore interface {
	UnitOfWork
	QuickVerify(ctx context.Context) ([]Mismatch, error)
	DeepVerifyCandidates(ctx context.Context, window time.Duration, sample int) ([]string, error)
	InstanceKeys(ctx context.Context, afterKey string, limit int) ([]string, error)
	ReadInstance(ctx context.Context, key string) (InstanceReading, error)
}

// Recovery berisi use case verify dan rebuild proyeksi dari execution_record.
type Recovery struct {
	store  RecoveryStore
	defs   deploymentapi.ProcessDefinitions
	log    *slog.Logger
	window time.Duration
	sample int
}

// RecoveryConfig mengatur verifikasi dalam saat start.
type RecoveryConfig struct {
	DeepWindow time.Duration // default 10 m
	DeepSample int           // default 100
}

// NewRecovery membuat use case pemulihan.
func NewRecovery(store RecoveryStore, defs deploymentapi.ProcessDefinitions, log *slog.Logger, cfg RecoveryConfig) *Recovery {
	if cfg.DeepWindow <= 0 {
		cfg.DeepWindow = 10 * time.Minute
	}
	if cfg.DeepSample <= 0 {
		cfg.DeepSample = 100
	}
	if log == nil {
		log = slog.Default()
	}
	return &Recovery{store: store, defs: defs, log: log, window: cfg.DeepWindow, sample: cfg.DeepSample}
}

var _ recovery.Verifier = (*Recovery)(nil)

// Name memenuhi recovery.Verifier.
func (r *Recovery) Name() string { return "execution" }

// VerifyOnStart: pemeriksaan cepat atas semua instance, lalu pemeriksaan dalam atas instance aktif
// yang berubah dalam jendela terakhir ditambah sampel acak.
func (r *Recovery) VerifyOnStart(ctx context.Context) (recovery.Report, error) {
	started := time.Now()
	rep := recovery.Report{}
	quick, err := r.store.QuickVerify(ctx)
	if err != nil {
		return rep, err
	}
	rep.Mismatches = append(rep.Mismatches, quick...)
	keys, err := r.store.DeepVerifyCandidates(ctx, r.window, r.sample)
	if err != nil {
		return rep, err
	}
	for _, k := range keys {
		m, err := r.DeepVerify(ctx, k)
		if err != nil {
			return rep, err
		}
		rep.Mismatches = append(rep.Mismatches, m...)
	}
	rep.Checked = len(keys)
	rep.Duration = time.Since(started)
	return rep, nil
}

// VerifyAll menjalankan pemeriksaan cepat lalu pemeriksaan dalam untuk setiap instance.
func (r *Recovery) VerifyAll(ctx context.Context) (recovery.Report, error) {
	started := time.Now()
	rep := recovery.Report{}
	quick, err := r.store.QuickVerify(ctx)
	if err != nil {
		return rep, err
	}
	rep.Mismatches = append(rep.Mismatches, quick...)
	after := ""
	for {
		keys, err := r.store.InstanceKeys(ctx, after, 500)
		if err != nil {
			return rep, err
		}
		if len(keys) == 0 {
			break
		}
		for _, k := range keys {
			m, err := r.DeepVerify(ctx, k)
			if err != nil {
				return rep, err
			}
			rep.Mismatches = append(rep.Mismatches, m...)
			rep.Checked++
		}
		after = keys[len(keys)-1]
	}
	rep.Duration = time.Since(started)
	return rep, nil
}

// QuickVerifyFor menjalankan pemeriksaan cepat dan mengembalikan ketidakcocokan untuk key tertentu saja.
func (r *Recovery) QuickVerifyFor(ctx context.Context, keys []string) ([]Mismatch, error) {
	all, err := r.store.QuickVerify(ctx)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	var out []Mismatch
	for _, m := range all {
		if want[m.Subject] {
			out = append(out, m)
		}
	}
	return out, nil
}

// DeepVerify menerapkan ulang seluruh record instance ke state kosong lalu membandingkannya dengan
// state runtime yang dimuat dari proyeksi.
func (r *Recovery) DeepVerify(ctx context.Context, key string) ([]Mismatch, error) {
	reading, err := r.store.ReadInstance(ctx, key)
	if err != nil {
		return nil, err
	}
	_, proc, err := r.defs.GetByKey(ctx, reading.Snapshot.Definition.Key)
	if err != nil {
		return nil, fmt.Errorf("load definition of %s: %w", key, err)
	}
	replayed := domain.NewInstanceState(domain.DefinitionRef{}, proc)
	for i := range reading.Records {
		if err := domain.Apply(replayed, &reading.Records[i]); err != nil {
			return []Mismatch{{Subject: key, Kind: "replay", Detail: fmt.Sprintf("record %d: %v", i+1, err)}}, nil
		}
	}
	projected := domain.Restore(reading.Snapshot, proc)
	return compareStates(key, replayed, projected), nil
}

// RebuildResult meringkas satu rebuild.
type RebuildResult struct {
	ProcessInstanceKey string
	Records            int
	// FromSnapshot bernilai true bila rebuild mulai dari snapshot (hanya record sesudahnya).
	FromSnapshot bool
}

// RebuildInstance membangun ulang proyeksi instance dari record di bawah row lock. full = true
// menghapus proyeksi lalu memutar ulang semua record (memperbaiki kerusakan di mana pun). full =
// false memulai dari snapshot bila ada dan hanya memproyeksikan ulang record sesudahnya di atas
// proyeksi yang ada (proyektor idempoten); cocok untuk instance berloop panjang yang proyeksinya
// sebelum snapshot masih utuh. Rebuild tidak menulis outbox: rebuild bukan eksekusi ulang.
func (r *Recovery) RebuildInstance(ctx context.Context, key string, full bool) (RebuildResult, error) {
	defKey, err := r.store.DefinitionKeyOf(ctx, key)
	if err != nil {
		return RebuildResult{}, err
	}
	_, proc, err := r.defs.GetByKey(ctx, defKey)
	if err != nil {
		return RebuildResult{}, fmt.Errorf("load definition %s: %w", defKey, err)
	}
	res := RebuildResult{ProcessInstanceKey: key}
	err = r.store.Do(ctx, func(ctx context.Context, tx TxScope) error {
		scope, ok := tx.(RebuildScope)
		if !ok {
			return fmt.Errorf("unit of work does not support rebuild")
		}
		if _, err := scope.LockAndLoad(ctx, key); err != nil {
			return err
		}
		st := domain.NewInstanceState(domain.DefinitionRef{}, proc)
		from := int32(0)
		if !full {
			snap, ok, err := scope.LoadSnapshot(ctx, key)
			if err != nil {
				return err
			}
			if ok {
				restored, err := domain.RestoreSnapshot(proc, snap)
				switch {
				case err == nil:
					st, from, res.FromSnapshot = restored, snap.Sequence, true
				case errors.Is(err, domain.ErrSnapshotFormat):
					r.log.Warn("snapshot format not supported, rebuilding from the first record",
						slog.String("process_instance_key", key), slog.Any("error", err))
				default:
					return err
				}
			}
		}
		records, err := scope.LoadRecords(ctx, key, from)
		if err != nil {
			return err
		}
		for i := range records {
			if err := domain.Apply(st, &records[i]); err != nil {
				return fmt.Errorf("replay record %d of %s: %w", int(from)+i+1, key, err)
			}
		}
		res.Records = len(records)
		if !res.FromSnapshot {
			if err := scope.ResetProjection(ctx, key); err != nil {
				return err
			}
		}
		if len(records) == 0 {
			return nil
		}
		last := from + int32(len(records)) //nolint:gosec // jumlah record per instance < 2^31
		return scope.Reproject(ctx, records, st, last, records[len(records)-1].Timestamp)
	})
	if err != nil {
		return res, fmt.Errorf("rebuild %s: %w", key, err)
	}
	r.log.Info("instance rebuilt", slog.String("process_instance_key", key), slog.Int("records", res.Records))
	return res, nil
}

// Repair memenuhi recovery.Verifier: rebuild setiap instance yang menyimpang.
func (r *Recovery) Repair(ctx context.Context, subjects []string) error {
	for _, k := range subjects {
		if _, err := r.RebuildInstance(ctx, k, true); err != nil {
			return err
		}
	}
	return nil
}

// RebuildAll membangun ulang semua instance berurutan key.
func (r *Recovery) RebuildAll(ctx context.Context, full bool, progress func(RebuildResult)) (int, error) {
	after, n := "", 0
	for {
		keys, err := r.store.InstanceKeys(ctx, after, 500)
		if err != nil {
			return n, err
		}
		if len(keys) == 0 {
			return n, nil
		}
		for _, k := range keys {
			res, err := r.RebuildInstance(ctx, k, full)
			if err != nil {
				return n, err
			}
			n++
			if progress != nil {
				progress(res)
			}
		}
		after = keys[len(keys)-1]
	}
}

// stateView adalah bentuk state yang dibandingkan: deterministik dan netral terhadap nil vs kosong.
type stateView struct {
	Status     domain.InstanceStatus
	Definition domain.DefinitionRef
	Elements   []domain.ElementInstance
	TakenFlows []domain.TakenFlow
	Variables  map[string]string
	Jobs       []domain.Job
	Incidents  []domain.Incident
}

func viewOf(s *domain.InstanceState) stateView {
	v := stateView{Status: s.Status, Definition: s.Definition, TakenFlows: s.SortedTakenFlows(), Variables: map[string]string{}}
	for _, e := range s.Elements {
		v.Elements = append(v.Elements, *e)
	}
	sort.Slice(v.Elements, func(i, j int) bool { return v.Elements[i].Key < v.Elements[j].Key })
	for _, vr := range s.Variables {
		v.Variables[vr.ScopeKey+"/"+vr.Name] = vr.Key + "=" + string(vr.Value)
	}
	for _, j := range s.Jobs {
		c := *j
		if len(c.CustomHeaders) == 0 {
			c.CustomHeaders = nil
		}
		v.Jobs = append(v.Jobs, c)
	}
	sort.Slice(v.Jobs, func(i, j int) bool { return v.Jobs[i].Key < v.Jobs[j].Key })
	for _, inc := range s.Incidents {
		v.Incidents = append(v.Incidents, *inc)
	}
	sort.Slice(v.Incidents, func(i, j int) bool { return v.Incidents[i].Key < v.Incidents[j].Key })
	if len(v.TakenFlows) == 0 {
		v.TakenFlows = nil
	}
	return v
}

// compareStates membandingkan state hasil replay dengan state proyeksi, per bagian.
func compareStates(key string, replayed, projected *domain.InstanceState) []Mismatch {
	a, b := viewOf(replayed), viewOf(projected)
	var out []Mismatch
	check := func(kind string, x, y any) {
		if !reflect.DeepEqual(x, y) {
			out = append(out, Mismatch{Subject: key, Kind: kind, Detail: diffSummary(x, y)})
		}
	}
	check("status", a.Status, b.Status)
	check("definition", a.Definition, b.Definition)
	check("element_instances", a.Elements, b.Elements)
	check("taken_flows", a.TakenFlows, b.TakenFlows)
	check("variables", a.Variables, b.Variables)
	check("jobs", a.Jobs, b.Jobs)
	check("incidents", a.Incidents, b.Incidents)
	return out
}

func diffSummary(replayed, projected any) string {
	ra, _ := json.Marshal(replayed)
	pb, _ := json.Marshal(projected)
	trim := func(b []byte) string {
		s := string(b)
		if len(s) > 300 {
			s = s[:300] + "…"
		}
		return strings.TrimSpace(s)
	}
	return "replayed=" + trim(ra) + " projected=" + trim(pb)
}
