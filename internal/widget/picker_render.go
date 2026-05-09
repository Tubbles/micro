package widget

import (
	runewidth "github.com/mattn/go-runewidth"

	"github.com/Tubbles/tcell/v3"

	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
)

const (
	pickerCornerTL = '┌'
	pickerCornerTR = '┐'
	pickerCornerBL = '└'
	pickerCornerBR = '┘'
	pickerHoriz    = '─'
	pickerVert     = '│'
)

// Display paints the picker into its current geometry rect.
func (p *Picker) Display() {
	sw, sh := screenSize()
	rect := Resolve(p.opts.Geometry, sw, sh)
	if rect.W < 4 || rect.H < 3 {
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

	p.fillBackground(rect, row)
	p.drawBorder(rect, frame)
	p.drawTitle(rect, frame)
	if p.opts.Query {
		p.drawInputRow(rect, row, frame)
	}
	p.drawBody(rect, row, rowCur)
	p.drawHint(rect, frame)
}

func (p *Picker) fillBackground(r ScreenRect, st tcell.Style) {
	for y := r.Y + 1; y < r.Y+r.H-1; y++ {
		for x := r.X + 1; x < r.X+r.W-1; x++ {
			screen.SetContent(x, y, ' ', nil, st)
		}
	}
}

func (p *Picker) drawBorder(r ScreenRect, st tcell.Style) {
	// corners
	screen.SetContent(r.X, r.Y, pickerCornerTL, nil, st)
	screen.SetContent(r.X+r.W-1, r.Y, pickerCornerTR, nil, st)
	screen.SetContent(r.X, r.Y+r.H-1, pickerCornerBL, nil, st)
	screen.SetContent(r.X+r.W-1, r.Y+r.H-1, pickerCornerBR, nil, st)
	// horizontals
	for x := r.X + 1; x < r.X+r.W-1; x++ {
		screen.SetContent(x, r.Y, pickerHoriz, nil, st)
		screen.SetContent(x, r.Y+r.H-1, pickerHoriz, nil, st)
	}
	// verticals
	for y := r.Y + 1; y < r.Y+r.H-1; y++ {
		screen.SetContent(r.X, y, pickerVert, nil, st)
		screen.SetContent(r.X+r.W-1, y, pickerVert, nil, st)
	}
}

func (p *Picker) drawTitle(r ScreenRect, st tcell.Style) {
	if p.opts.Title == "" {
		return
	}
	// centre " title " in the top border, with surrounding spaces
	// so the box-drawing characters are not adjacent to text.
	maxW := r.W - 4
	if maxW < 1 {
		return
	}
	t := truncateToWidth(p.opts.Title, maxW)
	tw := stringWidth(t)
	x := r.X + (r.W-tw-2)/2
	if x < r.X+1 {
		x = r.X + 1
	}
	screen.SetContent(x, r.Y, ' ', nil, st)
	x++
	for _, ch := range t {
		screen.SetContent(x, r.Y, ch, nil, st)
		x += runewidth.RuneWidth(ch)
	}
	screen.SetContent(x, r.Y, ' ', nil, st)
}

// drawInputRow paints the "> query" row at r.Y+1 and positions the
// terminal cursor at the caret. The prefix uses the frame style so
// it's visually demoted from the typed text.
func (p *Picker) drawInputRow(r ScreenRect, rowSt tcell.Style, frameSt tcell.Style) {
	y := r.Y + 1
	x := r.X + 1
	// padding " > "
	screen.SetContent(x, y, ' ', nil, rowSt)
	x++
	screen.SetContent(x, y, '>', nil, frameSt)
	x++
	screen.SetContent(x, y, ' ', nil, rowSt)
	x++

	queryX0 := x
	availW := r.X + r.W - 1 - x
	q := truncateToWidth(p.query, availW)
	for _, ch := range q {
		screen.SetContent(x, y, ch, nil, rowSt)
		x += runewidth.RuneWidth(ch)
	}

	caretByteOff := byteOffsetForRune(p.query, p.qcur)
	if caretByteOff > len(q) {
		caretByteOff = len(q)
	}
	caretCellOffset := stringWidth(p.query[:caretByteOff])
	if screen.Screen != nil {
		screen.ShowCursor(queryX0+caretCellOffset, y)
	}
}

func (p *Picker) drawBody(r ScreenRect, row, rowCur tcell.Style) {
	bodyY0 := p.bodyY0(r)
	bodyH := p.bodyHeight()
	bodyX0 := r.X + 1
	bodyW := r.W - 2

	for j := 0; j < bodyH; j++ {
		idx := p.top + j
		y := bodyY0 + j
		st := row
		if idx == p.current {
			st = rowCur
			// paint the whole row with the highlight style so the
			// background colour extends to the right edge.
			for x := bodyX0; x < bodyX0+bodyW; x++ {
				screen.SetContent(x, y, ' ', nil, st)
			}
		}
		if idx >= p.displayedLen() {
			continue
		}
		itIdx := p.itemIndexAt(idx)
		if itIdx < 0 {
			continue
		}
		it := p.opts.Items[itIdx]
		match := p.matchAt(idx)
		auxW := stringWidth(it.Aux)
		labelMax := bodyW - 1 // 1 cell of left padding
		if auxW > 0 {
			labelMax -= auxW + 1 // separator space before Aux
		}
		if labelMax < 1 {
			labelMax = 1
		}
		label := truncateToWidth(it.Label, labelMax)
		x := bodyX0 + 1
		byteOff := 0
		for _, ch := range label {
			cellSt := st
			if isMatchedByteIdx(match, byteOff) {
				cellSt = st.Bold(true)
			}
			screen.SetContent(x, y, ch, nil, cellSt)
			x += runewidth.RuneWidth(ch)
			byteOff += len(string(ch))
		}
		if auxW > 0 {
			ax := bodyX0 + bodyW - auxW
			for _, ch := range it.Aux {
				screen.SetContent(ax, y, ch, nil, st)
				ax += runewidth.RuneWidth(ch)
			}
		}
	}
}

func (p *Picker) drawHint(r ScreenRect, st tcell.Style) {
	hint := p.opts.Hint
	if hint == "" {
		hint = "<Up>/<Down> move - <Enter> select - <Esc> cancel"
	}
	maxW := r.W - 4
	if maxW < 1 {
		return
	}
	h := truncateToWidth(hint, maxW)
	hw := stringWidth(h)
	x := r.X + (r.W-hw-2)/2
	if x < r.X+1 {
		x = r.X + 1
	}
	screen.SetContent(x, r.Y+r.H-1, ' ', nil, st)
	x++
	for _, ch := range h {
		screen.SetContent(x, r.Y+r.H-1, ch, nil, st)
		x += runewidth.RuneWidth(ch)
	}
	screen.SetContent(x, r.Y+r.H-1, ' ', nil, st)
}

func stringWidth(s string) int {
	w := 0
	for _, r := range s {
		w += runewidth.RuneWidth(r)
	}
	return w
}

func truncateToWidth(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	w := 0
	for i, r := range s {
		rw := runewidth.RuneWidth(r)
		if w+rw > maxW {
			if w+1 <= maxW {
				return s[:i] + "…"
			}
			return s[:i]
		}
		w += rw
	}
	return s
}
