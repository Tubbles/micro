package action

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// isHidden reports whether the entry's basename is "hidden" in the
// dotfile sense. Used by both pickers so they share the same rule.
func isHidden(name string) bool {
	return strings.HasPrefix(name, ".") && name != "." && name != ".."
}

// findGitRoot walks upward from start looking for a .git entry (a
// directory in the common case, a file when start lives inside a
// submodule or worktree). Returns the absolute path of the discovered
// root and true on hit, or ("", false) when no .git is found before
// the filesystem root.
func findGitRoot(start string) (string, bool) {
	cur := filepath.Clean(start)
	for {
		if _, err := os.Stat(filepath.Join(cur, ".git")); err == nil {
			return cur, true
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", false
		}
		cur = parent
	}
}

// IgnoreFilter is the single source of truth for "is this entry
// excluded from a picker listing?". It composes the leading-dot
// hidden filter and a gitignore-syntax matcher (which absorbs the
// literal .git directory) so callers don't reimplement either rule.
//
// Construct with NewIgnoreFilter; query with ShouldExclude. The
// matcher is built lazily inside the constructor only when
// ShowIgnored is false: when the user asks to see ignored entries we
// don't need the matcher state at all.
type IgnoreFilter struct {
	Root        string
	ShowHidden  bool
	ShowIgnored bool
	matcher     *ignoreMatcher
}

// NewIgnoreFilter builds a filter rooted at root. root must be an
// absolute, cleaned path; gitignore patterns are interpreted
// relative to it. When showIgnored is true the matcher is skipped
// entirely (cheap path: no .gitignore reads, no regex compiles).
func NewIgnoreFilter(root string, showHidden, showIgnored bool) *IgnoreFilter {
	f := &IgnoreFilter{
		Root:        root,
		ShowHidden:  showHidden,
		ShowIgnored: showIgnored,
	}
	if !showIgnored {
		f.matcher = newIgnoreMatcher(root)
	}
	return f
}

// ShouldExclude reports whether absPath should be omitted from a
// picker listing. The order — hidden first, then literal .git, then
// gitignore patterns — matches the visible toggles' independence:
// flipping Ctrl-h while Ctrl-i is unchanged should change only the
// hidden axis.
//
// absPath is expected to be under f.Root; if it is not, only the
// hidden-name check fires (the gitignore matcher silently ignores
// out-of-tree paths).
func (f *IgnoreFilter) ShouldExclude(absPath string, isDir bool) bool {
	base := filepath.Base(absPath)
	if !f.ShowHidden && isHidden(base) {
		return true
	}
	if f.ShowIgnored {
		return false
	}
	if base == ".git" && isDir {
		return true
	}
	if f.matcher == nil {
		return false
	}
	rel, err := filepath.Rel(f.Root, absPath)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return false
	}
	return f.matcher.Match(filepath.ToSlash(rel), isDir)
}

// ignoreRule is a single compiled gitignore-syntax pattern.
type ignoreRule struct {
	negate  bool
	dirOnly bool
	re      *regexp.Regexp
}

// ignoreLevel collects the rules from one .gitignore file together
// with the directory the file lived in (relative to the matcher
// root, forward-slashes; root .gitignore has dir == "").
//
// above re-roots queries when the rule file lives in an ancestor of
// the matcher root (git info/exclude when the root is a subdir of
// the repo): it is the forward-slash path from that ancestor down to
// the root, prepended to every queried rel so anchored patterns
// resolve against the repo root. dir is "" whenever above is set.
type ignoreLevel struct {
	dir   string
	above string
	rules []ignoreRule
}

// ignoreMatcher matches paths against gitignore-syntax rule sets
// loaded from .gitignore files within a root subtree. Levels are
// populated lazily on first query: a Match for one path only opens
// the .gitignore files on that path's ancestor chain, and a sibling
// subtree the caller never asks about contributes no I/O. Repeated
// queries reuse cached levels via the seen set.
//
// The type is named for the syntax it parses, not for the source
// filename: the same parser handles .gitignore, .ignore, .fdignore,
// .npmignore, .eslintignore, etc., differing only in what the loader
// picks up. The loaders today are .gitignore files in the root
// subtree plus the enclosing repo's .git/info/exclude; future
// loaders can plug in to the same matcher.
type ignoreMatcher struct {
	root   string
	levels []ignoreLevel
	seen   map[string]bool
}

// newIgnoreMatcher returns an empty matcher rooted at root. No
// filesystem access happens here; .gitignore files are opened on
// demand by Match. root must be an absolute, cleaned path; relative
// keys into seen are forward-slash relative paths into the tree, with
// "" denoting root itself.
func newIgnoreMatcher(root string) *ignoreMatcher {
	return &ignoreMatcher{root: root, seen: map[string]bool{}}
}

// ensureLevel opens root/relDir/.gitignore at most once and appends a
// level when the file contributes usable rules. Missing or empty
// .gitignore files are recorded in seen so Match doesn't reopen on
// every call. relDir is forward-slash, "" for the matcher root.
//
// Match calls ensureLevel top-down (root, then each ancestor of the
// queried path), which keeps the invariant matchDirect relies on:
// deeper levels appear later in m.levels than their ancestors, so
// the deepest-first iteration in matchDirect (len(m.levels)-1 down to
// 0) picks the most specific rule set first.
func (m *ignoreMatcher) ensureLevel(relDir string) {
	if m.seen[relDir] {
		return
	}
	m.seen[relDir] = true
	if relDir == "" {
		// info/exclude is appended before the root .gitignore level so
		// the deepest-first walk in matchDirect prefers any .gitignore,
		// matching git's precedence (info/exclude is the weakest
		// in-repo source).
		m.loadGitInfoExclude()
	}
	absDir := m.root
	if relDir != "" {
		absDir = filepath.Join(m.root, filepath.FromSlash(relDir))
	}
	rules, err := loadIgnoreFile(filepath.Join(absDir, ".gitignore"))
	if err != nil || len(rules) == 0 {
		return
	}
	m.levels = append(m.levels, ignoreLevel{dir: relDir, rules: rules})
}

// loadGitInfoExclude appends the enclosing repo's .git/info/exclude
// as the shallowest level. Patterns there are anchored at the repo
// root, which may sit above the matcher root (the recursive file
// picker roots at its start directory); the level's above offset
// re-roots queries in that case. In a submodule or worktree .git is
// a file, the open fails, and the level is simply skipped.
func (m *ignoreMatcher) loadGitInfoExclude() {
	gitRoot, ok := findGitRoot(m.root)
	if !ok {
		return
	}
	rules, err := loadIgnoreFile(filepath.Join(gitRoot, ".git", "info", "exclude"))
	if err != nil || len(rules) == 0 {
		return
	}
	above := ""
	if gitRoot != m.root {
		rel, err := filepath.Rel(gitRoot, m.root)
		if err != nil {
			return
		}
		above = filepath.ToSlash(rel)
	}
	m.levels = append(m.levels, ignoreLevel{above: above, rules: rules})
}

// Match reports whether rel (forward-slash, relative to the matcher
// root) is matched by any rule in the stack, with descendants of an
// ignored directory always reported as ignored regardless of any
// negation rule that would otherwise un-ignore them.
//
// Lazy loading order: ensureLevel("") first, then each ancestor of
// rel from shallowest to deepest. The ancestor's own .gitignore is
// only loaded after confirming the ancestor is not itself ignored,
// which lets a `vendor/` rule short-circuit the load of any nested
// .gitignore inside an excluded subtree.
func (m *ignoreMatcher) Match(rel string, isDir bool) bool {
	m.ensureLevel("")
	parts := strings.Split(rel, "/")
	for i := 1; i < len(parts); i++ {
		ancestor := strings.Join(parts[:i], "/")
		if m.matchDirect(ancestor, true) {
			return true
		}
		m.ensureLevel(ancestor)
	}
	return m.matchDirect(rel, isDir)
}

// matchDirect evaluates rel against the rule stack without the
// ancestor-ignored override. Levels are walked deepest-first so a
// nested .gitignore wins over an ancestor's; within a level rules
// are walked bottom-up so the last matching line wins.
func (m *ignoreMatcher) matchDirect(rel string, isDir bool) bool {
	for li := len(m.levels) - 1; li >= 0; li-- {
		lvl := m.levels[li]
		var relInLevel string
		if lvl.above != "" {
			relInLevel = lvl.above + "/" + rel
		} else if lvl.dir == "" {
			relInLevel = rel
		} else {
			prefix := lvl.dir + "/"
			if !strings.HasPrefix(rel, prefix) {
				continue
			}
			relInLevel = strings.TrimPrefix(rel, prefix)
		}
		for ri := len(lvl.rules) - 1; ri >= 0; ri-- {
			r := lvl.rules[ri]
			if r.dirOnly && !isDir {
				continue
			}
			if r.re.MatchString(relInLevel) {
				return !r.negate
			}
		}
	}
	return false
}

// loadIgnoreFile parses one .gitignore-syntax file into rules.
// Errors opening the file or compiling individual lines are not
// fatal: a bad pattern is dropped silently rather than disabling
// the whole filter.
func loadIgnoreFile(path string) ([]ignoreRule, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var rules []ignoreRule
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), " \t\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		negate := false
		switch {
		case strings.HasPrefix(line, "\\!") || strings.HasPrefix(line, "\\#"):
			line = line[1:]
		case strings.HasPrefix(line, "!"):
			negate = true
			line = line[1:]
		}
		dirOnly := false
		if strings.HasSuffix(line, "/") {
			dirOnly = true
			line = strings.TrimSuffix(line, "/")
		}
		anchored := false
		if strings.HasPrefix(line, "/") {
			anchored = true
			line = strings.TrimPrefix(line, "/")
		} else if strings.Contains(line, "/") {
			anchored = true
		}
		if line == "" {
			continue
		}
		re, err := compileGitignorePattern(line, anchored)
		if err != nil {
			continue
		}
		rules = append(rules, ignoreRule{negate: negate, dirOnly: dirOnly, re: re})
	}
	return rules, sc.Err()
}

// compileGitignorePattern translates the gitignore-syntax body of a
// rule (already stripped of leading !, leading /, trailing /) into
// a fully anchored regexp. anchored selects whether the pattern is
// rooted at the level's directory or matches by basename anywhere
// inside it.
func compileGitignorePattern(pat string, anchored bool) (*regexp.Regexp, error) {
	var sb strings.Builder
	sb.WriteString(`^`)
	if !anchored {
		sb.WriteString(`(?:.*/)?`)
	}
	i := 0
	for i < len(pat) {
		c := pat[i]
		switch {
		case c == '*' && i+1 < len(pat) && pat[i+1] == '*':
			after := i + 2
			leftSlash := i == 0 || pat[i-1] == '/'
			rightSlash := after == len(pat) || pat[after] == '/'
			switch {
			case leftSlash && rightSlash && after < len(pat):
				sb.WriteString(`(?:.*/)?`)
				i = after + 1
				continue
			case leftSlash && rightSlash && after == len(pat):
				sb.WriteString(`.*`)
				i = after
				continue
			default:
				sb.WriteString(`.*`)
				i = after
				continue
			}
		case c == '*':
			sb.WriteString(`[^/]*`)
			i++
		case c == '?':
			sb.WriteString(`[^/]`)
			i++
		case c == '[':
			end := strings.IndexByte(pat[i:], ']')
			if end <= 0 {
				sb.WriteString(regexp.QuoteMeta(string(c)))
				i++
				continue
			}
			sb.WriteString(pat[i : i+end+1])
			i += end + 1
		case c == '\\' && i+1 < len(pat):
			sb.WriteString(regexp.QuoteMeta(string(pat[i+1])))
			i += 2
		case c == '/':
			sb.WriteString(`/`)
			i++
		default:
			sb.WriteString(regexp.QuoteMeta(string(c)))
			i++
		}
	}
	sb.WriteString(`$`)
	return regexp.Compile(sb.String())
}
