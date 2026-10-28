//go:build faultinject

// Package faultpoint menyediakan titik injeksi kegagalan untuk uji pemulihan (rencana Fase 3, 11.1).
// Dengan build tag faultinject, ENGINE_FAULTPOINTS="nama=aksi,..." mengaktifkan titik: panic
// (simulasi crash proses), error (kembalikan ErrInjected), atau sleep:<durasi>. Test juga bisa
// memasang fungsi sendiri lewat Set.
package faultpoint

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// Nama titik injeksi yang dipasang di engine.
const (
	CommandAfterLock         = "command.after_lock"
	CommandAfterAppend       = "command.after_append"
	CommandBeforeCommit      = "command.before_commit"
	CommandAfterCommit       = "command.after_commit"
	OutboxAfterHandle        = "outbox.after_handle"
	OutboxBeforeCursorCommit = "outbox.before_cursor_commit"
	IdempotencyAfterClaim    = "idempotency.after_claim"
	RebuildMidStream         = "rebuild.mid_stream"
	SnapshotBeforeWrite      = "snapshot.before_write"
)

// ErrInjected dikembalikan aksi "error".
var ErrInjected = errors.New("injected fault")

// Action dijalankan saat titik aktif.
type Action func(ctx context.Context) error

var (
	mu      sync.RWMutex
	actions = map[string]Action{}
)

func init() {
	for _, spec := range strings.Split(os.Getenv("ENGINE_FAULTPOINTS"), ",") {
		name, act, ok := strings.Cut(strings.TrimSpace(spec), "=")
		if !ok || name == "" {
			continue
		}
		a, err := Parse(act)
		if err != nil {
			panic(fmt.Sprintf("ENGINE_FAULTPOINTS %q: %v", spec, err))
		}
		actions[name] = a
	}
}

// Parse mengubah teks aksi (panic, error, sleep:<durasi>) menjadi Action.
func Parse(s string) (Action, error) {
	switch {
	case s == "panic":
		return func(context.Context) error { panic("injected crash") }, nil
	case s == "error":
		return func(context.Context) error { return ErrInjected }, nil
	case strings.HasPrefix(s, "sleep:"):
		d, err := time.ParseDuration(strings.TrimPrefix(s, "sleep:"))
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d):
				return nil
			}
		}, nil
	default:
		return nil, fmt.Errorf("unknown fault action %q", s)
	}
}

// Set memasang aksi untuk satu titik; nil menghapusnya.
func Set(name string, a Action) {
	mu.Lock()
	defer mu.Unlock()
	if a == nil {
		delete(actions, name)
		return
	}
	actions[name] = a
}

// Reset menghapus semua aksi.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	actions = map[string]Action{}
}

// Hit menjalankan aksi titik bila aktif.
func Hit(ctx context.Context, name string) error {
	mu.RLock()
	a := actions[name]
	mu.RUnlock()
	if a == nil {
		return nil
	}
	return a(ctx)
}
