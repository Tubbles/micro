package action

import (
	"sort"
	"strings"

	"github.com/micro-editor/micro/v2/internal/config"
	ulua "github.com/micro-editor/micro/v2/internal/lua"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/micro/v2/internal/widget"
	lua "github.com/yuin/gopher-lua"
)

// paletteKind discriminates the three categories the command palette
// surfaces: built-in buffer actions, commands, and Lua plugin
// functions.
type paletteKind int

const (
	paletteAction paletteKind = iota
	paletteCommand
	paletteLua
)

// paletteEntry is one row in the command palette. Bindings is the
// list of key event names from config.Bindings["buffer"] that target
// this entry's action string. v1 only matches by exact equality of
// the action string, so an entry bound only via a chain expression
// like "Save,Quit" surfaces with an empty Bindings list.
type paletteEntry struct {
	Kind     paletteKind
	Name     string
	Bindings []string
}

// actionString is the form a key binding would use to invoke this
// entry. Used both as the reverse-binding lookup key and as the
// dispatch shape consumed by BufMapEvent's parser.
func (e paletteEntry) actionString() string {
	switch e.Kind {
	case paletteCommand:
		return "command:" + e.Name
	case paletteLua:
		return "lua:" + e.Name
	}
	return e.Name
}

// buildPaletteEntries produces the full list of palette entries,
// sorted within each kind. The reverse-binding map is built once and
// shared across all three enumeration steps.
func buildPaletteEntries() []paletteEntry {
	rev := buildBindingReverseMap()
	var out []paletteEntry

	if isPaletteEnabled("commandpalette.actions") {
		var names []string
		for name := range BufKeyActions {
			if _, isMouse := BufMouseActions[name]; isMouse {
				continue
			}
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			out = append(out, paletteEntry{
				Kind:     paletteAction,
				Name:     name,
				Bindings: rev[name],
			})
		}
	}

	if isPaletteEnabled("commandpalette.commands") {
		var names []string
		for name := range commands {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			out = append(out, paletteEntry{
				Kind:     paletteCommand,
				Name:     name,
				Bindings: rev["command:"+name],
			})
		}
	}

	if isPaletteEnabled("commandpalette.lua") {
		out = append(out, buildLuaPaletteEntries(rev)...)
	}

	return out
}

// buildBindingReverseMap walks the buffer bindings and returns a map
// from action string to the event names targeting it. Each value
// list is sorted so palette entries render deterministically.
func buildBindingReverseMap() map[string][]string {
	rev := map[string][]string{}
	for ev, action := range config.Bindings["buffer"] {
		rev[action] = append(rev[action], ev)
	}
	for _, evs := range rev {
		sort.Strings(evs)
	}
	return rev
}

// buildLuaPaletteEntries enumerates the loaded plugin tables and
// surfaces every Lua-defined function as a paletteLua entry. The
// "Lua-defined" filter keeps Proto != nil: gopher-lua marks
// Lua-source closures with a non-nil compiled Proto, while functions
// bound from Go via luar leave Proto nil. package.seeall delegates
// stdlib lookups through the metatable __index, so stdlib never
// appears in ForEach over the module table itself.
func buildLuaPaletteEntries(rev map[string][]string) []paletteEntry {
	if ulua.L == nil {
		return nil
	}
	var out []paletteEntry
	for _, p := range config.Plugins {
		if !p.IsLoaded() {
			continue
		}
		g := ulua.L.GetGlobal(p.Name)
		tbl, ok := g.(*lua.LTable)
		if !ok {
			continue
		}
		var fns []string
		tbl.ForEach(func(k, v lua.LValue) {
			ks, kok := k.(lua.LString)
			if !kok {
				return
			}
			f, vok := v.(*lua.LFunction)
			if !vok || f.Proto == nil {
				return
			}
			fns = append(fns, string(ks))
		})
		sort.Strings(fns)
		for _, fn := range fns {
			full := p.Name + "." + fn
			out = append(out, paletteEntry{
				Kind:     paletteLua,
				Name:     full,
				Bindings: rev["lua:"+full],
			})
		}
	}
	return out
}

func isPaletteEnabled(opt string) bool {
	v, ok := config.GlobalSettings[opt]
	if !ok {
		return true
	}
	b, _ := v.(bool)
	return b
}

// paletteKindTag returns the short prefix rendered on each palette
// row. Trailing spaces line entries up across kinds for readability.
func paletteKindTag(k paletteKind) string {
	switch k {
	case paletteAction:
		return "action "
	case paletteCommand:
		return "cmd    "
	case paletteLua:
		return "lua    "
	}
	return ""
}

// paletteItemLabel formats a paletteEntry as a picker row label.
// Layout: "<kindTag><Name> [<bindings>]". The bindings group is
// omitted when empty. Keystrokes are packed into the label so the
// fuzzy filter can match queries like "ctrl-shift-x" against the
// bound entries.
func paletteItemLabel(e paletteEntry) string {
	s := paletteKindTag(e.Kind) + e.Name
	if len(e.Bindings) > 0 {
		s += " [" + strings.Join(e.Bindings, ", ") + "]"
	}
	return s
}

// paletteItems wraps the entries into the picker's row type.
func paletteItems(entries []paletteEntry) []widget.PickerItem {
	items := make([]widget.PickerItem, len(entries))
	for i, e := range entries {
		items[i].Label = paletteItemLabel(e)
	}
	return items
}

// commandPaletteRect computes the on-screen rect for the palette
// overlay, leaving a 2-cell margin around the editor area and
// accounting for the tab bar (when more than one tab is open) and
// the info bar.
func commandPaletteRect() widget.ScreenRect {
	sw, sh := screen.Screen.Size()
	iOff := config.GetInfoBarOffset()
	tabBar := 0
	if Tabs != nil && len(Tabs.List) > 1 {
		tabBar = 1
	}
	const margin = 2
	x := margin
	y := tabBar + margin
	w := sw - 2*margin
	h := (sh - tabBar - iOff) - 2*margin
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return widget.ScreenRect{X: x, Y: y, W: w, H: h}
}

// CommandPalette opens the command palette overlay. No default key
// binding ships. Users invoke it from the command bar by typing
// commandpalette, or by binding command:commandpalette themselves.
func (h *BufPane) CommandPalette() {
	entries := buildPaletteEntries()
	items := paletteItems(entries)
	picker := widget.NewPicker(widget.PickerOptions{
		Title:    "Command palette",
		Hint:     "<type> filter - <Up>/<Down> move - <Enter> run - <Esc> cancel",
		Query:    true,
		Items:    items,
		Geometry: widget.Geometry{Kind: widget.GeomScreenRect, Rect: commandPaletteRect()},
		OnSelect: func(idx int) {
			widget.CloseActive()
			if idx < 0 || idx >= len(entries) {
				return
			}
			executePaletteEntry(h, entries[idx])
		},
	})
	widget.Open(picker)
}

// CommandPaletteCmd is the command-bar entry point. Args are
// ignored.
func (h *BufPane) CommandPaletteCmd(args []string) {
	h.CommandPalette()
}

// executePaletteEntry dispatches the selected entry through the
// same code paths a real keystroke or command-bar invocation would,
// so plugin pre/on hooks fire and macros record consistently.
//
// The action-kind branch duplicates the MultiActions per-cursor
// loop from BufMapEvent (mirrored by RunActionCmd on the
// runaction-command branch, commit 6ea16cf1). When both branches
// land in integration this body and that one should be folded into
// a shared helper.
func executePaletteEntry(h *BufPane, e paletteEntry) {
	switch e.Kind {
	case paletteAction:
		fn, ok := BufKeyActions[e.Name]
		if !ok {
			return
		}
		if _, multi := MultiActions[e.Name]; multi {
			for _, c := range h.Buf.GetCursors() {
				h.Buf.SetCurCursor(c.Num)
				h.Cursor = c
				h.execAction(fn, e.Name, nil)
			}
		} else {
			h.Buf.SetCurCursor(0)
			h.Cursor = h.Buf.GetActiveCursor()
			h.execAction(fn, e.Name, nil)
		}
	case paletteCommand:
		h.HandleCommand(e.Name)
	case paletteLua:
		a := LuaAction(e.Name, KeyEvent{})
		fn, ok := a.(BufKeyAction)
		if !ok || fn == nil {
			return
		}
		// Match BufMapEvent's hook-name convention for `lua:` bindings:
		// title-case the plugin and function names so the pre/on hook
		// keys mirror what plugins receive when invoked from a real
		// keybinding.
		split := strings.SplitN(e.Name, ".", 2)
		var hookName string
		if len(split) > 1 {
			hookName = strings.Title(split[0]) + strings.Title(split[1])
		} else {
			hookName = strings.Title(e.Name)
		}
		h.execAction(fn, hookName, nil)
	}
}
