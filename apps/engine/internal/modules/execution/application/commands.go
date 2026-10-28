// Package application berisi use case modul execution: menjalankan command terhadap process
// instance dalam satu transaksi (kunci → muat → Processor → tulis record + proyeksi → commit →
// publikasi event) dan query atas proyeksi.
package application

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	deploymentapi "github.com/indr622/workflow-engine/apps/engine/internal/modules/deployment/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/api"
	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/domain"
	"github.com/indr622/workflow-engine/apps/engine/internal/platform/eventbus"
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
	repo      domain.Repository
	defs      deploymentapi.ProcessDefinitions
	processor *domain.Processor
	keys      domain.KeyGenerator
	bus       eventbus.Bus
	log       *slog.Logger
	cfg       Config
}

var (
	_ api.ProcessInstances = (*Commands)(nil)
	_ api.Jobs             = (*Commands)(nil)
)

// NewCommands merakit use case command.
func NewCommands(repo domain.Repository, defs deploymentapi.ProcessDefinitions, processor *domain.Processor,
	keys domain.KeyGenerator, bus eventbus.Bus, log *slog.Logger, cfg Config) *Commands {
	if cfg.MaxVariablesBytes <= 0 {
		cfg.MaxVariablesBytes = DefaultMaxVariablesBytes
	}
	if log == nil {
		log = slog.Default()
	}
	return &Commands{repo: repo, defs: defs, processor: processor, keys: keys, bus: bus, log: log, cfg: cfg}
}

// ErrDefinitionRequired dikembalikan bila request tidak memilih process definition.
var ErrDefinitionRequired = errors.New("process definition key or bpmn process id is required")

// CreateProcessInstance memilih versi proses lalu menjalankan instance baru sampai wait state.
func (c *Commands) CreateProcessInstance(ctx context.Context, req api.CreateInstanceRequest) (api.CreateInstanceResult, error) {
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
	err = c.repo.WithTx(ctx, func(ctx context.Context, tx domain.TxRepository) error {
		st = domain.NewInstanceState(ref, proc)
		records, err = c.processor.Process(st, domain.CreateInstance{Variables: vars}, c.keys)
		if err != nil {
			return err
		}
		return tx.AppendAndProject(ctx, records, st)
	})
	if err != nil {
		return api.CreateInstanceResult{}, fmt.Errorf("create process instance: %w", err)
	}
	c.publish(ctx, st, records)
	return api.CreateInstanceResult{
		ProcessInstanceKey: st.Key, ProcessDefinitionKey: def.Key, BpmnProcessID: def.BpmnProcessID,
		Version: def.Version, State: string(st.Status),
	}, nil
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
func (c *Commands) CompleteJob(ctx context.Context, jobKey string, variables []byte) error {
	vars, err := domain.ParseVariables(variables, c.cfg.MaxVariablesBytes)
	if err != nil {
		return err
	}
	piKey, err := c.repo.InstanceKeyOfJob(ctx, jobKey)
	if err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	proc, err := c.processOf(ctx, piKey)
	if err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	var st *domain.InstanceState
	var records []domain.Record
	err = c.repo.WithTx(ctx, func(ctx context.Context, tx domain.TxRepository) error {
		var err error
		st, err = lockAndLoad(ctx, tx, piKey, proc)
		if err != nil {
			return err
		}
		records, err = c.processor.Process(st, domain.CompleteJob{JobKey: jobKey, Variables: vars}, c.keys)
		if err != nil {
			return err
		}
		return tx.AppendAndProject(ctx, records, st)
	})
	if err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	c.publish(ctx, st, records)
	return nil
}

// CancelProcessInstance menghentikan instance aktif.
func (c *Commands) CancelProcessInstance(ctx context.Context, processInstanceKey string) error {
	proc, err := c.processOf(ctx, processInstanceKey)
	if err != nil {
		return fmt.Errorf("cancel process instance: %w", err)
	}
	var st *domain.InstanceState
	var records []domain.Record
	err = c.repo.WithTx(ctx, func(ctx context.Context, tx domain.TxRepository) error {
		var err error
		st, err = lockAndLoad(ctx, tx, processInstanceKey, proc)
		if err != nil {
			return err
		}
		records, err = c.processor.Process(st, domain.CancelInstance{}, c.keys)
		if err != nil {
			return err
		}
		return tx.AppendAndProject(ctx, records, st)
	})
	if err != nil {
		return fmt.Errorf("cancel process instance: %w", err)
	}
	c.publish(ctx, st, records)
	return nil
}

// processOf memuat model proses sebuah instance SEBELUM transaksi dibuka. Memuatnya di dalam
// transaksi (sambil memegang row lock) bisa meminta koneksi kedua dari pool; pada banyak command
// paralel untuk satu instance, pool habis dan semua transaksi saling menunggu.
func (c *Commands) processOf(ctx context.Context, instanceKey string) (*bpmn.Process, error) {
	defKey, err := c.repo.DefinitionKeyOf(ctx, instanceKey)
	if err != nil {
		return nil, err
	}
	_, proc, err := c.defs.GetByKey(ctx, defKey)
	if err != nil {
		return nil, fmt.Errorf("load process definition %s: %w", defKey, err)
	}
	return proc, nil
}

// lockAndLoad mengunci instance lalu memuat state runtime-nya.
func lockAndLoad(ctx context.Context, tx domain.TxRepository, key string, proc *bpmn.Process) (*domain.InstanceState, error) {
	if err := tx.LockInstance(ctx, key); err != nil {
		return nil, err
	}
	snap, err := tx.LoadRuntimeState(ctx, key)
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
