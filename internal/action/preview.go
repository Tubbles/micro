package action

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/micro-editor/micro/v2/internal/buffer"
)

// previewCache loads and caches file lines for widget Preview
// callbacks, which run on every draw. Content comes from the live
// buffer when the file is open (so unsaved edits preview correctly)
// and from disk otherwise. The cache lives for one widget's lifetime:
// create a fresh one per picker open so it never serves stale content
// across openings.
type previewCache struct {
	lines map[string][]string
}

func newPreviewCache() *previewCache {
	return &previewCache{lines: make(map[string][]string)}
}

// fileLines returns path's content as lines, or nil if the file is
// neither open nor readable.
func (pc *previewCache) fileLines(path string) []string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if cached, ok := pc.lines[abs]; ok {
		return cached
	}
	var lines []string
	if b := openBufferByPath(abs); b != nil {
		lines = make([]string, b.LinesNum())
		for i := range lines {
			lines[i] = string(b.LineBytes(i))
		}
	} else if data, readErr := os.ReadFile(abs); readErr == nil {
		lines = strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	}
	pc.lines[abs] = lines
	return lines
}

// openBufferByPath finds an open buffer whose AbsPath matches abs, or
// nil.
func openBufferByPath(abs string) *buffer.Buffer {
	for _, b := range buffer.OpenBuffers {
		if b.AbsPath == abs {
			return b
		}
	}
	return nil
}

// previewWindow slices lines into a height-line window with focusLine
// (0-based) vertically centered, clamped at the file edges, and
// prefixes each line with a 1-based line-number gutter. It returns the
// window plus the focus line's index within it, matching the widget
// Preview callback's contract.
func previewWindow(lines []string, focusLine, height int) ([]string, int) {
	if height < 1 || len(lines) == 0 {
		return nil, -1
	}
	if focusLine < 0 {
		focusLine = 0
	}
	if focusLine >= len(lines) {
		focusLine = len(lines) - 1
	}
	start := focusLine - height/2
	if start > len(lines)-height {
		start = len(lines) - height
	}
	if start < 0 {
		start = 0
	}
	end := start + height
	if end > len(lines) {
		end = len(lines)
	}

	gutterWidth := len(strconv.Itoa(len(lines)))
	out := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		out = append(out, fmt.Sprintf("%*d│%s", gutterWidth, i+1, lines[i]))
	}
	return out, focusLine - start
}
