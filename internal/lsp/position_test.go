package lsp

import (
	"testing"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/lsp/protocol"
)

// asciiWorld is plain ASCII: every encoding agrees rune count == code
// unit count.
const asciiWorld = "hello world"

// bmpLine mixes ASCII with two multi-byte Basic Multilingual Plane
// runes: "e" with an acute accent (2 UTF-8 bytes, 1 UTF-16 unit) and
// the CJK character for "world" (3 UTF-8 bytes, 1 UTF-16 unit). BMP
// runes are exactly where a rune-count/UTF-16-length conflation
// happens to work, so this fixture pins down utf-8 without exercising
// the non-BMP case below.
const bmpLine = "café 世界" // "café 世界"

// nonBMPLine embeds an emoji (U+1F600, outside the BMP): 4 UTF-8
// bytes, 2 UTF-16 code units, 1 rune. This is the fixture that catches
// the "UTF-16 length == rune count" bug D-44 calls out.
const nonBMPLine = "ab\U0001F600cd" // "ab😀cd"

func TestLocToPositionASCIIAgreesAcrossEncodings(t *testing.T) {
	loc := buffer.Loc{X: 5, Y: 3} // "hello" | " world"
	for _, encoding := range []string{"utf-8", "utf-16", "utf-32", ""} {
		got := LocToPosition(asciiWorld, loc, encoding)
		want := protocol.Position{Line: 3, Character: 5}
		if got != want {
			t.Errorf("LocToPosition(%q, encoding=%q) = %+v, want %+v", encoding, encoding, got, want)
		}
	}
}

func TestLocToPositionBMPDiffersBetweenUTF8AndUTF16(t *testing.T) {
	// loc.X = 6 is the rune index of the space between "café" and
	// "世界" (c-a-f-é-space is 5 runes, so X=5 is the space itself;
	// X=6 lands right after the space, at the start of "世").
	loc := buffer.Loc{X: 6, Y: 0}

	utf16Got := LocToPosition(bmpLine, loc, "utf-16")
	utf8Got := LocToPosition(bmpLine, loc, "utf-8")
	utf32Got := LocToPosition(bmpLine, loc, "utf-32")

	// Rune-by-rune: c(1) a(1) f(1) é(1) space(1) = 5 runes before "世"
	// at rune index 5. loc.X=6 stops after "世" too, i.e. covers
	// "café 世": utf-8 bytes = 1+1+1+2+1+3 = 9, utf-16 units =
	// 1+1+1+1+1+1 = 6, utf-32/rune count = 6.
	if utf8Got.Character != 9 {
		t.Errorf("utf-8 Character = %d, want 9", utf8Got.Character)
	}
	if utf16Got.Character != 6 {
		t.Errorf("utf-16 Character = %d, want 6", utf16Got.Character)
	}
	if utf32Got.Character != 6 {
		t.Errorf("utf-32 Character = %d, want 6", utf32Got.Character)
	}
	if utf8Got.Character == utf16Got.Character {
		t.Errorf("utf-8 and utf-16 Character both %d, want them to differ (multi-byte BMP rune)", utf8Got.Character)
	}
}

func TestLocToPositionNonBMPDiffersFromRuneCount(t *testing.T) {
	// loc.X = 3 is the rune index of 'c', i.e. after "ab😀".
	loc := buffer.Loc{X: 3, Y: 0}

	utf8Got := LocToPosition(nonBMPLine, loc, "utf-8")
	utf16Got := LocToPosition(nonBMPLine, loc, "utf-16")
	utf16DefaultGot := LocToPosition(nonBMPLine, loc, "")
	utf32Got := LocToPosition(nonBMPLine, loc, "utf-32")

	// a(1) b(1) emoji(4 bytes utf-8 / 2 units utf-16 / 1 rune) = "ab😀"
	if utf8Got.Character != 6 {
		t.Errorf("utf-8 Character = %d, want 6", utf8Got.Character)
	}
	if utf16Got.Character != 4 {
		t.Errorf("utf-16 Character = %d, want 4 (rune count is 3, proving utf-16 != rune count for non-BMP)", utf16Got.Character)
	}
	if utf16DefaultGot != utf16Got {
		t.Errorf(`encoding="" (default) = %+v, want it to match utf-16 = %+v`, utf16DefaultGot, utf16Got)
	}
	if utf32Got.Character != 3 {
		t.Errorf("utf-32 Character = %d, want 3 (equals the rune index)", utf32Got.Character)
	}
}

func TestPositionToLocRoundTrips(t *testing.T) {
	cases := []struct {
		name string
		line string
	}{
		{"ascii", asciiWorld},
		{"bmp", bmpLine},
		{"nonBMP", nonBMPLine},
	}
	encodings := []string{"utf-8", "utf-16", "utf-32", ""}

	for _, c := range cases {
		runeCount := len([]rune(c.line))
		for _, encoding := range encodings {
			for col := 0; col <= runeCount; col++ {
				loc := buffer.Loc{X: col, Y: 7}
				pos := LocToPosition(c.line, loc, encoding)
				if pos.Line != 7 {
					t.Fatalf("%s/%s/col=%d: Position.Line = %d, want 7", c.name, encoding, col, pos.Line)
				}
				gotLoc := PositionToLoc(c.line, pos, encoding)
				if gotLoc != loc {
					t.Errorf("%s/%s/col=%d: round trip via %+v produced %+v, want %+v", c.name, encoding, col, pos, gotLoc, loc)
				}
			}
		}
	}
}

func TestPositionToLocUnknownEncodingDefaultsToUTF16(t *testing.T) {
	pos := protocol.Position{Line: 0, Character: 4}
	got := PositionToLoc(nonBMPLine, pos, "bogus-encoding")
	want := PositionToLoc(nonBMPLine, pos, "utf-16")
	if got != want {
		t.Errorf("PositionToLoc with unknown encoding = %+v, want it to match utf-16 = %+v", got, want)
	}
	if got.X != 3 {
		t.Errorf("PositionToLoc(Character=4) = %+v, want X=3 (after the emoji)", got)
	}
}
