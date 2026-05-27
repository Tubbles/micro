package buffer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func msgAt(start, end Loc) *Message {
	return NewMessage("test", "", start, end, MTInfo)
}

func TestMessagesUnderLoc_Empty(t *testing.T) {
	b := NewBufferFromString("hello\nworld\n", "", BTDefault)
	got := b.MessagesUnderLoc(Loc{0, 0})
	assert.Empty(t, got)
}

func TestMessagesUnderLoc_SingleRange(t *testing.T) {
	b := NewBufferFromString("hello world", "", BTDefault)
	m := msgAt(Loc{2, 0}, Loc{6, 0})
	b.AddMessage(m)

	assert.Empty(t, b.MessagesUnderLoc(Loc{1, 0}), "before range")
	assert.Equal(t, []*Message{m}, b.MessagesUnderLoc(Loc{2, 0}), "at start")
	assert.Equal(t, []*Message{m}, b.MessagesUnderLoc(Loc{4, 0}), "inside")
	assert.Equal(t, []*Message{m}, b.MessagesUnderLoc(Loc{6, 0}), "at end")
	assert.Empty(t, b.MessagesUnderLoc(Loc{7, 0}), "after range")
	assert.Empty(t, b.MessagesUnderLoc(Loc{4, 1}), "different line")
}

func TestMessagesUnderLoc_LineOnlySentinel(t *testing.T) {
	b := NewBufferFromString("aaa\nbbb\nccc\n", "", BTDefault)
	// NewMessageAtLine produces Start=End=Loc{-1, line-1}
	m := NewMessageAtLine("test", "", 2, MTWarning)
	b.AddMessage(m)

	assert.Empty(t, b.MessagesUnderLoc(Loc{0, 0}), "earlier line")
	assert.Equal(t, []*Message{m}, b.MessagesUnderLoc(Loc{0, 1}), "col 0 of target line")
	assert.Equal(t, []*Message{m}, b.MessagesUnderLoc(Loc{99, 1}), "any column on target line")
	assert.Empty(t, b.MessagesUnderLoc(Loc{0, 2}), "later line")
}

func TestMessagesUnderLoc_MultiLineRange(t *testing.T) {
	b := NewBufferFromString("line0\nline1\nline2\nline3\n", "", BTDefault)
	m := msgAt(Loc{2, 1}, Loc{3, 2})
	b.AddMessage(m)

	assert.Empty(t, b.MessagesUnderLoc(Loc{1, 1}), "before start, same start line")
	assert.Equal(t, []*Message{m}, b.MessagesUnderLoc(Loc{2, 1}), "at start")
	assert.Equal(t, []*Message{m}, b.MessagesUnderLoc(Loc{99, 1}), "after start, on start line")
	assert.Equal(t, []*Message{m}, b.MessagesUnderLoc(Loc{0, 2}), "start of next line")
	assert.Equal(t, []*Message{m}, b.MessagesUnderLoc(Loc{3, 2}), "at end")
	assert.Empty(t, b.MessagesUnderLoc(Loc{4, 2}), "after end")
}

func TestMessagesUnderLoc_OverlappingPreservesOrder(t *testing.T) {
	b := NewBufferFromString("aaaaaaaaaa", "", BTDefault)
	a := msgAt(Loc{0, 0}, Loc{9, 0})
	c := msgAt(Loc{3, 0}, Loc{6, 0})
	d := msgAt(Loc{4, 0}, Loc{4, 0})
	b.AddMessage(a)
	b.AddMessage(c)
	b.AddMessage(d)

	assert.Equal(t, []*Message{a}, b.MessagesUnderLoc(Loc{0, 0}))
	assert.Equal(t, []*Message{a, c}, b.MessagesUnderLoc(Loc{3, 0}))
	assert.Equal(t, []*Message{a, c, d}, b.MessagesUnderLoc(Loc{4, 0}))
	assert.Equal(t, []*Message{a, c}, b.MessagesUnderLoc(Loc{5, 0}))
	assert.Equal(t, []*Message{a}, b.MessagesUnderLoc(Loc{7, 0}))
}
