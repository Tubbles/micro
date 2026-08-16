package action

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/display"
	ulua "github.com/micro-editor/micro/v2/internal/lua"
	"github.com/micro-editor/micro/v2/internal/views"
	lua "github.com/yuin/gopher-lua"
)

func init() {
	// Mirrors internal/buffer/buffer_test.go: NewBufferFromString needs Lua
	// state and config initialized before it can run. Tests that don't use
	// real buffers ignore this; the cost is negligible.
	ulua.L = lua.NewState()
	config.InitRuntimeFiles(false)
	config.InitGlobalSettings()
	config.GlobalSettings["backup"] = false
	config.GlobalSettings["fastdirty"] = true
}

func newTestList(max int) *JumpList {
	return &JumpList{max: max}
}

// alwaysAlive treats every pane as still-present. Used by tests that don't
// exercise the dead-pane pruning path.
func alwaysAlive(uint64) bool { return true }

// snapshot returns the (paneID, line) tuples in entry order plus pos. Tests
// compare against this rather than the raw struct to avoid coupling to the
// Buf pointer (always nil in unit tests).
func snapshot(jl *JumpList) ([][2]int, int) {
	out := make([][2]int, len(jl.entries))
	for i, e := range jl.entries {
		out[i] = [2]int{int(e.PaneID), e.Loc.Y}
	}
	return out, jl.pos
}

func at(pane uint64, line int) JumpEntry {
	return JumpEntry{PaneID: pane, Loc: buffer.Loc{X: 0, Y: line}}
}

// atXY is the (X, Y)-explicit variant for tests that care about X-level
// dedup behavior. Most tests use at() because the Y-only view suffices.
func atXY(pane uint64, x, y int) JumpEntry {
	return JumpEntry{PaneID: pane, Loc: buffer.Loc{X: x, Y: y}}
}

func TestPush_appendsAndAdvancesPos(t *testing.T) {
	jl := newTestList(10)
	jl.pushLocked(at(1, 5))
	jl.pushLocked(at(1, 50))
	jl.pushLocked(at(2, 100))

	got, pos := snapshot(jl)
	want := [][2]int{{1, 5}, {1, 50}, {2, 100}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
	if pos != 3 {
		t.Fatalf("pos = %d, want 3", pos)
	}
}

func TestPush_dedupsExactSameLoc(t *testing.T) {
	jl := newTestList(10)
	jl.pushLocked(at(1, 5))
	jl.pushLocked(at(1, 5)) // same pane, same exact loc: replace
	jl.pushLocked(at(1, 5)) // still replace
	jl.pushLocked(at(1, 6)) // different line: append

	got, pos := snapshot(jl)
	want := [][2]int{{1, 5}, {1, 6}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
	if pos != 2 {
		t.Fatalf("pos = %d, want 2", pos)
	}
}

func TestPush_distinctXOnSameLineAppends(t *testing.T) {
	// Two big-jump destinations on the same line at different columns are
	// independently useful targets for JumpBack and must not collapse.
	jl := newTestList(10)
	jl.pushLocked(atXY(1, 5, 10))
	jl.pushLocked(atXY(1, 25, 10))

	if got := len(jl.entries); got != 2 {
		t.Fatalf("entries len = %d, want 2", got)
	}
	if jl.entries[0].Loc.X != 5 || jl.entries[1].Loc.X != 25 {
		t.Fatalf("entries Xs = %d,%d, want 5,25",
			jl.entries[0].Loc.X, jl.entries[1].Loc.X)
	}
	if jl.pos != 2 {
		t.Fatalf("pos = %d, want 2", jl.pos)
	}
}

func TestPush_dedupAcrossPanesIsAppend(t *testing.T) {
	jl := newTestList(10)
	jl.pushLocked(at(1, 5))
	jl.pushLocked(at(2, 5)) // different pane on same line: append, not replace

	got, _ := snapshot(jl)
	want := [][2]int{{1, 5}, {2, 5}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
}

func TestPush_truncatesForwardBranch(t *testing.T) {
	jl := newTestList(10)
	jl.pushLocked(at(1, 1))
	jl.pushLocked(at(1, 2))
	jl.pushLocked(at(1, 3))
	// User goes Back twice; pos lands on 1 (entries[1] is line 2).
	if _, ok := jl.Back(1, nil, buffer.Loc{Y: 99}, alwaysAlive); !ok {
		t.Fatal("first Back failed")
	}
	if _, ok := jl.Back(1, nil, buffer.Loc{Y: 99}, alwaysAlive); !ok {
		t.Fatal("second Back failed")
	}
	// New push should drop everything past pos.
	jl.pushLocked(at(1, 42))

	got, pos := snapshot(jl)
	// After two Back's pos was 1; pushLocked truncates entries[:1] then dedups
	// against the previous entry (line 1 != line 42, so append).
	want := [][2]int{{1, 1}, {1, 42}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
	if pos != 2 {
		t.Fatalf("pos = %d, want 2", pos)
	}
}

func TestPush_evictsAtCap(t *testing.T) {
	jl := newTestList(3)
	for i := 1; i <= 5; i++ {
		jl.pushLocked(at(uint64(i), i*10))
	}
	got, pos := snapshot(jl)
	want := [][2]int{{3, 30}, {4, 40}, {5, 50}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
	if pos != 3 {
		t.Fatalf("pos = %d, want 3", pos)
	}
}

func TestBack_recordsCurrentBeforeFirstStep(t *testing.T) {
	jl := newTestList(10)
	jl.pushLocked(at(1, 10))
	jl.pushLocked(at(1, 20))
	// Live cursor is at pane 1, line 99.
	e, ok := jl.Back(1, nil, buffer.Loc{Y: 99}, alwaysAlive)
	if !ok || e.Loc.Y != 20 {
		t.Fatalf("Back returned (%v, %v), want (entry@line20, true)", e, ok)
	}
	got, pos := snapshot(jl)
	// Live cursor (line 99) was inserted at the tail; pos points at line 20.
	want := [][2]int{{1, 10}, {1, 20}, {1, 99}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
	if pos != 1 {
		t.Fatalf("pos = %d, want 1", pos)
	}
}

func TestBack_recordCurrentDedupsAgainstLast(t *testing.T) {
	// User invokes Back while sitting on the exact same loc as the most
	// recent entry. Don't pad the list with a redundant copy: Forward from
	// here would just return to where they already are. Back walks past
	// the deduped tip and returns the entry before it, so the user
	// actually moves.
	jl := newTestList(10)
	jl.pushLocked(at(1, 10))
	jl.pushLocked(atXY(2, 5, 20))

	// Live cursor coincides with entries[1] = (2, X=5, Y=20).
	e, ok := jl.Back(2, nil, buffer.Loc{X: 5, Y: 20}, alwaysAlive)
	if !ok {
		t.Fatal("Back failed")
	}
	if e.PaneID != 1 || e.Loc.Y != 10 {
		t.Errorf("Back returned %+v, want pane 1 line 10", e)
	}
	if got := len(jl.entries); got != 2 {
		t.Errorf("entries len = %d, want 2 (no record-current entry)", got)
	}
	if jl.pos != 0 {
		t.Errorf("pos = %d, want 0 (anchored on entries[0] after walk-back)", jl.pos)
	}
}

func TestBack_recordCurrentDistinctXAppends(t *testing.T) {
	// Live cursor is on the same line as the most recent entry but a
	// different column. Record it: a different X is a meaningful target.
	jl := newTestList(10)
	jl.pushLocked(atXY(1, 5, 20))

	e, ok := jl.Back(1, nil, buffer.Loc{X: 50, Y: 20}, alwaysAlive)
	if !ok {
		t.Fatal("Back failed")
	}
	if e.Loc.X != 5 || e.Loc.Y != 20 {
		t.Errorf("Back returned %+v, want X:5 Y:20", e.Loc)
	}
	if got := len(jl.entries); got != 2 {
		t.Errorf("entries len = %d, want 2 (live cursor recorded)", got)
	}
	if jl.entries[1].Loc.X != 50 || jl.entries[1].Loc.Y != 20 {
		t.Errorf("recorded entry = %+v, want X:50 Y:20", jl.entries[1].Loc)
	}
}

func TestPush_suppressedIsNoop(t *testing.T) {
	// Push respects the suppression flag set by withSuppression.
	jl := newTestList(10)
	jl.suppress = true
	jl.Push(1, nil, buffer.Loc{X: 0, Y: 5})
	if got := len(jl.entries); got != 0 {
		t.Errorf("entries len = %d, want 0 (push under suppression)", got)
	}
}

func TestPushAlways_ignoresSuppression(t *testing.T) {
	// PushAlways is the manual-breadcrumb path and runs even under
	// suppression; otherwise PushJump invoked from inside a plugin hook
	// during a Tabs.SetActive cascade would silently drop.
	jl := newTestList(10)
	jl.suppress = true
	jl.PushAlways(1, nil, buffer.Loc{X: 0, Y: 5})
	if got := len(jl.entries); got != 1 {
		t.Errorf("entries len = %d, want 1 (manual push under suppression)", got)
	}
}

func TestBack_returnsFalseAtStart(t *testing.T) {
	jl := newTestList(10)
	jl.pushLocked(at(1, 10))
	if _, ok := jl.Back(1, nil, buffer.Loc{Y: 99}, alwaysAlive); !ok {
		t.Fatal("first Back should succeed")
	}
	if _, ok := jl.Back(1, nil, buffer.Loc{Y: 99}, alwaysAlive); ok {
		t.Fatal("second Back should fail (at start)")
	}
}

func TestForward_returnsFalseAtEnd(t *testing.T) {
	jl := newTestList(10)
	jl.pushLocked(at(1, 10))
	// pos == len(entries), so nothing forward of us.
	if _, ok := jl.Forward(alwaysAlive); ok {
		t.Fatal("Forward at tail should fail")
	}
}

func TestRoundTrip_BackForward(t *testing.T) {
	jl := newTestList(10)
	jl.pushLocked(at(1, 10))
	jl.pushLocked(at(1, 20))
	jl.pushLocked(at(1, 30))

	// Back from cursor@99 → records 99, jumps to 30.
	e, _ := jl.Back(1, nil, buffer.Loc{Y: 99}, alwaysAlive)
	if e.Loc.Y != 30 {
		t.Fatalf("Back#1 → line %d, want 30", e.Loc.Y)
	}
	// Back again → 20.
	e, _ = jl.Back(1, nil, buffer.Loc{Y: 30}, alwaysAlive)
	if e.Loc.Y != 20 {
		t.Fatalf("Back#2 → line %d, want 20", e.Loc.Y)
	}
	// Forward → 30.
	e, _ = jl.Forward(alwaysAlive)
	if e.Loc.Y != 30 {
		t.Fatalf("Forward#1 → line %d, want 30", e.Loc.Y)
	}
	// Forward → 99 (the recorded live cursor).
	e, _ = jl.Forward(alwaysAlive)
	if e.Loc.Y != 99 {
		t.Fatalf("Forward#2 → line %d, want 99", e.Loc.Y)
	}
	// Forward at tail returns false.
	if _, ok := jl.Forward(alwaysAlive); ok {
		t.Fatal("Forward past tail should fail")
	}
}

func TestBack_skipsAndPrunesDeadEntries(t *testing.T) {
	jl := newTestList(10)
	jl.pushLocked(at(1, 10))
	jl.pushLocked(at(2, 20)) // dead pane
	jl.pushLocked(at(1, 30))

	alive := func(id uint64) bool { return id != 2 }

	// Back from live: records cursor, then tries 30 (alive).
	e, ok := jl.Back(1, nil, buffer.Loc{Y: 99}, alive)
	if !ok || e.Loc.Y != 30 {
		t.Fatalf("Back#1 = (%v, %v), want line 30", e, ok)
	}
	// Back again: should skip pane 2 and land on pane 1 line 10.
	e, ok = jl.Back(1, nil, buffer.Loc{Y: 30}, alive)
	if !ok || e.Loc.Y != 10 || e.PaneID != 1 {
		t.Fatalf("Back#2 = (%v, %v), want pane 1 line 10", e, ok)
	}
	// Verify the dead entry was pruned.
	got, _ := snapshot(jl)
	for _, p := range got {
		if p[0] == 2 {
			t.Fatalf("dead pane 2 entry not pruned: %v", got)
		}
	}
}

func TestOnTextEdit_shiftsSameLineInsert(t *testing.T) {
	// Insert "abc" at (start=2, Y=5). Entry at (X=10, Y=5) on the same line
	// shifts right by 3. ShiftLoc's same-line Insert branch doesn't read
	// LineArray, so a sentinel SharedBuffer is fine.
	b := &buffer.SharedBuffer{}
	jl := newTestList(10)
	jl.entries = []JumpEntry{
		{PaneID: 1, Buf: b, Loc: buffer.Loc{X: 10, Y: 5}},
	}

	// No newline in the inserted text → lastnl == -1, textX == 3.
	jl.OnTextEdit(b, buffer.Loc{X: 2, Y: 5}, buffer.Loc{X: 5, Y: 5},
		buffer.TextEventInsert, -1, 3)

	if got := jl.entries[0].Loc; got.X != 13 || got.Y != 5 {
		t.Errorf("entry on same line as Insert: got %+v, want {X:13 Y:5}", got)
	}
}

func TestOnTextEdit_shiftsRemoveAcrossLines(t *testing.T) {
	// Remove a 3-line block (Y=2..Y=5). Entries above the block are
	// unchanged; entries below shift up by (end.Y - start.Y) = 3. Different
	// line from end.Y, so ShiftLoc's "loc.Y != end.Y" branch fires and no
	// LineArray dereference happens.
	b := &buffer.SharedBuffer{}
	jl := newTestList(10)
	jl.entries = []JumpEntry{
		{PaneID: 1, Buf: b, Loc: buffer.Loc{X: 4, Y: 1}}, // above block
		{PaneID: 2, Buf: b, Loc: buffer.Loc{X: 0, Y: 8}}, // below block
	}

	jl.OnTextEdit(b, buffer.Loc{X: 0, Y: 2}, buffer.Loc{X: 0, Y: 5},
		buffer.TextEventRemove, 0, 0)

	if got := jl.entries[0].Loc; got.X != 4 || got.Y != 1 {
		t.Errorf("entry above Remove: got %+v, want {X:4 Y:1}", got)
	}
	if got := jl.entries[1].Loc; got.X != 0 || got.Y != 5 {
		t.Errorf("entry below Remove: got %+v, want {X:0 Y:5}", got)
	}
}

func TestOnTextEdit_endToEndThroughDoTextEvent(t *testing.T) {
	// Real Buffer + EventHandler exercises the registration, hook
	// invocation, and ShiftLoc with a live LineArray. Catches breakage if
	// the hook gets disconnected from DoTextEvent or its arguments drift.
	b := buffer.NewBufferFromString("alpha\nbeta\ngamma\ndelta\n", "", buffer.BTDefault)

	jl := newTestList(10)
	jl.entries = []JumpEntry{
		{PaneID: 1, Buf: b.SharedBuffer, Loc: buffer.Loc{X: 0, Y: 2}}, // "gamma"
	}

	// Register only this test's listener for the duration of the test, so
	// we don't depend on the global Jumps and don't pollute it.
	prev := buffer.OnTextEditListeners
	buffer.OnTextEditListeners = []func(*buffer.SharedBuffer, buffer.Loc, buffer.Loc, int, int, int){jl.OnTextEdit}
	defer func() { buffer.OnTextEditListeners = prev }()

	// Insert two lines at the start of the file. The entry pointing at
	// "gamma" was at Y=2; afterwards "gamma" lives on Y=4.
	b.EventHandler.Insert(buffer.Loc{X: 0, Y: 0}, "one\ntwo\n")

	if got := jl.entries[0].Loc.Y; got != 4 {
		t.Errorf("after 2-line Insert at top: entry.Y = %d, want 4 (lines = %v)", got, strings.Split(string(b.Bytes()), "\n"))
	}

	// Remove one line at the top. Entry shifts back up by 1.
	b.EventHandler.Remove(buffer.Loc{X: 0, Y: 0}, buffer.Loc{X: 0, Y: 1})

	if got := jl.entries[0].Loc.Y; got != 3 {
		t.Errorf("after 1-line Remove at top: entry.Y = %d, want 3 (lines = %v)", got, strings.Split(string(b.Bytes()), "\n"))
	}
}

func TestOnTextEdit_shiftsMatchingBufferEntries(t *testing.T) {
	// Hand-build minimal SharedBuffer sentinels: OnTextEdit only compares
	// pointer identity to route the event, and the Insert path of ShiftLoc
	// for entries above the edit (start.Y != loc.Y) doesn't dereference
	// LineArray. Going through NewBufferFromString would initialize Lua
	// plugin state and crash this unit test.
	b1 := &buffer.SharedBuffer{}
	b2 := &buffer.SharedBuffer{}

	jl := newTestList(10)
	jl.entries = []JumpEntry{
		{PaneID: 1, Buf: b1, Loc: buffer.Loc{X: 0, Y: 3}},
		{PaneID: 2, Buf: b2, Loc: buffer.Loc{X: 0, Y: 0}},
		{PaneID: 3, Buf: b1, Loc: buffer.Loc{X: 0, Y: 7}},
	}
	jl.pos = len(jl.entries)

	// Simulate inserting two lines at the very top of b1.
	jl.OnTextEdit(b1, buffer.Loc{X: 0, Y: 0}, buffer.Loc{X: 0, Y: 2},
		buffer.TextEventInsert, 1 /* lastnl present */, 0 /* textX */)

	if got := jl.entries[0].Loc.Y; got != 5 {
		t.Errorf("b1 entry #0: line %d, want 5 after inserting 2 lines above", got)
	}
	if got := jl.entries[1].Loc.Y; got != 0 {
		t.Errorf("b2 entry shifted unexpectedly: line %d, want 0", got)
	}
	if got := jl.entries[2].Loc.Y; got != 9 {
		t.Errorf("b1 entry #2: line %d, want 9 after inserting 2 lines above", got)
	}
}

// makeTestPane constructs a Tab containing one BufPane backed by buf, wired
// up to the global Tabs (saving and returning a restore function). It does
// the bare minimum to make findPaneByID / Tabs.SetActive / pane.SetActive
// runnable in tests, sidestepping the screen-dependent NewTabList path.
func makeTestPane(buf *buffer.Buffer) (*BufPane, func()) {
	tab := &Tab{
		Node:     views.NewRoot(0, 0, 80, 24),
		UIWindow: display.NewUIWindow(views.NewRoot(0, 0, 80, 24)),
	}
	tab.release = true
	bp := NewBufPaneFromBuf(buf, tab)
	bp.SetID(tab.ID())
	tab.Panes = append(tab.Panes, bp)

	prev := Tabs
	Tabs = &TabList{
		TabWindow: display.NewTabWindow(80, 0),
		List:      []*Tab{tab},
	}
	return bp, func() { Tabs = prev }
}

func TestApplyJump_clampsStaleLocAgainstBuffer(t *testing.T) {
	// Buffer has 4 content lines plus a trailing empty line from the final
	// newline (Y in [0..4]). A stale jump entry at Y=99 must land within
	// the buffer's bounds rather than past EOF.
	b := buffer.NewBufferFromString("alpha\nbeta\ngamma\ndelta\n", "", buffer.BTDefault)
	bp, restore := makeTestPane(b)
	defer restore()

	end := b.End()
	ok := applyJump(JumpEntry{
		PaneID: bp.ID(),
		Buf:    b.SharedBuffer,
		Loc:    buffer.Loc{X: 100, Y: 99},
	})
	if !ok {
		t.Fatal("applyJump returned false; expected success")
	}
	if got := bp.Cursor.Loc; got.Y > end.Y || got.Y < 0 {
		t.Errorf("cursor landed at %+v, expected within Y [0,%d]", got, end.Y)
	}
	if got := bp.Cursor.Loc.X; got < 0 {
		t.Errorf("cursor X = %d, expected >= 0", got)
	}
}

func TestApplyJump_clearsExtraCursorsOnDestination(t *testing.T) {
	// Multi-cursor on the destination pane: applyJump should leave a
	// single cursor at the jump target so the user lands clean.
	b := buffer.NewBufferFromString("alpha\nbeta\ngamma\ndelta\n", "", buffer.BTDefault)
	b.AddCursor(buffer.NewCursor(b, buffer.Loc{X: 0, Y: 1}))
	b.AddCursor(buffer.NewCursor(b, buffer.Loc{X: 0, Y: 2}))
	if got := b.NumCursors(); got != 3 {
		t.Fatalf("setup: NumCursors = %d, want 3", got)
	}

	bp, restore := makeTestPane(b)
	defer restore()

	if !applyJump(JumpEntry{
		PaneID: bp.ID(),
		Buf:    b.SharedBuffer,
		Loc:    buffer.Loc{X: 0, Y: 2},
	}) {
		t.Fatal("applyJump returned false")
	}
	if got := b.NumCursors(); got != 1 {
		t.Errorf("after jump: NumCursors = %d, want 1", got)
	}
	if got := bp.Cursor.Loc; got.X != 0 || got.Y != 2 {
		t.Errorf("after jump: cursor at %+v, want X:0 Y:2", got)
	}
}

// makeExtraTestTab appends a second Tab containing one BufPane backed by
// buf to the global (test) Tabs list, mirroring makeTestPane's bare
// wiring. The caller must have installed a test Tabs via makeTestPane
// first; makeTestPane's restore function cleans this tab up too.
func makeExtraTestTab(buf *buffer.Buffer) *BufPane {
	tab := &Tab{
		Node:     views.NewRoot(0, 0, 80, 24),
		UIWindow: display.NewUIWindow(views.NewRoot(0, 0, 80, 24)),
	}
	tab.release = true
	bp := NewBufPaneFromBuf(buf, tab)
	bp.SetID(tab.ID())
	tab.Panes = append(tab.Panes, bp)
	Tabs.List = append(Tabs.List, tab)
	return bp
}

// writeTestFile creates a file for jump-restore tests and returns its path.
func writeTestFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "target.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRebindBuffer_rewritesMatchingEntries(t *testing.T) {
	jl := newTestList(10)
	oldBuf := &buffer.SharedBuffer{}
	otherBuf := &buffer.SharedBuffer{}
	newBuf := &buffer.SharedBuffer{}
	jl.pushLocked(JumpEntry{PaneID: 1, Buf: oldBuf, Loc: buffer.Loc{Y: 1}})
	jl.pushLocked(JumpEntry{PaneID: 2, Buf: otherBuf, Loc: buffer.Loc{Y: 2}})
	jl.pushLocked(JumpEntry{PaneID: 3, Buf: oldBuf, Loc: buffer.Loc{Y: 3}})

	jl.RebindBuffer(oldBuf, newBuf)

	if jl.entries[0].Buf != newBuf {
		t.Errorf("entry 0 not rebound")
	}
	if jl.entries[1].Buf != otherBuf {
		t.Errorf("entry 1 (different buffer) was rebound; want untouched")
	}
	if jl.entries[2].Buf != newBuf {
		t.Errorf("entry 2 not rebound")
	}
}

func TestApplyJump_reopensSwappedOutBufferInPlace(t *testing.T) {
	// A jump entry whose pane had its buffer replaced (e.g. by the
	// `open` command) must land in the recorded file again, not in the
	// buffer now occupying the pane.
	path := writeTestFile(t, "alpha\nbeta\ngamma\ndelta\n")
	b, err := buffer.NewBufferFromFile(path, buffer.BTDefault)
	if err != nil {
		t.Fatal(err)
	}
	recordedPath := b.AbsPath
	bp, restore := makeTestPane(b)
	defer restore()
	entry := JumpEntry{PaneID: bp.ID(), Buf: b.SharedBuffer, Loc: buffer.Loc{X: 0, Y: 2}}

	bp.OpenBuffer(buffer.NewBufferFromString("intruder\n", "", buffer.BTDefault))

	if !applyJump(entry) {
		t.Fatal("applyJump returned false; expected the recorded file to be restored")
	}
	if got := bp.Buf.AbsPath; got != recordedPath {
		t.Errorf("pane shows %q, want the recorded file %q", got, recordedPath)
	}
	if got := string(bp.Buf.Line(2)); got != "gamma" {
		t.Errorf("restored buffer line 2 = %q, want %q", got, "gamma")
	}
	if got := bp.Cursor.Loc; got.X != 0 || got.Y != 2 {
		t.Errorf("cursor at %+v, want X:0 Y:2", got)
	}
}

func TestApplyJump_prefersPaneAlreadyShowingRecordedFile(t *testing.T) {
	// When the recorded file is displayed in some other pane, the jump
	// should land there instead of touching the recorded pane's current
	// buffer.
	path := writeTestFile(t, "alpha\nbeta\ngamma\ndelta\n")
	b, err := buffer.NewBufferFromFile(path, buffer.BTDefault)
	if err != nil {
		t.Fatal(err)
	}
	bp, restore := makeTestPane(b)
	defer restore()
	entry := JumpEntry{PaneID: bp.ID(), Buf: b.SharedBuffer, Loc: buffer.Loc{X: 0, Y: 1}}

	b2, err := buffer.NewBufferFromFile(path, buffer.BTDefault)
	if err != nil {
		t.Fatal(err)
	}
	bp2 := makeExtraTestTab(b2)
	bp.OpenBuffer(buffer.NewBufferFromString("intruder\n", "", buffer.BTDefault))

	if !applyJump(entry) {
		t.Fatal("applyJump returned false; expected jump to the displaying pane")
	}
	if got := Tabs.Active(); got != 1 {
		t.Errorf("active tab = %d, want 1 (the tab displaying the file)", got)
	}
	if got := bp2.Cursor.Loc.Y; got != 1 {
		t.Errorf("displaying pane cursor Y = %d, want 1", got)
	}
	if got := bp.Buf.AbsPath; got != "" {
		t.Errorf("recorded pane's buffer was swapped to %q; want untouched", got)
	}
}

// setupFocusedPane wires makeTestPane into a resized, focused pane and installs
// a fresh global Jumps list, returning the pane, the list, and a restore func.
// Resize is required because the bare harness yields BufView height 0 (the
// zero-layout trap): the >= half-screen threshold divides by that height, so
// the tests assert it is positive here. OnCursorMove reads the receiver's
// suppress flag but pushes to the global Jumps, so both must be the same
// object for the assertions to observe the push.
func setupFocusedPane(t *testing.T, buf *buffer.Buffer) (*BufPane, *JumpList, func()) {
	t.Helper()
	bp, restoreTabs := makeTestPane(buf)
	bp.Resize(80, 24)
	if bp.BufView().Height <= 0 {
		t.Fatalf("harness BufView height = %d, want > 0", bp.BufView().Height)
	}
	jl := newTestList(10)
	prevJumps := Jumps
	Jumps = jl
	return bp, jl, func() {
		Jumps = prevJumps
		restoreTabs()
	}
}

func TestOnCursorMove_farMoveRecordsOrigin(t *testing.T) {
	b := buffer.NewBufferFromString(strings.Repeat("x\n", 100), "", buffer.BTDefault)
	bp, jl, restore := setupFocusedPane(t, b)
	defer restore()

	origin := buffer.Loc{X: 0, Y: 2}
	Jumps.OnCursorMove(b.SharedBuffer, origin, buffer.Loc{X: 0, Y: 80})

	got, _ := snapshot(jl)
	want := [][2]int{{int(bp.ID()), 2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
}

func TestOnCursorMove_subThresholdDoesNotRecord(t *testing.T) {
	b := buffer.NewBufferFromString(strings.Repeat("x\n", 100), "", buffer.BTDefault)
	_, jl, restore := setupFocusedPane(t, b)
	defer restore()

	// BufView height is 23, so height/2 == 11; a 6-line move is below it.
	Jumps.OnCursorMove(b.SharedBuffer, buffer.Loc{X: 0, Y: 2}, buffer.Loc{X: 0, Y: 8})

	if got := len(jl.entries); got != 0 {
		t.Fatalf("entries len = %d, want 0 (sub-threshold move)", got)
	}
}

func TestOnCursorMove_nonFocusedBufferIgnored(t *testing.T) {
	b := buffer.NewBufferFromString(strings.Repeat("x\n", 100), "", buffer.BTDefault)
	_, jl, restore := setupFocusedPane(t, b)
	defer restore()

	// A far move reported against a buffer that is not the focused pane's is
	// filtered out (background-split / info-bar cursors).
	other := buffer.NewBufferFromString(strings.Repeat("y\n", 100), "", buffer.BTDefault)
	Jumps.OnCursorMove(other.SharedBuffer, buffer.Loc{X: 0, Y: 2}, buffer.Loc{X: 0, Y: 80})

	if got := len(jl.entries); got != 0 {
		t.Fatalf("entries len = %d, want 0 (move on non-focused buffer)", got)
	}
}

func TestOnCursorMove_suppressedDoesNothing(t *testing.T) {
	b := buffer.NewBufferFromString(strings.Repeat("x\n", 100), "", buffer.BTDefault)
	_, jl, restore := setupFocusedPane(t, b)
	defer restore()

	// Covers applyJump's own moves, which run inside withSuppression.
	jl.suppress = true
	Jumps.OnCursorMove(b.SharedBuffer, buffer.Loc{X: 0, Y: 2}, buffer.Loc{X: 0, Y: 80})

	if got := len(jl.entries); got != 0 {
		t.Fatalf("entries len = %d, want 0 (suppressed)", got)
	}
}

func TestOnCursorMove_dedupsAgainstExecActionPush(t *testing.T) {
	// A whitelisted mover pushes the origin via execAction, then the same move
	// fires the hook carrying that same origin. pushLocked's adjacency dedup
	// must collapse the two into a single entry so JumpBack has one stop.
	b := buffer.NewBufferFromString(strings.Repeat("x\n", 100), "", buffer.BTDefault)
	bp, jl, restore := setupFocusedPane(t, b)
	defer restore()

	origin := buffer.Loc{X: 0, Y: 2}
	Jumps.Push(bp.ID(), b.SharedBuffer, origin)
	Jumps.OnCursorMove(b.SharedBuffer, origin, buffer.Loc{X: 0, Y: 80})

	if got := len(jl.entries); got != 1 {
		t.Fatalf("entries len = %d, want 1 (execAction push + hook dedup)", got)
	}
}

func TestOnCursorMove_endToEndThroughGotoLoc(t *testing.T) {
	// Exercise the full path: a far Cursor.GotoLoc fires the registered hook,
	// which resolves the focused pane and records the origin. Mirrors the
	// OnTextEdit end-to-end test's listener-swap pattern.
	b := buffer.NewBufferFromString(strings.Repeat("x\n", 100), "", buffer.BTDefault)
	bp, jl, restore := setupFocusedPane(t, b)
	defer restore()

	prev := buffer.OnCursorMoveListeners
	buffer.OnCursorMoveListeners = []func(*buffer.SharedBuffer, buffer.Loc, buffer.Loc){jl.OnCursorMove}
	defer func() { buffer.OnCursorMoveListeners = prev }()

	// Cursor starts at {0,0}; a far GotoLoc must record that origin.
	bp.Cursor.GotoLoc(buffer.Loc{X: 0, Y: 80})

	got, _ := snapshot(jl)
	want := [][2]int{{int(bp.ID()), 0}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
}
