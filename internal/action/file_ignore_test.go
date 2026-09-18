package action

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsHidden(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"foo", false},
		{"", false},
		{".", false},
		{"..", false},
		{".git", true},
		{".gitignore", true},
		{"foo.bar", false},
		{".dotfile", true},
	}
	for _, c := range cases {
		if got := isHidden(c.name); got != c.want {
			t.Errorf("isHidden(%q)=%v want %v", c.name, got, c.want)
		}
	}
}

func TestFindGitRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "a", "b", "c"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, ok := findGitRoot(filepath.Join(root, "a", "b", "c"))
	if !ok || got != root {
		t.Errorf("findGitRoot from depth: got=%q ok=%v want=%q", got, ok, root)
	}
	got, ok = findGitRoot(root)
	if !ok || got != root {
		t.Errorf("findGitRoot at root: got=%q ok=%v want=%q", got, ok, root)
	}

	other := t.TempDir()
	if _, ok := findGitRoot(other); ok {
		t.Errorf("findGitRoot on non-git dir: expected miss")
	}
}

func TestCompileGitignorePattern(t *testing.T) {
	cases := []struct {
		pat       string
		anchored  bool
		hits      []string
		misses    []string
	}{
		{
			pat:      "foo",
			anchored: false,
			hits:     []string{"foo", "a/foo", "a/b/foo"},
			misses:   []string{"foobar", "afoo", "foo/bar"},
		},
		{
			pat:      "foo",
			anchored: true,
			hits:     []string{"foo"},
			misses:   []string{"a/foo", "afoo"},
		},
		{
			pat:      "*.log",
			anchored: false,
			hits:     []string{"a.log", "x.log", "a/b.log"},
			misses:   []string{"a.txt", "log", "a/b.log/c"},
		},
		{
			pat:      "**/foo",
			anchored: true,
			hits:     []string{"foo", "a/foo", "a/b/foo"},
			misses:   []string{"foo/bar"},
		},
		{
			pat:      "foo/**",
			anchored: true,
			hits:     []string{"foo/x", "foo/x/y"},
			misses:   []string{"foo", "afoo/x"},
		},
		{
			pat:      "a/**/b",
			anchored: true,
			hits:     []string{"a/b", "a/x/b", "a/x/y/b"},
			misses:   []string{"a/b/c", "x/a/b"},
		},
		{
			pat:      "doc/*.tmp",
			anchored: true,
			hits:     []string{"doc/a.tmp"},
			misses:   []string{"doc/sub/a.tmp", "src/doc/a.tmp"},
		},
		{
			pat:      "[Mm]akefile.gen",
			anchored: false,
			hits:     []string{"Makefile.gen", "makefile.gen", "a/Makefile.gen"},
			misses:   []string{"Xakefile.gen"},
		},
		{
			pat:      "?.txt",
			anchored: false,
			hits:     []string{"a.txt", "x.txt", "sub/a.txt"},
			misses:   []string{"ab.txt", ".txt"},
		},
	}
	for _, c := range cases {
		re, err := compileGitignorePattern(c.pat, c.anchored)
		if err != nil {
			t.Errorf("compile %q anchored=%v: %v", c.pat, c.anchored, err)
			continue
		}
		for _, h := range c.hits {
			if !re.MatchString(h) {
				t.Errorf("pat=%q anchored=%v should match %q (regex=%q)", c.pat, c.anchored, h, re.String())
			}
		}
		for _, m := range c.misses {
			if re.MatchString(m) {
				t.Errorf("pat=%q anchored=%v should NOT match %q (regex=%q)", c.pat, c.anchored, m, re.String())
			}
		}
	}
}

func TestIgnoreMatcherStacking(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, ".gitignore", "*.log\n!keep.log\n")
	mustWrite(t, root, "src/.gitignore", "!override.log\n")
	mustMkdir(t, root, "src")

	m := newIgnoreMatcher(root)
	cases := []struct {
		rel     string
		isDir   bool
		want    bool
		comment string
	}{
		{"a.log", false, true, "root rule matches"},
		{"keep.log", false, false, "root negation un-ignores"},
		{"src/foo.log", false, true, "ancestor rule still ignores in subdir"},
		{"src/override.log", false, false, "leaf negation un-ignores"},
		{"src/keep.log", false, false, "ancestor negation carries down"},
	}
	for _, c := range cases {
		if got := m.Match(c.rel, c.isDir); got != c.want {
			t.Errorf("%s: Match(%q)=%v want %v", c.comment, c.rel, got, c.want)
		}
	}
}

func TestIgnoreMatcherAncestorOverride(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, ".gitignore", "build/\n!build/keep.txt\n")
	mustMkdir(t, root, "build")

	m := newIgnoreMatcher(root)
	if !m.Match("build", true) {
		t.Errorf("build dir should be ignored")
	}
	if !m.Match("build/keep.txt", false) {
		t.Errorf("file inside ignored dir must stay ignored even with explicit !-rule (git semantics)")
	}
}

func TestIgnoreFilterRespectsToggles(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, ".gitignore", "ignored.txt\n")
	mustMkdir(t, root, ".git")

	hidden := filepath.Join(root, ".hidden")
	regular := filepath.Join(root, "regular.txt")
	ignored := filepath.Join(root, "ignored.txt")
	gitDir := filepath.Join(root, ".git")
	mustTouch(t, hidden)
	mustTouch(t, regular)
	mustTouch(t, ignored)

	all := []struct {
		path  string
		isDir bool
	}{
		{hidden, false},
		{regular, false},
		{ignored, false},
		{gitDir, true},
	}

	check := func(label string, f *IgnoreFilter, want map[string]bool) {
		t.Helper()
		for _, e := range all {
			got := f.ShouldExclude(e.path, e.isDir)
			if got != want[filepath.Base(e.path)] {
				t.Errorf("%s: ShouldExclude(%s)=%v want %v", label, filepath.Base(e.path), got, want[filepath.Base(e.path)])
			}
		}
	}

	check("hide hidden, hide ignored", NewIgnoreFilter(root, false, false), map[string]bool{
		".hidden":     true,
		"regular.txt": false,
		"ignored.txt": true,
		".git":        true,
	})
	check("show hidden, hide ignored", NewIgnoreFilter(root, true, false), map[string]bool{
		".hidden":     false,
		"regular.txt": false,
		"ignored.txt": true,
		".git":        true,
	})
	check("hide hidden, show ignored", NewIgnoreFilter(root, false, true), map[string]bool{
		".hidden":     true,
		"regular.txt": false,
		"ignored.txt": false,
		".git":        true,
	})
	check("show all", NewIgnoreFilter(root, true, true), map[string]bool{
		".hidden":     false,
		"regular.txt": false,
		"ignored.txt": false,
		".git":        false,
	})
}

func TestIgnoreMatcherSkipsAlreadyIgnoredSubtreeDuringLoad(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, ".gitignore", "vendor/\n")
	// A nested .gitignore inside the ignored vendor dir would only
	// matter if we descended into it; assert it is not loaded after
	// a query inside the ignored subtree. The lazy matcher loads
	// root/.gitignore for the query, sees vendor is excluded, and
	// short-circuits before touching vendor/.gitignore.
	mustWrite(t, root, "vendor/.gitignore", "!everything\n")

	m := newIgnoreMatcher(root)
	if got := m.Match("vendor/anything", false); !got {
		t.Fatalf("vendor/anything should be ignored by root rule")
	}
	if got := len(m.levels); got != 1 {
		t.Errorf("expected only root level loaded after query inside ignored subtree, got %d levels", got)
	}
	if m.seen["vendor"] {
		t.Errorf("vendor's own .gitignore must not be probed when its parent rule already excludes the subtree")
	}
}

func TestNewIgnoreMatcherDoesNoIO(t *testing.T) {
	// Pointing the matcher at a nonexistent path must not error out:
	// construction is pure, and lazy ensureLevel just records "no
	// rules" when the .gitignore can't be opened. A subsequent Match
	// with no rules loaded falls through to "not ignored".
	m := newIgnoreMatcher(filepath.Join(t.TempDir(), "does-not-exist"))
	if got := len(m.levels); got != 0 {
		t.Errorf("expected zero levels before any Match call, got %d", got)
	}
	if got := m.Match("foo", false); got {
		t.Errorf("expected Match=false for empty matcher, got true")
	}
}

func TestIgnoreMatcherLazyIdempotent(t *testing.T) {
	// Repeated Match calls under the same ancestor chain reuse the
	// loaded .gitignore: levels stay at 2 (root + src) regardless of
	// how many times we query different files under src.
	root := t.TempDir()
	mustWrite(t, root, ".gitignore", "*.log\n")
	mustWrite(t, root, "src/.gitignore", "*.tmp\n")
	mustMkdir(t, root, "src")

	m := newIgnoreMatcher(root)
	m.Match("src/a.tmp", false)
	if got := len(m.levels); got != 2 {
		t.Fatalf("expected root + src levels after first query, got %d", got)
	}
	m.Match("src/b.tmp", false)
	m.Match("src/c.go", false)
	m.Match("other.log", false)
	if got := len(m.levels); got != 2 {
		t.Errorf("expected level count to stay at 2 after repeated queries, got %d", got)
	}
}

// TestIgnoreMatcherCrossCheckGit runs our matcher and `git
// check-ignore -q` against the same fixture tree and asserts they
// agree on every probe path. This is the only way to catch
// gitignore-spec corner cases (negation anchoring, ** semantics,
// dirOnly trailing slash) without a battle-tested dep.
func TestIgnoreMatcherCrossCheckGit(t *testing.T) {
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not available")
	}

	root := t.TempDir()
	if err := exec.Command(gitBin, "-C", root, "init", "-q").Run(); err != nil {
		t.Skipf("git init failed: %v", err)
	}

	mustWrite(t, root, ".gitignore", strings.Join([]string{
		"*.log",
		"build/",
		"/topfile",
		"!important.log",
		"**/temp",
		"doc/*.tmp",
		"src/**/generated",
		"[Mm]akefile.gen",
	}, "\n")+"\n")
	mustWrite(t, root, "src/.gitignore", strings.Join([]string{
		"*.tmp",
		"!keep.tmp",
	}, "\n")+"\n")

	mustMkdir(t, root, "build")
	mustMkdir(t, root, "src/sub")
	mustMkdir(t, root, "doc")
	mustMkdir(t, root, "nested")
	for _, rel := range []string{
		"debug.log",
		"important.log",
		"build/file",
		"src/main.go",
		"src/main.tmp",
		"src/keep.tmp",
		"src/sub/temp",
		"src/sub/generated",
		"doc/foo.tmp",
		"doc/foo.txt",
		"topfile",
		"nested/topfile",
		"Makefile.gen",
		"temp",
	} {
		mustTouch(t, filepath.Join(root, filepath.FromSlash(rel)))
	}

	m := newIgnoreMatcher(root)

	cases := []struct {
		rel   string
		isDir bool
	}{
		{"debug.log", false},
		{"important.log", false},
		{"build", true},
		{"build/file", false},
		{"topfile", false},
		{"nested/topfile", false},
		{"src/main.go", false},
		{"src/main.tmp", false},
		{"src/keep.tmp", false},
		{"src/sub/temp", false},
		{"src/sub/generated", false},
		{"doc/foo.tmp", false},
		{"doc/foo.txt", false},
		{"Makefile.gen", false},
		{"temp", false},
	}

	for _, c := range cases {
		want := gitCheckIgnore(t, gitBin, root, c.rel)
		got := m.Match(c.rel, c.isDir)
		if got != want {
			t.Errorf("Match(%q, isDir=%v)=%v, git says %v", c.rel, c.isDir, got, want)
		}
	}
}

func TestIgnoreMatcherGitInfoExclude(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, ".git/info/exclude", "*.secret\n/topbuild\n")
	mustMkdir(t, root, "sub")

	m := newIgnoreMatcher(root)
	cases := []struct {
		rel     string
		want    bool
		comment string
	}{
		{"a.secret", true, "exclude basename rule matches at root"},
		{"sub/b.secret", true, "exclude basename rule matches in subdir"},
		{"topbuild", true, "exclude anchored rule matches at root"},
		{"sub/topbuild", false, "anchored rule must not match below root"},
		{"a.txt", false, "unrelated file stays visible"},
	}
	for _, c := range cases {
		if got := m.Match(c.rel, false); got != c.want {
			t.Errorf("%s: Match(%q)=%v want %v", c.comment, c.rel, got, c.want)
		}
	}
}

func TestIgnoreMatcherGitInfoExcludeRootedBelowGitRoot(t *testing.T) {
	// The recursive file picker roots the matcher at its start
	// directory, which can be a subdir of the repo. Anchored exclude
	// patterns must still resolve against the repo root.
	gitRoot := t.TempDir()
	mustWrite(t, gitRoot, ".git/info/exclude", "/sub/topbuild\n*.secret\n")
	mustMkdir(t, gitRoot, "sub/inner")

	m := newIgnoreMatcher(filepath.Join(gitRoot, "sub"))
	cases := []struct {
		rel     string
		want    bool
		comment string
	}{
		{"topbuild", true, "repo-root-anchored rule matches via above offset"},
		{"inner/topbuild", false, "anchored rule must not match deeper"},
		{"a.secret", true, "basename rule matches below the offset"},
		{"a.txt", false, "unrelated file stays visible"},
	}
	for _, c := range cases {
		if got := m.Match(c.rel, false); got != c.want {
			t.Errorf("%s: Match(%q)=%v want %v", c.comment, c.rel, got, c.want)
		}
	}
}

func TestIgnoreMatcherGitignoreWinsOverInfoExclude(t *testing.T) {
	// git precedence: any .gitignore outranks info/exclude, so a
	// negation in the root .gitignore un-ignores an exclude rule.
	root := t.TempDir()
	mustWrite(t, root, ".git/info/exclude", "*.log\n")
	mustWrite(t, root, ".gitignore", "!keep.log\n")

	m := newIgnoreMatcher(root)
	if !m.Match("a.log", false) {
		t.Errorf("a.log should be ignored by info/exclude")
	}
	if m.Match("keep.log", false) {
		t.Errorf("keep.log should be un-ignored by .gitignore negation")
	}
}

func gitCheckIgnore(t *testing.T, gitBin, root, rel string) bool {
	t.Helper()
	cmd := exec.Command(gitBin, "-C", root, "check-ignore", "-q", "--", rel)
	err := cmd.Run()
	if err == nil {
		return true
	}
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		return false
	}
	t.Fatalf("git check-ignore failed for %q: %v", rel, err)
	return false
}

func mustWrite(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustMkdir(t *testing.T, root, rel string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(full, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustTouch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
}
