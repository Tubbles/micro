package tshighlight

import (
	"github.com/micro-editor/micro/v2/pkg/highlight"
)

// captureGroups maps gotreesitter highlight-query capture names (as seen
// in HighlightRange.Capture, without the leading '@') to micro's syntax
// highlighting group vocabulary.
//
// This table was built from the actual bundled highlight queries of the
// initial language set (go, c, cpp, rust, python, javascript,
// typescript, lua, bash, json, yaml, html, css, markdown,
// markdown_inline), not guessed: for each of those names,
// grammars.DetectLanguageByName(name).HighlightQuery was scanned for its
// unique "@capture" tokens against the odvcencio/gotreesitter version
// pinned in go.mod.
//
// Where a capture matches an existing regex-engine group 1:1 (e.g.
// "comment"), the table reuses that name so colorschemes need no
// changes. Where a capture is more specific than any existing group,
// the table mints a new dot-suffixed name on top of the closest
// existing base, the same convention micro's own syntax files use (e.g.
// "constant.string.url"), so config.GetColor's longest-dot-prefix
// fallback still colors it even in colorschemes that don't know the new
// name, while leaving a hook for colorschemes that want to.
//
// Captures not listed here still highlight: captureGroup falls back to
// registering the raw capture name verbatim as a new group, so a future
// grammar update or language addition degrades gracefully instead of
// dropping highlighting for unrecognized captures.
var captureGroups = map[string]string{
	// comments
	"comment":               "comment",
	"comment.documentation": "comment.doc",

	// literals
	"boolean":            "constant.bool",
	"constant":           "constant",
	"constant.builtin":   "constant.builtin",
	"number":             "constant.number",
	"string":             "constant.string",
	"string.escape":      "constant.specialChar",
	"escape":             "constant.specialChar",
	"string.special":     "constant.string.special",
	"string.special.key": "constant.string.special",
	"text.uri":           "constant.string.url",
	"text.literal":       "constant.string",

	// keywords / control flow
	"keyword":          "statement",
	"keyword.function": "statement.declaration",
	"keyword.operator": "statement",
	"keyword.return":   "statement",
	"conditional":      "statement.control",
	"repeat":           "statement.control",
	"label":            "statement.control",
	"preproc":          "preproc",
	"charset":          "preproc",
	"import":           "preproc",
	"keyframes":        "preproc",
	"media":            "preproc",
	"supports":         "preproc",

	// identifiers
	"function":           "identifier",
	"function.builtin":   "identifier.builtin",
	"function.method":    "identifier.method",
	"function.macro":     "identifier.macro",
	"function.call":      "identifier",
	"function.special":   "identifier.special",
	"method":             "identifier.method",
	"method.call":        "identifier.method",
	"constructor":        "identifier.class",
	"variable":           "identifier.var",
	"variable.builtin":   "identifier.builtin",
	"variable.parameter": "identifier.var",
	"parameter":          "identifier.var",
	"field":              "statement.member",
	"property":           "statement.member",
	"module":             "statement.declaration",
	"namespace":          "statement.declaration",
	"attribute":          "symbol.attribute",
	"text.reference":     "identifier",

	// types
	"type":         "type",
	"type.builtin": "type.builtin",

	// operators / punctuation
	"operator":              "operator",
	"punctuation.bracket":   "symbol.brackets",
	"punctuation.delimiter": "symbol",
	"punctuation.special":   "symbol",
	"delimiter":             "symbol",

	// tags and no-op captures
	"tag":       "symbol.tag",
	"tag.error": "error",
	"embedded":  "default",
	"none":      "default",

	// markdown-specific
	"text.title":    "markup.heading",
	"markup.strong": "markup.bold",
}

// captureGroup resolves a gotreesitter capture name to a micro
// highlight.Group, registering a new group in highlight.Groups on first
// use if the capture (or its mapped target) is not already a known
// group. See the captureGroups doc comment for the mapping rules.
func captureGroup(capture string) highlight.Group {
	name, ok := captureGroups[capture]
	if !ok {
		name = capture
	}
	return highlight.RegisterGroup(name)
}
