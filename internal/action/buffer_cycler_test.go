package action

import (
	"strings"
	"testing"

	"github.com/Tubbles/tcell/v3"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/widget"
)

// Opening the real picker (openBufferCycler) needs screen.Screen for
// widgetOverlayRect, which is nil in these unit tests, so these tests
// exercise the screen-independent pieces it's built from instead:
// buildCyclerItems, cyclerBoundKeyNames, newCyclerPicker, and
// commitBufferCycler.

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

func TestNewCyclerPicker_PreselectsIndexOne(t *testing.T) {
	items := []cyclerItem{{id: 1, label: "a"}, {id: 2, label: "b"}, {id: 3, label: "c"}}
	p := newCyclerPicker(items, nil, nil, widget.ScreenRect{X: 0, Y: 0, W: 40, H: 10}, func(int) {})
	if p.Current() != 1 {
		t.Fatalf("Current() = %d, want 1 (preselect the previous buffer)", p.Current())
	}
}

func TestNewCyclerPicker_PreselectClampsWithFewerThanTwoItems(t *testing.T) {
	items := []cyclerItem{{id: 1, label: "a"}}
	p := newCyclerPicker(items, nil, nil, widget.ScreenRect{X: 0, Y: 0, W: 40, H: 10}, func(int) {})
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
	forward := []string{keyEvent(fwdKey).Name()}
	backward := []string{keyEvent(bwdKey).Name()}

	p := newCyclerPicker(items, forward, backward, widget.ScreenRect{X: 0, Y: 0, W: 40, H: 10}, func(int) {})

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
}

func TestNewCyclerPicker_OnSelectReceivesItemsIndex(t *testing.T) {
	items := []cyclerItem{{id: 10, label: "a"}, {id: 20, label: "b"}}
	var gotIndex int
	var calls int
	p := newCyclerPicker(items, nil, nil, widget.ScreenRect{X: 0, Y: 0, W: 40, H: 10},
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
