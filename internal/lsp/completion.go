package lsp

import (
	"bytes"
	"encoding/json"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/lsp/protocol"
	"github.com/micro-editor/micro/v2/internal/util"
)

// CompletionCandidate is micro's client-side view of one
// textDocument/completion result, decoupled from the raw
// protocol.CompletionItem so the action and widget layers don't need
// to know about LSP's TextEdit-vs-InsertText union. Edits is always
// populated (synthesized from InsertText/Label when the server didn't
// send a TextEdit) so applying a candidate is always "run these
// edits", never a separate insert-at-cursor path.
//
// When the source item's InsertTextFormat is
// protocol.InsertTextFormatSnippet, Edits[*].NewText still contains
// raw snippet syntax (e.g. "$1", "${1:name}"): snippet placeholder
// expansion is not implemented, so the text is inserted verbatim.
type CompletionCandidate struct {
	Label  string
	Detail string
	Kind   protocol.CompletionItemKind
	Edits  []protocol.TextEdit
}

// DecodeCompletionResult decodes a textDocument/completion result into
// a candidate list. Per the LSP spec the result is one of a
// CompletionList object, a bare CompletionItem array, or null (no
// completions); raw's first non-whitespace byte ('[' for the array
// form, '{' for the object form) disambiguates which to decode.
// lineText and cursor give the fallback edit range for items that
// only set InsertText (see newCompletionCandidate).
func DecodeCompletionResult(raw json.RawMessage, lineText string, cursor buffer.Loc, encoding string) []CompletionCandidate {
	items := decodeCompletionItems(raw)
	if len(items) == 0 {
		return nil
	}
	candidates := make([]CompletionCandidate, len(items))
	for i, item := range items {
		candidates[i] = newCompletionCandidate(item, lineText, cursor, encoding)
	}
	return candidates
}

// decodeCompletionItems decodes the array|object|null result shapes
// described on DecodeCompletionResult into a flat item list.
func decodeCompletionItems(raw json.RawMessage) []protocol.CompletionItem {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}

	if trimmed[0] == '[' {
		var items []protocol.CompletionItem
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return nil
		}
		return items
	}

	var list protocol.CompletionList
	if err := json.Unmarshal(trimmed, &list); err != nil {
		return nil
	}
	return list.Items
}

// newCompletionCandidate converts one protocol.CompletionItem into a
// CompletionCandidate. When the item carries a TextEdit that edit is
// used as-is; otherwise a single edit is synthesized that replaces the
// word run touching cursor (the partially-typed word the user
// requested completion from) with the item's InsertText, falling back
// to Label when InsertText is empty (per the LSP spec, a CompletionItem
// with neither is completed by inserting Label at the cursor).
func newCompletionCandidate(item protocol.CompletionItem, lineText string, cursor buffer.Loc, encoding string) CompletionCandidate {
	var edits []protocol.TextEdit
	if item.TextEdit != nil {
		edits = []protocol.TextEdit{*item.TextEdit}
	} else {
		text := item.InsertText
		if text == "" {
			text = item.Label
		}
		start, end := wordRangeAt(lineText, cursor.X)
		edits = []protocol.TextEdit{{
			Range: protocol.Range{
				Start: LocToPosition(lineText, buffer.Loc{X: start, Y: cursor.Y}, encoding),
				End:   LocToPosition(lineText, buffer.Loc{X: end, Y: cursor.Y}, encoding),
			},
			NewText: text,
		}}
	}

	return CompletionCandidate{
		Label:  item.Label,
		Detail: item.Detail,
		Kind:   item.Kind,
		Edits:  edits,
	}
}

// wordRangeAt returns the rune-column range [start, end) of the word
// touching column col on lineText, using util.IsWordChar (the same
// word-char predicate bufpane word movement and the picker's query
// input use). col may land in the middle of the word; both directions
// are scanned so the whole word is replaced, not just its prefix.
func wordRangeAt(lineText string, col int) (start, end int) {
	runes := []rune(lineText)
	if col < 0 {
		col = 0
	}
	if col > len(runes) {
		col = len(runes)
	}
	start, end = col, col
	for start > 0 && util.IsWordChar(runes[start-1]) {
		start--
	}
	for end < len(runes) && util.IsWordChar(runes[end]) {
		end++
	}
	return start, end
}
