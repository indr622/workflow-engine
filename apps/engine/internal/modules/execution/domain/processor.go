package domain

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/indr622/workflow-engine/libs/go/bpmn"
)

// DefaultMaxRecordsPerCommand adalah batas record satu command bila Limits tidak diisi.
const DefaultMaxRecordsPerCommand = 10_000

// Limits membatasi kerja satu command.
type Limits struct {
	// MaxRecordsPerCommand: melebihi batas ini saat mengaktifkan elemen menghasilkan incident
	// EXECUTION_LIMIT_EXCEEDED dan menghentikan command (record sebelumnya tetap ditulis).
	MaxRecordsPerCommand int
}

// Command adalah perintah eksternal: CreateInstance, CompleteJob, atau CancelInstance.
type Command interface{ isCommand() }

// CreateInstance membuat process instance baru pada state kosong.
type CreateInstance struct {
	Variables []NamedValue
}

// CompleteJob menyelesaikan job terbuka dan melanjutkan token.
type CompleteJob struct {
	JobKey    int64
	Variables []NamedValue
}

// CancelInstance menghentikan seluruh element instance aktif dan menandai instance TERMINATED.
type CancelInstance struct{}

func (CreateInstance) isCommand() {}
func (CompleteJob) isCommand()    {}
func (CancelInstance) isCommand() {}

// Processor menjalankan command secara murni dan deterministik: state, command, key, dan jam
// yang sama selalu menghasilkan record yang sama. Aman dipakai bersama banyak goroutine.
type Processor struct {
	evaluator ExpressionEvaluator
	clock     Clock
	limits    Limits

	mu       sync.Mutex
	analyses map[*bpmn.Process]*processAnalysis
}

// NewProcessor membuat Processor.
func NewProcessor(evaluator ExpressionEvaluator, clock Clock, limits Limits) *Processor {
	if limits.MaxRecordsPerCommand <= 0 {
		limits.MaxRecordsPerCommand = DefaultMaxRecordsPerCommand
	}
	return &Processor{evaluator: evaluator, clock: clock, limits: limits, analyses: map[*bpmn.Process]*processAnalysis{}}
}

// Process menjalankan satu command terhadap state sampai semua token mencapai wait state atau
// selesai, lalu mengembalikan record yang dihasilkan (state sudah diperbarui lewat Apply). Jika
// error dikembalikan, state mungkin sudah berubah sebagian dan harus dibuang oleh pemanggil.
func (p *Processor) Process(st *InstanceState, cmd Command, keys KeyGenerator) ([]Record, error) {
	r := &run{p: p, st: st, keys: keys, now: p.clock.Now()}
	var err error
	switch c := cmd.(type) {
	case CreateInstance:
		err = r.createInstance(c)
	case CompleteJob:
		err = r.completeJob(c)
	case CancelInstance:
		err = r.cancelInstance()
	default:
		err = fmt.Errorf("unknown command %T", cmd)
	}
	if err != nil {
		return nil, err
	}
	return r.records, nil
}

// workKind adalah jenis pekerjaan di antrean internal (command in-memory, tidak disimpan).
type workKind uint8

const (
	workActivate workKind = iota + 1
	workTakeFlow
	workComplete
)

type work struct {
	kind     workKind
	node     bpmn.FlowNode
	flow     *bpmn.SequenceFlow
	scopeKey int64
	elemKey  int64
}

// run adalah satu eksekusi command.
type run struct {
	p       *Processor
	st      *InstanceState
	keys    KeyGenerator
	now     time.Time
	records []Record
	queue   []work
	halted  bool
}

func (r *run) nextKey() (int64, error) {
	k, ok := r.keys.Next()
	if !ok {
		return 0, ErrKeysExhausted
	}
	return k, nil
}

// emit menulis record dan langsung menerapkannya ke state.
func (r *run) emit(key int64, vt ValueType, intent Intent, value any) error {
	rec := Record{
		Key: key, ProcessInstanceKey: r.st.Key, ValueType: vt, Intent: intent, Timestamp: r.now, Value: value,
	}
	if vt == ValueProcessInstance && intent == IntentElementActivating {
		if v, ok := value.(*ProcessInstanceValue); ok && v.BpmnElementType == ElementTypeProcess {
			rec.ProcessInstanceKey = key
		}
	}
	if err := Apply(r.st, &rec); err != nil {
		return err
	}
	r.records = append(r.records, rec)
	return nil
}

func (r *run) piValue(elementID, elementType string, scopeKey int64) *ProcessInstanceValue {
	d := r.st.Definition
	return &ProcessInstanceValue{
		BpmnProcessID: d.BpmnProcessID, ProcessDefinitionKey: d.Key, Version: d.Version,
		ElementID: elementID, BpmnElementType: elementType, FlowScopeKey: scopeKey,
	}
}

// emitElement menulis record transisi element instance yang sudah ada.
func (r *run) emitElement(el *ElementInstance, intent Intent) error {
	return r.emit(el.Key, ValueProcessInstance, intent, r.piValue(el.ElementID, el.BpmnElementType, el.FlowScopeKey))
}

func (r *run) createInstance(c CreateInstance) error {
	if r.st.Key != 0 {
		return fmt.Errorf("%w: create on a non-empty state", ErrInvalidRecord)
	}
	proc := r.st.Process
	start, ok := proc.NoneStartEvent()
	if !ok {
		return ErrNoNoneStartEvent
	}
	piKey, err := r.nextKey()
	if err != nil {
		return err
	}
	if err := r.emit(piKey, ValueProcessInstance, IntentElementActivating, r.piValue(proc.ID(), ElementTypeProcess, 0)); err != nil {
		return err
	}
	if err := r.mergeVariables(c.Variables); err != nil {
		return err
	}
	creationKey, err := r.nextKey()
	if err != nil {
		return err
	}
	names := make([]string, 0, len(c.Variables))
	for _, v := range c.Variables {
		names = append(names, v.Name)
	}
	d := r.st.Definition
	if err := r.emit(creationKey, ValueProcessInstanceCreation, IntentCreated, &ProcessInstanceCreationValue{
		BpmnProcessID: d.BpmnProcessID, ProcessDefinitionKey: d.Key, Version: d.Version,
		ProcessInstanceKey: piKey, VariableNames: names,
	}); err != nil {
		return err
	}
	if err := r.emitElement(r.st.Elements[piKey], IntentElementActivated); err != nil {
		return err
	}
	r.queue = append(r.queue, work{kind: workActivate, node: start, scopeKey: piKey})
	return r.loop()
}

func (r *run) completeJob(c CompleteJob) error {
	if r.st.Status != InstanceActive {
		return ErrInstanceNotActive
	}
	job, ok := r.st.Jobs[c.JobKey]
	if !ok {
		return ErrJobNotActive
	}
	el, ok := r.st.Elements[job.ElementInstanceKey]
	if !ok || el.State != ElementActivated {
		return ErrJobNotActive
	}
	d := r.st.Definition
	if err := r.emit(job.Key, ValueJob, IntentCompleted, &JobValue{
		Type: job.Type, Retries: job.Retries, CustomHeaders: job.CustomHeaders,
		ElementInstanceKey: job.ElementInstanceKey, ElementID: job.ElementID,
		BpmnProcessID: d.BpmnProcessID, ProcessDefinitionKey: d.Key, Version: d.Version,
	}); err != nil {
		return err
	}
	if err := r.mergeVariables(c.Variables); err != nil {
		return err
	}
	r.queue = append(r.queue, work{kind: workComplete, elemKey: el.Key})
	return r.loop()
}

func (r *run) cancelInstance() error {
	if r.st.Status != InstanceActive {
		return ErrInstanceNotActive
	}
	proc := r.st.Elements[r.st.Key]
	if proc == nil {
		return fmt.Errorf("%w: active instance without process element", ErrInvalidRecord)
	}
	if proc.State != ElementTerminating {
		if err := r.emitElement(proc, IntentElementTerminating); err != nil {
			return err
		}
	}
	if err := r.terminateChildren(r.st.Key); err != nil {
		return err
	}
	if err := r.resolveIncidents(); err != nil {
		return err
	}
	return r.emitElement(proc, IntentElementTerminated)
}

// terminateChildren menghentikan anak non-final sebuah scope, key terbesar dahulu; job terbuka
// milik anak dibatalkan sebelum elemennya dihentikan.
func (r *run) terminateChildren(scopeKey int64) error {
	children := r.st.ChildrenOf(scopeKey)
	for i := len(children) - 1; i >= 0; i-- {
		child := children[i]
		if err := r.cancelJobsOf(child.Key); err != nil {
			return err
		}
		if child.State != ElementTerminating {
			if err := r.emitElement(child, IntentElementTerminating); err != nil {
				return err
			}
		}
		if err := r.emitElement(child, IntentElementTerminated); err != nil {
			return err
		}
	}
	return nil
}

func (r *run) cancelJobsOf(elemKey int64) error {
	var keys []int64
	for k, j := range r.st.Jobs {
		if j.ElementInstanceKey == elemKey {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	d := r.st.Definition
	for _, k := range keys {
		j := r.st.Jobs[k]
		if err := r.emit(k, ValueJob, IntentCanceled, &JobValue{
			Type: j.Type, Retries: j.Retries, CustomHeaders: j.CustomHeaders,
			ElementInstanceKey: j.ElementInstanceKey, ElementID: j.ElementID,
			BpmnProcessID: d.BpmnProcessID, ProcessDefinitionKey: d.Key, Version: d.Version,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *run) resolveIncidents() error {
	keys := make([]int64, 0, len(r.st.Incidents))
	for k := range r.st.Incidents {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for _, k := range keys {
		inc := r.st.Incidents[k]
		if err := r.emit(k, ValueIncident, IntentResolved, &IncidentValue{
			ErrorType: inc.Type, ErrorMessage: inc.Message, ElementInstanceKey: inc.ElementInstanceKey, ElementID: inc.ElementID,
		}); err != nil {
			return err
		}
	}
	return nil
}

// mergeVariables menulis variabel ke scope process instance: CREATED bila baru, UPDATED bila
// nilainya berubah, tanpa record bila byte JSON ringkasnya identik.
func (r *run) mergeVariables(vars []NamedValue) error {
	for _, v := range vars {
		vk := VariableKey{ScopeKey: r.st.Key, Name: v.Name}
		value := &VariableValue{Name: v.Name, Value: v.Value, ScopeKey: r.st.Key}
		if existing, ok := r.st.Variables[vk]; ok {
			if string(existing.Value) == string(v.Value) {
				continue
			}
			if err := r.emit(existing.Key, ValueVariable, IntentUpdated, value); err != nil {
				return err
			}
			continue
		}
		key, err := r.nextKey()
		if err != nil {
			return err
		}
		if err := r.emit(key, ValueVariable, IntentCreated, value); err != nil {
			return err
		}
	}
	return nil
}

// loop memproses antrean FIFO sampai kosong, lalu menjalankan pengecekan quiescence (join
// inclusive tertunda, penyelesaian flow scope) yang bisa menambah kerja baru.
func (r *run) loop() error {
	for {
		for len(r.queue) > 0 && !r.halted {
			w := r.queue[0]
			r.queue = r.queue[1:]
			var err error
			switch w.kind {
			case workActivate:
				err = r.activate(w.node, w.scopeKey)
			case workTakeFlow:
				err = r.takeFlow(w.flow, w.scopeKey)
			case workComplete:
				err = r.complete(w.elemKey)
			}
			if err != nil {
				return err
			}
		}
		if r.halted {
			r.queue = nil
			return nil
		}
		activated, err := r.tryInclusiveJoins()
		if err != nil {
			return err
		}
		if activated {
			continue
		}
		if err := r.tryCompleteScope(r.st.Key); err != nil {
			return err
		}
		if len(r.queue) == 0 {
			return nil
		}
	}
}

// activate membuat element instance baru untuk node dan menjalankan perilaku aktivasinya.
func (r *run) activate(node bpmn.FlowNode, scopeKey int64) error {
	key, err := r.nextKey()
	if err != nil {
		return err
	}
	elementType := BpmnElementType(node.Type())
	if err := r.emit(key, ValueProcessInstance, IntentElementActivating, r.piValue(node.ID(), elementType, scopeKey)); err != nil {
		return err
	}
	el := r.st.Elements[key]
	if len(r.records) > r.p.limits.MaxRecordsPerCommand {
		r.halted = true
		return r.raiseIncident(el, IncidentExecutionLimitExceeded, fmt.Sprintf(
			"execution limit of %d records per command exceeded while activating %q; the process probably loops without a wait state",
			r.p.limits.MaxRecordsPerCommand, node.ID()))
	}
	switch node.Type() {
	case bpmn.StartEventType, bpmn.EndEventType, bpmn.TaskType, bpmn.ManualTaskType, bpmn.ParallelGatewayType:
		return r.passThrough(el, node.Outgoing())
	case bpmn.ExclusiveGatewayType:
		flows, msg := r.chooseExclusive(node)
		if msg != "" {
			return r.raiseIncident(el, IncidentConditionError, msg)
		}
		return r.passThrough(el, flows)
	case bpmn.InclusiveGatewayType:
		flows, msg := r.chooseInclusive(node)
		if msg != "" {
			return r.raiseIncident(el, IncidentConditionError, msg)
		}
		return r.passThrough(el, flows)
	case bpmn.ServiceTaskType:
		return r.activateServiceTask(el, node)
	default:
		return fmt.Errorf("%w: %s %q", ErrUnsupportedElement, node.Type(), node.ID())
	}
}

// passThrough menyelesaikan elemen yang tidak menunggu lalu mengambil flow yang dipilih.
func (r *run) passThrough(el *ElementInstance, flows []*bpmn.SequenceFlow) error {
	for _, intent := range []Intent{IntentElementActivated, IntentElementCompleting, IntentElementCompleted} {
		if err := r.emitElement(el, intent); err != nil {
			return err
		}
	}
	r.enqueueFlows(flows, el.FlowScopeKey)
	return nil
}

func (r *run) enqueueFlows(flows []*bpmn.SequenceFlow, scopeKey int64) {
	for _, f := range flows {
		r.queue = append(r.queue, work{kind: workTakeFlow, flow: f, scopeKey: scopeKey})
	}
}

// complete menyelesaikan elemen yang menunggu (service task setelah job selesai).
func (r *run) complete(elemKey int64) error {
	el, ok := r.st.Elements[elemKey]
	if !ok {
		return fmt.Errorf("%w: complete unknown element instance %d", ErrInvalidRecord, elemKey)
	}
	if err := r.emitElement(el, IntentElementCompleting); err != nil {
		return err
	}
	if err := r.emitElement(el, IntentElementCompleted); err != nil {
		return err
	}
	node, err := r.node(el.ElementID)
	if err != nil {
		return err
	}
	r.enqueueFlows(node.Outgoing(), el.FlowScopeKey)
	return nil
}

// takeFlow mencatat sequence flow lalu mengaktifkan target; untuk gateway join, token menjadi
// counter. Parallel join diaktifkan segera saat semua incoming sudah tiba; inclusive join
// diputuskan saat quiescence.
func (r *run) takeFlow(flow *bpmn.SequenceFlow, scopeKey int64) error {
	key, err := r.nextKey()
	if err != nil {
		return err
	}
	if err := r.emit(key, ValueProcessInstance, IntentSequenceFlowTaken, r.piValue(flow.ID(), ElementTypeSequenceFlow, scopeKey)); err != nil {
		return err
	}
	target := flow.Target()
	if !isJoin(target) {
		r.queue = append(r.queue, work{kind: workActivate, node: target, scopeKey: scopeKey})
		return nil
	}
	if target.Type() != bpmn.ParallelGatewayType {
		return nil
	}
	for _, in := range target.Incoming() {
		if r.st.TakenFlows[JoinKey{FlowScopeKey: scopeKey, GatewayID: target.ID(), SequenceFlowID: in.ID()}] == 0 {
			return nil
		}
	}
	return r.activate(target, scopeKey)
}

// tryCompleteScope menyelesaikan flow scope yang tidak punya anak aktif dan tidak punya counter
// join tersisa. Counter yang tidak akan pernah lengkap membuat instance tetap ACTIVE.
func (r *run) tryCompleteScope(scopeKey int64) error {
	scope, ok := r.st.Elements[scopeKey]
	if !ok || scope.State != ElementActivated {
		return nil
	}
	if len(r.st.ChildrenOf(scopeKey)) > 0 {
		return nil
	}
	for k := range r.st.TakenFlows {
		if k.FlowScopeKey == scopeKey {
			return nil
		}
	}
	if err := r.emitElement(scope, IntentElementCompleting); err != nil {
		return err
	}
	return r.emitElement(scope, IntentElementCompleted)
}

func (r *run) raiseIncident(el *ElementInstance, t IncidentType, msg string) error {
	key, err := r.nextKey()
	if err != nil {
		return err
	}
	return r.emit(key, ValueIncident, IntentCreated, &IncidentValue{
		ErrorType: t, ErrorMessage: msg, ElementInstanceKey: el.Key, ElementID: el.ElementID,
	})
}

func (r *run) node(id string) (bpmn.FlowNode, error) {
	el, ok := r.st.Process.Element(id)
	if !ok {
		return nil, fmt.Errorf("%w: element %q is not in the process", ErrInvalidRecord, id)
	}
	n, ok := el.(bpmn.FlowNode)
	if !ok {
		return nil, fmt.Errorf("%w: %q is not a flow node", ErrInvalidRecord, id)
	}
	return n, nil
}

// IsDomainError mengembalikan true untuk error yang berasal dari aturan domain (bukan bug/I/O).
func IsDomainError(err error) bool {
	for _, e := range []error{ErrInstanceNotFound, ErrInstanceNotActive, ErrJobNotFound, ErrJobNotActive, ErrInvalidVariables, ErrNoNoneStartEvent} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}
