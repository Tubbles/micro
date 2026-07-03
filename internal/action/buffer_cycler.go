package action

import (
	"github.com/Tubbles/tcell/v3"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/widget"
)

// cyclerItem pairs a picker row with the pane ID it switches to.
type cyclerItem struct {
	id    uint64
	label string
}

// buildCyclerItems lists every live BufPane in MRU order (most
// recently focused first). Label reuses BufPane.Name(), the exact
// string TabList.UpdateNames feeds the tab bar (including the
// trailing " +" it appends for a modified buffer), so a row's title
// matches what the tab bar would show for that pane.
func buildCyclerItems() []cyclerItem {
	ids := MRU.List(paneAlive)
	items := make([]cyclerItem, 0, len(ids))
	for _, id := range ids {
		if _, _, bp := findPaneByID(id); bp != nil {
			items = append(items, cyclerItem{id: id, label: bp.Name()})
		}
	}
	return items
}

// cyclerBoundKeyNames returns the canonical KeyEvent.Name() strings
// bound to action, by inverting config.Bindings["buffer"] (event name
// -> action string; BufMapEvent is what populates it, keyed by
// k.Name()). Bindings don't fire while a widget is active (widget
// dispatch precedes the binding tree), so the cycler resolves its own
// bound keys once at open time and intercepts them directly via
// Picker.OnKey instead of relying on the normal binding path.
func cyclerBoundKeyNames(action string) []string {
	var names []string
	for name, bound := range config.Bindings["buffer"] {
		if bound == action {
			names = append(names, name)
		}
	}
	return names
}

// newCyclerPicker builds the picker for the MRU buffer cycler from
// already-resolved data, independent of Tabs/findPaneByID, so it is
// unit-testable without a live screen. forwardNames/backwardNames are
// the bound key names from cyclerBoundKeyNames; onSelect receives the
// index into items. Selection starts at index 1 (the previous buffer)
// so a single press-then-commit is an alt-tab toggle; Picker.SetCurrent
// clamps gracefully when there are fewer than 2 items.
func newCyclerPicker(items []cyclerItem, forwardNames, backwardNames []string, rect widget.ScreenRect, onSelect func(index int)) *widget.Picker {
	pickerItems := make([]widget.PickerItem, len(items))
	for i, it := range items {
		pickerItems[i] = widget.PickerItem{Label: it.label}
	}

	var p *widget.Picker
	p = widget.NewPicker(widget.PickerOptions{
		Title: "Buffers",
		Hint:  "Enter: switch  Esc: cancel",
		Items: pickerItems,
		Query: true,
		Geometry: widget.Geometry{
			Kind: widget.GeomScreenRect,
			Rect: rect,
		},
		OnKey: func(e *tcell.EventKey) bool {
			name := keyEvent(e).Name()
			for _, n := range forwardNames {
				if n == name {
					p.MoveWrap(1)
					return true
				}
			}
			for _, n := range backwardNames {
				if n == name {
					p.MoveWrap(-1)
					return true
				}
			}
			return false
		},
		OnSelect: onSelect,
		// OnClose is intentionally nil: Esc/click-outside cancels by
		// doing nothing. The picker is a pure overlay (opening it
		// never touches Tabs/Tab focus), so the origin pane is still
		// the active one for as long as the picker is open; only a
		// commit (OnSelect) ever moves focus. "Return focus to
		// origin" on cancel is therefore already true by construction.
	})
	p.SetCurrent(1)
	return p
}

// commitBufferCycler switches focus to targetID, replicating the
// Tabs.SetActive(ti) + tab.SetActive(pi) pane-switch mechanics used
// throughout internal/action (see applyJump in jumplist.go), and, if
// that switch actually moved focus, records the origin location on
// the jump list so JumpBack returns to where the cycle started
// (D-29). Both SetActive calls run under Jumps.withSuppression so
// their own automatic pushes don't fire: without suppression, a
// cross-tab switch would have Tab.SetActive push whichever pane was
// previously active in the *target* tab, not the origin, once focus
// has already moved to that tab. This function pushes exactly the one
// entry the cycler wants, and only when a real switch happened, so
// re-selecting the pane that was already active is a no-op rather
// than a spurious jump.
func commitBufferCycler(targetID, originID uint64, originBuf *buffer.SharedBuffer, originLoc buffer.Loc) {
	moved := false
	Jumps.withSuppression(func() {
		ti, pi, bp := findPaneByID(targetID)
		if bp == nil {
			return
		}
		if Tabs.Active() != ti {
			Tabs.SetActive(ti)
			moved = true
		}
		if Tabs.List[ti].active != pi {
			Tabs.List[ti].SetActive(pi)
			moved = true
		}
	})
	if moved {
		Jumps.Push(originID, originBuf, originLoc)
	}
}

// openBufferCycler is the shared body of CycleBuffersForward and
// CycleBuffersBackward.
func (h *BufPane) openBufferCycler() bool {
	items := buildCyclerItems()

	originID := h.ID()
	originBuf := h.Buf.SharedBuffer
	originLoc := h.Cursor.Loc

	forward := cyclerBoundKeyNames("CycleBuffersForward")
	backward := cyclerBoundKeyNames("CycleBuffersBackward")

	p := newCyclerPicker(items, forward, backward, widgetOverlayRect(),
		func(index int) {
			widget.CloseActive()
			if index < 0 || index >= len(items) {
				return
			}
			commitBufferCycler(items[index].id, originID, originBuf, originLoc)
		})

	widget.Open(p)
	return true
}

// CycleBuffersForward opens the MRU buffer-cycler picker positioned
// over the editor area, preselecting the previous buffer so a single
// press-then-Enter is an alt-tab toggle. No default binding; see
// runtime/help/keybindings.md for the suggested Ctrl-Tab binding and
// its zellij release-commit caveat (D-27: v1 is press-based, not
// release-to-commit, because a modifier release can't reach micro
// inside zellij).
func (h *BufPane) CycleBuffersForward() bool {
	return h.openBufferCycler()
}

// CycleBuffersBackward opens the same picker as CycleBuffersForward.
// The two actions only differ once the picker is open, where each
// bound key is captured directly by Picker.OnKey as next/prev
// (bindings don't fire while a widget is active); as the action that
// opens the picker, either one behaves identically.
func (h *BufPane) CycleBuffersBackward() bool {
	return h.openBufferCycler()
}
