package widget

import (
	runewidth "github.com/mattn/go-runewidth"

	"github.com/Tubbles/tcell/v3"

	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
)

// Display paints the popup into its current geometry rect with the
// shared widget frame style: border, centered title, wrapped text
// body.
func (p *Popup) Display() {
	sw, sh := screenSize()
	rect := Resolve(p.Geometry(), sw, sh)
	if rect.W < 3 || rect.H < 3 {
		return // too small to render anything legible
	}

	frame := config.GetColor("widget-frame")
	row := config.GetColor("widget-row")

	lines := wrapToWidth(p.opts.Text, rect.W-2)
	bodyH := rect.H - 2
	p.lastBodyH = bodyH
	p.lastLineCount = len(lines)
	// Re-clamp against the current wrap: a resize can shrink the line
	// count below an older scroll offset.
	p.scroll(0)

	p.fillBackground(rect, row)
	p.drawBorder(rect, frame)
	p.drawTitle(rect, frame)
	p.drawBody(rect, lines, row)
}

func (p *Popup) fillBackground(r ScreenRect, st tcell.Style) {
	for y := r.Y + 1; y < r.Y+r.H-1; y++ {
		for x := r.X + 1; x < r.X+r.W-1; x++ {
			screen.SetContent(x, y, ' ', nil, st)
		}
	}
}

func (p *Popup) drawBorder(r ScreenRect, st tcell.Style) {
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

func (p *Popup) drawTitle(r ScreenRect, st tcell.Style) {
	title := p.opts.Title
	if title == "" {
		return
	}
	title = " " + title + " "
	tw := stringWidth(title)
	if tw > r.W-2 {
		title = truncateToWidth(title, r.W-2)
		tw = stringWidth(title)
	}
	x := r.X + (r.W-tw)/2
	for _, ch := range title {
		screen.SetContent(x, r.Y, ch, nil, st)
		x += runewidth.RuneWidth(ch)
	}
}

func (p *Popup) drawBody(r ScreenRect, lines []string, st tcell.Style) {
	bodyY0 := r.Y + 1
	bodyH := r.H - 2
	bodyX0 := r.X + 1

	for j := 0; j < bodyH; j++ {
		idx := p.top + j
		if idx < 0 || idx >= len(lines) {
			continue
		}
		x := bodyX0
		for _, ch := range lines[idx] {
			screen.SetContent(x, bodyY0+j, ch, nil, st)
			x += runewidth.RuneWidth(ch)
		}
	}
}
