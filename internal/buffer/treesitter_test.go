package buffer

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// TestBufferMatchTreesitterVsRegex checks that SharedBuffer.Match
// actually dispatches on the "treesitter" setting rather than always
// using one engine, and that tree-sitter's byte-offset captures land at
// the right RUNE column once routed through the real Match seam (not
// just tshighlight's own unit tests).
//
// Line 2 is `var café = 1`: café's é is a 2-byte, 1-rune UTF-8 character,
// so the '=' operator after it sits at byte offset 10 but rune column 9.
// If Match's byte->rune conversion were wrong, the breakpoint would land
// on column 10 instead.
//
// "var" is also a convenient marker for which engine ran: go.yaml's
// regex rules put it in the "preproc" group, while the tree-sitter Go
// grammar captures it as "@keyword", which internal/tshighlight's
// mapping table sends to "statement" (see internal/tshighlight/mapping.go).
// Different group names from the same source line is proof Match
// genuinely switched engines, not just that both happened to agree.
func TestBufferMatchTreesitterVsRegex(t *testing.T) {
	const src = "package main\n\nvar café = 1\n"
	b := NewBufferFromString(src, "main.go", BTDefault)
	if ft, _ := b.Settings["filetype"].(string); ft != "go" {
		t.Fatalf("buffer filetype = %q, want %q (path-based filetype detection)", ft, "go")
	}

	b.Settings["treesitter"] = true
	tsMatch := b.Match(2)
	tsKeyword, ok := tsMatch[0]
	if !ok {
		t.Fatalf("Match(2) with treesitter=true has no entry at column 0; got %v", tsMatch)
	}
	if want := "statement"; tsKeyword.String() != want {
		t.Errorf(`tree-sitter group for "var" = %q, want %q`, tsKeyword.String(), want)
	}
	// "café" is a rune 4..8 (byte 4..9, since é is 2 bytes): its
	// variable capture should start at rune column 4 and its
	// end-of-capture reset should land on rune column 8, not byte
	// column 9. The '=' operator right after it should then start at
	// rune column 9, not byte column 10. Getting byte->rune conversion
	// wrong shifts both of these by one.
	if variable, ok := tsMatch[4]; !ok || variable.String() != "identifier.var" {
		t.Errorf(`Match(2)[4] = (%v, %v), want ("identifier.var", true) for café's start; got %v`, variable, ok, tsMatch)
	}
	if reset, ok := tsMatch[8]; !ok || reset.String() != "" {
		t.Errorf(`Match(2)[8] = (%v, %v), want (default, true) for café's end (rune column 8, not byte column 9); got %v`, reset, ok, tsMatch)
	}
	if operator, ok := tsMatch[9]; !ok || operator.String() != "operator" {
		t.Errorf(`Match(2)[9] = (%v, %v), want ("operator", true) for '=' (rune column 9, not byte column 10); got %v`, operator, ok, tsMatch)
	}

	b.Settings["treesitter"] = false
	regexMatch := b.Match(2)
	regexKeyword, ok := regexMatch[0]
	if !ok {
		t.Fatalf("Match(2) with treesitter=false has no entry at column 0; got %v", regexMatch)
	}
	if want := "preproc"; regexKeyword.String() != want {
		t.Errorf(`regex group for "var" = %q, want %q (go.yaml's preproc pattern)`, regexKeyword.String(), want)
	}
}

// TestBufferMatchNoGrammarFallsBackToRegex checks that Match falls
// through to the regex engine's own LineArray.Match unchanged when
// treesitter is on but tshighlight has no grammar for the buffer's
// filetype (here "unknown", the default syntax, which is deliberately
// absent from tshighlight's filetype table).
func TestBufferMatchNoGrammarFallsBackToRegex(t *testing.T) {
	b := NewBufferFromString("hello world\n", "", BTDefault)
	if ft, _ := b.Settings["filetype"].(string); ft != "unknown" {
		t.Fatalf("buffer filetype = %q, want %q", ft, "unknown")
	}
	b.Settings["treesitter"] = true

	got := b.Match(0)
	want := b.LineArray.Match(0)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Match(0) = %v with treesitter=true and no grammar, want the regex engine's own %v", got, want)
	}
}

// genGoSource builds a syntactically valid, repetitive Go source file
// with nFuncs small functions, for benchmarking highlight cost on a
// large-ish buffer.
func genGoSource(nFuncs int) string {
	var sb strings.Builder
	sb.WriteString("package bench\n\n")
	for i := 0; i < nFuncs; i++ {
		fmt.Fprintf(&sb, "func f%d(x int) int {\n\treturn x + %d\n}\n\n", i, i)
	}
	return sb.String()
}

// benchmarkEditAndMatch simulates the cost pattern a real edit session
// puts on the highlight engine: an edit signal (MarkModified) followed
// by re-drawing a viewport's worth of lines (Match), repeated b.N times,
// on a ~2000-line Go buffer.
func benchmarkEditAndMatch(b *testing.B, treesitter bool) {
	src := genGoSource(500)
	buf := NewBufferFromString(src, "bench.go", BTDefault)
	buf.Settings["treesitter"] = treesitter

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.MarkModified(10, 10)
		for y := 10; y < 40; y++ {
			buf.Match(y)
		}
	}
}

// BenchmarkTreesitterEditGoBuffer and BenchmarkRegexEditGoBuffer
// compare highlight cost with the "treesitter" setting on vs off,
// editing near the top of a large-ish Go buffer and re-reading a
// 30-line viewport, the pattern a real render pass follows after a
// keystroke. Run with:
//
//	go test ./internal/buffer/... -bench 'EditGoBuffer' -benchmem -run '^$'
func BenchmarkTreesitterEditGoBuffer(b *testing.B) { benchmarkEditAndMatch(b, true) }
func BenchmarkRegexEditGoBuffer(b *testing.B)      { benchmarkEditAndMatch(b, false) }
