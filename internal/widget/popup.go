package widget

import (
	runewidth "github.com/mattn/go-runewidth"

	"github.com/Tubbles/tcell/v3"
)

// PopupOptions configures a Popup at construction.
type PopupOptions struct {
	// Title is drawn centered in the top border, like Picker's title.
	Title string
	// Text is the popup's content. It is split on newlines and
	// soft-wrapped to the popup's inner width at render time, so
	// callers pass it unwrapped.
	Text string
	// Geometry places the popup; both screen-rect and buffer-anchored
	// geometries work.
	Geometry Geometry
	// OnClose fires once when the popup stops being the active widget.
	OnClose func()
}

// Popup is a modal read-only text overlay: it shows Text until the
// user dismisses it with Esc (or Enter). Up/Down and PageUp/PageDown
// scroll long content; every other event is consumed without effect,
// which is what makes it modal. It is deliberately generic (no LSP
// coupling): any feature that wants a transient text panel can open
// one.
type Popup struct {
	opts PopupOptions
	top  int

	// lastBodyH and lastLineCount memoize the most recent render's
	// dimensions so key handling can clamp scrolling without
	// re-resolving geometry (events can arrive before the first
	// Display on a zero-size screen; the zero values are safe).
	lastBodyH     int
	lastLineCount int
}

// NewPopup builds a popup. It is not yet active; pass it to
// widget.Open to make it the current overlay.
func NewPopup(opts PopupOptions) *Popup {
	return &Popup{opts: opts}
}

// Geometry returns the placement given at construction.
func (p *Popup) Geometry() Geometry {
	return p.opts.Geometry
}

// Close fires OnClose once; idempotent like Picker.Close.
func (p *Popup) Close() {
	cb := p.opts.OnClose
	p.opts.OnClose = nil
	if cb != nil {
		cb()
	}
}

// HandleEvent scrolls on navigation keys and dismisses on Esc or
// Enter. Everything else is consumed without effect: the popup is
// modal, so no event leaks to the pane below while it is open.
func (p *Popup) HandleEvent(ev tcell.Event) bool {
	e, ok := ev.(*tcell.EventKey)
	if !ok {
		return true
	}

	switch e.Key() {
	case tcell.KeyUp, tcell.KeyCtrlP:
		p.scroll(-1)
	case tcell.KeyDown, tcell.KeyCtrlN:
		p.scroll(1)
	case tcell.KeyPgUp:
		p.scroll(-p.pageSize())
	case tcell.KeyPgDn:
		p.scroll(p.pageSize())
	case tcell.KeyEsc, tcell.KeyEnter:
		CloseActive()
	}
	return true
}

func (p *Popup) pageSize() int {
	if p.lastBodyH > 1 {
		return p.lastBodyH
	}
	return 1
}

// scroll moves the view window by delta lines, clamped so the last
// content line never scrolls above the bottom of the body.
func (p *Popup) scroll(delta int) {
	p.top += delta
	max := p.lastLineCount - p.lastBodyH
	if p.top > max {
		p.top = max
	}
	if p.top < 0 {
		p.top = 0
	}
}

// wrapToWidth splits text on newlines and soft-wraps each line to at
// most width display cells (rune-width aware, no word-boundary
// preference: a hover payload is arbitrary text, and mid-word wrap is
// better than dropping content). width < 1 yields one line per input
// line, unwrapped.
func wrapToWidth(text string, width int) []string {
	var out []string
	start := 0
	for i := 0; i <= len(text); i++ {
		if i == len(text) || text[i] == '\n' {
			out = append(out, wrapLine(text[start:i], width)...)
			start = i + 1
		}
	}
	return out
}

func wrapLine(line string, width int) []string {
	if width < 1 {
		return []string{line}
	}
	var out []string
	current := ""
	currentW := 0
	for _, ch := range line {
		chW := runewidth.RuneWidth(ch)
		if currentW+chW > width && current != "" {
			out = append(out, current)
			current = ""
			currentW = 0
		}
		current += string(ch)
		currentW += chW
	}
	out = append(out, current)
	return out
}
