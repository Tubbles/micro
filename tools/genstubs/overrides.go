package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// overrides is the manual correction layer applied on top of the AST/types
// extraction. Currently supports renaming a Go type to a different LuaCATS
// class name and forcing inclusion of types that the closure walker misses.
type overrides struct {
	// TypeNames remaps "<pkg>.<TypeName>" -> "<lua-class-name>".
	// e.g. "github.com/Tubbles/tcell/v3.EventMouse" -> "EventMouse".
	TypeNames map[string]string `yaml:"type_names"`

	// IncludeTypes adds short type names to the closure pending queue.
	IncludeTypes []string `yaml:"include_types"`

	// SkipMethods prevents emission of specific methods. Keyed by class
	// short name, value is a list of method names to omit.
	SkipMethods map[string][]string `yaml:"skip_methods"`

	// ExtraFields adds @field entries to a class verbatim. Keyed by class
	// short name; each value is a raw "name type" string written into the
	// @field line.
	ExtraFields map[string][]string `yaml:"extra_fields"`
}

func loadOverrides(path string) (*overrides, error) {
	o := &overrides{
		TypeNames:    map[string]string{},
		SkipMethods:  map[string][]string{},
		ExtraFields:  map[string][]string{},
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return o, nil
		}
		return nil, err
	}
	if err := yaml.Unmarshal(data, o); err != nil {
		return nil, fmt.Errorf("parse overrides %s: %w", path, err)
	}
	if o.TypeNames == nil {
		o.TypeNames = map[string]string{}
	}
	if o.SkipMethods == nil {
		o.SkipMethods = map[string][]string{}
	}
	if o.ExtraFields == nil {
		o.ExtraFields = map[string][]string{}
	}
	return o, nil
}

func (o *overrides) typeName(pkgPath, typeName string) string {
	return o.TypeNames[pkgPath+"."+typeName]
}

func (o *overrides) skipMethod(class, method string) bool {
	for _, m := range o.SkipMethods[class] {
		if m == method {
			return true
		}
	}
	return false
}

func (o *overrides) extraFields(class string) []string {
	return o.ExtraFields[class]
}
