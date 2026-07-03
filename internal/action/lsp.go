package action

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/creachadair/jrpc2"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/lsp"
	"github.com/micro-editor/micro/v2/internal/lsp/protocol"
)

// requestPosition builds the textDocument/position pair for h's cursor,
// and returns the client and negotiated encoding needed to both send
// the request and later decode its response.
func (h *BufPane) requestPosition() (client *lsp.Client, uri protocol.DocumentURI, params protocol.TextDocumentPositionParams, encoding string, ok bool) {
	client, uri, ok = lsp.ClientFor(h.Buf.SharedBuffer)
	if !ok {
		return nil, "", protocol.TextDocumentPositionParams{}, "", false
	}
	encoding = string(client.PositionEncoding())
	pos := lsp.LocToPosition(h.Buf.Line(h.Cursor.Y), h.Cursor.Loc, encoding)
	params = protocol.TextDocumentPositionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Position:     pos,
	}
	return client, uri, params, encoding, true
}

// formatHoverMessage turns a textDocument/hover result into a single
// line for the InfoBar (v1 renders hover in the InfoBar rather than an
// anchored popup, see D-44). It returns "" for a hover with no content,
// which covers both a `null` LSP result and an explicit empty string,
// so the caller can show one quiet "no hover info" message for either.
func formatHoverMessage(hover protocol.Hover) string {
	text := strings.TrimSpace(hover.Contents.Value)
	if text == "" {
		return ""
	}
	// The InfoBar is a single display line; collapse markdown/plaintext
	// line breaks and repeated whitespace rather than truncating.
	return strings.Join(strings.Fields(text), " ")
}

// LspHover requests textDocument/hover at the cursor and shows the
// result in the InfoBar. The request is async: this only sends it, and
// the response callback (which runs on the main goroutine, like every
// lsp.Client callback) updates the InfoBar once the server replies.
func (h *BufPane) LspHover() bool {
	client, _, params, _, ok := h.requestPosition()
	if !ok {
		InfoBar.Message("lsp: not attached")
		return true
	}

	client.Call(context.Background(), "textDocument/hover", params, func(rsp *jrpc2.Response, err error) {
		if err != nil {
			InfoBar.Error("lsp: hover: ", err)
			return
		}
		var hover protocol.Hover
		if err := rsp.UnmarshalResult(&hover); err != nil {
			InfoBar.Error("lsp: hover: ", err)
			return
		}
		msg := formatHoverMessage(hover)
		if msg == "" {
			InfoBar.Message("lsp: no hover info")
			return
		}
		InfoBar.Message(msg)
	})
	return true
}

// decodeDefinitionLocation extracts the first Location from a
// textDocument/definition result, which per the LSP spec may be a
// single Location, a Location array, or null (no definition found).
// ok is false for null or an empty array.
func decodeDefinitionLocation(raw json.RawMessage) (loc protocol.Location, ok bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return protocol.Location{}, false
	}

	var single protocol.Location
	if err := json.Unmarshal(raw, &single); err == nil && single.URI != "" {
		return single, true
	}

	var multi []protocol.Location
	if err := json.Unmarshal(raw, &multi); err == nil {
		if len(multi) == 0 {
			return protocol.Location{}, false
		}
		return multi[0], true
	}

	return protocol.Location{}, false
}

// gotoLocator is the subset of BufPane's behavior gotoLoc needs,
// letting tests substitute a lightweight fake instead of constructing a
// full BufPane with a live screen.
type gotoLocator interface {
	GotoLoc(buffer.Loc)
}

// gotoLoc converts pos (measured on lineText in encoding) to a
// buffer.Loc and moves pane there.
func gotoLoc(pane gotoLocator, lineText string, pos protocol.Position, encoding string) {
	pane.GotoLoc(lsp.PositionToLoc(lineText, pos, encoding))
}

// switchOrOpenFile focuses the pane already displaying path if one is
// open in any tab, or opens it by replacing the current pane's buffer
// otherwise (the same behavior as the `open` command). It returns the
// BufPane now displaying path, or nil (after reporting the error to the
// InfoBar) if opening a not-yet-open file failed.
func switchOrOpenFile(h *BufPane, path string) *BufPane {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}

	for tabIndex, tab := range Tabs.List {
		for paneIndex, pane := range tab.Panes {
			bp, ok := pane.(*BufPane)
			if ok && bp.Buf.AbsPath == abs {
				Tabs.SetActive(tabIndex)
				tab.SetActive(paneIndex)
				return bp
			}
		}
	}

	b, err := buffer.NewBufferFromFile(path, buffer.BTDefault)
	if err != nil {
		InfoBar.Error("lsp: opening ", path, ": ", err)
		return nil
	}
	h.OpenBuffer(b)
	return h
}

// LspGotoDefinition requests textDocument/definition at the cursor and
// jumps to the result, opening or switching to the target file first if
// it is not the current buffer. It does not add jump-list bookkeeping
// itself: BufPane.GotoLoc already records a jump once the jump-list
// branch is present (as it is on integration).
func (h *BufPane) LspGotoDefinition() bool {
	client, uri, params, encoding, ok := h.requestPosition()
	if !ok {
		InfoBar.Message("lsp: not attached")
		return true
	}

	client.Call(context.Background(), "textDocument/definition", params, func(rsp *jrpc2.Response, err error) {
		if err != nil {
			InfoBar.Error("lsp: goto definition: ", err)
			return
		}
		var raw json.RawMessage
		if err := rsp.UnmarshalResult(&raw); err != nil {
			InfoBar.Error("lsp: goto definition: ", err)
			return
		}
		loc, found := decodeDefinitionLocation(raw)
		if !found {
			InfoBar.Message("lsp: no definition found")
			return
		}

		target := h
		if loc.URI != uri {
			path, err := lsp.PathFromURI(loc.URI)
			if err != nil {
				InfoBar.Error("lsp: goto definition: ", err)
				return
			}
			target = switchOrOpenFile(h, path)
			if target == nil {
				return
			}
		}

		gotoLoc(target, target.Buf.Line(int(loc.Range.Start.Line)), loc.Range.Start, encoding)
	})
	return true
}

