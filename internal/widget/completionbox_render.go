package widget

import (
	runewidth "github.com/mattn/go-runewidth"

	"github.com/Tubbles/tcell/v3"

	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
)

// Display paints the box into its current geometry rect, reusing the
// same box-drawing style (corners/borders/rows) as Picker.
func (c *CompletionBox) Display() {
	sw, sh := screenSize()
	rect := Resolve(c.Geometry(), sw, sh)
	if rect.W < 3 || rect.H < 3 {
		return // too small to render anything legible
	}

	frame := config.GetColor("widget-frame")
	row := config.GetColor("widget-row")
	rowCur := config.GetColor("widget-row-current")
	if rowCur == config.DefStyle {
		// fall back to a reverse-video highlight when the colorscheme
		// has no explicit "widget-row-current" group, so the current
		// row is always visually distinct.
		rowCur = row.Reverse(true)
	}

	c.fillBackground(rect, row)
	c.drawBorder(rect, frame)
	c.drawBody(rect, row, rowCur, frame)
	c.drawDocsPanel(rect, row, frame, sw, sh)
}

const (
	completionDocsMinWidth  = 20
	completionDocsMaxWidth  = 54
	completionDocsMaxHeight = 14
)

// drawDocsPanel paints the highlighted candidate's documentation in a
// second bordered box beside the completion box: to its right when
// there is room, to its left otherwise, and not at all when neither
// side fits completionDocsMinWidth. The text is wrapped plain text
// (markdown is not rendered) and truncated vertically at
// completionDocsMaxHeight; the panel exists to judge a candidate at a
// glance, not to read a full manual page.
func (c *CompletionBox) drawDocsPanel(boxRect ScreenRect, row, frame tcell.Style, sw, sh int) {
	if c.current < 0 || c.current >= len(c.visible) {
		return
	}
	doc := c.opts.Items[c.visible[c.current]].Doc
	if doc == "" {
		return
	}

	rightRoom := sw - (boxRect.X + boxRect.W)
	leftRoom := boxRect.X
	var x, w int
	switch {
	case rightRoom >= completionDocsMinWidth:
		w = min(completionDocsMaxWidth, rightRoom)
		x = boxRect.X + boxRect.W
	case leftRoom >= completionDocsMinWidth:
		w = min(completionDocsMaxWidth, leftRoom)
		x = boxRect.X - w
	default:
		return
	}

	lines := wrapStyledLines(TextToLines(doc), w-2)
	h := len(lines) + 2
	if h > completionDocsMaxHeight {
		h = completionDocsMaxHeight
	}
	rect := clipRect(ScreenRect{X: x, Y: boxRect.Y, W: w, H: h}, sw, sh)
	if rect.W < 3 || rect.H < 3 {
		return
	}

	c.fillBackground(rect, row)
	c.drawBorder(rect, frame)
	for j := 0; j < rect.H-2 && j < len(lines); j++ {
		drawStyledLine(lines[j], rect.X+1, rect.Y+1+j, rect.W-2, row)
	}
}

func (c *CompletionBox) fillBackground(r ScreenRect, st tcell.Style) {
	for y := r.Y + 1; y < r.Y+r.H-1; y++ {
		for x := r.X + 1; x < r.X+r.W-1; x++ {
			screen.SetContent(x, y, ' ', nil, st)
		}
	}
}

func (c *CompletionBox) drawBorder(r ScreenRect, st tcell.Style) {
	screen.SetContent(r.X, r.Y, pickerCornerTL, nil, st)
	screen.SetContent(r.X+r.W-1, r.Y, pickerCornerTR, nil, st)
	screen.SetContent(r.X, r.Y+r.H-1, pickerCornerBL, nil, st)
	screen.SetContent(r.X+r.W-1, r.Y+r.H-1, pickerCornerBR, nil, st)
	for x := r.X + 1; x < r.X+r.W-1; x++ {
		screen.SetContent(x, r.Y, pickerHoriz, nil, st)
		screen.SetContent(x, r.Y+r.H-1, pickerHoriz, nil, st)
	}
	for y := r.Y + 1; y < r.Y+r.H-1; y++ {
		screen.SetContent(r.X, y, pickerVert, nil, st)
		screen.SetContent(r.X+r.W-1, y, pickerVert, nil, st)
	}
}

// drawBody renders the (possibly scrolled) filtered rows. Each row's
// Label is left-aligned; Detail is right-aligned in frame's dimmer
// style when it fits alongside the label, and dropped entirely rather
// than overlapping the label when it doesn't.
func (c *CompletionBox) drawBody(r ScreenRect, row, rowCur, frame tcell.Style) {
	bodyY0 := r.Y + 1
	bodyH := r.H - 2
	bodyX0 := r.X + 1
	bodyW := r.W - 2

	for j := 0; j < bodyH; j++ {
		idx := c.top + j
		y := bodyY0 + j
		st := row
		detailSt := frame
		if idx == c.current {
			st = rowCur
			detailSt = rowCur
			for x := bodyX0; x < bodyX0+bodyW; x++ {
				screen.SetContent(x, y, ' ', nil, st)
			}
		}
		if idx < 0 || idx >= len(c.visible) {
			continue
		}
		it := c.opts.Items[c.visible[idx]]

		detailW := stringWidth(it.Detail)
		labelMax := bodyW
		if detailW > 0 {
			labelMax -= detailW + 1
		}
		if labelMax < 1 {
			labelMax = 1
		}
		label := truncateToWidth(it.Label, labelMax)

		x := bodyX0
		for _, ch := range label {
			screen.SetContent(x, y, ch, nil, st)
			x += runewidth.RuneWidth(ch)
		}

		if detailW > 0 && bodyW-stringWidth(label) > detailW {
			dx := bodyX0 + bodyW - detailW
			for _, ch := range it.Detail {
				screen.SetContent(dx, y, ch, nil, detailSt)
				dx += runewidth.RuneWidth(ch)
			}
		}
	}
}
