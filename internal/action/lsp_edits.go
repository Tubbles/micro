package action

import (
	"sort"
	"strings"

	dmp "github.com/sergi/go-diff/diffmatchpatch"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/lsp"
	"github.com/micro-editor/micro/v2/internal/lsp/protocol"
	"github.com/micro-editor/micro/v2/internal/util"
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
// ends up clamped to the last line. A character-level diff only removes
// and inserts what actually changed, so cursors ride along through the
// untouched text the way they do for any other edit.
//
// Completion and rename keep applyTextEdits: a completion edit ends at
// the cursor, and a diff is free to put an insertion on either side of
// an equal run, which could leave the cursor before the inserted text.
func applyFormattingEdits(buf *buffer.Buffer, edits []protocol.TextEdit, encoding string) {
	if len(edits) == 0 {
		return
	}

	original := lineArrayText(buf)
	formatted := original
	for _, edit := range sortedDescending(edits) {
		start, end := editLocs(buf, edit, encoding)
		// The line array never stores a carriage return (Bytes() adds
		// them back for DOS files on the way out), so fold the server's
		// line endings to match before diffing.
		replacement := strings.ReplaceAll(edit.NewText, "\r\n", "\n")
		formatted = formatted[:byteOffset(buf, start)] + replacement + formatted[byteOffset(buf, end):]
	}
	applyDiff(buf, original, formatted)
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
func lineArrayText(buf *buffer.Buffer) string {
	lines := make([]string, buf.LinesNum())
	for index := range lines {
		lines[index] = buf.Line(index)
	}
	return strings.Join(lines, "\n")
}

// byteOffset is loc's offset into lineArrayText(buf). buffer.ByteOffset
// is not used because it slices the line by loc.X as if it were a byte
// index, which is wrong after any multi-byte character.
func byteOffset(buf *buffer.Buffer, loc buffer.Loc) int {
	offset := 0
	for line := 0; line < loc.Y; line++ {
		offset += len(buf.LineBytes(line)) + 1
	}
	return offset + len(util.SliceStart(buf.LineBytes(loc.Y), loc.X))
}

// applyDiff turns buf's text, currently equal to old, into want through
// the minimal character-level removals and insertions, each a regular
// undoable text event that shifts cursors and anchors.
func applyDiff(buf *buffer.Buffer, old, want string) {
	loc := buf.Start()
	for _, diff := range dmp.New().DiffMain(old, want, false) {
		count := util.CharacterCountInString(diff.Text)
		switch diff.Type {
		case dmp.DiffDelete:
			buf.Remove(loc, loc.Move(count, buf))
		case dmp.DiffInsert:
			buf.Insert(loc, diff.Text)
			loc = loc.Move(count, buf)
		default:
			loc = loc.Move(count, buf)
		}
	}
}

// positionAfter reports whether a comes strictly after b in document
// order.
func positionAfter(a, b protocol.Position) bool {
	if a.Line != b.Line {
		return a.Line > b.Line
	}
	return a.Character > b.Character
}
