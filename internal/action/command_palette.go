package action

import (
	"sort"
	"strings"

	"github.com/micro-editor/micro/v2/internal/config"
	ulua "github.com/micro-editor/micro/v2/internal/lua"
	"github.com/micro-editor/micro/v2/internal/widget"
	lua "github.com/yuin/gopher-lua"
)

// paletteKind discriminates the four categories the command palette
// surfaces: built-in buffer actions, commands, Lua plugin functions,
// and one-argument-level command invocations (D-20/D-21, e.g.
// "help options").
type paletteKind int

const (
	paletteAction paletteKind = iota
	paletteCommand
	paletteLua
	paletteCommandArg
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
	case paletteCommand, paletteCommandArg:
		return "command:" + e.Name
	case paletteLua:
		return "lua:" + e.Name
	}
	return e.Name
}

// paletteEntryByKindName looks up an entry by (Kind, Name) in a
// freshly-built atlas list, so a history entry can reuse the atlas
// label (including the bindings column). Returns (zero, false) when
// the entry is no longer in the atlas (e.g. a plugin uninstalled
// mid-session); callers fall back to a synthetic label.
func paletteEntryByKindName(entries []paletteEntry, k paletteKind, name string) (paletteEntry, bool) {
	for _, e := range entries {
		if e.Kind == k && e.Name == name {
			return e, true
		}
	}
	return paletteEntry{}, false
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
		out = append(out, buildCommandArgPaletteEntries(rev)...)
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
	case paletteCommandArg:
		return "arg    "
	}
	return ""
}

// historyKindTag is the per-kind prefix for a history row. The free-
// text variant gets its own "text" tag so a typed `saveas foo.txt`
// row is visually distinct from a registered `saveas` command row.
func historyKindTag(k historyKind) string {
	switch k {
	case historyAction:
		return "action "
	case historyCommand:
		return "cmd    "
	case historyLua:
		return "lua    "
	case historyFreeText:
		return "text   "
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

// historyItemLabel formats a single history row. For action/command/
// lua entries that still exist in the fresh atlas, the atlas label
// (including the bindings column) is reused so the row looks the
// same in both views. When the atlas no longer carries the entry (a
// plugin unloaded mid-session, say) a bare "<kind> <name>" fallback
// is used. Free-text entries always use the verbatim typed string.
func historyItemLabel(atlas []paletteEntry, hist historyEntry) string {
	switch hist.Kind {
	case historyFreeText:
		return historyKindTag(hist.Kind) + hist.Name
	case historyAction:
		if e, ok := paletteEntryByKindName(atlas, paletteAction, hist.Name); ok {
			return paletteItemLabel(e)
		}
	case historyCommand:
		if e, ok := paletteEntryByKindName(atlas, paletteCommand, hist.Name); ok {
			return paletteItemLabel(e)
		}
	case historyLua:
		if e, ok := paletteEntryByKindName(atlas, paletteLua, hist.Name); ok {
			return paletteItemLabel(e)
		}
	}
	return historyKindTag(hist.Kind) + hist.Name
}

// historyPaletteItems is paletteItems for history rows. The atlas
// is passed in so labels stay in sync with the registered bindings.
func historyPaletteItems(atlas []paletteEntry, hist []historyEntry) []widget.PickerItem {
	items := make([]widget.PickerItem, len(hist))
	for i, h := range hist {
		items[i].Label = historyItemLabel(atlas, h)
	}
	return items
}

// paletteMode discriminates the two views the picker can show. Atlas
// is the registered-action catalog; History is the per-session
// most-recent-first list of items previously dispatched through this
// palette.
type paletteMode int

const (
	paletteModeAtlas paletteMode = iota
	paletteModeHistory
)

func paletteTitle(m paletteMode) string {
	if m == paletteModeHistory {
		return "Command palette · History"
	}
	return "Command palette · Atlas"
}

// paletteHint reflects both the current mode and whether history is
// available at all. When historysize is 0 the Tab toggle is dropped
// from the hint (and from the OnTab handler).
func paletteHint(m paletteMode, size int) string {
	switch {
	case m == paletteModeHistory:
		return "<Tab> Atlas - <Up>/<Down> move - <Enter> rerun - <Ctrl-Enter> run as command - <Esc> cancel"
	case size > 0:
		return "<Tab> History - <Up>/<Down> move - <Enter> run match - <Ctrl-Enter> run as command - <Esc> cancel"
	}
	return "<Up>/<Down> move - <Enter> run match - <Ctrl-Enter> run as command - <Esc> cancel"
}

// historyToPaletteKind maps the dispatchable subset of historyKind
// onto paletteKind for re-running. historyFreeText has no paletteKind
// counterpart; callers must handle it on the HandleCommand path.
func historyToPaletteKind(k historyKind) paletteKind {
	switch k {
	case historyCommand:
		return paletteCommand
	case historyLua:
		return paletteLua
	}
	return paletteAction
}

// CommandPalette opens the command palette overlay. No default key
// binding ships. Users invoke it from the command bar by typing
// commandpalette, or by binding command:commandpalette themselves.
//
// The picker has two modes. Atlas (the original behaviour) lists
// every registered action, command, and Lua plugin function. History
// lists the items the user has dispatched through this palette in
// this session, most-recent-first. Tab toggles between the two. On
// open the picker starts in History when history is non-empty, else
// in Atlas. With commandpalette.historysize == 0 the History mode is
// disabled entirely: Tab is a no-op, nothing is recorded.
//
// Enter behaviour (unchanged from the Atlas-only era):
//   - With matches in the filtered list, Enter runs the highlighted
//     entry via executePaletteEntry (action / command / lua) in
//     Atlas, or re-dispatches the recorded entry in History.
//   - With a non-empty query and zero matches, Enter falls through
//     to OnSubmit, which dispatches the typed text as a command
//     line via HandleCommand and records it as a free-text entry.
//
// The palette wires no OnSelectCtrl/OnSubmitCtrl, so Ctrl-Enter routes
// through the picker's shared hook model to the plain OnSelect/OnSubmit
// path, i.e. it behaves like Enter here (the picker's Ctrl variant is
// used by consumers like the options picker that need a second commit
// mode).
func (h *BufPane) CommandPalette() {
	atlas := buildPaletteEntries()
	atlasItems := paletteItems(atlas)
	hist := recentHistory()
	histItems := historyPaletteItems(atlas, hist)
	size := historySize()

	mode := paletteModeAtlas
	if size > 0 && len(hist) > 0 {
		mode = paletteModeHistory
	}

	itemsFor := func(m paletteMode) []widget.PickerItem {
		if m == paletteModeHistory {
			return histItems
		}
		return atlasItems
	}

	dispatchHistory := func(idx int) {
		if idx < 0 || idx >= len(hist) {
			return
		}
		e := hist[idx]
		if e.Kind == historyFreeText {
			recordHistory(e)
			h.HandleCommand(e.Name)
			return
		}
		pk := historyToPaletteKind(e.Kind)
		pe, ok := paletteEntryByKindName(atlas, pk, e.Name)
		if !ok {
			pe = paletteEntry{Kind: pk, Name: e.Name}
		}
		executePaletteEntry(h, pe)
	}

	var picker *widget.Picker
	picker = widget.NewPicker(widget.PickerOptions{
		Title:    paletteTitle(mode),
		Hint:     paletteHint(mode, size),
		Query:    true,
		Items:    itemsFor(mode),
		Geometry: widget.Geometry{Kind: widget.GeomScreenRect, Rect: widgetOverlayRect()},
		OnSelect: func(idx int) {
			widget.CloseActive()
			if mode == paletteModeHistory {
				dispatchHistory(idx)
				return
			}
			if idx < 0 || idx >= len(atlas) {
				return
			}
			executePaletteEntry(h, atlas[idx])
		},
		OnSubmit: func(query string) {
			widget.CloseActive()
			recordHistory(historyEntry{Kind: historyFreeText, Name: query})
			h.HandleCommand(query)
		},
		OnTab: func() {
			if size <= 0 {
				return
			}
			if mode == paletteModeAtlas {
				mode = paletteModeHistory
			} else {
				mode = paletteModeAtlas
			}
			// RefreshItems, not SetItems: the typed filter should keep
			// narrowing whichever list the user toggles to.
			picker.RefreshItems(itemsFor(mode))
			picker.SetTitle(paletteTitle(mode))
			picker.SetHint(paletteHint(mode, size))
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
// The action-kind branch shares its MultiActions per-cursor dispatch
// with RunActionCmd via runBufActionByName.
func executePaletteEntry(h *BufPane, e paletteEntry) {
	switch e.Kind {
	case paletteAction:
		if _, ok := BufKeyActions[e.Name]; !ok {
			return
		}
		recordHistory(historyEntry{Kind: historyAction, Name: e.Name})
		runBufActionByName(h, e.Name)
	case paletteCommand:
		recordHistory(historyEntry{Kind: historyCommand, Name: e.Name})
		h.HandleCommand(e.Name)
	case paletteCommandArg:
		// The whole "<cmd> <arg>" label is dispatched verbatim through
		// the command bar, same as a free-text entry the user typed
		// themselves; there is no single-word command name to key a
		// historyCommand entry on.
		recordHistory(historyEntry{Kind: historyFreeText, Name: e.Name})
		h.HandleCommand(e.Name)
	case paletteLua:
		a := LuaAction(e.Name, KeyEvent{})
		fn, ok := a.(BufKeyAction)
		if !ok || fn == nil {
			return
		}
		recordHistory(historyEntry{Kind: historyLua, Name: e.Name})
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
