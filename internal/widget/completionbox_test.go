package widget

import (
	"testing"

	"github.com/Tubbles/tcell/v3"

	"github.com/micro-editor/micro/v2/internal/buffer"
)

// completionHarness builds a CompletionBox and records what its
// callbacks were fired with, mirroring pickerHarness.
type completionHarness struct {
	c           *CompletionBox
	selectCount int
	lastSelect  int
	closeCount  int
}

func newCompletionHarness(items []CompletionItem) *completionHarness {
	h := &completionHarness{}
	h.c = NewCompletionBox(CompletionBoxOptions{
		Loc:   buffer.Loc{X: 0, Y: 0},
		Items: items,
		OnSelect: func(i int) {
			h.selectCount++
			h.lastSelect = i
		},
		OnClose: func() { h.closeCount++ },
	})
	return h
}

func TestCompletionBoxArrowsClampNoWrap(t *testing.T) {
	items := []CompletionItem{{Label: "a"}, {Label: "b"}, {Label: "c"}}
	h := newCompletionHarness(items)

	h.c.HandleEvent(key(tcell.KeyUp))
	if h.c.current != 0 {
		t.Fatalf("Up at top: want current=0, got %d", h.c.current)
	}

	for i := 0; i < 5; i++ {
		h.c.HandleEvent(key(tcell.KeyDown))
	}
	if h.c.current != 2 {
		t.Fatalf("Down past end: want current=2, got %d", h.c.current)
	}

	h.c.HandleEvent(key(tcell.KeyDown))
	if h.c.current != 2 {
		t.Fatalf("Down at end: want current=2, got %d", h.c.current)
	}
}

func TestCompletionBoxCtrlPCtrlNMoveLikeArrows(t *testing.T) {
	items := []CompletionItem{{Label: "a"}, {Label: "b"}, {Label: "c"}}
	h := newCompletionHarness(items)

	h.c.HandleEvent(key(tcell.KeyCtrlN))
	if h.c.current != 1 {
		t.Fatalf("Ctrl-N: want current=1, got %d", h.c.current)
	}
	h.c.HandleEvent(key(tcell.KeyCtrlN))
	if h.c.current != 2 {
		t.Fatalf("Ctrl-N: want current=2, got %d", h.c.current)
	}
	h.c.HandleEvent(key(tcell.KeyCtrlP))
	if h.c.current != 1 {
		t.Fatalf("Ctrl-P: want current=1, got %d", h.c.current)
	}
}

func TestCompletionBoxScrollsWhenMoreItemsThanRows(t *testing.T) {
	items := make([]CompletionItem, completionMaxVisibleRows+5)
	for i := range items {
		items[i].Label = "item"
	}
	h := newCompletionHarness(items)
	bh := h.c.bodyHeight()
	if bh != completionMaxVisibleRows {
		t.Fatalf("bodyHeight() = %d, want %d", bh, completionMaxVisibleRows)
	}

	for i := 0; i < len(items)-1; i++ {
		h.c.HandleEvent(key(tcell.KeyDown))
	}
	if h.c.current != len(items)-1 {
		t.Fatalf("current = %d, want %d", h.c.current, len(items)-1)
	}
	if h.c.top != len(items)-bh {
		t.Fatalf("top = %d, want %d (last row scrolled into view)", h.c.top, len(items)-bh)
	}
}

func TestCompletionBoxEnterAppliesThenCloses(t *testing.T) {
	items := []CompletionItem{{Label: "a"}, {Label: "b"}, {Label: "c"}}
	h := newCompletionHarness(items)
	h.c.HandleEvent(key(tcell.KeyDown)) // current = 1
	Open(h.c)

	consumed := h.c.HandleEvent(key(tcell.KeyEnter))
	if !consumed {
		t.Fatalf("Enter: consumed = false, want true")
	}
	if h.selectCount != 1 || h.lastSelect != 1 {
		t.Fatalf("OnSelect fired %d times with index %d, want 1 time with index 1", h.selectCount, h.lastSelect)
	}
	if h.closeCount != 1 {
		t.Fatalf("OnClose fired %d times, want 1", h.closeCount)
	}
	if Active() != nil {
		t.Fatalf("Active() = %v, want nil after accept", Active())
	}
}

func TestCompletionBoxTabAcceptsLikeEnter(t *testing.T) {
	items := []CompletionItem{{Label: "a"}}
	h := newCompletionHarness(items)
	Open(h.c)

	consumed := h.c.HandleEvent(key(tcell.KeyTab))
	if !consumed {
		t.Fatalf("Tab: consumed = false, want true")
	}
	if h.selectCount != 1 || h.lastSelect != 0 {
		t.Fatalf("OnSelect fired %d times with index %d, want 1 time with index 0", h.selectCount, h.lastSelect)
	}
}

func TestCompletionBoxEscClosesWithoutSelecting(t *testing.T) {
	items := []CompletionItem{{Label: "a"}}
	h := newCompletionHarness(items)
	Open(h.c)

	consumed := h.c.HandleEvent(key(tcell.KeyEsc))
	if !consumed {
		t.Fatalf("Esc: consumed = false, want true")
	}
	if h.selectCount != 0 {
		t.Fatalf("OnSelect fired %d times, want 0", h.selectCount)
	}
	if h.closeCount != 1 {
		t.Fatalf("OnClose fired %d times, want 1", h.closeCount)
	}
	if Active() != nil {
		t.Fatalf("Active() = %v, want nil after Esc", Active())
	}
}

func TestCompletionBoxOtherKeyClosesAndFallsThrough(t *testing.T) {
	items := []CompletionItem{{Label: "a"}}
	h := newCompletionHarness(items)
	Open(h.c)

	consumed := h.c.HandleEvent(runeKey('x'))
	if consumed {
		t.Fatalf("unrecognized key: consumed = true, want false (falls through to the buffer)")
	}
	if h.selectCount != 0 {
		t.Fatalf("OnSelect fired %d times, want 0", h.selectCount)
	}
	if h.closeCount != 1 {
		t.Fatalf("OnClose fired %d times, want 1", h.closeCount)
	}
	if Active() != nil {
		t.Fatalf("Active() = %v, want nil after fallthrough dismissal", Active())
	}
}

func TestCompletionBoxMouseClosesAndFallsThrough(t *testing.T) {
	items := []CompletionItem{{Label: "a"}}
	h := newCompletionHarness(items)
	Open(h.c)

	consumed := h.c.HandleEvent(mouse(0, 0, tcell.Button1))
	if consumed {
		t.Fatalf("mouse event: consumed = true, want false")
	}
	if h.closeCount != 1 {
		t.Fatalf("OnClose fired %d times, want 1", h.closeCount)
	}
}

func TestCompletionBoxSizeGrowsWithContentAndClamps(t *testing.T) {
	w, h := completionBoxSize(nil)
	if w != completionMinWidth {
		t.Errorf("empty items: w = %d, want completionMinWidth %d", w, completionMinWidth)
	}
	if h != 3 { // 1 row (clamped up from 0) + 2 borders
		t.Errorf("empty items: h = %d, want 3", h)
	}

	longLabel := make([]byte, completionMaxWidth*2)
	for i := range longLabel {
		longLabel[i] = 'x'
	}
	w, _ = completionBoxSize([]CompletionItem{{Label: string(longLabel)}})
	if w != completionMaxWidth {
		t.Errorf("very long label: w = %d, want clamp to completionMaxWidth %d", w, completionMaxWidth)
	}

	items := make([]CompletionItem, completionMaxVisibleRows+3)
	for i := range items {
		items[i].Label = "x"
	}
	_, h = completionBoxSize(items)
	if h != completionMaxVisibleRows+2 {
		t.Errorf("many items: h = %d, want clamp to completionMaxVisibleRows+2 %d", h, completionMaxVisibleRows+2)
	}
}

func TestCompletionBoxAcceptOutOfRangeCloses(t *testing.T) {
	// Zero items: current (0) is out of range for accept, which
	// should still close cleanly without firing OnSelect or panicking.
	h := newCompletionHarness(nil)
	Open(h.c)

	h.c.HandleEvent(key(tcell.KeyEnter))
	if h.selectCount != 0 {
		t.Fatalf("OnSelect fired %d times, want 0", h.selectCount)
	}
	if h.closeCount != 1 {
		t.Fatalf("OnClose fired %d times, want 1", h.closeCount)
	}
}
