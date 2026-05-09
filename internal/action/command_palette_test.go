package action

import (
	"testing"

	"github.com/micro-editor/micro/v2/internal/config"
)

func setupPaletteTest(t *testing.T) {
	t.Helper()
	if config.GlobalSettings == nil {
		config.GlobalSettings = config.DefaultAllSettings()
	}
	for _, k := range []string{"commandpalette.actions", "commandpalette.commands", "commandpalette.lua"} {
		if _, ok := config.GlobalSettings[k]; !ok {
			config.GlobalSettings[k] = true
		}
	}
	prev := map[string]any{
		"commandpalette.actions":  config.GlobalSettings["commandpalette.actions"],
		"commandpalette.commands": config.GlobalSettings["commandpalette.commands"],
		"commandpalette.lua":      config.GlobalSettings["commandpalette.lua"],
	}
	config.GlobalSettings["commandpalette.actions"] = true
	config.GlobalSettings["commandpalette.commands"] = true
	config.GlobalSettings["commandpalette.lua"] = true
	t.Cleanup(func() {
		for k, v := range prev {
			config.GlobalSettings[k] = v
		}
	})
	if commands == nil {
		InitCommands()
	}
}

func findPaletteEntry(entries []paletteEntry, k paletteKind, n string) *paletteEntry {
	for i := range entries {
		if entries[i].Kind == k && entries[i].Name == n {
			return &entries[i]
		}
	}
	return nil
}

func paletteContainsBinding(e *paletteEntry, ev string) bool {
	if e == nil {
		return false
	}
	for _, b := range e.Bindings {
		if b == ev {
			return true
		}
	}
	return false
}

func TestPaletteEntriesIncludeActionsAndCommands(t *testing.T) {
	setupPaletteTest(t)
	entries := buildPaletteEntries()

	if findPaletteEntry(entries, paletteAction, "DuplicateLine") == nil {
		t.Errorf("paletteAction DuplicateLine missing from entries")
	}
	if findPaletteEntry(entries, paletteCommand, "save") == nil {
		t.Errorf("paletteCommand save missing from entries")
	}
}

func TestPaletteEntriesExcludeMouseActions(t *testing.T) {
	setupPaletteTest(t)
	entries := buildPaletteEntries()

	for _, e := range entries {
		if e.Kind == paletteAction {
			if _, isMouse := BufMouseActions[e.Name]; isMouse {
				t.Errorf("mouse action %q leaked into action entries", e.Name)
			}
		}
	}
}

func TestPaletteEntriesActionStringShape(t *testing.T) {
	setupPaletteTest(t)
	entries := buildPaletteEntries()

	if a := findPaletteEntry(entries, paletteAction, "DuplicateLine"); a != nil {
		if got := a.actionString(); got != "DuplicateLine" {
			t.Errorf("action actionString: got %q, want %q", got, "DuplicateLine")
		}
	}
	if c := findPaletteEntry(entries, paletteCommand, "save"); c != nil {
		if got := c.actionString(); got != "command:save" {
			t.Errorf("command actionString: got %q, want %q", got, "command:save")
		}
	}
}

func TestPaletteReverseBindingExactMatch(t *testing.T) {
	setupPaletteTest(t)

	prevAct, hadAct := config.Bindings["buffer"]["Ctrl-Alt-x"]
	prevCmd, hadCmd := config.Bindings["buffer"]["Ctrl-Alt-y"]
	config.Bindings["buffer"]["Ctrl-Alt-x"] = "DuplicateLine"
	config.Bindings["buffer"]["Ctrl-Alt-y"] = "command:save"
	t.Cleanup(func() {
		if hadAct {
			config.Bindings["buffer"]["Ctrl-Alt-x"] = prevAct
		} else {
			delete(config.Bindings["buffer"], "Ctrl-Alt-x")
		}
		if hadCmd {
			config.Bindings["buffer"]["Ctrl-Alt-y"] = prevCmd
		} else {
			delete(config.Bindings["buffer"], "Ctrl-Alt-y")
		}
	})

	entries := buildPaletteEntries()

	a := findPaletteEntry(entries, paletteAction, "DuplicateLine")
	if !paletteContainsBinding(a, "Ctrl-Alt-x") {
		got := []string(nil)
		if a != nil {
			got = a.Bindings
		}
		t.Errorf("DuplicateLine bindings: want Ctrl-Alt-x, got %v", got)
	}

	c := findPaletteEntry(entries, paletteCommand, "save")
	if !paletteContainsBinding(c, "Ctrl-Alt-y") {
		got := []string(nil)
		if c != nil {
			got = c.Bindings
		}
		t.Errorf("save bindings: want Ctrl-Alt-y, got %v", got)
	}
}

func TestPaletteReverseBindingChainNotMatched(t *testing.T) {
	setupPaletteTest(t)

	prev, had := config.Bindings["buffer"]["Ctrl-Alt-z"]
	config.Bindings["buffer"]["Ctrl-Alt-z"] = "Save,Quit"
	t.Cleanup(func() {
		if had {
			config.Bindings["buffer"]["Ctrl-Alt-z"] = prev
		} else {
			delete(config.Bindings["buffer"], "Ctrl-Alt-z")
		}
	})

	entries := buildPaletteEntries()
	if save := findPaletteEntry(entries, paletteAction, "Save"); save != nil {
		if paletteContainsBinding(save, "Ctrl-Alt-z") {
			t.Errorf("Save bindings should NOT include chain-bound Ctrl-Alt-z (v1 limitation), got %v", save.Bindings)
		}
	}
}

func TestPaletteSettingsGateActions(t *testing.T) {
	setupPaletteTest(t)
	config.GlobalSettings["commandpalette.actions"] = false

	entries := buildPaletteEntries()
	for _, e := range entries {
		if e.Kind == paletteAction {
			t.Errorf("action entry %q present despite commandpalette.actions=false", e.Name)
		}
	}
}

func TestPaletteSettingsGateCommands(t *testing.T) {
	setupPaletteTest(t)
	config.GlobalSettings["commandpalette.commands"] = false

	entries := buildPaletteEntries()
	for _, e := range entries {
		if e.Kind == paletteCommand {
			t.Errorf("command entry %q present despite commandpalette.commands=false", e.Name)
		}
	}
}

func TestPaletteSettingsGateLua(t *testing.T) {
	setupPaletteTest(t)
	config.GlobalSettings["commandpalette.lua"] = false

	entries := buildPaletteEntries()
	for _, e := range entries {
		if e.Kind == paletteLua {
			t.Errorf("lua entry %q present despite commandpalette.lua=false", e.Name)
		}
	}
}

func TestPaletteEntriesAreSortedWithinKind(t *testing.T) {
	setupPaletteTest(t)
	entries := buildPaletteEntries()

	var lastAction, lastCommand, lastLua string
	for _, e := range entries {
		switch e.Kind {
		case paletteAction:
			if lastAction != "" && e.Name < lastAction {
				t.Errorf("action entries unsorted: %q before %q", lastAction, e.Name)
			}
			lastAction = e.Name
		case paletteCommand:
			if lastCommand != "" && e.Name < lastCommand {
				t.Errorf("command entries unsorted: %q before %q", lastCommand, e.Name)
			}
			lastCommand = e.Name
		case paletteLua:
			if lastLua != "" && e.Name < lastLua {
				t.Errorf("lua entries unsorted: %q before %q", lastLua, e.Name)
			}
			lastLua = e.Name
		}
	}
}
