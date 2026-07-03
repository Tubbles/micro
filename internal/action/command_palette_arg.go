package action

import (
	"reflect"
	"sort"

	"github.com/micro-editor/micro/v2/internal/buffer"
)

// buildCommandArgPaletteEntries enumerates one argument level for
// every registered command whose completer can produce a fixed list
// of candidates, yielding entries shaped "<cmd> <arg>" (e.g.
// "help options", "set tabsize"). Each completer is driven against a
// synthetic single-line buffer containing "<cmd> " with the cursor at
// end-of-line, exactly the input a completer sees after the user
// types the command name and a space into the command bar.
// Completers only read the active cursor and GetArg-style helpers,
// so an in-memory buffer satisfies both Go completers and
// Lua-registered ones (via MakeCommand) uniformly, with no new
// plugin API.
//
// Excluded: nil completers, buffer.FileComplete (file navigation
// belongs to the open-file picker, matched by function-pointer
// identity), and the "runaction" command (actions are already
// first-class palette entries). Only one argument level is
// enumerated; the arg's own sub-arguments are not recursed into.
func buildCommandArgPaletteEntries(rev map[string][]string) []paletteEntry {
	fileComplete := reflect.ValueOf(buffer.FileComplete).Pointer()

	var names []string
	for name := range commands {
		names = append(names, name)
	}
	sort.Strings(names)

	var out []paletteEntry
	for _, name := range names {
		if name == "runaction" {
			continue
		}
		completer := commands[name].completer
		if completer == nil {
			continue
		}
		if reflect.ValueOf(completer).Pointer() == fileComplete {
			continue
		}
		for _, arg := range completerArgs(completer, name) {
			label := name + " " + arg
			out = append(out, paletteEntry{
				Kind:     paletteCommandArg,
				Name:     label,
				Bindings: rev["command:"+label],
			})
		}
	}
	return out
}

// completerArgs drives a command completer against a synthetic
// one-line buffer "<name> " with the cursor positioned at
// end-of-line, and returns the completions it produces. The
// synthetic arg is always empty, so each completion is the
// argument's full text rather than a partial suffix.
func completerArgs(completer buffer.Completer, name string) []string {
	b := buffer.NewBufferFromString(name+" ", "", buffer.BTDefault)
	b.GetActiveCursor().End()
	completions, _ := completer(b)
	return completions
}
