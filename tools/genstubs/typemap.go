package main

import (
	"fmt"
	"go/types"
	"strings"
)

// allowedPackages limits which Go packages get their named types emitted as
// LuaCATS @class declarations. Anything outside is mapped to `any` (with an
// optional override).
var allowedPackages = []string{
	"github.com/micro-editor/micro/v2/internal/action",
	"github.com/micro-editor/micro/v2/internal/buffer",
	"github.com/micro-editor/micro/v2/internal/config",
	"github.com/micro-editor/micro/v2/internal/display",
	"github.com/micro-editor/micro/v2/internal/info",
	"github.com/micro-editor/micro/v2/internal/shell",
	"github.com/micro-editor/micro/v2/internal/util",
}

// luaTypeOf maps a Go type to its LuaCATS string. The closure parameter is
// updated as a side effect: any named in-tree type encountered is added so
// the type-closure pass emits a @class for it.
func luaTypeOf(t types.Type, closure *typeCloser, ovr *overrides) string {
	if t == nil {
		return "any"
	}
	switch tt := t.(type) {
	case *types.Basic:
		return luaBasic(tt)
	case *types.Pointer:
		// gopher-luar exposes both pointer and value receivers; from Lua
		// the experience is the same conceptual type.
		return luaTypeOf(tt.Elem(), closure, ovr)
	case *types.Slice:
		return luaTypeOf(tt.Elem(), closure, ovr) + "[]"
	case *types.Array:
		return luaTypeOf(tt.Elem(), closure, ovr) + "[]"
	case *types.Map:
		k := luaTypeOf(tt.Key(), closure, ovr)
		v := luaTypeOf(tt.Elem(), closure, ovr)
		return fmt.Sprintf("table<%s, %s>", k, v)
	case *types.Chan:
		return "any"
	case *types.Interface:
		// `interface{}` / `any`.
		if tt.Empty() {
			return "any"
		}
		return "any"
	case *types.Named:
		obj := tt.Obj()
		pkg := obj.Pkg()
		if pkg == nil {
			// Built-in named type (e.g. "error").
			if obj.Name() == "error" {
				return "string?"
			}
			return "any"
		}
		// Check for an override first.
		if name := ovr.typeName(pkg.Path(), obj.Name()); name != "" {
			closure.seedByName(name)
			return name
		}
		if !isAllowedPackage(pkg.Path()) {
			return "any"
		}
		closure.seed(t)
		return obj.Name()
	case *types.Signature:
		return luaSignature(tt, closure, ovr)
	case *types.Tuple:
		// Returned only as part of a function's results; handled in luaSignature.
		return "any"
	case *types.Struct:
		return "any"
	default:
		return "any"
	}
}

func luaBasic(t *types.Basic) string {
	switch t.Kind() {
	case types.Bool, types.UntypedBool:
		return "boolean"
	case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
		types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
		types.Uintptr, types.UntypedInt, types.UntypedRune:
		// types.Byte == types.Uint8 and types.Rune == types.Int32 (aliases),
		// so they don't need separate cases.
		return "integer"
	case types.Float32, types.Float64, types.UntypedFloat:
		return "number"
	case types.String, types.UntypedString:
		return "string"
	case types.UntypedNil:
		return "nil"
	default:
		return "any"
	}
}

// luaSignature renders a Go function signature as a LuaCATS `fun(...): ...`
// type. Receiver is not included; callers (struct method emission) splice
// `self: T` in if needed.
func luaSignature(sig *types.Signature, closure *typeCloser, ovr *overrides) string {
	var ps []string
	params := sig.Params()
	for i := 0; i < params.Len(); i++ {
		p := params.At(i)
		name := p.Name()
		if name == "" {
			name = fmt.Sprintf("a%d", i)
		}
		// Variadic last param.
		if sig.Variadic() && i == params.Len()-1 {
			if slice, ok := p.Type().(*types.Slice); ok {
				ps = append(ps, fmt.Sprintf("...: %s", luaTypeOf(slice.Elem(), closure, ovr)))
				continue
			}
		}
		ps = append(ps, fmt.Sprintf("%s: %s", luaSafeName(name), luaTypeOf(p.Type(), closure, ovr)))
	}

	res := sig.Results()
	var rs []string
	for i := 0; i < res.Len(); i++ {
		r := res.At(i)
		rs = append(rs, luaTypeOf(r.Type(), closure, ovr))
	}
	out := "fun(" + strings.Join(ps, ", ") + ")"
	if len(rs) > 0 {
		out += ": " + strings.Join(rs, ", ")
	}
	return out
}

// luaSafeName escapes Lua reserved words so they can appear as parameter
// names without breaking the LuaCATS parser.
func luaSafeName(name string) string {
	switch name {
	case "end", "function", "local", "nil", "true", "false", "and", "or",
		"not", "in", "do", "while", "for", "if", "then", "else", "elseif",
		"return", "break", "repeat", "until", "goto":
		return name + "_"
	}
	return name
}

func isAllowedPackage(path string) bool {
	for _, p := range allowedPackages {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// luaModuleClass returns the class name used to type the module table for a
// given Lua module path. e.g. "micro/buffer" -> "MicroBufferModule".
func luaModuleClass(luaPath string) string {
	parts := strings.Split(luaPath, "/")
	out := ""
	for _, p := range parts {
		out += strings.Title(p)
	}
	return out + "Module"
}

// luaModuleFile returns the stub filename for a given Lua module path.
func luaModuleFile(luaPath string) string {
	return strings.ReplaceAll(luaPath, "/", "_") + ".lua"
}
