package widget

import (
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
	// OnSelect fires when the user accepts a row (Enter/Tab), after
	// the box has already closed itself. index is into Items.
	OnSelect func(index int)
	// OnClose fires whenever the box stops being the active widget:
	// on Esc, on accept, or when a fallthrough key dismisses it.
	OnClose func()
}

// CompletionBox is a small anchored popup listing completion
// candidates. Unlike Picker it has no query/filter input: v2
// completion is manual-trigger only, so the candidate list is fixed
// for the box's lifetime.
type CompletionBox struct {
	opts    CompletionBoxOptions
	current int
	top     int

	// prefW/prefH are the box's content-driven size, computed once at
	// construction from Items and reused by every Geometry() call. The
	// box's on-screen position still moves every frame (buffer-anchor
	// tracking), only the size is fixed.
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
	w, h := completionBoxSize(opts.Items)
	return &CompletionBox{opts: opts, prefW: w, prefH: h}
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
// content-driven prefW/prefH computed at construction.
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

// HandleEvent absorbs navigation/accept/dismiss keys. Any other event
// (an unrecognized key, or a non-key event such as a mouse click)
// closes the box and returns false so the event falls through to the
// pane below, matching how most editors dismiss an autocomplete popup
// on any keystroke that isn't itself a popup command.
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
	default:
		CloseActive()
		return false
	}
}

// move shifts the highlighted row by delta, clamped to the item list
// (no wraparound).
func (c *CompletionBox) move(delta int) {
	n := len(c.opts.Items)
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

// accept fires OnSelect for the highlighted row and then closes the
// box, in that order, so the callback can still see the box's state
// (e.g. via a closure over the candidate list) while it applies the
// edit.
func (c *CompletionBox) accept() {
	if c.current < 0 || c.current >= len(c.opts.Items) {
		CloseActive()
		return
	}
	idx := c.current
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
