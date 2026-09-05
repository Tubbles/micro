package clipboard

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// testReg is a register private to these tests so they do not disturb
// the real clipboard registers.
const testReg Register = 100

func writeThreeCursors() {
	writeMulti("alpha", testReg, 0, 3, Internal)
	writeMulti("beta", testReg, 1, 3, Internal)
	writeMulti("gamma", testReg, 2, 3, Internal)
}

func TestMultiCursorCopyJoinsSlotsWithNewlines(t *testing.T) {
	writeThreeCursors()

	clip, err := read(testReg, Internal)
	assert.Nil(t, err)
	assert.Equal(t, "alpha\nbeta\ngamma", clip)
}

func TestMultiCursorPasteWithMatchingCursorCountSpreads(t *testing.T) {
	writeThreeCursors()

	clip, err := read(testReg, Internal)
	assert.Nil(t, err)
	assert.True(t, ValidMulti(testReg, clip, 3))
	assert.Equal(t, "alpha", ReadMultiText(clip, testReg, 0, 3))
	assert.Equal(t, "beta", ReadMultiText(clip, testReg, 1, 3))
	assert.Equal(t, "gamma", ReadMultiText(clip, testReg, 2, 3))
}

func TestMultiCursorPasteWithSingleCursorGetsWholeClip(t *testing.T) {
	writeThreeCursors()

	clip, err := read(testReg, Internal)
	assert.Nil(t, err)
	assert.False(t, ValidMulti(testReg, clip, 1))
	assert.Equal(t, "alpha\nbeta\ngamma", ReadMultiText(clip, testReg, 0, 1))
}

func TestMultiCursorPasteAfterExternalClipboardChangeFallsBack(t *testing.T) {
	writeThreeCursors()

	assert.False(t, ValidMulti(testReg, "something else", 3))
	assert.Equal(t, "something else", ReadMultiText("something else", testReg, 1, 3))
}
