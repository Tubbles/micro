package action

import (
	"testing"

	"github.com/micro-editor/micro/v2/internal/buffer"
	ulua "github.com/micro-editor/micro/v2/internal/lua"
	lua "github.com/yuin/gopher-lua"
)

func TestPaletteFreeTextResolvesAsActionChain(t *testing.T) {
	registerFakeLuaPlugin(t, "freetexttestplugin", "ping")

	cases := []struct {
		name  string
		query string
		want  bool
	}{
		{"plain action", "CursorEnd", true},
		{"comma chain of actions", "CursorEnd,CursorStart", true},
		{"lua atom", "lua:freetexttestplugin.ping", true},
		{"command atom always resolves", "command:save", true},
		{"real command with an argument is not a chain (no space splitting)", "help options", false},
		{"gibberish single word", "thisIsNotAnything", false},
		{"empty query", "", false},
		{"blank query", "   ", false},
		{"mouse-only atom does not resolve", "MousePress", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := paletteFreeTextResolvesAsActionChain(c.query); got != c.want {
				t.Errorf("paletteFreeTextResolvesAsActionChain(%q) = %v, want %v", c.query, got, c.want)
			}
		})
	}
}

func TestRunFreeTextActionChainDispatchesResolvableChain(t *testing.T) {
	h := setupRunActionTest(t)

	ran := runFreeTextActionChain(h, "CursorEnd,CursorStart")

	if !ran {
		t.Fatalf("runFreeTextActionChain returned false for a resolvable chain")
	}
	if h.Cursor.Loc != (buffer.Loc{X: 0, Y: 0}) {
		t.Errorf("Cursor.Loc = %v, want {0 0}", h.Cursor.Loc)
	}
}

func TestRunFreeTextActionChainDispatchesLuaAtom(t *testing.T) {
	h := setupRunActionTest(t)
	registerFakeLuaPlugin(t, "freetextluadispatch", "ping")

	ran := runFreeTextActionChain(h, "lua:freetextluadispatch.ping")

	if !ran {
		t.Fatalf("runFreeTextActionChain returned false for a resolvable lua atom")
	}
	v := ulua.L.GetGlobal("freetextluadispatch_ping_calls")
	n, ok := v.(lua.LNumber)
	if !ok || n != 1 {
		t.Errorf("freetextluadispatch_ping_calls = %v, want 1", v)
	}
}

// TestRunFreeTextActionChainFallsThroughForCommands proves ordinary
// command text (with or without arguments) is left untouched: it is
// not treated as an action chain, so OnSubmit's HandleCommand
// fallback still runs it, and the buffer's cursor is unaffected by
// runFreeTextActionChain itself.
func TestRunFreeTextActionChainFallsThroughForCommands(t *testing.T) {
	h := setupRunActionTest(t)

	cases := []string{"help options", "thisIsNotAnything", "quit", ""}
	for _, query := range cases {
		t.Run(query, func(t *testing.T) {
			before := h.Cursor.Loc
			if ran := runFreeTextActionChain(h, query); ran {
				t.Errorf("runFreeTextActionChain(%q) = true, want false (should fall through to HandleCommand)", query)
			}
			if h.Cursor.Loc != before {
				t.Errorf("Cursor.Loc changed to %v after a non-dispatching call, want unchanged %v", h.Cursor.Loc, before)
			}
		})
	}
}
