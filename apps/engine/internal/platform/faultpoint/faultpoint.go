//go:build !faultinject

// Package faultpoint menyediakan titik injeksi kegagalan untuk uji pemulihan (rencana Fase 3, 11.1).
// Tanpa build tag faultinject, Hit selalu mengembalikan nil dan dikompilasi menjadi no-op.
package faultpoint

import "context"

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

// Hit tidak melakukan apa pun tanpa build tag faultinject.
func Hit(context.Context, string) error { return nil }
