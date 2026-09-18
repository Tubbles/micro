package main

import (
	"testing"

	"github.com/Tubbles/tcell/v3"
	"github.com/micro-editor/micro/v2/internal/action"
	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/stretchr/testify/assert"
)

// newTabWithFile opens a fresh file with content in a new tab via the
// "> tab" command and returns its buffer. The new tab is active.
func newTabWithFile(t *testing.T, content string) *buffer.Buffer {
	t.Helper()
	file := createTestFile(t, content)
	injectKey(tcell.KeyCtrlE, rune(tcell.KeyCtrlE), tcell.ModCtrl)
	injectString("tab " + file)
	injectKey(tcell.KeyEnter, rune(tcell.KeyEnter), tcell.ModNone)
	buf := findBuffer(file)
	if buf == nil {
		t.Fatalf("Could not find buffer %s", file)
	}
	return buf
}

// quitExtraTabs quits tabs from the end until one remains, answering
// any save prompt with "n", so a test can start from a known layout.
func quitExtraTabs() {
	for attempt := 0; attempt < 50 && len(action.Tabs.List) > 1; attempt++ {
		action.Tabs.SetActive(len(action.Tabs.List) - 1)
		action.MainTab().CurPane().Quit()
		drainEvents()
		if action.InfoBar.HasPrompt {
			injectString("n")
		}
	}
}

// paneBuffers lists the buffers shown by tab's panes, in pane order.
func paneBuffers(tab *action.Tab) []*buffer.Buffer {
	var bufs []*buffer.Buffer
	for _, pane := range tab.Panes {
		if bp, ok := pane.(*action.BufPane); ok {
			bufs = append(bufs, bp.Buf)
		}
	}
	return bufs
}

// tabHolding returns the index of the tab with a pane showing buf, or -1.
func tabHolding(buf *buffer.Buffer) int {
	for index, tab := range action.Tabs.List {
		for _, b := range paneBuffers(tab) {
			if b == buf {
				return index
			}
		}
	}
	return -1
}

func activatePaneShowing(t *testing.T, buf *buffer.Buffer) {
	t.Helper()
	index := tabHolding(buf)
	if index < 0 {
		t.Fatalf("no tab holds the buffer")
	}
	action.Tabs.SetActive(index)
	tab := action.Tabs.List[index]
	for paneIndex, pane := range tab.Panes {
		if bp, ok := pane.(*action.BufPane); ok && bp.Buf == buf {
			tab.SetActive(paneIndex)
			return
		}
	}
}

// TestMovePaneToNextWalksThroughOwnTab drives a pane from the right edge
// of tab A to the end of the tab list: own tab between A and C, left
// edge of C, right edge of C, own tab after C, then no further.
func TestMovePaneToNextWalksThroughOwnTab(t *testing.T) {
	quitExtraTabs()
	fileA := createTestFile(t, "a\n")
	openFile(fileA)
	a := findBuffer(fileA)
	if a == nil {
		t.Fatalf("Could not find buffer %s", fileA)
	}
	moving := buffer.NewBufferFromString("moving", "", buffer.BTDefault)
	action.MainTab().CurPane().VSplitBuf(moving)
	c := newTabWithFile(t, "c\n")
	activatePaneShowing(t, moving)
	if !assert.Len(t, action.Tabs.List, 2, "layout: A(a, moving), C(c)") {
		return
	}

	pane := action.MainTab().CurPane()

	// Right edge of A with another pane present: promoted to its own tab B.
	pane.MovePaneToNext()
	assert.Len(t, action.Tabs.List, 3)
	assert.Equal(t, []*buffer.Buffer{a}, paneBuffers(action.Tabs.List[0]))
	assert.Equal(t, []*buffer.Buffer{moving}, paneBuffers(action.Tabs.List[1]))
	assert.Equal(t, []*buffer.Buffer{c}, paneBuffers(action.Tabs.List[2]))
	assert.Equal(t, 1, action.Tabs.Active())

	// Alone in B: crosses into C, and B disappears.
	pane = action.MainTab().CurPane()
	pane.MovePaneToNext()
	assert.Len(t, action.Tabs.List, 2)
	assert.ElementsMatch(t, []*buffer.Buffer{c, moving}, paneBuffers(action.Tabs.List[1]))
	assert.Equal(t, 1, action.Tabs.Active())
	assert.Equal(t, moving, action.MainTab().CurPane().Buf)

	// Inside C: swaps to the right edge, same tabs.
	pane = action.MainTab().CurPane()
	pane.MovePaneToNext()
	assert.Len(t, action.Tabs.List, 2)

	// Right edge of C with another pane present: own tab D after C.
	pane = action.MainTab().CurPane()
	pane.MovePaneToNext()
	assert.Len(t, action.Tabs.List, 3)
	assert.Equal(t, []*buffer.Buffer{c}, paneBuffers(action.Tabs.List[1]))
	assert.Equal(t, []*buffer.Buffer{moving}, paneBuffers(action.Tabs.List[2]))

	// Alone in the last tab: nowhere to go.
	pane = action.MainTab().CurPane()
	assert.False(t, pane.MovePaneToNext())
	assert.Len(t, action.Tabs.List, 3)

	// And back: alone in D crosses into C's right edge.
	pane = action.MainTab().CurPane()
	pane.MovePaneToPrevious()
	assert.Len(t, action.Tabs.List, 2)
	assert.ElementsMatch(t, []*buffer.Buffer{c, moving}, paneBuffers(action.Tabs.List[1]))
}
