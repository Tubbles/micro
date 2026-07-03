# Workspaces

A dir-backed workspace remembers the tabs, splits and cursor positions you had open for a given directory, so leaving and coming back to a project restores where you left off.

## Opening a workspace

`> opendir DIR` switches the editor to the dir-backed workspace rooted at DIR. You can also start micro directly on a directory from the command line, for example `micro myproject/`, which opens that directory as a workspace on startup. A directory argument must be the only positional argument on the command line: mixing a directory with file arguments is an error.

Opening a directory does the following, in order. First, if a dir-backed workspace is already active, its current layout is saved. Second, any modified buffers are saved one at a time, prompting for each in turn. Third, all open buffers are closed. Fourth, the working directory changes to DIR. Fifth, DIR's previously saved layout is replayed if one exists, otherwise a single empty tab is opened.

`> cd DIR` is the plain chdir tool. It only changes the working directory and re-displays open buffers relative to it. It does not save, prompt, close or replay anything, and it does not make DIR a workspace. Use `> opendir` when you want the full workspace switch.

## Jumping between workspaces

`> workspaces` opens a picker listing recently opened dir-backed workspaces, most recent first. Selecting an entry switches to it the same way `> opendir` does. Typing filters the list.

The same picker is available as the `WorkspacePicker` action for a keybinding, for example in `bindings.json`:

```json
{
    "Alt-w": "WorkspacePicker"
}
```

It has no default binding.

## What gets saved

A workspace's saved layout includes its tabs, the split tree within each tab, the proportion of the screen each split occupies, which file is open in each pane, and the cursor position in each file.

Panes that show a terminal or a raw event viewer instead of a file are not saved. A running child process cannot be restored, so a tab that only ever held one of these is dropped entirely rather than replayed as something empty.

A workspace's layout is saved when you switch to a different workspace, when you quit micro, and also on a signal-driven exit (for example the terminal closing), so a crash-adjacent exit does not silently lose the session.

## Where workspace state lives

Workspace state is kept separate from your project. The saved layout and the most-recently-used workspace list are machine-local files under `ConfigDir/workspaces/`, normally `~/.config/micro/workspaces/`. Nothing is written into the project directory itself, so opening a workspace never shows up as noise in the project's own version control status.

`micro -clean` will offer to remove workspace state files whose directory no longer exists on disk.

## Workspace config

Unlike the machine-local state above, a workspace can also carry human-edited config that lives inside the project itself, under `${dir}/.ide/micro/`:

* `settings.json` and `settings.local.json` add two more layers to the settings precedence chain, above your own `settings.json` and `settings.local.json` and below command-line flags. See `> help options` for the full precedence order and the `set`/`setlocal`/`ft:`/`glob:` syntax, which is identical here.
* `bindings.local.json` adds one more layer to the key-binding chain, above your own `bindings.json` and `bindings.local.json`. It uses the same format as `bindings.json` (see `> help keybindings`).

`${dir}/.ide/micro/settings.json` is meant to be checked into the project's own version control, so a team can share editor settings the same way they share a `.editorconfig`. The two `.local.json` siblings are for a contributor's own machine or personal preference and are not meant to be committed. Micro never writes to either `.local.json` file; edit them by hand.

Both layers are loaded when a dir-backed workspace is opened or switched to, and cleared when no dir-backed workspace is active. Switching from one workspace to another replaces the previous workspace's config with the new one rather than merging them.

Use `> setworkspace <option> <value>` to write an option into the active workspace's own `settings.json`, the same way `> set` writes to your own `settings.json`. It is only valid while a dir-backed workspace is open, otherwise it reports an error in the info bar. There is no `setworkspace`-equivalent command for bindings: edit `bindings.local.json` by hand, same as `bindings.local.json` at the user level.
