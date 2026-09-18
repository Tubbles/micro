package action

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/micro-editor/micro/v2/internal/widget"
)

// walkFiles recursively lists files under root, applying filter at
// every entry. Excluded directories are pruned (SkipDir) so their
// contents are never visited; excluded files are simply skipped.
// Symlinks are not followed.
//
// Returned items have rel-paths (root-relative, forward slashes)
// as their Label so the picker shows "src/x.go" rather than the
// full absolute path; rel-paths fuzzy-match better.
func walkFiles(root string, filter *IgnoreFilter) ([]widget.PickerItem, error) {
	var items []widget.PickerItem
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, errIn error) error {
		if errIn != nil {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if path == root {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		isDir := d.IsDir()
		if filter != nil && filter.ShouldExclude(path, isDir) {
			if isDir {
				return fs.SkipDir
			}
			return nil
		}
		if isDir {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		items = append(items, widget.PickerItem{Label: filepath.ToSlash(rel)})
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, fs.SkipAll) {
		return nil, walkErr
	}
	return items, nil
}

// buildOpenFilePickerHint produces the hint row text for the recursive
// file picker. The [x] / [ ] markers after the Ctrl-h and Ctrl-i
// legends reflect the current showHidden and showIgnored state.
func buildOpenFilePickerHint(showHidden, showIgnored bool) string {
	mark := func(on bool) string {
		if on {
			return "[x]"
		}
		return "[ ]"
	}
	return "<type> filter | <Up>/<Down> move | <Ctrl-h> show hidden " +
		mark(showHidden) + " | <Ctrl-i> show ignored " +
		mark(showIgnored) + " | <Enter> open | <Esc> cancel"
}

// openFilePicker opens a recursive file picker rooted at start.
// Hidden and ignored entries are filtered per the
// filemanager.showhidden / filemanager.showignored settings; Ctrl-h
// and Ctrl-i toggle the corresponding axis in-session.
func openFilePicker(invoker *BufPane, start string) {
	root := filepath.Clean(start)
	showHidden := getShowHidden()
	showIgnored := getShowIgnored()

	build := func() (filter *IgnoreFilter, items []widget.PickerItem, ok bool) {
		filter = NewIgnoreFilter(root, showHidden, showIgnored)
		var err error
		items, err = walkFiles(root, filter)
		if err != nil {
			InfoBar.Error(err)
			return nil, nil, false
		}
		return filter, items, true
	}

	_, items, ok := build()
	if !ok {
		return
	}

	var picker *widget.Picker
	rebuild := func() {
		_, newItems, ok := build()
		if !ok {
			return
		}
		items = newItems
		picker.RefreshItems(items)
		picker.SetHint(buildOpenFilePickerHint(showHidden, showIgnored))
	}

	picker = widget.NewPicker(widget.PickerOptions{
		Title:    root,
		Items:    items,
		Hint:     buildOpenFilePickerHint(showHidden, showIgnored),
		Query:    true,
		Geometry: widget.Geometry{Kind: widget.GeomScreenRect, Rect: widgetOverlayRect()},
		OnCtrlH: func() {
			showHidden = !showHidden
			rebuild()
		},
		OnCtrlI: func() {
			showIgnored = !showIgnored
			rebuild()
		},
		OnSelect: func(idx int) {
			if idx < 0 || idx >= len(items) {
				return
			}
			target := filepath.Join(root, filepath.FromSlash(items[idx].Label))
			widget.CloseActive()
			openFileFromPicker(invoker, target)
		},
		OnSubmit: func(query string) {
			target := resolveQueryPath(root, query)
			widget.CloseActive()
			openFileFromPicker(invoker, filepath.Clean(target))
		},
		OnClose: func() {},
	})
	widget.Open(picker)
}

// OpenFilePickerAtCwd opens the recursive file picker rooted at the
// current working directory.
func (h *BufPane) OpenFilePickerAtCwd() bool {
	cwd, err := os.Getwd()
	if err != nil {
		InfoBar.Error(err)
		return true
	}
	openFilePicker(h, cwd)
	return true
}

// OpenFilePickerAtFile opens the recursive file picker rooted at
// the directory of the active buffer's file. Falls back silently
// to the current working directory when the buffer has no
// associated file path. Gating on Buf.Path matches FileExplorerAtFile.
func (h *BufPane) OpenFilePickerAtFile() bool {
	if h.Buf.Path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			InfoBar.Error(err)
			return true
		}
		openFilePicker(h, cwd)
		return true
	}
	openFilePicker(h, filepath.Dir(h.Buf.AbsPath))
	return true
}
