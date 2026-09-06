package widget

import (
	"strings"
	"unicode/utf8"

	"github.com/Tubbles/tcell/v3"

	"github.com/micro-editor/micro/v2/internal/buffer"
)

// CompletionItem is one row in a CompletionBox. It is intentionally
// decoupled from LSP's protocol.CompletionItem (and from
// internal/lsp.CompletionCandidate) so this package does not need to
// import internal/lsp; the action layer converts a completion
// response into these plus an OnSelect callback that knows how to
// apply the candidate the index refers to.
type CompletionItem struct {
	Label  string
	Detail string
	// Doc is the candidate's documentation as plain text. When the
	// highlighted item has a non-empty Doc, the box paints it in a
	// side panel so the user can judge the candidate before
	// accepting it (see drawDocsPanel).
	Doc string
	// FilterText is the string the box filters against; empty falls
	// back to Label. Mirrors LSP's CompletionItem.filterText.
	FilterText string
}

// CompletionBoxOptions configures a CompletionBox at construction.
type CompletionBoxOptions struct {
	// Pane and Loc anchor the box to a buffer position, normally the
	// cursor at the moment completion was requested.
	Pane AnchorPane
	Loc  buffer.Loc
	// Items is the candidate list. An empty Items still constructs a
	// usable (if pointless) box; callers should skip widget.Open for
	// an empty completion response instead of relying on the box to
	// suppress itself.
	Items []CompletionItem
	// Filter is the already-typed fragment of the word being
	// completed at the moment the box opened. The box only shows
	// items whose FilterText (or Label) fuzzy-matches it, and typed
	// runes that fall through to the buffer while the box is open
	// extend it, narrowing the list live (see HandleEvent).
	Filter string
	// OnSelect fires when the user accepts a row (Enter/Tab), after
	// the box has already closed itself. index is into Items.
	OnSelect func(index int)
	// OnClose fires whenever the box stops being the active widget:
	// on Esc, on accept, or when a fallthrough key dismisses it.
	OnClose func()
}

// CompletionBox is a small anchored popup listing completion
// candidates. Unlike Picker it has no query input row: the filter is
// the partially-typed word in the buffer itself, seeded by
// Options.Filter and extended as the user keeps typing "through" the
// open box.
type CompletionBox struct {
	opts CompletionBoxOptions

	// typed holds the runes typed while the box was open (they fell
	// through to the buffer as normal input). The effective filter is
	// opts.Filter + typed. Backspace pops typed runes only; erasing
	// past the invocation point closes the box, which keeps the
	// accept-time edit-range arithmetic in the action layer exact
	// (see TypedRunes).
	typed string

	// visible maps display rows to Items indexes under the current
	// filter; current indexes visible, not Items.
	visible []int
	current int
	top     int

	// prefW/prefH are recomputed from the visible subset on every
	// filter change, so the box shrinks as the list narrows. The
	// box's on-screen position still moves every frame (buffer-anchor
	// tracking).
	prefW, prefH int
}

const (
	completionMaxVisibleRows = 8
	completionMinWidth       = 12
	completionMaxWidth       = 60
)

// NewCompletionBox builds a completion popup. The box is not yet
// active; pass it to widget.Open to make it the current overlay.
func NewCompletionBox(opts CompletionBoxOptions) *CompletionBox {
	c := &CompletionBox{opts: opts}
	c.recomputeVisible()
	return c
}

// VisibleCount reports how many items survive the current filter.
// Callers use it to skip opening a box that would show nothing.
func (c *CompletionBox) VisibleCount() int {
	return len(c.visible)
}

// TypedRunes reports how many runes were typed through the box since
// it opened (net of backspaces). The action layer widens the accepted
// candidate's edit range by exactly this count so text the server
// never saw is consumed by the replacement.
func (c *CompletionBox) TypedRunes() int {
	return utf8.RuneCountInString(c.typed)
}

// filterString is the effective filter: the fragment typed before
// invocation plus everything typed through the open box.
func (c *CompletionBox) filterString() string {
	return c.opts.Filter + c.typed
}

// fuzzyMatchesFold reports whether needle is a case-insensitive
// subsequence of hay: every needle rune appears in hay in order. The
// classic completion-filter match; deliberately not a ranking, so
// surviving items keep the server's order (its sortText intent).
func fuzzyMatchesFold(needle, hay string) bool {
	needle = strings.ToLower(needle)
	hay = strings.ToLower(hay)
	for _, ch := range needle {
		idx := strings.IndexRune(hay, ch)
		if idx < 0 {
			return false
		}
		hay = hay[idx+utf8.RuneLen(ch):]
	}
	return true
}

// recomputeVisible rebuilds the visible row set for the current
// filter, keeps the highlight on the same underlying item when it
// survives the change, and resizes the box to the surviving rows.
func (c *CompletionBox) recomputeVisible() {
	selected := -1
	if c.current >= 0 && c.current < len(c.visible) {
		selected = c.visible[c.current]
	}

	filter := c.filterString()
	c.visible = c.visible[:0]
	for i, item := range c.opts.Items {
		target := item.FilterText
		if target == "" {
			target = item.Label
		}
		if filter == "" || fuzzyMatchesFold(filter, target) {
			c.visible = append(c.visible, i)
		}
	}

	c.current = 0
	for row, itemIndex := range c.visible {
		if itemIndex == selected {
			c.current = row
			break
		}
	}
	c.top = 0
	c.scrollIntoView()

	c.prefW, c.prefH = completionBoxSize(c.visibleItems())
}

func (c *CompletionBox) visibleItems() []CompletionItem {
	items := make([]CompletionItem, len(c.visible))
	for row, itemIndex := range c.visible {
		items[row] = c.opts.Items[itemIndex]
	}
	return items
}

// completionBoxSize sizes the box to fit the longest label/detail
// pair, clamped to [completionMinWidth, completionMaxWidth], and caps
// the visible row count at completionMaxVisibleRows (the box scrolls
// beyond that, see scrollIntoView).
func completionBoxSize(items []CompletionItem) (w, h int) {
	maxLabel, maxDetail := 0, 0
	for _, it := range items {
		if lw := stringWidth(it.Label); lw > maxLabel {
			maxLabel = lw
		}
		if dw := stringWidth(it.Detail); dw > maxDetail {
			maxDetail = dw
		}
	}

	w = maxLabel
	if maxDetail > 0 {
		w += maxDetail + 1 // separating space before the dimmed detail
	}
	w += 2 // left/right border columns
	if w < completionMinWidth {
		w = completionMinWidth
	}
	if w > completionMaxWidth {
		w = completionMaxWidth
	}

	rows := len(items)
	if rows > completionMaxVisibleRows {
		rows = completionMaxVisibleRows
	}
	if rows < 1 {
		rows = 1
	}
	h = rows + 2 // top/bottom border rows

	return w, h
}

// Geometry anchors the box at opts.Loc in opts.Pane, sized to the
// visible rows.
func (c *CompletionBox) Geometry() Geometry {
	return Geometry{
		Kind: GeomBufferAnchor,
		Anchor: BufferAnchor{
			Pane:       c.opts.Pane,
			Loc:        c.opts.Loc,
			PreferredW: c.prefW,
			PreferredH: c.prefH,
			Placement:  PlaceAuto,
		},
	}
}

// Close fires OnClose once; idempotent like Picker.Close.
func (c *CompletionBox) Close() {
	cb := c.opts.OnClose
	c.opts.OnClose = nil
	if cb != nil {
		cb()
	}
}

// HandleEvent absorbs navigation/accept/dismiss keys. A plain typed
// rune is NOT consumed: it falls through to the buffer as normal
// input while also extending the box's filter, so typing keeps
// narrowing the open list instead of dismissing it; Backspace
// likewise falls through and un-narrows, until it would erase past
// the invocation point, which closes the box. Any other event (a
// modified or unrecognized key, a mouse click) closes the box and
// falls through, matching how most editors dismiss an autocomplete
// popup on a keystroke that isn't itself a popup command.
func (c *CompletionBox) HandleEvent(ev tcell.Event) bool {
	e, ok := ev.(*tcell.EventKey)
	if !ok {
		CloseActive()
		return false
	}

	switch e.Key() {
	case tcell.KeyUp, tcell.KeyCtrlP:
		c.move(-1)
		return true
	case tcell.KeyDown, tcell.KeyCtrlN:
		c.move(1)
		return true
	case tcell.KeyEnter, tcell.KeyTab:
		c.accept()
		return true
	case tcell.KeyEsc:
		CloseActive()
		return true
	case tcell.KeyBackspace:
		if c.typed == "" {
			CloseActive()
			return false
		}
		runes := []rune(c.typed)
		c.typed = string(runes[:len(runes)-1])
		c.recomputeVisible()
		return false
	case tcell.KeyRune:
		if e.Modifiers()&(tcell.ModCtrl|tcell.ModAlt) != 0 {
			CloseActive()
			return false
		}
		// v3 reports keystrokes as grapheme-cluster strings; take the
		// first rune, same truncation as the picker's keystroke path.
		rs := []rune(e.Str())
		if len(rs) == 0 {
			return true
		}
		c.typed += string(rs[0])
		c.recomputeVisible()
		if len(c.visible) == 0 {
			// Nothing matches anymore: the box has nothing left to
			// offer, so get out of the user's way.
			CloseActive()
		}
		return false
	default:
		CloseActive()
		return false
	}
}

// move shifts the highlighted row by delta, clamped to the visible
// list (no wraparound).
func (c *CompletionBox) move(delta int) {
	n := len(c.visible)
	if n == 0 {
		return
	}
	x := c.current + delta
	if x < 0 {
		x = 0
	}
	if x >= n {
		x = n - 1
	}
	c.current = x
	c.scrollIntoView()
}

// accept fires OnSelect for the highlighted row's underlying item and
// then closes the box, in that order, so the callback can still see
// the box's state (e.g. TypedRunes) while it applies the edit.
func (c *CompletionBox) accept() {
	if c.current < 0 || c.current >= len(c.visible) {
		CloseActive()
		return
	}
	idx := c.visible[c.current]
	if c.opts.OnSelect != nil {
		c.opts.OnSelect(idx)
	}
	CloseActive()
}

// bodyHeight is the row count the visible item list can span,
// excluding the top/bottom border rows.
func (c *CompletionBox) bodyHeight() int {
	h := c.prefH - 2
	if h < 1 {
		h = 1
	}
	return h
}

func (c *CompletionBox) scrollIntoView() {
	bh := c.bodyHeight()
	if c.current < c.top {
		c.top = c.current
	} else if c.current >= c.top+bh {
		c.top = c.current - bh + 1
	}
	if c.top < 0 {
		c.top = 0
	}
}
