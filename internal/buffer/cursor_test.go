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
