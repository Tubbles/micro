package tshighlight

import (
	"github.com/odvcencio/gotreesitter/grammars"
)

// filetypeLanguage maps micro's Settings["filetype"] values (as set by
// runtime/syntax/*.yaml's "filetype:" header) to the gotreesitter grammar
// name passed to grammars.DetectLanguageByName.
//
// Most entries are 1:1, but a few of micro's filetype names predate
// tree-sitter's naming and differ from it: C++ is "c++" in
// runtime/syntax/cpp.yaml but "cpp" as a grammar name, and shell scripts
// are filetype "shell" (runtime/syntax/sh.yaml) but grammar "bash".
var filetypeLanguage = map[string]string{
	"go":         "go",
	"c":          "c",
	"c++":        "cpp",
	"rust":       "rust",
	"python":     "python",
	"javascript": "javascript",
	"typescript": "typescript",
	"lua":        "lua",
	"shell":      "bash",
	"json":       "json",
	"yaml":       "yaml",
	"html":       "html",
	"css":        "css",
	"markdown":   "markdown",
	"odin":       "odin",
}

// languageEntry returns the gotreesitter grammar entry for a micro
// filetype, or nil if treesitter has no grammar for it. Callers treat a
// nil result as "fall back to the regex highlighter".
func languageEntry(filetype string) *grammars.LangEntry {
	name, ok := filetypeLanguage[filetype]
	if !ok {
		return nil
	}
	return grammars.DetectLanguageByName(name)
}
