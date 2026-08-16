# Keybindings

Micro has a plethora of hotkeys that make it easy and powerful to use and all
hotkeys are fully customizable to your liking.

Custom keybindings are stored internally in micro if changed with the `> bind`
command or can also be added in the file `~/.config/micro/bindings.json` as
discussed below. For a list of the default keybindings in the json format used
by micro, please see the end of this file. For a more user-friendly list with
explanations of what the default hotkeys are and what they do, please see
`> help defaultkeys` (a json formatted list of default keys is included
at the end of this document).

If `~/.config/micro/bindings.json` does not exist, you can simply create it.
Micro will know what to do with it.

You can use Ctrl + arrows to move word by word (Alt + arrows for Mac). Alt + left and right
move the cursor to the start and end of the line (Ctrl + left/right for Mac), and Ctrl + up and down move the
cursor to the start and end of the buffer.

You can hold shift with all of these movement actions to select while moving.

## Rebinding keys

The bindings may be rebound using the `~/.config/micro/bindings.json` file.
Each key is bound to an action.

For example, to bind `Ctrl-y` to undo and `Ctrl-z` to redo, you could put the
following in the `bindings.json` file.

```json
{
    "Ctrl-y": "Undo",
    "Ctrl-z": "Redo"
}
```

**Note:** The syntax `<Modifier><key>` is equivalent to `<Modifier>-<key>`. In
addition, `Ctrl-Shift` bindings are not supported by terminals, and are the same
as simply `Ctrl` bindings. This means that `CtrlG`, `Ctrl-G`, and `Ctrl-g` all
mean the same thing. However, for `Alt` this is not the case: `AltG` and `Alt-G`
mean `Alt-Shift-g`, while `Alt-g` does not require the Shift modifier.

In addition to editing your `~/.config/micro/bindings.json`, you can run
`>bind <keycombo> <action>` For a list of bindable actions, see below.

You can also chain commands when rebinding. For example, if you want `Alt-s` to
save and quit you can bind it like so:

```json
{
    "Alt-s": "Save,Quit"
}
```

Each action will return a success flag. Actions can be chained such that
the chain only continues when there are successes, or failures, or either.
The `,` separator will always chain to the next action. The `|` separator
will abort the chain if the action preceding it succeeds, and the `&` will
abort the chain if the action preceding it fails. For example, in the default
bindings, tab is bound as

```
"Tab": "Autocomplete|IndentSelection|InsertTab"
```

This means that if the `Autocomplete` action is successful, the chain will
abort. Otherwise, it will try `IndentSelection`, and if that fails too, it
will execute `InsertTab`. To use `,`, `|` or `&` in an action (as an argument
to a command, for example), escape it with `\` or wrap it in single or double
quotes.

If the action has an `onAction` lua callback, for example `onAutocomplete` (see
`> help plugins`), then the action is only considered successful if the action
itself succeeded *and* the callback returned true. If there are multiple
`onAction` callbacks for this action, registered by multiple plugins, then the
action is only considered successful if the action itself succeeded and all the
callbacks returned true.

## Binding commands

You can also bind a key to execute a command in command mode (see
`help commands`). Simply prepend the binding with `command:`. For example:

```json
{
    "Alt-p": "command:pwd"
}
```

**Note for macOS**: By default, macOS terminals do not forward alt events and
instead insert unicode characters. To fix this, do the following:

* iTerm2: select `Esc+` for `Left Option Key` in `Preferences->Profiles->Keys`.
* Terminal.app: Enable `Use Option key as Meta key` in `Preferences->Profiles->Keyboard`.

Now when you press `Alt-p` the `pwd` command will be executed which will show
your working directory in the infobar.

You can also bind an "editable" command with `command-edit:`. This means that
micro won't immediately execute the command when you press the binding, but
instead just place the string in the infobar in command mode. For example,
you could rebind `Ctrl-g` to `> help`:

```json
{
    "Ctrl-g": "command-edit:help "
}
```

Now when you press `Ctrl-g`, `help` will appear in the command bar and your
cursor will be placed after it (note the space in the json that controls the
cursor placement).

## Binding Lua functions

You can also bind a key to a Lua function provided by a plugin, or by your own
`~/.config/micro/init.lua`. For example:

```json
{
    "Alt-q": "lua:foo.bar"
}
```

where `foo` is the name of the plugin and `bar` is the name of the lua function
in it, e.g.:

```lua
local micro = import("micro")

function bar(bp)
    micro.InfoBar():Message("Bar action triggered")
    return true
end
```

See `> help plugins` for more informations on how to write lua functions.

For `~/.config/micro/init.lua` the plugin name is `initlua` (so the keybinding
in this example would be `"Alt-q": "lua:initlua.bar"`).

The currently active bufpane is passed to the lua function as the argument. If
the key is a mouse button, e.g. `MouseLeft` or `MouseWheelUp`, the mouse event
info is passed to the lua function as the second argument, of type
`*tcell.EventMouse`. See https://pkg.go.dev/github.com/micro-editor/tcell/v2#EventMouse
for the description of this type and its methods.

The return value of the lua function defines whether the action has succeeded.
This is used when chaining lua functions with other actions. They can be chained
the same way as regular actions as described above, for example:

```
"Alt-q": "lua:initlua.bar|Quit"
```

## Binding raw escape sequences

Only read this section if you are interested in binding keys that aren't on the
list of supported keys for binding.

One of the drawbacks of using a terminal-based editor is that the editor must
get all of its information about key events through the terminal. The terminal
sends these events in the form of escape sequences often (but not always)
starting with `0x1b`.

For example, if micro reads `\x1b[1;5D`, on most terminals this will mean the
user pressed CtrlLeft.

For many key chords though, the terminal won't send any escape code or will
send an escape code already in use. For example for `CtrlBackspace`, my
terminal sends `\u007f` (note this doesn't start with `0x1b`), which it also
sends for `Backspace` meaning micro can't bind `CtrlBackspace`.

However, some terminals do allow you to bind keys to send specific escape
sequences you define. Then from micro you can directly bind those escape
sequences to actions. For example, to bind `CtrlBackspace` you can instruct
your terminal to send `\x1bctrlback` and then bind it in `bindings.json`:

```json
{
    "\u001bctrlback": "DeleteWordLeft"
}
```

Here are some instructions for sending raw escapes in different terminals

### iTerm2

In iTerm2, you can do this in  `Preferences->Profiles->Keys` then click the
`+`, input your keybinding, and for the `Action` select `Send Escape Sequence`.
For the above example your would type `ctrlback` into the box (the `\x1b`) is
automatically sent by iTerm2.

### Linux using loadkeys

You can do this in linux using the loadkeys program.

Coming soon!

## Unbinding keys

It is also possible to disable any of the default key bindings by use of the
`None` action in the user's `bindings.json` file.

## Local machine overrides

Alongside `bindings.json`, micro reads an optional second bindings file at
`$XDG_CONFIG_HOME/micro/bindings.local.json` (typically
`~/.config/micro/bindings.local.json`). It uses the same syntax as
`bindings.json`, including pane type maps, and any key bound in
`bindings.local.json` overrides the same key in `bindings.json`. The intent
is to keep `bindings.json` clean enough to check into version control or
sync between machines, while keeping machine-specific bindings (keys that
only one terminal delivers, hardware-specific keyboards) in
`bindings.local.json`.

The editor never writes to `bindings.local.json`. The `> bind` and
`> unbind` commands always persist to `bindings.json`, so a binding saved
there stays shadowed by a conflicting local override on the next start or
`> reload`. If you want to change an override permanently, edit
`bindings.local.json` by hand.

## Bindable actions and bindable keys

The list of default keybindings contains most of the possible actions and keys
which you can use, but not all of them. Here is a full list of both.

Full list of possible actions:

```
CursorUp
CursorDown
CursorPageUp
CursorPageDown
CursorLeft
CursorRight
CursorStart
CursorEnd
CursorToViewTop
CursorToViewCenter
CursorToViewBottom
SelectToStart
SelectToEnd
SelectUp
SelectDown
SelectLeft
SelectRight
WordRight
WordLeft
SubWordRight
SubWordLeft
SelectWordRight
SelectWordLeft
SelectSubWordRight
SelectSubWordLeft
DeleteWordRight
DeleteWordLeft
DeleteSubWordRight
DeleteSubWordLeft
SelectLine
SelectToStartOfLine
SelectToStartOfText
SelectToStartOfTextToggle
SelectToEndOfLine
ParagraphPrevious
ParagraphNext
SelectToParagraphPrevious
SelectToParagraphNext
InsertNewline
Backspace
Delete
InsertTab
Save
SaveAll
SaveAs
Find
FindLiteral
FindNext
FindPrevious
FindNextWord
FindPreviousWord
DiffNext
DiffPrevious
Center
Undo
Redo
Copy
CopyAbsolutePath
CopyFileName
CopyLine
CopyRelativePath
Cut
CutLine
Duplicate
DuplicateLine
DuplicateLineDown
DuplicateLineUp
DeleteLine
MoveLinesUp
MoveLinesDown
IndentSelection
OutdentSelection
Autocomplete
CycleAutocompleteBack
OutdentLine
IndentLine
Paste
PastePrimary
SelectAll
OpenFile
FileExplorerAtCwd
FileExplorerAtFile
OpenFilePickerAtCwd
OpenFilePickerAtFile
Start
End
PageUp
PageDown
SelectPageUp
SelectPageDown
HalfPageUp
HalfPageDown
StartOfText
StartOfTextToggle
StartOfLine
EndOfLine
ToggleHelp
ToggleKeyMenu
ToggleDiffGutter
ToggleRuler
ToggleHighlightSearch
UnhighlightSearch
ResetSearch
ClearStatus
ShellMode
CommandMode
ToggleOverwriteMode
Escape
Quit
QuitAll
ForceQuit
AddTab
PreviousTab
NextTab
FirstTab
LastTab
NextSplit
PreviousSplit
FirstSplit
LastSplit
CycleBuffersForward
CycleBuffersBackward
Unsplit
VSplit
HSplit
MovePaneToNext
MovePaneToPrevious
NextLeafSplit
PreviousLeafSplit
ToggleMacro
PlayMacro
Suspend (Unix only)
ScrollUp
ScrollDown
SpawnMultiCursor
SpawnMultiCursorAll
SpawnMultiCursorUp
SpawnMultiCursorDown
SpawnMultiCursorSelect
RemoveMultiCursor
RemoveAllMultiCursors
SkipMultiCursor
SkipMultiCursorBack
JumpToMatchingBrace
JumpLine
JumpBack
JumpForward
PushJump
Deselect
ClearInfo
None
```

The `StartOfTextToggle` and `SelectToStartOfTextToggle` actions toggle between
jumping to the start of the text (first) and start of the line.

The `JumpBack` and `JumpForward` actions step through a global cursor history,
returning you to recent positions across panes and tabs. The history is
populated automatically before any "long jump" action runs (CursorStart,
CursorEnd, JumpLine, JumpToMatchingBrace, Find/FindLiteral/FindNext/FindPrevious,
CursorPageUp/CursorPageDown, HalfPageUp/HalfPageDown, MousePress) and whenever
the active pane or tab changes. The first `JumpBack` after a series of edits
also records your current cursor so `JumpForward` can return to it. Entries
whose pane has been closed are skipped silently. If a pane still exists but
no longer displays the recorded file (something replaced its buffer, e.g.
the `> open` command), the jump restores that file: it switches to another
pane already displaying it if one exists, reopens it in place if the pane's
current buffer has no unsaved changes, or opens it in a new tab otherwise.
`PushJump` records the current cursor as a manual breadcrumb for the same
list. None of these are
bound by default; add entries in `bindings.json` to use them.

The `FileExplorerAtCwd` and `FileExplorerAtFile` actions open a centred
picker showing the directory listing. `FileExplorerAtCwd` starts at the
current working directory; `FileExplorerAtFile` starts at the directory of
the active buffer's file (falling back to the current working directory
when the buffer has no file path). Use `Up`/`Down`/`PageUp`/`PageDown`/
`Home`/`End` or the mouse wheel to move the highlight, `Enter` or
double-click to activate, `Esc` or click outside the picker to cancel.
A `../` entry navigates to the parent directory unless already at a
filesystem root. Selecting a file opens it: if it is already open in some
pane, focus jumps there; otherwise, if the invoking pane holds an unused
scratch buffer (no file path and unmodified), the file replaces it in
place; otherwise it opens in a new tab. The `filemanager.showhidden`
option controls whether dotfiles appear in the listing by default;
pressing `Ctrl-h` while the picker is open toggles visibility for that
session. The `filemanager.showignored` option controls whether the
`.git` directory and entries matched by `.gitignore` appear; pressing
`Ctrl-i` toggles that visibility. The toggles are independent. The
gitignore matcher is anchored at the nearest enclosing git root, so
ancestor `.gitignore` files apply when navigating into a subtree of
a project; outside a git repo only the literal `.git` skip applies.
Neither action is bound by default; add entries in `bindings.json`
to use them.

The `OpenFilePickerAtCwd` and `OpenFilePickerAtFile` actions open a
centred picker showing files recursively under a starting directory,
filterable by the typed query (fuzzy match). `OpenFilePickerAtCwd`
starts at the current working directory; `OpenFilePickerAtFile`
starts at the directory of the active buffer's file (falling back to
the current working directory when the buffer has no file path).
The walker collects every non-ignored entry under the start directory
so the typed filter operates across the full subtree. The same
hidden / ignored toggles apply: `Ctrl-h` flips the
`filemanager.showhidden` axis and `Ctrl-i` flips
`filemanager.showignored` for the session, both rebuilding the list
in place. `Enter` opens the highlighted file with the same precedence
as the file-explorer picker (focus existing pane, swap unused scratch
in place, otherwise open a new tab); typing a path that matches no
entry and pressing `Enter` opens that path verbatim. `Esc` cancels.
Symlinks are skipped to avoid cycles. Neither action is bound by
default; add entries in `bindings.json` to use them.

`CycleBuffersForward` and `CycleBuffersBackward` open a most-recently-used
buffer switcher: a list of every open buffer across all panes and tabs,
ordered most-recently-focused first, with the previous buffer preselected so
a single press of either action followed by Enter is an alt-tab style
toggle. While the list is open, pressing whichever key is bound to either
action moves the selection forward or backward and wraps around at either
end; the arrow keys, Page Up/Down, and typing to fuzzy-filter the list also
work. Enter or clicking a row switches to that buffer and records where you
were as a jump (so `JumpBack` returns to it); Esc cancels and leaves you on
the buffer you started from. Neither action is bound by default. A typical
setup binds them to `Ctrl-Tab` and `Ctrl-Shift-Tab`:

```
{
    "Ctrl-Tab":       "CycleBuffersForward",
    "Ctrl-Shift-Tab": "CycleBuffersBackward"
}
```

Important: this is a press-and-commit cycler, not a release-to-commit one.
Holding a modifier down and tapping the other key repeatedly, then
releasing the modifier to land on a buffer, is how alt-tab works in most
desktop window switchers, but micro cannot do that here. Inside zellij (or
any multiplexer/terminal that does not forward key-release events), the
release of Ctrl can never reach micro at all, so there is nothing to bind
it to. You must press Enter (or click a row) to switch, same as any other
picker in micro.

The `CutLine` action cuts the current line and adds it to the previously cut
lines in the clipboard since the last paste (rather than just replaces the
clipboard contents with this line). So you can cut multiple, not necessarily
consecutive lines to the clipboard just by pressing `Ctrl-k` multiple times,
without selecting them. If you want the more traditional behavior i.e. just
rewrite the clipboard every time, you can use `CopyLine,DeleteLine` action
instead of `CutLine`.

The `MovePaneToNext` and `MovePaneToPrevious` actions shift the active pane
one slot through the global pane sequence formed by every tab's leaves taken
in tree (display) order. Three cases drive the behavior:

- If there is an adjacent leaf in the same tab, the pane swaps visual
  positions with it (the pane stays active in the same slice slot, but
  draws into the swapped leaf).
- If the active pane is at the edge of its tab and an adjacent tab exists,
  the pane is detached and re-attached as a vsplit on the adjacent tab's
  edge leaf. If the source tab loses its last pane, it is removed.
- If the active pane is at the global edge and its tab has more than one
  pane, a fresh tab is created beyond the edge and the pane is moved into
  it. When the source tab has only one pane (the active one), the action
  is a no-op since the result would be the same shape.

The buffer (cursor, undo history, unsaved edits, viewport) and, for
terminal panes, the running pty are preserved across moves.

`NextLeafSplit` and `PreviousLeafSplit` are read-only counterparts that
move focus through the same tree-order sequence inside the current tab,
without restructuring anything. They return false at the edge of the
tab, so they can be chained with `NextTab`/`PreviousTab` for a wrap to
the adjacent tab. For example: `"Alt-Right": "NextLeafSplit|NextTab"`.
Unlike the existing `NextSplit` and `PreviousSplit`, which step by
`tab.Panes` slice index (creation order), these walk the split tree
and so produce the same ordering as `MovePaneToNext`.

The `FindNextWord` and `FindPreviousWord` actions search for the word
currently under the cursor (or, if there is an active single-line selection,
that selection's text), without opening a prompt. The query is picked using
the same logic as the `hlselection` setting, so the cursor jumps between the
matches that `hlselection` highlights. Word-mode searches are whole-word
(`\b…\b`); selection-mode searches are literal substrings. Neither action
has a default binding.

You can also bind some mouse actions (these must be bound to mouse buttons)

```
MousePress
MouseDrag
MouseRelease
MouseMultiCursor
```

Here is the list of all possible keys you can bind:

```
Up
Down
Right
Left
UpLeft
UpRight
DownLeft
DownRight
Center
PageUp
PageDown
Home
End
Insert
Delete
Help
Exit
Clear
Cancel
Print
Pause
Backtab
F1
F2
F3
F4
F5
F6
F7
F8
F9
F10
F11
F12
F13
F14
F15
F16
F17
F18
F19
F20
F21
F22
F23
F24
F25
F26
F27
F28
F29
F30
F31
F32
F33
F34
F35
F36
F37
F38
F39
F40
F41
F42
F43
F44
F45
F46
F47
F48
F49
F50
F51
F52
F53
F54
F55
F56
F57
F58
F59
F60
F61
F62
F63
F64
Ctrl-a
Ctrl-b
Ctrl-c
Ctrl-d
Ctrl-e
Ctrl-f
Ctrl-g
Ctrl-h
Ctrl-i
Ctrl-j
Ctrl-k
Ctrl-l
Ctrl-m
Ctrl-n
Ctrl-o
Ctrl-p
Ctrl-q
Ctrl-r
Ctrl-s
Ctrl-t
Ctrl-u
Ctrl-v
Ctrl-w
Ctrl-x
Ctrl-y
Ctrl-z
Backspace
OldBackspace
Tab
Esc
Escape
Enter
```

You can also bind some mouse buttons (they may be bound to normal actions or
mouse actions)

```
MouseLeft
MouseLeftDrag
MouseLeftRelease
MouseMiddle
MouseMiddleDrag
MouseMiddleRelease
MouseRight
MouseRightDrag
MouseRightRelease
MouseWheelUp
MouseWheelDown
MouseWheelLeft
MouseWheelRight
```

## Key sequences

Key sequences can be bound by specifying valid keys one after another in brackets, such
as `<Ctrl-x><Ctrl-c>`.

# Default keybinding configuration.

A select few keybindings are different on MacOS compared to other
operating systems. This is because different OSes have different
conventions for text editing defaults.

```json
{
    "Up":             "CursorUp",
    "Down":           "CursorDown",
    "Right":          "CursorRight",
    "Left":           "CursorLeft",
    "ShiftUp":        "SelectUp",
    "ShiftDown":      "SelectDown",
    "ShiftLeft":      "SelectLeft",
    "ShiftRight":     "SelectRight",
    "AltLeft":        "WordLeft", (Mac)
    "AltRight":       "WordRight", (Mac)
    "AltUp":          "MoveLinesUp",
    "AltDown":        "MoveLinesDown",
    "CtrlShiftRight": "SelectWordRight",
    "CtrlShiftLeft":  "SelectWordLeft",
    "AltLeft":        "StartOfTextToggle",
    "AltRight":       "EndOfLine",
    "AltShiftRight":  "SelectWordRight", (Mac)
    "AltShiftLeft":   "SelectWordLeft", (Mac)
    "CtrlLeft":       "StartOfText", (Mac)
    "CtrlRight":      "EndOfLine", (Mac)
    "AltShiftLeft":   "SelectToStartOfTextToggle",
    "CtrlShiftLeft":  "SelectToStartOfTextToggle", (Mac)
    "ShiftHome":      "SelectToStartOfTextToggle",
    "AltShiftRight":  "SelectToEndOfLine",
    "CtrlShiftRight": "SelectToEndOfLine", (Mac)
    "ShiftEnd":       "SelectToEndOfLine",
    "CtrlUp":         "CursorStart",
    "CtrlDown":       "CursorEnd",
    "CtrlShiftUp":    "SelectToStart",
    "CtrlShiftDown":  "SelectToEnd",
    "Alt-{":          "ParagraphPrevious",
    "Alt-}":          "ParagraphNext",
    "Enter":          "InsertNewline",
    "Ctrl-h":         "Backspace",
    "Backspace":      "Backspace",
    "Alt-CtrlH":      "DeleteWordLeft",
    "Alt-Backspace":  "DeleteWordLeft",
    "Tab":            "Autocomplete|IndentSelection|InsertTab",
    "Backtab":        "OutdentSelection|OutdentLine",
    "Ctrl-o":         "OpenFile",
    "Ctrl-s":         "Save",
    "Ctrl-f":         "Find",
    "Alt-F":          "FindLiteral",
    "Ctrl-n":         "FindNext",
    "Ctrl-p":         "FindPrevious",
    "Alt-[":          "DiffPrevious|CursorStart",
    "Alt-]":          "DiffNext|CursorEnd",
    "Ctrl-z":         "Undo",
    "Ctrl-y":         "Redo",
    "Ctrl-c":         "Copy|CopyLine",
    "Ctrl-x":         "Cut|CutLine",
    "Ctrl-k":         "CutLine",
    "Ctrl-d":         "Duplicate|DuplicateLine",
    "Ctrl-v":         "Paste",
    "Ctrl-a":         "SelectAll",
    "Ctrl-t":         "AddTab",
    "Alt-,":          "PreviousTab|LastTab",
    "Alt-.":          "NextTab|FirstTab",
    "Home":           "StartOfText",
    "End":            "EndOfLine",
    "CtrlHome":       "CursorStart",
    "CtrlEnd":        "CursorEnd",
    "PageUp":         "CursorPageUp",
    "PageDown":       "CursorPageDown",
    "CtrlPageUp":     "PreviousTab|LastTab",
    "CtrlPageDown":   "NextTab|FirstTab",
    "ShiftPageUp":    "SelectPageUp",
    "ShiftPageDown":  "SelectPageDown",
    "Ctrl-g":         "ToggleHelp",
    "Alt-g":          "ToggleKeyMenu",
    "Ctrl-r":         "ToggleRuler",
    "Ctrl-l":         "command-edit:goto ",
    "Delete":         "Delete",
    "Ctrl-b":         "ShellMode",
    "Ctrl-q":         "Quit",
    "Ctrl-e":         "CommandMode",
    "Ctrl-w":         "NextSplit|FirstSplit",
    "Ctrl-u":         "ToggleMacro",
    "Ctrl-j":         "PlayMacro",
    "Insert":         "ToggleOverwriteMode",

    // Emacs-style keybindings
    "Alt-f": "WordRight",
    "Alt-b": "WordLeft",
    "Alt-a": "StartOfLine",
    "Alt-e": "EndOfLine",

    // Integration with file managers
    "F2":  "Save",
    "F3":  "Find",
    "F4":  "Quit",
    "F7":  "Find",
    "F10": "Quit",
    "Esc": "Escape",

    // Mouse bindings
    "MouseWheelUp":     "ScrollUp",
    "MouseWheelDown":   "ScrollDown",
    "MouseLeft":        "MousePress",
    "MouseLeftDrag":    "MouseDrag",
    "MouseLeftRelease": "MouseRelease",
    "MouseMiddle":      "PastePrimary",
    "Ctrl-MouseLeft":   "MouseMultiCursor",

    // Multi-cursor bindings
    "Alt-n":        "SpawnMultiCursor",
    "AltShiftUp":   "SpawnMultiCursorUp",
    "AltShiftDown": "SpawnMultiCursorDown",
    "Alt-m":        "SpawnMultiCursorSelect",
    "Alt-p":        "RemoveMultiCursor",
    "Alt-c":        "RemoveAllMultiCursors",
    "Alt-x":        "SkipMultiCursor",
}
```

## Pane type bindings

Keybindings can be specified for different pane types as well. For example, to
make a binding that only affects the command bar, use the `command` subgroup:

```
{
    "command": {
        "Ctrl-w": "WordLeft"
    }
}
```

The possible pane types are `buffer` (normal buffer), `command` (command bar),
and `terminal` (terminal pane).

Bindings inside the `command` subgroup can also use the `command:` and `lua:`
prefixes described above in "Binding commands" and "Binding Lua functions".
The command or Lua function runs against the buffer pane behind the command
bar, not the command bar itself, so `setlocal`/`togglelocal` and similar
apply to the buffer as expected even while the command bar is focused. For
example, this rebinds `Alt-i` so that pressing it while a Find prompt is
open toggles case-sensitivity for the search:

```json
{
    "command": {
        "Alt-i": "command:togglelocal ignorecase"
    }
}
```

`command-edit:` cannot be used inside the `command` subgroup, since it would
try to open a second command prompt on top of the one that is already
focused; micro reports an error and ignores such a binding.

The defaults for the command and terminal panes are given below:

```
{
    "terminal": {
        "<Ctrl-q><Ctrl-q>": "Exit",
        "<Ctrl-e><Ctrl-e>": "CommandMode",
        "<Ctrl-w><Ctrl-w>": "NextSplit"
    },

    "command": {
        "Up":             "HistoryUp",
        "Down":           "HistoryDown",
        "Right":          "CursorRight",
        "Left":           "CursorLeft",
        "ShiftUp":        "SelectUp",
        "ShiftDown":      "SelectDown",
        "ShiftLeft":      "SelectLeft",
        "ShiftRight":     "SelectRight",
        "AltLeft":        "StartOfTextToggle",
        "AltRight":       "EndOfLine",
        "AltUp":          "CursorStart",
        "AltDown":        "CursorEnd",
        "AltShiftRight":  "SelectWordRight",
        "AltShiftLeft":   "SelectWordLeft",
        "CtrlLeft":       "WordLeft",
        "CtrlRight":      "WordRight",
        "CtrlShiftLeft":  "SelectToStartOfTextToggle",
        "ShiftHome":      "SelectToStartOfTextToggle",
        "CtrlShiftRight": "SelectToEndOfLine",
        "ShiftEnd":       "SelectToEndOfLine",
        "CtrlUp":         "CursorStart",
        "CtrlDown":       "CursorEnd",
        "CtrlShiftUp":    "SelectToStart",
        "CtrlShiftDown":  "SelectToEnd",
        "Enter":          "ExecuteCommand",
        "CtrlH":          "Backspace",
        "Backspace":      "Backspace",
        "OldBackspace":   "Backspace",
        "Alt-CtrlH":      "DeleteWordLeft",
        "Alt-Backspace":  "DeleteWordLeft",
        "Tab":            "CommandComplete",
        "Backtab":        "CycleAutocompleteBack",
        "Ctrl-z":         "Undo",
        "Ctrl-y":         "Redo",
        "Ctrl-c":         "Copy",
        "Ctrl-x":         "Cut",
        "Ctrl-k":         "CutLine",
        "Ctrl-v":         "Paste",
        "Home":           "StartOfTextToggle",
        "End":            "EndOfLine",
        "CtrlHome":       "CursorStart",
        "CtrlEnd":        "CursorEnd",
        "Delete":         "Delete",
        "Ctrl-q":         "AbortCommand",
        "Ctrl-e":         "EndOfLine",
        "Ctrl-a":         "StartOfLine",
        "Ctrl-w":         "DeleteWordLeft",
        "Insert":         "ToggleOverwriteMode",
        "Ctrl-b":         "WordLeft",
        "Ctrl-f":         "WordRight",
        "Ctrl-d":         "DeleteWordLeft",
        "Ctrl-m":         "ExecuteCommand",
        "Ctrl-n":         "HistoryDown",
        "Ctrl-p":         "HistoryUp",
        "Ctrl-u":         "SelectToStart",

        // Emacs-style keybindings
        "Alt-f": "WordRight",
        "Alt-b": "WordLeft",
        "Alt-a": "StartOfText",
        "Alt-e": "EndOfLine",

        // Integration with file managers
        "F10": "AbortCommand",
        "Esc": "AbortCommand",

        // Mouse bindings
        "MouseWheelUp":     "HistoryUp",
        "MouseWheelDown":   "HistoryDown",
        "MouseLeft":        "MousePress",
        "MouseLeftDrag":    "MouseDrag",
        "MouseLeftRelease": "MouseRelease",
        "MouseMiddle":      "PastePrimary"
    }
}
```

## Final notes

Note: On some old terminal emulators and on Windows machines, `Ctrl-h` should be
used for backspace.

Additionally, alt keys can be bound by using `Alt-key`. For example `Alt-a` or
`Alt-Up`. Micro supports an optional `-` between modifiers like `Alt` and
`Ctrl` so `Alt-a` could be rewritten as `Alta` (case matters for alt bindings).
This is why in the default keybindings you can see `AltShiftLeft` instead of
`Alt-ShiftLeft` (they are equivalent).

Please note that terminal emulators are strange applications and micro only
receives key events that the terminal decides to send. Some terminal emulators
may not send certain events even if this document says micro can receive the
event. To see exactly what micro receives from the terminal when you press a
key, run the `> raw` command.
