package action

import (
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/micro/v2/internal/views"
)

// movePaneInTabList shifts the active pane one slot forward (dir=+1) or
// backward (dir=-1) through the global pane sequence: every tab's leaves
// taken in tree-order, concatenated. Three cases drive the behavior.
//
//   - Adjacent slot is in the same tab: swap visual positions by
//     exchanging split IDs. Slice index of the active pane doesn't
//     change; only which leaf node the pane draws into.
//   - Adjacent slot is in a neighboring tab: detach from src, attach
//     into dst via VSplit on dst's leftmost (or rightmost) leaf. If src
//     loses its last pane, src tab is dropped from Tabs.List.
//   - Adjacent slot would be past the global edge: open a fresh tab
//     beyond the last (or before the first) and put the pane there.
//     Skipped if src has only this one pane, since the result would be
//     the same shape with the same content.
//
// Returns true if the editor state changed.
func movePaneInTabList(srcTab *Tab, srcPaneIdx, dir int) bool {
	if srcTab == nil || srcPaneIdx < 0 || srcPaneIdx >= len(srcTab.Panes) {
		return false
	}
	srcTabIdx := tabsIndexOf(srcTab)
	if srcTabIdx < 0 {
		return false
	}

	p := srcTab.Panes[srcPaneIdx]

	leaves := collectLeafIDs(srcTab.Node)
	leafIdx := -1
	for i, id := range leaves {
		if id == p.ID() {
			leafIdx = i
			break
		}
	}
	if leafIdx < 0 {
		return false
	}

	adjLeaf := leafIdx + dir
	if adjLeaf >= 0 && adjLeaf < len(leaves) {
		swapPaneIDs(srcTab, p.ID(), leaves[adjLeaf])
		return true
	}

	dstTabIdx := srcTabIdx + dir
	if dstTabIdx >= 0 && dstTabIdx < len(Tabs.List) {
		movePaneIntoExistingTab(p, srcTab, Tabs.List[dstTabIdx], dir)
		return true
	}

	if len(srcTab.Panes) == 1 {
		return false
	}
	movePaneToNewEdgeTab(p, srcTab, dir)
	return true
}

func tabsIndexOf(t *Tab) int {
	for i, tab := range Tabs.List {
		if tab == t {
			return i
		}
	}
	return -1
}

// collectLeafIDs walks the split tree in DFS order and returns each
// leaf's ID. Reading order: left-to-right for STHoriz parents,
// top-to-bottom for STVert parents.
func collectLeafIDs(n *views.Node) []uint64 {
	if n == nil {
		return nil
	}
	var out []uint64
	var walk func(*views.Node)
	walk = func(node *views.Node) {
		if node.IsLeaf() {
			out = append(out, node.ID())
			return
		}
		for _, c := range node.Children() {
			walk(c)
		}
	}
	walk(n)
	return out
}

// swapPaneIDs swaps the visual positions of two panes in t by
// exchanging their split IDs. The split tree leaves keep their geometry
// in place; only which pane renders into which leaf changes. The active
// slice index is preserved, so whatever pane was active stays active
// (now drawing in the swapped slot).
func swapPaneIDs(t *Tab, idA, idB uint64) {
	if idA == idB {
		return
	}
	aIdx := t.GetPane(idA)
	bIdx := t.GetPane(idB)
	paneA := t.Panes[aIdx]
	paneB := t.Panes[bIdx]
	paneA.SetID(idB)
	paneB.SetID(idA)
	t.Resize()
}

// movePaneIntoExistingTab detaches p from src and attaches it as the
// leftmost (dir=+1) or rightmost (dir=-1) leaf of dst via a vsplit on
// dst's edge leaf. p becomes the active pane of dst, dst becomes the
// active tab, and src is removed from Tabs.List if it has no panes
// left.
func movePaneIntoExistingTab(p Pane, src, dst *Tab, dir int) {
	detachPaneFromTab(p, src)

	dstLeaves := collectLeafIDs(dst.Node)
	var anchorID uint64
	var rightSide bool
	if dir > 0 {
		anchorID = dstLeaves[0]
		rightSide = false
	} else {
		anchorID = dstLeaves[len(dstLeaves)-1]
		rightSide = true
	}
	anchorNode := dst.GetNode(anchorID)
	newID := anchorNode.VSplit(rightSide)

	p.SetID(newID)
	p.SetTab(dst)
	dst.Panes = append(dst.Panes, p)

	finalizeAfterMove(src, dst, len(dst.Panes)-1)
}

// movePaneToNewEdgeTab detaches p from src and wraps it in a fresh tab
// inserted immediately after src (dir=+1) or before src (dir=-1) in
// Tabs.List. The new tab becomes active.
func movePaneToNewEdgeTab(p Pane, src *Tab, dir int) {
	detachPaneFromTab(p, src)

	w, h := screen.Screen.Size()
	iOffset := config.GetInfoBarOffset()
	newTab := NewTabFromPane(0, 1, w, h-1-iOffset, p)

	srcIdx := tabsIndexOf(src)
	insIdx := srcIdx
	if dir > 0 {
		insIdx = srcIdx + 1
	}
	Tabs.List = append(Tabs.List, nil)
	copy(Tabs.List[insIdx+1:], Tabs.List[insIdx:])
	Tabs.List[insIdx] = newTab

	finalizeAfterMove(src, newTab, 0)
}

// detachPaneFromTab removes p from src's split tree and panes slice.
// When src holds only this one pane the leaf is the root and cannot be
// unsplit, so the tree call is skipped; the caller is expected to drop
// or replace the now-empty src tab.
func detachPaneFromTab(p Pane, src *Tab) {
	leaf := src.GetNode(p.ID())
	if len(src.Panes) > 1 && leaf != nil {
		leaf.Unsplit()
	}
	idx := src.GetPane(p.ID())
	src.RemovePane(idx)
}

// finalizeAfterMove handles bookkeeping after a cross-tab move: drop
// emptied source tabs, clamp src's active index, set dst's active pane,
// promote dst to the active tab, and trigger the layout/redraw passes.
func finalizeAfterMove(src, dst *Tab, dstPaneIdx int) {
	if len(src.Panes) == 0 {
		idx := tabsIndexOf(src)
		if idx >= 0 {
			copy(Tabs.List[idx:], Tabs.List[idx+1:])
			Tabs.List[len(Tabs.List)-1] = nil
			Tabs.List = Tabs.List[:len(Tabs.List)-1]
		}
	} else {
		if src.active >= len(src.Panes) {
			src.active = len(src.Panes) - 1
		}
		src.SetActive(src.active)
		src.Resize()
	}

	dst.SetActive(dstPaneIdx)
	dst.Resize()

	dstIdx := tabsIndexOf(dst)
	if dstIdx >= 0 {
		Tabs.SetActive(dstIdx)
	}
	Tabs.Resize()
	Tabs.UpdateNames()
}
