package action

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/util"
	"github.com/micro-editor/micro/v2/internal/views"
	"github.com/micro-editor/micro/v2/internal/workspace"
)

// currentWorkspaceDir is the directory of the dir-backed workspace
// that is currently active, or "" when none is (a plain `micro
// file.txt` session). It is what SaveActiveWorkspace saves and what
// OpenDirWorkspace saves-and-replaces on a switch.
var currentWorkspaceDir string

// encodeLeafPath stores a leaf's path workspace-relative when it
// falls under dir, absolute otherwise (D-54), so a workspace saved
// from one checkout replays correctly as long as the relative
// structure still holds. An unnamed scratch buffer encodes as "".
func encodeLeafPath(bp *BufPane, dir string) string {
	if bp.Buf.Path == "" {
		return ""
	}
	rel, err := filepath.Rel(dir, bp.Buf.AbsPath)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return bp.Buf.AbsPath
	}
	return rel
}

// childWeight returns c's share of parent along the axis that varies
// for parent's split kind: width for views.STHoriz (panes arranged
// side by side), height for views.STVert (panes stacked).
func childWeight(parent, c *views.Node) float64 {
	if parent.Kind == views.STHoriz {
		return float64(c.W)
	}
	return float64(c.H)
}

// encodeNode converts a live split-tree node into its saved form. It
// drops TermPane/RawPane leaves (D-54: a running child process is
// not restorable) and, when pruning empties out or collapses a
// split's children, prunes or promotes the same way micro's own
// Node.flatten does. It returns the encoded node (nil if nothing
// under n is worth saving) alongside the ordered list of surviving
// leaf panes, which the caller uses to translate the tab's active
// pane into an index into the saved (pruned) leaf order.
//
// captureContent is the persistent scratch workspace's opt-in (D-55):
// when true, a leaf with no path also gets its buffer's live text
// copied into Content, since a scratch buffer has no file to reload
// from on restore. It only applies to BTDefault buffers: the same
// empty-path leaf shape also covers auxiliary panes (log, help, raw,
// ...), whose content is never meant to be replayed as user text.
// Dir-backed workspaces pass false and behave exactly as before this
// field existed.
func encodeNode(n *views.Node, paneByID map[uint64]Pane, dir string, captureContent bool) (*workspace.Node, []*BufPane) {
	if n.IsLeaf() {
		p, ok := paneByID[n.ID()]
		if !ok {
			return nil, nil
		}
		bp, ok := p.(*BufPane)
		if !ok {
			// TermPane, RawPane, etc: no live process to restore.
			return nil, nil
		}
		cur := bp.Buf.GetActiveCursor().Loc
		node := &workspace.Node{
			Kind:   "leaf",
			Path:   encodeLeafPath(bp, dir),
			Cursor: &workspace.CursorLoc{X: cur.X, Y: cur.Y},
		}
		if captureContent && node.Path == "" && bp.Buf.Type == buffer.BTDefault {
			node.Content = string(bp.Buf.Bytes())
		}
		return node, []*BufPane{bp}
	}

	var kids []*workspace.Node
	var weights []float64
	var leaves []*BufPane
	for _, c := range n.Children() {
		ec, el := encodeNode(c, paneByID, dir, captureContent)
		if ec == nil {
			continue
		}
		kids = append(kids, ec)
		weights = append(weights, childWeight(n, c))
		leaves = append(leaves, el...)
	}
	if len(kids) == 0 {
		return nil, nil
	}
	if len(kids) == 1 {
		// This split collapsed to a single surviving child: promote
		// it in place of this node, the saved-state equivalent of
		// Node.flatten.
		return kids[0], leaves
	}

	total := 0.0
	for _, w := range weights {
		total += w
	}
	if total <= 0 {
		total = float64(len(weights))
		for i := range weights {
			weights[i] = 1
		}
	}
	for i, k := range kids {
		k.Proportion = weights[i] / total
	}

	kind := "hsplit"
	if n.Kind == views.STHoriz {
		kind = "vsplit"
	}
	return &workspace.Node{Kind: kind, Children: kids}, leaves
}

// encodeTab converts one live Tab into its saved form, or nil if the
// tab held nothing worth saving (e.g. it only ever contained a
// terminal pane). See encodeNode for captureContent.
func encodeTab(t *Tab, dir string, captureContent bool) *workspace.TabState {
	paneByID := make(map[uint64]Pane, len(t.Panes))
	for _, p := range t.Panes {
		paneByID[p.ID()] = p
	}

	layout, leaves := encodeNode(t.Node, paneByID, dir, captureContent)
	if layout == nil {
		return nil
	}

	activePane := 0
	if active, ok := t.Panes[t.active].(*BufPane); ok {
		for i, bp := range leaves {
			if bp == active {
				activePane = i
				break
			}
		}
	}
	return &workspace.TabState{ActivePane: activePane, Layout: layout}
}

// firstLeaf returns the leaf reached by always descending into
// Children[0]. Its buffer is the one that must already be loaded
// into a subtree's anchor pane before replaySubtree walks the rest
// of node (see replaySubtree).
func firstLeaf(node *workspace.Node) *workspace.Node {
	for node.Kind != "leaf" && len(node.Children) > 0 {
		node = node.Children[0]
	}
	return node
}

// openLeafBuffer resolves node's saved path against dir (relative
// paths are workspace-relative, D-54) and opens it with the saved
// cursor position via buffer.Command.StartCursor, the same mechanism
// LoadInput uses for a `+LINE:COL` CLI argument. A path-less leaf
// becomes an unnamed buffer seeded from node.Content, which is empty
// for a dir-backed workspace (encodeNode never sets it there) and the
// saved text for a persistent scratch workspace leaf (D-55).
func openLeafBuffer(node *workspace.Node, dir string) (*buffer.Buffer, error) {
	cmd := buffer.Command{StartCursor: buffer.Loc{X: -1, Y: -1}}
	if node.Cursor != nil {
		cmd.StartCursor = buffer.Loc{X: node.Cursor.X, Y: node.Cursor.Y}
	}

	if node.Path == "" {
		return buffer.NewBufferFromStringWithCommand(node.Content, "", buffer.BTDefault, cmd), nil
	}

	path := node.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	return buffer.NewBufferFromFileWithCommand(path, buffer.BTDefault, cmd)
}

// replaySubtree rebuilds node's split structure starting from anchor,
// a pane that already displays the buffer for node's first leaf (see
// firstLeaf); buildTab establishes this invariant for the whole
// tab's root, and every recursive step below preserves it for each
// child it descends into. It returns the pane holding the last
// (rightmost, or bottommost) leaf of the subtree, which the caller
// uses as the anchor for node's next sibling, if any.
//
// Proportions are not applied here: VSplitIndex/HSplitIndex always
// split the available space evenly. applyProportions does a second,
// top-down pass once the whole shape exists.
func replaySubtree(node *workspace.Node, anchor *BufPane, dir string) (*BufPane, error) {
	if node.Kind == "leaf" {
		return anchor, nil
	}
	if len(node.Children) == 0 {
		return anchor, nil
	}

	last, err := replaySubtree(node.Children[0], anchor, dir)
	if err != nil {
		return nil, err
	}
	for _, child := range node.Children[1:] {
		buf, err := openLeafBuffer(firstLeaf(child), dir)
		if err != nil {
			return nil, err
		}
		var next *BufPane
		if node.Kind == "vsplit" {
			next = last.VSplitIndex(buf, true)
		} else {
			next = last.HSplitIndex(buf, true)
		}
		last, err = replaySubtree(child, next, dir)
		if err != nil {
			return nil, err
		}
	}
	return last, nil
}

// applyProportions re-derives each split's boundary from node's saved
// proportions, scaled against the tab's actual current size (D-54).
// It must run after the whole shape exists (replaySubtree only ever
// produces an even split) and walks top-down so each level resizes
// against its parent's already-corrected size.
//
// Resizing children[i] to its exact target absolute size dumps
// whatever slack remains into children[i+1]; repeating that left to
// right (top to bottom) leaves every child but the last at its exact
// target, and the last one lands on its own target too as long as
// proportions sum to 1 (true for anything this package itself wrote).
func applyProportions(n *views.Node, node *workspace.Node) {
	if node == nil || n.IsLeaf() || len(node.Children) < 2 {
		return
	}
	children := n.Children()
	if len(children) != len(node.Children) {
		// Shape mismatch: should not happen for state this package
		// wrote itself. Leave the default even split alone.
		return
	}

	total := n.W
	if n.Kind == views.STVert {
		total = n.H
	}
	for i := 0; i < len(children)-1; i++ {
		size := int(float64(total)*node.Children[i].Proportion + 0.5)
		if size < 1 {
			size = 1
		} else if size > total-1 {
			size = total - 1
		}
		children[i].ResizeSplit(size)
	}

	for i, c := range children {
		applyProportions(c, node.Children[i])
	}
}

// buildTab constructs one Tab from a saved TabState, sized to
// x,y,width,height (already accounting for the tab bar and info bar,
// same convention as NewTabFromBuffer's other callers).
func buildTab(ts workspace.TabState, dir string, x, y, width, height int) (*Tab, error) {
	if ts.Layout == nil {
		return nil, errors.New("workspace tab has no layout")
	}

	buf0, err := openLeafBuffer(firstLeaf(ts.Layout), dir)
	if err != nil {
		return nil, err
	}
	tab := NewTabFromBuffer(x, y, width, height, buf0)
	pane0 := tab.Panes[0].(*BufPane)

	if _, err := replaySubtree(ts.Layout, pane0, dir); err != nil {
		return nil, err
	}

	applyProportions(tab.Node, ts.Layout)
	tab.Resize()

	activePane := util.Clamp(ts.ActivePane, 0, len(tab.Panes)-1)
	tab.SetActive(activePane)

	return tab, nil
}

// buildTabsFromState converts a decoded State's tabs into live *Tab
// values sized to width x height, resolving each leaf's path against
// dir (workspace-relative when the leaf stored a relative path).
// Shared by loadWorkspaceState (dir-backed) and the persistent
// scratch workspace's own loader (D-55), whose leaves always store
// absolute paths, making dir irrelevant there ("" is passed).
func buildTabsFromState(state *workspace.State, dir string, width, height int) (tabs []*Tab, activeTab int, err error) {
	if len(state.Tabs) == 0 {
		return nil, 0, errors.New("workspace state has no tabs")
	}

	y, h := tabListGeometry(len(state.Tabs), width, height)

	tabs = make([]*Tab, 0, len(state.Tabs))
	for _, ts := range state.Tabs {
		tab, err := buildTab(ts, dir, 0, y, width, h)
		if err != nil {
			return nil, 0, err
		}
		tabs = append(tabs, tab)
	}

	activeTab = util.Clamp(state.ActiveTab, 0, len(tabs)-1)
	return tabs, activeTab, nil
}

// loadWorkspaceState decodes dir's saved state, if any, and replays
// it into a fresh []*Tab sized to width x height. The bool result
// reports whether a saved state existed; when false the caller
// should fall back to a single empty tab (this function does not do
// that itself, since "no saved state for this dir" and "no dir-
// backed workspace at all" are handled identically by callers).
func loadWorkspaceState(configDir, dir string, width, height int) (tabs []*Tab, activeTab int, existed bool, err error) {
	state, existed, err := workspace.Load(configDir, dir)
	if err != nil || !existed {
		return nil, 0, existed, err
	}
	tabs, activeTab, err = buildTabsFromState(state, dir, width, height)
	return tabs, activeTab, true, err
}

// saveWorkspaceState captures the live Tabs into a workspace.State
// and persists it under dir's escaped key. It is called on every
// workspace switch and wired into every quit path (see
// SaveActiveWorkspace) so a dir-backed workspace's layout survives a
// restart.
func saveWorkspaceState(dir string) error {
	state := &workspace.State{
		Version: workspace.CurrentVersion,
		Dir:     dir,
	}
	if wd, err := os.Getwd(); err == nil {
		state.Cwd = wd
	}

	for _, t := range Tabs.List {
		ts := encodeTab(t, dir, false)
		if ts == nil {
			continue
		}
		state.Tabs = append(state.Tabs, *ts)
	}
	if len(state.Tabs) == 0 {
		// Every tab collapsed to nothing worth saving (e.g. an
		// all-terminal session): keep the state loadable by saving a
		// single empty scratch tab rather than zero tabs.
		state.Tabs = []workspace.TabState{{Layout: &workspace.Node{Kind: "leaf", Cursor: &workspace.CursorLoc{}}}}
	}
	state.ActiveTab = util.Clamp(Tabs.Active(), 0, len(state.Tabs)-1)

	return workspace.Save(config.ConfigDir, state)
}
