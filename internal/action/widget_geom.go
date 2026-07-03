package action

import (
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/micro/v2/internal/widget"
)

// widgetOverlayRect returns the screen rectangle used to center a
// full-editor overlay widget (the file picker, file explorer, command
// palette, buffer cycler) inside the editor area, inset by a uniform margin
// below the tab bar and above the info bar. This is the shared geometry
// that the file explorer and command palette each computed locally
// (editorAreaRect, commandPaletteRect); those consumers switch to this
// helper in their own commits.
func widgetOverlayRect() widget.ScreenRect {
	sw, sh := screen.Screen.Size()
	iOff := config.GetInfoBarOffset()
	tabBar := 0
	if Tabs != nil && len(Tabs.List) > 1 {
		tabBar = 1
	}
	const margin = 2
	x := margin
	y := tabBar + margin
	w := sw - 2*margin
	h := (sh - tabBar - iOff) - 2*margin
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return widget.ScreenRect{X: x, Y: y, W: w, H: h}
}
