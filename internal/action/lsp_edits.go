package action

import (
	"sort"

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
// helper is shared by completion, formatting, and rename, all of
// which can return multiple edits in one response.
func applyTextEdits(buf *buffer.Buffer, edits []protocol.TextEdit, encoding string) {
	if len(edits) == 0 {
		return
	}

	sorted := make([]protocol.TextEdit, len(edits))
	copy(sorted, edits)
	sort.Slice(sorted, func(i, j int) bool {
		return positionAfter(sorted[i].Range.Start, sorted[j].Range.Start)
	})

	for _, edit := range sorted {
		start := lsp.PositionToLoc(buf.Line(int(edit.Range.Start.Line)), edit.Range.Start, encoding)
		end := lsp.PositionToLoc(buf.Line(int(edit.Range.End.Line)), edit.Range.End, encoding)
		buf.Replace(start, end, edit.NewText)
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
