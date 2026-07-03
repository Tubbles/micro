package tshighlight

import "testing"

// TestNewBufferNoGrammar checks that a filetype with no entry in
// filetypeLanguage (and so no gotreesitter grammar attempted) reports
// (nil, false), which is the signal internal/buffer uses to fall back
// to the regex highlighter.
func TestNewBufferNoGrammar(t *testing.T) {
	buf, ok := NewBuffer("brainfuck")
	if ok || buf != nil {
		t.Fatalf(`NewBuffer("brainfuck") = (%v, %v), want (nil, false)`, buf, ok)
	}
}

// TestGoHighlightRuneColumns parses a Go line containing a multi-byte
// rune (é, 2 UTF-8 bytes) before a later token on the same line, and
// checks that the token's LineMatch breakpoint lands on its rune
// column, not its byte offset. Getting this wrong is the classic
// off-by-N-bytes bug when wiring a byte-oriented parser into a
// rune-indexed highlighter.
func TestGoHighlightRuneColumns(t *testing.T) {
	// Line 2 (0-indexed): `var café = "hi"`.
	// Byte offset of the opening '"' is 12 (é costs 2 bytes); its rune
	// column is 11 (é is 1 rune). If byte->rune conversion is wrong,
	// the LineMatch breakpoint for the string capture would land on 12
	// instead.
	const src = "package main\n\nvar café = \"hi\"\n"

	buf, ok := NewBuffer("go")
	if !ok {
		t.Fatal(`NewBuffer("go") = false, want a grammar for "go"`)
	}
	buf.Reparse([]byte(src))

	line := buf.LineMatch(2)

	group, ok := line[11]
	if !ok {
		t.Fatalf("line 2 has no LineMatch breakpoint at rune column 11 (expected start of the string capture); got %v", line)
	}
	if want := "constant.string"; group.String() != want {
		t.Errorf("group at rune column 11 = %q, want %q", group.String(), want)
	}
	if _, atByteOffset := line[12]; atByteOffset {
		t.Errorf("line 2 has a LineMatch breakpoint at column 12, the string's BYTE offset; byte->rune conversion looks wrong: %v", line)
	}
}

// TestMarkdownGoFenceInjectionRuneColumns checks that a Go code fence
// inside a Markdown document is highlighted using the injected Go
// grammar (not left as plain Markdown text), and that the injected
// captures land at correct rune columns within their line, mirroring
// internal/tshighlight/spike_test.go's injection acceptance test but
// going through the production Buffer type and asserting on the
// resulting LineMatch instead of raw HighlightRanges.
func TestMarkdownGoFenceInjectionRuneColumns(t *testing.T) {
	const src = "# Title\n\n```go\nfunc main() {\n\tprintln(\"hi\")\n}\n```\n"
	// line 0: # Title
	// line 1: (blank)
	// line 2: ```go
	// line 3: func main() {
	// line 4: \tprintln("hi")
	// line 5: }
	// line 6: ```

	buf, ok := NewBuffer("markdown")
	if !ok {
		t.Fatal(`NewBuffer("markdown") = false, want a grammar for "markdown"`)
	}
	buf.Reparse([]byte(src))

	funcLine := buf.LineMatch(3)
	group, ok := funcLine[0]
	if !ok {
		t.Fatalf(`line 3 ("func main() {") has no LineMatch breakpoint at column 0; got %v`, funcLine)
	}
	if want := "statement"; group.String() != want {
		t.Errorf(`injected "func" keyword group = %q, want %q (markdown's own query never emits this capture, so seeing it proves the Go grammar ran)`, group.String(), want)
	}

	// `\tprintln("hi")`: the tab is a single byte and rune, so the
	// string's opening quote sits at rune column 9 in both byte and
	// rune terms; TestGoHighlightRuneColumns already covers the
	// multi-byte-rune case, this asserts the injected content is
	// found and mapped at all.
	printlnLine := buf.LineMatch(4)
	strGroup, ok := printlnLine[9]
	if !ok {
		t.Fatalf(`line 4 has no LineMatch breakpoint at column 9 (expected start of the injected string capture); got %v`, printlnLine)
	}
	if want := "constant.string"; strGroup.String() != want {
		t.Errorf("injected string group = %q, want %q", strGroup.String(), want)
	}
}
