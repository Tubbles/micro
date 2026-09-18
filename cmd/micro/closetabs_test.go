package main

import (
	"os"
	"testing"

	"github.com/Tubbles/tcell/v3"
	"github.com/micro-editor/micro/v2/internal/action"
	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/stretchr/testify/assert"
)

// openInNewTab opens a fresh file with content in a new tab via the
// "> tab" command and returns its buffer. The new tab is active.
func openInNewTab(t *testing.T, content string) *buffer.Buffer {
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

// tabShowing returns the tab whose current pane shows buf, or nil.
func tabShowing(buf *buffer.Buffer) *action.Tab {
	for _, tab := range action.Tabs.List {
		if pane := tab.CurPane(); pane != nil && pane.Buf == buf {
			return tab
		}
	}
	return nil
}

func activateTabShowing(t *testing.T, buf *buffer.Buffer) {
	t.Helper()
	tab := tabShowing(buf)
	if tab == nil {
		t.Fatalf("no tab shows the buffer")
	}
	action.Tabs.SetActive(action.Tabs.IndexOf(tab))
}

// discardPrompts answers any pending save prompt with "n" so tabs left
// behind by earlier tests cannot stall a close.
func discardPrompts() {
	for attempt := 0; attempt < 20 && action.InfoBar.HasPrompt; attempt++ {
		injectString("n")
	}
}

func TestCloseOtherTabsKeepsCurrent(t *testing.T) {
	openInNewTab(t, "a\n")
	b := openInNewTab(t, "b\n")
	openInNewTab(t, "c\n")
	activateTabShowing(t, b)

	action.MainTab().CurPane().CloseOtherTabs()
	drainEvents()
	discardPrompts()

	assert.Len(t, action.Tabs.List, 1)
	assert.Equal(t, b, action.MainTab().CurPane().Buf)
}

func TestCloseTabsToTheRightAndLeft(t *testing.T) {
	a := openInNewTab(t, "a\n")
	b := openInNewTab(t, "b\n")
	c := openInNewTab(t, "c\n")
	d := openInNewTab(t, "d\n")
	activateTabShowing(t, c)

	action.MainTab().CurPane().CloseTabsToTheRight()
	drainEvents()
	discardPrompts()
	assert.Nil(t, tabShowing(d))
	assert.NotNil(t, tabShowing(a))
	assert.NotNil(t, tabShowing(b))
	assert.Equal(t, c, action.MainTab().CurPane().Buf)

	action.MainTab().CurPane().CloseTabsToTheLeft()
	drainEvents()
	discardPrompts()
	assert.Len(t, action.Tabs.List, 1)
	assert.Equal(t, c, action.MainTab().CurPane().Buf)
}

func TestCloseOtherTabsPromptsForUnsavedChanges(t *testing.T) {
	// Start from a single clean tab so the only prompt is the one
	// this test provokes.
	openInNewTab(t, "clean\n")
	action.MainTab().CurPane().CloseOtherTabs()
	drainEvents()
	discardPrompts()

	a := openInNewTab(t, "a\n")
	a.Insert(buffer.Loc{0, 0}, "changed ")
	b := openInNewTab(t, "b\n")
	activateTabShowing(t, b)

	// Escape cancels: the modified tab stays.
	action.MainTab().CurPane().CloseOtherTabs()
	drainEvents()
	assert.True(t, action.InfoBar.HasPrompt)
	injectKey(tcell.KeyEscape, 0, tcell.ModNone)
	assert.NotNil(t, tabShowing(a))
	assert.Equal(t, b, action.MainTab().CurPane().Buf)

	// "n" closes it without saving.
	action.MainTab().CurPane().CloseOtherTabs()
	drainEvents()
	assert.True(t, action.InfoBar.HasPrompt)
	injectString("n")
	assert.Nil(t, tabShowing(a))
	assert.Equal(t, b, action.MainTab().CurPane().Buf)
	data, err := os.ReadFile(a.Path)
	if assert.NoError(t, err) {
		assert.Equal(t, "a\n", string(data))
	}
}

func TestCloseAllTabsLeavesOneEmptyBuffer(t *testing.T) {
	openInNewTab(t, "a\n")
	openInNewTab(t, "b\n")

	action.MainTab().CurPane().CloseAllTabs()
	drainEvents()
	discardPrompts()

	assert.Len(t, action.Tabs.List, 1)
	buf := action.MainTab().CurPane().Buf
	assert.Equal(t, "", buf.Path)
	assert.Equal(t, "", string(buf.Bytes()))
}
