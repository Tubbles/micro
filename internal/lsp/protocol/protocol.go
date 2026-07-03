// Package protocol contains hand-written Go types for the subset of the
// Language Server Protocol (LSP, see
// https://microsoft.github.io/language-server-protocol/specification)
// that micro's LSP client needs. This is a small, reviewable, v1 subset
// rather than a pruned copy of gopls's generated tsprotocol.go: it
// covers only the lifecycle (initialize/initialized/shutdown) and the
// document/diagnostic/hover building blocks chunk A through C need.
// Later phases extend it as more of the protocol is wired up.
//
// JSON field names and casing follow the LSP spec exactly so values can
// round-trip through a real language server without translation.
package protocol

import "encoding/json"

// DocumentURI identifies a text document, e.g. "file:///a/b.go".
type DocumentURI string

// Position is a zero-based line/character offset within a text
// document. The unit of Character depends on the negotiated position
// encoding (see PositionEncodingKind); it is UTF-16 code units unless
// the server negotiated a different encoding during initialize.
type Position struct {
	Line      uint32 `json:"line"`
	Character uint32 `json:"character"`
}

// Range is a range within a text document, expressed as start and end
// Positions. The end position is exclusive.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location represents a location inside a resource, such as a range in
// a text file.
type Location struct {
	URI   DocumentURI `json:"uri"`
	Range Range       `json:"range"`
}

// TextDocumentIdentifier identifies a text document by URI.
type TextDocumentIdentifier struct {
	URI DocumentURI `json:"uri"`
}

// VersionedTextDocumentIdentifier adds a version number to a
// TextDocumentIdentifier, identifying a specific version of a document
// as it evolves across textDocument/didChange notifications.
type VersionedTextDocumentIdentifier struct {
	TextDocumentIdentifier
	Version int32 `json:"version"`
}

// TextDocumentItem transfers a text document's full content from the
// client to the server, used by textDocument/didOpen.
type TextDocumentItem struct {
	URI        DocumentURI `json:"uri"`
	LanguageID string      `json:"languageId"`
	Version    int32       `json:"version"`
	Text       string      `json:"text"`
}

// TextDocumentContentChangeEvent describes a change to a text document
// sent via textDocument/didChange. If Range is nil, Text replaces the
// document's entire content (full sync); otherwise Text replaces just
// the given range (incremental sync).
type TextDocumentContentChangeEvent struct {
	Range *Range `json:"range,omitempty"`
	Text  string `json:"text"`
}

// TextDocumentSyncKind says how the client should sync document changes
// to the language server.
type TextDocumentSyncKind int32

const (
	TextDocumentSyncKindNone        TextDocumentSyncKind = 0
	TextDocumentSyncKindFull        TextDocumentSyncKind = 1
	TextDocumentSyncKindIncremental TextDocumentSyncKind = 2
)

// DidOpenTextDocumentParams are the parameters of the
// textDocument/didOpen notification, sent once when the client starts
// managing a text document.
type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

// DidChangeTextDocumentParams are the parameters of the
// textDocument/didChange notification. Micro only ever sends a single
// full-document TextDocumentContentChangeEvent (TextDocumentSyncKindFull);
// incremental range-based changes are a v2 concern.
type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

// DidCloseTextDocumentParams are the parameters of the
// textDocument/didClose notification, sent when the client stops
// managing a text document.
type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// DidSaveTextDocumentParams are the parameters of the
// textDocument/didSave notification. Text is optional per the LSP
// spec, but micro always includes the document's full content.
type DidSaveTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Text         string                 `json:"text"`
}

// DiagnosticSeverity is the severity of a Diagnostic.
type DiagnosticSeverity int32

const (
	DiagnosticSeverityError       DiagnosticSeverity = 1
	DiagnosticSeverityWarning     DiagnosticSeverity = 2
	DiagnosticSeverityInformation DiagnosticSeverity = 3
	DiagnosticSeverityHint        DiagnosticSeverity = 4
)

// Diagnostic represents a compiler error, warning, or similar, reported
// by a server via textDocument/publishDiagnostics.
type Diagnostic struct {
	Range    Range              `json:"range"`
	Severity DiagnosticSeverity `json:"severity,omitempty"`
	Code     json.RawMessage    `json:"code,omitempty"`
	Source   string             `json:"source,omitempty"`
	Message  string             `json:"message"`
}

// PublishDiagnosticsParams is sent from the server to the client via
// the textDocument/publishDiagnostics notification.
type PublishDiagnosticsParams struct {
	URI         DocumentURI  `json:"uri"`
	Version     int32        `json:"version,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// MarkupKind is the content format of a MarkupContent value.
type MarkupKind string

const (
	MarkupKindPlainText MarkupKind = "plaintext"
	MarkupKindMarkdown  MarkupKind = "markdown"
)

// MarkupContent is a client-rendered string, either plain text or
// markdown, used for hover contents and similar.
type MarkupContent struct {
	Kind  MarkupKind `json:"kind"`
	Value string     `json:"value"`
}

// Hover is the result of a textDocument/hover request.
type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

// PositionEncodingKind is the unit Position.Character is measured in.
// The client advertises the encodings it supports in
// ClientCapabilities.General.PositionEncodings; the server picks one (or
// none, meaning UTF-16, the LSP default) and reports it back in
// InitializeResult.Capabilities.PositionEncoding.
type PositionEncodingKind string

const (
	PositionEncodingUTF8  PositionEncodingKind = "utf-8"
	PositionEncodingUTF16 PositionEncodingKind = "utf-16"
	PositionEncodingUTF32 PositionEncodingKind = "utf-32"
)

// GeneralClientCapabilities carries client capabilities that don't fall
// under textDocument/workspace, notably the position encodings micro is
// willing to negotiate.
type GeneralClientCapabilities struct {
	PositionEncodings []PositionEncodingKind `json:"positionEncodings,omitempty"`
}

// ClientCapabilities describes the capabilities micro advertises to the
// server during initialize. Only the fields micro's client sets are
// modeled; the LSP spec defines many more.
type ClientCapabilities struct {
	General *GeneralClientCapabilities `json:"general,omitempty"`
}

// ServerCapabilities is the subset of a server's advertised capabilities
// that micro's client understands.
//
// TextDocumentSync, HoverProvider, and DefinitionProvider are left as
// raw JSON because the LSP spec defines each as a union (for example
// hoverProvider is `boolean | HoverOptions`); the chunk that consumes a
// given field decodes it into whichever of those shapes it needs.
type ServerCapabilities struct {
	TextDocumentSync   json.RawMessage      `json:"textDocumentSync,omitempty"`
	HoverProvider      json.RawMessage      `json:"hoverProvider,omitempty"`
	DefinitionProvider json.RawMessage      `json:"definitionProvider,omitempty"`
	PositionEncoding   PositionEncodingKind `json:"positionEncoding,omitempty"`
}

// InitializeParams are the parameters of the initialize request, the
// first message a client sends to a server.
type InitializeParams struct {
	// ProcessID is the process ID of micro itself, or null. It is a
	// pointer rather than an omitempty value so that a nil ProcessID
	// still serializes to "processId": null per the spec, rather than
	// being dropped.
	ProcessID             *int32             `json:"processId"`
	RootURI               DocumentURI        `json:"rootUri,omitempty"`
	Capabilities          ClientCapabilities `json:"capabilities"`
	InitializationOptions json.RawMessage    `json:"initializationOptions,omitempty"`
}

// InitializeResult is the result of the initialize request.
type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
}

// InitializedParams are the (always empty) parameters of the
// initialized notification, sent by the client after it has received
// and processed the InitializeResult.
type InitializedParams struct{}

// The shutdown request and exit notification take no parameters per the
// LSP spec, so no Params types are defined for them.
