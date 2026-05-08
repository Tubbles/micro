// Package widget is the overlay layer for floating UI elements
// (pickers, autocomplete popups, etc) that paint on top of the
// regular pane layout.
//
// Dispatch order in cmd/micro/micro.go is widget > InfoBar prompt
// > active tab/pane: when a widget is active it consumes the event
// stream, so Esc closes the widget before anything below sees it.
//
// v1 holds at most one active widget. The // TODO(stack) comment
// in this file marks the spot where the single Widget slot should
// become a slice if/when nested overlays land.
package widget

import (
	"github.com/micro-editor/tcell/v2"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/display"
)

// Widget is implemented by every overlay that wants to be painted
// above the panes and to consume events while it is open.
type Widget interface {
	Display()
	HandleEvent(ev tcell.Event) (consumed bool)
	Geometry() Geometry
	Close()
}

// GeometryKind discriminates the two ways a widget can position
// itself on the screen.
type GeometryKind int

const (
	// GeomScreenRect places the widget at an absolute screen rect.
	GeomScreenRect GeometryKind = iota
	// GeomBufferAnchor anchors the widget to a buffer location in a
	// BufPane. Reserved for future popups that follow the cursor;
	// no v1 widget uses this path.
	GeomBufferAnchor
)

// Placement controls which side of a buffer anchor the widget sits
// on. Auto picks the side with more room.
type Placement int

const (
	PlaceAuto Placement = iota
	PlaceAbove
	PlaceBelow
)

// ScreenRect is an absolute rectangle in screen cells.
type ScreenRect struct {
	X, Y, W, H int
}

// AnchorPane is the minimal capability the buffer-anchor resolver
// needs from a BufPane. Defining it here breaks the import cycle
// that would otherwise form between internal/widget and
// internal/action.
type AnchorPane interface {
	GetView() *display.View
	BufView() display.View
}

// BufferAnchor anchors a widget to a buffer location. The widget is
// repositioned each frame so it tracks the underlying buffer as it
// scrolls. v1 has no consumer; the resolver math is exercised by
// unit tests.
type BufferAnchor struct {
	Pane                   AnchorPane
	Loc                    buffer.Loc
	PreferredW, PreferredH int
	Placement              Placement
}

// Geometry is the sum type returned from Widget.Geometry. Exactly
// one of Rect or Anchor is meaningful, selected by Kind.
type Geometry struct {
	Kind   GeometryKind
	Rect   ScreenRect
	Anchor BufferAnchor
}

// Resolve turns a Geometry into a concrete on-screen rectangle,
// clipped to screen bounds. For buffer-anchored widgets it consults
// the anchor pane's view to compute the caret cell, then places the
// widget above or below per Placement.
func Resolve(g Geometry, screenW, screenH int) ScreenRect {
	switch g.Kind {
	case GeomScreenRect:
		return clipRect(g.Rect, screenW, screenH)
	case GeomBufferAnchor:
		return resolveAnchor(g.Anchor, screenW, screenH)
	}
	return ScreenRect{}
}

func clipRect(r ScreenRect, screenW, screenH int) ScreenRect {
	if r.X < 0 {
		r.W += r.X
		r.X = 0
	}
	if r.Y < 0 {
		r.H += r.Y
		r.Y = 0
	}
	if r.X+r.W > screenW {
		r.W = screenW - r.X
	}
	if r.Y+r.H > screenH {
		r.H = screenH - r.Y
	}
	if r.W < 0 {
		r.W = 0
	}
	if r.H < 0 {
		r.H = 0
	}
	return r
}

// resolveAnchor places a buffer-anchored widget. The caret cell is
// (view.X + (Loc.X - StartCol), view.Y + (Loc.Y - StartLine.Line)),
// approximated; soft-wrap and tab expansion are not handled here
// (no v1 consumer). Below is preferred unless that would clip and
// above has more room.
func resolveAnchor(a BufferAnchor, screenW, screenH int) ScreenRect {
	if a.Pane == nil {
		return ScreenRect{}
	}
	v := a.Pane.GetView()
	bv := a.Pane.BufView()
	cx := bv.X + (a.Loc.X - v.StartCol)
	cy := bv.Y + (a.Loc.Y - v.StartLine.Line)

	w, h := a.PreferredW, a.PreferredH
	if w <= 0 {
		w = 1
	}
	if h <= 0 {
		h = 1
	}

	below := cy + 1
	above := cy - h

	place := a.Placement
	if place == PlaceAuto {
		if below+h <= screenH {
			place = PlaceBelow
		} else if above >= 0 {
			place = PlaceAbove
		} else {
			// neither side fits; below clips less in practice
			place = PlaceBelow
		}
	}

	var y int
	if place == PlaceAbove {
		y = above
	} else {
		y = below
	}

	x := cx
	if x+w > screenW {
		x = screenW - w
	}
	if x < 0 {
		x = 0
	}
	return clipRect(ScreenRect{X: x, Y: y, W: w, H: h}, screenW, screenH)
}

// TODO(stack): replace `active` with a stack to support layered
// widgets (e.g. a confirm popup over the picker). The HandleEvent
// dispatch would then walk top-down until one returns consumed.
var active Widget

// Open registers w as the active widget, closing any previously
// active widget. Open(nil) is a silent no-op so callers can simply
// not pass nil; if they do, behavior is intentionally inert.
func Open(w Widget) {
	if w == nil {
		return
	}
	if active != nil && active != w {
		active.Close()
	}
	active = w
}

// Close clears w from the active slot if it is currently active. If
// some other widget is active, the call is a no-op (so a stale
// reference cannot evict the current widget).
func Close(w Widget) {
	if active == w {
		active = nil
	}
}

// CloseActive drops whatever widget is currently active, calling
// Close on it first. Used when the dispatcher needs to dismiss the
// overlay regardless of identity.
func CloseActive() {
	if active != nil {
		w := active
		active = nil
		w.Close()
	}
}

// Active returns the current active widget, or nil if none.
func Active() Widget {
	return active
}

// HandleEvent forwards the event to the active widget. Returns
// false if no widget is active (so the caller falls through to the
// pane dispatch). When a widget is active it is expected to consume
// the event regardless of whether it was acted on, so the buffer
// below stays inert.
func HandleEvent(ev tcell.Event) bool {
	if active == nil {
		return false
	}
	return active.HandleEvent(ev)
}

// Display draws the active widget. No-op when none is active.
func Display() {
	if active == nil {
		return
	}
	active.Display()
}
