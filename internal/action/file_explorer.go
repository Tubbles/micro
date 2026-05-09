package action

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/micro/v2/internal/widget"
)

// listDir returns the picker rows for absPath. Directories are
// listed first (with a trailing "/"), then files; both groups are
// sorted case-insensitively. Hidden entries are filtered when
// showHidden is false. A "../" entry is prepended unless absPath is
// a filesystem root.
//
// Symlinks-to-directories are listed as files (no os.Stat per
// entry); pressing Enter on one will fail in NewBufferFromFile
// rather than navigate. Acceptable for v1; revisit if it bites.
func listDir(absPath string, showHidden bool) ([]widget.PickerItem, error) {
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil, err
	}

	var dirs, files []os.DirEntry
	for _, e := range entries {
		if !showHidden && strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			dirs = append(dirs, e)
		} else {
			files = append(files, e)
		}
	}

	byNameCI := func(s []os.DirEntry) func(i, j int) bool {
		return func(i, j int) bool {
			return strings.ToLower(s[i].Name()) < strings.ToLower(s[j].Name())
		}
	}
	sort.Slice(dirs, byNameCI(dirs))
	sort.Slice(files, byNameCI(files))

	items := make([]widget.PickerItem, 0, len(dirs)+len(files)+1)
	if !isFilesystemRoot(absPath) {
		items = append(items, widget.PickerItem{Label: "../"})
	}
	for _, e := range dirs {
		items = append(items, widget.PickerItem{Label: e.Name() + "/"})
	}
	for _, e := range files {
		items = append(items, widget.PickerItem{Label: e.Name()})
	}
	return items, nil
}

// isFilesystemRoot returns true when p resolves to the top of its
// volume (e.g. "/" on POSIX, `C:\` on Windows).
func isFilesystemRoot(p string) bool {
	cleaned := filepath.Clean(p)
	vol := filepath.VolumeName(cleaned)
	return cleaned == vol+string(filepath.Separator)
}

// editorAreaRect returns the picker rect with a 2-cell margin on
// every side of the editor area, accounting for the tab bar and
// info bar.
func editorAreaRect() widget.ScreenRect {
	sw, sh := screen.Screen.Size()
	iOff := config.GetInfoBarOffset()
	tabBar := 0
	if Tabs != nil && len(Tabs.List) > 1 {
		tabBar = 1
	}
	const margin = 2
	x := margin
	y := tabBar + margin
	w := sw - 2*margin
	h := (sh - tabBar - iOff) - 2*margin
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return widget.ScreenRect{X: x, Y: y, W: w, H: h}
}

// indexOfLabel returns the index of the first item whose Label
// equals label, or -1 if no such item exists.
func indexOfLabel(items []widget.PickerItem, label string) int {
	for i, it := range items {
		if it.Label == label {
			return i
		}
	}
	return -1
}

// resolveQueryPath turns a user-typed query into an absolute target
// path under cur. Absolute queries are honoured as-is; relative
// queries are joined onto cur. The result is filepath.Clean'd, but a
// trailing separator from the query is preserved (callers use it as
// a hint that the user is asking for directory navigation).
func resolveQueryPath(cur, query string) string {
	var p string
	if filepath.IsAbs(query) {
		p = query
	} else {
		p = filepath.Join(cur, query)
	}
	cleaned := filepath.Clean(p)
	if strings.HasSuffix(query, string(filepath.Separator)) &&
		!strings.HasSuffix(cleaned, string(filepath.Separator)) {
		cleaned += string(filepath.Separator)
	}
	return cleaned
}

// openFileExplorer opens a file-explorer picker rooted at start.
// The invoker pane is the BufPane the user invoked the action from;
// it is used by openFileFromPicker to decide whether to reuse a
// scratch buffer or open a new tab.
//
// selectName, when non-empty, is matched literally against the row
// labels (regular files have a bare name; directories have a "/"
// suffix; "../" is always available). On match the corresponding row
// is preselected; on miss the picker opens at the top of the list.
func openFileExplorer(invoker *BufPane, start string, selectName string) {
	cur := filepath.Clean(start)
	showHidden := getShowHidden()

	items, err := listDir(cur, showHidden)
	if err != nil {
		InfoBar.Error(err)
		return
	}

	// navigate redirects the picker at newCur, swallowing read errors
	// to the info bar. Used by both the directory branch of OnSelect
	// and the directory branch of OnSubmit.
	var picker *widget.Picker
	navigate := func(newCur string) {
		newItems, err := listDir(newCur, showHidden)
		if err != nil {
			InfoBar.Error(err)
			return
		}
		cur = newCur
		items = newItems
		picker.SetTitle(cur)
		picker.SetItems(items) // also clears the query
	}

	picker = widget.NewPicker(widget.PickerOptions{
		Title:    cur,
		Items:    items,
		Hint:     "<type> filter - <Up>/<Down> move - <Enter> open - <Esc> cancel",
		Query:    true,
		Geometry: widget.Geometry{Kind: widget.GeomScreenRect, Rect: editorAreaRect()},
		OnSelect: func(idx int) {
			if idx < 0 || idx >= len(items) {
				return
			}
			label := items[idx].Label
			if strings.HasSuffix(label, "/") {
				var newCur string
				if label == "../" {
					newCur = filepath.Dir(cur)
				} else {
					newCur = filepath.Join(cur, strings.TrimSuffix(label, "/"))
				}
				navigate(newCur)
				return
			}

			target := filepath.Join(cur, label)
			widget.CloseActive()
			openFileFromPicker(invoker, target)
		},
		OnSubmit: func(query string) {
			target := resolveQueryPath(cur, query)
			// Trailing-separator query: treat as directory regardless
			// of stat result. Lets the user descend into a new
			// directory whose name they are about to mkdir.
			trailingSep := strings.HasSuffix(target, string(filepath.Separator))
			cleanedTarget := filepath.Clean(target)
			if info, err := os.Stat(cleanedTarget); err == nil && info.IsDir() {
				navigate(cleanedTarget)
				return
			}
			if trailingSep {
				navigate(cleanedTarget)
				return
			}
			widget.CloseActive()
			openFileFromPicker(invoker, cleanedTarget)
		},
		OnClose: func() {},
	})
	if selectName != "" {
		if idx := indexOfLabel(items, selectName); idx >= 0 {
			picker.SetCurrent(idx)
		}
	}
	widget.Open(picker)
}

// openFileFromPicker opens path with a precedence rule:
//  1. If any pane currently displays a buffer at path, focus that
//     pane (no new buffer load).
//  2. Else if invoker is an unused scratch buffer (no Path AND not
//     Modified), swap in the new buffer in place.
//  3. Otherwise, open in a new tab.
//
// Buffers in OpenBuffers that are not attached to any pane are
// ignored (dedup has nothing to switch to). When the same buffer is
// open in multiple panes, the first hit (top-down tab order, then
// pane order within a tab) wins.
func openFileFromPicker(invoker *BufPane, path string) {
	for ti, t := range Tabs.List {
		for pi, p := range t.Panes {
			bp, ok := p.(*BufPane)
			if !ok {
				continue
			}
			if bp.Buf.AbsPath == path {
				Tabs.SetActive(ti)
				t.SetActive(pi)
				return
			}
		}
	}

	b, err := buffer.NewBufferFromFile(path, buffer.BTDefault)
	if err != nil {
		InfoBar.Error(err)
		return
	}

	if invoker != nil && invoker.Buf.Path == "" && !invoker.Buf.Modified() {
		invoker.OpenBuffer(b)
		return
	}

	w, h := screen.Screen.Size()
	iOff := config.GetInfoBarOffset()
	Tabs.AddTab(NewTabFromBuffer(0, 0, w, h-1-iOff, b))
	Tabs.SetActive(len(Tabs.List) - 1)
}

func getShowHidden() bool {
	v, ok := config.GlobalSettings["filemanager.showhidden"].(bool)
	return ok && v
}

// FileExplorerAtCwd opens the file explorer rooted at the current
// working directory. No row is preselected.
func (h *BufPane) FileExplorerAtCwd() bool {
	cwd, err := os.Getwd()
	if err != nil {
		InfoBar.Error(err)
		return true
	}
	openFileExplorer(h, cwd, "")
	return true
}

// FileExplorerAtFile opens the file explorer rooted at the
// directory of the active buffer's file, with the buffer's filename
// preselected so the user lands on the row they came from. When the
// buffer has no associated file path, falls back silently to the
// current working directory with no preselection.
//
// Gating on Buf.Path (the user-supplied filename) rather than
// Buf.AbsPath is deliberate: NewBuffer feeds an empty path through
// filepath.Abs, which returns the cwd, so AbsPath is never empty
// even for a fresh scratch buffer. Using AbsPath here would make
// the explorer open at filepath.Dir(cwd) instead of cwd.
func (h *BufPane) FileExplorerAtFile() bool {
	if h.Buf.Path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			InfoBar.Error(err)
			return true
		}
		openFileExplorer(h, cwd, "")
		return true
	}
	openFileExplorer(h, filepath.Dir(h.Buf.AbsPath), filepath.Base(h.Buf.AbsPath))
	return true
}
