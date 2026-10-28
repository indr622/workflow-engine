// Package recovery menjalankan pemulihan saat start (ADR-0016): verifikasi konsistensi proyeksi
// terhadap record, reaksi atas ketidakcocokan, lalu Participant.Recover setiap modul, sebelum
// engine dinyatakan ready.
package recovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Mismatch adalah satu ketidakcocokan proyeksi.
type Mismatch struct {
	// Subject adalah entitas yang menyimpang, mis. process instance key.
	Subject string `json:"subject"`
	Kind    string `json:"kind"`
	Detail  string `json:"detail"`
}

// Report adalah hasil verifikasi.
type Report struct {
	Checked    int           `json:"checked"`
	Mismatches []Mismatch    `json:"mismatches"`
	Duration   time.Duration `json:"durationNs"`
}

// Subjects mengembalikan subject unik yang menyimpang, urutan kemunculan.
func (r Report) Subjects() []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range r.Mismatches {
		if !seen[m.Subject] {
			seen[m.Subject] = true
			out = append(out, m.Subject)
		}
	}
	return out
}

// Verifier memeriksa konsistensi saat start dan bisa memperbaiki subject yang menyimpang.
type Verifier interface {
	Name() string
	VerifyOnStart(ctx context.Context) (Report, error)
	Repair(ctx context.Context, subjects []string) error
}

// Participant membangun ulang state in-memory turunan tabel sebelum engine ready.
type Participant interface {
	Name() string
	Recover(ctx context.Context) error
}

// OnMismatch menentukan reaksi atas ketidakcocokan.
type OnMismatch string

// Reaksi atas ketidakcocokan.
const (
	OnMismatchFail    OnMismatch = "fail"
	OnMismatchRebuild OnMismatch = "rebuild"
	OnMismatchWarn    OnMismatch = "warn"
)

// ParseOnMismatch memvalidasi nilai konfigurasi.
func ParseOnMismatch(s string) (OnMismatch, error) {
	switch v := OnMismatch(strings.ToLower(strings.TrimSpace(s))); v {
	case OnMismatchFail, OnMismatchRebuild, OnMismatchWarn:
		return v, nil
	case "":
		return OnMismatchFail, nil
	default:
		return "", fmt.Errorf("unknown recovery mismatch mode %q (fail, rebuild, warn)", s)
	}
}

// ErrMismatch dikembalikan Runner bila ada ketidakcocokan dalam mode fail.
var ErrMismatch = errors.New("projection mismatch detected")

// Runner menjalankan verifier lalu participant, berurutan sesuai registrasi.
type Runner struct {
	Verifiers    []Verifier
	Participants []Participant
	OnMismatch   OnMismatch
	Timeout      time.Duration
	Log          *slog.Logger
	Gate         *Gate
}

// Run menjalankan pemulihan. Mode fail mengembalikan ErrMismatch; mode rebuild memperbaiki lalu
// memverifikasi ulang; mode warn hanya mencatat.
func (r *Runner) Run(ctx context.Context) (Report, error) {
	log := r.Log
	if log == nil {
		log = slog.Default()
	}
	log = log.With(slog.String("component", "recovery"))
	if r.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.Timeout)
		defer cancel()
	}
	started := time.Now()
	log.Info("recovery started", slog.Int("verifiers", len(r.Verifiers)), slog.Int("participants", len(r.Participants)))
	var total Report
	for _, v := range r.Verifiers {
		r.Gate.setPhase("verify:" + v.Name())
		rep, err := v.VerifyOnStart(ctx)
		if err != nil {
			return total, fmt.Errorf("verify %s: %w", v.Name(), err)
		}
		for _, m := range rep.Mismatches {
			log.Error("projection mismatch", slog.String("verifier", v.Name()), slog.String("subject", m.Subject),
				slog.String("kind", m.Kind), slog.String("diff", m.Detail))
		}
		if len(rep.Mismatches) > 0 {
			switch r.OnMismatch {
			case OnMismatchRebuild:
				r.Gate.setPhase("repair:" + v.Name())
				if err := v.Repair(ctx, rep.Subjects()); err != nil {
					return total, fmt.Errorf("repair %s: %w", v.Name(), err)
				}
				again, err := v.VerifyOnStart(ctx)
				if err != nil {
					return total, err
				}
				if len(again.Mismatches) > 0 {
					return total, fmt.Errorf("%w after rebuild: %d mismatches remain in %s", ErrMismatch, len(again.Mismatches), v.Name())
				}
				log.Warn("projection repaired", slog.String("verifier", v.Name()), slog.Int("subjects", len(rep.Subjects())))
			case OnMismatchWarn:
			default:
				total.Checked += rep.Checked
				total.Mismatches = append(total.Mismatches, rep.Mismatches...)
				return total, fmt.Errorf("%w: %d in %s; run `engine verify --all` and `engine rebuild --instance <key>`",
					ErrMismatch, len(rep.Mismatches), v.Name())
			}
		}
		total.Checked += rep.Checked
		total.Mismatches = append(total.Mismatches, rep.Mismatches...)
	}
	for _, p := range r.Participants {
		r.Gate.setPhase("recover:" + p.Name())
		if err := p.Recover(ctx); err != nil {
			return total, fmt.Errorf("recover %s: %w", p.Name(), err)
		}
	}
	total.Duration = time.Since(started)
	log.Info("recovery finished", slog.Int("checked", total.Checked), slog.Int("mismatched", len(total.Mismatches)),
		slog.Int64("duration_ms", total.Duration.Milliseconds()))
	return total, nil
}

// Gate adalah gerbang readiness: /readyz 503 dan RPC Unavailable sampai Set(true).
type Gate struct {
	mu    sync.RWMutex
	ready bool
	phase string
}

// NewGate membuat gerbang yang belum ready.
func NewGate() *Gate { return &Gate{phase: "starting"} }

// SetReady menandai engine siap melayani.
func (g *Gate) SetReady() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.ready, g.phase = true, "ready"
}

// Ready mengembalikan status readiness.
func (g *Gate) Ready() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.ready
}

func (g *Gate) setPhase(p string) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.phase = p
}

// Phase mengembalikan tahap pemulihan yang sedang berjalan.
func (g *Gate) Phase() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.phase
}

// ReadyzHandler menjawab /readyz: 200 bila ready, 503 dengan tahap yang sedang berjalan bila belum.
func (g *Gate) ReadyzHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		ready, phase := g.Ready(), g.Phase()
		w.Header().Set("Content-Type", "application/json")
		if !ready {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ready": ready, "phase": phase})
	})
}

// Middleware menolak request dengan 503 selama engine belum ready, kecuali path yang dikecualikan
// (liveness dan readiness).
func (g *Gate) Middleware(next http.Handler, exempt ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !g.Ready() {
			for _, p := range exempt {
				if r.URL.Path == p {
					next.ServeHTTP(w, r)
					return
				}
			}
			w.Header().Set("Retry-After", "1")
			http.Error(w, "engine is recovering: "+g.Phase(), http.StatusServiceUnavailable)
			return
		}
		next.ServeHTTP(w, r)
	})
}
