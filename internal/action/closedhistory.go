package action

import (
	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/display"
	"github.com/micro-editor/micro/v2/internal/screen"
)

// closedBufferEntry records enough state about a closed buffer to reopen it
// later at the same cursor position and scroll offset.
type closedBufferEntry struct {
	AbsPath   string
	Cursor    buffer.Loc
	StartLine display.SLoc
	StartCol  int
}

// closedBufferHistoryCap bounds how many closed buffers are remembered.
// Pushing past the cap evicts the oldest entry.
const closedBufferHistoryCap = 20

// closedBufferHistory is the package-level LRU stack of recently closed
// buffers, oldest first, most recently closed last.
var closedBufferHistory []closedBufferEntry

// pushClosedBuffer records a closed buffer, evicting the oldest entry once
// the history is over capacity.
func pushClosedBuffer(entry closedBufferEntry) {
	closedBufferHistory = append(closedBufferHistory, entry)
	if len(closedBufferHistory) > closedBufferHistoryCap {
		closedBufferHistory = closedBufferHistory[1:]
	}
}

// popClosedBuffer removes and returns the most recently closed entry.
func popClosedBuffer() (closedBufferEntry, bool) {
	if len(closedBufferHistory) == 0 {
		return closedBufferEntry{}, false
	}
	entry := closedBufferHistory[len(closedBufferHistory)-1]
	closedBufferHistory = closedBufferHistory[:len(closedBufferHistory)-1]
	return entry, true
}

// sharedBufferOpenElsewhere reports whether some pane other than exclude,
// among the given tabs, is displaying a buffer backed by sb.
func sharedBufferOpenElsewhere(sb *buffer.SharedBuffer, exclude Pane, tabs []*Tab) bool {
	for _, tab := range tabs {
		for _, p := range tab.Panes {
			if p == exclude {
				continue
			}
			if bp, ok := p.(*BufPane); ok && bp.Buf.SharedBuffer == sb {
				return true
			}
		}
	}
	return false
}

// findPaneWithAbsPath returns the tab and pane index, among the given tabs,
// of a pane currently displaying a buffer with the given absolute path.
func findPaneWithAbsPath(absPath string, tabs []*Tab) (tabIndex, paneIndex int, found bool) {
	for ti, tab := range tabs {
		for pi, p := range tab.Panes {
			if bp, ok := p.(*BufPane); ok && bp.Buf.AbsPath == absPath {
				return ti, pi, true
			}
		}
	}
	return 0, 0, false
}

// recordClosedBuffer saves h's file, cursor and scroll position to the
// closed-buffer history, unless h is not a real on-disk file or the same
// SharedBuffer is still visible in another pane (in which case the file
// isn't actually closed, and reopening it later would just duplicate it).
// Must be called before h.Buf.Close(), while h is still among tabs.
func recordClosedBuffer(h *BufPane, tabs []*Tab) {
	if h.Buf.Type != buffer.BTDefault || h.Buf.AbsPath == "" {
		return
	}
	if sharedBufferOpenElsewhere(h.Buf.SharedBuffer, h, tabs) {
		return
	}

	view := h.GetView()
	pushClosedBuffer(closedBufferEntry{
		AbsPath:   h.Buf.AbsPath,
		Cursor:    h.Cursor.Loc,
		StartLine: view.StartLine,
		StartCol:  view.StartCol,
	})
}

// ReopenLastClosed reopens the most recently closed buffer. If that file is
// already open in a pane, focus switches there instead of duplicating it.
// Otherwise the file is opened in a new tab with its cursor and scroll
// position restored. If the file can no longer be opened, the entry is
// dropped and the next most recently closed one is tried, so repeated
// invocations dig deeper into the history.
func (h *BufPane) ReopenLastClosed() bool {
	for {
		entry, ok := popClosedBuffer()
		if !ok {
			return false
		}
		if switchToPaneWithAbsPath(entry.AbsPath) {
			return true
		}
		if reopenClosedBufferInNewTab(entry) {
			return true
		}
	}
}

// switchToPaneWithAbsPath focuses the pane displaying absPath, if any.
func switchToPaneWithAbsPath(absPath string) bool {
	ti, pi, found := findPaneWithAbsPath(absPath, Tabs.List)
	if !found {
		return false
	}
	Tabs.List[ti].SetActive(pi)
	Tabs.SetActive(ti)
	return true
}

// reopenClosedBufferInNewTab opens entry's file in a new tab, restoring its
// cursor and scroll position. It reports an InfoBar error and returns false
// if the file can no longer be opened.
func reopenClosedBufferInNewTab(entry closedBufferEntry) bool {
	b, err := buffer.NewBufferFromFileWithCommand(entry.AbsPath, buffer.BTDefault,
		buffer.Command{StartCursor: entry.Cursor})
	if err != nil {
		InfoBar.Error(err)
		return false
	}

	width, height := screen.Screen.Size()
	iOffset := config.GetInfoBarOffset()
	tab := NewTabFromBuffer(0, 0, width, height-1-iOffset, b)
	Tabs.AddTab(tab)
	Tabs.SetActive(len(Tabs.List) - 1)

	if bp, ok := tab.Panes[0].(*BufPane); ok {
		view := bp.GetView()
		view.StartLine = entry.StartLine
		view.StartCol = entry.StartCol
		bp.SetView(view)
	}
	return true
}
