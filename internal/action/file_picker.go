package action

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/micro-editor/micro/v2/internal/widget"
)

// openFilePickerMax caps the number of items the recursive walker
// collects before stopping. The picker fuzzy-matches against this
// already-narrowed list, so a typed query operates on the first
// 100 entries in walk order rather than across an unbounded tree.
// File-scoped const for v1; promote to a setting if anyone wants
// to tune.
const openFilePickerMax = 100

// walkFiles recursively lists files under root, applying filter at
// every entry and stopping after max files have been collected.
// Excluded directories are pruned (SkipDir) so their contents are
// never visited; excluded files are simply skipped. Symlinks are
// not followed.
//
// Returned items have rel-paths (root-relative, forward slashes)
// as their Label so the picker shows "src/x.go" rather than the
// full absolute path; rel-paths fuzzy-match better.
//
// truncated reports whether the cap stopped the walk before the
// tree was exhausted — used to badge the picker title.
func walkFiles(root string, filter *IgnoreFilter, max int) (items []widget.PickerItem, truncated bool, err error) {
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
		if len(items) >= max {
			truncated = true
			return fs.SkipAll
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		items = append(items, widget.PickerItem{Label: filepath.ToSlash(rel)})
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, fs.SkipAll) {
		return nil, false, walkErr
	}
	return items, truncated, nil
}

// openFilePicker opens a recursive file picker rooted at start.
// Hidden and ignored entries are filtered per the
// filemanager.showhidden / filemanager.showignored settings; Ctrl-h
// and Ctrl-i toggle the corresponding axis in-session.
func openFilePicker(invoker *BufPane, start string) {
	root := filepath.Clean(start)
	showHidden := getShowHidden()
	showIgnored := getShowIgnored()

	build := func() (filter *IgnoreFilter, items []widget.PickerItem, title string, ok bool) {
		filter = NewIgnoreFilter(root, showHidden, showIgnored)
		var truncated bool
		var err error
		items, truncated, err = walkFiles(root, filter, openFilePickerMax)
		if err != nil {
			InfoBar.Error(err)
			return nil, nil, "", false
		}
		title = root
		if truncated {
			title += "  (truncated to " + strconv.Itoa(openFilePickerMax) + ")"
		}
		return filter, items, title, true
	}

	_, items, title, ok := build()
	if !ok {
		return
	}

	var picker *widget.Picker
	rebuild := func() {
		_, newItems, newTitle, ok := build()
		if !ok {
			return
		}
		items = newItems
		picker.SetTitle(newTitle)
		picker.RefreshItems(items)
	}

	picker = widget.NewPicker(widget.PickerOptions{
		Title:    title,
		Items:    items,
		Hint:     "<type> filter - <Up>/<Down> move - <Ctrl-h> hidden - <Ctrl-i> ignored - <Enter> open - <Esc> cancel",
		Query:    true,
		Geometry: widget.Geometry{Kind: widget.GeomScreenRect, Rect: editorAreaRect()},
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
