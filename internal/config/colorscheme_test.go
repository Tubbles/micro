package config

import (
	"testing"

	"github.com/Tubbles/tcell/v3"
	"github.com/stretchr/testify/assert"
)

func TestSimpleStringToStyle(t *testing.T) {
	s := StringToStyle("lightblue,magenta")

	fg, bg := s.GetForeground(), s.GetBackground()

	assert.Equal(t, tcell.ColorBlue, fg)
	assert.Equal(t, tcell.ColorPurple, bg)
}

func TestAttributeStringToStyle(t *testing.T) {
	s := StringToStyle("bold cyan,brightcyan")

	fg, bg, attr := s.GetForeground(), s.GetBackground(), s.GetAttributes()

	assert.Equal(t, tcell.ColorTeal, fg)
	assert.Equal(t, tcell.ColorAqua, bg)
	assert.NotEqual(t, 0, attr&tcell.AttrBold)
}

func TestMultiAttributesStringToStyle(t *testing.T) {
	s := StringToStyle("bold italic underline cyan,brightcyan")

	fg, bg, attr := s.GetForeground(), s.GetBackground(), s.GetAttributes()

	assert.Equal(t, tcell.ColorTeal, fg)
	assert.Equal(t, tcell.ColorAqua, bg)
	assert.NotEqual(t, 0, attr&tcell.AttrBold)
	assert.NotEqual(t, 0, attr&tcell.AttrItalic)
	assert.True(t, s.HasUnderline())
}

func TestColor256StringToStyle(t *testing.T) {
	s := StringToStyle("128,60")

	fg, bg := s.GetForeground(), s.GetBackground()

	assert.Equal(t, tcell.Color128, fg)
	assert.Equal(t, tcell.Color60, bg)
}

func TestColorHexStringToStyle(t *testing.T) {
	s := StringToStyle("#deadbe,#ef1234")

	fg, bg := s.GetForeground(), s.GetBackground()

	assert.Equal(t, tcell.NewRGBColor(222, 173, 190), fg)
	assert.Equal(t, tcell.NewRGBColor(239, 18, 52), bg)
}

func TestColorschemeParser(t *testing.T) {
	testColorscheme := `color-link default "#F8F8F2,#282828"
color-link comment "#75715E,#282828"
# comment
color-link identifier "#66D9EF,#282828" #comment
color-link constant "#AE81FF,#282828"
color-link constant.string "#E6DB74,#282828"
color-link constant.string.char "#BDE6AD,#282828"`

	c, m, err := ParseColorscheme("testColorscheme", testColorscheme, nil)
	assert.Nil(t, err)

	fg, bg := c["comment"].GetForeground(), c["comment"].GetBackground()
	assert.Equal(t, tcell.NewRGBColor(117, 113, 94), fg)
	assert.Equal(t, tcell.NewRGBColor(40, 40, 40), bg)

	assert.True(t, m["comment"].Fg)
	assert.True(t, m["comment"].Bg)
	assert.False(t, m["comment"].Bold)
}

func TestStringToStyleMaskBgOnly(t *testing.T) {
	_, m := StringToStyleMask(",#88C0D0")
	assert.False(t, m.Fg)
	assert.True(t, m.Bg)
	assert.False(t, m.FgPreserve)
	assert.False(t, m.BgPreserve)
	assert.False(t, m.Bold)
	assert.False(t, m.Italic)
	assert.False(t, m.Underline)
	assert.False(t, m.Reverse)
}

func TestStringToStyleMaskFgOnly(t *testing.T) {
	_, m := StringToStyleMask("#abcdef,")
	assert.True(t, m.Fg)
	assert.False(t, m.Bg)
	assert.False(t, m.FgPreserve)
	assert.False(t, m.BgPreserve)
}

func TestStringToStyleMaskAttrsOnly(t *testing.T) {
	_, m := StringToStyleMask("bold underline")
	assert.False(t, m.Fg)
	assert.False(t, m.Bg)
	assert.False(t, m.FgPreserve)
	assert.False(t, m.BgPreserve)
	assert.True(t, m.Bold)
	assert.True(t, m.Underline)
	assert.False(t, m.Italic)
	assert.False(t, m.Reverse)
}

func TestStringToStyleMaskDefaultIsUnset(t *testing.T) {
	// "default" in either field reads as unspecified, same as the empty
	// form. After the asterisk-preserve change, this means MergeOverlay
	// paints the field with DefStyle, not with the base value.
	_, m := StringToStyleMask("default,#88C0D0")
	assert.False(t, m.Fg)
	assert.True(t, m.Bg)
	assert.False(t, m.FgPreserve)
	assert.False(t, m.BgPreserve)

	_, m = StringToStyleMask("#abcdef,default")
	assert.True(t, m.Fg)
	assert.False(t, m.Bg)
	assert.False(t, m.FgPreserve)
	assert.False(t, m.BgPreserve)
}

func TestStringToStyleMaskFgPreserve(t *testing.T) {
	_, m := StringToStyleMask("*,#88C0D0")
	assert.False(t, m.Fg)
	assert.True(t, m.Bg)
	assert.True(t, m.FgPreserve)
	assert.False(t, m.BgPreserve)
}

func TestStringToStyleMaskBgPreserve(t *testing.T) {
	_, m := StringToStyleMask("#abcdef,*")
	assert.True(t, m.Fg)
	assert.False(t, m.Bg)
	assert.False(t, m.FgPreserve)
	assert.True(t, m.BgPreserve)
}

func TestStringToStyleMaskBothPreserve(t *testing.T) {
	_, m := StringToStyleMask("*,*")
	assert.False(t, m.Fg)
	assert.False(t, m.Bg)
	assert.True(t, m.FgPreserve)
	assert.True(t, m.BgPreserve)
}

func TestStringToStyleMaskFullSpec(t *testing.T) {
	_, m := StringToStyleMask("bold italic reverse underline #abcdef,#123456")
	assert.True(t, m.Fg)
	assert.True(t, m.Bg)
	assert.False(t, m.FgPreserve)
	assert.False(t, m.BgPreserve)
	assert.True(t, m.Bold)
	assert.True(t, m.Italic)
	assert.True(t, m.Underline)
	assert.True(t, m.Reverse)
}

func TestMergeOverlayBgOnly(t *testing.T) {
	base := tcell.StyleDefault.
		Foreground(tcell.NewRGBColor(10, 20, 30)).
		Background(tcell.NewRGBColor(0, 0, 0)).
		Bold(true)
	overlay := tcell.StyleDefault.Background(tcell.NewRGBColor(99, 99, 99))
	mask := StyleMask{Bg: true}

	got := MergeOverlay(base, overlay, mask)

	fg, bg, attr := got.GetForeground(), got.GetBackground(), got.GetAttributes()
	assert.Equal(t, tcell.NewRGBColor(10, 20, 30), fg)
	assert.Equal(t, tcell.NewRGBColor(99, 99, 99), bg)
	assert.NotEqual(t, 0, attr&tcell.AttrBold)
}

func TestMergeOverlayNoOpWhenMaskEmpty(t *testing.T) {
	base := tcell.StyleDefault.
		Foreground(tcell.NewRGBColor(10, 20, 30)).
		Background(tcell.NewRGBColor(40, 50, 60)).
		Italic(true)
	overlay := tcell.StyleDefault.
		Foreground(tcell.NewRGBColor(99, 99, 99)).
		Background(tcell.NewRGBColor(11, 11, 11))

	got := MergeOverlay(base, overlay, StyleMask{})

	fg, bg, attr := got.GetForeground(), got.GetBackground(), got.GetAttributes()
	assert.Equal(t, tcell.NewRGBColor(10, 20, 30), fg)
	assert.Equal(t, tcell.NewRGBColor(40, 50, 60), bg)
	assert.NotEqual(t, 0, attr&tcell.AttrItalic)
}

func TestMergeOverlayAttrsAreAdditive(t *testing.T) {
	base := tcell.StyleDefault.Italic(true)
	overlay := tcell.StyleDefault
	mask := StyleMask{Bold: true, Underline: true}

	got := MergeOverlay(base, overlay, mask)

	attr := got.GetAttributes()
	assert.NotEqual(t, 0, attr&tcell.AttrBold)
	assert.True(t, got.HasUnderline())
	assert.NotEqual(t, 0, attr&tcell.AttrItalic)
}

func TestMergeOverlayFullReplacement(t *testing.T) {
	base := tcell.StyleDefault.
		Foreground(tcell.NewRGBColor(10, 20, 30)).
		Background(tcell.NewRGBColor(40, 50, 60))
	overlay := tcell.StyleDefault.
		Foreground(tcell.NewRGBColor(99, 99, 99)).
		Background(tcell.NewRGBColor(11, 11, 11))
	mask := StyleMask{Fg: true, Bg: true}

	got := MergeOverlay(base, overlay, mask)

	fg, bg := got.GetForeground(), got.GetBackground()
	assert.Equal(t, tcell.NewRGBColor(99, 99, 99), fg)
	assert.Equal(t, tcell.NewRGBColor(11, 11, 11), bg)
}
