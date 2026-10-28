package recovery

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeVerifier struct {
	mismatches []Mismatch
	repaired   []string
	healAfter  bool
}

func (f *fakeVerifier) Name() string { return "fake" }

func (f *fakeVerifier) VerifyOnStart(context.Context) (Report, error) {
	if f.healAfter && len(f.repaired) > 0 {
		return Report{Checked: 1}, nil
	}
	return Report{Checked: 1, Mismatches: f.mismatches}, nil
}

func (f *fakeVerifier) Repair(_ context.Context, subjects []string) error {
	f.repaired = append(f.repaired, subjects...)
	return nil
}

type fakeParticipant struct{ called bool }

func (p *fakeParticipant) Name() string { return "participant" }

func (p *fakeParticipant) Recover(context.Context) error {
	p.called = true
	return nil
}

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestRunnerModes(t *testing.T) {
	bad := []Mismatch{{Subject: "a", Kind: "x"}, {Subject: "a", Kind: "y"}, {Subject: "b", Kind: "x"}}

	v, p := &fakeVerifier{mismatches: bad}, &fakeParticipant{}
	if _, err := (&Runner{Verifiers: []Verifier{v}, Participants: []Participant{p}, OnMismatch: OnMismatchFail, Log: quiet}).Run(context.Background()); !errors.Is(err, ErrMismatch) {
		t.Fatalf("fail mode: %v", err)
	}
	if p.called {
		t.Fatal("participants must not run after a mismatch in fail mode")
	}

	v, p = &fakeVerifier{mismatches: bad}, &fakeParticipant{}
	rep, err := (&Runner{Verifiers: []Verifier{v}, Participants: []Participant{p}, OnMismatch: OnMismatchWarn, Log: quiet}).Run(context.Background())
	if err != nil || len(rep.Mismatches) != 3 || !p.called {
		t.Fatalf("warn mode: %v %+v participant=%v", err, rep, p.called)
	}

	v = &fakeVerifier{mismatches: bad, healAfter: true}
	if _, err := (&Runner{Verifiers: []Verifier{v}, OnMismatch: OnMismatchRebuild, Log: quiet}).Run(context.Background()); err != nil {
		t.Fatalf("rebuild mode: %v", err)
	}
	if len(v.repaired) != 2 || v.repaired[0] != "a" || v.repaired[1] != "b" {
		t.Fatalf("repaired %v, want unique subjects a, b", v.repaired)
	}

	v = &fakeVerifier{mismatches: bad}
	if _, err := (&Runner{Verifiers: []Verifier{v}, OnMismatch: OnMismatchRebuild, Log: quiet}).Run(context.Background()); !errors.Is(err, ErrMismatch) {
		t.Fatalf("rebuild that does not heal must fail: %v", err)
	}
}

func TestParseOnMismatch(t *testing.T) {
	for in, want := range map[string]OnMismatch{"": OnMismatchFail, "FAIL": OnMismatchFail, "rebuild": OnMismatchRebuild, " warn ": OnMismatchWarn} {
		got, err := ParseOnMismatch(in)
		if err != nil || got != want {
			t.Errorf("ParseOnMismatch(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := ParseOnMismatch("ignore"); err == nil {
		t.Error("unknown mode accepted")
	}
}

func TestGateBlocksUntilReady(t *testing.T) {
	g := NewGate()
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	mux := http.NewServeMux()
	mux.Handle("/readyz", g.ReadyzHandler())
	mux.Handle("/healthz", inner)
	mux.Handle("/rpc", inner)
	h := g.Middleware(mux, "/healthz", "/readyz")

	status := func(path string) int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec.Code
	}
	if status("/readyz") != http.StatusServiceUnavailable || status("/rpc") != http.StatusServiceUnavailable || status("/healthz") != http.StatusTeapot {
		t.Fatal("before ready: readyz and rpc must be 503, healthz must pass")
	}
	g.SetReady()
	if status("/readyz") != http.StatusOK || status("/rpc") != http.StatusTeapot {
		t.Fatal("after ready: readyz 200 and rpc must pass")
	}
}
