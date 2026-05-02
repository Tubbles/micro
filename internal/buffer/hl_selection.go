package buffer

// UpdateHLSelection refreshes the hlselection match query from the
// active cursor's state. Cheap: it only updates the query string and
// marks per-line caches dirty if the query changed. The actual
// matching happens lazily inside HLSelectionAt during rendering.
//
// Call this on any cursor move, selection change, buffer modification,
// or option toggle that affects the match (hlselection, ignorecase).
func (b *Buffer) UpdateHLSelection() {
	prevQuery := b.HLSelectionQuery
	prevWhole := b.HLSelectionWholeWord
	b.HLSelection = b.Settings["hlselection"].(bool)

	var query string
	var wholeWord bool

	if b.HLSelection {
		if c := b.GetActiveCursor(); c != nil {
			if q, w, _, ok := c.WordOrSelection(); ok {
				query = q
				wholeWord = w
			}
		}
	}

	b.HLSelectionQuery = query
	b.HLSelectionWholeWord = wholeWord

	if prevQuery != query || prevWhole != wholeWord {
		b.LineArray.invalidateAllHLSelection()
	}
}

// HLSelectionAt reports whether pos is inside an hlselection match.
// Thin wrapper that delegates to LineArray.HLSelectionMatch and its
// per-line lazy cache.
func (b *Buffer) HLSelectionAt(pos Loc) bool {
	return b.LineArray.HLSelectionMatch(b, pos)
}
