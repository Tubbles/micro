# micro Lua API type stubs

This directory ships LuaLS / lua-language-server type definitions for the
micro plugin API. They give plugin authors autocomplete, hover docs, and
undefined-symbol diagnostics in any editor with a Lua language server
running.

The stubs are dev-time artifacts: micro itself does not load them at runtime.

## What's here

- `library/*.lua` — generated stubs covering the registered modules
  (`micro`, `micro/buffer`, `micro/config`, `micro/shell`, `micro/util`),
  the reflection-exposed Go types (`BufPane`, `Buffer`, `Cursor`, `Loc`,
  `Message`, ...), the `ActionName` string-literal alias, and every
  lifecycle / per-action plugin hook.
- `_overrides.lua` — hand-authored corrections layered on top of the
  generated output. LuaLS merges multiple `---@class Foo` declarations,
  so anything here augments the generated file of the same class.
- `config.json` — LuaLS addon manifest. Triggers auto-activation when
  LuaLS sees a Lua file calling `import(...)` or defining a known hook.

## Wiring this up for plugin development

Pick one of the install paths.

### 1. Symlink (simplest)

Symlink this directory next to your micro plug dir, then point a per-plugin
`.luarc.json` at it.

```sh
ln -s $(realpath runtime/meta) ~/.config/micro/meta
```

In each plugin's directory (`~/.config/micro/plug/<myplugin>/.luarc.json`):

```json
{
    "runtime.version": "Lua 5.1",
    "workspace.library": ["~/.config/micro/meta/library"],
    "diagnostics.globals": ["import"]
}
```

### 2. Per-project workspace.library

If you don't want a symlink, list the absolute path directly:

```json
{
    "runtime.version": "Lua 5.1",
    "workspace.library": ["/path/to/micro/runtime/meta/library"],
    "diagnostics.globals": ["import"]
}
```

### 3. LuaLS user-third-party

Drop a copy of `runtime/meta` into LuaLS's third-party lookup path
(see `https://luals.github.io/wiki/addons/`). LuaLS will offer to enable
the `micro-editor` addon when it sees a triggering file.

## Smoke test

Open `~/.config/micro/plug/<myplugin>/<myplugin>.lua` in your editor with
LuaLS running. Try:

```lua
local micro = import("micro")
local bp = micro.CurPane()
bp:CursorUp()
```

You should see hover docs on `CurPane` (returns `BufPane`), autocomplete
for `bp:` revealing the BufPane methods, and an undefined-method diagnostic
if you type `bp:NotARealAction()`.

## Regenerating after Go API changes

```sh
make stubs
```

Runs `tools/genstubs` which AST-walks `cmd/micro/initlua.go` for the
explicit registrations and uses `go/types` to enumerate the reflected
method/field surface of types reachable from those registrations. Output
overwrites `library/*.lua`. Hand-authored files (`_overrides.lua`,
`config.json`, this README) are left alone.

## Known caveats

See `runtime/help/plugins.md` for the gopher-luar quirks the stubs cannot
fully express:

- Value-typed struct fields come back as pointers from Lua (read-fine,
  pass-as-arg-fails).
- `ipairs` errors on Go-backed slices; use `for i = 1, #x do` or
  `for i, v in x() do`.
- Lua 5.1 syntax only (no `goto`, no integer subtype, bitops via `bit32`).
- Hook return values: `pre<Action>` returning `false` cancels; `on<Action>`
  returns are ignored.
