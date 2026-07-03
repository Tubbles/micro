package action

import (
	"encoding/json"
	"errors"
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

func TestWorkspaceEditFilesPrefersDocumentChanges(t *testing.T) {
	edit := protocol.WorkspaceEdit{
		Changes: map[protocol.DocumentURI][]protocol.TextEdit{
			"file:///only-in-changes.go": {{NewText: "ignored"}},
		},
		DocumentChanges: []protocol.TextDocumentEdit{
			{
				TextDocument: protocol.OptionalVersionedTextDocumentIdentifier{URI: "file:///a.go"},
				Edits:        []protocol.TextEdit{{NewText: "a"}},
			},
			{
				TextDocument: protocol.OptionalVersionedTextDocumentIdentifier{URI: "file:///b.go"},
				Edits:        []protocol.TextEdit{{NewText: "b"}},
			},
		},
	}

	got := workspaceEditFiles(edit)
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].URI != "file:///a.go" || got[1].URI != "file:///b.go" {
		t.Errorf("got = %+v, want URIs a.go then b.go, DocumentChanges order preserved", got)
	}
}

func TestWorkspaceEditFilesFallsBackToChanges(t *testing.T) {
	edit := protocol.WorkspaceEdit{
		Changes: map[protocol.DocumentURI][]protocol.TextEdit{
			"file:///only.go": {{NewText: "x"}},
		},
	}
	got := workspaceEditFiles(edit)
	if len(got) != 1 || got[0].URI != "file:///only.go" {
		t.Errorf("got = %+v, want one file:///only.go entry", got)
	}
}

func TestWorkspaceEditFilesEmpty(t *testing.T) {
	got := workspaceEditFiles(protocol.WorkspaceEdit{})
	if len(got) != 0 {
		t.Errorf("got = %+v, want empty", got)
	}
}

// TestApplyRenameEditMultiFile covers dispatch across two already-open
// buffers: applyRenameEdit must route each file's edits to the right
// buffer via the injected lookupBuffer, and never fall through to
// openBuffer for a path lookupBuffer already resolved.
func TestApplyRenameEditMultiFile(t *testing.T) {
	pathA, err := lsp.PathFromURI("file:///a.go")
	if err != nil {
		t.Fatal(err)
	}
	pathB, err := lsp.PathFromURI("file:///b.go")
	if err != nil {
		t.Fatal(err)
	}

	bufA := buffer.NewBufferFromString("foo\n", pathA, buffer.BTDefault)
	bufB := buffer.NewBufferFromString("bar\n", pathB, buffer.BTDefault)

	open := map[string]*buffer.Buffer{pathA: bufA, pathB: bufB}
	lookupBuffer := func(path string) *buffer.Buffer { return open[path] }
	openBuffer := func(path string) (*buffer.Buffer, error) {
		t.Fatalf("openBuffer called for %q, want lookupBuffer to have resolved it", path)
		return nil, nil
	}

	edit := protocol.WorkspaceEdit{
		Changes: map[protocol.DocumentURI][]protocol.TextEdit{
			"file:///a.go": {{Range: protocol.Range{Start: pos(0, 0), End: pos(0, 3)}, NewText: "FOO"}},
			"file:///b.go": {{Range: protocol.Range{Start: pos(0, 0), End: pos(0, 3)}, NewText: "BAR"}},
		},
	}

	touched, err := applyRenameEdit(edit, "utf-16", lookupBuffer, openBuffer)
	if err != nil {
		t.Fatalf("applyRenameEdit error: %v", err)
	}
	if touched != 2 {
		t.Errorf("touched = %d, want 2", touched)
	}
	if got := string(bufA.Bytes()); got != "FOO\n" {
		t.Errorf("bufA.Bytes() = %q, want %q", got, "FOO\n")
	}
	if got := string(bufB.Bytes()); got != "BAR\n" {
		t.Errorf("bufB.Bytes() = %q, want %q", got, "BAR\n")
	}
}

// TestApplyRenameEditOpensClosedFile covers the not-open-yet path:
// applyRenameEdit must call openBuffer and apply edits to the buffer
// it returns.
func TestApplyRenameEditOpensClosedFile(t *testing.T) {
	closedBuf := buffer.NewBufferFromString("baz\n", "", buffer.BTDefault)
	opened := false
	lookupBuffer := func(path string) *buffer.Buffer { return nil }
	openBuffer := func(path string) (*buffer.Buffer, error) {
		opened = true
		return closedBuf, nil
	}

	edit := protocol.WorkspaceEdit{
		Changes: map[protocol.DocumentURI][]protocol.TextEdit{
			"file:///closed.go": {{Range: protocol.Range{Start: pos(0, 0), End: pos(0, 3)}, NewText: "BAZ"}},
		},
	}

	touched, err := applyRenameEdit(edit, "utf-16", lookupBuffer, openBuffer)
	if err != nil {
		t.Fatalf("applyRenameEdit error: %v", err)
	}
	if !opened {
		t.Error("openBuffer was not called for a file lookupBuffer could not resolve")
	}
	if touched != 1 {
		t.Errorf("touched = %d, want 1", touched)
	}
	if got := string(closedBuf.Bytes()); got != "BAZ\n" {
		t.Errorf("closedBuf.Bytes() = %q, want %q", got, "BAZ\n")
	}
}

// TestApplyRenameEditContinuesPastOpenError covers a file that fails
// to open: the error is reported but the remaining files are still
// touched, rather than the whole rename aborting on the first failure.
func TestApplyRenameEditContinuesPastOpenError(t *testing.T) {
	pathB, err := lsp.PathFromURI("file:///b.go")
	if err != nil {
		t.Fatal(err)
	}
	bufB := buffer.NewBufferFromString("bar\n", pathB, buffer.BTDefault)
	open := map[string]*buffer.Buffer{pathB: bufB}
	lookupBuffer := func(path string) *buffer.Buffer { return open[path] }

	wantErr := errors.New("permission denied")
	openBuffer := func(path string) (*buffer.Buffer, error) {
		return nil, wantErr
	}

	edit := protocol.WorkspaceEdit{
		DocumentChanges: []protocol.TextDocumentEdit{
			{
				TextDocument: protocol.OptionalVersionedTextDocumentIdentifier{URI: "file:///unreadable.go"},
				Edits:        []protocol.TextEdit{{NewText: "x"}},
			},
			{
				TextDocument: protocol.OptionalVersionedTextDocumentIdentifier{URI: "file:///b.go"},
				Edits:        []protocol.TextEdit{{Range: protocol.Range{Start: pos(0, 0), End: pos(0, 3)}, NewText: "BAR"}},
			},
		},
	}

	touched, gotErr := applyRenameEdit(edit, "utf-16", lookupBuffer, openBuffer)
	if !errors.Is(gotErr, wantErr) {
		t.Errorf("applyRenameEdit error = %v, want %v", gotErr, wantErr)
	}
	if touched != 1 {
		t.Errorf("touched = %d, want 1 (the file that did open)", touched)
	}
	if got := string(bufB.Bytes()); got != "BAR\n" {
		t.Errorf("bufB.Bytes() = %q, want %q", got, "BAR\n")
	}
}
