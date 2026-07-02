package action

import (
	"testing"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
	ulua "github.com/micro-editor/micro/v2/internal/lua"
	lua "github.com/yuin/gopher-lua"
)

// setupPaletteArgTest builds on setupPaletteTest by also loading the
// embedded runtime help assets, so HelpComplete (driven by the
// "help" command's completer) has real topics to enumerate, and by
// giving buffer.NewBuffer a live Lua state to run onBufferOpen
// against (mirrors internal/buffer's own test init: NewBuffer always
// evaluates luar.New(ulua.L, b) for the onBufferOpen hook, which
// segfaults against a nil ulua.L).
func setupPaletteArgTest(t *testing.T) {
	t.Helper()
	setupPaletteTest(t)
	if ulua.L == nil {
		ulua.L = lua.NewState()
	}
	config.InitRuntimeFiles(false)
}

// registerFakeCommand installs a throwaway command with a fixed-list
// completer, standing in for what config.MakeCommand-registered Lua
// commands (or the runaction-command branch's "runaction") look like
// from the palette's point of view: any Command with a non-nil,
// non-FileComplete completer. It is removed again on test cleanup so
// it cannot leak into other tests sharing the package-level commands
// map.
func registerFakeCommand(t *testing.T, name string, args []string) {
	t.Helper()
	MakeCommand(name, func(bp *BufPane, a []string) {}, func(b *buffer.Buffer) ([]string, []string) {
		return args, args
	})
	t.Cleanup(func() {
		delete(commands, name)
	})
}

func findCommandArgEntry(entries []paletteEntry, label string) *paletteEntry {
	return findPaletteEntry(entries, paletteCommandArg, label)
}

func TestPaletteCommandArgHelpEnumeratesTopics(t *testing.T) {
	setupPaletteArgTest(t)
	entries := buildPaletteEntries()

	if findCommandArgEntry(entries, "help commands") == nil {
		t.Errorf(`"help commands" arg entry missing from palette`)
	}
	if findCommandArgEntry(entries, "help options") == nil {
		t.Errorf(`"help options" arg entry missing from palette`)
	}
}

func TestPaletteCommandArgSetEnumeratesOptions(t *testing.T) {
	setupPaletteArgTest(t)
	entries := buildPaletteEntries()

	if findCommandArgEntry(entries, "set tabsize") == nil {
		t.Errorf(`"set tabsize" arg entry missing from palette`)
	}
}

func TestPaletteCommandArgPluginEnumeratesSubcommands(t *testing.T) {
	setupPaletteArgTest(t)
	entries := buildPaletteEntries()

	if findCommandArgEntry(entries, "plugin install") == nil {
		t.Errorf(`"plugin install" arg entry missing from palette`)
	}
	if findCommandArgEntry(entries, "plugin list") == nil {
		t.Errorf(`"plugin list" arg entry missing from palette`)
	}
}

// TestPaletteCommandArgLuaStyleCompleterWorks proves the synthetic-
// buffer mechanism drives a plugin-shaped completer (registered
// through MakeCommand, the same entry point Lua plugins use) exactly
// like a built-in Go completer, with zero new plugin API.
func TestPaletteCommandArgLuaStyleCompleterWorks(t *testing.T) {
	setupPaletteArgTest(t)
	registerFakeCommand(t, "palettetestfakecmd", []string{"alpha", "beta"})

	entries := buildPaletteEntries()

	if findCommandArgEntry(entries, "palettetestfakecmd alpha") == nil {
		t.Errorf(`"palettetestfakecmd alpha" arg entry missing from palette`)
	}
	if findCommandArgEntry(entries, "palettetestfakecmd beta") == nil {
		t.Errorf(`"palettetestfakecmd beta" arg entry missing from palette`)
	}
}

// TestPaletteCommandArgExcludesFileComplete proves FileComplete-based
// commands (file navigation belongs to the open-file picker, not the
// palette) produce no "<cmd> <arg>" entries, even though they do have
// a non-nil completer.
func TestPaletteCommandArgExcludesFileComplete(t *testing.T) {
	setupPaletteArgTest(t)
	entries := buildPaletteEntries()

	for _, e := range entries {
		if e.Kind != paletteCommandArg {
			continue
		}
		for _, prefix := range []string{"vsplit ", "open ", "hsplit ", "tab ", "cd "} {
			if len(e.Name) >= len(prefix) && e.Name[:len(prefix)] == prefix {
				t.Errorf("FileComplete-based command leaked an arg entry: %q", e.Name)
			}
		}
	}
}

// TestPaletteCommandArgExcludesRunaction proves the "runaction"
// command is skipped by name regardless of its completer. The
// runaction-command branch (not part of this branch's history) is
// the real source of a "runaction" command; a stand-in is registered
// here so the exclusion rule is tested independent of which feature
// branches happen to be merged locally.
func TestPaletteCommandArgExcludesRunaction(t *testing.T) {
	setupPaletteArgTest(t)
	registerFakeCommand(t, "runaction", []string{"DuplicateLine"})

	entries := buildPaletteEntries()

	if findCommandArgEntry(entries, "runaction DuplicateLine") != nil {
		t.Errorf(`"runaction DuplicateLine" arg entry present despite runaction exclusion`)
	}
}

// TestPaletteCommandArgGatedByCommandsOption proves the arg entries
// are gated by the existing commandpalette.commands option (D-21: no
// new option is introduced for this feature).
func TestPaletteCommandArgGatedByCommandsOption(t *testing.T) {
	setupPaletteArgTest(t)
	config.GlobalSettings["commandpalette.commands"] = false

	entries := buildPaletteEntries()
	for _, e := range entries {
		if e.Kind == paletteCommandArg {
			t.Errorf("arg entry %q present despite commandpalette.commands=false", e.Name)
		}
	}
}
