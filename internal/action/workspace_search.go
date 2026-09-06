package action

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/widget"
)

const (
	// workspaceSearchMaxFileSize caps how large a file is loaded into
	// the search index; larger ones are skipped (binary blobs,
	// minified assets, logs).
	workspaceSearchMaxFileSize = 1 << 20 // 1 MiB
	// workspaceSearchMaxHits caps the result rows per query so a
	// short common needle cannot produce a hundred-thousand-row list.
	workspaceSearchMaxHits = 500
	// workspaceSearchMinQuery is the minimum query rune count before
	// searching starts; shorter needles match nearly every line.
	workspaceSearchMinQuery = 2
)

// workspaceSearchFile is one indexed file: its root-relative path for
// row labels, absolute path for jumps, and content split into lines.
type workspaceSearchFile struct {
	rel   string
	abs   string
	lines []string
}

// workspaceSearchHit is one match: a (file, line, column) triple into
// the index. col is in runes, matching cursor Loc.X.
type workspaceSearchHit struct {
	fileIndex int
	line      int
	col       int
}

// loadWorkspaceSearchIndex walks root with the same ignore rules as
// the open-file picker and loads every candidate file's lines into
// memory, so per-keystroke searches never touch the disk. Open
// buffers contribute their live (possibly unsaved) content. Files
// that are too large or contain a NUL byte (binary) are skipped.
func loadWorkspaceSearchIndex(root string) []workspaceSearchFile {
	filter := NewIgnoreFilter(root, getShowHidden(), getShowIgnored())
	items, err := walkFiles(root, filter)
	if err != nil {
		InfoBar.Error(err)
		return nil
	}
	files := make([]workspaceSearchFile, 0, len(items))
	for _, item := range items {
		abs := filepath.Join(root, filepath.FromSlash(item.Label))
		lines, ok := loadSearchLines(abs)
		if !ok {
			continue
		}
		files = append(files, workspaceSearchFile{rel: item.Label, abs: abs, lines: lines})
	}
	return files
}

// loadSearchLines returns abs's content as lines: the live buffer's
// when the file is open, the on-disk content otherwise. ok is false
// for files that should not be searched (unreadable, too large, or
// binary).
func loadSearchLines(abs string) (lines []string, ok bool) {
	if b := openBufferByPath(abs); b != nil {
		lines = make([]string, b.LinesNum())
		for i := range lines {
			lines[i] = string(b.LineBytes(i))
		}
		return lines, true
	}
	info, err := os.Stat(abs)
	if err != nil || info.Size() > workspaceSearchMaxFileSize {
		return nil, false
	}
	data, err := os.ReadFile(abs)
	if err != nil || bytes.IndexByte(data, 0) >= 0 {
		return nil, false
	}
	return strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n"), true
}

// searchWorkspace scans the index for a literal needle, smart-case:
// an all-lowercase query matches case-insensitively, any uppercase
// rune makes it exact. One hit per matching line (the first
// occurrence), capped at workspaceSearchMaxHits.
func searchWorkspace(files []workspaceSearchFile, query string) []workspaceSearchHit {
	if utf8.RuneCountInString(query) < workspaceSearchMinQuery {
		return nil
	}
	caseSensitive := strings.ToLower(query) != query
	needle := query
	if !caseSensitive {
		needle = strings.ToLower(query)
	}

	var hits []workspaceSearchHit
	for fileIndex := range files {
		for lineIndex, line := range files[fileIndex].lines {
			hay := line
			if !caseSensitive {
				hay = strings.ToLower(line)
			}
			byteCol := strings.Index(hay, needle)
			if byteCol < 0 {
				continue
			}
			hits = append(hits, workspaceSearchHit{
				fileIndex: fileIndex,
				line:      lineIndex,
				col:       utf8.RuneCountInString(hay[:byteCol]),
			})
			if len(hits) >= workspaceSearchMaxHits {
				return hits
			}
		}
	}
	return hits
}

// workspaceSearchItems converts hits into picker rows:
// "path:line:col  <line text>", indexes aligned with hits.
func workspaceSearchItems(files []workspaceSearchFile, hits []workspaceSearchHit) []widget.PickerItem {
	items := make([]widget.PickerItem, len(hits))
	for i, hit := range hits {
		file := files[hit.fileIndex]
		text := strings.TrimSpace(file.lines[hit.line])
		items[i] = widget.PickerItem{
			Label: fmt.Sprintf("%s:%d:%d  %s", file.rel, hit.line+1, hit.col+1, text),
		}
	}
	return items
}

// WorkspaceSearch opens a live text search over every (non-ignored)
// file under the working directory, which is the opened workspace
// directory when one is active. Typing in the query row re-runs the
// search; the lower half of the widget previews the highlighted hit
// with its line centered; Enter jumps to the hit.
func (h *BufPane) WorkspaceSearch() bool {
	root, err := os.Getwd()
	if err != nil {
		InfoBar.Error(err)
		return true
	}
	files := loadWorkspaceSearchIndex(root)
	previews := newPreviewCache()

	var hits []workspaceSearchHit
	var picker *widget.Picker
	picker = widget.NewPicker(widget.PickerOptions{
		Title: "Search " + root,
		Hint:  "<type> search | <Up>/<Down> move | <Enter> jump | <Esc> cancel",
		Query: true,
		Geometry: widget.Geometry{
			Kind: widget.GeomScreenRect,
			Rect: widgetOverlayRect(),
		},
		OnQueryChange: func(query string) {
			hits = searchWorkspace(files, query)
			picker.RefreshItems(workspaceSearchItems(files, hits))
		},
		Preview: func(index, width, height int) ([]widget.StyledLine, int) {
			if index < 0 || index >= len(hits) {
				return nil, -1
			}
			hit := hits[index]
			return previews.window(files[hit.fileIndex].abs, hit.line, height)
		},
		OnSelect: func(index int) {
			widget.CloseActive()
			if index < 0 || index >= len(hits) {
				return
			}
			hit := hits[index]
			target := switchOrOpenFile(h, files[hit.fileIndex].abs)
			if target == nil {
				return
			}
			loc := buffer.Loc{X: hit.col, Y: hit.line}
			target.GotoLoc(loc.Clamp(target.Buf.Start(), target.Buf.End()))
		},
		OnClose: func() {},
	})
	widget.Open(picker)
	return true
}
