package action

import (
	"encoding/json"
	"testing"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/lsp"
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

func TestCompletionItemsConvertsLabelAndDetail(t *testing.T) {
	candidates := []lsp.CompletionCandidate{
		{Label: "foo", Detail: "func foo()"},
		{Label: "bar"},
	}

	got := completionItems(candidates)

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Label != "foo" || got[0].Detail != "func foo()" {
		t.Errorf("got[0] = %+v, want Label=foo Detail=\"func foo()\"", got[0])
	}
	if got[1].Label != "bar" || got[1].Detail != "" {
		t.Errorf("got[1] = %+v, want Label=bar Detail=\"\"", got[1])
	}
}

func TestCompletionItemsEmpty(t *testing.T) {
	got := completionItems(nil)
	if len(got) != 0 {
		t.Errorf("completionItems(nil) = %+v, want empty", got)
	}
}

func TestCapabilityEnabled(t *testing.T) {
	cases := []struct {
		name string
		raw  json.RawMessage
		want bool
	}{
		{name: "absent field", raw: nil, want: false},
		{name: "explicit null", raw: json.RawMessage("null"), want: false},
		{name: "explicit false", raw: json.RawMessage("false"), want: false},
		{name: "explicit true", raw: json.RawMessage("true"), want: true},
		{name: "options object", raw: json.RawMessage(`{"prepareProvider":true}`), want: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := capabilityEnabled(c.raw); got != c.want {
				t.Errorf("capabilityEnabled(%s) = %v, want %v", c.raw, got, c.want)
			}
		})
	}
}

func TestFormattingOptionsFromSettings(t *testing.T) {
	cases := []struct {
		name     string
		settings map[string]any
		want     protocol.FormattingOptions
	}{
		{
			name:     "tabs, size 4",
			settings: map[string]any{"tabsize": float64(4), "tabstospaces": false},
			want:     protocol.FormattingOptions{TabSize: 4, InsertSpaces: false},
		},
		{
			name:     "spaces, size 2",
			settings: map[string]any{"tabsize": float64(2), "tabstospaces": true},
			want:     protocol.FormattingOptions{TabSize: 2, InsertSpaces: true},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := formattingOptionsFromSettings(c.settings); got != c.want {
				t.Errorf("formattingOptionsFromSettings(%+v) = %+v, want %+v", c.settings, got, c.want)
			}
		})
	}
}

func TestFormattingRangeOrdersReversedSelection(t *testing.T) {
	buf := buffer.NewBufferFromString("line one\nline two\n", "", buffer.BTDefault)
	// The cursor's selection can run in either direction; here end
	// comes before start in document order.
	selection := [2]buffer.Loc{{X: 5, Y: 1}, {X: 5, Y: 0}}

	rng, ok := formattingRange(buf, selection, "utf-16")
	if !ok {
		t.Fatal("formattingRange ok = false, want true")
	}
	want := protocol.Range{Start: pos(0, 5), End: pos(1, 5)}
	if rng != want {
		t.Errorf("formattingRange = %+v, want %+v", rng, want)
	}
}

func TestFormattingRangeOutOfBounds(t *testing.T) {
	buf := buffer.NewBufferFromString("short\n", "", buffer.BTDefault)
	selection := [2]buffer.Loc{{X: 0, Y: 0}, {X: 0, Y: 50}}

	if _, ok := formattingRange(buf, selection, "utf-16"); ok {
		t.Error("formattingRange ok = true for an out-of-bounds selection, want false")
	}
}

func TestFormatBeforeSave(t *testing.T) {
	cases := []struct {
		attached, formatOnSave, providesFormatting bool
		want                                       bool
	}{
		{attached: true, formatOnSave: true, providesFormatting: true, want: true},
		{attached: false, formatOnSave: true, providesFormatting: true, want: false},
		{attached: true, formatOnSave: false, providesFormatting: true, want: false},
		{attached: true, formatOnSave: true, providesFormatting: false, want: false},
		{attached: false, formatOnSave: false, providesFormatting: false, want: false},
	}
	for _, c := range cases {
		got := formatBeforeSave(c.attached, c.formatOnSave, c.providesFormatting)
		if got != c.want {
			t.Errorf("formatBeforeSave(%v, %v, %v) = %v, want %v", c.attached, c.formatOnSave, c.providesFormatting, got, c.want)
		}
	}
}
