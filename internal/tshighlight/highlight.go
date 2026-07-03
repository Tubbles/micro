// Package tshighlight is micro's tree-sitter syntax highlighting backend
// (D-45 in work/roadmap-2026-07-decisions.md). It is strictly opt-in via
// the per-buffer "treesitter" setting; the regex engine in pkg/highlight
// remains the permanent default and fallback.
//
// Buffer fills the exact same highlight.LineMatch shape the regex engine
// fills (see internal/buffer's SharedBuffer.Match), so colorschemes work
// unchanged: internal/config/colorscheme.go's GetColor resolves group
// names by longest-dot-prefix regardless of which engine produced them.
package tshighlight

import (
	"sort"
	"unicode/utf8"

	"github.com/micro-editor/micro/v2/pkg/highlight"
	gotreesitter "github.com/odvcencio/gotreesitter"
)

// Buffer holds the tree-sitter parsing and highlighting state for one
// micro buffer. It is created (via NewBuffer) once a grammar is known to
// exist for the buffer's filetype, then reused across edits: call
// Reparse after edits, and LineMatch to read a line's highlight groups.
//
// Buffer is not safe for concurrent use; callers (internal/buffer) drive
// it from the same goroutine that owns the underlying micro Buffer, the
// same assumption pkg/highlight's regex engine already makes.
type Buffer struct {
	highlighter *gotreesitter.Highlighter

	source     []byte
	lineStarts []int // byte offset of the start of each line in source
	ranges     []gotreesitter.HighlightRange

	// lineCache memoizes LineMatch results between calls to Reparse.
	// bufwindow.go's getStyle calls Match(y) once per rendered column on
	// a line, so without this a single render pass would redo the
	// byte->rune conversion for the same line width times; see D-48
	// discussion in Reparse's doc comment.
	lineCache map[int]highlight.LineMatch
}

// NewBuffer creates a tree-sitter highlighting Buffer for the given
// micro filetype, or returns (nil, false) if no grammar is registered
// for it (the caller should fall back to the regex highlighter).
func NewBuffer(filetype string) (*Buffer, bool) {
	entry := languageEntry(filetype)
	if entry == nil {
		return nil, false
	}
	lang := entry.Language()
	if lang == nil {
		return nil, false
	}

	var opts []gotreesitter.HighlighterOption
	if entry.TokenSourceFactory != nil {
		// Some grammars (go, c, cpp, json, lua, ...) parse more
		// accurately with a language-specific lexer bridge instead of
		// the generic DFA lexer (e.g. Go's bridge uses go/scanner).
		// grammars.ParseFile follows the same nil-check to decide
		// between Parser.Parse and Parser.ParseWithTokenSource; the
		// Highlighter equivalent is this option.
		factory := entry.TokenSourceFactory
		opts = append(opts, gotreesitter.WithTokenSourceFactory(func(source []byte) gotreesitter.TokenSource {
			return factory(source, lang)
		}))
	}

	highlighter, err := gotreesitter.NewHighlighter(lang, entry.HighlightQuery, opts...)
	if err != nil {
		return nil, false
	}
	return &Buffer{highlighter: highlighter}, true
}

// Reparse re-parses and re-highlights source (the buffer's full
// content, e.g. LineArray.Bytes()) and caches the resulting captures for
// LineMatch to read.
//
// This always reparses and re-queries the whole document, which is D-48's
// explicitly sanctioned v1 fallback. Feeding gotreesitter's incremental
// edit API (Tree.Edit + HighlightIncremental) would let it reuse
// unaffected subtrees instead, but that needs a precise byte-level
// InputEdit (old/new byte and point ranges); the buffer's edit hook,
// SharedBuffer.MarkModified(startLine, endLine int), only has line
// numbers, and feeding HighlightIncremental a stale, un-Edited old tree
// against genuinely different source risks the parser reusing subtrees
// it shouldn't, i.e. silently wrong highlights, not just a missed
// optimization. So v1 always calls the plain, safe Highlight(source).
//
// What D-48 actually asks to restrict, whole-buffer QUERY EXECUTION
// including injections turning into nvim-treesitter's documented
// per-edit lag trap, is mitigated on the read side instead: LineMatch
// only converts gotreesitter's byte-offset captures into a line's
// rune-indexed highlight.LineMatch when that line is actually asked
// for, and internal/buffer only calls Reparse lazily from Match(),
// not from MarkModified directly, so a buffer that changes off-screen
// (a background tab, a bulk replace-all before the next render) reparses
// at most once, right before its next redraw, not once per edit.
func (b *Buffer) Reparse(source []byte) {
	b.source = source
	b.ranges = b.highlighter.Highlight(source)
	b.lineStarts = computeLineStarts(source)
	if b.lineCache == nil {
		b.lineCache = make(map[int]highlight.LineMatch)
	} else {
		clear(b.lineCache)
	}
}

// LineMatch returns the tree-sitter-derived highlight.LineMatch for line
// y, converting gotreesitter's document-relative byte-offset
// HighlightRanges to micro's per-line rune-column LineMatch. Results are
// cached until the next Reparse.
func (b *Buffer) LineMatch(y int) highlight.LineMatch {
	if m, ok := b.lineCache[y]; ok {
		return m
	}
	m := b.computeLineMatch(y)
	b.lineCache[y] = m
	return m
}

func (b *Buffer) computeLineMatch(y int) highlight.LineMatch {
	match := highlight.LineMatch{0: 0}
	if y < 0 || y >= len(b.lineStarts) {
		return match
	}

	lineStart := b.lineStarts[y]
	var lineEnd int
	if y+1 < len(b.lineStarts) {
		lineEnd = b.lineStarts[y+1]
	} else {
		lineEnd = len(b.source)
	}
	lineData := b.source[lineStart:lineEnd]
	// computeLineStarts only ever splits on '\n', so a Windows-style
	// line ending leaves a trailing '\r' in lineData; strip it so rune
	// columns line up with micro's own line storage, which never
	// includes line-ending bytes (see LineArray.Bytes).
	if n := len(lineData); n > 0 && lineData[n-1] == '\r' {
		lineData = lineData[:n-1]
	}

	// b.ranges is sorted by StartByte and non-overlapping (gotreesitter's
	// Highlight/HighlightIncremental resolve overlapping captures before
	// returning, picking the narrowest one at each position), so a
	// binary search for the first range that could reach this line plus
	// a linear scan until ranges move past it is enough.
	first := sort.Search(len(b.ranges), func(i int) bool {
		return b.ranges[i].EndByte > uint32(lineStart)
	})
	for i := first; i < len(b.ranges); i++ {
		r := b.ranges[i]
		if int(r.StartByte) >= lineEnd {
			break
		}
		startCol := runeColumn(lineData, clampInt(int(r.StartByte)-lineStart, 0, len(lineData)))
		endCol := runeColumn(lineData, clampInt(int(r.EndByte)-lineStart, 0, len(lineData)))
		match[startCol] = captureGroup(r.Capture)
		match[endCol] = 0
	}
	return match
}

// runeColumn converts a byte offset within line into a rune (character)
// column, matching the rune-indexed convention pkg/highlight's regex
// engine uses for LineMatch keys. A capture starting after a multi-byte
// rune must land on that rune's column, not its byte offset, or the
// display layer (which walks LineMatch by rune column, see
// bufwindow.go's getStyle) would color the wrong character.
func runeColumn(line []byte, byteOffset int) int {
	return utf8.RuneCount(line[:byteOffset])
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// computeLineStarts returns the byte offset of the start of each line in
// source, split on '\n' (matching exactly how LineArray.Bytes joins
// lines, whether the buffer uses unix or dos line endings).
func computeLineStarts(source []byte) []int {
	starts := make([]int, 1, 16)
	starts[0] = 0
	for i, c := range source {
		if c == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}
