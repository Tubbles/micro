package lsp

import (
	"github.com/creachadair/jrpc2"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/lsp/protocol"
)

// handleNotify is the jrpc2.ClientOptions.OnNotify callback wired in
// newClientOverChannel. It runs on a jrpc2-internal goroutine, never
// the main goroutine, so it does nothing but decode and hand the
// result to post; v1 only acts on publishDiagnostics, so this is a
// single-entry dispatch rather than a general routing table.
func (c *Client) handleNotify(req *jrpc2.Request) {
	if req.Method() != "textDocument/publishDiagnostics" {
		return
	}
	var params protocol.PublishDiagnosticsParams
	if err := req.UnmarshalParams(&params); err != nil {
		return
	}
	post(func() { applyPublishDiagnostics(c, params) })
}

// diagnosticsOwner is the gutter-message Owner used for every
// diagnostic c reports, scoping ClearMessages(owner) to this server so
// it never clobbers the diff gutter or another server's diagnostics on
// the same buffer.
func diagnosticsOwner(c *Client) string {
	return "lsp:" + c.Name
}

// applyPublishDiagnostics finds the buffer params.URI names, drops the
// batch if it is stale or unrecognized, and otherwise replaces that
// server's gutter messages on the buffer with the new set. It runs on
// the main goroutine (via post in handleNotify).
func applyPublishDiagnostics(c *Client, params protocol.PublishDiagnosticsParams) {
	sb, client, version, ok := documentByURI(params.URI)
	if !ok || client != c {
		// Unknown URI (never attached, or already closed), or a
		// notification from a client a restart has since replaced:
		// nothing to attach these diagnostics to.
		return
	}
	if params.Version != 0 && params.Version < version {
		// The server is still diagnosing a version we have since
		// edited past; a fresher publish for the new version is
		// expected to follow.
		return
	}

	buf := openBufferFor(sb)
	if buf == nil {
		return
	}

	owner := diagnosticsOwner(c)
	buf.ClearMessages(owner)

	encoding := string(c.PositionEncoding())
	for _, diag := range params.Diagnostics {
		start := PositionToLoc(buf.Line(int(diag.Range.Start.Line)), diag.Range.Start, encoding)
		end := PositionToLoc(buf.Line(int(diag.Range.End.Line)), diag.Range.End, encoding)
		buf.AddMessage(buffer.NewMessage(owner, diag.Message, start, end, severityToMsgType(diag.Severity)))
	}
}

// severityToMsgType maps an LSP DiagnosticSeverity onto micro's gutter
// message kinds. A zero Severity (the field is optional per spec) is
// treated the same as DiagnosticSeverityError, matching how most LSP
// clients interpret an unspecified severity.
func severityToMsgType(severity protocol.DiagnosticSeverity) buffer.MsgType {
	switch severity {
	case protocol.DiagnosticSeverityWarning:
		return buffer.MTWarning
	case protocol.DiagnosticSeverityInformation, protocol.DiagnosticSeverityHint:
		return buffer.MTInfo
	default:
		return buffer.MTError
	}
}

// openBufferFor returns the open *buffer.Buffer backing sb, if any.
// Gutter messages (AddMessage/ClearMessages) are methods on Buffer, not
// SharedBuffer, since they are the same public surface Lua linter
// plugins use; this bridges attach tracking's SharedBuffer identity to
// that surface. Returns nil if sb is not currently backing any open
// Buffer.
func openBufferFor(sb *buffer.SharedBuffer) *buffer.Buffer {
	for _, b := range buffer.OpenBuffers {
		if b.SharedBuffer == sb {
			return b
		}
	}
	return nil
}

// countDiagnostics counts buf's current gutter messages under owner by
// severity, for `> lsp status` and the statusline diagnostic count.
func countDiagnostics(buf *buffer.Buffer, owner string) (errors, warnings int) {
	for _, m := range buf.Messages {
		if m.Owner != owner {
			continue
		}
		switch m.Kind {
		case buffer.MTError:
			errors++
		case buffer.MTWarning:
			warnings++
		}
	}
	return errors, warnings
}
