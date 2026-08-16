package action

import (
	"sync"

	"github.com/micro-editor/micro/v2/internal/config"
)

// historyKind discriminates the four ways a palette dispatch can be
// shaped. The first three mirror paletteKind so a history entry can
// re-enter executePaletteEntry. historyFreeText covers what the user
// typed verbatim through the picker's OnSubmit path (e.g.
// "saveas foo.txt"): no matching paletteEntry exists, so re-dispatch
// goes back through HandleCommand.
type historyKind int

const (
	historyAction historyKind = iota
	historyCommand
	historyLua
	historyFreeText
)

// historyEntry is one row in the per-session history. Identity for
// dedup is the (Kind, Name) tuple.
type historyEntry struct {
	Kind historyKind
	Name string
}

var (
	historyMu      sync.Mutex
	historyEntries []historyEntry // most-recent-first
)

// recordHistory prepends e to the history, removing any prior copy
// (LRU-dedup) and trimming to the configured size. The size is read
// fresh each call so a live `> set commandpalette.historysize` is
// honoured. Size 0 disables the feature: the slice is cleared and
// the call is a no-op for further recording.
func recordHistory(e historyEntry) {
	size := historySize()
	historyMu.Lock()
	defer historyMu.Unlock()
	if size <= 0 {
		historyEntries = nil
		return
	}
	for i, prior := range historyEntries {
		if prior == e {
			historyEntries = append(historyEntries[:i], historyEntries[i+1:]...)
			break
		}
	}
	historyEntries = append([]historyEntry{e}, historyEntries...)
	if len(historyEntries) > size {
		historyEntries = historyEntries[:size]
	}
}

// recentHistory returns a snapshot copy of the current history,
// most-recent-first. The copy isolates the caller from concurrent
// recording; the slice can be ranged over without holding the lock.
func recentHistory() []historyEntry {
	historyMu.Lock()
	defer historyMu.Unlock()
	out := make([]historyEntry, len(historyEntries))
	copy(out, historyEntries)
	return out
}

// clearHistory wipes the history. Used by tests.
func clearHistory() {
	historyMu.Lock()
	defer historyMu.Unlock()
	historyEntries = nil
}

// historySize reads the configured maximum. Defaults to 0 (disabled)
// when the setting is missing or wrong-typed, which matches the
// behaviour for an unrecognised option elsewhere.
func historySize() int {
	v, ok := config.GlobalSettings["commandpalette.historysize"]
	if !ok {
		return 0
	}
	f, ok := v.(float64)
	if !ok {
		return 0
	}
	if f < 0 {
		return 0
	}
	return int(f)
}
