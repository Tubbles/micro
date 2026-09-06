package main

import (
	"testing"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/stretchr/testify/assert"
)

// openAndSelect opens a file with content and selects start..end on the
// active cursor, leaving the cursor at end the way an interactive
// selection would. A modified buffer would make the next test's
// "> open" stop at the save-changes prompt, so the buffer is saved on
// cleanup.
func openAndSelect(t *testing.T, content string, start, end buffer.Loc) *buffer.Buffer {
	file := createTestFile(t, content)
	openFile(file)
	buf := findBuffer(file)
	if buf == nil {
		t.Fatalf("Could not find buffer %s", file)
	}
	t.Cleanup(func() {
		buf.ClearCursors()
		if buf.Modified() {
			if err := buf.Save(); err != nil {
				t.Log(err)
			}
		}
	})
	cursor := buf.GetActiveCursor()
	cursor.SetSelectionStart(start)
	cursor.SetSelectionEnd(end)
	cursor.Loc = end
	return buf
}

func TestAutocloseWrapsSelectionAndNests(t *testing.T) {
	buf := openAndSelect(t, "hello world\n", buffer.Loc{0, 0}, buffer.Loc{5, 0})
	cursor := buf.GetActiveCursor()

	injectString("(")
	assert.Equal(t, "(hello) world\n", string(buf.Bytes()))
	assert.Equal(t, "hello", string(cursor.GetSelection()))
	assert.Equal(t, buffer.Loc{6, 0}, cursor.Loc)

	injectString("[")
	assert.Equal(t, "([hello]) world\n", string(buf.Bytes()))
	assert.Equal(t, "hello", string(cursor.GetSelection()))
	assert.Equal(t, buffer.Loc{7, 0}, cursor.Loc)
}

func TestAutocloseWrapsMultiLineBackwardSelection(t *testing.T) {
	// Selected upwards: the cursor sits at the earlier end.
	buf := openAndSelect(t, "ab\ncd\n", buffer.Loc{1, 1}, buffer.Loc{1, 0})
	cursor := buf.GetActiveCursor()

	injectString("{")
	assert.Equal(t, "a{b\nc}d\n", string(buf.Bytes()))
	assert.Equal(t, "b\nc", string(cursor.GetSelection()))
	assert.Equal(t, buffer.Loc{2, 0}, cursor.Loc)
}

func TestAutocloseWrapsEverySelectingCursor(t *testing.T) {
	buf := openAndSelect(t, "foo bar\n", buffer.Loc{0, 0}, buffer.Loc{3, 0})
	second := buffer.NewCursor(buf, buffer.Loc{7, 0})
	second.SetSelectionStart(buffer.Loc{4, 0})
	second.SetSelectionEnd(buffer.Loc{7, 0})
	buf.AddCursor(second)

	injectString("\"")
	assert.Equal(t, "\"foo\" \"bar\"\n", string(buf.Bytes()))
	assert.Equal(t, "foo", string(buf.GetCursor(0).GetSelection()))
	assert.Equal(t, "bar", string(buf.GetCursor(1).GetSelection()))
}

func TestAutocloseStillClosesWithoutSelection(t *testing.T) {
	buf := openAndSelect(t, "x\n", buffer.Loc{1, 0}, buffer.Loc{1, 0})
	cursor := buf.GetActiveCursor()
	cursor.ResetSelection()

	injectString("(")
	assert.Equal(t, "x()\n", string(buf.Bytes()))
	assert.Equal(t, buffer.Loc{2, 0}, cursor.Loc)
}
