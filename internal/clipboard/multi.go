package clipboard

import (
	"strings"
)

// For storing multi cursor clipboard contents
type multiClipboard map[Register][]string

var multi multiClipboard

// getAllText returns the text written to the system clipboard for a
// register: one slot per cursor, joined with newlines, so that pasting
// a multi-cursor copy with a single cursor yields one selection per
// line instead of everything run together on one line. The same joined
// form is what isValid compares against, so a paste with a matching
// cursor count still gets the per-slot text via getText.
func (c multiClipboard) getAllText(r Register) string {
	return strings.Join(c[r], "\n")
}

func (c multiClipboard) getText(r Register, num int) string {
	content := c[r]
	if content == nil || len(content) <= num {
		return ""
	}

	return content[num]
}

// isValid checks if the text stored in this multi-clipboard is the same as the
// text stored in the system clipboard (provided as an argument), and therefore
// if it is safe to use the multi-clipboard for pasting instead of the system
// clipboard.
func (c multiClipboard) isValid(r Register, clipboard string, ncursors int) bool {
	content := c[r]
	if content == nil || len(content) != ncursors {
		return false
	}

	return clipboard == c.getAllText(r)
}

func (c multiClipboard) writeText(text string, r Register, num int, ncursors int) {
	content := c[r]
	if content == nil || len(content) != ncursors {
		content = make([]string, ncursors, ncursors)
		c[r] = content
	}

	if num >= ncursors {
		return
	}

	content[num] = text
}

func init() {
	multi = make(multiClipboard)
}
