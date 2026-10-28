package domain_test

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"
	"testing"

	"github.com/indr622/workflow-engine/apps/engine/internal/modules/execution/domain"
)

var propIter = flag.Int("prop.iter", 1000, "number of random graphs for TestGatewayProperties")

// blockKind adalah jenis blok terstruktur di graf acak.
type blockKind uint8

const (
	blockLeaf blockKind = iota
	blockSeq
	blockParallel
	blockInclusive
	blockExclusive
)

// block adalah pohon blok: setiap fork punya join pasangan (graf blok).
type block struct {
	kind     blockKind
	children []*block
	// conds[i] adalah nama variabel boolean untuk cabang i; cabang terakhir inclusive/exclusive
	// adalah default (tanpa kondisi).
	conds []string
	// ids gateway hasil render, untuk memeriksa pasangan fork/join.
	fork, join string
}

const propVars = 5

func randomBlock(rng *rand.Rand, depth int) *block {
	if depth == 0 || rng.IntN(4) == 0 {
		return &block{kind: blockLeaf}
	}
	kind := blockKind(1 + rng.IntN(4))
	width := 2 + rng.IntN(4)
	b := &block{kind: kind}
	for i := 0; i < width; i++ {
		b.children = append(b.children, randomBlock(rng, depth-1))
		if kind == blockInclusive || kind == blockExclusive {
			b.conds = append(b.conds, "v"+strconv.Itoa(rng.IntN(propVars)))
		}
	}
	return b
}

// renderer menulis blok ke modelBuilder dan mengembalikan id node masuk dan keluar.
type renderer struct {
	m *modelBuilder
	n int
}

func (r *renderer) id(prefix string) string {
	r.n++
	return fmt.Sprintf("%s%d", prefix, r.n)
}

func (r *renderer) link(from, to, cond string) {
	r.m.flow(r.id("f"), from, to, cond)
}

func (r *renderer) render(b *block) (string, string) {
	switch b.kind {
	case blockLeaf:
		id := r.id("t")
		r.m.service(id, "work", "")
		return id, id
	case blockSeq:
		first, last := r.render(b.children[0])
		for _, c := range b.children[1:] {
			in, out := r.render(c)
			r.link(last, in, "")
			last = out
		}
		return first, last
	default:
		gwKind := map[blockKind]string{blockParallel: "parallelGateway", blockInclusive: "inclusiveGateway", blockExclusive: "exclusiveGateway"}[b.kind]
		b.fork, b.join = r.id("fork"), r.id("join")
		var defaultFlow string
		var flows []func()
		for i, c := range b.children {
			in, out := r.render(c)
			flowID := r.id("f")
			cond := ""
			isDefault := b.kind != blockParallel && i == len(b.children)-1
			if b.kind != blockParallel && !isDefault {
				cond = "=" + b.conds[i]
			}
			if isDefault {
				defaultFlow = flowID
			}
			fid, from, to, cnd := flowID, b.fork, in, cond
			flows = append(flows, func() { r.m.flow(fid, from, to, cnd) })
			r.link(out, b.join, "")
		}
		r.m.gateway(b.fork, gwKind, defaultFlow)
		r.m.gateway(b.join, gwKind, "")
		for _, f := range flows {
			f()
		}
		return b.fork, b.join
	}
}

// expectedJobs menghitung jumlah job (service task yang dijalankan) dari blok dan variabel.
func expectedJobs(b *block, vars map[string]bool) int {
	switch b.kind {
	case blockLeaf:
		return 1
	case blockSeq, blockParallel:
		n := 0
		for _, c := range b.children {
			n += expectedJobs(c, vars)
		}
		return n
	case blockExclusive:
		for i, c := range b.children[:len(b.children)-1] {
			if vars[b.conds[i]] {
				return expectedJobs(c, vars)
			}
		}
		return expectedJobs(b.children[len(b.children)-1], vars)
	case blockInclusive:
		n, taken := 0, false
		for i, c := range b.children[:len(b.children)-1] {
			if vars[b.conds[i]] {
				n += expectedJobs(c, vars)
				taken = true
			}
		}
		if !taken {
			n = expectedJobs(b.children[len(b.children)-1], vars)
		}
		return n
	}
	return 0
}

// forkJoinPairs mengumpulkan pasangan gateway fork/join yang aktif (bukan exclusive) dari pohon.
func forkJoinPairs(b *block, out *[][2]string) {
	if b.kind == blockParallel || b.kind == blockInclusive {
		*out = append(*out, [2]string{b.fork, b.join})
	}
	for _, c := range b.children {
		forkJoinPairs(c, out)
	}
}

func TestGatewayProperties(t *testing.T) {
	seed := uint64(20261028)
	if s := os.Getenv("PROP_SEED"); s != "" {
		v, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			t.Fatalf("PROP_SEED: %v", err)
		}
		seed = v
	}
	iterations := *propIter
	if testing.Short() {
		iterations = 100
	}
	for i := 0; i < iterations; i++ {
		caseSeed := seed + uint64(i)
		if !runGatewayProperty(t, caseSeed) {
			t.Fatalf("property failed; reproduce with PROP_SEED=%d -prop.iter=1", caseSeed)
		}
	}
}

func runGatewayProperty(t *testing.T, seed uint64) bool {
	t.Helper()
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	root := randomBlock(rng, 4)
	r := &renderer{m: newModel("prop")}
	r.m.node("start", "startEvent").node("end", "endEvent")
	in, out := r.render(root)
	r.link("start", in, "")
	r.link(out, "end", "")
	proc := r.m.build(t)

	vars := map[string]bool{}
	doc := "{"
	for i := 0; i < propVars; i++ {
		name := "v" + strconv.Itoa(i)
		vars[name] = rng.IntN(2) == 0
		if i > 0 {
			doc += ","
		}
		doc += fmt.Sprintf("%q: %t", name, vars[name])
	}
	doc += "}"

	h := startInstance(t, proc, doc)
	jobs := 0
	for len(h.st.Jobs) > 0 {
		keys := make([]int64, 0, len(h.st.Jobs))
		for k := range h.st.Jobs {
			keys = append(keys, k)
		}
		// Pilih job acak secara deterministik: urutkan dulu agar iterasi map tidak berpengaruh.
		sortInt64(keys)
		k := keys[rng.IntN(len(keys))]
		if err := h.run(domain.CompleteJob{JobKey: k}); err != nil {
			t.Errorf("seed %d: complete job: %v", seed, err)
			return false
		}
		jobs++
	}

	ok := true
	fail := func(format string, args ...any) {
		t.Errorf("seed %d: "+format, append([]any{seed}, args...)...)
		ok = false
	}
	if h.st.Status != domain.InstanceCompleted {
		fail("status %s, active %v, counters %v", h.st.Status, h.activeElements(), h.st.TakenFlows)
	}
	if len(h.st.TakenFlows) != 0 {
		fail("leftover counters %v", h.st.TakenFlows)
	}
	if want := expectedJobs(root, vars); jobs != want {
		fail("completed %d jobs, oracle expects %d", jobs, want)
	}
	var pairs [][2]string
	forkJoinPairs(root, &pairs)
	for _, p := range pairs {
		forks, joins := h.countIntent(p[0], domain.IntentElementActivating), h.countIntent(p[1], domain.IntentElementActivating)
		if forks != joins {
			fail("fork %s activated %d times but join %s %d times", p[0], forks, p[1], joins)
		}
	}
	if ok {
		h.assertReplay()
	}
	return ok
}

func sortInt64(s []int64) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
