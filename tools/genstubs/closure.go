package main

import (
	"go/types"
	"sort"

	"golang.org/x/tools/go/packages"
)

// typeCloser walks the type graph from seed types, collecting every named
// in-tree type whose @class needs to be emitted. It also records the public
// fields and method sets so emit.go can write them out.
type typeCloser struct {
	pkgs     []*packages.Package
	ovr      *overrides
	pending  []types.Type
	seen     map[string]bool         // by package-qualified name
	classes  map[string]*classRecord // ditto
	byShort  map[string]*classRecord // by short name (for emit ordering)
	resolved map[string]bool
}

type classRecord struct {
	name     string // bare type name, e.g. "BufPane"
	pkgPath  string
	doc      string
	fields   []fieldRecord
	methods  []methodRecord
	embedded []string // names of embedded types, for @class inheritance
}

type fieldRecord struct {
	name string
	typ  string
	doc  string
}

type methodRecord struct {
	name string
	sig  string // "fun(self: T, x: integer): boolean"
	doc  string
}

func newTypeCloser(pkgs []*packages.Package, ovr *overrides) *typeCloser {
	return &typeCloser{
		pkgs:     pkgs,
		ovr:      ovr,
		seen:     map[string]bool{},
		classes:  map[string]*classRecord{},
		byShort:  map[string]*classRecord{},
		resolved: map[string]bool{},
	}
}

func (c *typeCloser) size() int { return len(c.classes) }

// seed adds a type to the closure pending queue.
func (c *typeCloser) seed(t types.Type) {
	if t == nil {
		return
	}
	switch tt := t.(type) {
	case *types.Pointer:
		c.seed(tt.Elem())
	case *types.Slice:
		c.seed(tt.Elem())
	case *types.Map:
		c.seed(tt.Key())
		c.seed(tt.Elem())
	case *types.Named:
		obj := tt.Obj()
		if obj.Pkg() == nil {
			return
		}
		if !isAllowedPackage(obj.Pkg().Path()) {
			return
		}
		key := obj.Pkg().Path() + "." + obj.Name()
		if c.seen[key] {
			return
		}
		c.seen[key] = true
		c.pending = append(c.pending, t)
	case *types.Signature:
		// signatures don't reach types directly here
	}
}

// seedByName forces inclusion of a named type by short name. Used by overrides
// and the explicit seed list (seed.go).
func (c *typeCloser) seedByName(name string) {
	for _, p := range c.pkgs {
		obj := p.Types.Scope().Lookup(name)
		if obj == nil {
			continue
		}
		tn, ok := obj.(*types.TypeName)
		if !ok {
			continue
		}
		c.seed(tn.Type())
		return
	}
}

func (c *typeCloser) run() {
	for len(c.pending) > 0 {
		t := c.pending[0]
		c.pending = c.pending[1:]
		c.process(t)
	}
}

func (c *typeCloser) process(t types.Type) {
	named, ok := t.(*types.Named)
	if !ok {
		return
	}
	obj := named.Obj()
	key := obj.Pkg().Path() + "." + obj.Name()
	if c.resolved[key] {
		return
	}
	c.resolved[key] = true

	rec := &classRecord{
		name:    obj.Name(),
		pkgPath: obj.Pkg().Path(),
	}

	// Walk fields if it's a struct (or has a struct underlying).
	if st, ok := named.Underlying().(*types.Struct); ok {
		for i := 0; i < st.NumFields(); i++ {
			f := st.Field(i)
			if !f.Exported() {
				continue
			}
			if f.Embedded() {
				// Track for @class inheritance, but also flatten. Only
				// record the embedded name when the type is in an
				// allowed package; otherwise the inheritance line ends
				// up referencing a class we never emit, and LuaLS
				// flags it as undefined.
				ft := f.Type()
				if pt, ok := ft.(*types.Pointer); ok {
					ft = pt.Elem()
				}
				if nt, ok := ft.(*types.Named); ok {
					if nt.Obj().Pkg() != nil && isAllowedPackage(nt.Obj().Pkg().Path()) {
						rec.embedded = append(rec.embedded, nt.Obj().Name())
						c.seed(ft)
					}
				}
				continue
			}
			rec.fields = append(rec.fields, fieldRecord{
				name: f.Name(),
				typ:  luaTypeOf(f.Type(), c, c.ovr),
			})
		}
	}

	// Walk method set on the pointer receiver — that's what gopher-luar
	// uses, and it's a superset of the value-receiver method set.
	mset := types.NewMethodSet(types.NewPointer(named))
	for i := 0; i < mset.Len(); i++ {
		m := mset.At(i)
		fn, ok := m.Obj().(*types.Func)
		if !ok || !fn.Exported() {
			continue
		}
		sig, ok := fn.Type().(*types.Signature)
		if !ok {
			continue
		}
		// Render the signature with self-parameter.
		selfType := obj.Name()
		paramStr := renderParamList(sig, c, c.ovr)
		retStr := renderReturnList(sig, c, c.ovr)
		full := "fun(self: " + selfType
		if paramStr != "" {
			full += ", " + paramStr
		}
		full += ")"
		if retStr != "" {
			full += ": " + retStr
		}
		rec.methods = append(rec.methods, methodRecord{
			name: fn.Name(),
			sig:  full,
		})
	}

	sort.Slice(rec.fields, func(i, j int) bool { return rec.fields[i].name < rec.fields[j].name })
	sort.Slice(rec.methods, func(i, j int) bool { return rec.methods[i].name < rec.methods[j].name })

	c.classes[key] = rec
	if existing, ok := c.byShort[obj.Name()]; ok {
		// If two packages export a type with the same short name, prefer
		// the one already recorded; the rare collision can be patched via
		// overrides.
		_ = existing
	} else {
		c.byShort[obj.Name()] = rec
	}
}

func (c *typeCloser) classByShort(name string) *classRecord {
	return c.byShort[name]
}

func renderParamList(sig *types.Signature, closure *typeCloser, ovr *overrides) string {
	params := sig.Params()
	var ps []string
	for i := 0; i < params.Len(); i++ {
		p := params.At(i)
		name := p.Name()
		if name == "" {
			name = "a"
		}
		if sig.Variadic() && i == params.Len()-1 {
			if slice, ok := p.Type().(*types.Slice); ok {
				ps = append(ps, "...: "+luaTypeOf(slice.Elem(), closure, ovr))
				continue
			}
		}
		ps = append(ps, luaSafeName(name)+": "+luaTypeOf(p.Type(), closure, ovr))
	}
	return joinComma(ps)
}

func renderReturnList(sig *types.Signature, closure *typeCloser, ovr *overrides) string {
	res := sig.Results()
	var rs []string
	for i := 0; i < res.Len(); i++ {
		rs = append(rs, luaTypeOf(res.At(i).Type(), closure, ovr))
	}
	return joinComma(rs)
}

func joinComma(xs []string) string {
	out := ""
	for i, x := range xs {
		if i > 0 {
			out += ", "
		}
		out += x
	}
	return out
}

// orderedClasses returns classes sorted by short name for deterministic
// emit output.
func (c *typeCloser) orderedClasses() []*classRecord {
	out := make([]*classRecord, 0, len(c.classes))
	for _, r := range c.classes {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}
