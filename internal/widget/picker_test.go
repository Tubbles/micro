package widget

import (
	"testing"
	"time"

	"github.com/Tubbles/tcell/v3"
	"github.com/sahilm/fuzzy"
)

// pickerHarness builds a Picker with a fixed geometry and a fake
// clock so tests are deterministic.
type pickerHarness struct {
	p           *Picker
	selectCount int
	closeCount  int
	submitCount int
	lastSelect  int
	lastSubmit  string
	clock       time.Time
}

func newPickerHarness(items []PickerItem) *pickerHarness {
	return newPickerHarnessOpts(items, false)
}

func newPickerHarnessOpts(items []PickerItem, query bool) *pickerHarness {
	h := &pickerHarness{clock: time.Unix(1_700_000_000, 0)}
	h.p = NewPicker(PickerOptions{
		Title: "test",
		Items: items,
		Query: query,
		Geometry: Geometry{
			Kind: GeomScreenRect,
			Rect: ScreenRect{X: 10, Y: 5, W: 40, H: 12},
		},
		OnSelect: func(i int) { h.selectCount++; h.lastSelect = i },
		OnSubmit: func(s string) { h.submitCount++; h.lastSubmit = s },
		OnClose:  func() { h.closeCount++ },
	})
	h.p.nowFn = func() time.Time { return h.clock }
	return h
}

func (h *pickerHarness) advance(d time.Duration) { h.clock = h.clock.Add(d) }

func mockScreenSize(t *testing.T) {
	t.Helper()
	old := screenSize
	screenSize = func() (int, int) { return 80, 24 }
	t.Cleanup(func() { screenSize = old })
}

func key(k tcell.Key) *tcell.EventKey {
	return tcell.NewEventKey(k, "", tcell.ModNone)
}

func runeKey(r rune) *tcell.EventKey {
	return tcell.NewEventKey(tcell.KeyRune, string(r), tcell.ModNone)
}

func mouse(x, y int, btn tcell.ButtonMask) *tcell.EventMouse {
	return tcell.NewEventMouse(x, y, btn, tcell.ModNone)
}

func TestPickerArrowsClampNoWrap(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "a"}, {Label: "b"}, {Label: "c"}}
	h := newPickerHarness(items)

	h.p.HandleEvent(key(tcell.KeyUp))
	if h.p.Current() != 0 {
		t.Fatalf("Up at top: want current=0, got %d", h.p.Current())
	}

	for i := 0; i < 5; i++ {
		h.p.HandleEvent(key(tcell.KeyDown))
	}
	if h.p.Current() != 2 {
		t.Fatalf("Down past end: want current=2, got %d", h.p.Current())
	}

	h.p.HandleEvent(key(tcell.KeyDown))
	if h.p.Current() != 2 {
		t.Fatalf("Down at end: want current=2, got %d", h.p.Current())
	}
}

func TestPickerHomeEnd_ClassicJumpsList(t *testing.T) {
	mockScreenSize(t)
	items := make([]PickerItem, 50)
	for i := range items {
		items[i].Label = "x"
	}
	h := newPickerHarness(items)

	h.p.HandleEvent(key(tcell.KeyEnd))
	if h.p.Current() != 49 {
		t.Fatalf("End: want current=49, got %d", h.p.Current())
	}

	h.p.HandleEvent(key(tcell.KeyHome))
	if h.p.Current() != 0 {
		t.Fatalf("Home: want current=0, got %d", h.p.Current())
	}
}

func TestPickerHomeEnd_QueryMovesCaret(t *testing.T) {
	mockScreenSize(t)
	h := newPickerHarnessOpts([]PickerItem{{Label: "alpha"}}, true)
	// Type a couple of chars so the caret has somewhere to move.
	h.p.HandleEvent(runeKey('a'))
	h.p.HandleEvent(runeKey('b'))
	h.p.HandleEvent(runeKey('c'))
	if h.p.qcur != 3 {
		t.Fatalf("after typing abc: qcur=%d, want 3", h.p.qcur)
	}
	h.p.HandleEvent(key(tcell.KeyHome))
	if h.p.qcur != 0 {
		t.Fatalf("Home with Query=true: qcur=%d, want 0", h.p.qcur)
	}
	h.p.HandleEvent(key(tcell.KeyEnd))
	if h.p.qcur != 3 {
		t.Fatalf("End with Query=true: qcur=%d, want 3", h.p.qcur)
	}
}

func TestPickerCaretLeftRight(t *testing.T) {
	mockScreenSize(t)
	h := newPickerHarnessOpts([]PickerItem{{Label: "x"}}, true)
	h.p.HandleEvent(runeKey('a'))
	h.p.HandleEvent(runeKey('b'))
	h.p.HandleEvent(runeKey('c'))
	h.p.HandleEvent(key(tcell.KeyLeft))
	if h.p.qcur != 2 {
		t.Fatalf("Left from end: qcur=%d, want 2", h.p.qcur)
	}
	h.p.HandleEvent(key(tcell.KeyLeft))
	h.p.HandleEvent(key(tcell.KeyLeft))
	h.p.HandleEvent(key(tcell.KeyLeft)) // clamps at 0
	if h.p.qcur != 0 {
		t.Fatalf("Left at start: qcur=%d, want 0", h.p.qcur)
	}
	h.p.HandleEvent(key(tcell.KeyRight))
	if h.p.qcur != 1 {
		t.Fatalf("Right from start: qcur=%d, want 1", h.p.qcur)
	}
	for i := 0; i < 10; i++ {
		h.p.HandleEvent(key(tcell.KeyRight))
	}
	if h.p.qcur != 3 {
		t.Fatalf("Right past end: qcur=%d, want 3", h.p.qcur)
	}
}

func TestPickerInsertRuneAtCaret(t *testing.T) {
	mockScreenSize(t)
	h := newPickerHarnessOpts([]PickerItem{{Label: "x"}}, true)
	h.p.HandleEvent(runeKey('a'))
	h.p.HandleEvent(runeKey('c'))
	h.p.HandleEvent(key(tcell.KeyLeft))
	h.p.HandleEvent(runeKey('b'))
	if h.p.query != "abc" {
		t.Fatalf("insert at caret: query=%q, want %q", h.p.query, "abc")
	}
	if h.p.qcur != 2 {
		t.Fatalf("after mid-insert: qcur=%d, want 2", h.p.qcur)
	}
}

func TestPickerBackspaceRemovesAtCaret(t *testing.T) {
	mockScreenSize(t)
	h := newPickerHarnessOpts([]PickerItem{{Label: "x"}}, true)
	h.p.HandleEvent(runeKey('a'))
	h.p.HandleEvent(runeKey('b'))
	h.p.HandleEvent(runeKey('c'))
	h.p.HandleEvent(key(tcell.KeyLeft))   // caret at 2 (between b and c)
	h.p.HandleEvent(key(tcell.KeyBackspace))
	if h.p.query != "ac" {
		t.Fatalf("backspace mid: query=%q, want ac", h.p.query)
	}
	if h.p.qcur != 1 {
		t.Fatalf("backspace mid: qcur=%d, want 1", h.p.qcur)
	}
	// Backspace at caret 0 is a no-op.
	h.p.HandleEvent(key(tcell.KeyHome))
	h.p.HandleEvent(key(tcell.KeyBackspace))
	if h.p.query != "ac" || h.p.qcur != 0 {
		t.Fatalf("backspace at start: query=%q qcur=%d, want ac/0",
			h.p.query, h.p.qcur)
	}
}

func TestPickerDeleteRemovesAtCaret(t *testing.T) {
	mockScreenSize(t)
	h := newPickerHarnessOpts([]PickerItem{{Label: "x"}}, true)
	h.p.HandleEvent(runeKey('a'))
	h.p.HandleEvent(runeKey('b'))
	h.p.HandleEvent(runeKey('c'))
	h.p.HandleEvent(key(tcell.KeyHome))
	h.p.HandleEvent(key(tcell.KeyDelete))
	if h.p.query != "bc" {
		t.Fatalf("delete at start: query=%q, want bc", h.p.query)
	}
	if h.p.qcur != 0 {
		t.Fatalf("delete at start: qcur=%d, want 0", h.p.qcur)
	}
	// Delete at end is a no-op.
	h.p.HandleEvent(key(tcell.KeyEnd))
	h.p.HandleEvent(key(tcell.KeyDelete))
	if h.p.query != "bc" {
		t.Fatalf("delete at end: query=%q, want bc", h.p.query)
	}
}

func TestPickerPageStep(t *testing.T) {
	mockScreenSize(t)
	items := make([]PickerItem, 50)
	for i := range items {
		items[i].Label = "x"
	}
	h := newPickerHarness(items)

	body := h.p.bodyHeight()
	h.p.HandleEvent(key(tcell.KeyPgDn))
	if h.p.Current() != body {
		t.Fatalf("PgDn: want current=%d, got %d", body, h.p.Current())
	}

	h.p.HandleEvent(key(tcell.KeyPgUp))
	if h.p.Current() != 0 {
		t.Fatalf("PgUp: want current=0, got %d", h.p.Current())
	}
}

func TestPickerEnterFiresOnSelect(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "a"}, {Label: "b"}}
	h := newPickerHarness(items)

	h.p.HandleEvent(key(tcell.KeyDown))
	h.p.HandleEvent(key(tcell.KeyEnter))

	if h.selectCount != 1 || h.lastSelect != 1 {
		t.Fatalf("OnSelect: count=%d lastSelect=%d", h.selectCount, h.lastSelect)
	}
}

func TestPickerEscFiresOnClose(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "a"}}
	h := newPickerHarness(items)
	Open(h.p)
	t.Cleanup(func() { CloseActive() })

	h.p.HandleEvent(key(tcell.KeyEsc))

	if h.closeCount != 1 {
		t.Fatalf("OnClose: count=%d", h.closeCount)
	}
	if Active() != nil {
		t.Fatalf("active widget should be cleared after Esc")
	}
}

func TestPickerRunesAreSwallowed_Classic(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "a"}, {Label: "b"}}
	h := newPickerHarness(items)

	consumed := h.p.HandleEvent(runeKey('x'))
	if !consumed {
		t.Fatalf("rune events must be consumed to keep the buffer below inert")
	}
	if h.selectCount != 0 || h.closeCount != 0 || h.p.Current() != 0 {
		t.Fatalf("rune events must be a no-op for state in classic mode")
	}
	if h.p.query != "" {
		t.Fatalf("classic picker must not accumulate query, got %q", h.p.query)
	}
}

func TestPickerRunesGoToQueryWhenEnabled(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "alpha"}, {Label: "beta"}}
	h := newPickerHarnessOpts(items, true)

	consumed := h.p.HandleEvent(runeKey('a'))
	if !consumed {
		t.Fatalf("rune events must be consumed by an active picker")
	}
	if h.p.query != "a" {
		t.Fatalf("Query=true should append rune: got %q", h.p.query)
	}
}

func TestPickerFuzzyFiltersAndOrders(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "alpha"}, {Label: "beta"}, {Label: "gamma"}}
	h := newPickerHarnessOpts(items, true)

	h.p.HandleEvent(runeKey('g'))
	h.p.HandleEvent(runeKey('a'))

	if got := len(h.p.matches); got != 1 {
		t.Fatalf("filter ga: got %d matches, want 1", got)
	}
	if h.p.matches[0].Index != 2 {
		t.Fatalf("filter ga: matched item idx=%d, want 2", h.p.matches[0].Index)
	}
	if h.p.Current() != 0 {
		t.Fatalf("filter ga: highlighted display row=%d, want 0", h.p.Current())
	}
}

func TestPickerBackspaceRestoresFullList(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "a"}, {Label: "b"}}
	h := newPickerHarnessOpts(items, true)

	h.p.HandleEvent(runeKey('z'))
	if h.p.matches == nil {
		t.Fatalf("after typing 'z', matches must not be nil")
	}
	if len(h.p.matches) != 0 {
		t.Fatalf("'z' should match nothing in [a,b], got %d", len(h.p.matches))
	}
	h.p.HandleEvent(key(tcell.KeyBackspace))
	if h.p.matches != nil {
		t.Fatalf("after backspace to empty query, matches must be nil")
	}
	if h.p.displayedLen() != 2 {
		t.Fatalf("after backspace: displayedLen=%d, want 2", h.p.displayedLen())
	}
}

func TestPickerEnterOnNoMatchFiresOnSubmit(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "alpha"}}
	h := newPickerHarnessOpts(items, true)

	h.p.HandleEvent(runeKey('z'))
	h.p.HandleEvent(runeKey('z'))
	h.p.HandleEvent(key(tcell.KeyEnter))

	if h.submitCount != 1 || h.lastSubmit != "zz" {
		t.Fatalf("OnSubmit: count=%d last=%q, want 1/zz",
			h.submitCount, h.lastSubmit)
	}
	if h.selectCount != 0 {
		t.Fatalf("OnSelect must not fire on no-match Enter, got %d",
			h.selectCount)
	}
}

func TestPickerEnterOnMatchUsesItemIndex(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "zero"}, {Label: "one"}, {Label: "two"}}
	h := newPickerHarnessOpts(items, true)

	h.p.HandleEvent(runeKey('t'))
	h.p.HandleEvent(runeKey('w'))
	h.p.HandleEvent(key(tcell.KeyEnter))

	if h.selectCount != 1 {
		t.Fatalf("OnSelect: count=%d, want 1", h.selectCount)
	}
	if h.lastSelect != 2 {
		t.Fatalf("OnSelect index: got %d, want 2 (the original Items idx)",
			h.lastSelect)
	}
	if h.submitCount != 0 {
		t.Fatalf("OnSubmit must not fire when a match exists")
	}
}

func TestPickerEnterEmptyQueryEmptyMatchesNoOp(t *testing.T) {
	mockScreenSize(t)
	h := newPickerHarnessOpts(nil, true)
	h.p.HandleEvent(key(tcell.KeyEnter))
	if h.selectCount != 0 || h.submitCount != 0 {
		t.Fatalf("Enter on empty everything: select=%d submit=%d",
			h.selectCount, h.submitCount)
	}
}

func TestPickerPasteBatchesFilterRecompute(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "alpha"}, {Label: "beta"}}
	h := newPickerHarnessOpts(items, true)

	calls := 0
	old := fuzzyFind
	fuzzyFind = func(pattern string, data []string) fuzzy.Matches {
		calls++
		return old(pattern, data)
	}
	t.Cleanup(func() { fuzzyFind = old })

	// v3 streams a paste as Start, EventKey runes, End. The picker
	// must defer the filter recompute until End, so a 3-char paste
	// runs the matcher once, not three times.
	h.p.HandleEvent(tcell.NewEventPaste(true))
	h.p.HandleEvent(runeKey('a'))
	h.p.HandleEvent(runeKey('l'))
	h.p.HandleEvent(runeKey('p'))
	h.p.HandleEvent(tcell.NewEventPaste(false))

	if calls != 1 {
		t.Fatalf("paste of 3 chars: filter ran %d times, want 1", calls)
	}
	if h.p.query != "alp" {
		t.Fatalf("paste content: query=%q, want alp", h.p.query)
	}
	if h.p.qcur != 3 {
		t.Fatalf("paste content: qcur=%d, want 3", h.p.qcur)
	}
}

func TestPickerPasteSkipsControlKeys(t *testing.T) {
	mockScreenSize(t)
	h := newPickerHarnessOpts([]PickerItem{{Label: "x"}}, true)
	// v3's input parser turns \n into KeyEnter and \t into KeyTab in
	// the middle of a paste stream. The picker drops non-rune keys
	// while pasting so an embedded newline doesn't activate.
	h.p.HandleEvent(tcell.NewEventPaste(true))
	h.p.HandleEvent(runeKey('a'))
	h.p.HandleEvent(key(tcell.KeyEnter))
	h.p.HandleEvent(runeKey('b'))
	h.p.HandleEvent(key(tcell.KeyTab))
	h.p.HandleEvent(runeKey('c'))
	h.p.HandleEvent(tcell.NewEventPaste(false))
	if h.p.query != "abc" {
		t.Fatalf("paste with controls: query=%q, want abc", h.p.query)
	}
}

func TestPickerMatchedIndexesArePreserved(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "alpha"}}
	h := newPickerHarnessOpts(items, true)
	h.p.HandleEvent(runeKey('l'))
	h.p.HandleEvent(runeKey('h'))
	if len(h.p.matches) != 1 {
		t.Fatalf("filter lh: matches=%d, want 1", len(h.p.matches))
	}
	mi := h.p.matches[0].MatchedIndexes
	// "alpha": l@1, h@3 (byte offsets). 'l' and 'h' are both ASCII so
	// rune-index == byte-index here.
	if len(mi) != 2 || mi[0] != 1 || mi[1] != 3 {
		t.Fatalf("MatchedIndexes for lh in alpha: got %v, want [1 3]", mi)
	}
}

func TestPickerMouseClickMovesHighlight_Classic(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "a"}, {Label: "b"}, {Label: "c"}, {Label: "d"}}
	h := newPickerHarness(items)
	// rect is X=10,Y=5,W=40,H=12 → bodyY0=6 (Query=false), bodyH=10
	h.p.HandleEvent(mouse(20, 8, tcell.Button1))

	if h.p.Current() != 2 { // top=0, y=8 → idx = 0 + (8-6) = 2
		t.Fatalf("click row classic: want current=2, got %d", h.p.Current())
	}
	if h.selectCount != 0 {
		t.Fatalf("single click must not activate (got %d selects)", h.selectCount)
	}
}

func TestPickerMouseClickMovesHighlight_Query(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "a"}, {Label: "b"}, {Label: "c"}, {Label: "d"}}
	h := newPickerHarnessOpts(items, true)
	// rect Y=5: top border @5, input @6, body starts @7.
	h.p.HandleEvent(mouse(20, 9, tcell.Button1))

	if h.p.Current() != 2 { // top=0, y=9 → idx = 0 + (9-7) = 2
		t.Fatalf("click row query: want current=2, got %d", h.p.Current())
	}
}

func TestPickerMouseClickIgnoresInputRow(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "a"}}
	h := newPickerHarnessOpts(items, true)
	// y=6 is the input row; clicking it must not move the highlight
	// or activate.
	h.p.HandleEvent(mouse(20, 6, tcell.Button1))
	if h.p.Current() != 0 || h.selectCount != 0 {
		t.Fatalf("input-row click should be inert")
	}
}

func TestPickerDoubleClickActivates(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "a"}, {Label: "b"}, {Label: "c"}}
	h := newPickerHarness(items)

	h.p.HandleEvent(mouse(20, 7, tcell.Button1)) // row 1
	h.advance(100 * time.Millisecond)
	h.p.HandleEvent(mouse(20, 7, tcell.Button1))

	if h.selectCount != 1 || h.lastSelect != 1 {
		t.Fatalf("double-click row 1: selects=%d last=%d", h.selectCount, h.lastSelect)
	}
}

func TestPickerSecondClickOnDifferentRowDoesNotActivate(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "a"}, {Label: "b"}, {Label: "c"}}
	h := newPickerHarness(items)

	h.p.HandleEvent(mouse(20, 7, tcell.Button1)) // row 1
	h.advance(100 * time.Millisecond)
	h.p.HandleEvent(mouse(20, 8, tcell.Button1)) // row 2

	if h.selectCount != 0 {
		t.Fatalf("clicks on different rows must not activate (got %d selects)", h.selectCount)
	}
	if h.p.Current() != 2 {
		t.Fatalf("second click moves highlight: want current=2, got %d", h.p.Current())
	}
}

func TestPickerDoubleClickWindowExpires(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "a"}, {Label: "b"}}
	h := newPickerHarness(items)

	h.p.HandleEvent(mouse(20, 7, tcell.Button1))
	h.advance(doubleClickWindow + time.Millisecond)
	h.p.HandleEvent(mouse(20, 7, tcell.Button1))

	if h.selectCount != 0 {
		t.Fatalf("clicks outside the window must not activate (got %d)", h.selectCount)
	}
}

func TestPickerClickOutsideRectCloses(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "a"}}
	h := newPickerHarness(items)
	Open(h.p)
	t.Cleanup(func() { CloseActive() })

	h.p.HandleEvent(mouse(0, 0, tcell.Button1))

	if h.closeCount != 1 {
		t.Fatalf("OnClose count=%d after click outside", h.closeCount)
	}
}

func TestPickerWheelMovesHighlight(t *testing.T) {
	mockScreenSize(t)
	items := make([]PickerItem, 20)
	for i := range items {
		items[i].Label = "x"
	}
	h := newPickerHarness(items)

	h.p.HandleEvent(mouse(20, 8, tcell.WheelDown))
	if h.p.Current() != 1 {
		t.Fatalf("WheelDown: want current=1, got %d", h.p.Current())
	}
	h.p.HandleEvent(mouse(20, 8, tcell.WheelUp))
	if h.p.Current() != 0 {
		t.Fatalf("WheelUp: want current=0, got %d", h.p.Current())
	}
}

func TestPickerSetItemsResetsQuery(t *testing.T) {
	mockScreenSize(t)
	h := newPickerHarnessOpts([]PickerItem{{Label: "a"}}, true)
	h.p.HandleEvent(runeKey('a'))
	if h.p.query != "a" {
		t.Fatalf("setup: query=%q", h.p.query)
	}
	h.p.SetItems([]PickerItem{{Label: "x"}, {Label: "y"}})
	if h.p.query != "" {
		t.Fatalf("SetItems should reset query, got %q", h.p.query)
	}
	if h.p.Current() != 0 {
		t.Fatalf("SetItems should reset current, got %d", h.p.Current())
	}
	if h.p.matches != nil {
		t.Fatalf("SetItems should reset matches, got %v", h.p.matches)
	}
}

func TestPickerEnterEmptyListNoCallback(t *testing.T) {
	mockScreenSize(t)
	h := newPickerHarness(nil)
	h.p.HandleEvent(key(tcell.KeyEnter))
	if h.selectCount != 0 {
		t.Fatalf("Enter on empty list must not fire OnSelect")
	}
}

func TestResolveScreenRectClips(t *testing.T) {
	r := Resolve(Geometry{Kind: GeomScreenRect, Rect: ScreenRect{X: 70, Y: 20, W: 20, H: 10}}, 80, 24)
	if r.W != 10 || r.H != 4 {
		t.Fatalf("clip: want W=10 H=4, got W=%d H=%d", r.W, r.H)
	}
}

func TestRegistrySingleActive(t *testing.T) {
	mockScreenSize(t)
	a := newPickerHarness(nil).p
	b := newPickerHarness(nil).p

	Open(a)
	if Active() != a {
		t.Fatal("Active should be a")
	}
	Open(b) // should close a, install b
	if Active() != b {
		t.Fatal("Active should be b")
	}
	Close(a) // wrong identity, no-op
	if Active() != b {
		t.Fatal("Close(a) when b is active should be a no-op")
	}
	Close(b)
	if Active() != nil {
		t.Fatal("Close(b) should clear active")
	}
}

func TestPickerCtrlHFiresHookWhenSet_Query(t *testing.T) {
	mockScreenSize(t)
	fired := 0
	p := NewPicker(PickerOptions{
		Title:    "test",
		Items:    []PickerItem{{Label: "a"}},
		Query:    true,
		Geometry: Geometry{Kind: GeomScreenRect, Rect: ScreenRect{X: 0, Y: 0, W: 40, H: 12}},
		OnCtrlH:  func() { fired++ },
	})
	// Pre-load a query so we can verify Ctrl-H does NOT delete from it.
	p.HandleEvent(runeKey('a'))
	p.HandleEvent(runeKey('b'))
	p.HandleEvent(key(tcell.KeyCtrlH))
	if fired != 1 {
		t.Fatalf("OnCtrlH: fired=%d, want 1", fired)
	}
	if p.query != "ab" {
		t.Fatalf("Ctrl-H with hook must not delete: query=%q, want ab", p.query)
	}
}

func TestPickerCtrlHDeletesWhenHookUnset_Query(t *testing.T) {
	mockScreenSize(t)
	// Hook left unset — preserves legacy behaviour on terminals that
	// route the Backspace key to 0x08 instead of 0x7f.
	h := newPickerHarnessOpts([]PickerItem{{Label: "x"}}, true)
	h.p.HandleEvent(runeKey('a'))
	h.p.HandleEvent(runeKey('b'))
	h.p.HandleEvent(key(tcell.KeyCtrlH))
	if h.p.query != "a" {
		t.Fatalf("Ctrl-H without hook must delete: query=%q, want a", h.p.query)
	}
}

func TestPickerBackspace2AlwaysDeletes_Query(t *testing.T) {
	mockScreenSize(t)
	// Even with a hook installed, KeyBackspace2 (0x7f) must keep
	// behaving as delete-before-caret — that is the modern-terminal
	// Backspace path and we don't want a hook to break it.
	p := NewPicker(PickerOptions{
		Items:    []PickerItem{{Label: "x"}},
		Query:    true,
		Geometry: Geometry{Kind: GeomScreenRect, Rect: ScreenRect{X: 0, Y: 0, W: 40, H: 12}},
		OnCtrlH:  func() { t.Fatalf("OnCtrlH must not fire on KeyBackspace2") },
	})
	p.HandleEvent(runeKey('a'))
	p.HandleEvent(runeKey('b'))
	p.HandleEvent(key(tcell.KeyBackspace2))
	if p.query != "a" {
		t.Fatalf("KeyBackspace2 must delete regardless of hook: query=%q, want a", p.query)
	}
}

func TestPickerCtrlHIgnoredInClassicMode(t *testing.T) {
	mockScreenSize(t)
	// Classic (Query=false) mode swallows runes and is unrelated to
	// the hook. Even if OnCtrlH is set, classic mode must not invoke
	// it, since Ctrl-H is reserved by the buffer-side binding tree.
	fired := 0
	p := NewPicker(PickerOptions{
		Items:    []PickerItem{{Label: "a"}},
		Query:    false,
		Geometry: Geometry{Kind: GeomScreenRect, Rect: ScreenRect{X: 0, Y: 0, W: 40, H: 12}},
		OnCtrlH:  func() { fired++ },
	})
	p.HandleEvent(key(tcell.KeyCtrlH))
	if fired != 0 {
		t.Fatalf("classic mode must not fire OnCtrlH, got %d", fired)
	}
}

func TestPickerRefreshItemsKeepsQuery(t *testing.T) {
	mockScreenSize(t)
	h := newPickerHarnessOpts([]PickerItem{
		{Label: "alpha"}, {Label: "beta"},
	}, true)
	h.p.HandleEvent(runeKey('a'))
	if h.p.query != "a" || len(h.p.matches) == 0 {
		t.Fatalf("setup: query=%q matches=%d", h.p.query, len(h.p.matches))
	}
	// Replace items with a set where the query still matches one row.
	h.p.RefreshItems([]PickerItem{
		{Label: ".alpha"}, {Label: "beta"}, {Label: "gamma"},
	})
	if h.p.query != "a" {
		t.Fatalf("RefreshItems must preserve query, got %q", h.p.query)
	}
	if h.p.qcur != 1 {
		t.Fatalf("RefreshItems must preserve qcur, got %d", h.p.qcur)
	}
	// Filter must have been recomputed against the new items: 'a'
	// fuzzy-matches all three rows now (".alpha" gains, "beta" still
	// has "a", "gamma" has "a").
	if len(h.p.matches) != 3 {
		t.Fatalf("RefreshItems must recompute filter: got %d matches, want 3",
			len(h.p.matches))
	}
	if h.p.Current() != 0 || h.p.top != 0 {
		t.Fatalf("RefreshItems must reset current/top to 0, got current=%d top=%d",
			h.p.Current(), h.p.top)
	}
}

func TestPickerByteOffsetForRune(t *testing.T) {
	// "aé€b" — a (1 byte), é (2), € (3), b (1) = 7 bytes total.
	s := "aé€b"
	got := []int{
		byteOffsetForRune(s, 0),
		byteOffsetForRune(s, 1),
		byteOffsetForRune(s, 2),
		byteOffsetForRune(s, 3),
		byteOffsetForRune(s, 4),
	}
	want := []int{0, 1, 3, 6, 7}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("byteOffsetForRune(%q, %d) = %d, want %d",
				s, i, got[i], want[i])
		}
	}
}
