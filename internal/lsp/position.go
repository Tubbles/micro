package lsp

import (
	"unicode/utf16"
	"unicode/utf8"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/lsp/protocol"
)

// codeUnits returns the number of code units r occupies in encoding,
// which is one of "utf-8" (bytes), "utf-16" (UTF-16 code units, 2 for
// runes outside the Basic Multilingual Plane such as most emoji), or
// "utf-32" (runes, always 1). Any other value, including "", defaults
// to "utf-16", the LSP default position encoding and the encoding
// every server understands even if it negotiates something else (see
// D-44 in work/roadmap-2026-07-decisions.md): assuming rune count
// equals UTF-16 length is the most common LSP client bug, since it
// only breaks on non-BMP runes.
func codeUnits(r rune, encoding string) uint32 {
	switch encoding {
	case "utf-8":
		return uint32(utf8.RuneLen(r))
	case "utf-32":
		return 1
	default:
		return uint32(utf16.RuneLen(r))
	}
}

// LocToPosition converts a buffer.Loc (0-based line, 0-based rune
// column) into an LSP protocol.Position on the given line's text.
// Position.Character is measured in the code units named by encoding
// (see codeUnits). loc.Y is copied straight to Position.Line: LSP
// lines, like buffer.Loc lines, are 0-based and newline-delimited, so
// no conversion is needed there.
func LocToPosition(line string, loc buffer.Loc, encoding string) protocol.Position {
	col := loc.X
	if col < 0 {
		col = 0
	}

	var character uint32
	runeIndex := 0
	for _, r := range line {
		if runeIndex >= col {
			break
		}
		character += codeUnits(r, encoding)
		runeIndex++
	}

	return protocol.Position{Line: uint32(loc.Y), Character: character}
}

// PositionToLoc converts an LSP protocol.Position back into a
// buffer.Loc on the given line's text, the inverse of LocToPosition
// for the same encoding. A Character that lands inside a multi-unit
// rune (for example the low surrogate of a UTF-16 surrogate pair) is
// rounded up to the rune boundary that follows it.
func PositionToLoc(line string, pos protocol.Position, encoding string) buffer.Loc {
	target := pos.Character

	var consumed uint32
	runeIndex := 0
	for _, r := range line {
		if consumed >= target {
			break
		}
		consumed += codeUnits(r, encoding)
		runeIndex++
	}

	return buffer.Loc{X: runeIndex, Y: int(pos.Line)}
}
