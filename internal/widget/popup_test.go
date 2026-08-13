package widget

import (
	"reflect"
	"testing"

	"github.com/Tubbles/tcell/v3"
)

func TestWrapToWidth(t *testing.T) {
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
		if got := wrapToWidth(c.text, c.width); !reflect.DeepEqual(got, c.want) {
			t.Errorf("wrapToWidth(%q, %d) = %q, want %q", c.text, c.width, got, c.want)
		}
	}
}

func TestWrapToWidthWideRunes(t *testing.T) {
	// é is 1 cell; 漢 is 2 cells. Width 4 fits 漢漢 but not 漢漢漢.
	got := wrapToWidth("漢漢漢", 4)
	want := []string{"漢漢", "漢"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("wrapToWidth wide runes = %q, want %q", got, want)
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
