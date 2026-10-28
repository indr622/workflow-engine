package domain_test

import (
	"fmt"
	"testing"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/domain"
	"github.com/indr622/workflow-engine/libs/go/bpmn"
)

func linearServiceModel(b testing.TB, n int) *bpmn.Process {
	m := newModel("linear20").node("start", "startEvent").node("end", "endEvent")
	prev := "start"
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("t%d", i)
		m.service(id, "work", "")
		m.flow("f"+id, prev, id, "")
		prev = id
	}
	m.flow("fend", prev, "end", "")
	return m.build(b)
}

func parallelModelN(b testing.TB, n int) *bpmn.Process {
	m := newModel("par").node("start", "startEvent").gateway("fork", "parallelGateway", "").
		gateway("join", "parallelGateway", "").node("end", "endEvent").
		flow("f0", "start", "fork", "").flow("fz", "join", "end", "")
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("t%d", i)
		m.service(id, "work", "")
		m.flow("a"+id, "fork", id, "").flow("b"+id, id, "join", "")
	}
	return m.build(b)
}

// runToEnd membuat instance lalu menyelesaikan job terbuka (key terkecil dahulu) sampai habis dan
// mengembalikan jumlah element instance yang dibuat.
func runToEnd(b *testing.B, p *domain.Processor, proc *bpmn.Process, vars []domain.NamedValue) int {
	def := domain.DefinitionRef{Key: "00000000-0000-7000-8000-000000000001", BpmnProcessID: proc.ID(), Version: 1}
	st := domain.NewInstanceState(def, proc)
	keys := &counterKeys{}
	recs, err := p.Process(st, domain.CreateInstance{Variables: vars}, keys)
	if err != nil {
		b.Fatal(err)
	}
	elements := countActivations(recs)
	for len(st.Jobs) > 0 {
		var next string
		for k := range st.Jobs {
			if next == "" || k < next {
				next = k
			}
		}
		recs, err := p.Process(st, domain.CompleteJob{JobKey: next}, keys)
		if err != nil {
			b.Fatal(err)
		}
		elements += countActivations(recs)
	}
	if st.Status != domain.InstanceCompleted {
		b.Fatalf("status %s", st.Status)
	}
	return elements
}

func countActivations(recs []domain.Record) int {
	n := 0
	for i := range recs {
		if recs[i].Intent == domain.IntentElementActivating {
			n++
		}
	}
	return n
}

func benchmarkModel(b *testing.B, proc *bpmn.Process, vars []domain.NamedValue) {
	p := newProcessor(0)
	b.ReportAllocs()
	b.ResetTimer()
	elements := 0
	for i := 0; i < b.N; i++ {
		elements += runToEnd(b, p, proc, vars)
	}
	b.ReportMetric(float64(elements)/b.Elapsed().Seconds(), "elements/s")
}

func BenchmarkProcessorLinear(b *testing.B) {
	benchmarkModel(b, linearServiceModel(b, 20), nil)
}

func BenchmarkProcessorParallel50(b *testing.B) {
	benchmarkModel(b, parallelModelN(b, 50), nil)
}

func BenchmarkInclusiveJoin(b *testing.B) {
	benchmarkModel(b, inclusiveModel(b), mustVars(b, `{"x": 500}`))
}
