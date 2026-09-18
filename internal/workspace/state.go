// Package workspace implements the persisted state for dir-backed
// workspaces (workspaces v1): the JSON schema for a workspace's saved
// tab/split layout, the most-recently-used directory list, and the
// on-disk paths both live under.
//
// Everything in this package is pure data plus file I/O, so it is
// unit-testable without a running screen or editor. The live glue
// that walks the actual Tabs/Panes to build a State, and rebuilds
// Tabs/Panes from a decoded one, lives in internal/action (it needs
// the Tab/Pane types, which would create an import cycle if they
// lived here).
package workspace

// CurrentVersion is the schema version written by this build. Bump
// it if the State/Node shape changes in a way that is not
// backward-compatible.
const CurrentVersion = 1

// CursorLoc is a saved cursor position. It mirrors buffer.Loc without
// importing internal/buffer, keeping this package dependency-free.
type CursorLoc struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Node is one entry in a tab's layout tree. A split node has Kind
// "vsplit" or "hsplit" and two or more Children, each carrying its
// own Proportion (the child's share of the parent along the axis
// that varies for that split direction; proportions of a node's
// children sum to 1). A leaf node has Kind "leaf", a Path (workspace-
// relative when the file is under the workspace dir, absolute
// otherwise, or empty for a scratch buffer) and a Cursor.
//
// Content holds a scratch (unnamed, unsaved) leaf's buffer text
// inline: such a leaf has no file on disk to reload from, so its
// text has to travel in the state itself. It is only ever populated
// by the persistent scratch workspace's save path (scratch.json,
// D-55); a dir-backed workspace's own state never sets it, so an
// unnamed leaf there still restores empty, exactly as before this
// field was added.
//
// Kind follows the user-facing split actions/commands (VSplit
// produces panes side by side, HSplit produces panes stacked), which
// is the transpose of micro's internal views.SplitType naming
// (STHoriz = side by side, STVert = stacked): see
// internal/action/workspace.go for the mapping.
type Node struct {
	Kind       string     `json:"kind"`
	Proportion float64    `json:"proportion,omitempty"`
	Children   []*Node    `json:"children,omitempty"`
	Path       string     `json:"path,omitempty"`
	Content    string     `json:"content,omitempty"`
	Cursor     *CursorLoc `json:"cursor,omitempty"`
}

// TabState is one tab: which of its (post-pruning) leaves is active,
// and its layout tree.
type TabState struct {
	ActivePane int   `json:"activePane"`
	Layout     *Node `json:"layout"`
}

// State is the full persisted state for one dir-backed workspace.
type State struct {
	Version   int        `json:"version"`
	Dir       string     `json:"dir"`
	Cwd       string     `json:"cwd"`
	ActiveTab int        `json:"activeTab"`
	Tabs      []TabState `json:"tabs"`
}
