package action

import (
	"path/filepath"
	"testing"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/display"
	ulua "github.com/micro-editor/micro/v2/internal/lua"
	"github.com/stretchr/testify/assert"
	lua "github.com/yuin/gopher-lua"
)

func init() {
	ulua.L = lua.NewState()
	config.InitRuntimeFiles(false)
	config.InitGlobalSettings()
}

// newTestTabPane builds a single-pane tab wrapping a buffer with the given
// text, path and type. It does not touch the screen: NewTabFromBuffer only
// finishes initializing the pane (relocating the view, etc.) on its first
// Resize, which this helper never calls.
func newTestTabPane(path, text string, btype buffer.BufType) (*Tab, *BufPane) {
	b := buffer.NewBufferFromString(text, path, btype)
	tab := NewTabFromBuffer(0, 0, 80, 24, b)
	return tab, tab.Panes[0].(*BufPane)
}

func resetClosedBufferHistory() {
	closedBufferHistory = nil
}

func TestPushPopClosedBufferOrder(t *testing.T) {
	resetClosedBufferHistory()

	pushClosedBuffer(closedBufferEntry{AbsPath: "a"})
	pushClosedBuffer(closedBufferEntry{AbsPath: "b"})
	pushClosedBuffer(closedBufferEntry{AbsPath: "c"})

	entry, ok := popClosedBuffer()
	assert.True(t, ok)
	assert.Equal(t, "c", entry.AbsPath)

	entry, ok = popClosedBuffer()
	assert.True(t, ok)
	assert.Equal(t, "b", entry.AbsPath)

	entry, ok = popClosedBuffer()
	assert.True(t, ok)
	assert.Equal(t, "a", entry.AbsPath)

	_, ok = popClosedBuffer()
	assert.False(t, ok)
}

func TestClosedBufferHistoryCapEviction(t *testing.T) {
	resetClosedBufferHistory()

	for i := 0; i < closedBufferHistoryCap+1; i++ {
		pushClosedBuffer(closedBufferEntry{AbsPath: filepath.Join("/", "path", string(rune('a'+i)))})
	}

	assert.Len(t, closedBufferHistory, closedBufferHistoryCap)
	// The oldest entry (index 0, "a") should have been evicted; the next
	// oldest surviving entry is "b".
	assert.Equal(t, filepath.Join("/", "path", "b"), closedBufferHistory[0].AbsPath)
	// The most recently pushed entry survives at the top of the stack.
	assert.Equal(t, filepath.Join("/", "path", string(rune('a'+closedBufferHistoryCap))), closedBufferHistory[len(closedBufferHistory)-1].AbsPath)
}

func TestRecordClosedBufferFilters(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("scratch buffer is not recorded", func(t *testing.T) {
		resetClosedBufferHistory()
		tab, bp := newTestTabPane(filepath.Join(tempDir, "scratch"), "hello", buffer.BTScratch)

		recordClosedBuffer(bp, []*Tab{tab})

		assert.Empty(t, closedBufferHistory)
	})

	t.Run("unnamed buffer is not recorded", func(t *testing.T) {
		resetClosedBufferHistory()
		tab, bp := newTestTabPane("", "hello", buffer.BTDefault)

		recordClosedBuffer(bp, []*Tab{tab})

		assert.Empty(t, closedBufferHistory)
	})

	t.Run("buffer still open in another pane is not recorded", func(t *testing.T) {
		resetClosedBufferHistory()
		path := filepath.Join(tempDir, "shared")
		tab1, bp1 := newTestTabPane(path, "hello", buffer.BTDefault)
		tab2, _ := newTestTabPane(path, "hello", buffer.BTDefault)

		recordClosedBuffer(bp1, []*Tab{tab1, tab2})

		assert.Empty(t, closedBufferHistory)
	})

	t.Run("plain file not open elsewhere is recorded", func(t *testing.T) {
		resetClosedBufferHistory()
		path := filepath.Join(tempDir, "plain")
		tab, bp := newTestTabPane(path, "hello\nworld\n", buffer.BTDefault)
		bp.Cursor.Loc = buffer.Loc{X: 3, Y: 1}
		view := bp.GetView()
		view.StartLine = display.SLoc{Line: 1, Row: 0}
		view.StartCol = 2
		bp.SetView(view)

		recordClosedBuffer(bp, []*Tab{tab})

		if assert.Len(t, closedBufferHistory, 1) {
			entry := closedBufferHistory[0]
			assert.Equal(t, bp.Buf.AbsPath, entry.AbsPath)
			assert.Equal(t, buffer.Loc{X: 3, Y: 1}, entry.Cursor)
			assert.Equal(t, display.SLoc{Line: 1, Row: 0}, entry.StartLine)
			assert.Equal(t, 2, entry.StartCol)
		}
	})
}

func TestFindPaneWithAbsPath(t *testing.T) {
	tempDir := t.TempDir()
	pathA := filepath.Join(tempDir, "a")
	pathB := filepath.Join(tempDir, "b")

	tabA, bpA := newTestTabPane(pathA, "a", buffer.BTDefault)
	tabB, _ := newTestTabPane(pathB, "b", buffer.BTDefault)
	tabs := []*Tab{tabA, tabB}

	ti, pi, found := findPaneWithAbsPath(bpA.Buf.AbsPath, tabs)
	assert.True(t, found)
	assert.Equal(t, 0, ti)
	assert.Equal(t, 0, pi)

	_, _, found = findPaneWithAbsPath(filepath.Join(tempDir, "missing"), tabs)
	assert.False(t, found)
}
