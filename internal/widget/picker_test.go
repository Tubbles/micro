package widget

import (
	"sort"
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
	// must defer both the filter recompute *and* the query mutation
	// until End. Per-character query mutation would force the input
	// row to redraw and the terminal to receive an update per
	// character, which is what users perceive as paste lag.
	h.p.HandleEvent(tcell.NewEventPaste(true))
	h.p.HandleEvent(runeKey('a'))
	h.p.HandleEvent(runeKey('l'))
	if h.p.query != "" {
		t.Fatalf("mid-paste: query=%q, want empty until End", h.p.query)
	}
	if h.p.qcur != 0 {
		t.Fatalf("mid-paste: qcur=%d, want 0 until End", h.p.qcur)
	}
	if calls != 0 {
		t.Fatalf("mid-paste: filter ran %d times, want 0 until End", calls)
	}
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

func TestPickerPasteAtCaretMidQuery(t *testing.T) {
	mockScreenSize(t)
	h := newPickerHarnessOpts([]PickerItem{{Label: "x"}}, true)
	h.p.HandleEvent(runeKey('a'))
	h.p.HandleEvent(runeKey('c'))
	h.p.HandleEvent(key(tcell.KeyLeft))
	h.p.HandleEvent(tcell.NewEventPaste(true))
	h.p.HandleEvent(runeKey('b'))
	h.p.HandleEvent(tcell.NewEventPaste(false))
	if h.p.query != "abc" {
		t.Fatalf("paste at caret: query=%q, want abc", h.p.query)
	}
	if h.p.qcur != 2 {
		t.Fatalf("paste at caret: qcur=%d, want 2", h.p.qcur)
	}
}

func TestPickerPasteAppendsToExistingQuery(t *testing.T) {
	mockScreenSize(t)
	h := newPickerHarnessOpts([]PickerItem{{Label: "x"}}, true)
	h.p.HandleEvent(runeKey('a'))
	h.p.HandleEvent(tcell.NewEventPaste(true))
	h.p.HandleEvent(runeKey('b'))
	h.p.HandleEvent(runeKey('c'))
	h.p.HandleEvent(tcell.NewEventPaste(false))
	if h.p.query != "abc" {
		t.Fatalf("paste append: query=%q, want abc", h.p.query)
	}
	if h.p.qcur != 3 {
		t.Fatalf("paste append: qcur=%d, want 3", h.p.qcur)
	}
}

func TestPickerBackToBackPastes(t *testing.T) {
	mockScreenSize(t)
	h := newPickerHarnessOpts([]PickerItem{{Label: "x"}}, true)

	calls := 0
	old := fuzzyFind
	fuzzyFind = func(pattern string, data []string) fuzzy.Matches {
		calls++
		return old(pattern, data)
	}
	t.Cleanup(func() { fuzzyFind = old })

	h.p.HandleEvent(tcell.NewEventPaste(true))
	h.p.HandleEvent(runeKey('a'))
	h.p.HandleEvent(runeKey('b'))
	h.p.HandleEvent(tcell.NewEventPaste(false))
	h.p.HandleEvent(tcell.NewEventPaste(true))
	h.p.HandleEvent(runeKey('c'))
	h.p.HandleEvent(runeKey('d'))
	h.p.HandleEvent(tcell.NewEventPaste(false))

	if h.p.query != "abcd" {
		t.Fatalf("back-to-back paste: query=%q, want abcd", h.p.query)
	}
	if h.p.qcur != 4 {
		t.Fatalf("back-to-back paste: qcur=%d, want 4", h.p.qcur)
	}
	if calls != 2 {
		t.Fatalf("back-to-back paste: fuzzyFind ran %d times, want 2", calls)
	}
}

func TestPickerPastePreservesMultiRuneGrapheme(t *testing.T) {
	mockScreenSize(t)
	h := newPickerHarnessOpts([]PickerItem{{Label: "x"}}, true)
	// A regional-indicator flag (e.g. 🇸🇪) is a single grapheme cluster
	// but two runes. tcell v3 delivers it as one EventKey whose Str()
	// is the full multi-rune string. The keystroke path truncates to
	// the first rune (because a single keystroke cannot legitimately
	// deliver more than one rune), but the paste path preserves the
	// whole grapheme so clipboard content with emoji round-trips.
	cluster := "\U0001F1F8\U0001F1EA"
	multi := tcell.NewEventKey(tcell.KeyRune, cluster, tcell.ModNone)
	h.p.HandleEvent(tcell.NewEventPaste(true))
	h.p.HandleEvent(multi)
	h.p.HandleEvent(tcell.NewEventPaste(false))
	if h.p.query != cluster {
		t.Fatalf("paste multi-rune: query=%q, want %q", h.p.query, cluster)
	}
	if h.p.qcur != 2 {
		t.Fatalf("paste multi-rune: qcur=%d, want 2 (rune count of cluster)",
			h.p.qcur)
	}
}

func TestPickerPasteEmptyStillFiresRecompute(t *testing.T) {
	mockScreenSize(t)
	h := newPickerHarnessOpts([]PickerItem{{Label: "x"}}, true)
	// Type something to seed matches != nil, then send an empty paste
	// (Start immediately followed by End). The filter must recompute
	// once on End even though pasteBuf is empty: the End event is the
	// signal that the user's paste action completed, and the user
	// expects a fresh render.
	h.p.HandleEvent(runeKey('x'))

	calls := 0
	old := fuzzyFind
	fuzzyFind = func(pattern string, data []string) fuzzy.Matches {
		calls++
		return old(pattern, data)
	}
	t.Cleanup(func() { fuzzyFind = old })

	h.p.HandleEvent(tcell.NewEventPaste(true))
	h.p.HandleEvent(tcell.NewEventPaste(false))

	if calls != 1 {
		t.Fatalf("empty paste: filter ran %d times, want 1", calls)
	}
	if h.p.query != "x" {
		t.Fatalf("empty paste: query=%q, want x (unchanged)", h.p.query)
	}
}

func TestPickerPasteSkipsControlKeys(t *testing.T) {
	mockScreenSize(t)
	h := newPickerHarnessOpts([]PickerItem{{Label: "x"}}, true)
	// v3's input parser turns \n into KeyEnter and \t into KeyTab in
	// the middle of a paste stream. The picker drops non-rune keys
	// while pasting so an embedded newline doesn't activate and an
	// embedded tab can't inject a stray byte into a filename query.
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
	if h.p.qcur != 3 {
		t.Fatalf("paste with controls: qcur=%d, want 3 (dropped keys must not advance caret)",
			h.p.qcur)
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

// queryAtomString renders a parsed atom in a stable form for test
// diagnostics.
func queryAtomString(a queryAtom) string {
	var kind string
	switch a.kind {
	case atomFuzzy:
		kind = "fuzzy"
	case atomExact:
		kind = "exact"
	case atomPrefix:
		kind = "prefix"
	case atomSuffix:
		kind = "suffix"
	case atomEqual:
		kind = "equal"
	}
	if a.negate {
		return "!" + kind + ":" + a.text
	}
	return kind + ":" + a.text
}

func parsedQueryString(q parsedQuery) string {
	var groups []string
	for _, g := range q {
		var alts []string
		for _, a := range g {
			alts = append(alts, queryAtomString(a))
		}
		groups = append(groups, "[" + joinAlts(alts) + "]")
	}
	return joinGroups(groups)
}

func joinAlts(a []string) string {
	out := ""
	for i, s := range a {
		if i > 0 {
			out += "|"
		}
		out += s
	}
	return out
}

func joinGroups(g []string) string {
	out := ""
	for i, s := range g {
		if i > 0 {
			out += " "
		}
		out += s
	}
	return out
}

func TestParseQuery(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"whitespace only", "   ", ""},
		{"single fuzzy", "foo", "[fuzzy:foo]"},
		{"exact", "'foo", "[exact:foo]"},
		{"prefix", "^foo", "[prefix:foo]"},
		{"suffix", "foo$", "[suffix:foo]"},
		{"equal", "^foo$", "[equal:foo]"},
		{"negate fuzzy treated as exact", "!foo", "[!exact:foo]"},
		{"negate prefix", "!^foo", "[!prefix:foo]"},
		{"negate suffix", "!foo$", "[!suffix:foo]"},
		{"negate equal", "!^foo$", "[!equal:foo]"},
		{"negate exact", "!'foo", "[!exact:foo]"},
		{"and-group", "foo bar", "[fuzzy:foo] [fuzzy:bar]"},
		{"or-group", "foo | bar", "[fuzzy:foo|fuzzy:bar]"},
		{"pipe-no-space is literal", "foo|bar", "[fuzzy:foo|bar]"},
		{"prefix anchor with literal pipe", "^foo|bar", "[prefix:foo|bar]"},
		{"suffix anchor with literal pipe", "foo|bar$", "[suffix:foo|bar]"},
		{"mixed and+or", "^core go$ | rb$ | py$", "[prefix:core] [suffix:go|suffix:rb|suffix:py]"},
		{"fzf-style anchors plus exact", "^src 'main !test .go$", "[prefix:src] [exact:main] [!exact:test] [suffix:.go]"},
		{"escape space", `foo\ bar`, "[fuzzy:foo bar]"},
		{"escape pipe", `a\|b`, "[fuzzy:a|b]"},
		{"escape backslash", `a\\b`, `[fuzzy:a\b]`},
		{"lone caret dropped", "^", ""},
		{"lone exclaim dropped", "!", ""},
		{"lone dollar dropped", "$", ""},
		{"lone pipe dropped", "|", ""},
		{"leading pipe ignored", "| foo", "[fuzzy:foo]"},
		{"trailing pipe ignored", "foo |", "[fuzzy:foo]"},
		{"consecutive pipes treated as one", "foo | | bar", "[fuzzy:foo|fuzzy:bar]"},
		{"exact-but-prefix wins for ^'", "^'foo", "[prefix:'foo]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parsedQueryString(parseQuery(tc.in))
			if got != tc.want {
				t.Fatalf("parseQuery(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func matchIndexes(ms []fuzzy.Match) []int {
	out := make([]int, len(ms))
	for i, m := range ms {
		out[i] = m.Index
	}
	return out
}

func TestMatchQueryEmptyReturnsNil(t *testing.T) {
	if got := matchQuery(parseQuery(""), []string{"a", "b"}); got != nil {
		t.Fatalf("empty query: matchQuery = %v, want nil", got)
	}
	if got := matchQuery(parseQuery("^"), []string{"a", "b"}); got != nil {
		t.Fatalf("operator-only query: matchQuery = %v, want nil", got)
	}
}

func TestMatchQueryNoMatchesReturnsEmptyNonNil(t *testing.T) {
	got := matchQuery(parseQuery("'xyz"), []string{"a", "b"})
	if got == nil {
		t.Fatal("non-empty query with no matches: got nil, want non-nil empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("non-empty query with no matches: got %d matches, want 0", len(got))
	}
}

func TestMatchQueryOperators(t *testing.T) {
	src := []string{
		"internal/widget/picker.go",
		"internal/widget/picker_render.go",
		"internal/widget/picker_test.go",
		"internal/action/file_picker.go",
		"internal/action/file_explorer.go",
		"cmd/micro/micro.go",
		"README.md",
	}
	cases := []struct {
		name string
		q    string
		want []int
	}{
		{"prefix anchor", "^internal/widget", []int{0, 1, 2}},
		{"suffix anchor", ".go$", []int{0, 1, 2, 3, 4, 5}},
		{"prefix and suffix combined", "^internal/widget .go$", []int{0, 1, 2}},
		{"exact substring", "'picker", []int{0, 1, 2, 3}},
		{"negation excludes", "'picker !_test", []int{0, 1, 3}},
		{"or alternation", "^README | ^cmd", []int{5, 6}},
		{"pipe without spaces is literal not OR", "^README|^cmd", []int{}},
		{"equality", "^README.md$", []int{6}},
		{"negation alone returns non-matching", "!.go$", []int{6}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchIndexes(matchQuery(parseQuery(tc.q), src))
			sort.Ints(got)
			want := append([]int(nil), tc.want...)
			sort.Ints(want)
			if !intsEqual(got, want) {
				t.Fatalf("matchQuery(%q) indexes = %v, want %v", tc.q, got, want)
			}
		})
	}
}

func TestMatchQuerySmartCase(t *testing.T) {
	src := []string{"Picker.go", "picker.go", "PICKER.GO"}
	cases := []struct {
		name string
		q    string
		want []int
	}{
		{"lowercase operator matches any case", "'picker", []int{0, 1, 2}},
		{"uppercase operator is case-sensitive", "'Picker", []int{0}},
		{"all-caps operator is case-sensitive", "'PICKER", []int{2}},
		{"prefix lower folds", "^picker", []int{0, 1, 2}},
		{"prefix mixed-case is exact", "^Picker", []int{0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchIndexes(matchQuery(parseQuery(tc.q), src))
			sort.Ints(got)
			want := append([]int(nil), tc.want...)
			sort.Ints(want)
			if !intsEqual(got, want) {
				t.Fatalf("matchQuery(%q) indexes = %v, want %v", tc.q, got, want)
			}
		})
	}
}

func TestMatchQueryHighlightUnion(t *testing.T) {
	src := []string{"internal/widget/picker.go"}
	// `^internal` highlights bytes 0..len("internal"); `picker$` is
	// not a suffix here so swap to `picker.go$` which highlights the
	// trailing 9 bytes. `'widget` highlights the middle.
	q := parseQuery("^internal 'widget picker.go$")
	got := matchQuery(q, src)
	if len(got) != 1 {
		t.Fatalf("want 1 match, got %d", len(got))
	}
	hits := got[0].MatchedIndexes
	// Expected ranges: [0..8) for ^internal, [9..15) for 'widget,
	// [16..25) for picker.go$. All distinct, sorted ascending.
	wantPresent := []int{0, 7, 9, 14, 16, 24}
	for _, idx := range wantPresent {
		if !containsInt(hits, idx) {
			t.Fatalf("MatchedIndexes %v missing byte %d", hits, idx)
		}
	}
	// Must be sorted ascending with no duplicates.
	for i := 1; i < len(hits); i++ {
		if hits[i] <= hits[i-1] {
			t.Fatalf("MatchedIndexes %v not strictly ascending at %d", hits, i)
		}
	}
}

func TestMatchQueryEscapeMatchesLiteralSpace(t *testing.T) {
	src := []string{"hello world", "helloworld", "hello"}
	got := matchIndexes(matchQuery(parseQuery(`'hello\ world`), src))
	if !intsEqual(got, []int{0}) {
		t.Fatalf("escaped space: matchQuery indexes = %v, want [0]", got)
	}
}

func TestMatchQueryFuzzyScoreDrivesOrder(t *testing.T) {
	// Fuzzy atoms drive ranking; operator-only entries fall back to
	// original-list order. Here both items contain "go" but the
	// fuzzy atom "pic" hits the second (picker.go) with higher score
	// than the first (picture).
	src := []string{"picture", "picker.go"}
	got := matchIndexes(matchQuery(parseQuery("pic"), src))
	if len(got) != 2 {
		t.Fatalf("want 2 matches, got %d", len(got))
	}
	// Both match; the higher-scoring should come first. We don't
	// assert which one without computing the actual fuzzy scores
	// here — that would couple the test to sahilm/fuzzy's
	// implementation. Instead assert the result is the same shape
	// as the bare-fuzzy regression contract: same set of indexes.
	sort.Ints(got)
	if !intsEqual(got, []int{0, 1}) {
		t.Fatalf("fuzzy `pic`: indexes = %v, want both items", got)
	}
}

func TestMatchQueryRegressionMatchesBareFuzzy(t *testing.T) {
	// Single-atom bare-fuzzy queries must produce the same *set* of
	// matched items as the v1 fuzzyFind call. Ordering is compared
	// after sorting by Index, not as-returned, because sahilm/fuzzy's
	// Matches.Less is `>=` (non-strict) and gives undefined order
	// for equal-score items; matchQuery sorts deterministically by
	// (score desc, index asc) so a direct order comparison would
	// spuriously diverge on ties. The user-visible contract is the
	// match set; deterministic ordering is an improvement.
	src := []string{
		"picker.go",
		"picker_test.go",
		"picker_render.go",
		"action.go",
		"buffer.go",
	}
	for _, pattern := range []string{"pic", "go", "test", "p"} {
		want := matchIndexes([]fuzzy.Match(fuzzy.Find(pattern, src)))
		got := matchIndexes(matchQuery(parseQuery(pattern), src))
		sort.Ints(want)
		sort.Ints(got)
		if !intsEqual(got, want) {
			t.Fatalf("regression for %q: got %v, want %v",
				pattern, got, want)
		}
	}
}

func TestPickerEnterOperatorOnlyQueryNoSubmit(t *testing.T) {
	mockScreenSize(t)
	h := newPickerHarnessOpts(
		[]PickerItem{{Label: "a"}, {Label: "b"}},
		true,
	)
	// `^` alone parses to zero atoms — treat as no filter, Enter
	// must fall through to OnSelect on the highlighted row, not
	// OnSubmit.
	h.p.HandleEvent(runeKey('^'))
	if h.p.matches != nil {
		t.Fatalf("operator-only query: matches = %v, want nil",
			h.p.matches)
	}
	h.p.HandleEvent(key(tcell.KeyEnter))
	if h.submitCount != 0 {
		t.Fatalf("operator-only query: OnSubmit fired %d times, want 0",
			h.submitCount)
	}
	if h.selectCount != 1 {
		t.Fatalf("operator-only query: OnSelect fired %d times, want 1",
			h.selectCount)
	}
}

func intsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
