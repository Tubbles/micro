package action

import (
	"sync"

	"github.com/micro-editor/micro/v2/internal/buffer"
)

// DefaultJumpListMax is the cap on how many jump entries the list retains. When
// the list grows past this, the oldest entry is evicted on each new push.
// Matches vim's default `'jumpoptions'`-shaped behavior: 100 entries.
const DefaultJumpListMax = 100

// A JumpEntry records a cursor position in some BufPane. It captures the
// pane ID (monotonically assigned in views.NewID, never recycled), the shared
// buffer the loc belongs to (used to match incoming text events for shifting),
// and the loc itself.
type JumpEntry struct {
	PaneID uint64
	Buf    *buffer.SharedBuffer
	Loc    buffer.Loc
}

// JumpList implements the back/forward navigation history. Entries are stored
// chronologically. Pos points at the conceptual "current" slot: if pos ==
// len(entries) the user is past the most recent push (e.g. just performed a
// new jump), and Back will first record their current location so Forward can
// return to it.
type JumpList struct {
	mu       sync.Mutex
	entries  []JumpEntry
	pos      int
	max      int
	suppress bool
}

// Jumps is the global jump list. Lives for the duration of the editor session;
// not persisted across restarts (per-buffer savecursor is the existing
// persistence story for the most recent position).
var Jumps = &JumpList{max: DefaultJumpListMax}

// bigJumpActions is the curated whitelist of action names that cause an
// implicit push of the cursor's prior location. Keep this conservative:
// adding too many actions makes JumpBack noisy; missing one makes a class of
// motion un-recoverable. MousePress is included because clicking is morally a
// jump even when the destination is on screen.
var bigJumpActions = map[string]bool{
	"CursorStart":         true,
	"CursorEnd":           true,
	"JumpLine":            true,
	"JumpToMatchingBrace": true,
	"Find":                true,
	"FindLiteral":         true,
	"FindNext":            true,
	"FindPrevious":        true,
	"CursorPageUp":        true,
	"CursorPageDown":      true,
	"HalfPageUp":          true,
	"HalfPageDown":        true,
	"MousePress":          true,
}

func init() {
	buffer.RegisterOnTextEditListener(Jumps.OnTextEdit)
}

// Push records a cursor position from an automatic source (the
// execAction whitelist, Tab.SetActive, TabList.SetActive). It is a no-op
// when suppression is active, so callers don't need to gate themselves
// against in-flight Back/Forward navigation.
func (jl *JumpList) Push(paneID uint64, buf *buffer.SharedBuffer, loc buffer.Loc) {
	jl.mu.Lock()
	defer jl.mu.Unlock()
	if jl.suppress {
		return
	}
	jl.pushLocked(JumpEntry{PaneID: paneID, Buf: buf, Loc: loc})
}

// PushAlways records a cursor position unconditionally, ignoring
// suppression. Reserved for the manual-breadcrumb path (PushJump):
// when the user explicitly drops a marker they always want it kept,
// even if the editor happens to be inside a withSuppression scope.
func (jl *JumpList) PushAlways(paneID uint64, buf *buffer.SharedBuffer, loc buffer.Loc) {
	jl.mu.Lock()
	defer jl.mu.Unlock()
	jl.pushLocked(JumpEntry{PaneID: paneID, Buf: buf, Loc: loc})
}

// pushLocked appends e to entries, truncating any forward branch first
// (a new branch invalidates the old forward history), deduplicating
// against the immediately preceding entry on the exact same pane and
// loc, and evicting the oldest entry once the list exceeds max. Dedup
// is by full loc (X and Y) so two distinct positions on the same line
// stay distinct, since both were reached via a "big jump" action and
// are independently useful targets for JumpBack.
func (jl *JumpList) pushLocked(e JumpEntry) {
	jl.entries = jl.entries[:jl.pos]
	if n := len(jl.entries); n > 0 {
		last := jl.entries[n-1]
		if last.PaneID == e.PaneID && last.Loc == e.Loc {
			jl.entries[n-1] = e
			jl.pos = n
			return
		}
	}
	jl.entries = append(jl.entries, e)
	if len(jl.entries) > jl.max {
		drop := len(jl.entries) - jl.max
		jl.entries = append(jl.entries[:0], jl.entries[drop:]...)
	}
	jl.pos = len(jl.entries)
}

// Back returns the entry to navigate to when going backward, or false if the
// list is exhausted. If pos is past the most recent entry, the current
// location is recorded first so Forward can return to it. The alive callback
// lets the action layer skip entries whose pane has been closed.
func (jl *JumpList) Back(curPaneID uint64, curBuf *buffer.SharedBuffer, curLoc buffer.Loc, alive func(uint64) bool) (JumpEntry, bool) {
	jl.mu.Lock()
	defer jl.mu.Unlock()

	if jl.pos == len(jl.entries) {
		// First step back from the live cursor: record where we are so
		// Forward can return here, but dedup against the immediately
		// preceding entry on the same (PaneID, Loc) so we don't pad the
		// list when the user is already sitting on the most recent jump
		// target.
		e := JumpEntry{PaneID: curPaneID, Buf: curBuf, Loc: curLoc}
		n := len(jl.entries)
		if n == 0 || jl.entries[n-1].PaneID != e.PaneID || jl.entries[n-1].Loc != e.Loc {
			jl.entries = append(jl.entries, e)
			if len(jl.entries) > jl.max {
				drop := len(jl.entries) - jl.max
				jl.entries = append(jl.entries[:0], jl.entries[drop:]...)
			}
		}
		jl.pos = len(jl.entries) - 1
	}

	for jl.pos > 0 {
		jl.pos--
		e := jl.entries[jl.pos]
		if alive(e.PaneID) {
			return e, true
		}
		// Drop the dead entry and continue searching backward. Splice it out;
		// pos already points at the slot we want to remove next.
		jl.entries = append(jl.entries[:jl.pos], jl.entries[jl.pos+1:]...)
	}
	return JumpEntry{}, false
}

// Forward returns the next entry past pos, or false if none. Like Back, dead
// entries are skipped and pruned via the alive callback.
func (jl *JumpList) Forward(alive func(uint64) bool) (JumpEntry, bool) {
	jl.mu.Lock()
	defer jl.mu.Unlock()

	for jl.pos+1 < len(jl.entries) {
		jl.pos++
		e := jl.entries[jl.pos]
		if alive(e.PaneID) {
			return e, true
		}
		jl.entries = append(jl.entries[:jl.pos], jl.entries[jl.pos+1:]...)
		jl.pos--
	}
	return JumpEntry{}, false
}

// OnTextEdit is registered with buffer.RegisterOnTextEditListener at init
// time. For any entry whose buffer matches the edited one, shift its loc
// using the same primitive that the live cursors use, so a jump entry that
// was at line 500 becomes line 550 when 50 newlines are inserted above it.
func (jl *JumpList) OnTextEdit(b *buffer.SharedBuffer, start, end buffer.Loc, eventType, lastnl, textX int) {
	jl.mu.Lock()
	defer jl.mu.Unlock()
	for i := range jl.entries {
		if jl.entries[i].Buf == b {
			jl.entries[i].Loc = buffer.ShiftLoc(jl.entries[i].Loc, start, end, eventType, lastnl, textX, b.LineArray)
		}
	}
}

// withSuppression runs fn with automatic pushes disabled. Used by JumpBack /
// JumpForward to prevent the focus changes and cursor moves they perform
// from feeding back into the list.
func (jl *JumpList) withSuppression(fn func()) {
	jl.mu.Lock()
	jl.suppress = true
	jl.mu.Unlock()
	defer func() {
		jl.mu.Lock()
		jl.suppress = false
		jl.mu.Unlock()
	}()
	fn()
}

// findPaneByID walks the global tab list looking for a BufPane with the given
// ID. Returns the containing tab index, the pane index within that tab, and
// the pane itself, or (-1, -1, nil) if not found. Pane IDs are monotonic and
// never recycled (views.NewID), so a miss reliably means the pane was closed.
func findPaneByID(id uint64) (int, int, *BufPane) {
	if Tabs == nil {
		return -1, -1, nil
	}
	for ti, t := range Tabs.List {
		for pi, p := range t.Panes {
			if bp, ok := p.(*BufPane); ok && bp.ID() == id {
				return ti, pi, bp
			}
		}
	}
	return -1, -1, nil
}

// paneAlive is the alive callback passed to Back/Forward in production.
func paneAlive(id uint64) bool {
	_, _, bp := findPaneByID(id)
	return bp != nil
}

// applyJump moves focus to the pane referenced by e and seeks its cursor.
// Returns false if the pane has gone away in the meantime (caller should
// retry with the next entry, but in practice Back/Forward have already
// validated via the alive callback).
//
// Multi-cursor selections on the destination pane are cleared so the user
// always lands with a single cursor at the jump target. The recorded Loc
// is also clamped against the destination buffer in case it has gone stale
// (e.g. the buffer was edited via a path that bypasses OnTextEditListeners
// such as MultipleReplace, leaving the entry pointing past EOF).
func applyJump(e JumpEntry) bool {
	ti, pi, bp := findPaneByID(e.PaneID)
	if bp == nil {
		return false
	}
	if Tabs.Active() != ti {
		Tabs.SetActive(ti)
	}
	if Tabs.List[ti].active != pi {
		Tabs.List[ti].SetActive(pi)
	}
	if bp.Buf.NumCursors() > 1 {
		bp.Buf.ClearCursors()
		bp.Cursor = bp.Buf.GetActiveCursor()
	}
	loc := e.Loc.Clamp(bp.Buf.Start(), bp.Buf.End())
	bp.Cursor.GotoLoc(loc)
	bp.Relocate()
	return true
}
