package widget

import (
	"testing"
	"time"

	"github.com/Tubbles/tcell/v3"
)

// pickerHarness builds a Picker with a fixed geometry and a fake
// clock so tests are deterministic.
type pickerHarness struct {
	p           *Picker
	selectCount int
	closeCount  int
	lastSelect  int
	clock       time.Time
}

func newPickerHarness(items []PickerItem) *pickerHarness {
	h := &pickerHarness{clock: time.Unix(1_700_000_000, 0)}
	h.p = NewPicker(PickerOptions{
		Title: "test",
		Items: items,
		Geometry: Geometry{
			Kind: GeomScreenRect,
			Rect: ScreenRect{X: 10, Y: 5, W: 40, H: 12},
		},
		OnSelect: func(i int) { h.selectCount++; h.lastSelect = i },
		OnClose:  func() { h.closeCount++ },
	})
	h.p.nowFn = func() time.Time { return h.clock }
	return h
}

func (h *pickerHarness) advance(d time.Duration) { h.clock = h.clock.Add(d) }

func mockScreenSize(t *testing.T) {
	t.Helper()
	old := screenSize
	screenSize = func() (int, int) { return 80, 24 }
	t.Cleanup(func() { screenSize = old })
}

func key(k tcell.Key) *tcell.EventKey {
	return tcell.NewEventKey(k, "", tcell.ModNone)
}

func mouse(x, y int, btn tcell.ButtonMask) *tcell.EventMouse {
	return tcell.NewEventMouse(x, y, btn, tcell.ModNone)
}

func TestPickerArrowsClampNoWrap(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "a"}, {Label: "b"}, {Label: "c"}}
	h := newPickerHarness(items)

	h.p.HandleEvent(key(tcell.KeyUp))
	if h.p.Current() != 0 {
		t.Fatalf("Up at top: want current=0, got %d", h.p.Current())
	}

	for i := 0; i < 5; i++ {
		h.p.HandleEvent(key(tcell.KeyDown))
	}
	if h.p.Current() != 2 {
		t.Fatalf("Down past end: want current=2, got %d", h.p.Current())
	}

	h.p.HandleEvent(key(tcell.KeyDown))
	if h.p.Current() != 2 {
		t.Fatalf("Down at end: want current=2, got %d", h.p.Current())
	}
}

func TestPickerHomeEnd(t *testing.T) {
	mockScreenSize(t)
	items := make([]PickerItem, 50)
	for i := range items {
		items[i].Label = "x"
	}
	h := newPickerHarness(items)

	h.p.HandleEvent(key(tcell.KeyEnd))
	if h.p.Current() != 49 {
		t.Fatalf("End: want current=49, got %d", h.p.Current())
	}

	h.p.HandleEvent(key(tcell.KeyHome))
	if h.p.Current() != 0 {
		t.Fatalf("Home: want current=0, got %d", h.p.Current())
	}
}

func TestPickerPageStep(t *testing.T) {
	mockScreenSize(t)
	items := make([]PickerItem, 50)
	for i := range items {
		items[i].Label = "x"
	}
	h := newPickerHarness(items)

	body := h.p.bodyHeight()
	h.p.HandleEvent(key(tcell.KeyPgDn))
	if h.p.Current() != body {
		t.Fatalf("PgDn: want current=%d, got %d", body, h.p.Current())
	}

	h.p.HandleEvent(key(tcell.KeyPgUp))
	if h.p.Current() != 0 {
		t.Fatalf("PgUp: want current=0, got %d", h.p.Current())
	}
}

func TestPickerEnterFiresOnSelect(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "a"}, {Label: "b"}}
	h := newPickerHarness(items)

	h.p.HandleEvent(key(tcell.KeyDown))
	h.p.HandleEvent(key(tcell.KeyEnter))

	if h.selectCount != 1 || h.lastSelect != 1 {
		t.Fatalf("OnSelect: count=%d lastSelect=%d", h.selectCount, h.lastSelect)
	}
}

func TestPickerEscFiresOnClose(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "a"}}
	h := newPickerHarness(items)
	Open(h.p)
	t.Cleanup(func() { CloseActive() })

	h.p.HandleEvent(key(tcell.KeyEsc))

	if h.closeCount != 1 {
		t.Fatalf("OnClose: count=%d", h.closeCount)
	}
	if Active() != nil {
		t.Fatalf("active widget should be cleared after Esc")
	}
}

func TestPickerRunesAreSwallowed(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "a"}, {Label: "b"}}
	h := newPickerHarness(items)

	consumed := h.p.HandleEvent(tcell.NewEventKey(tcell.KeyRune, "x", tcell.ModNone))
	if !consumed {
		t.Fatalf("rune events must be consumed to keep the buffer below inert")
	}
	if h.selectCount != 0 || h.closeCount != 0 || h.p.Current() != 0 {
		t.Fatalf("rune events must be a no-op for state")
	}
}

func TestPickerMouseClickMovesHighlight(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "a"}, {Label: "b"}, {Label: "c"}, {Label: "d"}}
	h := newPickerHarness(items)
	// rect is X=10,Y=5,W=40,H=12 → bodyY0=6, bodyH=10
	h.p.HandleEvent(mouse(20, 8, tcell.Button1))

	if h.p.Current() != 2 { // top=0, y=8 → idx = 0 + (8-6) = 2
		t.Fatalf("click row: want current=2, got %d", h.p.Current())
	}
	if h.selectCount != 0 {
		t.Fatalf("single click must not activate (got %d selects)", h.selectCount)
	}
}

func TestPickerDoubleClickActivates(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "a"}, {Label: "b"}, {Label: "c"}}
	h := newPickerHarness(items)

	h.p.HandleEvent(mouse(20, 7, tcell.Button1)) // row 1
	h.advance(100 * time.Millisecond)
	h.p.HandleEvent(mouse(20, 7, tcell.Button1))

	if h.selectCount != 1 || h.lastSelect != 1 {
		t.Fatalf("double-click row 1: selects=%d last=%d", h.selectCount, h.lastSelect)
	}
}

func TestPickerSecondClickOnDifferentRowDoesNotActivate(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "a"}, {Label: "b"}, {Label: "c"}}
	h := newPickerHarness(items)

	h.p.HandleEvent(mouse(20, 7, tcell.Button1)) // row 1
	h.advance(100 * time.Millisecond)
	h.p.HandleEvent(mouse(20, 8, tcell.Button1)) // row 2

	if h.selectCount != 0 {
		t.Fatalf("clicks on different rows must not activate (got %d selects)", h.selectCount)
	}
	if h.p.Current() != 2 {
		t.Fatalf("second click moves highlight: want current=2, got %d", h.p.Current())
	}
}

func TestPickerDoubleClickWindowExpires(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "a"}, {Label: "b"}}
	h := newPickerHarness(items)

	h.p.HandleEvent(mouse(20, 7, tcell.Button1))
	h.advance(doubleClickWindow + time.Millisecond)
	h.p.HandleEvent(mouse(20, 7, tcell.Button1))

	if h.selectCount != 0 {
		t.Fatalf("clicks outside the window must not activate (got %d)", h.selectCount)
	}
}

func TestPickerClickOutsideRectCloses(t *testing.T) {
	mockScreenSize(t)
	items := []PickerItem{{Label: "a"}}
	h := newPickerHarness(items)
	Open(h.p)
	t.Cleanup(func() { CloseActive() })

	h.p.HandleEvent(mouse(0, 0, tcell.Button1))

	if h.closeCount != 1 {
		t.Fatalf("OnClose count=%d after click outside", h.closeCount)
	}
}

func TestPickerWheelMovesHighlight(t *testing.T) {
	mockScreenSize(t)
	items := make([]PickerItem, 20)
	for i := range items {
		items[i].Label = "x"
	}
	h := newPickerHarness(items)

	h.p.HandleEvent(mouse(20, 8, tcell.WheelDown))
	if h.p.Current() != 1 {
		t.Fatalf("WheelDown: want current=1, got %d", h.p.Current())
	}
	h.p.HandleEvent(mouse(20, 8, tcell.WheelUp))
	if h.p.Current() != 0 {
		t.Fatalf("WheelUp: want current=0, got %d", h.p.Current())
	}
}

func TestPickerSetItemsClampsCurrent(t *testing.T) {
	mockScreenSize(t)
	h := newPickerHarness([]PickerItem{{Label: "a"}, {Label: "b"}, {Label: "c"}})
	h.p.SetCurrent(2)
	h.p.SetItems([]PickerItem{{Label: "x"}})
	if h.p.Current() != 0 {
		t.Fatalf("SetItems clamp: want current=0, got %d", h.p.Current())
	}
}

func TestPickerEnterEmptyListNoCallback(t *testing.T) {
	mockScreenSize(t)
	h := newPickerHarness(nil)
	h.p.HandleEvent(key(tcell.KeyEnter))
	if h.selectCount != 0 {
		t.Fatalf("Enter on empty list must not fire OnSelect")
	}
}

func TestResolveScreenRectClips(t *testing.T) {
	r := Resolve(Geometry{Kind: GeomScreenRect, Rect: ScreenRect{X: 70, Y: 20, W: 20, H: 10}}, 80, 24)
	if r.W != 10 || r.H != 4 {
		t.Fatalf("clip: want W=10 H=4, got W=%d H=%d", r.W, r.H)
	}
}

func TestRegistrySingleActive(t *testing.T) {
	mockScreenSize(t)
	a := newPickerHarness(nil).p
	b := newPickerHarness(nil).p

	Open(a)
	if Active() != a {
		t.Fatal("Active should be a")
	}
	Open(b) // should close a, install b
	if Active() != b {
		t.Fatal("Active should be b")
	}
	Close(a) // wrong identity, no-op
	if Active() != b {
		t.Fatal("Close(a) when b is active should be a no-op")
	}
	Close(b)
	if Active() != nil {
		t.Fatal("Close(b) should clear active")
	}
}
