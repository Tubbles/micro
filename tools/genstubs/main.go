// genstubs walks micro's Lua plugin API surface (the explicit registrations
// in cmd/micro/initlua.go plus the reflected method/field surface of the
// types those registrations expose) and emits LuaLS / lua-language-server
// type-stub files into a target directory.
//
// Usage:
//
//	go run ./tools/genstubs -repo <micro-checkout> -out runtime/meta/library
//
// The generator is intentionally independent of the main micro module:
// it has its own go.mod so the main project's dependencies stay clean.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

func main() {
	repo := flag.String("repo", ".", "path to the micro repository checkout")
	out := flag.String("out", "runtime/meta/library", "output directory (relative to -repo)")
	overrides := flag.String("overrides", "tools/genstubs/overrides/micro.yaml",
		"path to overrides YAML (relative to -repo)")
	verbose := flag.Bool("v", false, "verbose logging")
	flag.Parse()

	repoAbs, err := filepath.Abs(*repo)
	must(err)

	cfg := &config{
		repo:      repoAbs,
		outDir:    filepath.Join(repoAbs, *out),
		overrides: filepath.Join(repoAbs, *overrides),
		verbose:   *verbose,
	}

	if err := run(cfg); err != nil {
		log.Fatal(err)
	}
}

type config struct {
	repo      string
	outDir    string
	overrides string
	verbose   bool
}

func (c *config) logf(format string, args ...any) {
	if c.verbose {
		fmt.Fprintf(os.Stderr, format+"\n", args...)
	}
}

func run(cfg *config) error {
	if err := os.MkdirAll(cfg.outDir, 0o755); err != nil {
		return fmt.Errorf("mkdir out: %w", err)
	}

	pkgs, err := loadPackages(cfg)
	if err != nil {
		return fmt.Errorf("load packages: %w", err)
	}
	cfg.logf("loaded %d packages", len(pkgs))

	regs, err := extractRegistrations(cfg, pkgs)
	if err != nil {
		return fmt.Errorf("extract registrations: %w", err)
	}
	cfg.logf("found %d registrations across %d modules", regs.totalEntries(), len(regs.modules))

	actions, err := extractActions(cfg, pkgs)
	if err != nil {
		return fmt.Errorf("extract actions: %w", err)
	}
	cfg.logf("found %d action verbs in BufKeyActions", len(actions))

	ovr, err := loadOverrides(cfg.overrides)
	if err != nil {
		return fmt.Errorf("load overrides: %w", err)
	}

	closure := newTypeCloser(pkgs, ovr)
	for _, mod := range regs.modules {
		for _, e := range mod.entries {
			closure.seed(e.goType)
		}
	}
	for _, t := range seedExtraTypes() {
		closure.seedByName(t)
	}
	closure.run()
	cfg.logf("reached %d types via closure", closure.size())

	if err := emit(cfg, regs, actions, closure, ovr); err != nil {
		return fmt.Errorf("emit: %w", err)
	}
	cfg.logf("wrote stubs to %s", cfg.outDir)
	return nil
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
