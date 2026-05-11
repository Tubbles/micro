package main

import (
	"fmt"
	"go/ast"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// luaImportFuncs maps the Go function name in cmd/micro/initlua.go to the
// Lua module path it populates.
var luaImportFuncs = map[string]string{
	"luaImportMicro":       "micro",
	"luaImportMicroShell":  "micro/shell",
	"luaImportMicroBuffer": "micro/buffer",
	"luaImportMicroConfig": "micro/config",
	"luaImportMicroUtil":   "micro/util",
}

// registration is a single Lua-visible export.
type registration struct {
	name   string     // Lua name, e.g. "InfoBar"
	goExpr ast.Expr   // the AST expression passed to luar.New
	goType types.Type // resolved type of goExpr (may be nil if unresolved)
	doc    string     // best-effort doc comment from the bound symbol
}

type moduleRegistrations struct {
	luaPath string // e.g. "micro/buffer"
	goFunc  string // e.g. "luaImportMicroBuffer"
	entries []registration
}

type registrations struct {
	modules []*moduleRegistrations // in deterministic order: micro, micro/buffer, ...
}

func (r *registrations) totalEntries() int {
	n := 0
	for _, m := range r.modules {
		n += len(m.entries)
	}
	return n
}

func (r *registrations) module(luaPath string) *moduleRegistrations {
	for _, m := range r.modules {
		if m.luaPath == luaPath {
			return m
		}
	}
	return nil
}

// extractRegistrations finds every ulua.L.SetField call inside the
// luaImportMicro* functions in cmd/micro/initlua.go and returns a structured
// registry keyed by Lua module path.
func extractRegistrations(cfg *config, pkgs []*packages.Package) (*registrations, error) {
	cmdMicro := findPackage(pkgs, "cmd/micro")
	if cmdMicro == nil {
		return nil, fmt.Errorf("could not find cmd/micro package in loaded set")
	}

	// Find initlua.go within cmd/micro.
	var file *ast.File
	for _, f := range cmdMicro.Syntax {
		pos := cmdMicro.Fset.Position(f.Pos())
		if strings.HasSuffix(pos.Filename, "initlua.go") {
			file = f
			break
		}
	}
	if file == nil {
		return nil, fmt.Errorf("cmd/micro/initlua.go not found in package syntax")
	}

	regs := &registrations{}
	// Maintain deterministic module ordering.
	order := []string{"micro", "micro/buffer", "micro/config", "micro/shell", "micro/util"}
	byPath := map[string]*moduleRegistrations{}
	for _, p := range order {
		m := &moduleRegistrations{luaPath: p}
		for fn, lp := range luaImportFuncs {
			if lp == p {
				m.goFunc = fn
			}
		}
		byPath[p] = m
		regs.modules = append(regs.modules, m)
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		luaPath, isImporter := luaImportFuncs[fn.Name.Name]
		if !isImporter {
			continue
		}
		mod := byPath[luaPath]
		entries := walkSetFields(cmdMicro, fn.Body)
		mod.entries = entries
	}

	// Sort entries within each module for determinism.
	for _, m := range regs.modules {
		sort.SliceStable(m.entries, func(i, j int) bool {
			return m.entries[i].name < m.entries[j].name
		})
	}
	return regs, nil
}

// walkSetFields collects ulua.L.SetField(pkg, "<name>", luar.New(L, <expr>))
// calls inside the given function body.
func walkSetFields(pkg *packages.Package, body *ast.BlockStmt) []registration {
	var out []registration
	if body == nil {
		return out
	}
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "SetField" {
			return true
		}
		if len(call.Args) < 3 {
			return true
		}
		// Second arg is the Lua name as a string literal.
		nameLit, ok := call.Args[1].(*ast.BasicLit)
		if !ok {
			return true
		}
		name := strings.Trim(nameLit.Value, `"`)

		// Third arg is luar.New(ulua.L, <expr>). Unwrap it.
		expr := unwrapLuarNew(call.Args[2])
		if expr == nil {
			return true
		}
		var t types.Type
		if pkg.TypesInfo != nil {
			if tv, ok := pkg.TypesInfo.Types[expr]; ok {
				t = tv.Type
			}
		}
		out = append(out, registration{
			name:   name,
			goExpr: expr,
			goType: t,
		})
		return true
	})
	return out
}

// unwrapLuarNew detects a luar.New(L, X) call expression and returns X. If
// the input is not a luar.New call, it returns the input unchanged so the
// caller can still attempt to resolve it.
func unwrapLuarNew(e ast.Expr) ast.Expr {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return e
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return e
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return e
	}
	if pkg.Name != "luar" || sel.Sel.Name != "New" {
		return e
	}
	if len(call.Args) < 2 {
		return e
	}
	return call.Args[1]
}

// extractActions returns the keys of the BufKeyActions map literal in
// internal/action/bufpane.go, which is the canonical list of action verbs.
// Each verb is also the basis for pre<Verb>/on<Verb> hook names.
func extractActions(cfg *config, pkgs []*packages.Package) ([]string, error) {
	pkg := findPackage(pkgs, "internal/action")
	if pkg == nil {
		return nil, fmt.Errorf("could not find internal/action package")
	}
	var file *ast.File
	for _, f := range pkg.Syntax {
		pos := pkg.Fset.Position(f.Pos())
		if strings.HasSuffix(pos.Filename, "bufpane.go") {
			file = f
			break
		}
	}
	if file == nil {
		return nil, fmt.Errorf("internal/action/bufpane.go not found")
	}

	var keys []string
	ast.Inspect(file, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for i, name := range vs.Names {
			if name.Name != "BufKeyActions" {
				continue
			}
			if i >= len(vs.Values) {
				continue
			}
			cl, ok := vs.Values[i].(*ast.CompositeLit)
			if !ok {
				continue
			}
			for _, el := range cl.Elts {
				kv, ok := el.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				lit, ok := kv.Key.(*ast.BasicLit)
				if !ok {
					continue
				}
				keys = append(keys, strings.Trim(lit.Value, `"`))
			}
		}
		return true
	})
	if len(keys) == 0 {
		return nil, fmt.Errorf("BufKeyActions: no entries extracted")
	}
	sort.Strings(keys)
	return keys, nil
}
