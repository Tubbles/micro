# Command bar

The command bar is opened by pressing `Ctrl-e`. It is a single-line buffer,
meaning that all keybindings from a normal buffer are supported (as well
as mouse and selection).

When running a command, you can use extra syntax that micro will expand before
running the command. To use an argument with a space in it, put it in
quotes. The command bar parser uses the same rules for parsing arguments that
`/bin/sh` would use (single quotes, double quotes, escaping). The command bar
does not look up environment variables.

# Commands

Micro provides the following commands that can be executed at the command-bar
by pressing `Ctrl-e` and entering the command. Arguments are placed in single
quotes here but these are not necessary when entering the command in micro.

* `bind 'key' 'action'`: creates a keybinding from key to action. See the
   `keybindings` documentation for more information about binding keys.
   This command will modify `bindings.json` and overwrite any bindings to
   `key` that already exist.

* `help ['topic'] ['flags']`: opens the corresponding help topics.
   If no topic is provided opens the default help screen. If multiple topics are
   provided (separated via ` `) they are opened all as splits.
   Help topics are stored as `.md` files in the `runtime/help` directory of
   the source tree, which is embedded in the final binary.
   The `flags` are optional.
   * `-hsplit`: Opens the help topic in a horizontal split
   * `-vsplit`: Opens the help topic in a vertical split

   The default split type is defined by the global `helpsplit` option.

* `save ['filename']`: saves the current buffer. If the file is provided it
   will 'save as' the filename.

* `quit`: quits micro.

* `goto 'line[:col]'`: goes to the given absolute line (and optional column)
   number.
   A negative number can be passed to go inward from the end of the file.
   Example: -5 goes to the 5th-last line in the file.

* `jump 'line[:col]'`: goes to the given relative number from the current
   line (and optional absolute column) number.
   Example: -5 jumps 5 lines up in the file, while (+)3 jumps 3 lines down.

* `replace 'search' 'value' ['flags']`: This will replace `search` with `value`.
   The `flags` are optional. Possible flags are:
   * `-a`: Replace all occurrences at once
   * `-l`: Do a literal search instead of a regex search

   Note that `search` must be a valid regex (unless `-l` is passed). If one
   of the arguments does not have any spaces in it, you may omit the quotes.

   In case the search is done non-literal (without `-l`), the 'value'
   is interpreted as a template:
   * `$3` or `${3}` substitutes the submatch of the 3rd (capturing group)
   * `$foo` or `${foo}` substitutes the submatch of the (?P<foo>named group)
   * You have to write `$$` to substitute a literal dollar.

* `replaceall 'search' 'value'`: this will replace all occurrences of `search`
   with `value` without user confirmation.

   See `replace` command for more information.

* `set 'option' 'value'`: sets the option to value. See the `options` help
   topic for a list of options you can set. This will modify your
   `settings.json` with the new value. If value equals the option's default,
   the entry is removed from `settings.json` instead. `set` always writes to
   `settings.json`, never to `settings.local.json` (see the `options` help
   topic for details on `settings.local.json`).

* `setlocal 'option' 'value'`: sets the option to value locally (only in the
   current buffer). This will *not* modify `settings.json`.

* `toggle 'option'`: toggles the option. Only works with options that accept
   exactly two values. This will modify your `settings.json` with the new
   value (or remove the entry if the new value is the default).

* `togglelocal 'option'`: toggles the option locally (only in the
   current buffer). Only works with options that accept exactly two values.
   This will *not* modify `settings.json`.

* `reset 'option'`: resets the given option to its default value. This will
   also remove the entry from `settings.json`.

* `options`: opens a picker listing every option, its effective value, and
   which configuration layer supplied it. Selecting an option switches the
   picker to a value editor for it, with `Enter` applying the value for this
   session only and `Ctrl-Enter` applying it permanently. See the `options`
   help topic for details.

* `show 'option'`: shows the current value of the given option.

* `showkey 'key'`: Show the action(s) bound to a given key. For example
   running `> showkey Ctrl-c` will display `Copy`.

* `runaction 'action'`: runs the named buffer action once, as if its
   keybinding had been pressed. The name is the action identifier used in
   `bindings.json`, for example `> runaction DuplicateLine` or
   `> runaction CursorEnd`. Action names are case-sensitive and the
   command bar tab-completes them. Per-cursor actions (those listed in
   `MultiActions`) run once per cursor, matching how a real keybinding
   would behave with multiple cursors. Mouse actions cannot be invoked
   this way because they require a mouse event payload.

* `commandpalette`: opens a searchable picker that lists every buffer
   action, every command, and every Lua plugin function, with
   the keys bound to each entry shown alongside. Type to fuzzy-filter,
   `Up`/`Down` to move, `Enter` to run, `Esc` to cancel. The bindings
   column is part of the search haystack, so a query like
   `ctrl-shift-x` filters down to whatever is bound to that keystroke
   (assuming the binding is a single token; chain-bound entries do
   not surface their bindings in v1). If the typed query matches no
   entry, `Enter` instead runs the query as a command line, exactly as
   if you had typed it after `Ctrl-E`. That lets the palette double as
   a free-text command bar for cases that need arguments, e.g.
   `saveas foo.txt`, `help commands`, `set tabsize 4`. `Ctrl-Enter`
   forces the command-line dispatch even when the fuzzy matcher has
   highlighted a row, so a query like `ltm exec` can be sent verbatim
   instead of running whatever was highlighted (requires a CSI-u
   terminal such as kitty, or a multiplexer that forwards CSI-u;
   legacy terminals collapse `Ctrl-Enter` to `Enter`). The picker has
   two modes: Atlas (the full catalog) and History (the most-recent
   items you dispatched through this palette in this session). `Tab`
   toggles between them; the palette opens in History when history
   is non-empty, else in Atlas. Re-running an item from History also
   moves it back to the top. History is in-memory only and resets
   when micro restarts. The three kinds of entries can be toggled
   independently with `commandpalette.actions`,
   `commandpalette.commands` and `commandpalette.lua` (defaults all
   `true`). With `commandpalette.commands` on, the palette also lists
   one argument level for commands whose completer can enumerate a
   fixed set of candidates, as separate filterable entries such as
   `help options`, `set tabsize` or `plugin install`. Selecting one of
   these runs the whole line through the command bar, exactly like
   picking a plain command entry does. Commands whose only argument is
   a filename, such as `open` and `vsplit`, are excluded from this
   listing. `commandpalette.historysize` (default `20`) caps the
   history; set it to `0` to disable history entirely (Tab becomes a
   no-op and nothing is recorded). No default key binding ships;
   users who want a VSCode-style entry point can add
   `"CtrlShiftP": "command:commandpalette"` to bindings.json.

* `run 'sh-command'`: runs the given shell command in the background. The
   command's output will be displayed in one line when it finishes running.

* `vsplit ['filename']`: opens a vertical split with `filename`. If no filename
   is provided, a vertical split is opened with an empty buffer. If multiple
   files are provided (separated via ` `) they are opened all as splits.

* `hsplit ['filename']`: same as `vsplit` but opens a horizontal split instead
   of a vertical split.

* `tab ['filename']`: opens the given file in a new tab. If no filename
   is provided, a tab is opened with an empty buffer. If multiple files are
   provided (separated via ` `) they are opened all as tabs.

* `tabmove '[-+]n'`: Moves the active tab to another slot. `n` is an integer.
   If `n` is prefixed with `-` or `+`, then it represents a relative position
   (e.g. `tabmove +2` moves the tab to the right by `2`). If `n` has no prefix,
   it represents an absolute position (e.g. `tabmove 2` moves the tab to slot `2`).

* `tabswitch 'tab'`: This command will switch to the specified tab. The `tab`
   can either be a tab number, or a name of a tab.

* `textfilter 'sh-command'`: filters the current selection through a shell
   command as standard input and replaces the selection with the stdout of
   the shell command.  For example, to sort a list of numbers, first select
   them, and then execute `> textfilter sort -n`.
   If a cursor has no selection, the behaviour is controlled by the
   `textfilterselectword` option: by default the word at the cursor is
   selected and filtered. With `textfilterselectword` set to `false` the
   command is instead invoked with an empty standard input and its output
   is inserted at the cursor position.

* `log`: opens a log of all messages and debug statements.

* `plugin list`: lists all installed plugins.

* `plugin install 'pl'`: install a plugin.

* `plugin remove 'pl'`: remove a plugin.

* `plugin update ['pl']`: update a plugin (if no arguments are provided
   updates all plugins).

* `plugin search 'pl'`: search available plugins for a keyword.

* `plugin available`: show available plugins that can be installed.

* `reload`: reloads all runtime files (settings, keybindings, syntax files,
   colorschemes, plugins). All plugins will be unloaded by running their
   `deinit()` function (if it exists), and then loaded again by calling the
   `preinit()`, `init()` and `postinit()` functions (if they exist).

* `cd 'path'`: Change the working directory to the given `path`. This is the
   plain chdir tool; it does not touch what is open beyond re-displaying
   buffer paths relative to the new directory. See `> help workspaces` for
   `opendir`, which is the full workspace switch.

* `pwd`: Print the current working directory.

* `copyfilename`: Copy the current buffer's basename (the filename without
   any directory part) to the system clipboard.

* `copyrelativepath`: Copy the current buffer's path, relative to the
   directory micro was launched from, to the system clipboard.

* `copyabsolutepath`: Copy the current buffer's absolute filesystem path to
   the system clipboard.

* `open 'filename'`: Open a file in the current buffer.

* `opendir 'path'`: Switch to the dir-backed workspace rooted at `path`,
   saving the current workspace (if any), prompting to save modified
   buffers, and replaying `path`'s previously saved layout if one exists.
   See `> help workspaces`.

* `workspaces`: Open a picker listing recently opened dir-backed workspaces,
   most recent first. Selecting one switches to it. See `> help workspaces`.

* `reopen`: Reopens the current file from disk.

* `reopenclosed`: Reopens the most recently closed buffer in a new tab,
   restoring its cursor position. If that file is already open in a pane,
   switches focus there instead of opening a duplicate. Repeated invocations
   dig deeper into the history of closed buffers (up to the last 20).

* `retab`: Replaces all leading tabs with spaces or leading spaces with tabs
   depending on the value of `tabstospaces`.

* `raw`: micro will open a new tab and show the escape sequence for every event
   it receives from the terminal. This shows you what micro actually sees from
   the terminal and helps you see which bindings aren't possible and why. This
   is most useful for debugging keybindings.

* `term ['exec']`: Open a terminal emulator running the given executable. If no
   executable is given, this will open the default shell in the terminal
   emulator.

---

The following commands are provided by the default plugins:

* `lint`: Lint the current file for errors.
* `comment`: automatically comment or uncomment current selection or line.
