package buffer

// Anchor is a region of a SharedBuffer whose bounds follow the text through
// later edits, the way live cursors do. It lets work that finishes
// asynchronously (a background job, an LSP reply) act on the region it was
// started from even though the user kept editing in the meantime.
//
// Bounds are kept in sync through OnTextEditListeners, so they track the
// single-delta Insert / Remove / undo / redo paths. A bound inside a removed
// span collapses to the start of the removal. Paths that bypass the
// listeners (MultipleReplace and other Execute-direct callers) leave an
// anchor stale, which is why callers should compare Text() with what they
// captured before acting on one.
type Anchor struct {
	start Loc
	end   Loc
	buf   *SharedBuffer
}

// Start returns the current start of the anchored region.
func (a *Anchor) Start() Loc {
	return a.start
}

// End returns the current end of the anchored region.
func (a *Anchor) End() Loc {
	return a.end
}

// Text returns the text the anchor currently spans, or nil when its bounds
// no longer lie inside the buffer.
func (a *Anchor) Text() []byte {
	if !locInBounds(a.start, a.buf.LineArray) || !locInBounds(a.end, a.buf.LineArray) || a.start.GreaterThan(a.end) {
		return nil
	}
	return a.buf.Substr(a.start, a.end)
}

// AddAnchor starts tracking the region between start and end (given in
// either order). Call RemoveAnchor once the anchor is no longer needed.
func (b *SharedBuffer) AddAnchor(start, end Loc) *Anchor {
	if start.GreaterThan(end) {
		start, end = end, start
	}
	anchor := &Anchor{start: start, end: end, buf: b}
	b.anchors = append(b.anchors, anchor)
	return anchor
}

// RemoveAnchor stops tracking anchor. Removing it twice is a no-op.
func (b *SharedBuffer) RemoveAnchor(anchor *Anchor) {
	for index, candidate := range b.anchors {
		if candidate == anchor {
			b.anchors = append(b.anchors[:index], b.anchors[index+1:]...)
			return
		}
	}
}

// shiftAnchorLoc moves one anchor bound past a text event. A bound strictly
// inside a removed span collapses to the start of the span; everything else
// follows the same rules as live cursors.
func shiftAnchorLoc(loc, start, end Loc, eventType, lastnl, textX int, la *LineArray) Loc {
	if eventType == TextEventRemove && loc.GreaterThan(start) && end.GreaterThan(loc) {
		return start
	}
	return ShiftLoc(loc, start, end, eventType, lastnl, textX, la)
}

func shiftAnchors(b *SharedBuffer, start, end Loc, eventType, lastnl, textX int) {
	for _, anchor := range b.anchors {
		anchor.start = shiftAnchorLoc(anchor.start, start, end, eventType, lastnl, textX, b.LineArray)
		anchor.end = shiftAnchorLoc(anchor.end, start, end, eventType, lastnl, textX, b.LineArray)
	}
}

func init() {
	RegisterOnTextEditListener(shiftAnchors)
}
