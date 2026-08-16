package widget

import (
	"reflect"
	"testing"

	"github.com/Tubbles/tcell/v3"
)

// joinWrapped flattens wrapped styled lines back to plain strings for
// assertion.
func joinWrapped(lines []StyledLine) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		for _, span := range line {
			out[i] += span.Text
		}
	}
	return out
}

func TestWrapStyledText(t *testing.T) {
	cases := []struct {
		text  string
		width int
		want  []string
	}{
		{"", 10, []string{""}},
		{"short", 10, []string{"short"}},
		{"exactlyten", 10, []string{"exactlyten"}},
		{"elevenchars", 10, []string{"elevenchar", "s"}},
		{"a\nb", 10, []string{"a", "b"}},
		{"a\n\nb", 10, []string{"a", "", "b"}},
		{"unwrapped when width zero", 0, []string{"unwrapped when width zero"}},
	}
	for _, c := range cases {
		got := joinWrapped(wrapStyledLines(TextToLines(c.text), c.width))
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("wrap(%q, %d) = %q, want %q", c.text, c.width, got, c.want)
		}
	}
}

func TestWrapStyledTextWideRunes(t *testing.T) {
	// 漢 is 2 cells wide. Width 4 fits 漢漢 but not 漢漢漢.
	got := joinWrapped(wrapStyledLines(TextToLines("漢漢漢"), 4))
	want := []string{"漢漢", "漢"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("wrap wide runes = %q, want %q", got, want)
	}
}

func TestWrapStyledLinePreservesGroupsAcrossWrap(t *testing.T) {
	line := StyledLine{
		{Text: "aaaa", Group: "g1"},
		{Text: "bbbb", Group: "g2"},
	}
	wrapped := wrapStyledLine(line, 6)
	if len(wrapped) != 2 {
		t.Fatalf("wrapped into %d lines, want 2: %+v", len(wrapped), wrapped)
	}
	want0 := StyledLine{{Text: "aaaa", Group: "g1"}, {Text: "bb", Group: "g2"}}
	want1 := StyledLine{{Text: "bb", Group: "g2"}}
	if !reflect.DeepEqual(wrapped[0], want0) {
		t.Errorf("line 0 = %+v, want %+v", wrapped[0], want0)
	}
	if !reflect.DeepEqual(wrapped[1], want1) {
		t.Errorf("line 1 = %+v, want %+v", wrapped[1], want1)
	}
}

func TestPopupEscDismisses(t *testing.T) {
	closed := false
	p := NewPopup(PopupOptions{
		Text:    "hello",
		OnClose: func() { closed = true },
	})
	Open(p)
	if Active() != p {
		t.Fatal("popup did not become the active widget")
	}

	consumed := HandleEvent(tcell.NewEventKey(tcell.KeyEsc, "", tcell.ModNone))
	if !consumed {
		t.Error("Esc was not consumed")
	}
	if Active() != nil {
		t.Error("popup still active after Esc")
	}
	if !closed {
		t.Error("OnClose did not fire")
	}
}

func TestPopupEnterDismisses(t *testing.T) {
	p := NewPopup(PopupOptions{Text: "hello"})
	Open(p)
	defer CloseActive()

	HandleEvent(tcell.NewEventKey(tcell.KeyEnter, "", tcell.ModNone))
	if Active() != nil {
		t.Error("popup still active after Enter")
	}
}

func TestPopupIsModal(t *testing.T) {
	// Any other key must be consumed without dismissing the popup.
	p := NewPopup(PopupOptions{Text: "hello"})
	Open(p)
	defer CloseActive()

	consumed := HandleEvent(tcell.NewEventKey(tcell.KeyRune, "x", tcell.ModNone))
	if !consumed {
		t.Error("rune key was not consumed")
	}
	if Active() != p {
		t.Error("popup dismissed by a non-Esc key; want it to stay open")
	}
}

func TestPopupScrollClamps(t *testing.T) {
	p := NewPopup(PopupOptions{Text: "a\nb\nc\nd\ne"})
	p.lastLineCount = 5
	p.lastBodyH = 2

	p.scroll(100)
	if p.top != 3 {
		t.Errorf("scroll past end: top = %d, want 3 (lineCount-bodyH)", p.top)
	}
	p.scroll(-100)
	if p.top != 0 {
		t.Errorf("scroll past start: top = %d, want 0", p.top)
	}
}
