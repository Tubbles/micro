package action

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/widget"
	"github.com/micro-editor/micro/v2/pkg/highlight"
)

// syntaxDefCache caches syntax definitions resolved for preview
// windows and popup code blocks, keyed by "file:<basename>" or
// "ft:<filetype>". Failed lookups cache nil so unknown files don't
// rescan the runtime syntax files on every draw. The cache is never
// invalidated: syntax definitions only change on a micro rebuild or
// a user syntax-file edit plus `> reload`, both rarer than the life
// of this process caring.
var syntaxDefCache = map[string]*highlight.Def{}

// syntaxDefForFile resolves a highlight definition for path by
// filename matching, mirroring the filename subset of
// Buffer.UpdateRules. First-line and signature disambiguation are
// deliberately omitted: previews prefer speed and independence from
// buffer machinery over the detection long tail.
func syntaxDefForFile(path string) *highlight.Def {
	key := "file:" + filepath.Base(path)
	if def, ok := syntaxDefCache[key]; ok {
		return def
	}
	def := resolveSyntaxDef(func(header *highlight.Header) bool {
		return header.MatchFileName(path)
	})
	syntaxDefCache[key] = def
	return def
}

// syntaxDefForFiletype resolves a highlight definition by exact
// filetype name, e.g. a markdown fence tag like "go".
func syntaxDefForFiletype(filetype string) *highlight.Def {
	if filetype == "" {
		return nil
	}
	key := "ft:" + filetype
	if def, ok := syntaxDefCache[key]; ok {
		return def
	}
	def := resolveSyntaxDef(func(header *highlight.Header) bool {
		return header.FileType == filetype
	})
	syntaxDefCache[key] = def
	return def
}

// resolveSyntaxDef scans the user's custom syntax files, then the
// built-in syntax headers, for the first header accepted by match and
// loads its definition. Unlike the buffer path, load errors are
// silently skipped rather than TermMessage'd: a broken syntax file
// must not interrupt the UI from inside a preview draw, and the
// buffer path already reports it when a real file of that type opens.
func resolveSyntaxDef(match func(*highlight.Header) bool) *highlight.Def {
	for _, f := range config.ListRealRuntimeFiles(config.RTSyntax) {
		if f.Name() == "default" {
			continue
		}
		data, err := f.Data()
		if err != nil {
			continue
		}
		header, err := highlight.MakeHeaderYaml(data)
		if err != nil || !match(header) {
			continue
		}
		if def := parsePreviewDef(data, header); def != nil {
			return def
		}
	}

	for _, f := range config.ListRuntimeFiles(config.RTSyntaxHeader) {
		data, err := f.Data()
		if err != nil {
			continue
		}
		header, err := highlight.MakeHeader(data)
		if err != nil || !match(header) {
			continue
		}
		if def := loadBuiltinSyntaxDef(f.Name(), header); def != nil {
			return def
		}
	}
	return nil
}

func loadBuiltinSyntaxDef(name string, header *highlight.Header) *highlight.Def {
	for _, f := range config.ListRuntimeFiles(config.RTSyntax) {
		if f.Name() != name {
			continue
		}
		data, err := f.Data()
		if err != nil {
			return nil
		}
		return parsePreviewDef(data, header)
	}
	return nil
}

func parsePreviewDef(data []byte, header *highlight.Header) *highlight.Def {
	file, err := highlight.ParseFile(data)
	if err != nil {
		return nil
	}
	def, err := highlight.ParseDef(file, header)
	if err != nil {
		return nil
	}
	resolvePreviewIncludes(def)
	return def
}

// resolvePreviewIncludes is Buffer.UpdateRules' include resolution
// (e.g. html embedding javascript/css) minus the TermMessage error
// reporting.
func resolvePreviewIncludes(def *highlight.Def) {
	includes := highlight.GetIncludes(def)
	if len(includes) == 0 {
		return
	}
	var files []*highlight.File
	for _, f := range config.ListRuntimeFiles(config.RTSyntax) {
		data, err := f.Data()
		if err != nil {
			continue
		}
		header, err := highlight.MakeHeaderYaml(data)
		if err != nil {
			continue
		}
		for _, include := range includes {
			if header.FileType == include {
				file, parseErr := highlight.ParseFile(data)
				if parseErr == nil {
					files = append(files, file)
				}
				break
			}
		}
		if len(files) >= len(includes) {
			break
		}
	}
	highlight.ResolveIncludes(def, files)
}

// highlightLines runs def over lines and returns one LineMatch per
// line, or nil when def is nil (callers then render plain).
func highlightLines(lines []string, def *highlight.Def) []highlight.LineMatch {
	if def == nil {
		return nil
	}
	h := highlight.NewHighlighter(def)
	return h.HighlightString(strings.Join(lines, "\n"))
}

// styledSpansForLine converts one line plus its LineMatch into widget
// spans. LineMatch maps a rune column to the group that starts there
// (group 0 = back to default), the same shape the buffer display
// walks in getStyle.
func styledSpansForLine(text string, match highlight.LineMatch) widget.StyledLine {
	if len(match) == 0 {
		return widget.PlainLine(text)
	}

	columns := make([]int, 0, len(match))
	for col := range match {
		columns = append(columns, col)
	}
	sort.Ints(columns)

	runes := []rune(text)
	var out widget.StyledLine
	group := ""
	start := 0
	for _, col := range columns {
		segmentEnd := col
		if segmentEnd > len(runes) {
			segmentEnd = len(runes)
		}
		if segmentEnd > start {
			out = append(out, widget.StyledSpan{Text: string(runes[start:segmentEnd]), Group: group})
			start = segmentEnd
		}
		group = match[col].String()
	}
	if start < len(runes) {
		out = append(out, widget.StyledSpan{Text: string(runes[start:]), Group: group})
	}
	if len(out) == 0 {
		return widget.PlainLine(text)
	}
	return out
}
