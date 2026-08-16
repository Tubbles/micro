package lsp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creachadair/jrpc2/channel"
	"github.com/creachadair/jrpc2/handler"
	lua "github.com/yuin/gopher-lua"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/lsp/protocol"
	ulua "github.com/micro-editor/micro/v2/internal/lua"
)

// The tests in this file construct real *buffer.Buffer / *buffer.SharedBuffer
// values, which requires the same minimal runtime setup buffer's own
// tests perform (see internal/buffer/buffer_test.go's init).
func init() {
	ulua.L = lua.NewState()
	config.InitRuntimeFiles(false)
	config.InitGlobalSettings()
	config.GlobalSettings["backup"] = false
	config.GlobalSettings["fastdirty"] = true
}

// newTestSharedBuffer builds a minimal *buffer.SharedBuffer directly
// (bypassing buffer.NewBuffer's file/settings/plugin machinery) for
// tests that only need to drive handleBufferEvent's attach filter and
// don't care about a real Buffer's cursors, undo stack, etc.
func newTestSharedBuffer(text, absPath string, btype buffer.BufType) *buffer.SharedBuffer {
	sb := &buffer.SharedBuffer{}
	sb.LineArray = buffer.NewLineArray(uint64(len(text)), buffer.FFUnix, strings.NewReader(text))
	sb.Type = btype
	sb.AbsPath = absPath
	sb.Settings = make(map[string]any)
	return sb
}

// syncRecorder is a fake language server's recording of the document
// sync notifications it received, standing in for the "FAKE client"
// the chunk asks for: it reuses chunk A's channel.Direct()-backed
// in-process jrpc2 server seam (see newFakeServer in client_test.go)
// rather than requiring a real gopls.
type syncRecorder struct {
	opened  chan protocol.DidOpenTextDocumentParams
	changed chan protocol.DidChangeTextDocumentParams
	closed  chan protocol.DidCloseTextDocumentParams
	saved   chan protocol.DidSaveTextDocumentParams
}

func newSyncRecorder() *syncRecorder {
	return &syncRecorder{
		opened:  make(chan protocol.DidOpenTextDocumentParams, 8),
		changed: make(chan protocol.DidChangeTextDocumentParams, 8),
		closed:  make(chan protocol.DidCloseTextDocumentParams, 8),
		saved:   make(chan protocol.DidSaveTextDocumentParams, 8),
	}
}

func (s *syncRecorder) handlers() handler.Map {
	return handler.Map{
		"textDocument/didOpen": handler.New(func(_ context.Context, p protocol.DidOpenTextDocumentParams) error {
			s.opened <- p
			return nil
		}),
		"textDocument/didChange": handler.New(func(_ context.Context, p protocol.DidChangeTextDocumentParams) error {
			s.changed <- p
			return nil
		}),
		"textDocument/didClose": handler.New(func(_ context.Context, p protocol.DidCloseTextDocumentParams) error {
			s.closed <- p
			return nil
		}),
		"textDocument/didSave": handler.New(func(_ context.Context, p protocol.DidSaveTextDocumentParams) error {
			s.saved <- p
			return nil
		}),
	}
}

// recv waits for a value on ch, failing the test if none arrives
// within the timeout.
func recv[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
	var zero T
	return zero
}

// assertNoneReceived fails the test if a value is already waiting on
// ch. It does not wait: callers use it right after a synchronous call
// that, per attach's eligibility check, should never have queued
// anything to send in the first place.
func assertNoneReceived[T any](t *testing.T, ch <-chan T, what string) {
	t.Helper()
	select {
	case v := <-ch:
		t.Fatalf("unexpected %s: %+v", what, v)
	default:
	}
}

// resetDocuments points the package-level document tracker at a fresh,
// empty one for the duration of the test, so attach-state left over
// from a previous test can't leak in.
func resetDocuments(t *testing.T) {
	t.Helper()
	old := documents
	documents = newDocumentTracker()
	t.Cleanup(func() { documents = old })
}

// withTestRegistry points getRegistry at r for the duration of the
// test, the same substitution seam Registry.starter provides for
// GetOrStart (see registry_test.go).
func withTestRegistry(t *testing.T, r *Registry) {
	t.Helper()
	old := getRegistry
	getRegistry = func() *Registry { return r }
	t.Cleanup(func() { getRegistry = old })
}

// newTestRegistry returns a Registry with a single "testlang" server
// definition claiming filetype, whose starter hands back a Client
// wired to ch instead of spawning a real language server process.
func newTestRegistry(filetype string, ch channel.Channel) *Registry {
	return &Registry{
		definitions: map[string]ServerDefinition{
			"testlang": {Filetypes: []string{filetype}},
		},
		clients: make(map[registryKey]*Client),
		starter: func(name string, def ServerDefinition, root string) (*Client, error) {
			return newClientOverChannel(name, root, ch), nil
		},
	}
}

func TestAttachSkipsIneligibleBuffers(t *testing.T) {
	resetDocuments(t)

	// None of the cases below should be eligible for attachment, so
	// getRegistry must never even be consulted: fail loudly if it is,
	// rather than wiring up a fake language server that would sit
	// unconnected (no *Client is ever constructed over it in this
	// test, since eligible() should reject every case up front).
	old := getRegistry
	getRegistry = func() *Registry {
		t.Fatal("getRegistry was called for an ineligible buffer")
		return nil
	}
	t.Cleanup(func() { getRegistry = old })

	cases := []struct {
		name string
		sb   *buffer.SharedBuffer
	}{
		{
			name: "lsp disabled",
			sb: func() *buffer.SharedBuffer {
				sb := newTestSharedBuffer("package main\n", "/fake/a.testlang", buffer.BTDefault)
				sb.Settings["lsp"] = false
				sb.Settings["filetype"] = "testlang"
				return sb
			}(),
		},
		{
			name: "scratch buffer",
			sb: func() *buffer.SharedBuffer {
				sb := newTestSharedBuffer("package main\n", "/fake/b.testlang", buffer.BTScratch)
				sb.Settings["lsp"] = true
				sb.Settings["filetype"] = "testlang"
				return sb
			}(),
		},
		{
			name: "unnamed buffer",
			sb: func() *buffer.SharedBuffer {
				sb := newTestSharedBuffer("package main\n", "", buffer.BTDefault)
				sb.Settings["lsp"] = true
				sb.Settings["filetype"] = "testlang"
				return sb
			}(),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Change/Close on a never-attached buffer must also stay
			// silent; nothing in the filter is Open-specific.
			handleBufferEvent(c.sb, buffer.BufferEventOpen)
			handleBufferEvent(c.sb, buffer.BufferEventChange)
			handleBufferEvent(c.sb, buffer.BufferEventClose)

			documents.mu.Lock()
			attached := len(documents.docs)
			documents.mu.Unlock()
			if attached != 0 {
				t.Errorf("documents.docs has %d entries after an ineligible buffer's lifecycle, want 0", attached)
			}
		})
	}
}

func TestAttachChangeCloseLifecycleSendsNotificationsInOrder(t *testing.T) {
	resetDocuments(t)

	rec := newSyncRecorder()
	handlers, initializedCh := lifecycleHandlers(rec.handlers())
	ch := newFakeServer(t, handlers)
	withTestRegistry(t, newTestRegistry("testlang", ch))

	sb := newTestSharedBuffer("package main\n", "/fake/main.testlang", buffer.BTDefault)
	sb.Settings["lsp"] = true
	sb.Settings["filetype"] = "testlang"

	handleBufferEvent(sb, buffer.BufferEventOpen)
	drainEvents(t, 2*time.Second) // runs GetOrStart's onReady, which sends didOpen
	waitForSignal(t, initializedCh, "initialized")

	open := recv(t, rec.opened, "didOpen")
	if open.TextDocument.Text != "package main\n" {
		t.Errorf("didOpen text = %q, want %q", open.TextDocument.Text, "package main\n")
	}
	if open.TextDocument.Version != 1 {
		t.Errorf("didOpen version = %d, want 1", open.TextDocument.Version)
	}
	if open.TextDocument.LanguageID != "testlang" {
		t.Errorf("didOpen languageId = %q, want %q", open.TextDocument.LanguageID, "testlang")
	}

	// A second Open event for the same URI (e.g. a second split on the
	// same file reusing the same SharedBuffer) must not re-attach.
	handleBufferEvent(sb, buffer.BufferEventOpen)
	assertNoneReceived(t, rec.opened, "a second didOpen")

	handleBufferEvent(sb, buffer.BufferEventChange)
	change := recv(t, rec.changed, "didChange")
	if change.TextDocument.Version != 2 {
		t.Errorf("didChange version = %d, want 2", change.TextDocument.Version)
	}
	if len(change.ContentChanges) != 1 || change.ContentChanges[0].Text != "package main\n" {
		t.Errorf("didChange contentChanges = %+v, want a single full-text change", change.ContentChanges)
	}
	if change.TextDocument.URI != open.TextDocument.URI {
		t.Errorf("didChange URI = %q, want %q", change.TextDocument.URI, open.TextDocument.URI)
	}

	handleBufferEvent(sb, buffer.BufferEventClose)
	closeParams := recv(t, rec.closed, "didClose")
	if closeParams.TextDocument.URI != open.TextDocument.URI {
		t.Errorf("didClose URI = %q, want %q", closeParams.TextDocument.URI, open.TextDocument.URI)
	}

	// Once closed, further edits must not resurrect the document.
	handleBufferEvent(sb, buffer.BufferEventChange)
	assertNoneReceived(t, rec.changed, "didChange after didClose")
}

func TestNotifySaveSendsFullTextWhenAttached(t *testing.T) {
	resetDocuments(t)

	rec := newSyncRecorder()
	handlers, initializedCh := lifecycleHandlers(rec.handlers())
	ch := newFakeServer(t, handlers)
	withTestRegistry(t, newTestRegistry("testlang", ch))

	sb := newTestSharedBuffer("package main\n", "/fake/main.testlang", buffer.BTDefault)
	sb.Settings["lsp"] = true
	sb.Settings["filetype"] = "testlang"

	handleBufferEvent(sb, buffer.BufferEventOpen)
	drainEvents(t, 2*time.Second)
	waitForSignal(t, initializedCh, "initialized")
	recv(t, rec.opened, "didOpen")

	handleBufferEvent(sb, buffer.BufferEventSave)
	saved := recv(t, rec.saved, "didSave")
	if saved.Text != "package main\n" {
		t.Errorf("didSave text = %q, want %q", saved.Text, "package main\n")
	}
}

func TestNotifyChangeVersionIncreasesMonotonically(t *testing.T) {
	resetDocuments(t)

	rec := newSyncRecorder()
	handlers, initializedCh := lifecycleHandlers(rec.handlers())
	ch := newFakeServer(t, handlers)
	withTestRegistry(t, newTestRegistry("testlang", ch))

	sb := newTestSharedBuffer("a\n", "/fake/main.testlang", buffer.BTDefault)
	sb.Settings["lsp"] = true
	sb.Settings["filetype"] = "testlang"

	handleBufferEvent(sb, buffer.BufferEventOpen)
	drainEvents(t, 2*time.Second)
	waitForSignal(t, initializedCh, "initialized")

	open := recv(t, rec.opened, "didOpen")
	if open.TextDocument.Version != 1 {
		t.Fatalf("didOpen version = %d, want 1", open.TextDocument.Version)
	}

	for i, want := range []int32{2, 3, 4} {
		handleBufferEvent(sb, buffer.BufferEventChange)
		got := recv(t, rec.changed, "didChange")
		if got.TextDocument.Version != want {
			t.Errorf("didChange[%d] version = %d, want %d", i, got.TextDocument.Version, want)
		}
	}
}

// TestSaveAsKeepsOriginalURIAndDoesNotLeak proves the tracker keys off
// the buffer's stable identity, not its live AbsPath. buffer.saveToFile
// mutates AbsPath before firing BufferEventSave for a "Save As", so a
// URI-keyed tracker would look up the new path, find nothing, and
// silently stop syncing while leaking the old document open on the
// server forever.
func TestSaveAsKeepsOriginalURIAndDoesNotLeak(t *testing.T) {
	resetDocuments(t)

	rec := newSyncRecorder()
	handlers, initializedCh := lifecycleHandlers(rec.handlers())
	ch := newFakeServer(t, handlers)
	withTestRegistry(t, newTestRegistry("testlang", ch))

	sb := newTestSharedBuffer("package main\n", "/fake/main.testlang", buffer.BTDefault)
	sb.Settings["lsp"] = true
	sb.Settings["filetype"] = "testlang"

	handleBufferEvent(sb, buffer.BufferEventOpen)
	drainEvents(t, 2*time.Second)
	waitForSignal(t, initializedCh, "initialized")
	open := recv(t, rec.opened, "didOpen")
	originalURI := open.TextDocument.URI

	// Simulate the AbsPath mutation a "Save As" performs before the
	// BufferEventSave fires.
	sb.AbsPath = "/fake/renamed.testlang"

	handleBufferEvent(sb, buffer.BufferEventSave)
	saved := recv(t, rec.saved, "didSave")
	if saved.TextDocument.URI != originalURI {
		t.Errorf("didSave URI = %q, want original %q after a rename", saved.TextDocument.URI, originalURI)
	}

	handleBufferEvent(sb, buffer.BufferEventChange)
	change := recv(t, rec.changed, "didChange")
	if change.TextDocument.URI != originalURI {
		t.Errorf("didChange URI = %q, want original %q after a rename", change.TextDocument.URI, originalURI)
	}

	handleBufferEvent(sb, buffer.BufferEventClose)
	closed := recv(t, rec.closed, "didClose")
	if closed.TextDocument.URI != originalURI {
		t.Errorf("didClose URI = %q, want original %q; a mismatched key would leak the document", closed.TextDocument.URI, originalURI)
	}

	documents.mu.Lock()
	n := len(documents.docs)
	documents.mu.Unlock()
	if n != 0 {
		t.Errorf("documents.docs has %d entries after close, want 0 (leaked document)", n)
	}
}

// TestCloseDuringAttachSuppressesDidOpen proves that closing a buffer
// while its language server is still starting cancels the pending
// attach: no didOpen is sent for a document the client no longer
// manages, which would otherwise leak it open on the server (with no
// didClose ever following, since the buffer is already gone).
func TestCloseDuringAttachSuppressesDidOpen(t *testing.T) {
	resetDocuments(t)

	rec := newSyncRecorder()
	handlers, initializedCh := lifecycleHandlers(rec.handlers())
	ch := newFakeServer(t, handlers)
	withTestRegistry(t, newTestRegistry("testlang", ch))

	sb := newTestSharedBuffer("package main\n", "/fake/main.testlang", buffer.BTDefault)
	sb.Settings["lsp"] = true
	sb.Settings["filetype"] = "testlang"

	// attach reserves the slot synchronously; detach drops it before
	// the async GetOrStart callback (drained below) ever runs.
	handleBufferEvent(sb, buffer.BufferEventOpen)
	handleBufferEvent(sb, buffer.BufferEventClose)

	// The server still starts and GetOrStart's callback is still
	// posted; draining it must not produce a didOpen.
	drainEvents(t, 2*time.Second)
	waitForSignal(t, initializedCh, "initialized")

	assertNoneReceived(t, rec.opened, "didOpen for a buffer closed during attach")
	assertNoneReceived(t, rec.closed, "didClose (nothing was ever opened)")

	documents.mu.Lock()
	n := len(documents.docs)
	documents.mu.Unlock()
	if n != 0 {
		t.Errorf("documents.docs has %d entries after close-during-attach, want 0", n)
	}
}

// TestRealBufferLifecycleTriggersLSPNotifications exercises the actual
// trigger points (buffer construction and Buffer.Close, wired via
// fireBufferChange in internal/buffer/buffer.go, and MarkModified for
// edits) rather than calling handleBufferEvent directly, to prove the
// hook is wired into the real buffer-open/edit/close paths and not
// just correct in isolation.
func TestRealBufferLifecycleTriggersLSPNotifications(t *testing.T) {
	resetDocuments(t)
	withTempConfigDir(t)

	rec := newSyncRecorder()
	handlers, initializedCh := lifecycleHandlers(rec.handlers())
	ch := newFakeServer(t, handlers)
	// A file with an unrecognized extension gets micro's default
	// "unknown" filetype (see buffer.go's UpdateRules): claim that
	// filetype instead of trying to match a real syntax definition.
	withTestRegistry(t, newTestRegistry("unknown", ch))

	oldLSP := config.GlobalSettings["lsp"]
	config.GlobalSettings["lsp"] = true
	t.Cleanup(func() { config.GlobalSettings["lsp"] = oldLSP })

	path := filepath.Join(t.TempDir(), "scratch.doesnotexist12345")
	b := buffer.NewBufferFromString("hello\n", path, buffer.BTDefault)
	t.Cleanup(b.Close)

	drainEvents(t, 2*time.Second)
	waitForSignal(t, initializedCh, "initialized")
	open := recv(t, rec.opened, "didOpen")
	if open.TextDocument.LanguageID != "unknown" {
		t.Errorf("didOpen languageId = %q, want %q", open.TextDocument.LanguageID, "unknown")
	}

	b.Insert(buffer.Loc{X: 0, Y: 0}, "X")
	change := recv(t, rec.changed, "didChange")
	if len(change.ContentChanges) != 1 || change.ContentChanges[0].Text != "Xhello\n" {
		t.Errorf("didChange contentChanges = %+v, want full text %q", change.ContentChanges, "Xhello\n")
	}

	b.Close()
	recv(t, rec.closed, "didClose")
}
