package main

import (
	"fmt"

	"golang.org/x/tools/go/packages"
)

// loadPackages loads cmd/micro/* and internal/* with full type info so the
// generator can both AST-walk specific files and resolve method sets via
// go/types.
func loadPackages(cfg *config) ([]*packages.Package, error) {
	pcfg := &packages.Config{
		Mode: packages.NeedName |
			packages.NeedFiles |
			packages.NeedSyntax |
			packages.NeedTypes |
			packages.NeedTypesInfo |
			packages.NeedDeps |
			packages.NeedImports,
		Dir: cfg.repo,
	}
	pkgs, err := packages.Load(pcfg, "./cmd/micro/...", "./internal/...")
	if err != nil {
		return nil, err
	}
	hard := 0
	for _, p := range pkgs {
		for _, e := range p.Errors {
			if e.Kind == packages.ListError || e.Kind == packages.ParseError {
				hard++
				cfg.logf("package %s: %s", p.PkgPath, e)
			}
		}
	}
	if hard > 0 {
		return nil, fmt.Errorf("hard package errors: %d", hard)
	}
	return pkgs, nil
}

// findPackage returns the *packages.Package with the matching path suffix,
// or nil if not found.
func findPackage(pkgs []*packages.Package, suffix string) *packages.Package {
	for _, p := range pkgs {
		if p.PkgPath == suffix || pkgPathEndsWith(p.PkgPath, suffix) {
			return p
		}
	}
	return nil
}

func pkgPathEndsWith(path, suffix string) bool {
	if len(suffix) > len(path) {
		return false
	}
	if path == suffix {
		return true
	}
	return path[len(path)-len(suffix)-1:] == "/"+suffix
}
