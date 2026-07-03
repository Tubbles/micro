package action

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/display"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/micro/v2/internal/util"
	"github.com/micro-editor/micro/v2/internal/workspace"
)

// SaveActiveWorkspace persists the layout of the currently active
// dir-backed workspace, if any (a no-op otherwise). It is called on
// every workspace switch (see OpenDirWorkspace) and wired into every
// quit path, QuitAll, ForceQuit's exit branch, and cmd/micro's
// signal/EOF exit(), so a crash-adjacent exit does not silently lose
// the session (D-53).
func SaveActiveWorkspace() {
	if currentWorkspaceDir == "" {
		return
	}
	saveWorkspaceState(currentWorkspaceDir)
}

// relativizeOpenBufferPaths re-displays every open buffer's path
// relative to the current working directory. It is the buffer-path
// half of `> cd`'s existing behavior, factored out so opendir can
// reuse it: after opendir changes into a workspace dir, buffers
// opened by workspace-relative path resolve their Buffer.Path
// against the live cwd on Save, so keeping cwd in sync here is
// load-bearing, not cosmetic.
func relativizeOpenBufferPaths() {
	wd, _ := os.Getwd()
	for _, b := range buffer.OpenBuffers {
		if len(b.Path) > 0 {
			b.Path, _ = util.MakeRelative(b.AbsPath, wd)
			if p, _ := filepath.Abs(b.Path); !strings.Contains(p, wd) {
				b.Path = b.AbsPath
			}
		}
	}
}

// findPaneForBuffer returns a pane currently displaying b, or nil if
// none does.
func findPaneForBuffer(b *buffer.Buffer) *BufPane {
	for _, t := range Tabs.List {
		for _, p := range t.Panes {
			if bp, ok := p.(*BufPane); ok && bp.Buf == b {
				return bp
			}
		}
	}
	return nil
}

// promptCloseModifiedBuffers walks buffer.OpenBuffers, prompting to
// save each modified regular-file buffer one at a time, then calls
// done. Buffers that share a SharedBuffer (the same file open in two
// panes) are prompted once per group, not once per pane.
//
// This must be a continuation chain rather than a loop: InfoBuf's
// YNPrompt/Prompt only ever track one prompt at a time (see
// InfoBuf.Prompt in internal/info/infobuffer.go, which cancels
// whatever prompt is already active before starting a new one), so
// firing N prompts synchronously would just have each cancel the
// last. If the user cancels (Esc) any prompt in the chain, done is
// never called, mirroring the existing closePrompt/OpenCmd
// cancel-aborts-the-action behavior.
func promptCloseModifiedBuffers(done func()) {
	seen := make(map[*buffer.SharedBuffer]bool)
	var pending []*buffer.Buffer
	for _, b := range buffer.OpenBuffers {
		if b.Type != buffer.BTDefault || !b.Modified() {
			continue
		}
		if seen[b.SharedBuffer] {
			continue
		}
		seen[b.SharedBuffer] = true
		pending = append(pending, b)
	}

	var next func(i int)
	next = func(i int) {
		if i >= len(pending) {
			done()
			return
		}
		pane := findPaneForBuffer(pending[i])
		if pane == nil {
			// No live pane displays this buffer; skip it rather than
			// block the chain (should not happen for a BTDefault
			// buffer, every one of which is opened into some pane).
			next(i + 1)
			return
		}
		pane.closePrompt("Save", func() { next(i + 1) })
	}
	next(0)
}

// tabListGeometry returns the y/height every tab in a fresh TabList
// of n tabs should use, matching NewTabList's own accounting for the
// tab bar and info bar.
func tabListGeometry(numTabs, width, height int) (y, h int) {
	iOffset := config.GetInfoBarOffset()
	if tabBarVisible(numTabs) {
		return 1, height - 1 - iOffset
	}
	return 0, height - iOffset
}

// emptyTab builds a single tab containing one empty scratch buffer,
// the fallback used when a dir-backed workspace has no saved state
// yet.
func emptyTab(width, height int) *Tab {
	y, h := tabListGeometry(1, width, height)
	b := buffer.NewBufferFromString("", "", buffer.BTDefault)
	return NewTabFromBuffer(0, y, width, h, b)
}

// finishOpenDir performs the part of an opendir switch that is
// common to both the runtime command/picker and CLI startup: resolve
// and chdir into dir, and only once dir is confirmed reachable, close
// every open buffer, replay dir's saved layout (or fall back to one
// empty tab), and record dir as the most-recently-used workspace.
//
// Checking reachability before tearing anything down matters because
// the caller (OpenDirWorkspace) has already prompted for and saved
// every modified buffer by the time this runs: a stale target (a
// recent.json entry for a directory that has since been deleted or
// moved, or a plain race) must abort cleanly with the current session
// intact, not after the old buffers are already gone.
func finishOpenDir(dir string) error {
	dir = util.ResolvePath(dir)
	if err := os.Chdir(dir); err != nil {
		return err
	}

	buffer.CloseOpenBuffers()
	relativizeOpenBufferPaths()

	width, height := screen.Screen.Size()
	// loadWorkspaceState's error (a missing state file is not one; a
	// corrupted or otherwise unusable one is) is not fatal at this
	// point, since the old session is already gone: fall back to a
	// single empty tab either way, but still surface the error once
	// the fallback tab exists.
	tabs, activeTab, _, loadErr := loadWorkspaceState(config.ConfigDir, dir, width, height)
	if len(tabs) == 0 {
		tabs, activeTab = []*Tab{emptyTab(width, height)}, 0
	}

	Tabs = &TabList{
		List:      tabs,
		TabWindow: display.NewTabWindow(width, 0),
	}
	Tabs.Names = make([]string, len(tabs))
	Tabs.SetActive(activeTab)
	Tabs.Resize()
	screen.RestartCallback = Tabs.ResetMouse

	currentWorkspaceDir = dir
	applyWorkspaceConfig(dir)

	recent, err := workspace.LoadRecent(config.ConfigDir)
	if err != nil {
		recent = &workspace.Recent{}
	}
	recent.Touch(dir)
	if err := recent.Save(config.ConfigDir); err != nil {
		return err
	}
	return loadErr
}

// applyWorkspaceConfig loads dir's workspace-level config layers
// (${dir}/.ide/micro/settings.json, settings.local.json,
// bindings.local.json; D-50) and refreshes every piece of live state
// derived from them: GlobalSettings, the key-binding tree, and every
// open buffer's per-buffer settings. It runs the same
// settings/bindings refresh reloadRuntime performs for `> reload`,
// scoped to what a workspace switch needs (no plugin or colorscheme
// re-init).
//
// It must run after dir's own tabs/buffers exist (currentWorkspaceDir
// is already dir by the time finishOpenDir calls this), because the
// per-buffer refresh loop is what corrects settings on the buffers
// that loadWorkspaceState/emptyTab just built with whatever config
// was active before the switch.
//
// A read error is surfaced via TermMessage rather than aborting the
// switch, matching how a broken settings.json does not stop micro
// from starting.
func applyWorkspaceConfig(dir string) {
	if err := config.ReadWorkspaceSettings(dir); err != nil {
		screen.TermMessage(err)
	}
	if err := config.ReadWorkspaceLocalSettings(dir); err != nil {
		screen.TermMessage(err)
	}
	config.RebuildGlobalSettings()

	InitBindings()

	for _, b := range buffer.OpenBuffers {
		b.ReloadSettings(true)
	}
}

// OpenDirWorkspace switches the editor to the dir-backed workspace
// rooted at dir: it saves the currently active workspace (if any),
// prompts to save any modified buffers one at a time, then closes
// every open buffer, changes into dir, and replays dir's saved layout
// (or opens a single empty tab if dir has never been opened before).
// This is the full runtime sequence behind `> opendir` and selecting
// an entry in the WorkspacePicker.
func OpenDirWorkspace(dir string) {
	SaveActiveWorkspace()

	promptCloseModifiedBuffers(func() {
		if err := finishOpenDir(dir); err != nil {
			InfoBar.Error(err)
		}
	})
}

// OpenDirWorkspaceAtStartup opens dir as a dir-backed workspace
// during CLI startup (a bare `micro somedir/`). Unlike
// OpenDirWorkspace there is nothing open yet to save or prompt for
// (LoadInput has not run), so this only runs the shared
// chdir/close/replay/MRU sequence; buffer.CloseOpenBuffers inside
// finishOpenDir is a no-op on the still-empty buffer.OpenBuffers.
func OpenDirWorkspaceAtStartup(dir string) error {
	return finishOpenDir(dir)
}

// OpenDirCmd implements `> opendir DIR`: switch to the dir-backed
// workspace rooted at DIR (see OpenDirWorkspace).
func (h *BufPane) OpenDirCmd(args []string) {
	if len(args) == 0 {
		InfoBar.Error("No directory given")
		return
	}
	dir, err := util.ReplaceHome(args[0])
	if err != nil {
		InfoBar.Error(err)
		return
	}
	info, err := os.Stat(dir)
	if err != nil {
		InfoBar.Error(err)
		return
	}
	if !info.IsDir() {
		InfoBar.Error(dir, " is not a directory")
		return
	}
	OpenDirWorkspace(dir)
}
