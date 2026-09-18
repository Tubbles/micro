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
// index into items. preselect is the initially highlighted row: index 1
// (the previous buffer) makes a single press-then-commit an alt-tab
// toggle, index 0 (the current buffer) leaves the list unmoved so the
// first backward step lands on the oldest entry. Picker.SetCurrent
// clamps gracefully when there are fewer items than that.
func newCyclerPicker(items []cyclerItem, forwardNames, backwardNames []string, preselect int, rect widget.ScreenRect, onSelect func(index int)) *widget.Picker {
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
	p.SetCurrent(preselect)
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
// CycleBuffersBackward, differing only in which row starts highlighted.
// SwitchToRecentBuffer's keys join the forward set: it is the action a
// user is most likely to be holding when they decide they want the list
// after all, so it has to keep stepping down once the list is up.
func (h *BufPane) openBufferCycler(preselect int) bool {
	items := buildCyclerItems()

	originID := h.ID()
	originBuf := h.Buf.SharedBuffer
	originLoc := h.Cursor.Loc

	forward := append(cyclerBoundKeyNames("CycleBuffersForward"), cyclerBoundKeyNames("SwitchToRecentBuffer")...)
	backward := cyclerBoundKeyNames("CycleBuffersBackward")

	p := newCyclerPicker(items, forward, backward, preselect, halfScreenOverlayRect(),
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

// SwitchToRecentBuffer switches straight to the most recently focused
// other pane, no picker. It commits the same row the cycler picker
// preselects, through the same commitBufferCycler, so the jump list
// entry is identical either way. Repeated presses toggle between the
// two most recent panes: committing a switch re-touches the MRU order,
// which puts the pane just left behind at index 1 again.
//
// This is the everyday half of the cycler (D-27): a modifier release
// can't reach micro inside zellij, so there is no hold-and-tap gesture
// to build on, and the list is only worth drawing when the user wants
// to look past the previous buffer. No default binding; see
// runtime/help/keybindings.md for the suggested Ctrl-Tab binding.
func (h *BufPane) SwitchToRecentBuffer() bool {
	items := buildCyclerItems()
	if len(items) < 2 {
		return false
	}
	commitBufferCycler(items[1].id, h.ID(), h.Buf.SharedBuffer, h.Cursor.Loc)
	return true
}

// CycleBuffersForward opens the MRU buffer-cycler picker centered over
// the editor area, preselecting the previous buffer so a single
// press-then-Enter is an alt-tab toggle.
func (h *BufPane) CycleBuffersForward() bool {
	return h.openBufferCycler(1)
}

// CycleBuffersBackward opens the same picker as CycleBuffersForward,
// but highlighting the current buffer, so the first backward step from
// there lands on the least recently used pane rather than skipping it.
func (h *BufPane) CycleBuffersBackward() bool {
	return h.openBufferCycler(0)
}
