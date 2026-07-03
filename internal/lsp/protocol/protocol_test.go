package protocol

import (
	"encoding/json"
	"testing"
)

// TestWorkspaceEditChangesMapRoundTrip covers the older `changes`
// form of a WorkspaceEdit: a map from file URI to the TextEdits for
// that file.
func TestWorkspaceEditChangesMapRoundTrip(t *testing.T) {
	raw := []byte(`{
		"changes": {
			"file:///a.go": [
				{"range": {"start": {"line": 0, "character": 0}, "end": {"line": 0, "character": 3}}, "newText": "foo"}
			]
		}
	}`)

	var edit WorkspaceEdit
	if err := json.Unmarshal(raw, &edit); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(edit.DocumentChanges) != 0 {
		t.Errorf("DocumentChanges = %+v, want empty", edit.DocumentChanges)
	}
	edits, ok := edit.Changes["file:///a.go"]
	if !ok {
		t.Fatalf("Changes missing file:///a.go, got %+v", edit.Changes)
	}
	if len(edits) != 1 || edits[0].NewText != "foo" {
		t.Errorf("Changes[file:///a.go] = %+v, want one edit with NewText \"foo\"", edits)
	}

	out, err := json.Marshal(edit)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var roundTripped WorkspaceEdit
	if err := json.Unmarshal(out, &roundTripped); err != nil {
		t.Fatalf("Unmarshal(Marshal(edit)): %v", err)
	}
	if len(roundTripped.Changes["file:///a.go"]) != 1 {
		t.Errorf("round-tripped Changes = %+v, want the original entry preserved", roundTripped.Changes)
	}
}

// TestWorkspaceEditDocumentChangesArrayRoundTrip covers the newer
// `documentChanges` form: an ordered array of per-file edits, each
// pinned to an (optionally versioned) document.
func TestWorkspaceEditDocumentChangesArrayRoundTrip(t *testing.T) {
	raw := []byte(`{
		"documentChanges": [
			{
				"textDocument": {"uri": "file:///a.go", "version": 3},
				"edits": [
					{"range": {"start": {"line": 1, "character": 0}, "end": {"line": 1, "character": 1}}, "newText": "x"}
				]
			},
			{
				"textDocument": {"uri": "file:///b.go", "version": null},
				"edits": [
					{"range": {"start": {"line": 2, "character": 0}, "end": {"line": 2, "character": 1}}, "newText": "y"}
				]
			}
		]
	}`)

	var edit WorkspaceEdit
	if err := json.Unmarshal(raw, &edit); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if len(edit.Changes) != 0 {
		t.Errorf("Changes = %+v, want empty", edit.Changes)
	}
	if len(edit.DocumentChanges) != 2 {
		t.Fatalf("len(DocumentChanges) = %d, want 2", len(edit.DocumentChanges))
	}

	first := edit.DocumentChanges[0]
	if first.TextDocument.URI != "file:///a.go" {
		t.Errorf("DocumentChanges[0].TextDocument.URI = %q, want file:///a.go", first.TextDocument.URI)
	}
	if first.TextDocument.Version == nil || *first.TextDocument.Version != 3 {
		t.Errorf("DocumentChanges[0].TextDocument.Version = %v, want 3", first.TextDocument.Version)
	}
	if len(first.Edits) != 1 || first.Edits[0].NewText != "x" {
		t.Errorf("DocumentChanges[0].Edits = %+v, want one edit with NewText \"x\"", first.Edits)
	}

	second := edit.DocumentChanges[1]
	if second.TextDocument.URI != "file:///b.go" {
		t.Errorf("DocumentChanges[1].TextDocument.URI = %q, want file:///b.go", second.TextDocument.URI)
	}
	if second.TextDocument.Version != nil {
		t.Errorf("DocumentChanges[1].TextDocument.Version = %v, want nil (null in the source)", second.TextDocument.Version)
	}

	out, err := json.Marshal(edit)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var roundTripped WorkspaceEdit
	if err := json.Unmarshal(out, &roundTripped); err != nil {
		t.Fatalf("Unmarshal(Marshal(edit)): %v", err)
	}
	if len(roundTripped.DocumentChanges) != 2 {
		t.Errorf("round-tripped DocumentChanges = %+v, want 2 entries preserved", roundTripped.DocumentChanges)
	}
}

// TestFormattingOptionsJSONKeys pins the exact wire field names (LSP
// is case-sensitive camelCase) and confirms the trim/final-newline
// fields drop out of the payload at their zero value, since micro
// never sets them (see FormattingOptions's doc comment).
func TestFormattingOptionsJSONKeys(t *testing.T) {
	options := FormattingOptions{TabSize: 4, InsertSpaces: true}

	out, err := json.Marshal(options)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got, want := decoded["tabSize"], float64(4); got != want {
		t.Errorf(`decoded["tabSize"] = %v, want %v`, got, want)
	}
	if got, want := decoded["insertSpaces"], true; got != want {
		t.Errorf(`decoded["insertSpaces"] = %v, want %v`, got, want)
	}
	for _, key := range []string{"trimTrailingWhitespace", "insertFinalNewline", "trimFinalNewlines"} {
		if _, present := decoded[key]; present {
			t.Errorf("decoded[%q] present = true at zero value, want omitted (omitempty)", key)
		}
	}

	optionsWithTrim := FormattingOptions{TabSize: 2, InsertSpaces: false, TrimTrailingWhitespace: true, InsertFinalNewline: true, TrimFinalNewlines: true}
	out, err = json.Marshal(optionsWithTrim)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	decoded = nil
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, key := range []string{"trimTrailingWhitespace", "insertFinalNewline", "trimFinalNewlines"} {
		if got, present := decoded[key]; !present || got != true {
			t.Errorf("decoded[%q] = %v, present = %v, want true", key, got, present)
		}
	}
}
