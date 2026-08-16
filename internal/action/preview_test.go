package action

import (
	"reflect"
	"testing"

	"github.com/micro-editor/micro/v2/internal/widget"
	"github.com/micro-editor/micro/v2/pkg/highlight"
)

// flattenLine renders a styled line back to plain text so window
// tests can assert on content without coupling to span boundaries.
func flattenLine(line widget.StyledLine) string {
	out := ""
	for _, span := range line {
		out += span.Text
	}
	return out
}

func flattenLines(lines []widget.StyledLine) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = flattenLine(line)
	}
	return out
}

func TestPreviewWindowCentersFocus(t *testing.T) {
	lines := []string{"l0", "l1", "l2", "l3", "l4", "l5", "l6", "l7", "l8", "l9"}

	window, focus := previewWindow(lines, nil, 5, 4)
	// start = 5 - 4/2 = 3 -> lines 3..6, focus at offset 2. The gutter
	// pads to the width of the file's last line number (10 -> 2 cells).
	want := []string{" 4│l3", " 5│l4", " 6│l5", " 7│l6"}
	if got := flattenLines(window); !reflect.DeepEqual(got, want) {
		t.Errorf("window = %q, want %q", got, want)
	}
	if focus != 2 {
		t.Errorf("focus = %d, want 2", focus)
	}
}

func TestPreviewWindowClampsAtFileStart(t *testing.T) {
	lines := []string{"l0", "l1", "l2", "l3", "l4"}

	window, focus := previewWindow(lines, nil, 0, 4)
	want := []string{"1│l0", "2│l1", "3│l2", "4│l3"}
	if got := flattenLines(window); !reflect.DeepEqual(got, want) {
		t.Errorf("window = %q, want %q", got, want)
	}
	if focus != 0 {
		t.Errorf("focus = %d, want 0", focus)
	}
}

func TestPreviewWindowClampsAtFileEnd(t *testing.T) {
	lines := []string{"l0", "l1", "l2", "l3", "l4"}

	window, focus := previewWindow(lines, nil, 4, 4)
	want := []string{"2│l1", "3│l2", "4│l3", "5│l4"}
	if got := flattenLines(window); !reflect.DeepEqual(got, want) {
		t.Errorf("window = %q, want %q", got, want)
	}
	if focus != 3 {
		t.Errorf("focus = %d, want 3", focus)
	}
}

func TestPreviewWindowShortFile(t *testing.T) {
	window, focus := previewWindow([]string{"only"}, nil, 0, 10)
	want := []string{"1│only"}
	if got := flattenLines(window); !reflect.DeepEqual(got, want) {
		t.Errorf("window = %q, want %q", got, want)
	}
	if focus != 0 {
		t.Errorf("focus = %d, want 0", focus)
	}
}

func TestPreviewWindowOutOfRangeFocusClamps(t *testing.T) {
	if _, focus := previewWindow([]string{"l0", "l1", "l2"}, nil, 99, 2); focus < 0 {
		t.Errorf("focus = %d, want clamped to a valid line", focus)
	}
}

func TestPreviewWindowEmpty(t *testing.T) {
	if window, focus := previewWindow(nil, nil, 0, 5); window != nil || focus != -1 {
		t.Errorf("previewWindow(nil) = (%q, %d), want (nil, -1)", flattenLines(window), focus)
	}
}

func TestPreviewWindowGutterGroup(t *testing.T) {
	window, _ := previewWindow([]string{"x"}, nil, 0, 1)
	if len(window) != 1 || len(window[0]) < 1 {
		t.Fatalf("unexpected window shape: %+v", window)
	}
	if got := window[0][0].Group; got != "line-number" {
		t.Errorf("gutter span group = %q, want %q", got, "line-number")
	}
}

func TestStyledSpansForLine(t *testing.T) {
	// Register two groups directly: LineMatch says "kw" starts at rune
	// column 0, default resumes at column 4, "str" starts at column 9.
	if highlight.Groups == nil {
		highlight.Groups = make(map[string]highlight.Group)
	}
	highlight.Groups["testkw"] = 101
	highlight.Groups["teststr"] = 102

	line := "func x = \"y\""
	match := highlight.LineMatch{0: 101, 4: 0, 9: 102}

	spans := styledSpansForLine(line, match)

	want := widget.StyledLine{
		{Text: "func", Group: "testkw"},
		{Text: " x = ", Group: ""},
		{Text: "\"y\"", Group: "teststr"},
	}
	if !reflect.DeepEqual(spans, want) {
		t.Errorf("spans = %+v, want %+v", spans, want)
	}
}

func TestStyledSpansForLineNoMatchIsPlain(t *testing.T) {
	spans := styledSpansForLine("plain", nil)
	if !reflect.DeepEqual(spans, widget.PlainLine("plain")) {
		t.Errorf("spans = %+v, want a single plain span", spans)
	}
}

func TestStyledSpansForLineBreakpointPastEOL(t *testing.T) {
	// A breakpoint beyond the line's rune count (possible with stale
	// or byte-vs-rune-confused matches) must not panic or emit text.
	spans := styledSpansForLine("ab", highlight.LineMatch{0: 0, 99: 5})
	if got := flattenLine(spans); got != "ab" {
		t.Errorf("flattened = %q, want %q", got, "ab")
	}
}
