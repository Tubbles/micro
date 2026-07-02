package buffer

import (
	"testing"
)

// swapCursorMoveListeners installs listeners for the duration of a test and
// returns a restore function, mirroring the save/restore pattern used for
// OnTextEditListeners in the action package's jump-list tests.
func swapCursorMoveListeners(listeners ...func(*SharedBuffer, Loc, Loc)) func() {
	prev := OnCursorMoveListeners
	OnCursorMoveListeners = listeners
	return func() { OnCursorMoveListeners = prev }
}

func TestGotoLoc_firesListenersWithOldAndNew(t *testing.T) {
	b := NewBufferFromString("alpha\nbeta\ngamma\ndelta\n", "", BTDefault)
	c := b.GetActiveCursor()
	// Position the cursor without firing, so the recorded "old" is known.
	c.GotoLocBare(Loc{X: 1, Y: 0})

	var fired int
	var gotBuf *SharedBuffer
	var gotOld, gotNew Loc
	restore := swapCursorMoveListeners(func(sb *SharedBuffer, old, new Loc) {
		fired++
		gotBuf = sb
		gotOld = old
		gotNew = new
	})
	defer restore()

	c.GotoLoc(Loc{X: 2, Y: 3})

	if fired != 1 {
		t.Fatalf("listener fired %d times, want 1", fired)
	}
	if gotBuf != b.SharedBuffer {
		t.Errorf("listener buffer = %p, want %p", gotBuf, b.SharedBuffer)
	}
	if gotOld.X != 1 || gotOld.Y != 0 {
		t.Errorf("old = %+v, want {X:1 Y:0}", gotOld)
	}
	if gotNew.X != 2 || gotNew.Y != 3 {
		t.Errorf("new = %+v, want {X:2 Y:3}", gotNew)
	}
}

func TestGotoLocBare_firesNoListeners(t *testing.T) {
	b := NewBufferFromString("alpha\nbeta\ngamma\ndelta\n", "", BTDefault)
	c := b.GetActiveCursor()

	var fired int
	restore := swapCursorMoveListeners(func(*SharedBuffer, Loc, Loc) { fired++ })
	defer restore()

	c.GotoLocBare(Loc{X: 2, Y: 3})

	if fired != 0 {
		t.Fatalf("GotoLocBare fired listeners %d times, want 0", fired)
	}
	if got := c.Loc; got.X != 2 || got.Y != 3 {
		t.Errorf("GotoLocBare left cursor at %+v, want {X:2 Y:3}", got)
	}
}
