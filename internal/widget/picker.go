package widget

import (
	"sort"
	"time"
	"unicode/utf8"

	"github.com/Tubbles/tcell/v3"
	"github.com/sahilm/fuzzy"

	"github.com/micro-editor/micro/v2/internal/screen"
)

// screenSize is overridable for tests so the picker can be exercised
// without a real tcell.Screen.
var screenSize = func() (int, int) {
	if screen.Screen == nil {
		return 0, 0
	}
	return screen.Screen.Size()
}

// fuzzyFind is a package-level indirection to the fuzzy backend so
// tests can swap it for a counter (e.g. to assert that bracketed paste
// recomputes the filter once, not once per character).
var fuzzyFind = fuzzy.Find

// PickerItem is one row in a picker. Aux is rendered right-aligned
// (e.g. a file size or the trailing "/" on a directory).
type PickerItem struct {
	Label string
	Aux   string
}

// PickerOptions configures a Picker at construction time. After
// construction the items can be replaced with SetItems, etc.
type PickerOptions struct {
	Title    string
	Hint     string
	Items    []PickerItem
	Geometry Geometry
	// Query, when true, draws a single-line input row at the top of
	// the picker body and routes typed runes into a query string that
	// fuzzy-filters Items. When false (default) the picker behaves as
	// in v1: typed runes are swallowed, no input row is drawn,
	// Home/End jump the list.
	Query bool
	// OnSelect fires when the user activates a row (Enter,
	// double-click). The picker is NOT auto-closed; the callback
	// decides (it might want to refresh the items in place). The
	// supplied index is into Items, not into the (possibly filtered)
	// display list.
	OnSelect func(index int)
	// OnSubmit fires when the user presses Enter while Query is true,
	// the query is non-empty, and the filter produced no matches. The
	// callback receives the typed query verbatim. Use this to commit
	// arbitrary text from the picker (e.g. "open this path even if it
	// is not in the directory listing"). Nil leaves no-match Enter as
	// a silent no-op.
	OnSubmit func(query string)
	// OnClose fires when the user dismisses the picker (Esc, click
	// outside). The callback should treat the picker as gone; the
	// active slot is cleared by the dispatcher first.
	OnClose func()
}

// Picker is a generic list-of-rows overlay widget.
type Picker struct {
	opts PickerOptions
	// current is the index into the *displayed* list (matches when
	// non-nil, else opts.Items). Map back to opts.Items via
	// itemIndexAt.
	current int
	top     int

	// query is the typed search string when opts.Query is true.
	// qcur is the caret position, in *runes* (not bytes), within
	// query. matches holds the active fuzzy.Find result; when nil
	// the picker shows the unfiltered Items list.
	query   string
	qcur    int
	matches []fuzzy.Match

	// pasting is true between EventPaste{Start} and EventPaste{End}.
	// While set, recomputeFilter is a no-op so a long paste doesn't
	// re-run the matcher per character; on End the filter recomputes
	// once. handleKey also drops non-rune keys while pasting so a
	// pasted "\n" doesn't trigger activate() mid-paste.
	pasting bool

	lastClickTime time.Time
	lastClickRow  int

	// nowFn is used by tests to inject a fake clock for the
	// double-click window.
	nowFn func() time.Time
}

const doubleClickWindow = 500 * time.Millisecond

// NewPicker builds a picker. The picker is not yet active; pass it
// to widget.Open to make it the current overlay.
func NewPicker(opts PickerOptions) *Picker {
	return &Picker{
		opts:         opts,
		lastClickRow: -1,
		nowFn:        time.Now,
	}
}

// SetItems replaces the row list. Resets the query, caret, current
// row, and scroll offset, since a new item set is a new context.
func (p *Picker) SetItems(items []PickerItem) {
	p.opts.Items = items
	p.ResetQuery()
}

// SetTitle replaces the title shown on the top border.
func (p *Picker) SetTitle(t string) { p.opts.Title = t }

// SetHint replaces the hint line shown above the bottom border. Callers
// rebuild the hint when in-picker state changes (e.g. a visibility
// toggle flips) so the user can see the new state without dismissing
// the picker.
func (p *Picker) SetHint(s string) { p.opts.Hint = s }

// ResetQuery clears any typed query, the matches, and the highlight,
// returning the picker to a fresh just-opened state. Called by
// SetItems.
func (p *Picker) ResetQuery() {
	p.query = ""
	p.qcur = 0
	p.matches = nil
	p.current = 0
	p.top = 0
}

// SetCurrent moves the highlight to row i in the displayed list,
// clamped to that list's length. When a query is active, i indexes
// the filtered match list, not opts.Items.
func (p *Picker) SetCurrent(i int) {
	n := p.displayedLen()
	if n == 0 {
		p.current = 0
		return
	}
	if i < 0 {
		i = 0
	}
	if i >= n {
		i = n - 1
	}
	p.current = i
	p.scrollIntoView()
}

// Current returns the highlight index into the displayed list.
func (p *Picker) Current() int { return p.current }

// Geometry returns the picker's geometry, recomputed at draw time
// when buffer-anchored.
func (p *Picker) Geometry() Geometry { return p.opts.Geometry }

// Close fires the OnClose callback once, hides the input caret if
// one is showing, and is idempotent: subsequent calls do nothing.
//
// HideCursor here keeps the terminal caret from lingering on the
// (now removed) input row before the next pane redraw places it
// back on the buffer.
func (p *Picker) Close() {
	if p.opts.Query && screen.Screen != nil {
		screen.Screen.HideCursor()
	}
	cb := p.opts.OnClose
	p.opts.OnClose = nil
	if cb != nil {
		cb()
	}
}

// HandleEvent absorbs the event stream while the picker is active.
// Always returns true so events don't leak to the buffer below.
func (p *Picker) HandleEvent(ev tcell.Event) bool {
	switch e := ev.(type) {
	case *tcell.EventKey:
		p.handleKey(e)
	case *tcell.EventMouse:
		p.handleMouse(e)
	case *tcell.EventPaste:
		// tcell v3 streams a paste as Start, EventKey rune events,
		// End. The picker accumulates the runes through the regular
		// handleKey path and defers the filter recompute to End so a
		// long paste doesn't re-run fuzzy.Find per character.
		if e.Start() {
			p.pasting = true
		} else {
			p.pasting = false
			if p.opts.Query {
				p.recomputeFilter()
			}
		}
	}
	return true
}

func (p *Picker) handleKey(e *tcell.EventKey) {
	// During bracketed paste only literal rune events flow into the
	// query; control keys (Enter, Esc, arrows...) are dropped so a
	// pasted newline doesn't activate or close the picker.
	if p.pasting && e.Key() != tcell.KeyRune {
		return
	}
	if p.opts.Query {
		p.handleKeyQuery(e)
		return
	}
	p.handleKeyClassic(e)
}

func (p *Picker) handleKeyClassic(e *tcell.EventKey) {
	switch e.Key() {
	case tcell.KeyUp:
		p.move(-1)
	case tcell.KeyDown:
		p.move(1)
	case tcell.KeyPgUp:
		p.move(-p.bodyHeight())
	case tcell.KeyPgDn:
		p.move(p.bodyHeight())
	case tcell.KeyHome:
		p.SetCurrent(0)
	case tcell.KeyEnd:
		p.SetCurrent(p.displayedLen() - 1)
	case tcell.KeyEnter:
		p.activate()
	case tcell.KeyEsc:
		CloseActive()
	}
	// All other keys (incl. typed runes) are silently swallowed in
	// the classic (non-Query) picker.
}

func (p *Picker) handleKeyQuery(e *tcell.EventKey) {
	switch e.Key() {
	case tcell.KeyUp:
		p.move(-1)
	case tcell.KeyDown:
		p.move(1)
	case tcell.KeyPgUp:
		p.move(-p.bodyHeight())
	case tcell.KeyPgDn:
		p.move(p.bodyHeight())
	case tcell.KeyEnter:
		p.activate()
	case tcell.KeyEsc:
		CloseActive()
	case tcell.KeyHome:
		p.qcur = 0
	case tcell.KeyEnd:
		p.qcur = utf8.RuneCountInString(p.query)
	case tcell.KeyLeft:
		if p.qcur > 0 {
			p.qcur--
		}
	case tcell.KeyRight:
		if p.qcur < utf8.RuneCountInString(p.query) {
			p.qcur++
		}
	case tcell.KeyBackspace:
		// In v3 KeyBackspace2 (0x7f) is translated to KeyBackspace at
		// the input layer, so this single case covers Backspace from
		// any terminal.
		p.deleteBeforeCaret()
	case tcell.KeyDelete:
		p.deleteAtCaret()
	case tcell.KeyRune:
		// v3 reports keystrokes as a grapheme cluster string rather
		// than a single rune. Take the first rune; multi-rune
		// clusters are truncated on the keystroke path.
		if rs := []rune(e.Str()); len(rs) > 0 {
			p.insertRune(rs[0])
		}
	}
}

// insertRune appends a single rune at the caret and refreshes the
// filter.
func (p *Picker) insertRune(r rune) {
	if r == 0 {
		return
	}
	off := byteOffsetForRune(p.query, p.qcur)
	p.query = p.query[:off] + string(r) + p.query[off:]
	p.qcur++
	p.recomputeFilter()
}

// deleteBeforeCaret removes the rune just before the caret.
func (p *Picker) deleteBeforeCaret() {
	if p.qcur == 0 {
		return
	}
	end := byteOffsetForRune(p.query, p.qcur)
	start := byteOffsetForRune(p.query, p.qcur-1)
	p.query = p.query[:start] + p.query[end:]
	p.qcur--
	p.recomputeFilter()
}

// deleteAtCaret removes the rune at the caret.
func (p *Picker) deleteAtCaret() {
	n := utf8.RuneCountInString(p.query)
	if p.qcur >= n {
		return
	}
	start := byteOffsetForRune(p.query, p.qcur)
	end := byteOffsetForRune(p.query, p.qcur+1)
	p.query = p.query[:start] + p.query[end:]
	p.recomputeFilter()
}

// recomputeFilter runs the query layer against opts.Items.Label and
// snaps current/top to the top of the new list. The query language is
// fzf "extended-search" style: see internal/widget/picker_query.go
// for the grammar (fuzzy substrings, `'exact`, `^prefix`, `suffix$`,
// `^equal$`, `!negate`, and `|` for OR within an AND-group).
//
// Three cases distinguish what activate() does next:
//
//   - empty query string, or a query that parses to zero atoms (e.g.
//     `^` alone): matches stays nil, picker shows the unfiltered
//     list, Enter fires OnSelect on the highlighted row.
//   - parsed atoms with no surviving items: matches is a non-nil empty
//     slice, Enter fires OnSubmit with the typed query verbatim
//     (free-form path commit).
//   - parsed atoms with surviving items: matches is a populated slice,
//     Enter fires OnSelect on the highlighted row of the filtered
//     list.
//
// While p.pasting is true the call is a no-op; HandleEvent runs a
// single recompute on EventPaste{End} once the full block has been
// inserted into the query.
func (p *Picker) recomputeFilter() {
	if p.pasting {
		return
	}
	if p.query == "" {
		p.matches = nil
	} else {
		sources := make([]string, len(p.opts.Items))
		for i, it := range p.opts.Items {
			sources[i] = it.Label
		}
		p.matches = matchQuery(parseQuery(p.query), sources)
	}
	p.current = 0
	p.top = 0
}

// displayedLen is the row count that's currently visible —
// len(matches) when a query is active, else len(Items).
func (p *Picker) displayedLen() int {
	if p.matches != nil {
		return len(p.matches)
	}
	return len(p.opts.Items)
}

// itemIndexAt maps a displayed-row index to the underlying
// opts.Items index, or -1 when the displayed row is out of range.
func (p *Picker) itemIndexAt(displayed int) int {
	if displayed < 0 {
		return -1
	}
	if p.matches != nil {
		if displayed >= len(p.matches) {
			return -1
		}
		return p.matches[displayed].Index
	}
	if displayed >= len(p.opts.Items) {
		return -1
	}
	return displayed
}

// matchAt returns the fuzzy.Match for displayed-row idx, or nil when
// no query is active. Used by the renderer to bold matched runes.
func (p *Picker) matchAt(displayed int) *fuzzy.Match {
	if p.matches == nil {
		return nil
	}
	if displayed < 0 || displayed >= len(p.matches) {
		return nil
	}
	return &p.matches[displayed]
}

// isMatchedByteIdx reports whether the given byte offset in the
// label is one of the fuzzy-matched positions. MatchedIndexes is
// already sorted ascending by sahilm/fuzzy, so a binary search is
// safe.
func isMatchedByteIdx(m *fuzzy.Match, byteIdx int) bool {
	if m == nil || len(m.MatchedIndexes) == 0 {
		return false
	}
	i := sort.SearchInts(m.MatchedIndexes, byteIdx)
	return i < len(m.MatchedIndexes) && m.MatchedIndexes[i] == byteIdx
}

func (p *Picker) handleMouse(e *tcell.EventMouse) {
	mx, my := e.Position()
	sw, sh := screenSize()
	rect := Resolve(p.opts.Geometry, sw, sh)
	btn := e.Buttons()

	switch {
	case btn&tcell.WheelUp != 0:
		p.move(-1)
		return
	case btn&tcell.WheelDown != 0:
		p.move(1)
		return
	}

	// Button presses only — releases would double-fire on each
	// click. ButtonNone is the release sentinel in tcell.
	if btn == tcell.ButtonNone {
		return
	}

	if !inRect(mx, my, rect) {
		CloseActive()
		return
	}

	bodyY0 := p.bodyY0(rect)
	bodyH := p.bodyHeight()
	if my < bodyY0 || my >= bodyY0+bodyH {
		// click on top border, input row, bottom border, or hint
		// line
		return
	}
	idx := p.top + (my - bodyY0)
	if idx < 0 || idx >= p.displayedLen() {
		return
	}

	now := p.nowFn()
	if idx == p.lastClickRow && now.Sub(p.lastClickTime) <= doubleClickWindow {
		p.current = idx
		p.lastClickTime = time.Time{}
		p.lastClickRow = -1
		p.activate()
		return
	}

	p.current = idx
	p.scrollIntoView()
	p.lastClickTime = now
	p.lastClickRow = idx
}

func (p *Picker) move(delta int) {
	n := p.displayedLen()
	if n == 0 {
		return
	}
	x := p.current + delta
	if x < 0 {
		x = 0
	}
	if x >= n {
		x = n - 1
	}
	p.current = x
	p.scrollIntoView()
}

// activate fires OnSelect for the highlighted row, OnSubmit for a
// non-empty query with no matches, or no-op otherwise.
func (p *Picker) activate() {
	if p.opts.Query && p.query != "" && p.matches != nil && len(p.matches) == 0 {
		if p.opts.OnSubmit != nil {
			p.opts.OnSubmit(p.query)
		}
		return
	}
	if p.displayedLen() == 0 {
		return
	}
	idx := p.itemIndexAt(p.current)
	if idx < 0 {
		return
	}
	if p.opts.OnSelect != nil {
		p.opts.OnSelect(idx)
	}
}

// bodyHeight is the row count the visible item list can span. It
// excludes both border rows and the input row when Query is true.
func (p *Picker) bodyHeight() int {
	sw, sh := screenSize()
	rect := Resolve(p.opts.Geometry, sw, sh)
	overhead := 2 // top border + bottom border
	if p.opts.Query {
		overhead++ // input row
	}
	h := rect.H - overhead
	if h < 1 {
		h = 1
	}
	return h
}

// bodyY0 is the y of the first item row inside rect.
func (p *Picker) bodyY0(rect ScreenRect) int {
	if p.opts.Query {
		return rect.Y + 2
	}
	return rect.Y + 1
}

func (p *Picker) scrollIntoView() {
	bh := p.bodyHeight()
	if p.current < p.top {
		p.top = p.current
	} else if p.current >= p.top+bh {
		p.top = p.current - bh + 1
	}
	if p.top < 0 {
		p.top = 0
	}
}

// byteOffsetForRune walks s and returns the byte offset of the
// runeIdx-th rune. byteOffsetForRune(s, runeCount) == len(s).
func byteOffsetForRune(s string, runeIdx int) int {
	off := 0
	count := 0
	for off < len(s) {
		if count == runeIdx {
			return off
		}
		_, size := utf8.DecodeRuneInString(s[off:])
		off += size
		count++
	}
	return off
}

func inRect(x, y int, r ScreenRect) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}
