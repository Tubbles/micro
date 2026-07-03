Micro has a native Language Server Protocol (LSP) client. It talks
directly to a language server over stdio (no plugin required) and
provides diagnostics, hover information, and go to definition.

This is v1 of native LSP support (read-only features plus the server
lifecycle commands below) plus manual-trigger completion from v2.
Formatting and references are not implemented yet.

# Enabling LSP for a buffer

LSP attachment is controlled by the `lsp` option, a per-buffer boolean
that defaults to `false`. Turn it on for the current buffer with
`> setlocal lsp true`, or turn it on globally (or per filetype) the
same way as any other option, for example in `settings.json`:

```
"ft:go": {
    "lsp": true
}
```

When `lsp` is on and a server is configured for the buffer's filetype
(see below), micro starts (or reuses) that server, sends
`textDocument/didOpen`, and keeps it in sync with `didChange`,
`didSave`, and `didClose` as you edit, save, and close the buffer.

# The server registry

Which command to run for which filetype is defined in
`runtime/lspservers.json` (bundled with micro) and can be extended or
overridden per-user in `ConfigDir/lspservers.json`. Each entry looks
like:

```
{
    "go": {
        "command": "gopls",
        "args": [],
        "roots": ["go.mod", ".git"],
        "filetypes": ["go"]
    }
}
```

`command` and `args` are the executable and arguments used to start the
server. `roots` is a list of marker files or directories (checked
walking up from the buffer's directory) used to find the workspace
root; if none is found, micro falls back to its current working
directory. `filetypes` lists the micro filetypes this server handles.
An entry in `ConfigDir/lspservers.json` with the same key overrides the
bundled one; a new key adds a server.

One running server process is shared by every buffer with the same
(server name, workspace root) pair, so opening several files from the
same project reuses one connection.

# Commands

* `> lsp status`: opens a log-buffer report of every running server
   (name, workspace root, lifecycle state) and every attached document
   (its server, root, and current error/warning counts).

* `> lsp start ['server']`: starts (or confirms already running) a
   server for the current buffer's workspace root. If `server` is
   given, that server definition is used instead of the one registered
   for the current buffer's filetype.

* `> lsp stop ['server']`: shuts down the running server for the
   current buffer's workspace root (or the named `server`), via the LSP
   `shutdown`/`exit` handshake. Buffers that were attached to it keep
   their edits locally, but further document-sync notifications go
   nowhere until the buffer is closed and reopened, or the `lsp` option
   is toggled off and back on.

* `> lsp restart ['server']`: `stop` followed by `start`. Useful when a
   server has wedged itself. This supersedes the old `lspRestart`
   plugin's approach of finding and `killall`-ing the server process;
   the native command performs a clean shutdown handshake instead. Like
   `stop`, it does not re-attach documents that were open against the
   stopped server.

None of the `lsp` subcommands touch the `lsp` per-buffer option: they
manage server processes, not which buffers are attached to them.

# Actions

* `LspHover`: requests hover information at the cursor and shows it in
   the InfoBar. If the server has nothing to say about that position,
   micro shows a quiet "no hover info" message instead.

* `LspGotoDefinition`: requests the definition of the symbol at the
   cursor. If it is in a different file, that file is opened (or
   switched to, if already open in another tab or split) and the cursor
   is moved there; if it is in the current file, the cursor just moves.

* `LspCompletion`: requests completion candidates at the cursor and, if
   the server returns any, opens a popup listing them. See "Completion"
   below for the popup's keys and current limitations.

None of these actions has a default keybinding. Bind them the same way
as any other action, for example:

```
> bind Alt-h LspHover
> bind Alt-d LspGotoDefinition
> bind Alt-c LspCompletion
```

# Completion

`LspCompletion` is manual-trigger only: nothing runs automatically
while you type. There is no trigger-character support, no debounce,
and no incremental filtering of the popup as you keep typing. Every
keystroke after the popup opens either moves the highlight or
dismisses the popup; it never narrows the candidate list.

Once open, the popup responds to:

* `<Up>`/`<Down>` or `<Ctrl-P>`/`<Ctrl-N>`: move the highlight.
* `<Enter>` or `<Tab>`: insert the highlighted candidate and close the
   popup.
* `<Esc>`: close the popup without inserting anything.
* any other key: closes the popup and is then handled normally, so for
   example typing a character both dismisses the popup and inserts that
   character, the usual "keep typing past the suggestion" behavior.

If a candidate is a snippet (the server marked it with
`insertTextFormat: Snippet`), micro inserts its text verbatim,
including any `$1`/`${1:name}`-style placeholders. Expanding snippets
into tab stops is not implemented, so a snippet completion inserts its
raw placeholder syntax as plain text.

# Diagnostics

Diagnostics published by the server (`textDocument/publishDiagnostics`)
show up as gutter messages, the same mechanism the `linter` plugin and
manual `AddMessage` calls use, so they appear alongside (and do not
replace) any other gutter messages on the buffer. Each publish from a
server replaces that server's previous batch of messages on the buffer;
messages from other sources (a linter, the diff gutter) are untouched.

A running error/warning count is available for the statusline via
`$(lsp)`, for example by adding it to `statusformatr` in
`settings.json`. It is empty when there are no LSP diagnostics on the
buffer.

# Migrating from the Lua `lsp` plugin

If you have the community `lsp` Lua plugin installed
(`~/.config/micro/plug/lsp`), be aware that it registers its own
enable/disable option under the exact same name as micro's native `lsp`
option: every plugin gets an auto-registered common option named after
its own directory, and that plugin's directory is named `lsp`.

This means setting `lsp` to `true` to turn on native LSP support will
also satisfy the Lua plugin's own enabled check and cause it to load
and run at the same time, both talking to a language server for the
same buffer. Before enabling the native `lsp` option, remove or disable
the Lua plugin, for example with `> plugin remove lsp`.
