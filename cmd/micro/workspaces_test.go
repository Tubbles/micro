package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tubbles/tcell/v3"
	"github.com/micro-editor/micro/v2/internal/action"
	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/util"
	"github.com/micro-editor/micro/v2/internal/workspace"
)

// runCmd types a command at the `>` command bar and presses enter,
// mirroring how a user would run `> opendir some/dir`.
func runCmd(cmd string) {
	injectKey(tcell.KeyCtrlE, rune(tcell.KeyCtrlE), tcell.ModCtrl)
	injectString(cmd)
	injectKey(tcell.KeyEnter, rune(tcell.KeyEnter), tcell.ModNone)
}

// workspaceTestStartCwd is captured once, before any test in this
// file has a chance to chdir anywhere. Tests that opendir into a
// t.TempDir() must chdir back to a directory that will still exist
// once that TempDir is removed at test cleanup: os.Getwd() fails
// outright (even for an unrelated, later os.Chdir target) once the
// process's actual working directory has been unlinked from under
// it, and t.TempDir()'s own cleanup runs before the next test's body.
var workspaceTestStartCwd, _ = os.Getwd()

// chdirBackOnCleanup registers a cleanup that returns the process to
// workspaceTestStartCwd, so a test that opendirs into one or more
// t.TempDir() directories never leaves cwd inside a directory that
// is about to be deleted.
func chdirBackOnCleanup(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		if err := os.Chdir(workspaceTestStartCwd); err != nil {
			t.Fatalf("failed to restore cwd after test: %v", err)
		}
	})
}

func writeWorkspaceFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestWorkspaceDirArg exercises the CLI arg branching from D-52:
// a single directory argument routes to workspace-open, a directory
// mixed with file arguments is a usage error, and plain file/no-arg
// invocations (LoadInput's territory) are left alone.
func TestWorkspaceDirArg(t *testing.T) {
	dir := t.TempDir()
	file := writeWorkspaceFile(t, dir, "a.txt", "hi\n")

	cases := []struct {
		name    string
		args    []string
		wantDir string
		wantErr bool
	}{
		{"no args", nil, "", false},
		{"single file", []string{file}, "", false},
		{"single dir", []string{dir}, dir, false},
		{"dir plus file", []string{dir, file}, "", true},
		{"two dirs", []string{dir, dir}, "", true},
		{"dir plus position flag", []string{dir, "+3"}, dir, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := workspaceDirArg(c.args)
			if c.wantErr {
				if err == nil {
					t.Fatalf("workspaceDirArg(%v): expected an error, got dir=%q", c.args, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("workspaceDirArg(%v): unexpected error: %v", c.args, err)
			}
			if got != c.wantDir {
				t.Fatalf("workspaceDirArg(%v) = %q, want %q", c.args, got, c.wantDir)
			}
		})
	}
}

// TestOpendirSaveReplayAndMRU is the end-to-end open/save/replay
// cycle: build a nested-split layout with cursors in one dir-backed
// workspace, switch away (forcing a save), switch back (forcing a
// replay), and check the layout, file paths and cursor positions
// all survived, plus that recent.json tracks both directories
// most-recent-first.
func TestOpendirSaveReplayAndMRU(t *testing.T) {
	chdirBackOnCleanup(t)
	dirA := t.TempDir()
	writeWorkspaceFile(t, dirA, "a.txt", "hello a\nsecond line\n")
	writeWorkspaceFile(t, dirA, "b.txt", "hello b\n")

	runCmd("opendir " + dirA)

	resolvedA := util.ResolvePath(dirA)
	if action.Tabs == nil || len(action.Tabs.List) != 1 {
		t.Fatalf("opendir with no saved state: expected a single empty tab, got %#v", action.Tabs)
	}

	runCmd("open a.txt")
	runCmd("goto 2:4")
	paneA := action.MainTab().CurPane()
	if paneA.Buf.AbsPath != filepath.Join(resolvedA, "a.txt") {
		t.Fatalf("expected a.txt open, got %q", paneA.Buf.AbsPath)
	}
	cursorA := paneA.Buf.GetActiveCursor().Loc

	runCmd("vsplit b.txt")
	runCmd("goto 1:5")
	paneB := action.MainTab().CurPane()
	if paneB.Buf.AbsPath != filepath.Join(resolvedA, "b.txt") {
		t.Fatalf("expected b.txt open, got %q", paneB.Buf.AbsPath)
	}
	cursorB := paneB.Buf.GetActiveCursor().Loc

	if len(action.MainTab().Panes) != 2 {
		t.Fatalf("expected 2 panes after vsplit, got %d", len(action.MainTab().Panes))
	}

	dirB := t.TempDir()
	resolvedB := util.ResolvePath(dirB)
	runCmd("opendir " + dirB)

	state, existed, err := workspace.Load(config.ConfigDir, resolvedA)
	if err != nil {
		t.Fatal(err)
	}
	if !existed {
		t.Fatal("expected a saved state for dirA after switching away")
	}
	if len(state.Tabs) != 1 {
		t.Fatalf("expected 1 saved tab, got %d", len(state.Tabs))
	}
	layout := state.Tabs[0].Layout
	if layout.Kind != "vsplit" || len(layout.Children) != 2 {
		t.Fatalf("expected a 2-child vsplit layout, got %+v", layout)
	}
	gotPaths := map[string]*workspace.Node{}
	for _, c := range layout.Children {
		gotPaths[c.Path] = c
	}
	leafA, ok := gotPaths["a.txt"]
	if !ok {
		t.Fatalf("expected a.txt in saved layout, got %+v", layout.Children)
	}
	if leafA.Cursor.X != cursorA.X || leafA.Cursor.Y != cursorA.Y {
		t.Fatalf("a.txt cursor = %+v, want %+v", leafA.Cursor, cursorA)
	}
	leafB, ok := gotPaths["b.txt"]
	if !ok {
		t.Fatalf("expected b.txt in saved layout, got %+v", layout.Children)
	}
	if leafB.Cursor.X != cursorB.X || leafB.Cursor.Y != cursorB.Y {
		t.Fatalf("b.txt cursor = %+v, want %+v", leafB.Cursor, cursorB)
	}

	// Switch back: this replays dirA's saved layout.
	runCmd("opendir " + dirA)

	if len(action.Tabs.List) != 1 {
		t.Fatalf("expected 1 tab after replay, got %d", len(action.Tabs.List))
	}
	replayedTab := action.Tabs.List[0]
	if len(replayedTab.Panes) != 2 {
		t.Fatalf("expected 2 panes after replay, got %d", len(replayedTab.Panes))
	}
	byPath := map[string]*action.BufPane{}
	for _, p := range replayedTab.Panes {
		bp, ok := p.(*action.BufPane)
		if !ok {
			t.Fatalf("expected a *BufPane leaf, got %T", p)
		}
		byPath[bp.Buf.AbsPath] = bp
	}
	replayedA, ok := byPath[filepath.Join(resolvedA, "a.txt")]
	if !ok {
		t.Fatalf("a.txt not reopened after replay: %+v", byPath)
	}
	if got := replayedA.Buf.GetActiveCursor().Loc; got != cursorA {
		t.Fatalf("a.txt cursor after replay = %+v, want %+v", got, cursorA)
	}
	replayedB, ok := byPath[filepath.Join(resolvedA, "b.txt")]
	if !ok {
		t.Fatalf("b.txt not reopened after replay: %+v", byPath)
	}
	if got := replayedB.Buf.GetActiveCursor().Loc; got != cursorB {
		t.Fatalf("b.txt cursor after replay = %+v, want %+v", got, cursorB)
	}

	recent, err := workspace.LoadRecent(config.ConfigDir)
	if err != nil {
		t.Fatal(err)
	}
	// Check relative order (A touched last, so it must precede B)
	// rather than exact list equality: config.ConfigDir, and so
	// recent.json, is shared across every test in this binary (and
	// across every -count repetition of the whole suite), so other
	// tests' directories may legitimately also be present.
	indexOf := func(dir string) int {
		for i, d := range recent.Dirs {
			if d == dir {
				return i
			}
		}
		return -1
	}
	idxA, idxB := indexOf(resolvedA), indexOf(resolvedB)
	if idxA == -1 || idxB == -1 {
		t.Fatalf("recent.json = %v, want both %q and %q present", recent.Dirs, resolvedA, resolvedB)
	}
	if idxA >= idxB {
		t.Fatalf("recent.json = %v, want %q (touched last) before %q", recent.Dirs, resolvedA, resolvedB)
	}
}

// TestOpendirPromptChaining proves the modified-buffer save prompt is
// a continuation chain, not a loop: two modified buffers must be
// prompted one at a time (the second prompt only appears after the
// first is resolved), since InfoBuf.YNPrompt only ever tracks a
// single active prompt.
func TestOpendirPromptChaining(t *testing.T) {
	chdirBackOnCleanup(t)
	dirC := t.TempDir()
	writeWorkspaceFile(t, dirC, "x.txt", "x\n")
	writeWorkspaceFile(t, dirC, "y.txt", "y\n")

	runCmd("opendir " + dirC)
	runCmd("open x.txt")
	runCmd("vsplit y.txt")

	if len(action.MainTab().Panes) != 2 {
		t.Fatalf("expected 2 panes, got %d", len(action.MainTab().Panes))
	}
	paneX := action.MainTab().Panes[0].(*action.BufPane)
	paneY := action.MainTab().Panes[1].(*action.BufPane)

	paneX.Buf.Insert(buffer.Loc{X: 0, Y: 0}, "EDITED-X ")
	paneY.Buf.Insert(buffer.Loc{X: 0, Y: 0}, "EDITED-Y ")
	if !paneX.Buf.Modified() || !paneY.Buf.Modified() {
		t.Fatal("expected both buffers to be modified before switching")
	}

	dirD := t.TempDir()

	injectKey(tcell.KeyCtrlE, rune(tcell.KeyCtrlE), tcell.ModCtrl)
	injectString("opendir " + dirD)
	injectKey(tcell.KeyEnter, rune(tcell.KeyEnter), tcell.ModNone)

	if !action.InfoBar.HasPrompt || !action.InfoBar.HasYN {
		t.Fatal("expected a YN prompt for the first modified buffer")
	}

	injectString("y")
	if !action.InfoBar.HasPrompt || !action.InfoBar.HasYN {
		t.Fatal("expected a second YN prompt after resolving the first (chain, not a loop)")
	}

	injectString("y")
	if action.InfoBar.HasPrompt {
		t.Fatal("expected the prompt chain to be done after both buffers are resolved")
	}

	dataX, err := os.ReadFile(filepath.Join(dirC, "x.txt"))
	if err != nil {
		t.Fatal(err)
	}
	dataY, err := os.ReadFile(filepath.Join(dirC, "y.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(dataX) != "EDITED-X x\n" {
		t.Fatalf("x.txt on disk = %q, want the edit to be saved", dataX)
	}
	if string(dataY) != "EDITED-Y y\n" {
		t.Fatalf("y.txt on disk = %q, want the edit to be saved", dataY)
	}

	if util.ResolvePath(dirD) != resolveCwd(t) {
		t.Fatalf("expected the switch to dirD to complete once both prompts resolved")
	}
}

func resolveCwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return util.ResolvePath(wd)
}

// TestOpendirDropsNonBufferLeaves proves D-54: a tab whose only pane
// is not a *BufPane (here, the `raw` command's RawPane, standing in
// for the class of leaves with a live child process/state that isn't
// restorable, which also includes TermPane) is dropped entirely from
// the saved state, while a sibling tab with real files is kept.
func TestOpendirDropsNonBufferLeaves(t *testing.T) {
	chdirBackOnCleanup(t)
	dirE := t.TempDir()
	writeWorkspaceFile(t, dirE, "keep.txt", "keep me\n")

	runCmd("opendir " + dirE)
	runCmd("open keep.txt")
	runCmd("raw")

	if len(action.Tabs.List) != 2 {
		t.Fatalf("expected 2 tabs (keep.txt + raw), got %d", len(action.Tabs.List))
	}
	// RawPane.HandleEvent logs every event into its own buffer instead
	// of dispatching it (including Ctrl-E), so leaving it the active
	// pane would swallow every keystroke a later test injects. Switch
	// back to the normal tab before returning.
	t.Cleanup(func() { action.Tabs.SetActive(0) })

	action.SaveActiveWorkspace()

	resolvedE := util.ResolvePath(dirE)
	state, existed, err := workspace.Load(config.ConfigDir, resolvedE)
	if err != nil {
		t.Fatal(err)
	}
	if !existed {
		t.Fatal("expected a saved state")
	}
	if len(state.Tabs) != 1 {
		t.Fatalf("expected the raw-only tab to be dropped, got %d saved tabs", len(state.Tabs))
	}
	if state.Tabs[0].Layout.Path != "keep.txt" {
		t.Fatalf("expected the surviving tab to be keep.txt, got %+v", state.Tabs[0].Layout)
	}
}

// TestOpendirUnreachableDirAbortsCleanly is a regression test for a
// bug found while writing this feature: an early draft of
// finishOpenDir called buffer.CloseOpenBuffers() before confirming
// the target directory was actually reachable, so switching to a
// stale entry (e.g. a recent.json workspace whose directory was
// since deleted, which WorkspacePicker's OnSelect does not
// pre-validate) tore down the current session on the way to failing.
// The fix checks os.Chdir succeeds before closing anything.
func TestOpendirUnreachableDirAbortsCleanly(t *testing.T) {
	chdirBackOnCleanup(t)
	dirF := t.TempDir()
	writeWorkspaceFile(t, dirF, "stay.txt", "still here\n")

	runCmd("opendir " + dirF)
	runCmd("open stay.txt")

	before := len(buffer.OpenBuffers)
	beforeTabs := action.Tabs

	missing := filepath.Join(t.TempDir(), "does-not-exist")
	action.OpenDirWorkspace(missing)

	if action.Tabs != beforeTabs {
		t.Fatal("expected Tabs to be left untouched when the target dir is unreachable")
	}
	if len(buffer.OpenBuffers) != before {
		t.Fatalf("expected OpenBuffers to be untouched, had %d now has %d", before, len(buffer.OpenBuffers))
	}
	if !action.InfoBar.HasError {
		t.Fatal("expected an error message about the unreachable directory")
	}
	if got := resolveCwd(t); got != util.ResolvePath(dirF) {
		t.Fatalf("expected cwd to remain dirF, got %q", got)
	}
}
