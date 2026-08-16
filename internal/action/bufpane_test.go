package action

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/micro-editor/micro/v2/internal/config"
	ulua "github.com/micro-editor/micro/v2/internal/lua"
	lua "github.com/yuin/gopher-lua"
)

// silenceTermMessage redirects os.Stdin to a pipe that already has a
// newline queued up, so screen.TermMessage's blocking "press enter
// to continue" prompt (used for bindings.json parse errors) returns
// immediately instead of hanging the test on a real stdin read.
func silenceTermMessage(t *testing.T) {
	t.Helper()
	origStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString("\n"); err != nil {
		t.Fatal(err)
	}
	w.Close()
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = origStdin
		r.Close()
	})
}

// registerFakeLuaPlugin defines a Lua plugin table with a single
// function that returns true and counts its own calls in the global
// "<name>_<fn>_calls", and registers it in config.Plugins so
// LuaAction/config.FindPlugin can resolve "name.fn". Removed again
// on test cleanup so it cannot leak into other tests.
func registerFakeLuaPlugin(t *testing.T, name, fn string) {
	t.Helper()
	if ulua.L == nil {
		ulua.L = lua.NewState()
	}
	counter := name + "_" + fn + "_calls"
	src := name + " = {}\n" + counter + " = 0\nfunction " + name + "." + fn + "(bp)\n  " + counter + " = " + counter + " + 1\n  return true\nend\n"
	if err := ulua.L.DoString(src); err != nil {
		t.Fatal(err)
	}
	config.Plugins = append(config.Plugins, &config.Plugin{Name: name, Loaded: true})
	t.Cleanup(func() {
		for i, p := range config.Plugins {
			if p.Name == name {
				config.Plugins = append(config.Plugins[:i], config.Plugins[i+1:]...)
				break
			}
		}
	})
}

// TestParseBufActionChain covers the parsing outcomes that
// BufMapEvent relied on inline before the parser was extracted into
// parseBufActionChain: plain atoms, the &|, separators, quoted
// separators, and the command:/command-edit: prefixes. None of
// these atoms execute anything at parse time (the returned
// actionfns are unevaluated closures), so no BufPane is needed.
func TestParseBufActionChain(t *testing.T) {
	cases := []struct {
		name       string
		action     string
		wantNames  []string
		wantTypes  []byte
		wantFnsLen int
	}{
		{
			name:       "single action",
			action:     "CursorEnd",
			wantNames:  []string{"CursorEnd"},
			wantTypes:  []byte{' '},
			wantFnsLen: 1,
		},
		{
			name:       "comma chain",
			action:     "CursorEnd,CursorStart",
			wantNames:  []string{"CursorEnd", "CursorStart"},
			wantTypes:  []byte{',', ' '},
			wantFnsLen: 2,
		},
		{
			name:       "pipe chain",
			action:     "CursorEnd|CursorStart",
			wantNames:  []string{"CursorEnd", "CursorStart"},
			wantTypes:  []byte{'|', ' '},
			wantFnsLen: 2,
		},
		{
			name:       "ampersand chain",
			action:     "CursorEnd&CursorStart",
			wantNames:  []string{"CursorEnd", "CursorStart"},
			wantTypes:  []byte{'&', ' '},
			wantFnsLen: 2,
		},
		{
			// The comma is inside quotes, so IndexAnyUnquoted must not
			// treat it as a chain separator: the whole string is one
			// command: atom.
			name:       "quoted comma is not a chain separator",
			action:     `command:set divchars ","`,
			wantNames:  []string{""},
			wantTypes:  []byte{' '},
			wantFnsLen: 1,
		},
		{
			name:       "command prefix",
			action:     "command:save",
			wantNames:  []string{""},
			wantTypes:  []byte{' '},
			wantFnsLen: 1,
		},
		{
			name:       "command-edit prefix",
			action:     "command-edit:save",
			wantNames:  []string{""},
			wantTypes:  []byte{' '},
			wantFnsLen: 1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actionfns, names, types := parseBufActionChain(c.action, KeyEvent{})
			if len(actionfns) != c.wantFnsLen {
				t.Errorf("actionfns length = %d, want %d", len(actionfns), c.wantFnsLen)
			}
			if !reflect.DeepEqual(names, c.wantNames) {
				t.Errorf("names = %#v, want %#v", names, c.wantNames)
			}
			if !reflect.DeepEqual(types, c.wantTypes) {
				t.Errorf("types = %#v, want %#v", types, c.wantTypes)
			}
		})
	}
}

// TestParseBufActionChainLuaAtom proves a resolvable lua: atom
// produces a working BufKeyAction and the title-cased hook name
// BufMapEvent's pre/on PluginCB convention expects.
func TestParseBufActionChainLuaAtom(t *testing.T) {
	registerFakeLuaPlugin(t, "bufpanetestplugin", "dostuff")

	actionfns, names, types := parseBufActionChain("lua:bufpanetestplugin.dostuff", KeyEvent{})

	if len(actionfns) != 1 {
		t.Fatalf("actionfns length = %d, want 1", len(actionfns))
	}
	if _, ok := actionfns[0].(BufKeyAction); !ok {
		t.Errorf("actionfns[0] = %T, want BufKeyAction", actionfns[0])
	}
	wantName := strings.Title("bufpanetestplugin") + strings.Title("dostuff")
	if !reflect.DeepEqual(names, []string{wantName}) {
		t.Errorf("names = %#v, want %#v", names, []string{wantName})
	}
	if !reflect.DeepEqual(types, []byte{' '}) {
		t.Errorf("types = %#v, want %#v", types, []byte{' '})
	}
}

// TestParseBufActionChainUnknownAtom proves an atom that resolves in
// neither BufKeyActions nor BufMouseActions is skipped: it reports
// through screen.TermMessage (the same path an unresolvable
// bindings.json entry takes) and contributes no actionfn/name. The
// separator for the failed atom is still recorded, a pre-existing
// quirk of the loop this test protects: types is appended to before
// the resolve/error branch runs, so it can end up longer than
// actionfns/names when an atom fails to resolve.
func TestParseBufActionChainUnknownAtom(t *testing.T) {
	silenceTermMessage(t)

	actionfns, names, types := parseBufActionChain("ThisActionDoesNotExist", KeyEvent{})

	if len(actionfns) != 0 {
		t.Errorf("actionfns length = %d, want 0 for an unknown atom", len(actionfns))
	}
	if len(names) != 0 {
		t.Errorf("names length = %d, want 0 for an unknown atom", len(names))
	}
	if !reflect.DeepEqual(types, []byte{' '}) {
		t.Errorf("types = %#v, want %#v", types, []byte{' '})
	}
}
