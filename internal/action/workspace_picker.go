package action

import (
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/widget"
	"github.com/micro-editor/micro/v2/internal/workspace"
)

// WorkspacesCmd implements `> workspaces`: open the MRU workspace
// picker (see WorkspacePicker).
func (h *BufPane) WorkspacesCmd(args []string) {
	h.WorkspacePicker()
}

// WorkspacePicker opens a picker listing recently opened dir-backed
// workspaces, most-recent-first (recent.json). Selecting an entry
// switches to it via OpenDirWorkspace. Not bound by default; bind it
// in bindings.json or use `> workspaces`.
func (h *BufPane) WorkspacePicker() bool {
	recent, err := workspace.LoadRecent(config.ConfigDir)
	if err != nil {
		InfoBar.Error(err)
		return true
	}
	if len(recent.Dirs) == 0 {
		InfoBar.Message("No recent workspaces")
		return true
	}

	items := make([]widget.PickerItem, len(recent.Dirs))
	for i, d := range recent.Dirs {
		items[i] = widget.PickerItem{Label: d}
	}

	picker := widget.NewPicker(widget.PickerOptions{
		Title:    "Workspaces",
		Items:    items,
		Hint:     "<type> filter | <Up>/<Down> move | <Enter> open | <Esc> cancel",
		Query:    true,
		Geometry: widget.Geometry{Kind: widget.GeomScreenRect, Rect: widgetOverlayRect()},
		OnSelect: func(idx int) {
			if idx < 0 || idx >= len(recent.Dirs) {
				return
			}
			dir := recent.Dirs[idx]
			widget.CloseActive()
			OpenDirWorkspace(dir)
		},
		OnClose: func() {},
	})
	widget.Open(picker)
	return true
}
