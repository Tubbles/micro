package main

import (
	"testing"

	"github.com/micro-editor/micro/v2/internal/action"
	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/display"
	"github.com/micro-editor/micro/v2/internal/screen"
)

// withScratchConfigDir points config.ConfigDir at a fresh temp dir for
// the duration of the test, isolating scratch.json/scratch.lock from
// the shared test binary's real ConfigDir (which the dir-backed
// workspace tests in workspaces_test.go rely on staying stable across
// tests, since they read each other's recent.json entries).
func withScratchConfigDir(t *testing.T) string {
	t.Helper()
	old := config.ConfigDir
	dir := t.TempDir()
	config.ConfigDir = dir
	t.Cleanup(func() { config.ConfigDir = old })
	return dir
}

// resetTabsAfterScratchTest closes every open buffer and replaces
// Tabs with a single fresh, unmodified empty tab. Registered via
// t.Cleanup at the start of every test in this file: without it, a
// scratch test's leftover tabs (or, worse, a deliberately-unsaved
// modified buffer from the ephemeral test) would trip
// promptCloseModifiedBuffers's save prompt the next time a later test
// in the shared binary runs `> opendir`, and that prompt chain never
// resolves on its own inside a plain runCmd call.
func resetTabsAfterScratchTest(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		buffer.CloseOpenBuffers()
		width, height := screen.Screen.Size()
		tab := action.NewTabFromBuffer(0, 0, width, height, buffer.NewBufferFromString("", "", buffer.BTDefault))
		action.Tabs = &action.TabList{
			List:      []*action.Tab{tab},
			TabWindow: display.NewTabWindow(width, 0),
		}
		action.Tabs.Names = make([]string, 1)
		action.Tabs.SetActive(0)
	})
}

// TestScratchWorkspaceSaveAndRestoreCapturesContent proves the core of
// D-55's schema extension end to end: an unnamed, unsaved buffer's
// live text survives a save to scratch.json and a restore back into
// Tabs, since such a buffer has no file on disk to reload from.
func TestScratchWorkspaceSaveAndRestoreCapturesContent(t *testing.T) {
	withScratchConfigDir(t)
	resetTabsAfterScratchTest(t)

	runCmd("tab")
	injectString("hello scratch")

	if err := action.SaveScratchWorkspace(); err != nil {
		t.Fatalf("SaveScratchWorkspace: %v", err)
	}

	restored, err := action.RestoreScratchWorkspaceAtStartup()
	if err != nil {
		t.Fatalf("RestoreScratchWorkspaceAtStartup: %v", err)
	}
	if !restored {
		t.Fatal("expected a saved scratch state to be restored")
	}

	found := false
	for _, tab := range action.Tabs.List {
		for _, p := range tab.Panes {
			if bp, ok := p.(*action.BufPane); ok && string(bp.Buf.Bytes()) == "hello scratch" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("expected the restored scratch buffer to contain the saved content")
	}
}
