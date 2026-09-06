// Package lsp implements micro's native Language Server Protocol
// client: process lifecycle, JSON-RPC framing over stdio via
// creachadair/jrpc2, the initialize/initialized/shutdown handshake, and
// a named-server registry keyed by (server name, workspace root). This
// is the transport chunk of a three-part effort: document sync
// (didOpen/didChange/didClose and the Loc<->UTF-16 conversion layer)
// and diagnostics/hover/goto-definition plus the `> lsp` commands land
// in later chunks. See work/roadmap-2026-07-decisions.md D-38 through
// D-44 for the design rationale.
package lsp

import "github.com/micro-editor/micro/v2/internal/screen"

// Events is the channel through which lsp delivers effects to the main
// goroutine. Reader/writer goroutines and jrpc2 callbacks never touch
// buffer, screen, or Lua state directly; they build a closure and hand
// it to post, mirroring shell.Jobs. cmd/micro/micro.go's DoEvent drains
// this channel as a new case in its select, exactly like shell.Jobs.
var Events chan func()

func init() {
	Events = make(chan func(), 100)
}

// post enqueues fn to run on the main goroutine and wakes the main loop
// so it notices without waiting for the next input event. It never
// blocks: if Events is full the call is dropped rather than stalling
// the calling goroutine, since lsp callbacks run on arbitrary jrpc2
// goroutines that must not stall on a slow main loop.
func post(fn func()) {
	select {
	case Events <- fn:
	default:
	}
	screen.Redraw()
}
