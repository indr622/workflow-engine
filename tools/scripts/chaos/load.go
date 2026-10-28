package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"

	"connectrpc.com/connect"

	jobv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/job/v1"
	processv1 "github.com/indr622/workflow-engine/libs/go/api/gen/workflow/process/v1"
)

type commandKind int

const (
	kindCreate commandKind = iota
	kindComplete
)

// command adalah satu command mutasi beserta request_id-nya. Request yang sama (termasuk
// request_id) dipakai ulang persis saat dicoba ulang.
type command struct {
	kind     commandKind
	create   *processv1.CreateProcessInstanceRequest
	complete *jobv1.CompleteJobRequest
	endpoint int
	// instanceKey: hasil create, atau instance pemilik job untuk complete.
	instanceKey string
	replayed    bool
	lastErr     error
}

func (c *command) requestID() string {
	if c.kind == kindCreate {
		return c.create.GetRequestId()
	}
	return c.complete.GetRequestId()
}

func (c *command) String() string {
	if c.kind == kindCreate {
		return fmt.Sprintf("create %s request_id=%s", c.create.GetBpmnProcess().GetBpmnProcessId(), c.requestID())
	}
	return fmt.Sprintf("complete job=%s request_id=%s", c.complete.GetJobKey(), c.requestID())
}

// ledger mencatat setiap command yang di-ack (sumber kebenaran sisi klien) dan yang belum di-ack
// (error transport/engine) untuk dicoba ulang setelah pemulihan.
type ledger struct {
	mu        sync.Mutex
	creates   map[string]string // instance key → request_id
	completes map[string]string // job key → request_id
	pending   []*command
	failed    []string // command yang ditolak definitif saat dicoba ulang (pelanggaran invarian)

	acked    atomic.Int64
	errors   atomic.Int64
	replayed atomic.Int64
}

func newLedger() *ledger {
	return &ledger{creates: map[string]string{}, completes: map[string]string{}}
}

func (l *ledger) ack(c *command) {
	l.acked.Add(1)
	if c.replayed {
		l.replayed.Add(1)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if c.kind == kindCreate {
		l.creates[c.instanceKey] = c.requestID()
	} else {
		l.completes[c.complete.GetJobKey()] = c.requestID()
	}
}

func (l *ledger) addPending(c *command, err error) {
	l.errors.Add(1)
	c.lastErr = err
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pending = append(l.pending, c)
}

func (l *ledger) takePending() []*command {
	l.mu.Lock()
	defer l.mu.Unlock()
	p := l.pending
	l.pending = nil
	return p
}

func (l *ledger) snapshot() (creates, completes map[string]string, failed []string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	creates, completes = make(map[string]string, len(l.creates)), make(map[string]string, len(l.completes))
	for k, v := range l.creates {
		creates[k] = v
	}
	for k, v := range l.completes {
		completes[k] = v
	}
	return creates, completes, append([]string(nil), l.failed...)
}

// execute menjalankan command sekali terhadap klien endpoint-nya.
func execute(ctx context.Context, clients []*engineClient, c *command) error {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	cl := clients[c.endpoint%len(clients)]
	switch c.kind {
	case kindCreate:
		resp, err := cl.instances.CreateProcessInstance(ctx, connect.NewRequest(c.create))
		if err != nil {
			return err
		}
		c.instanceKey = resp.Msg.GetProcessInstanceKey()
		c.replayed = resp.Header().Get(replayedHeader) == "true"
	case kindComplete:
		resp, err := cl.jobs.CompleteJob(ctx, connect.NewRequest(c.complete))
		if err != nil {
			return err
		}
		c.replayed = resp.Header().Get(replayedHeader) == "true"
	}
	return nil
}

func newCreate(p process, endpoint int) *command {
	return &command{kind: kindCreate, endpoint: endpoint, create: &processv1.CreateProcessInstanceRequest{
		Definition: &processv1.CreateProcessInstanceRequest_BpmnProcess{BpmnProcess: &processv1.ProcessDefinitionRef{BpmnProcessId: p.id}},
		Variables:  fmt.Sprintf(`{"i": 0, "orderId": "chaos-%s"}`, newRequestID()[:8]),
		RequestId:  newRequestID(),
	}}
}

func newComplete(jobKey, instanceKey, variables string, endpoint int) *command {
	return &command{kind: kindComplete, endpoint: endpoint, instanceKey: instanceKey, complete: &jobv1.CompleteJobRequest{
		JobKey: jobKey, Variables: variables, RequestId: newRequestID(),
	}}
}

// loadGenerator menjalankan N worker yang membuat instance korpus lalu menyelesaikan job-nya
// sampai instance selesai. Command yang gagal dicatat pending dan instance-nya ditinggalkan
// (tidak ada command lain pada job yang sama sebelum retry).
type loadGenerator struct {
	clients []*engineClient
	ledger  *ledger
	workers int
	logf    func(format string, args ...any)

	instancesDone atomic.Int64
}

func (g *loadGenerator) run(ctx context.Context) {
	var wg sync.WaitGroup
	for w := 0; w < g.workers; w++ {
		wg.Add(1)
		go func(seed uint64) {
			defer wg.Done()
			g.worker(ctx, rand.New(rand.NewPCG(seed, uint64(time.Now().UnixNano())))) //nolint:gosec // beban uji, bukan kriptografi
		}(uint64(w) + 1) //nolint:gosec // indeks worker kecil dan positif
	}
	wg.Wait()
}

func (g *loadGenerator) worker(ctx context.Context, rng *rand.Rand) {
	for ctx.Err() == nil {
		p := corpus[rng.IntN(len(corpus))]
		c := newCreate(p, rng.IntN(len(g.clients)))
		// RPC memakai context terpisah: akhir durasi load tidak boleh membatalkan command yang sedang
		// berjalan (pembatalan buatan akan tercatat sebagai error).
		if err := execute(context.WithoutCancel(ctx), g.clients, c); err != nil {
			g.ledger.addPending(c, err)
			_ = sleepCtx(ctx, 300*time.Millisecond)
			continue
		}
		g.ledger.ack(c)
		if g.drive(ctx, rng, p, c.instanceKey) {
			g.instancesDone.Add(1)
		}
	}
}

// drive menyelesaikan job instance langkah demi langkah; job paralel (fork) diselesaikan serentak
// lewat endpoint acak sehingga dua replika bersaing pada instance yang sama. Mengembalikan true bila
// instance tidak punya job terbuka lagi.
func (g *loadGenerator) drive(ctx context.Context, rng *rand.Rand, p process, instanceKey string) bool {
	iteration, readFailures := 0, 0
	for step := 0; step < 1000 && ctx.Err() == nil; step++ {
		jobs, err := g.clients[rng.IntN(len(g.clients))].listOpenJobs(ctx, instanceKey)
		if err != nil {
			readFailures++
			if readFailures > 5 || sleepCtx(ctx, 300*time.Millisecond) != nil {
				return false
			}
			continue
		}
		if len(jobs) == 0 {
			return true
		}
		cmds := make([]*command, len(jobs))
		for i, j := range jobs {
			vars := "{}"
			if p.loop {
				vars = fmt.Sprintf(`{"i": %d}`, iteration+1)
			}
			cmds[i] = newComplete(j.GetJobKey(), instanceKey, vars, rng.IntN(len(g.clients)))
		}
		var wg sync.WaitGroup
		var failed atomic.Bool
		for _, c := range cmds {
			wg.Add(1)
			go func(c *command) {
				defer wg.Done()
				if err := execute(context.WithoutCancel(ctx), g.clients, c); err != nil {
					g.ledger.addPending(c, err)
					failed.Store(true)
					return
				}
				g.ledger.ack(c)
			}(c)
		}
		wg.Wait()
		if failed.Load() {
			_ = sleepCtx(ctx, 300*time.Millisecond)
			return false
		}
		iteration++
	}
	return false
}

// retryPending mengulang setiap command yang belum di-ack dengan request yang sama persis sampai
// berhasil atau batas waktu habis. Penolakan definitif (mis. NotFound, FailedPrecondition) dicatat
// sebagai pelanggaran: command dengan request_id yang sama harus menghasilkan tepat satu efek.
func (l *ledger) retryPending(ctx context.Context, clients []*engineClient, timeout time.Duration, logf func(string, ...any)) (retried int, err error) {
	pending := l.takePending()
	if len(pending) == 0 {
		return 0, nil
	}
	deadline := time.Now().Add(timeout)
	sem := make(chan struct{}, 16)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var unresolved []string
	for _, c := range pending {
		wg.Add(1)
		sem <- struct{}{}
		go func(c *command) {
			defer wg.Done()
			defer func() { <-sem }()
			for {
				err := execute(ctx, clients, c)
				if err == nil {
					l.ack(c)
					return
				}
				c.lastErr = err
				c.endpoint++ // coba replika lain bila ada
				if definitive(err) {
					l.mu.Lock()
					l.failed = append(l.failed, fmt.Sprintf("%s rejected on retry: %s", c, errText(err)))
					l.mu.Unlock()
					return
				}
				if time.Now().After(deadline) || sleepCtx(ctx, 500*time.Millisecond) != nil {
					mu.Lock()
					unresolved = append(unresolved, fmt.Sprintf("%s: %s", c, errText(err)))
					mu.Unlock()
					return
				}
			}
		}(c)
	}
	wg.Wait()
	if len(unresolved) > 0 {
		for i, u := range unresolved {
			if i == 10 {
				logf("  ... and %d more", len(unresolved)-10)
				break
			}
			logf("  unresolved: %s", u)
		}
		return len(pending), fmt.Errorf("%d of %d un-acked commands still failing after %s", len(unresolved), len(pending), timeout)
	}
	return len(pending), nil
}
