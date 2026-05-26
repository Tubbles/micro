package config

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/Tubbles/tcell/v3"
)

// DefStyle is Micro's default style
var DefStyle tcell.Style = tcell.StyleDefault

// Colorscheme is the current colorscheme
var Colorscheme map[string]tcell.Style

// StyleMask records which fields of a color-link directive were set
// explicitly. Overlay-mode renderers (currently only hlselection) consult
// the mask so they can layer the directive over an existing cell style
// without overwriting the dimensions the user did not specify.
type StyleMask struct {
	Fg         bool
	Bg         bool
	FgPreserve bool
	BgPreserve bool
	Bold       bool
	Italic     bool
	Underline  bool
	Reverse    bool
}

// ColorschemeMasks is the per-group set-mask companion to Colorscheme.
// It is rebuilt by InitColorscheme alongside Colorscheme.
var ColorschemeMasks map[string]StyleMask

// MergeOverlay returns base with the dimensions that mask flags as set
// replaced by the corresponding values from overlay. Attributes are
// additive only; the parser has no syntax to clear bold/italic/underline/
// reverse, so the overlay can only turn them on, never off.
func MergeOverlay(base, overlay tcell.Style, mask StyleMask) tcell.Style {
	s := base
	if mask.Fg {
		s = s.Foreground(overlay.GetForeground())
	}
	if mask.Bg {
		s = s.Background(overlay.GetBackground())
	}
	if mask.Bold {
		s = s.Bold(true)
	}
	if mask.Italic {
		s = s.Italic(true)
	}
	if mask.Underline {
		s = s.Underline(true)
	}
	if mask.Reverse {
		s = s.Reverse(true)
	}
	return s
}

// GetColor takes in a syntax group and returns the colorscheme's style for that group
func GetColor(color string) tcell.Style {
	st := DefStyle
	if color == "" {
		return st
	}
	groups := strings.Split(color, ".")
	if len(groups) > 1 {
		curGroup := ""
		for i, g := range groups {
			if i != 0 {
				curGroup += "."
			}
			curGroup += g
			if style, ok := Colorscheme[curGroup]; ok {
				st = style
			}
		}
	} else if style, ok := Colorscheme[color]; ok {
		st = style
	} else {
		st = StringToStyle(color)
	}

	return st
}

// ColorschemeExists checks if a given colorscheme exists
func ColorschemeExists(colorschemeName string) bool {
	return FindRuntimeFile(RTColorscheme, colorschemeName) != nil
}

// InitColorscheme picks and initializes the colorscheme when micro starts
func InitColorscheme() error {
	Colorscheme = make(map[string]tcell.Style)
	ColorschemeMasks = make(map[string]StyleMask)
	DefStyle = tcell.StyleDefault

	c, m, err := LoadDefaultColorscheme()
	if err == nil {
		Colorscheme = c
		ColorschemeMasks = m
	} else {
		// The colorscheme setting seems broken (maybe because we have not validated
		// it earlier, see comment in verifySetting()). So reset it to the default
		// colorscheme and try again.
		GlobalSettings["colorscheme"] = DefaultGlobalOnlySettings["colorscheme"]
		if c, m, err2 := LoadDefaultColorscheme(); err2 == nil {
			Colorscheme = c
			ColorschemeMasks = m
		}
	}

	return err
}

// LoadDefaultColorscheme loads the default colorscheme from $(ConfigDir)/colorschemes
func LoadDefaultColorscheme() (map[string]tcell.Style, map[string]StyleMask, error) {
	var parsedColorschemes []string
	return LoadColorscheme(GlobalSettings["colorscheme"].(string), &parsedColorschemes)
}

// LoadColorscheme loads the given colorscheme from a directory
func LoadColorscheme(colorschemeName string, parsedColorschemes *[]string) (map[string]tcell.Style, map[string]StyleMask, error) {
	c := make(map[string]tcell.Style)
	m := make(map[string]StyleMask)
	file := FindRuntimeFile(RTColorscheme, colorschemeName)
	if file == nil {
		return c, m, errors.New(colorschemeName + " is not a valid colorscheme")
	}
	if data, err := file.Data(); err != nil {
		return c, m, errors.New("Error loading colorscheme: " + err.Error())
	} else {
		var err error
		c, m, err = ParseColorscheme(file.Name(), string(data), parsedColorschemes)
		if err != nil {
			return c, m, err
		}
	}
	return c, m, nil
}

// ParseColorscheme parses the text definition for a colorscheme and returns the corresponding object
// Colorschemes are made up of color-link statements linking a color group to a list of colors
// For example, color-link keyword (blue,red) makes all keywords have a blue foreground and
// red background. Alongside the styles map, ParseColorscheme returns a
// per-group StyleMask map that records which fields each directive set
// explicitly; it is consumed by overlay-mode renderers that need to layer
// directives on top of an existing cell style.
func ParseColorscheme(name string, text string, parsedColorschemes *[]string) (map[string]tcell.Style, map[string]StyleMask, error) {
	var err error
	colorParser := regexp.MustCompile(`color-link\s+(\S*)\s+"(.*)"`)
	includeParser := regexp.MustCompile(`include\s+"(.*)"`)
	lines := strings.Split(text, "\n")
	c := make(map[string]tcell.Style)
	m := make(map[string]StyleMask)

	if parsedColorschemes != nil {
		*parsedColorschemes = append(*parsedColorschemes, name)
	}

lineLoop:
	for _, line := range lines {
		if strings.TrimSpace(line) == "" ||
			strings.TrimSpace(line)[0] == '#' {
			// Ignore this line
			continue
		}

		matches := includeParser.FindSubmatch([]byte(line))
		if len(matches) == 2 {
			// support includes only in case parsedColorschemes are given
			if parsedColorschemes != nil {
				include := string(matches[1])
				for _, name := range *parsedColorschemes {
					// check for circular includes...
					if name == include {
						// ...and prevent them
						continue lineLoop
					}
				}
				includeScheme, includeMasks, err := LoadColorscheme(include, parsedColorschemes)
				if err != nil {
					return c, m, err
				}
				for k, v := range includeScheme {
					c[k] = v
				}
				for k, v := range includeMasks {
					m[k] = v
				}
			}
			continue
		}

		matches = colorParser.FindSubmatch([]byte(line))
		if len(matches) == 3 {
			link := string(matches[1])
			colors := string(matches[2])

			style, mask := StringToStyleMask(colors)
			c[link] = style
			m[link] = mask

			if link == "default" {
				DefStyle = style
			}
		} else {
			err = errors.New("Color-link statement is not valid: " + line)
		}
	}

	return c, m, err
}

// StringToStyle returns a style from a string. The string format is
// "extra foregroundcolor,backgroundcolor". The 'extra' can be bold,
// reverse, italic or underline. Empty foreground (or "default") and empty
// background fields fall back to DefStyle's foreground and background.
func StringToStyle(str string) tcell.Style {
	style, _ := StringToStyleMask(str)
	return style
}

// StringToStyleMask is StringToStyle plus a StyleMask reporting which
// fields were specified explicitly. Empty or "default" foreground and
// background count as unspecified; recognised attribute keywords count as
// specified. An unrecognised color name falls back to DefStyle the same
// way it does in StringToStyle, but is recorded as unspecified so overlay
// renderers do not paint an unparseable value.
func StringToStyleMask(str string) (tcell.Style, StyleMask) {
	var fg, bg string
	spaceSplit := strings.Split(str, " ")
	split := strings.Split(spaceSplit[len(spaceSplit)-1], ",")
	if len(split) > 1 {
		fg, bg = split[0], split[1]
	} else {
		fg = split[0]
	}
	fg = strings.TrimSpace(fg)
	bg = strings.TrimSpace(bg)

	var mask StyleMask
	var fgColor, bgColor tcell.Color
	if fg == "*" {
		mask.FgPreserve = true
		fgColor = DefStyle.GetForeground()
	} else if fg == "" || fg == "default" {
		fgColor = DefStyle.GetForeground()
	} else {
		c, ok := StringToColor(fg)
		if !ok {
			fgColor = DefStyle.GetForeground()
		} else {
			fgColor = c
			mask.Fg = true
		}
	}
	if bg == "*" {
		mask.BgPreserve = true
		bgColor = DefStyle.GetBackground()
	} else if bg == "" || bg == "default" {
		bgColor = DefStyle.GetBackground()
	} else {
		c, ok := StringToColor(bg)
		if !ok {
			bgColor = DefStyle.GetBackground()
		} else {
			bgColor = c
			mask.Bg = true
		}
	}

	style := DefStyle.Foreground(fgColor).Background(bgColor)
	if strings.Contains(str, "bold") {
		style = style.Bold(true)
		mask.Bold = true
	}
	if strings.Contains(str, "italic") {
		style = style.Italic(true)
		mask.Italic = true
	}
	if strings.Contains(str, "reverse") {
		style = style.Reverse(true)
		mask.Reverse = true
	}
	if strings.Contains(str, "underline") {
		style = style.Underline(true)
		mask.Underline = true
	}
	return style, mask
}

// StringToColor returns a tcell color from a string representation of a color
// We accept either bright... or light... to mean the brighter version of a color
func StringToColor(str string) (tcell.Color, bool) {
	switch str {
	case "black":
		return tcell.ColorBlack, true
	case "red":
		return tcell.ColorMaroon, true
	case "green":
		return tcell.ColorGreen, true
	case "yellow":
		return tcell.ColorOlive, true
	case "blue":
		return tcell.ColorNavy, true
	case "magenta":
		return tcell.ColorPurple, true
	case "cyan":
		return tcell.ColorTeal, true
	case "white":
		return tcell.ColorSilver, true
	case "brightblack", "lightblack":
		return tcell.ColorGray, true
	case "brightred", "lightred":
		return tcell.ColorRed, true
	case "brightgreen", "lightgreen":
		return tcell.ColorLime, true
	case "brightyellow", "lightyellow":
		return tcell.ColorYellow, true
	case "brightblue", "lightblue":
		return tcell.ColorBlue, true
	case "brightmagenta", "lightmagenta":
		return tcell.ColorFuchsia, true
	case "brightcyan", "lightcyan":
		return tcell.ColorAqua, true
	case "brightwhite", "lightwhite":
		return tcell.ColorWhite, true
	case "default":
		return tcell.ColorDefault, true
	default:
		// Check if this is a 256 color
		if num, err := strconv.Atoi(str); err == nil {
			return GetColor256(num), true
		}
		// Check if this is a truecolor hex value
		if len(str) == 7 && str[0] == '#' {
			return tcell.GetColor(str), true
		}
		return tcell.ColorDefault, false
	}
}

// GetColor256 returns the tcell color for a number between 0 and 255
func GetColor256(color int) tcell.Color {
	if color == 0 {
		return tcell.ColorDefault
	}
	return tcell.PaletteColor(color)
}
