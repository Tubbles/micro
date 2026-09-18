package action

import (
	"github.com/micro-editor/micro/v2/internal/config"
)

// CloseOtherTabs closes every tab except the current one. Each buffer
// with unsaved changes gets the same save prompt Quit shows, one at a
// time, and cancelling a prompt keeps that tab and the ones after it.
func (h *BufPane) CloseOtherTabs() bool {
	var others []*Tab
	for _, tab := range Tabs.List {
		if tab != h.tab {
			others = append(others, tab)
		}
	}
	return closeTabs(others)
}

// CloseTabsToTheRight closes every tab after the current one.
func (h *BufPane) CloseTabsToTheRight() bool {
	index := Tabs.IndexOf(h.tab)
	if index < 0 {
		return false
	}
	return closeTabs(copyTabs(Tabs.List[index+1:]))
}

// CloseTabsToTheLeft closes every tab before the current one.
func (h *BufPane) CloseTabsToTheLeft() bool {
	index := Tabs.IndexOf(h.tab)
	if index < 0 {
		return false
	}
	return closeTabs(copyTabs(Tabs.List[:index]))
}

// CloseAllTabs closes every tab and leaves one fresh empty buffer, so the
// editor stays open. QuitAll is the action that exits.
func (h *BufPane) CloseAllTabs() bool {
	all := copyTabs(Tabs.List)
	h.AddTab()
	return closeTabs(all)
}

func copyTabs(tabs []*Tab) []*Tab {
	out := make([]*Tab, len(tabs))
	copy(out, tabs)
	return out
}

// closeTabs closes tabs in order through the same paths Quit and the
// terminal pane's Quit use, so unsaved buffers prompt and terminals shut
// down. Prompts are asynchronous, so the walk continues from each
// prompt's callback, and a cancelled prompt ends it with the remaining
// tabs untouched. A tab is made active while its panes close, because
// ForceQuit and TermPane.Quit act on the active tab; the tab that was
// active when the walk started is restored afterwards, also after a
// cancel. Returns false when there is nothing to close.
func closeTabs(tabs []*Tab) bool {
	if len(tabs) == 0 {
		return false
	}
	keep := Tabs.List[Tabs.Active()]
	finish := func() {
		if index := Tabs.IndexOf(keep); index >= 0 {
			Tabs.SetActive(index)
		}
	}

	var closeNext func()
	closeNext = func() {
		// A tab leaves Tabs.List when its last pane goes away.
		for len(tabs) > 0 && Tabs.IndexOf(tabs[0]) < 0 {
			tabs = tabs[1:]
		}
		if len(tabs) == 0 {
			finish()
			return
		}
		tab := tabs[0]
		Tabs.SetActive(Tabs.IndexOf(tab))
		closePane(tab.Panes[0], closeNext, finish)
	}
	closeNext()
	return true
}

// closePane closes one pane the way Quit would, then calls next. Buffer
// panes prompt for unsaved changes first; a cancelled prompt calls
// cancelled instead of next. A pane of a kind this does not know how
// to close ends the walk rather than looping on it.
func closePane(pane Pane, next, cancelled func()) {
	switch p := pane.(type) {
	case *BufPane:
		p.closeWhenSafe(func() {
			p.ForceQuit()
			next()
		}, cancelled)
	case *RawPane:
		p.closeWhenSafe(func() {
			p.ForceQuit()
			next()
		}, cancelled)
	case *TermPane:
		p.Quit()
		next()
	}
}

// closeWhenSafe runs then once this pane's buffer may be closed: right
// away when it has no unsaved changes of its own (a buffer still shown
// in another pane is not lost by closing this one), after an autosave
// when that option is on, and otherwise after the user answers the save
// prompt. Escape at the prompt calls cancelled instead. Quit makes the
// same decision inline through closePrompt, which has no cancel path,
// hence the prompt is issued here with the same wording.
func (h *BufPane) closeWhenSafe(then, cancelled func()) {
	if !h.Buf.Modified() || h.Buf.Shared() {
		then()
		return
	}
	if config.GlobalSettings["autosave"].(float64) > 0 && h.Buf.Path != "" {
		h.SaveCB("Quit", then)
		return
	}
	InfoBar.YNPrompt("Save changes to "+h.Buf.GetName()+" before closing? (y,n,esc)", func(yes, canceled bool) {
		switch {
		case canceled:
			cancelled()
		case yes:
			h.SaveCB("Quit", then)
		default:
			then()
		}
	})
}
