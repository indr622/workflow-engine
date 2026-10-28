package domain

import (
	"sort"

	"github.com/indr622/workflow-engine/libs/go/bpmn"
)

// processAnalysis menyimpan hasil analisis statis model untuk inclusive join (ADR-0012).
type processAnalysis struct {
	// upstream[gatewayID][flowID] = id flow node yang bisa mencapai flow tersebut tanpa melewati
	// gateway itu sendiri.
	upstream map[string]map[string]map[string]bool
}

func (p *Processor) analysis(proc *bpmn.Process) *processAnalysis {
	p.mu.Lock()
	defer p.mu.Unlock()
	if a, ok := p.analyses[proc]; ok {
		return a
	}
	a := analyze(proc)
	p.analyses[proc] = a
	return a
}

func analyze(proc *bpmn.Process) *processAnalysis {
	a := &processAnalysis{upstream: map[string]map[string]map[string]bool{}}
	var walk func(s *bpmn.Scope)
	walk = func(s *bpmn.Scope) {
		for _, el := range s.Elements() {
			g, ok := el.(*bpmn.Gateway)
			if !ok || g.Type() != bpmn.InclusiveGatewayType || len(g.Incoming()) < 2 {
				continue
			}
			perFlow := map[string]map[string]bool{}
			for _, in := range g.Incoming() {
				perFlow[in.ID()] = upstreamOf(g, in)
			}
			a.upstream[g.ID()] = perFlow
		}
		for _, child := range s.Children() {
			if inner := child.Inner(); inner != nil {
				walk(inner)
			}
		}
	}
	walk(&proc.Scope)
	return a
}

// upstreamOf menelusuri mundur dari sumber flow dan berhenti di gateway join.
func upstreamOf(gateway bpmn.FlowNode, flow *bpmn.SequenceFlow) map[string]bool {
	seen := map[string]bool{}
	var stack []bpmn.FlowNode
	if src := flow.Source(); src != nil {
		stack = append(stack, src)
	}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n.ID() == gateway.ID() || seen[n.ID()] {
			continue
		}
		seen[n.ID()] = true
		for _, in := range n.Incoming() {
			if src := in.Source(); src != nil {
				stack = append(stack, src)
			}
		}
	}
	return seen
}

// tryInclusiveJoins mengaktifkan satu inclusive join tertunda yang tidak lagi bisa menerima token
// dari upstream. Dipanggil hanya saat antrean kosong sehingga token dalam command yang sama sudah
// berada di posisi akhirnya. Mengembalikan true jika ada join yang diaktifkan.
func (r *run) tryInclusiveJoins() (bool, error) {
	type pending struct {
		scopeKey string
		gateway  bpmn.FlowNode
	}
	if len(r.st.TakenFlows) == 0 || len(r.p.analysis(r.st.Process).upstream) == 0 {
		return false, nil
	}
	var candidates []pending
	seen := map[[2]any]bool{}
	for _, tf := range r.st.SortedTakenFlows() {
		id := [2]any{tf.FlowScopeKey, tf.GatewayID}
		if seen[id] {
			continue
		}
		seen[id] = true
		node, err := r.node(tf.GatewayID)
		if err != nil {
			return false, err
		}
		if node.Type() == bpmn.InclusiveGatewayType {
			candidates = append(candidates, pending{scopeKey: tf.FlowScopeKey, gateway: node})
		}
	}
	if len(candidates) == 0 {
		return false, nil
	}
	a := r.p.analysis(r.st.Process)
	for _, c := range candidates {
		if r.joinReady(a, c.scopeKey, c.gateway) {
			return true, r.activate(c.gateway, c.scopeKey)
		}
	}
	return false, nil
}

// joinReady: join siap jika tidak ada token aktif di scope yang berada di upstream incoming flow
// yang belum punya token.
func (r *run) joinReady(a *processAnalysis, scopeKey string, g bpmn.FlowNode) bool {
	var waiting []string
	for _, in := range g.Incoming() {
		if r.st.TakenFlows[JoinKey{FlowScopeKey: scopeKey, GatewayID: g.ID(), SequenceFlowID: in.ID()}] == 0 {
			waiting = append(waiting, in.ID())
		}
	}
	if len(waiting) == 0 {
		return true
	}
	positions := r.tokenPositions(scopeKey, g.ID())
	for _, flowID := range waiting {
		up := a.upstream[g.ID()][flowID]
		for _, pos := range positions {
			if up[pos] {
				return false
			}
		}
	}
	return true
}

// tokenPositions mengembalikan id node tempat token aktif berada di scope: element instance
// non-final dan gateway join lain yang punya counter.
func (r *run) tokenPositions(scopeKey string, exceptGateway string) []string {
	var out []string
	for _, e := range r.st.ChildrenOf(scopeKey) {
		out = append(out, e.ElementID)
	}
	gateways := map[string]bool{}
	for k := range r.st.TakenFlows {
		if k.FlowScopeKey == scopeKey && k.GatewayID != exceptGateway {
			gateways[k.GatewayID] = true
		}
	}
	for id := range gateways {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}
