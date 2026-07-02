package action

import (
	"reflect"
	"testing"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/display"
	"github.com/micro-editor/micro/v2/internal/info"
	"github.com/micro-editor/micro/v2/internal/views"
)

// ensureInfoBar lazily initializes the global InfoBar. A real
// Tab.SetActive transition (a pane actually becoming active, as
// opposed to the ti==pi no-op guard most existing jumplist tests hit)
// calls BufPane.SetActive(true), which dereferences InfoBar for
// gutter-message bookkeeping. Built by hand rather than via the
// production NewInfoBar/display.NewInfoWindow path: NewInfoWindow
// reads screen.Screen.Size(), and screen.Screen is nil in unit tests.
// NewBufPaneFromBuf is the same screen-independent constructor
// makeTestPane (jumplist_test.go) already relies on. Lazy and
// idempotent so it's safe to call from any test harness regardless of
// file-level init() ordering; by the time a test body runs, the
// lua/config setup jumplist_test.go's init() performs has already
// completed package-wide, so buffer construction here is safe.
func ensureInfoBar() {
	if InfoBar != nil {
		return
	}
	ib := info.NewBuffer()
	InfoBar = &InfoPane{
		BufPane: NewBufPaneFromBuf(ib.Buffer, nil),
		InfoBuf: ib,
	}
}

// makeTestTabs builds n tabs, each with one BufPane on its own empty
// buffer, wired up to the global Tabs (saving and returning a restore
// function). Mirrors makeTestPane (jumplist_test.go) but for multiple
// tabs, so TabList.SetActive's MRU touch can be exercised across tab
// boundaries. Each pane's creation already touches MRU (NewTabFromBuffer
// does in production; here the harness builds tabs by hand, so it calls
// MRU.Touch itself to match), in list order, tab 0 first.
func makeTestTabs(t *testing.T, n int) ([]*BufPane, func()) {
	t.Helper()
	ensureInfoBar()
	tabs := make([]*Tab, n)
	panes := make([]*BufPane, n)
	for i := 0; i < n; i++ {
		tab := &Tab{
			Node:     views.NewRoot(0, 0, 80, 24),
			UIWindow: display.NewUIWindow(views.NewRoot(0, 0, 80, 24)),
		}
		tab.release = true
		buf := buffer.NewBufferFromString("", "", buffer.BTDefault)
		bp := NewBufPaneFromBuf(buf, tab)
		bp.SetID(tab.ID())
		tab.Panes = append(tab.Panes, bp)
		tabs[i] = tab
		panes[i] = bp
	}

	prev := Tabs
	Tabs = &TabList{
		TabWindow: display.NewTabWindow(80, 0),
		List:      tabs,
	}
	return panes, func() { Tabs = prev }
}

// makeTestSplitTab builds one tab containing n BufPanes (as if split n
// times), wired up to the global Tabs. Used to exercise Tab.SetActive's
// MRU touch across panes within a single tab. Pane IDs are allocated via
// views.NewID so they are distinct, mirroring how VSplitIndex/HSplitIndex
// assign a fresh split-tree node id to each new pane.
func makeTestSplitTab(n int) ([]*BufPane, func()) {
	ensureInfoBar()
	tab := &Tab{
		Node:     views.NewRoot(0, 0, 80, 24),
		UIWindow: display.NewUIWindow(views.NewRoot(0, 0, 80, 24)),
	}
	tab.release = true
	panes := make([]*BufPane, n)
	for i := 0; i < n; i++ {
		buf := buffer.NewBufferFromString("", "", buffer.BTDefault)
		bp := NewBufPaneFromBuf(buf, tab)
		bp.SetID(views.NewID())
		tab.Panes = append(tab.Panes, bp)
		panes[i] = bp
	}

	prev := Tabs
	Tabs = &TabList{
		TabWindow: display.NewTabWindow(80, 0),
		List:      []*Tab{tab},
	}
	return panes, func() { Tabs = prev }
}

func idsOf(panes ...*BufPane) []uint64 {
	out := make([]uint64, len(panes))
	for i, p := range panes {
		out[i] = p.ID()
	}
	return out
}

func TestMRU_TouchMovesToFront(t *testing.T) {
	m := &MRUList{}
	m.Touch(1)
	m.Touch(2)
	m.Touch(3)
	m.Touch(1) // re-touch an existing id moves it back to front

	got := m.List(alwaysAlive)
	want := []uint64{1, 3, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List() = %v, want %v", got, want)
	}
}

func TestMRU_TabListSetActiveOrdersMostRecentFirst(t *testing.T) {
	panes, restore := makeTestTabs(t, 3)
	defer restore()
	prevMRU := MRU
	MRU = &MRUList{}
	defer func() { MRU = prevMRU }()

	// Synthetic sequence of tab switches: 0 -> 1 -> 2 -> 0.
	Tabs.SetActive(0)
	Tabs.SetActive(1)
	Tabs.SetActive(2)
	Tabs.SetActive(0)

	got := MRU.List(alwaysAlive)
	want := idsOf(panes[0], panes[2], panes[1])
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MRU order = %v, want %v (most-recent first)", got, want)
	}
}

func TestMRU_TabSetActiveOrdersMostRecentFirst(t *testing.T) {
	panes, restore := makeTestSplitTab(3)
	defer restore()
	prevMRU := MRU
	MRU = &MRUList{}
	defer func() { MRU = prevMRU }()

	tab := Tabs.List[0]
	// Synthetic sequence of split switches: 0 -> 2 -> 1.
	tab.SetActive(0)
	tab.SetActive(2)
	tab.SetActive(1)

	got := MRU.List(alwaysAlive)
	want := idsOf(panes[1], panes[2], panes[0])
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MRU order = %v, want %v (most-recent first)", got, want)
	}
}

func TestMRU_PrunesDeadPaneOnRead(t *testing.T) {
	panes, restore := makeTestTabs(t, 3)
	defer restore()
	prevMRU := MRU
	MRU = &MRUList{}
	defer func() { MRU = prevMRU }()

	Tabs.SetActive(0)
	Tabs.SetActive(1)
	Tabs.SetActive(2)

	// Close tab 1 (simulate the pane going away): drop it from Tabs.List
	// without ever touching MRU, so the dangling id can only be removed
	// by List's pruning.
	deadID := panes[1].ID()
	Tabs.List = append(Tabs.List[:1], Tabs.List[2:]...)

	got := MRU.List(paneAlive)
	for _, id := range got {
		if id == deadID {
			t.Fatalf("dead pane %d not pruned: %v", deadID, got)
		}
	}
	want := idsOf(panes[2], panes[0])
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MRU order after prune = %v, want %v", got, want)
	}
}
