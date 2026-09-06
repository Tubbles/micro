package buffer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestByteOffsetCountsBytesOfMultiByteCharacters(t *testing.T) {
	b := NewBufferFromString("héllo\nwörld", "", BTDefault)

	assert.Equal(t, 0, ByteOffset(Loc{0, 0}, b))
	assert.Equal(t, 1, ByteOffset(Loc{1, 0}, b))
	// "hé" is three bytes.
	assert.Equal(t, 3, ByteOffset(Loc{2, 0}, b))
	assert.Equal(t, 6, ByteOffset(Loc{5, 0}, b))
	// Line 1 starts after "héllo\n".
	assert.Equal(t, 7, ByteOffset(Loc{0, 1}, b))
	assert.Equal(t, 10, ByteOffset(Loc{2, 1}, b))
	assert.Equal(t, len("héllo\nwörld"), ByteOffset(b.End(), b))
}
