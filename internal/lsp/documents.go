package lsp

import (
	"context"
	"sync"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/lsp/protocol"
	"github.com/micro-editor/micro/v2/internal/screen"
)

func init() {
	buffer.RegisterBufferChangeListener(handleBufferEvent)
}

// getRegistry returns the Registry used to resolve and start language
// servers for buffer attachment, building it lazily on first use (it
// reads config.ConfigDir, which is not set yet when package inits
// run). It is a package variable rather than a plain function so
// tests can substitute a Registry wired to a fake in-process server,
// the same seam Registry.starter provides for GetOrStart.
var getRegistry = defaultGetRegistry

var (
	registryOnce   sync.Once
	sharedRegistry *Registry
)

func defaultGetRegistry() *Registry {
	registryOnce.Do(func() {
		r, err := NewRegistry()
		if err != nil {
			screen.TermMessage("lsp: loading server definitions: ", err)
			return
		}
		sharedRegistry = r
	})
	return sharedRegistry
}

// attachedDocument is the state documents tracks for one SharedBuffer
// that has an outstanding textDocument/didOpen (no matching didClose
// yet). client is nil while GetOrStart is still in flight, so events
// arriving during server startup can tell "attach reserved but not yet
// open" from "fully attached".
//
// uri is captured once at attach time and used for every subsequent
// notification, so a "Save As" that mutates the buffer's AbsPath does
// not desync the document: the server keeps tracking it under the URI
// it was opened with. Proper rename handling (didClose old + didOpen
// new) is a v2 concern.
type attachedDocument struct {
	client  *Client
	uri     protocol.DocumentURI
	version int32
}

// documentTracker records which buffers currently have an attached LSP
// document, keyed by *buffer.SharedBuffer rather than by URI. The
// SharedBuffer pointer is the stable identity of an open file across
// its whole lifetime (all split views of one file share it, and it
// survives a rename), which URI-keying does not provide. Buffers not
// in the map (lsp=false, no matching server, scratch/unnamed/non-default
// buffers, or a document whose attach failed) have their change/save/
// close events ignored by a cheap map miss, with no per-event URI
// construction on the hot path.
type documentTracker struct {
	mu   sync.Mutex
	docs map[*buffer.SharedBuffer]*attachedDocument
}

func newDocumentTracker() *documentTracker {
	return &documentTracker{docs: make(map[*buffer.SharedBuffer]*attachedDocument)}
}

var documents = newDocumentTracker()

// handleBufferEvent is the buffer.OnBufferChangeListeners callback
// internal/lsp registers in init. It is the single entry point that
// turns buffer package's generic lifecycle events into LSP document
// sync traffic. All buffer lifecycle events fire on the main
// goroutine, and GetOrStart's callback is delivered there too (via
// Events), so documents.docs is only ever touched from one goroutine;
// its mutex guards against a future off-main-goroutine caller rather
// than any concurrency that exists today.
func handleBufferEvent(b *buffer.SharedBuffer, kind buffer.BufferEventKind) {
	switch kind {
	case buffer.BufferEventOpen:
		attach(b)
	case buffer.BufferEventChange:
		notifyChange(b)
	case buffer.BufferEventSave:
		notifySave(b)
	case buffer.BufferEventClose:
		detach(b)
	}
}

// eligible reports whether b is a candidate for LSP attachment at all:
// a default (not help/log/scratch/...) buffer backed by a real file,
// with the `lsp` setting on. It does not check whether a server is
// actually configured for b's filetype; attach still needs
// Registry.DefinitionForFiletype for that.
func eligible(b *buffer.SharedBuffer) bool {
	if b.Type != buffer.BTDefault || b.AbsPath == "" {
		return false
	}
	enabled, _ := b.Settings["lsp"].(bool)
	return enabled
}

// attach starts (or reuses) the language server for b's filetype and
// sends textDocument/didOpen, unless b is ineligible or already
// attached. The didOpen content is read fresh when the server is ready
// (not captured up front), so edits made while the server was starting
// are reflected in the initial document rather than lost.
func attach(b *buffer.SharedBuffer) {
	if !eligible(b) {
		return
	}

	documents.mu.Lock()
	if _, attached := documents.docs[b]; attached {
		documents.mu.Unlock()
		return
	}
	// Reserve the slot (client still nil) so a detach arriving before
	// the async GetOrStart callback resolves can signal "this buffer
	// was closed, do not open it" by deleting the reservation.
	documents.docs[b] = &attachedDocument{}
	documents.mu.Unlock()

	filetype, _ := b.Settings["filetype"].(string)
	uri := fileURI(b.AbsPath)

	r := getRegistry()
	if r == nil {
		documents.remove(b)
		return
	}

	name, def, ok := r.DefinitionForFiletype(filetype)
	if !ok {
		documents.remove(b)
		return
	}

	root := RootFor(def, b.AbsPath)

	r.GetOrStart(context.Background(), name, root, func(c *Client, err error) {
		if err != nil {
			documents.remove(b)
			return
		}

		documents.mu.Lock()
		doc, stillTracked := documents.docs[b]
		if !stillTracked {
			// The buffer was closed while the server was starting;
			// detach already dropped the reservation, so opening the
			// document now would leak it (no didClose would follow).
			documents.mu.Unlock()
			return
		}
		doc.client = c
		doc.uri = uri
		doc.version = 1
		c.DidOpen(uri, filetype, 1, string(b.Bytes()))
		documents.mu.Unlock()
	})
}

// notifyChange sends textDocument/didChange for b if it is fully
// attached, with a fresh, monotonically increasing version and b's
// current full content. It is a no-op for buffers that never attached
// or are still starting up, resolved by a single map lookup with no
// URI construction (this runs from MarkModified on every edit).
//
// v1 sends one full-document didChange per edit, so a burst of edits
// (typing, or a bulk operation like Retab that marks every line
// modified) sends one full-text notification each. This is the
// accepted correctness-over-efficiency tradeoff for v1; v2 can
// coalesce/debounce on a timer and switch to incremental range sync.
// The actual wire write is off the edit goroutine (Notify is
// fire-and-forget), but the b.Bytes() copy here is not.
func notifyChange(b *buffer.SharedBuffer) {
	documents.mu.Lock()
	doc, ok := documents.docs[b]
	if !ok || doc.client == nil {
		documents.mu.Unlock()
		return
	}
	doc.version++
	client, version, uri := doc.client, doc.version, doc.uri
	documents.mu.Unlock()

	client.DidChange(uri, version, string(b.Bytes()))
}

// notifySave sends textDocument/didSave with b's full content if it is
// attached.
func notifySave(b *buffer.SharedBuffer) {
	documents.mu.Lock()
	doc, ok := documents.docs[b]
	var client *Client
	var uri protocol.DocumentURI
	if ok && doc.client != nil {
		client, uri = doc.client, doc.uri
	}
	documents.mu.Unlock()

	if client != nil {
		client.DidSave(uri, string(b.Bytes()))
	}
}

// detach sends textDocument/didClose for b if it is attached, and
// stops tracking it. Dropping the entry unconditionally also cancels a
// still-pending attach (see attach's stillTracked check).
func detach(b *buffer.SharedBuffer) {
	documents.mu.Lock()
	doc, ok := documents.docs[b]
	delete(documents.docs, b)
	var client *Client
	var uri protocol.DocumentURI
	if ok && doc.client != nil {
		client, uri = doc.client, doc.uri
	}
	documents.mu.Unlock()

	if client != nil {
		client.DidClose(uri)
	}
}

// remove drops b from the tracker without sending a notification, used
// to release the attach reservation when starting the language server
// or resolving a definition for it fails.
func (t *documentTracker) remove(b *buffer.SharedBuffer) {
	t.mu.Lock()
	delete(t.docs, b)
	t.mu.Unlock()
}

// documentByURI finds the attached document tracked under uri, if any,
// along with its client and current version. It is used to route an
// incoming server notification (such as textDocument/publishDiagnostics)
// back to the buffer it describes.
func documentByURI(uri protocol.DocumentURI) (b *buffer.SharedBuffer, client *Client, version int32, ok bool) {
	documents.mu.Lock()
	defer documents.mu.Unlock()
	for sb, doc := range documents.docs {
		if doc.client != nil && doc.uri == uri {
			return sb, doc.client, doc.version, true
		}
	}
	return nil, nil, 0, false
}

// ClientFor returns the Client and URI a SharedBuffer is attached
// under, if it is fully attached (an attach reservation with no client
// yet, see attach, reports ok=false). It is exported for the hover and
// goto-definition actions, which need to reach a buffer's LSP
// connection without depending on documentTracker's internal shape.
func ClientFor(b *buffer.SharedBuffer) (*Client, protocol.DocumentURI, bool) {
	documents.mu.Lock()
	defer documents.mu.Unlock()
	doc, ok := documents.docs[b]
	if !ok || doc.client == nil {
		return nil, "", false
	}
	return doc.client, doc.uri, true
}
