package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// session adalah state satu skenario: lingkungan compose segar, klien engine, ledger, dan log
// runner (ditulis ke tmp/chaos-logs bila skenario gagal).
type session struct {
	cfg      config
	dc       *compose
	ledger   *ledger
	clients  []*engineClient
	replicas bool

	mu  sync.Mutex
	log strings.Builder
}

func newSession(cfg config, dc *compose) *session {
	return &session{cfg: cfg, dc: dc, ledger: newLedger()}
}

func (s *session) logf(format string, args ...any) {
	line := fmt.Sprintf("%s  %s", time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
	s.mu.Lock()
	s.log.WriteString(line + "\n")
	s.mu.Unlock()
	if s.cfg.verbose {
		fmt.Println("    " + line)
	}
}

func (s *session) engineServices() []string {
	if s.replicas {
		return []string{serviceEngine, serviceEngine2}
	}
	return []string{serviceEngine}
}

// fresh menghapus project chaos (termasuk volume) lalu menyalakan postgres dan engine dari nol,
// sehingga invarian tiap skenario hanya melihat data skenario itu sendiri.
func (s *session) fresh(ctx context.Context, replicas bool) error {
	s.replicas = replicas
	if err := s.dc.down(ctx); err != nil {
		return err
	}
	services := append([]string{servicePostgres}, s.engineServices()...)
	start := time.Now()
	if err := s.dc.up(ctx, services...); err != nil {
		return err
	}
	s.clients = []*engineClient{newEngineClient(serviceEngine, s.cfg.engineURL)}
	if replicas {
		s.clients = append(s.clients, newEngineClient(serviceEngine2, s.cfg.engine2URL))
	}
	if err := s.waitReady(ctx); err != nil {
		return err
	}
	s.logf("environment up (%s) in %s", strings.Join(services, ", "), time.Since(start).Round(time.Millisecond))
	return s.clients[0].deployCorpus(ctx, 30*time.Second)
}

// waitReady menunggu semua replika engine menjawab /readyz 200.
func (s *session) waitReady(ctx context.Context) error {
	for _, c := range s.clients {
		took, err := c.waitReady(ctx, s.cfg.readyTimeout)
		if err != nil {
			return err
		}
		s.logf("%s ready after %s", c.name, took.Round(time.Millisecond))
	}
	return nil
}

// loadWithInjection menjalankan load selama durasi skenario, memanggil inject setelah ~40% durasi,
// lalu menunggu pemulihan, mengulang command yang belum di-ack, dan memeriksa invarian.
func (s *session) loadWithInjection(ctx context.Context, inject func(context.Context) error) error {
	gen := &loadGenerator{clients: s.clients, ledger: s.ledger, workers: s.cfg.workers, logf: s.logf}
	loadCtx, cancel := context.WithTimeout(ctx, s.cfg.duration)
	defer cancel()
	done := make(chan struct{})
	go func() {
		gen.run(loadCtx)
		close(done)
	}()

	var injectErr error
	if inject != nil {
		if err := sleepCtx(loadCtx, s.cfg.duration*2/5); err == nil {
			s.logf("injecting fault (acked so far: %d, errors: %d)", s.ledger.acked.Load(), s.ledger.errors.Load())
			start := time.Now()
			injectErr = inject(ctx)
			s.logf("injection finished in %s (err: %v)", time.Since(start).Round(time.Millisecond), injectErr)
		}
	}
	<-done
	s.logf("load finished: %d acked commands (%d replayed), %d errors, %d instances driven to completion",
		s.ledger.acked.Load(), s.ledger.replayed.Load(), s.ledger.errors.Load(), gen.instancesDone.Load())
	if injectErr != nil {
		return fmt.Errorf("injection: %w", injectErr)
	}
	if s.ledger.acked.Load() == 0 {
		return fmt.Errorf("load generator acknowledged no commands")
	}
	return s.recoverAndVerify(ctx)
}

// recoverAndVerify menunggu /readyz, mengulang command pending dengan request_id yang sama, lalu
// memeriksa invarian.
func (s *session) recoverAndVerify(ctx context.Context) error {
	if err := s.waitReady(ctx); err != nil {
		return err
	}
	n, err := s.ledger.retryPending(ctx, s.clients, s.cfg.retryTimeout, s.logf)
	s.logf("retried %d un-acked commands with their original request_id (%d replayed in total)", n, s.ledger.replayed.Load())
	if err != nil {
		return err
	}
	if v := s.checkInvariants(ctx); len(v) > 0 {
		for _, line := range v {
			s.logf("VIOLATION: %s", line)
		}
		return fmt.Errorf("%d invariant violations; first: %s", len(v), v[0])
	}
	return nil
}

// engineNotRestarted memastikan engine tetap berjalan tanpa restart selama gangguan Postgres
// (pool harus menyambung ulang sendiri).
func (s *session) engineNotRestarted(ctx context.Context, startedBefore string) error {
	status, started, _, err := s.dc.containerState(ctx, serviceEngine)
	if err != nil {
		return err
	}
	if status != "running" || started != startedBefore {
		return fmt.Errorf("engine was not expected to restart: status=%s startedAt %s -> %s", status, startedBefore, started)
	}
	return nil
}
