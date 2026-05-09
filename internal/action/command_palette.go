package action

import (
	"sort"

	"github.com/micro-editor/micro/v2/internal/config"
	ulua "github.com/micro-editor/micro/v2/internal/lua"
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
