package widget

import (
	runewidth "github.com/mattn/go-runewidth"

	"github.com/Tubbles/tcell/v3"

	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
)

// StyledSpan is a run of text drawn with the colorscheme highlight
// group named Group; "" means the widget's own base style. Widgets
// resolve the group via config.GetColor at draw time and apply only
// its foreground over their own background, so syntax coloring
// composes with row and selection backgrounds instead of punching
// holes into them.
type StyledSpan struct {
	Text  string
	Group string
}

// StyledLine is one display line of styled spans.
type StyledLine []StyledSpan

// PlainLine wraps an unstyled string as a single-span StyledLine.
func PlainLine(text string) StyledLine {
	return StyledLine{{Text: text}}
}

// width returns the line's display width in cells.
func (l StyledLine) width() int {
	w := 0
	for _, span := range l {
		w += stringWidth(span.Text)
	}
	return w
}

// spanStyle composes a span's group foreground onto base. A span with
// no group, an unknown group, or a group with a default foreground
// keeps base unchanged.
func spanStyle(span StyledSpan, base tcell.Style) tcell.Style {
	if span.Group == "" {
		return base
	}
	fg := config.GetColor(span.Group).GetForeground()
	if fg == tcell.ColorDefault {
		return base
	}
	return base.Foreground(fg)
}

// drawStyledLine paints line starting at (x, y), truncated to maxW
// cells, with base as the background/default style.
func drawStyledLine(line StyledLine, x, y, maxW int, base tcell.Style) {
	used := 0
	for _, span := range line {
		st := spanStyle(span, base)
		for _, ch := range span.Text {
			chW := runewidth.RuneWidth(ch)
			if used+chW > maxW {
				return
			}
			screen.SetContent(x+used, y, ch, nil, st)
			used += chW
		}
	}
}

// wrapStyledLines soft-wraps every line to at most width cells,
// preserving span boundaries and groups across the wrap.
func wrapStyledLines(lines []StyledLine, width int) []StyledLine {
	var out []StyledLine
	for _, line := range lines {
		out = append(out, wrapStyledLine(line, width)...)
	}
	return out
}

// wrapStyledLine is the single-line worker for wrapStyledLines: a
// rune-width-aware hard wrap with no word-boundary preference (same
// policy as the plain-text popup wrap it replaces: arbitrary payloads
// must never lose content). width < 1 disables wrapping.
func wrapStyledLine(line StyledLine, width int) []StyledLine {
	if width < 1 {
		return []StyledLine{line}
	}
	var out []StyledLine
	var current StyledLine
	currentW := 0
	for _, span := range line {
		pending := ""
		flush := func() {
			if pending != "" {
				current = append(current, StyledSpan{Text: pending, Group: span.Group})
				pending = ""
			}
		}
		for _, ch := range span.Text {
			chW := runewidth.RuneWidth(ch)
			if currentW+chW > width && currentW > 0 {
				flush()
				out = append(out, current)
				current = nil
				currentW = 0
			}
			pending += string(ch)
			currentW += chW
		}
		flush()
	}
	return append(out, current)
}
