package action

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/widget"
	"github.com/micro-editor/micro/v2/pkg/highlight"
)

// previewFile is one cached file in a previewCache: its content as
// lines plus the per-line syntax matches (nil when no definition
// matched the filename; the preview then renders plain).
type previewFile struct {
	lines   []string
	matches []highlight.LineMatch
}

// previewCache loads, highlights, and caches files for widget Preview
// callbacks, which run on every draw. Content comes from the live
// buffer when the file is open (so unsaved edits preview correctly)
// and from disk otherwise. The cache lives for one widget's lifetime:
// create a fresh one per picker open so it never serves stale content
// across openings.
type previewCache struct {
	files map[string]*previewFile
}

func newPreviewCache() *previewCache {
	return &previewCache{files: make(map[string]*previewFile)}
}

// window returns a height-line styled window of path's content with
// focusLine (0-based) vertically centered, plus the focus line's
// index within the window; (nil, -1) when the file is neither open
// nor readable.
func (pc *previewCache) window(path string, focusLine, height int) ([]widget.StyledLine, int) {
	file := pc.load(path)
	return previewWindow(file.lines, file.matches, focusLine, height)
}

func (pc *previewCache) load(path string) *previewFile {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if cached, ok := pc.files[abs]; ok {
		return cached
	}
	file := &previewFile{lines: readFileLines(abs)}
	if file.lines != nil {
		file.matches = highlightLines(file.lines, syntaxDefForFile(abs))
	}
	pc.files[abs] = file
	return file
}

// readFileLines returns abs's content as lines: the live buffer's
// when the file is open, the on-disk content otherwise, or nil when
// unreadable.
func readFileLines(abs string) []string {
	if b := openBufferByPath(abs); b != nil {
		lines := make([]string, b.LinesNum())
		for i := range lines {
			lines[i] = string(b.LineBytes(i))
		}
		return lines
	}
	if data, err := os.ReadFile(abs); err == nil {
		return strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	}
	return nil
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
// prefixes each line with a 1-based line-number gutter (colorscheme
// group "line-number", matching the buffer display's gutter). matches
// may be nil (plain rendering) or index-aligned with lines. It
// returns the window plus the focus line's index within it, matching
// the widget Preview callback's contract.
func previewWindow(lines []string, matches []highlight.LineMatch, focusLine, height int) ([]widget.StyledLine, int) {
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
	out := make([]widget.StyledLine, 0, end-start)
	for i := start; i < end; i++ {
		line := widget.StyledLine{
			{Text: fmt.Sprintf("%*d│", gutterWidth, i+1), Group: "line-number"},
		}
		var match highlight.LineMatch
		if i < len(matches) {
			match = matches[i]
		}
		line = append(line, styledSpansForLine(lines[i], match)...)
		out = append(out, line)
	}
	return out, focusLine - start
}
