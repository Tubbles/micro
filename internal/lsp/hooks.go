package lsp

import (
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
	lua "github.com/yuin/gopher-lua"
)

// lspAttachArgs builds the onLspAttach plugin hook's argument list:
// the attached buffer's path and the server's registered name. Kept
// separate from fireLspAttach so the argument construction is
// testable without a live Lua VM.
func lspAttachArgs(path, server string) []lua.LValue {
	return []lua.LValue{lua.LString(path), lua.LString(server)}
}

// lspDetachArgs builds the onLspDetach plugin hook's argument list,
// the same (path, server) shape as lspAttachArgs.
func lspDetachArgs(path, server string) []lua.LValue {
	return []lua.LValue{lua.LString(path), lua.LString(server)}
}

// onDiagnosticsArgs builds the onDiagnostics plugin hook's argument
// list: the diagnosed buffer's path and how many diagnostics were
// published in this batch.
func onDiagnosticsArgs(path string, count int) []lua.LValue {
	return []lua.LValue{lua.LString(path), lua.LNumber(count)}
}

// runHook calls a plugin hook function with args and logs (rather
// than surfacing) any error a misbehaving plugin returns: attach,
// detach, and diagnostics application are load-bearing editor
// behavior that must not break because a plugin's hook function
// errored. This matches how other non-fatal plugin-fn call sites
// handle RunPluginFn's error, e.g. onBufferOpen in
// internal/buffer/buffer.go and onBufPaneOpen in
// internal/action/bufpane.go.
func runHook(fn string, args ...lua.LValue) {
	if err := config.RunPluginFn(fn, args...); err != nil {
		screen.TermMessage(err)
	}
}

// fireLspAttach runs the onLspAttach(path, server) plugin hook. Called
// once a buffer's document is actually open on the server (see
// attach), not merely reserved while the server is still starting.
func fireLspAttach(path, server string) {
	runHook("onLspAttach", lspAttachArgs(path, server)...)
}

// fireLspDetach runs the onLspDetach(path, server) plugin hook. Called
// from detach, mirroring fireLspAttach: only for a document that was
// actually attached (had a client), not a reservation that never
// resolved.
func fireLspDetach(path, server string) {
	runHook("onLspDetach", lspDetachArgs(path, server)...)
}

// fireOnDiagnostics runs the onDiagnostics(path, count) plugin hook.
// Called at the end of applyPublishDiagnostics, once a batch of
// diagnostics has actually been applied to the buffer's gutter
// messages.
func fireOnDiagnostics(path string, count int) {
	runHook("onDiagnostics", onDiagnosticsArgs(path, count)...)
}
