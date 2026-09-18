package action

import (
	"testing"

	"github.com/micro-editor/micro/v2/internal/config"
)

// withHistorySize sets the runtime historysize value and restores
// whatever was there on test exit. Returns nothing — tests inspect
// the package-level historyEntries via recentHistory().
func withHistorySize(t *testing.T, n int) {
	t.Helper()
	if config.GlobalSettings == nil {
		config.GlobalSettings = map[string]any{}
	}
	prev, had := config.GlobalSettings["commandpalette.historysize"]
	config.GlobalSettings["commandpalette.historysize"] = float64(n)
	clearHistory()
	t.Cleanup(func() {
		clearHistory()
		if had {
			config.GlobalSettings["commandpalette.historysize"] = prev
		} else {
			delete(config.GlobalSettings, "commandpalette.historysize")
		}
	})
}

func TestHistoryRecordAndRecall(t *testing.T) {
	withHistorySize(t, 20)
	recordHistory(historyEntry{Kind: historyAction, Name: "Save"})
	h := recentHistory()
	if len(h) != 1 || h[0].Kind != historyAction || h[0].Name != "Save" {
		t.Fatalf("history: %+v, want [{Save action}]", h)
	}
}

func TestHistoryDedupsAndMovesToTop(t *testing.T) {
	withHistorySize(t, 20)
	recordHistory(historyEntry{Kind: historyAction, Name: "Save"})
	recordHistory(historyEntry{Kind: historyAction, Name: "Quit"})
	recordHistory(historyEntry{Kind: historyAction, Name: "Save"})
	h := recentHistory()
	if len(h) != 2 {
		t.Fatalf("history len=%d, want 2 (Save deduped)", len(h))
	}
	if h[0].Name != "Save" || h[1].Name != "Quit" {
		t.Fatalf("history order=%+v, want [Save, Quit]", h)
	}
}

func TestHistoryTrimsToSize(t *testing.T) {
	withHistorySize(t, 3)
	recordHistory(historyEntry{Kind: historyAction, Name: "a"})
	recordHistory(historyEntry{Kind: historyAction, Name: "b"})
	recordHistory(historyEntry{Kind: historyAction, Name: "c"})
	recordHistory(historyEntry{Kind: historyAction, Name: "d"})
	h := recentHistory()
	if len(h) != 3 {
		t.Fatalf("history len=%d, want 3", len(h))
	}
	if h[0].Name != "d" || h[1].Name != "c" || h[2].Name != "b" {
		t.Fatalf("history order=%+v, want [d, c, b] (a dropped)", h)
	}
}

func TestHistorySizeZeroClearsAndSkips(t *testing.T) {
	withHistorySize(t, 5)
	recordHistory(historyEntry{Kind: historyCommand, Name: "save"})
	if len(recentHistory()) != 1 {
		t.Fatalf("precondition: history not recording")
	}
	config.GlobalSettings["commandpalette.historysize"] = float64(0)
	recordHistory(historyEntry{Kind: historyCommand, Name: "quit"})
	if h := recentHistory(); len(h) != 0 {
		t.Fatalf("history after size=0: %+v, want empty", h)
	}
}

func TestHistoryDifferentKindsAreDistinct(t *testing.T) {
	withHistorySize(t, 20)
	recordHistory(historyEntry{Kind: historyAction, Name: "Save"})
	recordHistory(historyEntry{Kind: historyCommand, Name: "Save"})
	if h := recentHistory(); len(h) != 2 {
		t.Fatalf("history len=%d, want 2 (action vs command both kept)", len(h))
	}
}

// TestOrderRecentFirstPromotesHistory checks the atlas ordering the
// palette opens with: recent rows first in most-recent-first order,
// the rest in atlas order, ghosts dropped, typed command lines shown
// as text rows unless the atlas enumerates that exact line.
func TestOrderRecentFirstPromotesHistory(t *testing.T) {
	atlas := []paletteEntry{
		{Kind: paletteAction, Name: "Quit"},
		{Kind: paletteAction, Name: "Save", Bindings: []string{"Ctrl-s"}},
		{Kind: paletteCommand, Name: "saveas"},
		{Kind: paletteCommandArg, Name: "help options"},
		{Kind: paletteLua, Name: "comment.comment"},
	}
	hist := []historyEntry{
		{Kind: historyFreeText, Name: "saveas /tmp/x"},
		{Kind: historyAction, Name: "Save"},
		{Kind: historyFreeText, Name: "help options"},
		{Kind: historyLua, Name: "gone.plugin"},
		{Kind: historyCommand, Name: "saveas"},
	}

	ordered, recent := orderRecentFirst(atlas, hist)

	want := []paletteEntry{
		{Kind: paletteFreeText, Name: "saveas /tmp/x"},
		{Kind: paletteAction, Name: "Save", Bindings: []string{"Ctrl-s"}},
		{Kind: paletteCommandArg, Name: "help options"},
		{Kind: paletteCommand, Name: "saveas"},
		{Kind: paletteAction, Name: "Quit"},
		{Kind: paletteLua, Name: "comment.comment"},
	}
	if recent != 4 {
		t.Fatalf("recent count = %d, want 4", recent)
	}
	if len(ordered) != len(want) {
		t.Fatalf("ordered = %+v, want %+v", ordered, want)
	}
	for i := range want {
		if ordered[i].Kind != want[i].Kind || ordered[i].Name != want[i].Name {
			t.Errorf("row %d = %+v, want %+v", i, ordered[i], want[i])
		}
	}
	if len(ordered[1].Bindings) != 1 {
		t.Errorf("a promoted row must keep its bindings column: %+v", ordered[1])
	}
}

func TestOrderRecentFirstWithoutHistoryKeepsAtlas(t *testing.T) {
	atlas := []paletteEntry{{Kind: paletteAction, Name: "Quit"}, {Kind: paletteAction, Name: "Save"}}
	ordered, recent := orderRecentFirst(atlas, nil)
	if recent != 0 || len(ordered) != 2 || ordered[0].Name != "Quit" {
		t.Fatalf("ordered = %+v recent = %d, want atlas order and 0", ordered, recent)
	}
}

func TestHistoryFreeTextDedups(t *testing.T) {
	withHistorySize(t, 20)
	recordHistory(historyEntry{Kind: historyFreeText, Name: "saveas /tmp/a"})
	recordHistory(historyEntry{Kind: historyFreeText, Name: "saveas /tmp/b"})
	recordHistory(historyEntry{Kind: historyFreeText, Name: "saveas /tmp/a"})
	h := recentHistory()
	if len(h) != 2 || h[0].Name != "saveas /tmp/a" || h[1].Name != "saveas /tmp/b" {
		t.Fatalf("free-text history=%+v, want [saveas /tmp/a, saveas /tmp/b]", h)
	}
}
