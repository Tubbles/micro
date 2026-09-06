package action

import (
	"testing"

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
