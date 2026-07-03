package action

import (
	"encoding/json"
	"testing"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/lsp/protocol"
)

func TestFormatHoverMessage(t *testing.T) {
	cases := []struct {
		name  string
		hover protocol.Hover
		want  string
	}{
		{
			name:  "plain text",
			hover: protocol.Hover{Contents: protocol.MarkupContent{Kind: protocol.MarkupKindPlainText, Value: "func Foo() int"}},
			want:  "func Foo() int",
		},
		{
			name:  "collapses newlines and repeated whitespace",
			hover: protocol.Hover{Contents: protocol.MarkupContent{Kind: protocol.MarkupKindMarkdown, Value: "func Foo() int\n\n  returns 42\n"}},
			want:  "func Foo() int returns 42",
		},
		{
			name:  "zero value (null LSP result)",
			hover: protocol.Hover{},
			want:  "",
		},
		{
			name:  "whitespace-only content",
			hover: protocol.Hover{Contents: protocol.MarkupContent{Value: "   \n  "}},
			want:  "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := formatHoverMessage(c.hover); got != c.want {
				t.Errorf("formatHoverMessage(%+v) = %q, want %q", c.hover, got, c.want)
			}
		})
	}
}

func TestDecodeDefinitionLocation(t *testing.T) {
	single := protocol.Location{
		URI:   "file:///a.go",
		Range: protocol.Range{Start: protocol.Position{Line: 1, Character: 2}, End: protocol.Position{Line: 1, Character: 5}},
	}
	singleJSON, err := json.Marshal(single)
	if err != nil {
		t.Fatal(err)
	}

	multi := []protocol.Location{
		single,
		{URI: "file:///b.go", Range: protocol.Range{Start: protocol.Position{Line: 9, Character: 0}, End: protocol.Position{Line: 9, Character: 1}}},
	}
	multiJSON, err := json.Marshal(multi)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		raw    json.RawMessage
		want   protocol.Location
		wantOk bool
	}{
		{name: "null result", raw: json.RawMessage("null"), wantOk: false},
		{name: "empty result", raw: nil, wantOk: false},
		{name: "empty array", raw: json.RawMessage("[]"), wantOk: false},
		{name: "single location", raw: singleJSON, want: single, wantOk: true},
		{name: "array of locations takes the first", raw: multiJSON, want: multi[0], wantOk: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := decodeDefinitionLocation(c.raw)
			if ok != c.wantOk {
				t.Fatalf("decodeDefinitionLocation(%s) ok = %v, want %v", c.raw, ok, c.wantOk)
			}
			if ok && got != c.want {
				t.Errorf("decodeDefinitionLocation(%s) = %+v, want %+v", c.raw, got, c.want)
			}
		})
	}
}

// fakeGotoLocator records the Loc it was sent, standing in for a
// *BufPane so gotoLoc's position-conversion delegation can be tested
// without constructing a live screen/tab/pane.
type fakeGotoLocator struct {
	got buffer.Loc
}

func (f *fakeGotoLocator) GotoLoc(loc buffer.Loc) {
	f.got = loc
}

func TestGotoLoc(t *testing.T) {
	pane := &fakeGotoLocator{}
	lineText := "héllo world" // exercises a multi-byte rune ahead of the target column
	pos := protocol.Position{Line: 3, Character: 3}

	gotoLoc(pane, lineText, pos, "utf-16")

	want := buffer.Loc{X: 3, Y: 3}
	if pane.got != want {
		t.Errorf("gotoLoc delegated Loc = %v, want %v", pane.got, want)
	}
}
