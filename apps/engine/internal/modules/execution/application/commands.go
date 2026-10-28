// Package application berisi use case modul execution: menjalankan command terhadap process
// instance dalam satu transaksi (kunci → muat → Processor → tulis record + proyeksi → commit →
// publikasi event) dan query atas proyeksi.
package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	deploymentapi "github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/domain"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/eventbus"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/faultpoint"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/idempotency"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/metrics"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/outbox"
	"github.com/indr622/workflow-engine/libs/go/bpmn"
)

// DefaultMaxVariablesBytes adalah ukuran maksimum dokumen variabel per request.
const DefaultMaxVariablesBytes = 1 << 20

// Config mengatur batas use case.
type Config struct {
	MaxVariablesBytes int
}

// Commands mengimplementasikan api.ProcessInstances dan api.Jobs.
type Commands struct {
	uow       UnitOfWork
	defs      deploymentapi.ProcessDefinitions
	processor *domain.Processor
	keys      domain.KeyGenerator
	bus       eventbus.Bus
	log       *slog.Logger
	cfg       Config
	waiter    *waiter
}

var (
	_ api.ProcessInstances = (*Commands)(nil)
	_ api.Jobs             = (*Commands)(nil)
)

// NewCommands merakit use case command.
func NewCommands(uow UnitOfWork, defs deploymentapi.ProcessDefinitions, processor *domain.Processor,
	keys domain.KeyGenerator, bus eventbus.Bus, log *slog.Logger, cfg Config) *Commands {
	if cfg.MaxVariablesBytes <= 0 {
		cfg.MaxVariablesBytes = DefaultMaxVariablesBytes
	}
	if log == nil {
		log = slog.Default()
	}
	return &Commands{uow: uow, defs: defs, processor: processor, keys: keys, bus: bus, log: log, cfg: cfg, waiter: newWaiter(bus)}
}

// ErrDefinitionRequired dikembalikan bila request tidak memilih process definition.
var ErrDefinitionRequired = errors.New("process definition key or bpmn process id is required")

// CreateProcessInstance memilih versi proses lalu menjalankan instance baru sampai wait state.
func (c *Commands) CreateProcessInstance(ctx context.Context, req api.CreateInstanceRequest) (api.CreateInstanceResult, error) {
	defer metrics.Since(ctx, metrics.Get().CommandDuration, time.Now(), metrics.Attr("command", "create_process_instance"))
	def, proc, err := c.resolve(ctx, req)
	if err != nil {
		return api.CreateInstanceResult{}, err
	}
	vars, err := domain.ParseVariables(req.Variables, c.cfg.MaxVariablesBytes)
	if err != nil {
		return api.CreateInstanceResult{}, err
	}
	ref := domain.DefinitionRef{Key: def.Key, BpmnProcessID: def.BpmnProcessID, Version: def.Version}
	var st *domain.InstanceState
	var records []domain.Record
	var result api.CreateInstanceResult
	err = c.uow.Do(ctx, func(ctx context.Context, tx TxScope) error {
		// Instance baru belum punya aggregate untuk dikunci, jadi klaim dilakukan di awal.
		claim, replay, err := c.begin(ctx, tx, req.Idempotency)
		if err != nil {
			return err
		}
		if replay != nil {
			if err := json.Unmarshal(replay, &result); err != nil {
				return fmt.Errorf("decode stored response: %w", err)
			}
			result.Replayed = true
			return nil
		}
		st = domain.NewInstanceState(ref, proc)
		records, err = c.processor.Process(st, domain.CreateInstance{Variables: vars}, c.keys)
		if err != nil {
			return err
		}
		result = api.CreateInstanceResult{
			ProcessInstanceKey: st.Key, ProcessDefinitionKey: def.Key, BpmnProcessID: def.BpmnProcessID,
			Version: def.Version, State: string(st.Status),
		}
		if claim != nil {
			stored, err := json.Marshal(result)
			if err != nil {
				return err
			}
			tx.StoreIdempotent(*claim, stored)
		}
		tx.EnqueueForRecords(outboxEvents(st, records))
		if err := tx.AppendAndProject(ctx, records, st); err != nil {
			return err
		}
		return faultpoint.Hit(ctx, faultpoint.CommandAfterAppend)
	})
	if err != nil {
		return api.CreateInstanceResult{}, fmt.Errorf("create process instance: %w", err)
	}
	if err := faultpoint.Hit(ctx, faultpoint.CommandAfterCommit); err != nil {
		return result, err
	}
	if !result.Replayed {
		c.publish(ctx, st, records)
	}
	return result, nil
}

// begin mengklaim request_id bila ada; replay berisi response tersimpan bila klaim sudah pernah
// dijalankan.
func (c *Commands) begin(ctx context.Context, tx TxScope, idem *api.Idempotency) (*idempotency.Claim, []byte, error) {
	if idem == nil {
		return nil, nil, nil
	}
	claim := idempotency.Claim{Scope: idem.Scope, Key: idem.Key, RequestHash: idem.RequestHash}
	res, err := tx.BeginIdempotent(ctx, claim)
	if err != nil {
		return nil, nil, err
	}
	if res.Outcome == idempotency.Replayed {
		c.log.Info("idempotent replay", slog.String("scope", idem.Scope), slog.String("request_id", idem.Key))
		return nil, res.Response, nil
	}
	return &claim, nil, nil
}

func (c *Commands) resolve(ctx context.Context, req api.CreateInstanceRequest) (deploymentapi.ProcessDefinition, *bpmn.Process, error) {
	switch {
	case req.DefinitionKey != "":
		return c.defs.GetByKey(ctx, req.DefinitionKey)
	case req.BpmnProcessID != "" && req.Version > 0:
		return c.defs.GetByVersion(ctx, req.BpmnProcessID, req.Version)
	case req.BpmnProcessID != "":
		return c.defs.GetLatest(ctx, req.BpmnProcessID)
	default:
		return deploymentapi.ProcessDefinition{}, nil, ErrDefinitionRequired
	}
}

// CompleteJob menyelesaikan job terbuka dan melanjutkan token instance pemiliknya.
func (c *Commands) CompleteJob(ctx context.Context, jobKey string, variables []byte, idem *api.Idempotency) (bool, error) {
	vars, err := domain.ParseVariables(variables, c.cfg.MaxVariablesBytes)
	if err != nil {
		return false, err
	}
	piKey, defKey, err := c.uow.JobRef(ctx, jobKey)
	if err != nil {
		return false, fmt.Errorf("complete job: %w", err)
	}
	proc, err := c.process(ctx, defKey)
	if err != nil {
		return false, fmt.Errorf("complete job: %w", err)
	}
	return c.runOnInstance(ctx, "complete job", piKey, proc, idem, domain.CompleteJob{JobKey: jobKey, Variables: vars})
}

// runOnInstance menjalankan command pada instance yang sudah ada: kunci + muat, klaim request_id
// (setelah lock), Processor, lalu tulis record, proyeksi, outbox, dan response dalam satu batch.
func (c *Commands) runOnInstance(ctx context.Context, op, piKey string, proc *bpmn.Process, idem *api.Idempotency, cmd domain.Command) (bool, error) {
	defer metrics.Since(ctx, metrics.Get().CommandDuration, time.Now(), metrics.Attr("command", op))
	var st *domain.InstanceState
	var records []domain.Record
	replayed := false
	err := c.uow.Do(ctx, func(ctx context.Context, tx TxScope) error {
		var err error
		lockStart := time.Now()
		st, err = lockAndLoad(ctx, tx, piKey, proc)
		metrics.Since(ctx, metrics.Get().CommandLockWait, lockStart, metrics.Attr("command", op))
		if err != nil {
			return err
		}
		if err := faultpoint.Hit(ctx, faultpoint.CommandAfterLock); err != nil {
			return err
		}
		claim, replay, err := c.begin(ctx, tx, idem)
		if err != nil {
			return err
		}
		if replay != nil {
			replayed = true
			return nil
		}
		records, err = c.processor.Process(st, cmd, c.keys)
		if err != nil {
			return err
		}
		if claim != nil {
			tx.StoreIdempotent(*claim, []byte{})
		}
		tx.EnqueueForRecords(outboxEvents(st, records))
		if err := tx.AppendAndProject(ctx, records, st); err != nil {
			return err
		}
		return faultpoint.Hit(ctx, faultpoint.CommandAfterAppend)
	})
	if err != nil {
		return false, fmt.Errorf("%s: %w", op, err)
	}
	if err := faultpoint.Hit(ctx, faultpoint.CommandAfterCommit); err != nil {
		return replayed, err
	}
	if !replayed {
		c.publish(ctx, st, records)
	}
	return replayed, nil
}

// CancelProcessInstance menghentikan instance aktif.
func (c *Commands) CancelProcessInstance(ctx context.Context, processInstanceKey string, idem *api.Idempotency) (bool, error) {
	proc, err := c.processOf(ctx, processInstanceKey)
	if err != nil {
		return false, fmt.Errorf("cancel process instance: %w", err)
	}
	return c.runOnInstance(ctx, "cancel process instance", processInstanceKey, proc, idem, domain.CancelInstance{})
}

// processOf memuat model proses sebuah instance SEBELUM transaksi dibuka. Memuatnya di dalam
// transaksi (sambil memegang row lock) bisa meminta koneksi kedua dari pool; pada banyak command
// paralel untuk satu instance, pool habis dan semua transaksi saling menunggu.
func (c *Commands) processOf(ctx context.Context, instanceKey string) (*bpmn.Process, error) {
	defKey, err := c.uow.DefinitionKeyOf(ctx, instanceKey)
	if err != nil {
		return nil, err
	}
	return c.process(ctx, defKey)
}

func (c *Commands) process(ctx context.Context, defKey string) (*bpmn.Process, error) {
	_, proc, err := c.defs.GetByKey(ctx, defKey)
	if err != nil {
		return nil, fmt.Errorf("load process definition %s: %w", defKey, err)
	}
	return proc, nil
}

// lockAndLoad mengunci instance lalu memuat state runtime-nya.
func lockAndLoad(ctx context.Context, tx domain.TxRepository, key string, proc *bpmn.Process) (*domain.InstanceState, error) {
	snap, err := tx.LockAndLoad(ctx, key)
	if err != nil {
		return nil, err
	}
	return domain.Restore(snap, proc), nil
}

// publish mengirim event domain setelah commit. Kegagalan handler hanya dicatat; outbox
// transaksional menyusul di Fase 3.
func (c *Commands) publish(ctx context.Context, st *domain.InstanceState, records []domain.Record) {
	for i := range records {
		ev := eventOf(st, &records[i])
		if ev == nil {
			continue
		}
		if err := c.bus.Publish(ctx, ev); err != nil {
			c.log.Error("publish execution event", slog.String("topic", ev.Topic()),
				slog.String("process_instance_key", st.Key), slog.Any("error", err))
		}
	}
}

func eventOf(st *domain.InstanceState, r *domain.Record) eventbus.Event {
	switch v := r.Value.(type) {
	case *domain.ProcessInstanceValue:
		if v.BpmnElementType != domain.ElementTypeProcess {
			return nil
		}
		switch r.Intent {
		case domain.IntentElementCompleted:
			return api.ProcessInstanceCompleted{ProcessInstanceKey: r.Key, ProcessDefinitionKey: v.ProcessDefinitionKey, BpmnProcessID: v.BpmnProcessID}
		case domain.IntentElementTerminated:
			return api.ProcessInstanceTerminated{ProcessInstanceKey: r.Key, ProcessDefinitionKey: v.ProcessDefinitionKey, BpmnProcessID: v.BpmnProcessID}
		}
	case *domain.JobValue:
		if r.Intent == domain.IntentCreated {
			return api.JobCreated{JobKey: r.Key, JobType: v.Type, ProcessInstanceKey: st.Key, ElementInstanceKey: v.ElementInstanceKey, ElementID: v.ElementID}
		}
	case *domain.IncidentValue:
		if r.Intent == domain.IntentCreated {
			return api.IncidentCreated{IncidentKey: r.Key, ErrorType: string(v.ErrorType), ProcessInstanceKey: st.Key, ElementInstanceKey: v.ElementInstanceKey, ElementID: v.ElementID}
		}
	}
	return nil
}

// outboxEvents menurunkan event durable dari record command (ADR-0015). Payload adalah JSON struct
// event api sehingga consumer tidak bergantung pada format record internal.
func outboxEvents(st *domain.InstanceState, records []domain.Record) []RecordEvent {
	var out []RecordEvent
	for i := range records {
		ev := eventOf(st, &records[i])
		if ev == nil {
			continue
		}
		payload, err := json.Marshal(ev)
		if err != nil {
			continue // struct event api selalu bisa di-marshal
		}
		out = append(out, RecordEvent{RecordIndex: i, Event: outbox.Event{
			Topic: ev.Topic(), AggregateType: "process_instance", AggregateKey: st.Key, Payload: payload,
		}})
	}
	return out
}
