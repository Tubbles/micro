// Package tshighlight is the landing spot for micro's tree-sitter
// highlighting work. This file is the go/no-go spike required by D-47 in
// work/roadmap-2026-07-decisions.md before any editor wiring is built
// against github.com/odvcencio/gotreesitter (a pure-Go, no-cgo
// reimplementation of the tree-sitter runtime). There is deliberately no
// production .go file in this package yet.
//
// Grammars: gotreesitter embeds all 206 of its bundled grammars in the
// binary by default (no build tags, no cgo, no on-disk grammar files, no
// GOTREESITTER_GRAMMAR_BLOB_DIR needed). A language's parser is reached
// through the generated grammars.<Lang>Language() constructor (e.g.
// grammars.GoLanguage()), and its bundled tree-sitter highlight query
// (.scm-format) is reached through
// grammars.DetectLanguageByName(name).HighlightQuery. Both are used as-is
// below; this spike does not hand-write any queries.
package tshighlight

import (
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// hasCapture reports whether ranges contains a capture named name whose
// source text equals text.
func hasCapture(ranges []gotreesitter.HighlightRange, src, name, text string) bool {
	for _, r := range ranges {
		if r.Capture == name && src[r.StartByte:r.EndByte] == text {
			return true
		}
	}
	return false
}

// TestSpikeGoHighlight parses a small Go snippet with gotreesitter's bundled
// Go grammar and highlight query, and checks that the highlighter produces
// sane captures: a keyword, an identifier, and a string literal, each over
// the expected source text. This is the base-highlighting go/no-go: if this
// fails, injections are moot.
func TestSpikeGoHighlight(t *testing.T) {
	const src = `package main

import "fmt"

func main() {
	message := "hello"
	fmt.Println(message)
}
`
	entry := grammars.DetectLanguageByName("go")
	if entry == nil {
		t.Fatal(`grammars.DetectLanguageByName("go") returned nil`)
	}
	if strings.TrimSpace(entry.HighlightQuery) == "" {
		t.Fatal(`go language entry has no bundled HighlightQuery`)
	}

	lang := grammars.GoLanguage()
	if lang == nil {
		t.Fatal("grammars.GoLanguage() returned nil")
	}

	hl, err := gotreesitter.NewHighlighter(lang, entry.HighlightQuery)
	if err != nil {
		t.Fatalf("NewHighlighter: %v", err)
	}

	ranges := hl.Highlight([]byte(src))
	if len(ranges) == 0 {
		t.Fatal("expected highlight ranges, got none")
	}

	// Note: gotreesitter's overlap resolver breaks ties between
	// equal-width captures on the same node by later-registered pattern
	// wins, not query source order. Empirically (verified by running the
	// highlighter directly) that means the "main" function-name identifier
	// resolves to a plain @variable capture, not @function, because the
	// generic "(identifier) @variable" pattern is registered after the
	// "(function_declaration name: (identifier) @function)" pattern. Assert
	// on "message" (an unambiguous identifier) instead of "main" to avoid
	// depending on that resolution quirk.
	if !hasCapture(ranges, src, "keyword", "func") {
		t.Errorf(`expected a @keyword capture over "func"; ranges=%+v`, ranges)
	}
	if !hasCapture(ranges, src, "variable", "message") {
		t.Errorf(`expected a @variable capture over "message"; ranges=%+v`, ranges)
	}
	if !hasCapture(ranges, src, "string", `"hello"`) {
		t.Errorf(`expected a @string capture over %q; ranges=%+v`, `"hello"`, ranges)
	}
}

// TestSpikeMarkdownInjection parses a Markdown document with a fenced Go
// code block and checks that gotreesitter's built-in markdown-fence
// injection (registered automatically for the "markdown" parent language
// by grammars/markdown_injection_register.go's init(), via
// gotreesitter.RegisterHighlighterInjection) resolves the "go" info-string
// hint and produces Go-language captures inside the fenced block. This is
// the markdown-injection acceptance test the whole tree-sitter-highlighting
// feature hinges on: micro's main use case is Go/Lua/etc. code fences
// inside Markdown help docs and README-style buffers.
//
// No manual gotreesitter.InjectionParser wiring is needed for this case:
// the Highlighter type resolves injections for any parent language that has
// a registered HighlighterInjectionSpec (markdown is one out of the box),
// so a single hl.Highlight(source) call over the whole Markdown document is
// enough to get both the Markdown-level and the injected Go-level captures
// back in one slice, at document-relative byte offsets.
func TestSpikeMarkdownInjection(t *testing.T) {
	const src = "# Title\n\nSome text.\n\n```go\nfunc main() {\n\tprintln(\"hi\")\n}\n```\n\nMore text.\n"

	entry := grammars.DetectLanguageByName("markdown")
	if entry == nil {
		t.Fatal(`grammars.DetectLanguageByName("markdown") returned nil`)
	}
	if strings.TrimSpace(entry.HighlightQuery) == "" {
		t.Fatal(`markdown language entry has no bundled HighlightQuery`)
	}

	lang := grammars.MarkdownLanguage()
	if lang == nil {
		t.Fatal("grammars.MarkdownLanguage() returned nil")
	}

	hl, err := gotreesitter.NewHighlighter(lang, entry.HighlightQuery)
	if err != nil {
		t.Fatalf("NewHighlighter(markdown): %v", err)
	}

	ranges := hl.Highlight([]byte(src))
	if len(ranges) == 0 {
		t.Fatal("expected highlight ranges for the markdown document, got none")
	}

	fenceContentStart := strings.Index(src, "func main")
	fenceContentEnd := strings.Index(src, "```\n\nMore")
	if fenceContentStart < 0 || fenceContentEnd < 0 {
		t.Fatal("test fixture offsets not found (fixture changed?)")
	}

	// The markdown highlight query on its own never emits a "keyword" or
	// "string" capture (its capture names are things like text.title,
	// text.literal, punctuation.delimiter). Finding those capture names
	// inside the fenced byte range is proof the child Go grammar actually
	// ran over the injected content, not just that the parent query
	// happened to reuse a capture name.
	var sawInjectedKeyword, sawInjectedString bool
	for _, r := range ranges {
		if int(r.StartByte) < fenceContentStart || int(r.EndByte) > fenceContentEnd {
			continue
		}
		text := src[r.StartByte:r.EndByte]
		switch {
		case r.Capture == "keyword" && text == "func":
			sawInjectedKeyword = true
		case r.Capture == "string" && text == `"hi"`:
			sawInjectedString = true
		}
	}

	if !sawInjectedKeyword {
		t.Errorf(`expected an injected @keyword capture over "func" inside the fenced block; ranges=%+v`, ranges)
	}
	if !sawInjectedString {
		t.Errorf(`expected an injected @string capture over %q inside the fenced block; ranges=%+v`, `"hi"`, ranges)
	}
}
