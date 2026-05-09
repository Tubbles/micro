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
	// matter if we descended into it; assert it is not loaded.
	mustWrite(t, root, "vendor/.gitignore", "!everything\n")

	m := newIgnoreMatcher(root)
	if got := len(m.levels); got != 1 {
		t.Errorf("expected only root level loaded, got %d levels", got)
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
