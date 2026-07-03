package action

import (
	"testing"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/display"
	"github.com/micro-editor/micro/v2/internal/info"
	ulua "github.com/micro-editor/micro/v2/internal/lua"
	lua "github.com/yuin/gopher-lua"
)

// setupRunActionTest builds a minimal, fully functional *BufPane
// (buffer + window + cursor, no tab) so RunActionCmd can dispatch
// real actions without a live terminal. It mirrors the setup
// internal/buffer's own tests use (ulua.L and config.GlobalSettings
// must be ready before buffer.NewBufferFromString runs its
// onBufferOpen hook), and additionally wires up InfoBar without
// going through display.NewInfoWindow, which requires a live
// screen.Screen for its Size() call.
func setupRunActionTest(t *testing.T) *BufPane {
	t.Helper()
	if ulua.L == nil {
		ulua.L = lua.NewState()
	}
	if config.GlobalSettings == nil {
		config.GlobalSettings = config.DefaultAllSettings()
	}
	if InfoBar == nil {
		InfoBar = &InfoPane{InfoBuf: info.NewBuffer()}
	}

	buf := buffer.NewBufferFromString("hello world", "", buffer.BTDefault)
	win := display.NewBufWindow(0, 0, 80, 24, buf)
	return newBufPane(buf, win, nil)
}

func TestRunActionCmdChain(t *testing.T) {
	h := setupRunActionTest(t)

	// CursorEnd,CursorStart: both atoms run regardless of outcome
	// (',' never short-circuits), so the cursor should end up back
	// at the buffer start.
	h.RunActionCmd([]string{"CursorEnd,CursorStart"})

	if h.Cursor.Loc != (buffer.Loc{X: 0, Y: 0}) {
		t.Errorf("Cursor.Loc = %v, want {0 0}", h.Cursor.Loc)
	}
}

func TestRunActionCmdLuaAtom(t *testing.T) {
	h := setupRunActionTest(t)
	registerFakeLuaPlugin(t, "runactiontestplugin", "ping")

	h.RunActionCmd([]string{"lua:runactiontestplugin.ping"})

	v := ulua.L.GetGlobal("runactiontestplugin_ping_calls")
	n, ok := v.(lua.LNumber)
	if !ok || n != 1 {
		t.Errorf("runactiontestplugin_ping_calls = %v, want 1", v)
	}
}

func TestRunActionCmdRejectsBareCommandAtom(t *testing.T) {
	h := setupRunActionTest(t)
	h.Cursor.Loc = h.Buf.End()
	before := h.Cursor.Loc

	h.RunActionCmd([]string{"command:save"})

	if h.Cursor.Loc != before {
		t.Errorf("Cursor.Loc changed to %v after a rejected command: atom, want unchanged %v", h.Cursor.Loc, before)
	}
	if !InfoBar.HasError {
		t.Errorf("InfoBar.HasError = false, want true after a rejected command: atom")
	}
}

func TestChainHasBareCommandAtom(t *testing.T) {
	cases := []struct {
		name   string
		action string
		want   bool
	}{
		{"bare command atom", "command:save", true},
		{"bare command atom in a chain", "CursorEnd,command:save", true},
		{"bare command atom with pipe", "command:save|CursorEnd", true},
		{"command-edit is not a bare command atom", "command-edit:save", false},
		{"plain chain has no command atom", "CursorEnd,CursorStart", false},
		{"lua atom has no command atom", "lua:plug.fn", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := chainHasBareCommandAtom(c.action); got != c.want {
				t.Errorf("chainHasBareCommandAtom(%q) = %v, want %v", c.action, got, c.want)
			}
		})
	}
}
