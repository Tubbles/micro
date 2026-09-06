package lsp

import (
	"context"

	"github.com/micro-editor/micro/v2/internal/lsp/protocol"
)

// DidOpen sends textDocument/didOpen, telling the server the client
// now owns uri and its initial content is text at version. Per the LSP
// spec this is the first notification sent for a document and the
// version numbering for it starts wherever the caller says; callers
// that follow the didOpen/didChange/didClose lifecycle described on
// Client (see internal/lsp/documents.go) start at 1.
func (c *Client) DidOpen(uri protocol.DocumentURI, languageID string, version int32, text string) {
	c.Notify(context.Background(), "textDocument/didOpen", protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        uri,
			LanguageID: languageID,
			Version:    version,
			Text:       text,
		},
	})
}

// DidChange sends textDocument/didChange with a single full-document
// TextDocumentContentChangeEvent, matching TextDocumentSyncKindFull.
// text replaces the server's entire view of the document; incremental
// range-based sync is deferred to v2. version must be strictly greater
// than the version last sent for uri (didOpen's version counts as the
// first one).
func (c *Client) DidChange(uri protocol.DocumentURI, version int32, text string) {
	c.Notify(context.Background(), "textDocument/didChange", protocol.DidChangeTextDocumentParams{
		TextDocument: protocol.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: protocol.TextDocumentIdentifier{URI: uri},
			Version:                version,
		},
		ContentChanges: []protocol.TextDocumentContentChangeEvent{{Text: text}},
	})
}

// DidClose sends textDocument/didClose, telling the server the client
// no longer manages uri.
func (c *Client) DidClose(uri protocol.DocumentURI) {
	c.Notify(context.Background(), "textDocument/didClose", protocol.DidCloseTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
	})
}

// DidSave sends textDocument/didSave with the document's full content.
func (c *Client) DidSave(uri protocol.DocumentURI, text string) {
	c.Notify(context.Background(), "textDocument/didSave", protocol.DidSaveTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Text:         text,
	})
}
