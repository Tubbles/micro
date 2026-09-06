# Kroken Plugin

The kroken plugin sends the selection to
[kroken](https://github.com/Tubbles/kroken), a one-shot LLM coding
harness built on Claude Code, and replaces the selection with what the
model wrote once the run finishes. Select a comment that says what a
function should do together with its empty body, trigger kroken, keep
editing, and the body gets filled in.

Requirements: the `kroken` executable on your `PATH`, and a `claude`
login that kroken can use. See kroken's own documentation for its
configuration files and profiles.

## Usage

Select the region to rewrite and run

```
> kroken
```

or press `Alt-Enter`, the default binding. Bind another key in your
`bindings.json`:

```json
{
    "Alt-k": "command:kroken"
}
```

Arguments are passed on to `kroken complete` unchanged, so a profile or
model can be picked per run:

```
> kroken --profile fast
> kroken --model claude-sonnet-5
```

`> kroken --dry-run` replaces the selection with the command line and
prompt kroken would have used, without running Claude. It is a cheap way
to check that the setup works.

## Behaviour

* Only the active cursor's selection is sent. Multiple cursors are not
  supported.
* Every trigger starts its own kroken process. Start as many as you like
  and keep editing while they run. The info bar shows `kroken: running`,
  then kroken's own status line, for example
  `kroken: done in 4.2 s, 3 turns, $0.0421`, or its error.
* The selected region is tracked through your edits with an anchor, so
  inserting or deleting text above or below it does not confuse the paste.
  Editing the selected text itself while kroken runs cancels the paste:
  the result is not applied and the info bar says so. The log directory
  kroken prints on its stderr, when logging is enabled, still holds the
  result.
* The replacement is a single undo step.
* The command line of every run and everything kroken prints on stderr
  go to micro's log buffer, prefixed `[kroken]`. Open it with `> log`
  when the info bar's one-liner is not enough.
* kroken hands Claude Code the file on disk, not the buffer. Save first
  when the surrounding context matters.
