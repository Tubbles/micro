package action

import (
	"sort"
	"strings"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/lsp"
	"github.com/micro-editor/micro/v2/internal/lsp/protocol"
)

// applyTextEdits applies a list of LSP TextEdits to buf. Every edit's
// Range is expressed in positions on the *original* document (the LSP
// spec leaves ordering to the client), so edits are applied in
// descending order by start position, bottom-most/right-most first:
// applying a later-in-the-document edit first never shifts the
// buffer offsets an earlier edit's range was computed against. This
// helper is shared by completion and rename, which can return
// multiple edits in one response. Formatting goes through
// applyFormattingEdits instead.
func applyTextEdits(buf *buffer.Buffer, edits []protocol.TextEdit, encoding string) {
	if len(edits) == 0 {
		return
	}

	for _, edit := range sortedDescending(edits) {
		start, end := editLocs(buf, edit, encoding)
		buf.Replace(start, end, edit.NewText)
	}
}

// applyFormattingEdits applies formatting edits by computing the
// formatted text and diffing it into buf, instead of replacing the
// edits' ranges. Formatters commonly answer with one edit covering the
// whole document, and a range replace of that removes everything and
// inserts the new text at the top: a cursor inside the removed range is
// not moved by the removal but is pushed down by the insertion, so it
// ends up clamped to the last line. ApplyDiff only removes and inserts
// what actually changed, so cursors ride along through the untouched
// text the way they do for any other edit.
//
// Completion and rename keep applyTextEdits: a completion edit ends at
// the cursor, and a diff is free to put an insertion on either side of
// an equal run, which could leave the cursor before the inserted text.
func applyFormattingEdits(buf *buffer.Buffer, edits []protocol.TextEdit, encoding string) {
	if len(edits) == 0 {
		return
	}

	formatted := lineArrayText(buf)
	for _, edit := range sortedDescending(edits) {
		start, end := editLocs(buf, edit, encoding)
		formatted = formatted[:buffer.ByteOffset(start, buf)] + edit.NewText + formatted[buffer.ByteOffset(end, buf):]
	}
	buf.ApplyDiff(formatted)
}

// sortedDescending returns a copy of edits ordered bottom-most/right-most
// first, the order in which original-document ranges stay valid while
// earlier edits are still pending.
func sortedDescending(edits []protocol.TextEdit) []protocol.TextEdit {
	sorted := make([]protocol.TextEdit, len(edits))
	copy(sorted, edits)
	sort.Slice(sorted, func(i, j int) bool {
		return positionAfter(sorted[i].Range.Start, sorted[j].Range.Start)
	})
	return sorted
}

// editLocs converts an edit's range into buffer locations.
func editLocs(buf *buffer.Buffer, edit protocol.TextEdit, encoding string) (buffer.Loc, buffer.Loc) {
	start := lsp.PositionToLoc(buf.Line(int(edit.Range.Start.Line)), edit.Range.Start, encoding)
	end := lsp.PositionToLoc(buf.Line(int(edit.Range.End.Line)), edit.Range.End, encoding)
	return start, end
}

// lineArrayText returns buf's text as the line array stores it: lines
// joined by "\n", with no carriage returns regardless of file format.
// That is the text buffer.ByteOffset indexes into and ApplyDiff diffs
// against.
func lineArrayText(buf *buffer.Buffer) string {
	lines := make([]string, buf.LinesNum())
	for index := range lines {
		lines[index] = buf.Line(index)
	}
	return strings.Join(lines, "\n")
}

// positionAfter reports whether a comes strictly after b in document
// order.
func positionAfter(a, b protocol.Position) bool {
	if a.Line != b.Line {
		return a.Line > b.Line
	}
	return a.Character > b.Character
}
