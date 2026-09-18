package action

import (
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/micro/v2/internal/widget"
)

// widgetOverlayRect returns the screen rectangle used to center a
// full-editor overlay widget (the file picker, file explorer, command
// palette) inside the editor area, inset by a uniform margin
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

// halfScreenOverlayRect is widgetOverlayRect's half-size sibling: it
// reads the live screen/tab-bar/info-bar values and hands them to
// centeredHalfRect, which holds the arithmetic so it can be tested
// without a screen.
func halfScreenOverlayRect() widget.ScreenRect {
	screenWidth, screenHeight := screen.Screen.Size()
	tabBarHeight := 0
	if Tabs != nil && len(Tabs.List) > 1 {
		tabBarHeight = 1
	}
	return centeredHalfRect(screenWidth, screenHeight, tabBarHeight, config.GetInfoBarOffset())
}

// centeredHalfRect centers a rectangle half the screen's width and
// height inside the editor area, the band below the tab bar
// (tabBarHeight rows, 0 when the bar is hidden) and above the info bar
// (infoBarOffset rows). Half of the *screen*, not of the editor area,
// so the overlay keeps the same size whether or not the tab bar shows.
func centeredHalfRect(screenWidth, screenHeight, tabBarHeight, infoBarOffset int) widget.ScreenRect {
	editorHeight := screenHeight - tabBarHeight - infoBarOffset
	if editorHeight < 0 {
		editorHeight = 0
	}
	width := screenWidth / 2
	if width < 0 {
		width = 0
	}
	height := screenHeight / 2
	if height > editorHeight {
		// A screen barely taller than its own chrome has less room
		// than half its height to give; the editor area wins.
		height = editorHeight
	}
	return widget.ScreenRect{
		X: (screenWidth - width) / 2,
		Y: tabBarHeight + (editorHeight-height)/2,
		W: width,
		H: height,
	}
}
