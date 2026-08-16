package lsp

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/creachadair/jrpc2"
	"github.com/creachadair/jrpc2/channel"
	"github.com/creachadair/jrpc2/handler"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/lsp/protocol"
)

// newFakeServerWithPush is newFakeServer plus AllowPush, which
// server-initiated notifications (textDocument/publishDiagnostics in
// particular) require; jrpc2 rejects Server.Notify with
// ErrPushUnsupported unless the server was built with it. It is a
// separate helper rather than a newFakeServer parameter change so the
// many existing callers of newFakeServer are untouched.
func newFakeServerWithPush(t *testing.T, handlers handler.Map) (channel.Channel, *jrpc2.Server) {
	t.Helper()
	clientSide, serverSide := channel.Direct()
	srv := jrpc2.NewServer(handlers, &jrpc2.ServerOptions{AllowPush: true}).Start(serverSide)
	t.Cleanup(func() {
		srv.Stop()
		srv.Wait()
	})
	return clientSide, srv
}

// attachRealBufferForDiagnostics opens a real *buffer.Buffer (so it
// lands in buffer.OpenBuffers, which openBufferFor scans) against a
// fake server claiming filetype "unknown" under server name
// "testlang" (see newTestRegistry), and waits for the resulting
// didOpen. It returns the buffer and the URI the server sees it under.
//
// Callers must close b themselves (and drain rec.closed) before the
// test returns, rather than relying on t.Cleanup: newFakeServerWithPush
// stops the fake server on cleanup, and an in-flight, un-awaited
// didClose racing that shutdown trips the same jrpc2.Server.Stop race
// documented on lifecycleHandlers.
func attachRealBufferForDiagnostics(t *testing.T, text string, rec *syncRecorder, initializedCh <-chan struct{}) (*buffer.Buffer, protocol.DocumentURI) {
	t.Helper()

	oldLSP := config.GlobalSettings["lsp"]
	config.GlobalSettings["lsp"] = true
	t.Cleanup(func() { config.GlobalSettings["lsp"] = oldLSP })

	path := filepath.Join(t.TempDir(), "scratch.doesnotexist12345")
	b := buffer.NewBufferFromString(text, path, buffer.BTDefault)

	drainEvents(t, 2*time.Second)
	waitForSignal(t, initializedCh, "initialized")
	open := recv(t, rec.opened, "didOpen")

	return b, open.TextDocument.URI
}

func publishDiagnostics(t *testing.T, srv *jrpc2.Server, params protocol.PublishDiagnosticsParams) {
	t.Helper()
	if err := srv.Notify(context.Background(), "textDocument/publishDiagnostics", params); err != nil {
		t.Fatalf("srv.Notify(publishDiagnostics): %v", err)
	}
	drainEvents(t, 2*time.Second)
}

func TestApplyPublishDiagnosticsAddsOwnerScopedMessagesAndClearsStaleOnes(t *testing.T) {
	resetDocuments(t)
	withTempConfigDir(t)

	rec := newSyncRecorder()
	handlers, initializedCh := lifecycleHandlers(rec.handlers())
	ch, srv := newFakeServerWithPush(t, handlers)
	withTestRegistry(t, newTestRegistry("unknown", ch))

	b, uri := attachRealBufferForDiagnostics(t, "line one\nline two\n", rec, initializedCh)

	publishDiagnostics(t, srv, protocol.PublishDiagnosticsParams{
		URI: uri,
		Diagnostics: []protocol.Diagnostic{
			{
				Range:    protocol.Range{Start: protocol.Position{Line: 0, Character: 0}, End: protocol.Position{Line: 0, Character: 4}},
				Severity: protocol.DiagnosticSeverityError,
				Message:  "first batch",
			},
		},
	})

	if got := len(b.Messages); got != 1 {
		t.Fatalf("len(b.Messages) after first publish = %d, want 1", got)
	}
	m := b.Messages[0]
	if m.Owner != "lsp:testlang" {
		t.Errorf("Owner = %q, want %q", m.Owner, "lsp:testlang")
	}
	if m.Msg != "first batch" {
		t.Errorf("Msg = %q, want %q", m.Msg, "first batch")
	}
	if m.Kind != buffer.MTError {
		t.Errorf("Kind = %v, want MTError", m.Kind)
	}
	wantStart, wantEnd := buffer.Loc{X: 0, Y: 0}, buffer.Loc{X: 4, Y: 0}
	if m.Start != wantStart || m.End != wantEnd {
		t.Errorf("Start,End = %v,%v want %v,%v", m.Start, m.End, wantStart, wantEnd)
	}

	// A second publish must replace the first batch, not append to it,
	// and must not disturb messages under a different owner (the diff
	// gutter's precedent for owner scoping).
	b.AddMessage(buffer.NewMessageAtLine("diff", "unrelated", 2, buffer.MTInfo))

	publishDiagnostics(t, srv, protocol.PublishDiagnosticsParams{
		URI: uri,
		Diagnostics: []protocol.Diagnostic{
			{
				Range:    protocol.Range{Start: protocol.Position{Line: 1, Character: 0}, End: protocol.Position{Line: 1, Character: 4}},
				Severity: protocol.DiagnosticSeverityWarning,
				Message:  "second batch",
			},
		},
	})

	var lspMsgs, otherMsgs int
	for _, msg := range b.Messages {
		if msg.Owner == "lsp:testlang" {
			lspMsgs++
			if msg.Msg != "second batch" {
				t.Errorf("surviving lsp message = %q, want %q", msg.Msg, "second batch")
			}
			if msg.Kind != buffer.MTWarning {
				t.Errorf("surviving lsp message Kind = %v, want MTWarning", msg.Kind)
			}
		} else {
			otherMsgs++
		}
	}
	if lspMsgs != 1 {
		t.Errorf("lsp:testlang messages after second publish = %d, want 1", lspMsgs)
	}
	if otherMsgs != 1 {
		t.Errorf("non-lsp messages after second publish = %d, want 1 (diff gutter must survive ClearMessages)", otherMsgs)
	}

	b.Close()
	recv(t, rec.closed, "didClose")
}

func TestApplyPublishDiagnosticsIgnoresUnknownURI(t *testing.T) {
	resetDocuments(t)
	withTempConfigDir(t)

	rec := newSyncRecorder()
	handlers, initializedCh := lifecycleHandlers(rec.handlers())
	ch, srv := newFakeServerWithPush(t, handlers)
	withTestRegistry(t, newTestRegistry("unknown", ch))

	b, _ := attachRealBufferForDiagnostics(t, "hello\n", rec, initializedCh)

	publishDiagnostics(t, srv, protocol.PublishDiagnosticsParams{
		URI: "file:///never/attached.go",
		Diagnostics: []protocol.Diagnostic{
			{Range: protocol.Range{}, Message: "should be dropped"},
		},
	})

	if got := len(b.Messages); got != 0 {
		t.Errorf("len(b.Messages) after publish for an unknown URI = %d, want 0", got)
	}

	b.Close()
	recv(t, rec.closed, "didClose")
}

func TestApplyPublishDiagnosticsDropsStaleVersion(t *testing.T) {
	resetDocuments(t)
	withTempConfigDir(t)

	rec := newSyncRecorder()
	handlers, initializedCh := lifecycleHandlers(rec.handlers())
	ch, srv := newFakeServerWithPush(t, handlers)
	withTestRegistry(t, newTestRegistry("unknown", ch))

	b, uri := attachRealBufferForDiagnostics(t, "a\n", rec, initializedCh)

	// Edit once so the tracked document version moves from 1 to 2.
	b.Insert(buffer.Loc{X: 0, Y: 0}, "X")
	recv(t, rec.changed, "didChange")

	publishDiagnostics(t, srv, protocol.PublishDiagnosticsParams{
		URI:         uri,
		Version:     1, // stale: the document has since moved to version 2
		Diagnostics: []protocol.Diagnostic{{Message: "stale"}},
	})
	if got := len(b.Messages); got != 0 {
		t.Fatalf("len(b.Messages) after a stale-version publish = %d, want 0", got)
	}

	publishDiagnostics(t, srv, protocol.PublishDiagnosticsParams{
		URI:         uri,
		Version:     2, // current
		Diagnostics: []protocol.Diagnostic{{Message: "current"}},
	})
	if got := len(b.Messages); got != 1 {
		t.Fatalf("len(b.Messages) after a current-version publish = %d, want 1", got)
	}
	if b.Messages[0].Msg != "current" {
		t.Errorf("Msg = %q, want %q", b.Messages[0].Msg, "current")
	}

	b.Close()
	recv(t, rec.closed, "didClose")
}

func TestSeverityToMsgType(t *testing.T) {
	cases := []struct {
		severity protocol.DiagnosticSeverity
		want     buffer.MsgType
	}{
		{0, buffer.MTError}, // unspecified severity defaults to error
		{protocol.DiagnosticSeverityError, buffer.MTError},
		{protocol.DiagnosticSeverityWarning, buffer.MTWarning},
		{protocol.DiagnosticSeverityInformation, buffer.MTInfo},
		{protocol.DiagnosticSeverityHint, buffer.MTInfo},
	}
	for _, c := range cases {
		if got := severityToMsgType(c.severity); got != c.want {
			t.Errorf("severityToMsgType(%v) = %v, want %v", c.severity, got, c.want)
		}
	}
}
