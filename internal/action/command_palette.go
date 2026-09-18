package action

import (
	"sort"
	"strings"

	"github.com/micro-editor/micro/v2/internal/config"
	ulua "github.com/micro-editor/micro/v2/internal/lua"
	"github.com/micro-editor/micro/v2/internal/widget"
	lua "github.com/yuin/gopher-lua"
)

// paletteKind discriminates the categories the command palette
// surfaces: built-in buffer actions, commands, Lua plugin functions,
// one-argument-level command invocations (D-20/D-21, e.g.
// "help options"), key bindings whose target is none of the above (a
// command with its own arguments, a command-edit: prefix, or an action
// chain), and free-text command lines the user typed into the palette
// earlier, which only ever appear among the recent rows.
type paletteKind int

const (
	paletteAction paletteKind = iota
	paletteCommand
	paletteLua
	paletteCommandArg
	paletteBinding
	paletteFreeText
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
// dispatch shape consumed by BufMapEvent's parser. A paletteBinding
// entry's Name already is that form, verbatim from bindings.json.
func (e paletteEntry) actionString() string {
	switch e.Kind {
	case paletteCommand, paletteCommandArg:
		return "command:" + e.Name
	case paletteLua:
		return "lua:" + e.Name
	}
	return e.Name
}

// paletteEntryIndex returns the position of the entry with the given
// (Kind, Name) in entries, or -1 when the atlas no longer has it (a
// plugin unloaded mid-session, say).
func paletteEntryIndex(entries []paletteEntry, k paletteKind, name string) int {
	for i, e := range entries {
		if e.Kind == k && e.Name == name {
			return i
		}
	}
	return -1
}

// buildPaletteEntries produces the full list of palette entries,
// sorted within each kind. The reverse-binding map is built once and
// shared across all enumeration steps. The binding kind lists what is
// left over once the other kinds have claimed their targets, so those
// lists are built whenever bindings are enabled even if their own
// setting hides them from the palette; the command list is the only
// one that costs anything (it drives every completer once).
func buildPaletteEntries() []paletteEntry {
	rev := buildBindingReverseMap()
	showActions := isPaletteEnabled("commandpalette.actions")
	showCommands := isPaletteEnabled("commandpalette.commands")
	showLua := isPaletteEnabled("commandpalette.lua")
	showBindings := isPaletteEnabled("commandpalette.bindings")

	actions := buildActionPaletteEntries(rev)
	var cmds []paletteEntry
	if showCommands || showBindings {
		cmds = buildCommandPaletteEntries(rev)
	}
	luas := buildLuaPaletteEntries(rev)

	var out []paletteEntry
	if showActions {
		out = append(out, actions...)
	}
	if showCommands {
		out = append(out, cmds...)
	}
	if showLua {
		out = append(out, luas...)
	}
	if showBindings {
		out = append(out, buildBindingPaletteEntries(rev, actions, cmds, luas)...)
	}
	return out
}

// buildActionPaletteEntries lists every key action (mouse actions
// have no keystroke shape to dispatch from the palette).
func buildActionPaletteEntries(rev map[string][]string) []paletteEntry {
	var names []string
	for name := range BufKeyActions {
		if _, isMouse := BufMouseActions[name]; isMouse {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]paletteEntry, 0, len(names))
	for _, name := range names {
		out = append(out, paletteEntry{
			Kind:     paletteAction,
			Name:     name,
			Bindings: rev[name],
		})
	}
	return out
}

// buildCommandPaletteEntries lists every registered command followed
// by the one-argument-level entries its completer can enumerate.
func buildCommandPaletteEntries(rev map[string][]string) []paletteEntry {
	var names []string
	for name := range commands {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]paletteEntry, 0, len(names))
	for _, name := range names {
		out = append(out, paletteEntry{
			Kind:     paletteCommand,
			Name:     name,
			Bindings: rev["command:"+name],
		})
	}
	return append(out, buildCommandArgPaletteEntries(rev)...)
}

// buildBindingPaletteEntries surfaces the bindings whose target is not
// itself a palette entry: a command carrying arguments the completer
// enumeration cannot produce ("command:tab ~/notes.md"), a
// command-edit: prefix, or an action chain ("Save,Quit"). Each distinct
// target becomes one row labelled with the target verbatim, so a search
// for a key combo finds it like any other entry. A target only gets a
// row if it resolves as an action chain, which drops mouse actions (the
// palette has no mouse event to run them with) and targets that never
// resolved when bindings.json was loaded. Targets bound only to mouse
// events are skipped for the same reason.
func buildBindingPaletteEntries(rev map[string][]string, claimedBy ...[]paletteEntry) []paletteEntry {
	claimed := map[string]bool{}
	for _, list := range claimedBy {
		for _, e := range list {
			claimed[e.actionString()] = true
		}
	}

	var targets []string
	for target := range rev {
		if claimed[target] || !paletteFreeTextResolvesAsActionChain(target) {
			continue
		}
		if len(keyEventsOnly(rev[target])) > 0 {
			targets = append(targets, target)
		}
	}
	sort.Strings(targets)

	out := make([]paletteEntry, 0, len(targets))
	for _, target := range targets {
		out = append(out, paletteEntry{
			Kind:     paletteBinding,
			Name:     target,
			Bindings: keyEventsOnly(rev[target]),
		})
	}
	return out
}

// keyEventsOnly drops mouse event names from a list of binding event
// names, keeping the order.
func keyEventsOnly(events []string) []string {
	var keys []string
	for _, ev := range events {
		if e, err := findEvent(ev); err == nil {
			if _, isMouse := e.(MouseEvent); isMouse {
				continue
			}
		}
		keys = append(keys, ev)
	}
	return keys
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
	case paletteBinding:
		return "bind   "
	case paletteFreeText:
		// A typed `saveas foo.txt` row stays visually distinct from the
		// registered `saveas` command row.
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

// paletteHint is the key legend under the list.
const paletteHint = "<Up>/<Down> move - <Enter> run match - <Ctrl-Enter> run as command - <Esc> cancel"

// historyToPaletteKind maps a history kind onto the palette kind whose
// row re-runs it.
func historyToPaletteKind(k historyKind) paletteKind {
	switch k {
	case historyCommand:
		return paletteCommand
	case historyLua:
		return paletteLua
	case historyBinding:
		return paletteBinding
	case historyFreeText:
		return paletteFreeText
	}
	return paletteAction
}

// orderRecentFirst returns atlas with the entries the user most recently
// ran through the palette moved to the front, most recent first, and
// everything else after them in atlas order. A history entry whose row
// is no longer in the atlas is dropped. A free-text entry reuses the
// "<cmd> <arg>" row when the atlas enumerates that exact line, and
// otherwise becomes a text row of its own, since the atlas never lists
// typed command lines. The second result is how many leading rows are
// recent, which the picker uses to keep them ahead of other matches
// while the user filters.
func orderRecentFirst(atlas []paletteEntry, hist []historyEntry) ([]paletteEntry, int) {
	var recent []paletteEntry
	taken := map[int]bool{}
	for _, h := range hist {
		index := paletteEntryIndex(atlas, historyToPaletteKind(h.Kind), h.Name)
		if index < 0 && h.Kind == historyFreeText {
			index = paletteEntryIndex(atlas, paletteCommandArg, h.Name)
		}
		switch {
		case index >= 0 && !taken[index]:
			recent = append(recent, atlas[index])
			taken[index] = true
		case index < 0 && h.Kind == historyFreeText:
			recent = append(recent, paletteEntry{Kind: paletteFreeText, Name: h.Name})
		}
	}
	out := make([]paletteEntry, 0, len(atlas)+len(recent))
	out = append(out, recent...)
	for i, e := range atlas {
		if !taken[i] {
			out = append(out, e)
		}
	}
	return out, len(recent)
}

// CommandPalette opens the command palette overlay. No default key
// binding ships. Users invoke it from the command bar by typing
// commandpalette, or by binding command:commandpalette themselves.
//
// The list is the full catalog of actions, commands, Lua plugin
// functions, and bindings, with the entries the user ran most recently
// through this palette moved to the top, most recent first. Those
// recent rows stay ahead of other matches while filtering, so the last
// thing run is one Enter away as long as it matches what was typed.
// With commandpalette.historysize == 0 nothing is recorded and the
// list is plain catalog order.
//
// Enter behaviour:
//   - With matches in the filtered list, Enter runs the highlighted
//     entry via executePaletteEntry.
//   - With a non-empty query and zero matches, Enter falls through
//     to OnSubmit, which runs the typed text as an action chain when
//     it resolves as one and as a command line otherwise, and records
//     it as a free-text entry so it shows up among the recent rows.
//
// The palette wires no OnSelectCtrl/OnSubmitCtrl, so Ctrl-Enter routes
// through the picker's shared hook model to the plain OnSelect/OnSubmit
// path, i.e. it behaves like Enter here (the picker's Ctrl variant is
// used by consumers like the options picker that need a second commit
// mode).
func (h *BufPane) CommandPalette() {
	entries, recent := orderRecentFirst(buildPaletteEntries(), recentHistory())

	picker := widget.NewPicker(widget.PickerOptions{
		Title:    "Command palette",
		Hint:     paletteHint,
		Query:    true,
		Items:    paletteItems(entries),
		Pinned:   recent,
		Geometry: widget.Geometry{Kind: widget.GeomScreenRect, Rect: widgetOverlayRect()},
		OnSelect: func(idx int) {
			widget.CloseActive()
			if idx < 0 || idx >= len(entries) {
				return
			}
			executePaletteEntry(h, entries[idx])
		},
		OnSubmit: func(query string) {
			widget.CloseActive()
			recordHistory(historyEntry{Kind: historyFreeText, Name: query})
			if runFreeTextActionChain(h, query) {
				return
			}
			h.HandleCommand(query)
		},
	})
	widget.Open(picker)
}

// CommandPaletteCmd is the command-bar entry point. Args are
// ignored.
func (h *BufPane) CommandPaletteCmd(args []string) {
	h.CommandPalette()
}

// paletteFreeTextResolvesAsActionChain reports whether query would
// resolve, atom by atom, as an action chain: every atom is either a
// command:/command-edit: prefix (both always resolve), a lua:
// atom whose plugin.fn resolves, or a plain BufKeyActions name.
// Atoms that would only resolve via BufMouseActions are treated as
// not resolving, since dispatching one from free text has no
// tcell.EventMouse to run it with.
//
// This mirrors parseBufActionChain's resolution rules without its
// side effect: parseBufActionChain reports an unresolved atom via
// screen.TermMessage, which blocks on a terminal-suspending "press
// enter to continue" prompt. That is fine for a bindings.json load
// error or an explicit "> runaction" typo, but firing it on every
// ordinary free-text command (e.g. "help options") would make the
// palette unusable, so resolution is checked here first and
// parseBufActionChain is only called once every atom is known to
// resolve cleanly.
func paletteFreeTextResolvesAsActionChain(query string) bool {
	if strings.TrimSpace(query) == "" {
		return false
	}

	action := query
	for action != "" {
		var a string
		a, _, action = nextActionChainAtom(action)

		switch {
		case strings.HasPrefix(a, "command:"), strings.HasPrefix(a, "command-edit:"):
			// always resolves
		case strings.HasPrefix(a, "lua:"):
			fn := strings.SplitN(a, ":", 2)[1]
			if LuaAction(fn, KeyEvent{}) == nil {
				return false
			}
		default:
			if _, ok := BufKeyActions[a]; !ok {
				return false
			}
		}
	}
	return true
}

// runFreeTextActionChain runs query as an action chain, dispatching
// through execAction per atom just like a real key binding, when
// every atom resolves per paletteFreeTextResolvesAsActionChain.
// Returns false without running anything when query does not
// resolve as an action chain, so OnSubmit falls back to
// HandleCommand. A single word like "save" is both a valid command
// and a valid action name; per D-22's runaction precedent this
// treats it as the action.
//
// Because the gate above only accepts atoms that re-resolve via
// BufKeyActions, command:/command-edit:, or lua:, every afn
// parseBufActionChain returns here is a BufKeyAction; no mouse-event
// guard is needed at dispatch time.
func runFreeTextActionChain(h *BufPane, query string) bool {
	if !paletteFreeTextResolvesAsActionChain(query) {
		return false
	}

	actionfns, names, types := parseBufActionChain(query, KeyEvent{})
	for i, afn := range actionfns {
		name := names[i]

		var success bool
		if _, ok := BufKeyActions[name]; ok {
			success = runBufActionByName(h, name)
		} else {
			h.Buf.SetCurCursor(0)
			h.Cursor = h.Buf.GetActiveCursor()
			success = h.execAction(afn, name, nil)
		}

		if (!success && types[i] == '&') || (success && types[i] == '|') {
			break
		}
	}
	return true
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
	case paletteBinding:
		// The target is a bindings.json action string, so it runs the
		// way the keystroke would: as an action chain.
		recordHistory(historyEntry{Kind: historyBinding, Name: e.Name})
		runFreeTextActionChain(h, e.Name)
	case paletteFreeText:
		// A recent row for a typed command line: rerun it the way
		// OnSubmit ran it the first time.
		recordHistory(historyEntry{Kind: historyFreeText, Name: e.Name})
		if runFreeTextActionChain(h, e.Name) {
			return
		}
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
