package action

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tubbles/tcell/v3"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/util"
	"github.com/micro-editor/micro/v2/internal/widget"
)

// Opening the real picker (openBufferCycler) needs screen.Screen for
// halfScreenOverlayRect, which is nil in these unit tests, so these
// tests exercise the screen-independent pieces it's built from instead:
// buildCyclerItems, cyclerBoundKeyNames, newCyclerPicker,
// centeredHalfRect, and commitBufferCycler. SwitchToRecentBuffer needs
// no screen at all, so it is tested directly.

func TestCyclerBoundKeyNames_InvertsBindingsMap(t *testing.T) {
	prev := config.Bindings["buffer"]
	config.Bindings["buffer"] = map[string]string{
		"Ctrl-Tab":       "CycleBuffersForward",
		"Shift-Ctrl-Tab": "CycleBuffersBackward",
		"Ctrl-S":         "Save",
	}
	defer func() { config.Bindings["buffer"] = prev }()

	got := cyclerBoundKeyNames("CycleBuffersForward")
	if len(got) != 1 || got[0] != "Ctrl-Tab" {
		t.Fatalf("cyclerBoundKeyNames(forward) = %v, want [Ctrl-Tab]", got)
	}
	got = cyclerBoundKeyNames("CycleBuffersBackward")
	if len(got) != 1 || got[0] != "Shift-Ctrl-Tab" {
		t.Fatalf("cyclerBoundKeyNames(backward) = %v, want [Shift-Ctrl-Tab]", got)
	}
}

func TestBuildCyclerItems_MRUOrderAndLabels(t *testing.T) {
	panes, restoreTabs := makeTestTabs(t, 3)
	defer restoreTabs()
	prevMRU := MRU
	MRU = &MRUList{}
	defer func() { MRU = prevMRU }()

	panes[1].Buf.Insert(buffer.Loc{X: 0, Y: 0}, "x") // dirty panes[1]

	Tabs.SetActive(0)
	Tabs.SetActive(1)
	Tabs.SetActive(2)

	items := buildCyclerItems()
	if len(items) != 3 {
		t.Fatalf("items len = %d, want 3", len(items))
	}
	wantIDs := []uint64{panes[2].ID(), panes[1].ID(), panes[0].ID()}
	for i, id := range wantIDs {
		if items[i].id != id {
			t.Fatalf("items[%d].id = %d, want %d (MRU order)", i, items[i].id, id)
		}
	}
	if items[0].label != panes[2].Name() {
		t.Fatalf("items[0].label = %q, want %q (reuse BufPane.Name())", items[0].label, panes[2].Name())
	}
	// panes[1] was edited, so its tab-bar title (BufPane.Name()) carries
	// the " +" modified marker; buildCyclerItems must reuse that string
	// verbatim rather than deriving its own marker.
	if !strings.HasSuffix(items[1].label, " +") {
		t.Fatalf("items[1].label = %q, want modified-marker suffix %q", items[1].label, " +")
	}
}

func TestWorkspaceRelativePath_RelativeOnlyForFilesInsideTheWorkspace(t *testing.T) {
	// t.TempDir() can itself sit behind a symlink, so resolve it the way
	// NewBuffer resolves a buffer's AbsPath and compare like with like.
	root := util.ResolvePath(t.TempDir())
	workspaceDir := filepath.Join(root, "ws")

	// Every case gets a path of its own: NewBuffer shares one
	// SharedBuffer (Settings included) between buffers with the same
	// absolute path, so a reused path would leak the basename case's
	// setting into the other buffers.
	newPathBuffer := func(btype buffer.BufType, elements ...string) *buffer.Buffer {
		t.Helper()
		path := filepath.Join(append([]string{root}, elements...)...)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) = %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatalf("WriteFile(%q) = %v", path, err)
		}
		return buffer.NewBufferFromString("", path, btype)
	}

	nested := newPathBuffer(buffer.BTDefault, "ws", "sub", "file.go")
	outside := newPathBuffer(buffer.BTDefault, "other", "file.go")
	namePrefixed := newPathBuffer(buffer.BTDefault, "ws2", "file.go")
	withBasename := newPathBuffer(buffer.BTDefault, "ws", "basename.go")
	withBasename.Settings["basename"] = true
	help := newPathBuffer(buffer.BTHelp, "ws", "help.go")
	readonly := newPathBuffer(buffer.BTDefault, "ws", "readonly.go")
	readonly.Type.Readonly = true // what NewBuffer does for the readonly setting
	pathless := buffer.NewBufferFromString("", "", buffer.BTDefault)

	tests := []struct {
		name         string
		buf          *buffer.Buffer
		workspaceDir string
		want         string
		wantOK       bool
	}{
		{"no active workspace", nested, "", "", false},
		{"path-less buffer", pathless, workspaceDir, "", false},
		{"file inside the workspace", nested, workspaceDir, filepath.Join("sub", "file.go"), true},
		{"file outside the workspace", outside, workspaceDir, "", false},
		{"directory whose name starts with the workspace name", namePrefixed, workspaceDir, "", false},
		{"basename setting on", withBasename, workspaceDir, "", false},
		{"non-default buffer type", help, workspaceDir, "", false},
		{"readonly file inside the workspace", readonly, workspaceDir, "readonly.go", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := workspaceRelativePath(tt.buf, tt.workspaceDir)
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("workspaceRelativePath(%q, %q) = (%q, %v), want (%q, %v)",
					tt.buf.AbsPath, tt.workspaceDir, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestBuildCyclerItems_WorkspaceRelativeLabels(t *testing.T) {
	panes, restoreTabs := makeTestTabs(t, 2)
	defer restoreTabs()
	prevMRU := MRU
	MRU = &MRUList{}
	defer func() { MRU = prevMRU }()

	// withActiveWorkspace restores currentWorkspaceDir through t.Cleanup,
	// so the "no workspace" leg below can clear it in place.
	workspaceDir := withActiveWorkspace(t)
	relative := filepath.Join("sub", "file.go")
	panes[0].Buf.Path = filepath.Join(workspaceDir, relative)
	panes[0].Buf.AbsPath = panes[0].Buf.Path
	panes[0].Buf.Insert(buffer.Loc{X: 0, Y: 0}, "x") // dirty panes[0]

	Tabs.SetActive(1)
	Tabs.SetActive(0)

	items := buildCyclerItems()
	if len(items) != 2 {
		t.Fatalf("items len = %d, want 2", len(items))
	}
	want := relative + " +"
	if items[0].label != want {
		t.Fatalf("items[0].label = %q, want %q (workspace-relative path plus the modified marker)", items[0].label, want)
	}
	// A buffer with no file keeps the tab bar's name for it.
	if items[1].label != panes[1].Name() {
		t.Fatalf("items[1].label = %q, want %q (unchanged)", items[1].label, panes[1].Name())
	}

	// With no workspace open the label is BufPane.Name() again, so the
	// row shows the path the file was opened with.
	currentWorkspaceDir = ""
	items = buildCyclerItems()
	if items[0].label != panes[0].Name() {
		t.Fatalf("items[0].label = %q, want %q (no workspace, so the tab-bar name)", items[0].label, panes[0].Name())
	}
}

func TestNewCyclerPicker_PreselectsRequestedRow(t *testing.T) {
	items := []cyclerItem{{id: 1, label: "a"}, {id: 2, label: "b"}, {id: 3, label: "c"}}

	// CycleBuffersForward's row: the previous buffer.
	p := newCyclerPicker(items, nil, nil, 1, widget.ScreenRect{X: 0, Y: 0, W: 40, H: 10}, func(int) {})
	if p.Current() != 1 {
		t.Fatalf("Current() = %d, want 1 (preselect the previous buffer)", p.Current())
	}

	// CycleBuffersBackward's row: the current buffer, so the first
	// backward step wraps onto the least recently used one.
	p = newCyclerPicker(items, nil, nil, 0, widget.ScreenRect{X: 0, Y: 0, W: 40, H: 10}, func(int) {})
	if p.Current() != 0 {
		t.Fatalf("Current() = %d, want 0 (preselect the current buffer)", p.Current())
	}
}

func TestNewCyclerPicker_PreselectClampsWithFewerThanTwoItems(t *testing.T) {
	items := []cyclerItem{{id: 1, label: "a"}}
	p := newCyclerPicker(items, nil, nil, 1, widget.ScreenRect{X: 0, Y: 0, W: 40, H: 10}, func(int) {})
	if p.Current() != 0 {
		t.Fatalf("Current() = %d, want 0 (graceful clamp with a single item)", p.Current())
	}
}

func TestNewCyclerPicker_OnKeyWrapsUsingBoundNames(t *testing.T) {
	items := []cyclerItem{{id: 1, label: "a"}, {id: 2, label: "b"}, {id: 3, label: "c"}}

	// Derive the bound-name strings the same way cyclerBoundKeyNames
	// would see them (config.Bindings is keyed by KeyEvent.Name(), the
	// string BufMapEvent stores), so this test exercises the real
	// name-matching path rather than a hand-picked literal.
	fwdKey := tcell.NewEventKey(tcell.KeyTab, "", tcell.ModCtrl)
	bwdKey := tcell.NewEventKey(tcell.KeyTab, "", tcell.ModCtrl|tcell.ModShift)
	// The key bound to SwitchToRecentBuffer keeps stepping down once
	// the list is open, so openBufferCycler folds its names into the
	// forward set.
	recentKey := tcell.NewEventKey(tcell.KeyTab, "", tcell.ModAlt)
	forward := []string{keyEvent(fwdKey).Name(), keyEvent(recentKey).Name()}
	backward := []string{keyEvent(bwdKey).Name()}

	p := newCyclerPicker(items, forward, backward, 1, widget.ScreenRect{X: 0, Y: 0, W: 40, H: 10}, func(int) {})

	// Current starts at 1 (preselect). Forward wraps 1 -> 2 -> 0.
	p.HandleEvent(fwdKey)
	if p.Current() != 2 {
		t.Fatalf("after one forward: current=%d, want 2", p.Current())
	}
	p.HandleEvent(fwdKey)
	if p.Current() != 0 {
		t.Fatalf("forward past end: current=%d, want 0 (wrap)", p.Current())
	}

	p.HandleEvent(bwdKey)
	if p.Current() != 2 {
		t.Fatalf("backward past start: current=%d, want 2 (wrap)", p.Current())
	}

	p.HandleEvent(recentKey)
	if p.Current() != 0 {
		t.Fatalf("after SwitchToRecentBuffer key: current=%d, want 0 (forward, wrap)", p.Current())
	}
}

func TestNewCyclerPicker_OnSelectReceivesItemsIndex(t *testing.T) {
	items := []cyclerItem{{id: 10, label: "a"}, {id: 20, label: "b"}}
	var gotIndex int
	var calls int
	p := newCyclerPicker(items, nil, nil, 1, widget.ScreenRect{X: 0, Y: 0, W: 40, H: 10},
		func(index int) { calls++; gotIndex = index })

	p.HandleEvent(tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone))
	if calls != 1 {
		t.Fatalf("onSelect calls = %d, want 1", calls)
	}
	if gotIndex != 1 {
		t.Fatalf("onSelect index = %d, want 1 (preselected row)", gotIndex)
	}
}

func TestCommitBufferCycler_SwitchesPaneAndRecordsJump(t *testing.T) {
	panes, restoreTabs := makeTestTabs(t, 3)
	defer restoreTabs()
	prevMRU := MRU
	MRU = &MRUList{}
	defer func() { MRU = prevMRU }()
	jl := newTestList(10)
	prevJumps := Jumps
	Jumps = jl
	defer func() { Jumps = prevJumps }()

	Tabs.SetActive(0)
	origin := panes[0]
	target := panes[2]

	commitBufferCycler(target.ID(), origin.ID(), origin.Buf.SharedBuffer, origin.Cursor.Loc)

	if Tabs.Active() != 2 {
		t.Fatalf("Tabs.Active() = %d, want 2 (switched to the target pane's tab)", Tabs.Active())
	}
	got, _ := snapshot(jl)
	if len(got) != 1 || got[0][0] != int(origin.ID()) {
		t.Fatalf("Jumps entries = %v, want a single entry for origin pane %d", got, origin.ID())
	}
}

func TestCommitBufferCycler_SwitchesSplitWithinSameTab(t *testing.T) {
	// Exercises the Tabs.Active()==ti but Tabs.List[ti].active!=pi branch
	// of commitBufferCycler: origin and target are two panes (splits) in
	// the *same* tab, so only the inner Tab.SetActive call should fire.
	panes, restoreTabs := makeTestSplitTab(3)
	defer restoreTabs()
	prevMRU := MRU
	MRU = &MRUList{}
	defer func() { MRU = prevMRU }()
	jl := newTestList(10)
	prevJumps := Jumps
	Jumps = jl
	defer func() { Jumps = prevJumps }()

	tab := Tabs.List[0]
	tab.SetActive(0)
	origin := panes[0]
	target := panes[2]

	commitBufferCycler(target.ID(), origin.ID(), origin.Buf.SharedBuffer, origin.Cursor.Loc)

	if tab.active != 2 {
		t.Fatalf("tab.active = %d, want 2 (switched split within the same tab)", tab.active)
	}
	got, _ := snapshot(jl)
	if len(got) != 1 || got[0][0] != int(origin.ID()) {
		t.Fatalf("Jumps entries = %v, want a single entry for origin pane %d", got, origin.ID())
	}
}

func TestCommitBufferCycler_SelfSelectRecordsNoJump(t *testing.T) {
	panes, restoreTabs := makeTestTabs(t, 2)
	defer restoreTabs()
	prevMRU := MRU
	MRU = &MRUList{}
	defer func() { MRU = prevMRU }()
	jl := newTestList(10)
	prevJumps := Jumps
	Jumps = jl
	defer func() { Jumps = prevJumps }()

	Tabs.SetActive(0)
	origin := panes[0]

	commitBufferCycler(origin.ID(), origin.ID(), origin.Buf.SharedBuffer, origin.Cursor.Loc)

	if len(jl.entries) != 0 {
		t.Fatalf("self-select must not record a jump, got %d entries", len(jl.entries))
	}
}

func TestSwitchToRecentBuffer_SwitchesToPreviousPaneAndRecordsJump(t *testing.T) {
	panes, restoreTabs := makeTestTabs(t, 3)
	defer restoreTabs()
	prevMRU := MRU
	MRU = &MRUList{}
	defer func() { MRU = prevMRU }()

	// Leaves the MRU order as panes[0], panes[1], panes[2]. These focus
	// moves push jumps of their own, so the test jump list only goes in
	// afterwards and sees nothing but the switch under test.
	Tabs.SetActive(2)
	Tabs.SetActive(1)
	Tabs.SetActive(0)

	jl := newTestList(10)
	prevJumps := Jumps
	Jumps = jl
	defer func() { Jumps = prevJumps }()

	origin := panes[0]
	if !origin.SwitchToRecentBuffer() {
		t.Fatal("SwitchToRecentBuffer() = false, want true with three live panes")
	}
	if Tabs.Active() != 1 {
		t.Fatalf("Tabs.Active() = %d, want 1 (the most recently focused other pane)", Tabs.Active())
	}
	got, _ := snapshot(jl)
	if len(got) != 1 || got[0][0] != int(origin.ID()) {
		t.Fatalf("Jumps entries = %v, want a single entry for origin pane %d", got, origin.ID())
	}
}

func TestSwitchToRecentBuffer_SinglePaneIsNoOp(t *testing.T) {
	panes, restoreTabs := makeTestTabs(t, 1)
	defer restoreTabs()
	prevMRU := MRU
	MRU = &MRUList{}
	defer func() { MRU = prevMRU }()
	jl := newTestList(10)
	prevJumps := Jumps
	Jumps = jl
	defer func() { Jumps = prevJumps }()

	Tabs.SetActive(0)

	if panes[0].SwitchToRecentBuffer() {
		t.Fatal("SwitchToRecentBuffer() = true, want false with a single live pane")
	}
	if Tabs.Active() != 0 {
		t.Fatalf("Tabs.Active() = %d, want 0 (focus unchanged)", Tabs.Active())
	}
	if len(jl.entries) != 0 {
		t.Fatalf("a no-op switch must not record a jump, got %d entries", len(jl.entries))
	}
}

func TestCenteredHalfRect_CentersHalfSizeInEditorArea(t *testing.T) {
	// 80x24 screen with the tab bar shown and a one-row info bar: a
	// 40x12 window, horizontally centered and vertically centered in
	// the 22 rows between the two bars.
	got := centeredHalfRect(80, 24, 1, 1)
	want := widget.ScreenRect{X: 20, Y: 6, W: 40, H: 12}
	if got != want {
		t.Fatalf("centeredHalfRect(80, 24, 1, 1) = %+v, want %+v", got, want)
	}
}

func TestCenteredHalfRect_ClampsOnTinyScreens(t *testing.T) {
	// Screens with no room left after their own chrome. The picker
	// draws straight from these numbers, so an empty rect is fine but a
	// negative or out-of-screen one is not.
	for _, size := range [][4]int{{0, 0, 0, 0}, {1, 1, 1, 1}, {3, 2, 1, 1}} {
		got := centeredHalfRect(size[0], size[1], size[2], size[3])
		if got.X < 0 || got.Y < 0 || got.W < 0 || got.H < 0 {
			t.Fatalf("centeredHalfRect%v = %+v, want a non-negative rect", size, got)
		}
		if got.W > size[0] || got.H > size[1] {
			t.Fatalf("centeredHalfRect%v = %+v, want a rect within the screen", size, got)
		}
	}
}
