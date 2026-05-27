package display

import (
	"os"

	runewidth "github.com/mattn/go-runewidth"
	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/micro/v2/internal/util"
)

// PathBar is a one-row strip drawn above the editor area showing the
// active buffer's file path relative to the current working directory.
// It is gated on the global "pathbar" setting; when off, no row is
// reserved and the editor area extends to where the tab bar (or the
// top of the screen) sits today.
type PathBar struct {
	Y     int
	Width int
}

// NewPathBar returns a fresh path bar with zero geometry. Call Resize
// before drawing.
func NewPathBar() *PathBar {
	return &PathBar{}
}

// Resize updates the row position and width of the bar.
func (p *PathBar) Resize(y, width int) {
	p.Y = y
	p.Width = width
}

// resolvePathText returns the string to show for the active buffer:
// the cwd-relative path for file-backed default buffers, otherwise the
// buffer's display name (covering Help, Log, Scratch, Raw, Stdout,
// Info, and unsaved no-name buffers).
func resolvePathText(b *buffer.Buffer) string {
	if b == nil {
		return ""
	}
	if b.Type.Kind != buffer.BTDefault.Kind || b.Path == "" {
		return b.GetName()
	}
	cwd, err := os.Getwd()
	if err != nil {
		return b.GetName()
	}
	rel, err := util.MakeRelative(b.AbsPath, cwd)
	if err != nil {
		return b.AbsPath
	}
	return rel
}

// Display draws the path bar for the given active buffer. Long paths
// are truncated from the left with a leading "<" marker so the
// basename stays on screen.
func (p *PathBar) Display(b *buffer.Buffer) {
	if p.Width <= 0 {
		return
	}

	style := config.DefStyle.Reverse(true)
	if s, ok := config.Colorscheme["pathbar"]; ok {
		style = s
	} else if s, ok := config.Colorscheme["statusline"]; ok {
		style = s
	}

	runes := []rune(resolvePathText(b))
	widths := make([]int, len(runes))
	total := 0
	for i, r := range runes {
		widths[i] = runewidth.RuneWidth(r)
		total += widths[i]
	}

	start := 0
	if total > p.Width {
		budget := p.Width - 1
		for start < len(runes) && total > budget {
			total -= widths[start]
			start++
		}
	}

	x := 0
	if start > 0 && x < p.Width {
		screen.SetContent(x, p.Y, '<', nil, style)
		x++
	}
	for i := start; i < len(runes); i++ {
		if x >= p.Width {
			break
		}
		screen.SetContent(x, p.Y, runes[i], nil, style)
		x += widths[i]
	}
	for x < p.Width {
		screen.SetContent(x, p.Y, ' ', nil, style)
		x++
	}
}
