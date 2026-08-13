package action

import (
	"reflect"
	"testing"
)

func TestPreviewWindowCentersFocus(t *testing.T) {
	lines := []string{"l0", "l1", "l2", "l3", "l4", "l5", "l6", "l7", "l8", "l9"}

	window, focus := previewWindow(lines, 5, 4)
	// start = 5 - 4/2 = 3 -> lines 3..6, focus at offset 2. The gutter
	// pads to the width of the file's last line number (10 -> 2 cells).
	want := []string{" 4│l3", " 5│l4", " 6│l5", " 7│l6"}
	if !reflect.DeepEqual(window, want) {
		t.Errorf("window = %q, want %q", window, want)
	}
	if focus != 2 {
		t.Errorf("focus = %d, want 2", focus)
	}
}

func TestPreviewWindowClampsAtFileStart(t *testing.T) {
	lines := []string{"l0", "l1", "l2", "l3", "l4"}

	window, focus := previewWindow(lines, 0, 4)
	want := []string{"1│l0", "2│l1", "3│l2", "4│l3"}
	if !reflect.DeepEqual(window, want) {
		t.Errorf("window = %q, want %q", window, want)
	}
	if focus != 0 {
		t.Errorf("focus = %d, want 0", focus)
	}
}

func TestPreviewWindowClampsAtFileEnd(t *testing.T) {
	lines := []string{"l0", "l1", "l2", "l3", "l4"}

	window, focus := previewWindow(lines, 4, 4)
	want := []string{"2│l1", "3│l2", "4│l3", "5│l4"}
	if !reflect.DeepEqual(window, want) {
		t.Errorf("window = %q, want %q", window, want)
	}
	if focus != 3 {
		t.Errorf("focus = %d, want 3", focus)
	}
}

func TestPreviewWindowShortFile(t *testing.T) {
	lines := []string{"only"}

	window, focus := previewWindow(lines, 0, 10)
	want := []string{"1│only"}
	if !reflect.DeepEqual(window, want) {
		t.Errorf("window = %q, want %q", window, want)
	}
	if focus != 0 {
		t.Errorf("focus = %d, want 0", focus)
	}
}

func TestPreviewWindowOutOfRangeFocusClamps(t *testing.T) {
	lines := []string{"l0", "l1", "l2"}

	_, focus := previewWindow(lines, 99, 2)
	if focus < 0 {
		t.Errorf("focus = %d, want clamped to a valid line", focus)
	}
}

func TestPreviewWindowEmpty(t *testing.T) {
	if window, focus := previewWindow(nil, 0, 5); window != nil || focus != -1 {
		t.Errorf("previewWindow(nil) = (%q, %d), want (nil, -1)", window, focus)
	}
}
