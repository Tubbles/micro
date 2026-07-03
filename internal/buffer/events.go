package buffer

// BufferEventKind identifies what happened to a buffer, for the
// listeners registered via RegisterBufferChangeListener.
type BufferEventKind int

const (
	// BufferEventOpen fires once per file, when the first Buffer view of
	// it has finished construction and been added to OpenBuffers. A
	// second split onto an already-open file (a reused SharedBuffer)
	// does not fire it again, mirroring BufferEventClose.
	BufferEventOpen BufferEventKind = iota
	// BufferEventChange fires whenever a buffer's content is modified
	// (insert, remove, undo, redo, retab, ...), via MarkModified. It
	// fires once per underlying edit, not debounced.
	BufferEventChange
	// BufferEventSave fires after a buffer has been written to disk.
	BufferEventSave
	// BufferEventClose fires when the last Buffer view of a shared file
	// is closed.
	BufferEventClose
)

// bufferChangeListeners are notified of buffer lifecycle events (see
// BufferEventKind). This is a minimal, general inversion-of-control
// hook: packages that need to react to buffer lifecycle but that
// buffer must not depend on (for example internal/lsp, which already
// imports buffer and so cannot be imported back) register a listener
// here instead of buffer calling into them directly. The slice is
// unexported so RegisterBufferChangeListener is the only way to add
// one, keeping the "register during init" contract enforceable.
var bufferChangeListeners []func(*SharedBuffer, BufferEventKind)

// RegisterBufferChangeListener adds fn to the buffer lifecycle
// listeners. It is not safe to call concurrently with buffer
// construction, edits, or closing; call it during package init.
func RegisterBufferChangeListener(fn func(*SharedBuffer, BufferEventKind)) {
	bufferChangeListeners = append(bufferChangeListeners, fn)
}

func fireBufferChange(b *SharedBuffer, kind BufferEventKind) {
	for _, fn := range bufferChangeListeners {
		fn(b, kind)
	}
}
