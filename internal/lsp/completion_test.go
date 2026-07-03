package lsp

import (
	"encoding/json"
	"testing"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/lsp/protocol"
)

func TestDecodeCompletionResultNull(t *testing.T) {
	cases := []json.RawMessage{nil, json.RawMessage("null"), json.RawMessage("")}
	for _, raw := range cases {
		got := DecodeCompletionResult(raw, "", buffer.Loc{}, "utf-16")
		if got != nil {
			t.Errorf("DecodeCompletionResult(%q) = %+v, want nil", raw, got)
		}
	}
}

func TestDecodeCompletionResultBareArray(t *testing.T) {
	items := []protocol.CompletionItem{
		{Label: "foo"},
		{Label: "bar", Detail: "func bar()"},
	}
	raw, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}

	got := DecodeCompletionResult(raw, "", buffer.Loc{}, "utf-16")
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Label != "foo" || got[1].Label != "bar" || got[1].Detail != "func bar()" {
		t.Errorf("got = %+v, want labels foo/bar with bar's detail preserved", got)
	}
}

func TestDecodeCompletionResultList(t *testing.T) {
	list := protocol.CompletionList{
		IsIncomplete: true,
		Items: []protocol.CompletionItem{
			{Label: "alpha"},
		},
	}
	raw, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}

	got := DecodeCompletionResult(raw, "", buffer.Loc{}, "utf-16")
	if len(got) != 1 || got[0].Label != "alpha" {
		t.Fatalf("got = %+v, want one candidate labeled alpha", got)
	}
}

func TestDecodeCompletionResultEmptyList(t *testing.T) {
	raw := json.RawMessage(`{"isIncomplete":false,"items":[]}`)
	got := DecodeCompletionResult(raw, "", buffer.Loc{}, "utf-16")
	if got != nil {
		t.Errorf("DecodeCompletionResult(empty list) = %+v, want nil", got)
	}
}

func TestDecodeCompletionResultKeepsServerTextEdit(t *testing.T) {
	edit := protocol.TextEdit{
		Range:   protocol.Range{Start: protocol.Position{Line: 2, Character: 1}, End: protocol.Position{Line: 2, Character: 4}},
		NewText: "fmt.Println",
	}
	items := []protocol.CompletionItem{
		{Label: "Println", TextEdit: &edit},
	}
	raw, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}

	// lineText/cursor are irrelevant here: a server-supplied TextEdit
	// must be used verbatim, never overridden by the word-range
	// fallback.
	got := DecodeCompletionResult(raw, "xxxxxxxxxx", buffer.Loc{X: 5, Y: 2}, "utf-16")
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if len(got[0].Edits) != 1 || got[0].Edits[0] != edit {
		t.Errorf("Edits = %+v, want [%+v]", got[0].Edits, edit)
	}
}

func TestDecodeCompletionResultSynthesizesEditFromInsertText(t *testing.T) {
	items := []protocol.CompletionItem{
		{Label: "println", InsertText: "println"},
	}
	raw, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}

	// "  prin" with the cursor at X=6, inside/after the partially
	// typed word "prin" which starts at rune column 2.
	got := DecodeCompletionResult(raw, "  prin", buffer.Loc{X: 6, Y: 0}, "utf-16")
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	want := protocol.TextEdit{
		Range:   protocol.Range{Start: protocol.Position{Line: 0, Character: 2}, End: protocol.Position{Line: 0, Character: 6}},
		NewText: "println",
	}
	if len(got[0].Edits) != 1 || got[0].Edits[0] != want {
		t.Errorf("Edits = %+v, want [%+v]", got[0].Edits, want)
	}
}

func TestDecodeCompletionResultSynthesizesEditFallsBackToLabel(t *testing.T) {
	// No InsertText at all: the LSP spec says to fall back to Label.
	items := []protocol.CompletionItem{
		{Label: "println"},
	}
	raw, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}

	got := DecodeCompletionResult(raw, "prin", buffer.Loc{X: 4, Y: 0}, "utf-16")
	if len(got) != 1 || len(got[0].Edits) != 1 {
		t.Fatalf("got = %+v, want one candidate with one edit", got)
	}
	if got[0].Edits[0].NewText != "println" {
		t.Errorf("NewText = %q, want %q", got[0].Edits[0].NewText, "println")
	}
	wantRange := protocol.Range{Start: protocol.Position{Line: 0, Character: 0}, End: protocol.Position{Line: 0, Character: 4}}
	if got[0].Edits[0].Range != wantRange {
		t.Errorf("Range = %+v, want %+v", got[0].Edits[0].Range, wantRange)
	}
}

func TestWordRangeAt(t *testing.T) {
	cases := []struct {
		name      string
		line      string
		col       int
		wantStart int
		wantEnd   int
	}{
		{name: "cursor mid-word covers whole word", line: "  prin", col: 6, wantStart: 2, wantEnd: 6},
		{name: "cursor right after a word touches it", line: "foo bar", col: 3, wantStart: 0, wantEnd: 3},
		{name: "cursor in whitespace touches nothing", line: "foo  bar", col: 4, wantStart: 4, wantEnd: 4},
		{name: "cursor inside a longer word", line: "foobar", col: 3, wantStart: 0, wantEnd: 6},
		{name: "empty line", line: "", col: 0, wantStart: 0, wantEnd: 0},
		{name: "column past end clamps", line: "foo", col: 10, wantStart: 0, wantEnd: 3},
		{name: "negative column clamps", line: "foo", col: -1, wantStart: 0, wantEnd: 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start, end := wordRangeAt(c.line, c.col)
			if start != c.wantStart || end != c.wantEnd {
				t.Errorf("wordRangeAt(%q, %d) = (%d, %d), want (%d, %d)", c.line, c.col, start, end, c.wantStart, c.wantEnd)
			}
		})
	}
}
