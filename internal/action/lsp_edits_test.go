package action

import (
	"testing"

	"github.com/stretchr/testify/assert"
	lua "github.com/yuin/gopher-lua"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/lsp/protocol"
	ulua "github.com/micro-editor/micro/v2/internal/lua"
)

// applyTextEdits tests construct real *buffer.Buffer values, which
// needs the same minimal runtime setup buffer's own tests perform
// (see internal/buffer/buffer_test.go's init).
func init() {
	ulua.L = lua.NewState()
	config.InitRuntimeFiles(false)
	config.InitGlobalSettings()
	config.GlobalSettings["backup"] = false
	config.GlobalSettings["fastdirty"] = true
}

func pos(line, char uint32) protocol.Position {
	return protocol.Position{Line: line, Character: char}
}

// TestApplyTextEditsDescendingOrderRequired constructs two edits on
// the same line that would corrupt each other if applied in ascending
// (document) order: the first edit changes the line's length before
// the second edit's column offsets are consumed. Applying them
// bottom-most/right-most-first (as applyTextEdits does) keeps both
// ranges valid against the original line for as long as they're
// needed.
func TestApplyTextEditsDescendingOrderRequired(t *testing.T) {
	buf := buffer.NewBufferFromString("foo bar baz\n", "", buffer.BTDefault)

	edits := []protocol.TextEdit{
		// Replaces "foo" (cols 0-3) with a longer string, which would
		// shift every column after it if applied first.
		{Range: protocol.Range{Start: pos(0, 0), End: pos(0, 3)}, NewText: "FOOOOOO"},
		// Replaces "baz" (cols 8-11). If the edit above ran first
		// (ascending order) without the caller re-deriving this
		// range, this edit's column offsets (computed against the
		// original line) would land in the wrong place after the
		// first edit shifted the line.
		{Range: protocol.Range{Start: pos(0, 8), End: pos(0, 11)}, NewText: "BAZZZZZ"},
	}

	applyTextEdits(buf, edits, "utf-16")

	want := "FOOOOOO bar BAZZZZZ\n"
	if got := string(buf.Bytes()); got != want {
		t.Errorf("buf.Bytes() = %q, want %q", got, want)
	}
}

func TestApplyTextEditsMultiLineSpan(t *testing.T) {
	buf := buffer.NewBufferFromString("line one\nline two\nline three\n", "", buffer.BTDefault)

	edits := []protocol.TextEdit{
		// Replaces from the middle of line 0 through the middle of
		// line 2, collapsing three lines into one.
		{
			Range:   protocol.Range{Start: pos(0, 5), End: pos(2, 5)},
			NewText: "ONE",
		},
	}

	applyTextEdits(buf, edits, "utf-16")

	want := "line ONEthree\n"
	if got := string(buf.Bytes()); got != want {
		t.Errorf("buf.Bytes() = %q, want %q", got, want)
	}
}

func TestApplyTextEditsEmpty(t *testing.T) {
	buf := buffer.NewBufferFromString("unchanged\n", "", buffer.BTDefault)
	applyTextEdits(buf, nil, "utf-16")
	if got := string(buf.Bytes()); got != "unchanged\n" {
		t.Errorf("buf.Bytes() = %q, want unchanged", got)
	}
}

// wholeDocumentEdit is the shape most formatters answer with: a single
// edit whose range covers the entire original document.
func wholeDocumentEdit(buf *buffer.Buffer, newText string) []protocol.TextEdit {
	end := buf.End()
	return []protocol.TextEdit{{
		Range:   protocol.Range{Start: pos(0, 0), End: pos(uint32(end.Y), uint32(end.X))},
		NewText: newText,
	}}
}

func TestApplyFormattingEditsKeepsCursorOnItsLine(t *testing.T) {
	buf := buffer.NewBufferFromString("func main() {\nfmt.Println(\"hi\")\nreturn\n}\n", "", buffer.BTDefault)
	cursor := buf.GetActiveCursor()
	cursor.GotoLoc(buffer.Loc{2, 2})

	formatted := "func main() {\n\tfmt.Println(\"hi\")\n\treturn\n}\n"
	applyFormattingEdits(buf, wholeDocumentEdit(buf, formatted), "utf-16")

	assert.Equal(t, formatted, string(buf.Bytes()))
	// Only the tab inserted on the cursor's own line moved it.
	assert.Equal(t, buffer.Loc{3, 2}, cursor.Loc)
}

func TestApplyFormattingEditsShiftsCursorPastInsertedLines(t *testing.T) {
	buf := buffer.NewBufferFromString("a\nb\nc\n", "", buffer.BTDefault)
	cursor := buf.GetActiveCursor()
	cursor.GotoLoc(buffer.Loc{1, 2})

	applyFormattingEdits(buf, wholeDocumentEdit(buf, "a\n\n\nb\nc\n"), "utf-16")

	assert.Equal(t, "a\n\n\nb\nc\n", string(buf.Bytes()))
	assert.Equal(t, buffer.Loc{1, 4}, cursor.Loc)
}

func TestApplyFormattingEditsHandlesMultiByteText(t *testing.T) {
	buf := buffer.NewBufferFromString("héllo wörld\n", "", buffer.BTDefault)
	cursor := buf.GetActiveCursor()
	cursor.GotoLoc(buffer.Loc{8, 0})

	edits := []protocol.TextEdit{{
		Range:   protocol.Range{Start: pos(0, 6), End: pos(0, 11)},
		NewText: "wörld!",
	}}
	applyFormattingEdits(buf, edits, "utf-16")

	assert.Equal(t, "héllo wörld!\n", string(buf.Bytes()))
	assert.Equal(t, buffer.Loc{8, 0}, cursor.Loc)
}

func TestApplyFormattingEditsFoldsServerCRLFForDosBuffers(t *testing.T) {
	buf := buffer.NewBufferFromString("a\r\nb\r\n", "", buffer.BTDefault)
	if buf.Endings != buffer.FFDos {
		t.Skip("DOS line endings were not detected from the string")
	}
	cursor := buf.GetActiveCursor()
	cursor.GotoLoc(buffer.Loc{1, 1})

	applyFormattingEdits(buf, wholeDocumentEdit(buf, "A\r\nb\r\n"), "utf-16")

	assert.Equal(t, "A\r\nb\r\n", string(buf.Bytes()))
	assert.Equal(t, "A", buf.Line(0))
	assert.Equal(t, buffer.Loc{1, 1}, cursor.Loc)
}

func TestApplyFormattingEditsIsUndoable(t *testing.T) {
	original := "x = 1\ny=2\n"
	buf := buffer.NewBufferFromString(original, "", buffer.BTDefault)

	applyFormattingEdits(buf, wholeDocumentEdit(buf, "x = 1\ny = 2\n"), "utf-16")
	assert.Equal(t, "x = 1\ny = 2\n", string(buf.Bytes()))

	for attempt := 0; attempt < 10 && string(buf.Bytes()) != original; attempt++ {
		buf.Undo()
	}
	assert.Equal(t, original, string(buf.Bytes()))
}
