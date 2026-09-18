package action

import (
	"os"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/display"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/micro/v2/internal/util"
	"github.com/micro-editor/micro/v2/internal/workspace"
)

// SaveScratchWorkspace captures the live Tabs into the persistent
// scratch workspace's state and persists it to scratch.json (D-55).
// Unlike saveWorkspaceState it captures each scratch (unnamed,
// unsaved) leaf's buffer content inline (encodeTab's
// captureContent=true), since such a leaf has no file on disk to
// reload from on restore.
//
// This is the unconditional primitive: it always writes, regardless
// of which instance is allowed to persist. SaveActiveScratchWorkspace
// is the gated entry point SaveActiveWorkspace actually calls; this
// one stays exported separately so it can be exercised directly (its
// own behavior does not depend on the single-instance lock).
func SaveScratchWorkspace() error {
	state := &workspace.State{
		Version: workspace.CurrentVersion,
	}
	if wd, err := os.Getwd(); err == nil {
		state.Cwd = wd
	}

	for _, t := range Tabs.List {
		ts := encodeTab(t, "", true)
		if ts == nil {
			continue
		}
		state.Tabs = append(state.Tabs, *ts)
	}
	if len(state.Tabs) == 0 {
		// Every tab collapsed to nothing worth saving: keep the state
		// loadable by saving a single empty scratch tab rather than
		// zero tabs (mirrors saveWorkspaceState's own fallback).
		state.Tabs = []workspace.TabState{{Layout: &workspace.Node{Kind: "leaf", Cursor: &workspace.CursorLoc{}}}}
	}
	state.ActiveTab = util.Clamp(Tabs.Active(), 0, len(state.Tabs)-1)

	return workspace.SaveScratch(config.ConfigDir, state)
}

// loadScratchState decodes the persistent scratch workspace's saved
// state, if any, and replays it into a fresh []*Tab sized to width x
// height. The bool result reports whether scratch.json existed; when
// false the caller should fall back to today's LoadInput behavior
// (a genuinely first-ever run).
func loadScratchState(configDir string, width, height int) (tabs []*Tab, activeTab int, existed bool, err error) {
	state, existed, err := workspace.LoadScratch(configDir)
	if err != nil || !existed {
		return nil, 0, existed, err
	}
	tabs, activeTab, err = buildTabsFromState(state, "", width, height)
	return tabs, activeTab, true, err
}

// RestoreScratchWorkspaceAtStartup rebuilds the previous scratch
// workspace's session from scratch.json, if one was saved (D-55). It
// runs once, in place of LoadInput/InitTabs, when micro starts with
// no dir/file/stdin argument, the only shape of invocation D-55
// restores into.
//
// restored is false only when scratch.json does not exist at all (a
// first-ever run): the caller should fall back to LoadInput+InitTabs
// in that case. A scratch.json that exists but fails to decode or
// replay still returns restored=true, falling back to a single empty
// tab instead and surfacing the error, the same tolerance
// finishOpenDir gives a corrupted dir-backed state.
func RestoreScratchWorkspaceAtStartup() (restored bool, err error) {
	width, height := screen.Screen.Size()

	tabs, activeTab, existed, loadErr := loadScratchState(config.ConfigDir, width, height)
	if !existed {
		return false, nil
	}
	if len(tabs) == 0 {
		tabs, activeTab = []*Tab{emptyTab(width, height)}, 0
	}

	buffer.CloseOpenBuffers()

	Tabs = &TabList{
		List:      tabs,
		TabWindow: display.NewTabWindow(width, 0),
	}
	Tabs.Names = make([]string, len(tabs))
	Tabs.SetActive(activeTab)
	Tabs.Resize()
	screen.RestartCallback = Tabs.ResetMouse

	return true, loadErr
}
