// Package api adalah kontrak publik modul health.
// Modul lain hanya boleh meng-import paket ini, tidak boleh paket internal health lainnya.
package api

import "context"

// Checker memberi tahu modul lain apakah engine sedang sehat.
type Checker interface {
	IsHealthy(ctx context.Context) bool
}
