package action

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/creachadair/jrpc2"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/lsp"
	"github.com/micro-editor/micro/v2/internal/lsp/protocol"
	"github.com/micro-editor/micro/v2/internal/widget"
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

// completionItems converts a decoded candidate list into the
// widget-local item type CompletionBox displays, keeping
// internal/widget ignorant of internal/lsp's protocol types.
func completionItems(candidates []lsp.CompletionCandidate) []widget.CompletionItem {
	items := make([]widget.CompletionItem, len(candidates))
	for i, cand := range candidates {
		items[i] = widget.CompletionItem{Label: cand.Label, Detail: cand.Detail}
	}
	return items
}

// openCompletionBox shows candidates in a popup anchored at loc. The
// select callback applies the chosen candidate's edits and relocates
// the view, same as any other buffer-mutating action.
func (h *BufPane) openCompletionBox(candidates []lsp.CompletionCandidate, loc buffer.Loc, encoding string) {
	box := widget.NewCompletionBox(widget.CompletionBoxOptions{
		Pane:  h,
		Loc:   loc,
		Items: completionItems(candidates),
		OnSelect: func(index int) {
			if index < 0 || index >= len(candidates) {
				return
			}
			applyTextEdits(h.Buf, candidates[index].Edits, encoding)
			h.Relocate()
		},
	})
	widget.Open(box)
}

// LspCompletion requests textDocument/completion at the cursor and, on
// a non-empty response, shows the candidates in an anchored popup
// (see internal/widget.CompletionBox). This is manual-trigger only:
// it fires when bound and invoked, never on typing, a trigger
// character, or any other implicit event, so the request always
// reports protocol.CompletionTriggerKindInvoked.
//
// The cursor position and line text at request time are captured and
// reused once the (async) response arrives, both for decoding
// InsertText-only candidates (which need the word range the request
// was made from) and for anchoring the popup, rather than re-reading
// a cursor that may have moved while the server was replying.
func (h *BufPane) LspCompletion() bool {
	client, _, params, encoding, ok := h.requestPosition()
	if !ok {
		InfoBar.Message("lsp: not attached")
		return true
	}

	loc := h.Cursor.Loc
	lineText := h.Buf.Line(loc.Y)
	completionParams := protocol.CompletionParams{
		TextDocumentPositionParams: params,
		Context:                    &protocol.CompletionContext{TriggerKind: protocol.CompletionTriggerKindInvoked},
	}

	client.Call(context.Background(), "textDocument/completion", completionParams, func(rsp *jrpc2.Response, err error) {
		if err != nil {
			InfoBar.Error("lsp: completion: ", err)
			return
		}
		var raw json.RawMessage
		if err := rsp.UnmarshalResult(&raw); err != nil {
			InfoBar.Error("lsp: completion: ", err)
			return
		}
		candidates := lsp.DecodeCompletionResult(raw, lineText, loc, encoding)
		if len(candidates) == 0 {
			InfoBar.Message("lsp: no completions")
			return
		}
		h.openCompletionBox(candidates, loc, encoding)
	})
	return true
}

// capabilityEnabled reports whether a boolean-or-options server
// capability field (for example ServerCapabilities.RenameProvider) is
// enabled. Per the LSP spec these fields are typed `boolean |
// XOptions`; an absent field or an explicit `false` means the server
// does not support the feature, anything else (`true`, or an options
// object) means it does.
func capabilityEnabled(raw json.RawMessage) bool {
	switch string(raw) {
	case "", "null", "false":
		return false
	default:
		return true
	}
}

// formattingOptionsFromSettings builds the FormattingOptions a
// formatting request sends from a buffer's settings, so a server-side
// formatter indents using the same width and tabs-vs-spaces choice
// micro itself uses for that buffer. It deliberately leaves the
// trim/final-newline options at their zero value (false): micro
// already applies rmtrailingws/eofnewline at save time (see
// buffer/save.go), and asking the server to redo them risks double-
// applying or fighting a setting the user configured on purpose.
func formattingOptionsFromSettings(settings map[string]any) protocol.FormattingOptions {
	return protocol.FormattingOptions{
		TabSize:      uint32(settings["tabsize"].(float64)),
		InsertSpaces: settings["tabstospaces"].(bool),
	}
}

// formattingRange converts a cursor selection (its two Locs in either
// order) into an LSP Range on buf, ordering start <= end. It reports
// ok=false if either endpoint is out of buf's current bounds: a
// selection is never clamped when edits shrink the buffer out from
// under it (Relocate/ShiftLoc don't touch CurSelection), so a stale
// selection needs the same InBounds guard Cursor.GetSelection uses
// before turning it into a Substr call.
func formattingRange(buf *buffer.Buffer, selection [2]buffer.Loc, encoding string) (protocol.Range, bool) {
	start, end := selection[0], selection[1]
	if start.GreaterThan(end) {
		start, end = end, start
	}
	if !buffer.InBounds(start, buf) || !buffer.InBounds(end, buf) {
		return protocol.Range{}, false
	}
	return protocol.Range{
		Start: lsp.LocToPosition(buf.Line(start.Y), start, encoding),
		End:   lsp.LocToPosition(buf.Line(end.Y), end, encoding),
	}, true
}

// LspFormat requests formatting for the current buffer. A selection
// formats just that range via textDocument/rangeFormatting when the
// server advertises range formatting; otherwise (or with no selection)
// the whole document formats via textDocument/formatting. See the
// "formatonsave" option for automatic formatting on save, which hooks
// in at BufPane.Save rather than through this action.
func (h *BufPane) LspFormat() bool {
	client, uri, _, encoding, ok := h.requestPosition()
	if !ok {
		InfoBar.Message("lsp: not attached")
		return true
	}

	caps := client.Capabilities()
	identifier := protocol.TextDocumentIdentifier{URI: uri}
	options := formattingOptionsFromSettings(h.Buf.Settings)

	method := "textDocument/formatting"
	var params any = protocol.DocumentFormattingParams{TextDocument: identifier, Options: options}

	if h.Cursor.HasSelection() && capabilityEnabled(caps.DocumentRangeFormattingProvider) {
		if rng, inBounds := formattingRange(h.Buf, h.Cursor.CurSelection, encoding); inBounds {
			method = "textDocument/rangeFormatting"
			params = protocol.DocumentRangeFormattingParams{TextDocument: identifier, Range: rng, Options: options}
		}
	}

	if method == "textDocument/formatting" && !capabilityEnabled(caps.DocumentFormattingProvider) {
		InfoBar.Message("lsp: server does not support formatting")
		return true
	}

	client.Call(context.Background(), method, params, func(rsp *jrpc2.Response, err error) {
		if err != nil {
			InfoBar.Error("lsp: format: ", err)
			return
		}
		var edits []protocol.TextEdit
		if err := rsp.UnmarshalResult(&edits); err != nil {
			InfoBar.Error("lsp: format: ", err)
			return
		}
		if len(edits) == 0 {
			InfoBar.Message("lsp: no formatting changes")
			return
		}
		applyTextEdits(h.Buf, edits, encoding)
		h.Relocate()
	})
	return true
}

// formatBeforeSave is the pure form of the "formatonsave" gate: format
// before saving only when all three hold. attached and
// providesFormatting come from the buffer's current LSP attachment
// (see lspFormatBeforeSave); formatOnSave is the per-buffer setting.
func formatBeforeSave(attached, formatOnSave, providesFormatting bool) bool {
	return attached && formatOnSave && providesFormatting
}

// lspFormatBeforeSave resolves the inputs formatBeforeSave needs from
// h's current LSP attachment and settings, returning the Client and
// document URI/encoding formatThenSave needs alongside the gate result
// so it doesn't have to re-resolve the attachment.
func (h *BufPane) lspFormatBeforeSave() (client *lsp.Client, uri protocol.DocumentURI, encoding string, shouldFormat bool) {
	client, uri, ok := lsp.ClientFor(h.Buf.SharedBuffer)
	if !ok {
		return nil, "", "", false
	}
	formatOnSave, _ := h.Buf.Settings["formatonsave"].(bool)
	providesFormatting := capabilityEnabled(client.Capabilities().DocumentFormattingProvider)
	return client, uri, string(client.PositionEncoding()), formatBeforeSave(ok, formatOnSave, providesFormatting)
}

// formatThenSave requests textDocument/formatting for the whole
// document and, once the (async) response arrives, applies any edits
// and then performs the actual save via SaveCB. SaveCB is called
// directly rather than Save, so the save that follows formatting never
// re-enters lspFormatBeforeSave: formatting happens at most once per
// Save, not once per underlying write (sudo retry, overwrite prompt,
// and so on). A formatting error is reported but does not stop the
// save: losing the user's save because the formatter errored would be
// worse than saving unformatted.
func (h *BufPane) formatThenSave(client *lsp.Client, uri protocol.DocumentURI, encoding string) bool {
	params := protocol.DocumentFormattingParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Options:      formattingOptionsFromSettings(h.Buf.Settings),
	}

	client.Call(context.Background(), "textDocument/formatting", params, func(rsp *jrpc2.Response, err error) {
		if err != nil {
			InfoBar.Error("lsp: format on save: ", err)
		} else {
			var edits []protocol.TextEdit
			if err := rsp.UnmarshalResult(&edits); err != nil {
				InfoBar.Error("lsp: format on save: ", err)
			} else {
				applyTextEdits(h.Buf, edits, encoding)
			}
		}
		h.SaveCB("Save", nil)
	})
	return true
}

// lspResolveServer resolves a server name and definition for the `> lsp
// start|stop|restart` commands: an explicit name in args[0] if given,
// otherwise the server registered for the current buffer's filetype
// (the same resolution attach uses for auto-attach).
func (h *BufPane) lspResolveServer(args []string) (name string, def lsp.ServerDefinition, ok bool) {
	r := lsp.GetRegistry()
	if r == nil {
		InfoBar.Error("lsp: server registry is unavailable")
		return "", lsp.ServerDefinition{}, false
	}

	if len(args) > 0 {
		name = args[0]
		def, ok = r.Definition(name)
		if !ok {
			InfoBar.Error("lsp: unknown server ", name)
			return "", lsp.ServerDefinition{}, false
		}
		return name, def, true
	}

	filetype, _ := h.Buf.Settings["filetype"].(string)
	name, def, ok = r.DefinitionForFiletype(filetype)
	if !ok {
		InfoBar.Error("lsp: no server configured for filetype ", filetype)
		return "", lsp.ServerDefinition{}, false
	}
	return name, def, true
}

// lspStart implements `> lsp start [server]`.
func (h *BufPane) lspStart(args []string) {
	name, def, ok := h.lspResolveServer(args)
	if !ok {
		return
	}
	root := lsp.RootFor(def, h.Buf.AbsPath)

	lsp.GetRegistry().GetOrStart(context.Background(), name, root, func(_ *lsp.Client, err error) {
		if err != nil {
			InfoBar.Error("lsp: starting ", name, ": ", err)
			return
		}
		InfoBar.Message("lsp: ", name, " ready at ", root)
	})
}

// lspStop implements `> lsp stop [server]`.
func (h *BufPane) lspStop(args []string) {
	name, def, ok := h.lspResolveServer(args)
	if !ok {
		return
	}
	root := lsp.RootFor(def, h.Buf.AbsPath)

	r := lsp.GetRegistry()
	if _, running := r.Get(name, root); !running {
		InfoBar.Message("lsp: ", name, " is not running at ", root)
		return
	}
	r.Stop(name, root, func(err error) {
		if err != nil {
			InfoBar.Error("lsp: stopping ", name, ": ", err)
			return
		}
		InfoBar.Message("lsp: stopped ", name)
	})
}

// lspRestart implements `> lsp restart [server]`.
func (h *BufPane) lspRestart(args []string) {
	name, def, ok := h.lspResolveServer(args)
	if !ok {
		return
	}
	root := lsp.RootFor(def, h.Buf.AbsPath)

	lsp.GetRegistry().Restart(context.Background(), name, root, func(_ *lsp.Client, err error) {
		if err != nil {
			InfoBar.Error("lsp: restarting ", name, ": ", err)
			return
		}
		InfoBar.Message("lsp: restarted ", name)
	})
}

// lspStatus implements `> lsp status`: a snapshot of every running
// server and attached document, written to the log buffer (the same
// mechanism `> log` and `> plugin list` use) since the report is
// multi-line and does not fit the single-line InfoBar.
func (h *BufPane) lspStatus() {
	r := lsp.GetRegistry()

	var report strings.Builder
	report.WriteString("LSP status\n")

	var clients []lsp.ClientStatus
	if r != nil {
		clients = r.Clients()
	}
	if len(clients) == 0 {
		report.WriteString("  no servers running\n")
	}
	for _, c := range clients {
		fmt.Fprintf(&report, "  %s @ %s: %s\n", c.Name, c.Root, c.State)
	}

	report.WriteString("Attached documents\n")
	docs := lsp.AttachedDocuments()
	if len(docs) == 0 {
		report.WriteString("  none\n")
	}
	for _, d := range docs {
		fmt.Fprintf(&report, "  %s (%s @ %s): %d errors, %d warnings\n", d.URI, d.Server, d.Root, d.Errors, d.Warnings)
	}

	WriteLog(report.String())
	h.OpenLogBuf()
}

// LspCmd implements `> lsp status|start|stop|restart [server]`.
func (h *BufPane) LspCmd(args []string) {
	if len(args) == 0 {
		InfoBar.Error("lsp: usage: lsp status|start|stop|restart ['server']")
		return
	}

	switch args[0] {
	case "status":
		h.lspStatus()
	case "start":
		h.lspStart(args[1:])
	case "stop":
		h.lspStop(args[1:])
	case "restart":
		h.lspRestart(args[1:])
	default:
		InfoBar.Error("lsp: unknown subcommand ", args[0])
	}
}
