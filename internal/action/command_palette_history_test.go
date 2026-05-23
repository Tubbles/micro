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

func TestHistoryItemLabelReusesAtlasLabel(t *testing.T) {
	atlas := []paletteEntry{
		{Kind: paletteAction, Name: "Save", Bindings: []string{"Ctrl-S"}},
		{Kind: paletteCommand, Name: "saveas"},
	}
	if got := historyItemLabel(atlas, historyEntry{Kind: historyAction, Name: "Save"}); got != "action Save [Ctrl-S]" {
		t.Fatalf("known action: got %q, want %q", got, "action Save [Ctrl-S]")
	}
	if got := historyItemLabel(atlas, historyEntry{Kind: historyCommand, Name: "saveas"}); got != "cmd    saveas" {
		t.Fatalf("known command: got %q", got)
	}
	if got := historyItemLabel(atlas, historyEntry{Kind: historyAction, Name: "Ghost"}); got != "action Ghost" {
		t.Fatalf("missing action falls back: got %q, want \"action Ghost\"", got)
	}
	if got := historyItemLabel(atlas, historyEntry{Kind: historyFreeText, Name: "saveas /tmp/x"}); got != "text   saveas /tmp/x" {
		t.Fatalf("free-text: got %q", got)
	}
}

func TestPaletteHintReflectsHistoryAvailability(t *testing.T) {
	atlasHint := paletteHint(paletteModeAtlas, 0)
	if atlasHint != "<Up>/<Down> move - <Enter> run match - <Ctrl-Enter> run as command - <Esc> cancel" {
		t.Fatalf("size 0 hint must omit Tab: got %q", atlasHint)
	}
	atlasHintWithHist := paletteHint(paletteModeAtlas, 20)
	if atlasHintWithHist[:5] != "<Tab>" {
		t.Fatalf("size 20 atlas hint must start with Tab: got %q", atlasHintWithHist)
	}
	historyHint := paletteHint(paletteModeHistory, 20)
	if historyHint[:12] != "<Tab> Atlas " {
		t.Fatalf("history hint must lead with Tab Atlas: got %q", historyHint)
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
