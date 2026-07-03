package buffer

import (
	"strconv"

	"github.com/editorconfig/editorconfig-core-go/v2"
)

// editorConfigWarned tracks which one-time editorconfig messages have
// already been shown, so a directory full of files hitting the same
// unsupported property does not spam the InfoBar once per open. Buffers
// are only ever opened from the main goroutine, so this is left
// unguarded, matching the rest of the package-level settings state in
// internal/config.
var editorConfigWarned = make(map[string]bool)

// warnEditorConfigOnce shows msg in the InfoBar the first time it occurs
// in this run; later calls with the same text are silent.
func warnEditorConfigOnce(msg string) {
	if editorConfigWarned[msg] {
		return
	}
	editorConfigWarned[msg] = true
	if prompt != nil {
		prompt.Message(msg)
	}
}

// resolveEditorConfig looks up the effective editorconfig definition for
// absPath, walking upward from its directory and stopping at a
// `root = true` file, per the editorconfig spec. It performs a fresh
// parse on every call: caching the result risks serving a stale
// definition after the user edits a .editorconfig file mid-session,
// which is worse than the parse cost of a small ini-format file.
func resolveEditorConfig(absPath string) (*editorconfig.Definition, error) {
	if absPath == "" {
		return nil, nil
	}
	return editorconfig.GetDefinitionForFilename(absPath)
}

// editorConfigCharsetToEncoding maps an editorconfig "charset" value to a
// htmlindex encoding name. ok is false when charset is unset (leave
// micro's value untouched) or unrepresentable, in which case warnMsg
// (when non-empty) describes why it was skipped.
func editorConfigCharsetToEncoding(charset string) (name string, warnMsg string) {
	switch charset {
	case "":
		return "", ""
	case editorconfig.CharsetUTF8:
		return "utf-8", ""
	case editorconfig.CharsetUTF16BE:
		return "utf-16be", ""
	case editorconfig.CharsetUTF16LE:
		return "utf-16le", ""
	case editorconfig.CharsetLatin1:
		return "windows-1252", ""
	case editorconfig.CharsetUTF8BOM:
		return "", "editorconfig: charset = utf-8-bom is not supported (micro has no BOM handling), skipping"
	default:
		return "", "editorconfig: unknown charset \"" + charset + "\", skipping"
	}
}

// editorConfigFileFormat maps an editorconfig "end_of_line" value to
// micro's fileformat option. ok is false when end_of_line is unset (leave
// micro's value untouched) or unrepresentable (cr), in which case
// warnMsg (when non-empty) describes why it was skipped.
func editorConfigFileFormat(endOfLine string) (fileformat string, warnMsg string) {
	switch endOfLine {
	case "":
		return "", ""
	case editorconfig.EndOfLineLf:
		return "unix", ""
	case editorconfig.EndOfLineCrLf:
		return "dos", ""
	case editorconfig.EndOfLineCr:
		return "", "editorconfig: end_of_line = cr is not supported, skipping"
	default:
		// Not one of the three values the spec defines; treat like unset
		// rather than warning, since this is not one of the two
		// documented skip cases.
		return "", ""
	}
}

// editorConfigTabSize resolves indent_size/tab_width to a single tabsize,
// per the editorconfig spec: indent_size = tab means use tab_width (if
// specified); otherwise a numeric indent_size is authoritative. The
// library already resolves def.TabWidth to tab_width if explicitly set,
// else to a numeric indent_size, so it directly covers the "indent_size
// unset, tab_width set" and "indent_size = tab" cases.
func editorConfigTabSize(def *editorconfig.Definition) (int, bool) {
	if def.IndentSize != "" && def.IndentSize != editorconfig.IndentStyleTab {
		if n, err := strconv.Atoi(def.IndentSize); err == nil && n > 0 {
			return n, true
		}
	}
	if def.TabWidth > 0 {
		return def.TabWidth, true
	}
	return 0, false
}

// editorConfigTabsToSpaces maps an editorconfig "indent_style" value to
// micro's tabstospaces option. ok is false when indent_style is unset.
func editorConfigTabsToSpaces(indentStyle string) (tabsToSpaces bool, ok bool) {
	switch indentStyle {
	case editorconfig.IndentStyleSpaces:
		return true, true
	case editorconfig.IndentStyleTab:
		return false, true
	default:
		return false, false
	}
}

// editorConfigColorColumn reads the "max_line_length" property (not a
// first-class field on Definition) and maps it to micro's colorcolumn, as
// a visual guide only; micro has no hard-wrap to map the property's
// original meaning onto. ok is false when the property is unset or its
// value doesn't parse as a non-negative integer.
func editorConfigColorColumn(raw map[string]string) (float64, bool) {
	value, present := raw["max_line_length"]
	if !present || value == editorconfig.UnsetValue {
		return 0, false
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, false
	}
	return float64(n), true
}

// mapEditorConfigSettings maps def's properties to micro option
// name/value pairs per D-32, skipping unset and unrepresentable
// properties. warnings holds the messages for skipped-but-unrepresentable
// properties (D-33); unset properties produce no warning.
func mapEditorConfigSettings(def *editorconfig.Definition) (values map[string]any, warnings []string) {
	values = make(map[string]any)

	if tabsToSpaces, ok := editorConfigTabsToSpaces(def.IndentStyle); ok {
		values["tabstospaces"] = tabsToSpaces
	}

	if tabsize, ok := editorConfigTabSize(def); ok {
		values["tabsize"] = float64(tabsize)
	}

	if fileformat, warn := editorConfigFileFormat(def.EndOfLine); fileformat != "" {
		values["fileformat"] = fileformat
	} else if warn != "" {
		warnings = append(warnings, warn)
	}

	if encoding, warn := editorConfigCharsetToEncoding(def.Charset); encoding != "" {
		values["encoding"] = encoding
	} else if warn != "" {
		warnings = append(warnings, warn)
	}

	if def.TrimTrailingWhitespace != nil {
		values["rmtrailingws"] = *def.TrimTrailingWhitespace
	}

	if def.InsertFinalNewline != nil {
		values["eofnewline"] = *def.InsertFinalNewline
	}

	if colorcolumn, ok := editorConfigColorColumn(def.Raw); ok {
		values["colorcolumn"] = colorcolumn
	}

	return values, warnings
}

// applyEditorConfig maps def's properties onto b's settings via
// b.DoSetOptionNative, per D-32.
//
// If overrideLocal is true, a key already present in b.LocalSettings is
// overridden anyway and then cleared from LocalSettings. This is used
// when applying at buffer-open time (D-35): a local flag at that point
// can only have come from fileformat autodetection or an onBufferOpen
// plugin hook (e.g. ftoptions), both of which project-level editorconfig
// settings should outrank.
//
// If overrideLocal is false, a key already local is left untouched and
// LocalSettings is never modified. This is used on ReloadSettings, where
// a local flag may be a real interactive `setlocal` that must keep
// winning until the buffer is reopened.
func applyEditorConfig(b *Buffer, def *editorconfig.Definition, overrideLocal bool) {
	if def == nil {
		return
	}

	values, warnings := mapEditorConfigSettings(def)
	for _, msg := range warnings {
		warnEditorConfigOnce(msg)
	}

	for key, value := range values {
		if !overrideLocal {
			if _, local := b.LocalSettings[key]; local {
				continue
			}
		}
		b.DoSetOptionNative(key, value)
		if overrideLocal {
			delete(b.LocalSettings, key)
		}
	}
}

// applyEditorConfigEncoding sets b.Settings["encoding"] from def's
// charset property, if representable. It must run before NewBuffer builds
// the file-read decoder from b.Settings["encoding"], so it writes the
// setting directly rather than through b.DoSetOptionNative/applyEditorConfig,
// which assume an already-constructed buffer: DoSetOptionNative's
// "encoding" case calls b.setModified(), which hashes b.lines -- not
// populated yet at this point in NewBuffer.
func applyEditorConfigEncoding(b *Buffer, def *editorconfig.Definition) {
	if def == nil {
		return
	}

	name, warn := editorConfigCharsetToEncoding(def.Charset)
	if warn != "" {
		warnEditorConfigOnce(warn)
	}
	if name == "" {
		return
	}

	b.Settings["encoding"] = name
	delete(b.LocalSettings, "encoding")
}
