package buffer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// newAnchoredBuffer anchors "two\nthree\n" inside a four-line buffer.
func newAnchoredBuffer(t *testing.T) (*Buffer, *Anchor) {
	b := NewBufferFromString("one\ntwo\nthree\nfour", "", BTDefault)
	anchor := b.AddAnchor(Loc{0, 1}, Loc{0, 3})
	assert.Equal(t, "two\nthree\n", string(anchor.Text()))
	return b, anchor
}

func TestAddAnchorOrdersBounds(t *testing.T) {
	b := NewBufferFromString("one\ntwo\nthree\nfour", "", BTDefault)
	anchor := b.AddAnchor(Loc{0, 3}, Loc{0, 1})
	assert.Equal(t, Loc{0, 1}, anchor.Start())
	assert.Equal(t, Loc{0, 3}, anchor.End())
}

func TestAnchorFollowsInsertAbove(t *testing.T) {
	b, anchor := newAnchoredBuffer(t)
	b.Insert(Loc{0, 0}, "zero\n")
	assert.Equal(t, Loc{0, 2}, anchor.Start())
	assert.Equal(t, Loc{0, 4}, anchor.End())
	assert.Equal(t, "two\nthree\n", string(anchor.Text()))
}

func TestAnchorFollowsInsertAtStart(t *testing.T) {
	b, anchor := newAnchoredBuffer(t)
	b.Insert(Loc{0, 1}, ">> ")
	assert.Equal(t, Loc{3, 1}, anchor.Start())
	assert.Equal(t, "two\nthree\n", string(anchor.Text()))
}

func TestAnchorIgnoresEditsBelow(t *testing.T) {
	b, anchor := newAnchoredBuffer(t)
	b.Insert(Loc{2, 3}, "!")
	assert.Equal(t, Loc{0, 1}, anchor.Start())
	assert.Equal(t, Loc{0, 3}, anchor.End())
	assert.Equal(t, "two\nthree\n", string(anchor.Text()))
}

func TestAnchorFollowsRemoveAbove(t *testing.T) {
	b, anchor := newAnchoredBuffer(t)
	b.Remove(Loc{0, 0}, Loc{0, 1})
	assert.Equal(t, Loc{0, 0}, anchor.Start())
	assert.Equal(t, Loc{0, 2}, anchor.End())
	assert.Equal(t, "two\nthree\n", string(anchor.Text()))
}

func TestAnchorBoundInsideRemovalCollapsesToRemovalStart(t *testing.T) {
	b, anchor := newAnchoredBuffer(t)
	// Removes "ne\nt", which straddles the anchor start.
	b.Remove(Loc{1, 0}, Loc{1, 1})
	assert.Equal(t, "owo\nthree\nfour", string(b.Bytes()))
	assert.Equal(t, Loc{1, 0}, anchor.Start())
	assert.Equal(t, Loc{0, 2}, anchor.End())
	assert.Equal(t, "wo\nthree\n", string(anchor.Text()))
}

func TestAnchorFollowsUndoAndRedo(t *testing.T) {
	b, anchor := newAnchoredBuffer(t)
	b.Insert(Loc{0, 0}, "zero\n")
	b.Undo()
	assert.Equal(t, Loc{0, 1}, anchor.Start())
	assert.Equal(t, Loc{0, 3}, anchor.End())
	b.Redo()
	assert.Equal(t, Loc{0, 2}, anchor.Start())
	assert.Equal(t, Loc{0, 4}, anchor.End())
}

func TestRemovedAnchorStopsFollowing(t *testing.T) {
	b, anchor := newAnchoredBuffer(t)
	b.RemoveAnchor(anchor)
	b.RemoveAnchor(anchor)
	b.Insert(Loc{0, 0}, "zero\n")
	assert.Equal(t, Loc{0, 1}, anchor.Start())
}

func TestAnchorTextIsNilWhenOutOfBounds(t *testing.T) {
	b, anchor := newAnchoredBuffer(t)
	// A raw removal bypasses the text-event hook, leaving the anchor stale.
	b.remove(b.Start(), b.End())
	assert.Nil(t, anchor.Text())
}
