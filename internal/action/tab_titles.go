package action

import (
	"path/filepath"
	"strings"

	"github.com/micro-editor/micro/v2/internal/config"
)

// computeTabTitles returns the display strings stored in TabList.Names,
// in tab order. For each tab the active pane's title is derived in one
// of three modes:
//
//   - "full":     b.AbsPath verbatim, plus " +" if the buffer is modified.
//   - "basename": filepath.Base(b.AbsPath), plus " +" if modified.
//   - "smart":    shortest unique tail of b.AbsPath such that no two
//                 tabs in smart mode collide, plus " +" if modified.
//
// Buffers with an explicit name (Help, Log, Raw event viewer), the
// "No name" scratch buffer, and any non-BufPane (TermPane, InfoPane,
// RawPane) bypass the path-display logic entirely and fall back to the
// pane's own Name() method, matching the pre-pathdisplay behaviour.
func computeTabTitles(tabs []*Tab) []string {
	titles := make([]string, len(tabs))
	mode, _ := config.GlobalSettings["pathdisplay"].(string)
	if mode == "" {
		mode = "full"
	}

	var smartIdx []int
	var smartPaths []string

	for i, tab := range tabs {
		bp := tab.CurPane()
		if bp == nil {
			titles[i] = tab.Panes[tab.active].Name()
			continue
		}
		b := bp.Buf
		if b.HasName() || b.Path == "" {
			titles[i] = bp.Name()
			continue
		}

		switch mode {
		case "basename":
			t := filepath.Base(b.AbsPath)
			if b.Modified() {
				t += " +"
			}
			titles[i] = t
		case "smart":
			smartIdx = append(smartIdx, i)
			smartPaths = append(smartPaths, b.AbsPath)
		default: // "full" or any unknown value
			t := b.AbsPath
			if b.Modified() {
				t += " +"
			}
			titles[i] = t
		}
	}

	if len(smartIdx) > 0 {
		shortened := disambiguateTitles(smartPaths)
		for k, idx := range smartIdx {
			t := shortened[k]
			if tabs[idx].CurPane().Buf.Modified() {
				t += " +"
			}
			titles[idx] = t
		}
	}

	return titles
}

// disambiguateTitles returns the shortest tail-of-path display string
// for each input path such that no two outputs collide, except when
// inputs are byte-identical (same file opened twice). Same-path
// entries stay grouped: they share a title and the group as a whole
// expands together when it collides with a different path.
//
// Iteration is monotonic: depth[i] only grows. The loop terminates
// because every iteration either expands at least one path (bounded
// by the number of components in the deepest path) or makes no change
// and exits.
func disambiguateTitles(paths []string) []string {
	titles := make([]string, len(paths))
	depth := make([]int, len(paths))
	for i, p := range paths {
		depth[i] = 1
		titles[i] = lastNComponents(p, 1)
	}

	for {
		groups := make(map[string][]int, len(paths))
		for i, t := range titles {
			groups[t] = append(groups[t], i)
		}

		changed := false
		for _, idxs := range groups {
			if len(idxs) <= 1 {
				continue
			}
			// If every member of the colliding group has the same
			// underlying path, no amount of expansion will separate
			// them — leave the group at its current depth.
			samePath := true
			for _, i := range idxs[1:] {
				if paths[i] != paths[idxs[0]] {
					samePath = false
					break
				}
			}
			if samePath {
				continue
			}
			for _, i := range idxs {
				newDepth := depth[i] + 1
				newTitle := lastNComponents(paths[i], newDepth)
				if newTitle == titles[i] {
					// Path has fewer components than newDepth;
					// saturated. Skip without flagging changed so the
					// loop can still terminate.
					continue
				}
				depth[i] = newDepth
				titles[i] = newTitle
				changed = true
			}
		}
		if !changed {
			return titles
		}
	}
}

// lastNComponents returns the last n slash-separated components of
// path, joined by "/". When n exceeds the number of components the
// whole path is returned. Path separators are normalised to "/" so
// Windows paths render consistently.
func lastNComponents(path string, n int) string {
	if path == "" || n <= 0 {
		return path
	}
	p := filepath.ToSlash(path)
	// Trim any trailing slash so a path like "a/b/" still yields "b"
	// at depth 1 rather than the empty string.
	p = strings.TrimRight(p, "/")
	if p == "" {
		return path
	}
	parts := strings.Split(p, "/")
	// Drop the leading empty component on absolute paths ("/a/b" ->
	// ["", "a", "b"]) so depth counts visible components rather than
	// the root.
	if len(parts) > 0 && parts[0] == "" {
		parts = parts[1:]
	}
	if n >= len(parts) {
		return strings.Join(parts, "/")
	}
	return strings.Join(parts[len(parts)-n:], "/")
}
