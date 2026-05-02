package buffer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWordUnder(t *testing.T) {
	cases := []struct {
		name string
		text string
		x, y int
		want string
		ok   bool
	}{
		{"middle of word", "foo bar baz", 1, 0, "foo", true},
		{"first char of word", "foo bar baz", 0, 0, "foo", true},
		{"last char of word", "foo bar baz", 2, 0, "foo", true},
		{"on whitespace", "foo bar baz", 3, 0, "", false},
		{"on punctuation", "foo.bar", 3, 0, "", false},
		{"subword delimiter included", "snake_case_name", 7, 0, "snake_case_name", true},
		{"empty line", "", 0, 0, "", false},
		{"whitespace-only line", "   ", 1, 0, "", false},
		{"unicode word char", "héllo wörld", 2, 0, "héllo", true},
		{"second word", "foo bar baz", 5, 0, "bar", true},
		{"end of last word", "foo bar baz", 10, 0, "baz", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := NewBufferFromString(tc.text, "", BTDefault)
			c := NewCursor(b, Loc{tc.x, tc.y})
			got, ok := c.WordUnder()
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestWordOrSelection_WordMode(t *testing.T) {
	b := NewBufferFromString("foo bar baz", "", BTDefault)
	c := NewCursor(b, Loc{1, 0}) // mid-"foo"

	q, whole, span, ok := c.WordOrSelection()
	assert.True(t, ok)
	assert.Equal(t, "foo", q)
	assert.True(t, whole)
	assert.Equal(t, [2]Loc{{0, 0}, {3, 0}}, span)
}

func TestWordOrSelection_SingleLineSelection(t *testing.T) {
	b := NewBufferFromString("foo bar foo", "", BTDefault)
	c := b.GetActiveCursor()
	c.SetSelectionStart(Loc{4, 0})
	c.SetSelectionEnd(Loc{7, 0}) // selects "bar"
	c.Loc = Loc{7, 0}

	q, whole, span, ok := c.WordOrSelection()
	assert.True(t, ok)
	assert.Equal(t, "bar", q)
	assert.False(t, whole)
	assert.Equal(t, [2]Loc{{4, 0}, {7, 0}}, span)
}

func TestWordOrSelection_ReversedSelectionNormalised(t *testing.T) {
	b := NewBufferFromString("foo bar foo", "", BTDefault)
	c := b.GetActiveCursor()
	// Reverse order: end < start. WordOrSelection must still return the
	// substring "bar" in document order with span sorted ascending.
	c.SetSelectionStart(Loc{7, 0})
	c.SetSelectionEnd(Loc{4, 0})
	c.Loc = Loc{4, 0}

	q, whole, span, ok := c.WordOrSelection()
	assert.True(t, ok)
	assert.Equal(t, "bar", q)
	assert.False(t, whole)
	assert.Equal(t, [2]Loc{{4, 0}, {7, 0}}, span)
}

func TestWordOrSelection_MultiLineSelectionRejected(t *testing.T) {
	b := NewBufferFromString("foo\nbar", "", BTDefault)
	c := b.GetActiveCursor()
	c.SetSelectionStart(Loc{0, 0})
	c.SetSelectionEnd(Loc{3, 1})
	c.Loc = Loc{3, 1}

	q, whole, span, ok := c.WordOrSelection()
	assert.False(t, ok)
	assert.Equal(t, "", q)
	assert.False(t, whole)
	assert.Equal(t, [2]Loc{}, span)
}

func TestWordOrSelection_OnNonWordChar(t *testing.T) {
	b := NewBufferFromString("foo . bar", "", BTDefault)
	c := NewCursor(b, Loc{4, 0}) // on the '.'

	_, _, _, ok := c.WordOrSelection()
	assert.False(t, ok)
}

func TestWordOrSelection_OnWhitespace(t *testing.T) {
	b := NewBufferFromString("foo bar", "", BTDefault)
	c := NewCursor(b, Loc{3, 0}) // on the space

	_, _, _, ok := c.WordOrSelection()
	assert.False(t, ok)
}

func TestWordOrSelection_EmptyLine(t *testing.T) {
	b := NewBufferFromString("", "", BTDefault)
	c := NewCursor(b, Loc{0, 0})

	_, _, _, ok := c.WordOrSelection()
	assert.False(t, ok)
}
