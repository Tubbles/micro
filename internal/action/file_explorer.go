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

// openFileExplorer opens a file-explorer picker rooted at start.
// The invoker pane is the BufPane the user invoked the action from;
// it is used by openFileFromPicker to decide whether to reuse a
// scratch buffer or open a new tab.
func openFileExplorer(invoker *BufPane, start string) {
	cur := filepath.Clean(start)
	showHidden := getShowHidden()

	items, err := listDir(cur, showHidden)
	if err != nil {
		InfoBar.Error(err)
		return
	}

	var picker *widget.Picker
	picker = widget.NewPicker(widget.PickerOptions{
		Title:    cur,
		Items:    items,
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
				newItems, err := listDir(newCur, showHidden)
				if err != nil {
					InfoBar.Error(err)
					return
				}
				cur = newCur
				items = newItems
				picker.SetTitle(cur)
				picker.SetItems(items)
				return
			}

			target := filepath.Join(cur, label)
			widget.CloseActive()
			openFileFromPicker(invoker, target)
		},
		OnClose: func() {},
	})
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
// working directory.
func (h *BufPane) FileExplorerAtCwd() bool {
	cwd, err := os.Getwd()
	if err != nil {
		InfoBar.Error(err)
		return true
	}
	openFileExplorer(h, cwd)
	return true
}

// FileExplorerAtFile opens the file explorer rooted at the
// directory of the active buffer's file. When the buffer has no
// associated file path, falls back silently to the current working
// directory; the picker title reflects the resolved directory.
//
// Gating on Buf.Path (the user-supplied filename) rather than
// Buf.AbsPath is deliberate: NewBuffer feeds an empty path through
// filepath.Abs, which returns the cwd, so AbsPath is never empty
// even for a fresh scratch buffer. Using AbsPath here would make
// the explorer open at filepath.Dir(cwd) instead of cwd.
func (h *BufPane) FileExplorerAtFile() bool {
	var start string
	if h.Buf.Path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			InfoBar.Error(err)
			return true
		}
		start = cwd
	} else {
		start = filepath.Dir(h.Buf.AbsPath)
	}
	openFileExplorer(h, start)
	return true
}
