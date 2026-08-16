package tshighlight

import (
	"testing"

	"github.com/micro-editor/micro/v2/pkg/highlight"
)

// TestCaptureGroupKnownMapping checks a few entries from captureGroups
// resolve to the documented target group name.
func TestCaptureGroupKnownMapping(t *testing.T) {
	cases := map[string]string{
		"comment":             "comment",
		"string":              "constant.string",
		"variable.builtin":    "identifier.builtin",
		"punctuation.bracket": "symbol.brackets",
	}
	for capture, wantName := range cases {
		got := captureGroup(capture)
		if got.String() != wantName {
			t.Errorf("captureGroup(%q).String() = %q, want %q", capture, got.String(), wantName)
		}
	}
}

// TestCaptureGroupUnmappedRegistersDottedGroup checks that a capture
// name with no entry in captureGroups still highlights: it is
// registered verbatim as a new group in highlight.Groups, so
// config.GetColor's longest-dot-prefix fallback can still find a parent
// group (or DefStyle) for it instead of the capture being dropped.
func TestCaptureGroupUnmappedRegistersDottedGroup(t *testing.T) {
	const capture = "future.exotic"

	if _, alreadyRegistered := highlight.Groups[capture]; alreadyRegistered {
		t.Fatalf("test capture %q is already registered; pick a different name", capture)
	}

	got := captureGroup(capture)

	registered, ok := highlight.Groups[capture]
	if !ok {
		t.Fatalf("captureGroup(%q) did not register %q in highlight.Groups", capture, capture)
	}
	if registered != got {
		t.Errorf("highlight.Groups[%q] = %v, want %v (the id captureGroup returned)", capture, registered, got)
	}
	if got.String() != capture {
		t.Errorf("captureGroup(%q).String() = %q, want %q", capture, got.String(), capture)
	}

	// Idempotent: a second call for the same capture must return the
	// same id, not mint a second group.
	again := captureGroup(capture)
	if again != got {
		t.Errorf("captureGroup(%q) returned %v then %v; want idempotent", capture, got, again)
	}
}
