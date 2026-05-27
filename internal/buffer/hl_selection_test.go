package buffer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// hlBuf creates a buffer for hlselection tests with the given option
// values applied locally so the global defaults are not perturbed.
func hlBuf(t *testing.T, text string, opts map[string]any) *Buffer {
	t.Helper()
	b := NewBufferFromString(text, "", BTDefault)
	for k, v := range opts {
		b.Settings[k] = v
	}
	return b
}

func TestHLSelectionWordMode(t *testing.T) {
	b := hlBuf(t, "foo bar foo foobar foo", map[string]any{
		"hlselection": true,
		"ignorecase":  false,
	})
	c := b.GetActiveCursor()
	c.GotoLoc(Loc{0, 0})
	b.UpdateHLSelection()

	assert.True(t, b.HLSelection)
	assert.Equal(t, "foo", b.HLSelectionQuery)
	assert.True(t, b.HLSelectionWholeWord)

	// `foo` at col 0 matches.
	assert.True(t, b.HLSelectionAt(Loc{0, 0}))
	assert.True(t, b.HLSelectionAt(Loc{2, 0}))
	// Space between "foo" and "bar" does not match.
	assert.False(t, b.HLSelectionAt(Loc{3, 0}))
	// `bar` does not match.
	assert.False(t, b.HLSelectionAt(Loc{4, 0}))
	// Second `foo` (col 8-10) matches.
	assert.True(t, b.HLSelectionAt(Loc{8, 0}))
	// `foobar` (col 12-17) does NOT match — whole-word boundary.
	assert.False(t, b.HLSelectionAt(Loc{12, 0}))
	// Final `foo` matches.
	assert.True(t, b.HLSelectionAt(Loc{19, 0}))
}

func TestHLSelectionDisabledByDefault(t *testing.T) {
	b := hlBuf(t, "foo foo", map[string]any{
		"hlselection": false,
	})
	b.GetActiveCursor().GotoLoc(Loc{0, 0})
	b.UpdateHLSelection()
	assert.False(t, b.HLSelection)
	assert.False(t, b.HLSelectionAt(Loc{0, 0}))
}

func TestHLSelectionWordOnNonWordChar(t *testing.T) {
	b := hlBuf(t, "foo . foo", map[string]any{
		"hlselection": true,
		"ignorecase":  false,
	})
	b.GetActiveCursor().GotoLoc(Loc{4, 0}) // on the '.'
	b.UpdateHLSelection()
	assert.Equal(t, "", b.HLSelectionQuery)
	assert.False(t, b.HLSelectionAt(Loc{0, 0}))
}

func TestHLSelectionSelectionMode(t *testing.T) {
	b := hlBuf(t, "abc abc abcd", map[string]any{
		"hlselection": true,
		"ignorecase":  false,
	})
	c := b.GetActiveCursor()
	c.SetSelectionStart(Loc{0, 0})
	c.SetSelectionEnd(Loc{3, 0})
	c.Loc = Loc{3, 0}
	b.UpdateHLSelection()

	assert.Equal(t, "abc", b.HLSelectionQuery)
	assert.False(t, b.HLSelectionWholeWord)

	// Substring matches: each `abc` and the `abc` prefix of `abcd`.
	assert.True(t, b.HLSelectionAt(Loc{0, 0}))
	assert.True(t, b.HLSelectionAt(Loc{4, 0}))
	assert.True(t, b.HLSelectionAt(Loc{8, 0}))
	// `d` after `abc` is not a match cell.
	assert.False(t, b.HLSelectionAt(Loc{11, 0}))
}

func TestHLSelectionRegexMetacharsEscaped(t *testing.T) {
	b := hlBuf(t, "a.b axb a.b", map[string]any{
		"hlselection": true,
		"ignorecase":  false,
	})
	c := b.GetActiveCursor()
	c.SetSelectionStart(Loc{0, 0})
	c.SetSelectionEnd(Loc{3, 0})
	c.Loc = Loc{3, 0}
	b.UpdateHLSelection()

	assert.Equal(t, "a.b", b.HLSelectionQuery)
	// Literal `a.b` matches `a.b` (cols 0 and 8).
	assert.True(t, b.HLSelectionAt(Loc{0, 0}))
	assert.True(t, b.HLSelectionAt(Loc{8, 0}))
	// `axb` (col 4-6) must NOT match because `.` is escaped.
	assert.False(t, b.HLSelectionAt(Loc{4, 0}))
	assert.False(t, b.HLSelectionAt(Loc{5, 0}))
}

func TestHLSelectionMultiLineSelectionDisables(t *testing.T) {
	b := hlBuf(t, "foo\nfoo\nfoo", map[string]any{
		"hlselection": true,
	})
	c := b.GetActiveCursor()
	c.SetSelectionStart(Loc{0, 0})
	c.SetSelectionEnd(Loc{3, 1})
	c.Loc = Loc{3, 1}
	b.UpdateHLSelection()

	assert.Equal(t, "", b.HLSelectionQuery)
	assert.False(t, b.HLSelectionAt(Loc{0, 0}))
	assert.False(t, b.HLSelectionAt(Loc{0, 2}))
}

func TestHLSelectionIgnoreCase(t *testing.T) {
	b := hlBuf(t, "Foo foo FOO", map[string]any{
		"hlselection": true,
		"ignorecase":  true,
	})
	b.GetActiveCursor().GotoLoc(Loc{0, 0})
	b.UpdateHLSelection()

	assert.Equal(t, "Foo", b.HLSelectionQuery)
	assert.True(t, b.HLSelectionAt(Loc{0, 0}))  // Foo
	assert.True(t, b.HLSelectionAt(Loc{4, 0}))  // foo
	assert.True(t, b.HLSelectionAt(Loc{8, 0}))  // FOO
}

func TestHLSelectionCaseSensitive(t *testing.T) {
	b := hlBuf(t, "Foo foo FOO", map[string]any{
		"hlselection": true,
		"ignorecase":  false,
	})
	b.GetActiveCursor().GotoLoc(Loc{0, 0})
	b.UpdateHLSelection()

	assert.True(t, b.HLSelectionAt(Loc{0, 0}))   // Foo
	assert.False(t, b.HLSelectionAt(Loc{4, 0}))  // foo, mismatched case
	assert.False(t, b.HLSelectionAt(Loc{8, 0}))  // FOO, mismatched case
}

func TestHLSelectionInvalidationOnLineEdit(t *testing.T) {
	b := hlBuf(t, "foo\nfoo", map[string]any{
		"hlselection": true,
		"ignorecase":  false,
	})
	b.GetActiveCursor().GotoLoc(Loc{0, 0})
	b.UpdateHLSelection()
	assert.True(t, b.HLSelectionAt(Loc{0, 1}))

	// Replace `foo` on line 1 with `bar`.
	b.Replace(Loc{0, 1}, Loc{3, 1}, "bar")
	assert.False(t, b.HLSelectionAt(Loc{0, 1}))
	// Line 0 still matches.
	assert.True(t, b.HLSelectionAt(Loc{0, 0}))
}

func TestHLSelectionQueryChangeInvalidates(t *testing.T) {
	b := hlBuf(t, "foo bar", map[string]any{
		"hlselection": true,
		"ignorecase":  false,
	})
	c := b.GetActiveCursor()
	c.GotoLoc(Loc{0, 0})
	b.UpdateHLSelection()
	assert.True(t, b.HLSelectionAt(Loc{0, 0}))
	assert.False(t, b.HLSelectionAt(Loc{4, 0}))

	// Move to `bar`; cache must update.
	c.GotoLoc(Loc{4, 0})
	b.UpdateHLSelection()
	assert.False(t, b.HLSelectionAt(Loc{0, 0}))
	assert.True(t, b.HLSelectionAt(Loc{4, 0}))
}
