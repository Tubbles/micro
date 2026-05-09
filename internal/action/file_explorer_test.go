package action

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// makeFile creates an empty regular file at path.
func makeFile(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
}

func makeDir(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func labels(t *testing.T, dir string, showHidden bool) []string {
	t.Helper()
	// showIgnored=true makes these tests independent of any
	// .gitignore that might exist under TempDir's parent — the
	// matcher walk would otherwise pick those up via findGitRoot.
	filter := NewIgnoreFilter(dir, showHidden, true)
	items, err := listDir(dir, filter)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Label
	}
	return out
}

func TestListDir_DirsFirstThenFilesCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	makeDir(t, filepath.Join(dir, "Beta"))
	makeDir(t, filepath.Join(dir, "alpha"))
	makeFile(t, filepath.Join(dir, "ZED.txt"))
	makeFile(t, filepath.Join(dir, "apple"))

	got := labels(t, dir, false)
	// "../" is always first (TempDir is never root); then sorted dirs
	// (case-insensitive), then sorted files (case-insensitive).
	want := []string{"../", "alpha/", "Beta/", "apple", "ZED.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("listDir order:\n got %v\nwant %v", got, want)
	}
}

func TestListDir_FiltersHidden(t *testing.T) {
	dir := t.TempDir()
	makeFile(t, filepath.Join(dir, "visible"))
	makeFile(t, filepath.Join(dir, ".secret"))
	makeDir(t, filepath.Join(dir, ".hiddendir"))

	got := labels(t, dir, false)
	want := []string{"../", "visible"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filter hidden:\n got %v\nwant %v", got, want)
	}
}

func TestListDir_ShowsHiddenWhenRequested(t *testing.T) {
	dir := t.TempDir()
	makeFile(t, filepath.Join(dir, "visible"))
	makeFile(t, filepath.Join(dir, ".secret"))
	makeDir(t, filepath.Join(dir, ".hiddendir"))

	got := labels(t, dir, true)
	want := []string{"../", ".hiddendir/", ".secret", "visible"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("show hidden:\n got %v\nwant %v", got, want)
	}
}

func TestListDir_RootHasNoDotDot(t *testing.T) {
	// On POSIX the volume root is "/"; constructing this rather than
	// hard-coding "/" so the test runs unchanged on Windows.
	root := string(filepath.Separator)
	if filepath.VolumeName(root) != "" {
		// Windows: the test would need Administrator to enumerate "/"
		// reliably. Skip rather than introduce a flaky case.
		t.Skip("filesystem root listing is unstable on this platform")
	}
	items, err := listDir(root, NewIgnoreFilter(root, false, true))
	if err != nil {
		t.Skipf("cannot read filesystem root: %v", err)
	}
	for _, it := range items {
		if it.Label == "../" {
			t.Fatalf("filesystem root listing should not contain '../'")
		}
	}
}

func TestIsFilesystemRoot(t *testing.T) {
	root := string(filepath.Separator)
	if !isFilesystemRoot(root) {
		t.Fatalf("expected %q to be filesystem root", root)
	}
	if isFilesystemRoot(t.TempDir()) {
		t.Fatalf("temp dir must not be filesystem root")
	}
	// Trailing separator should normalise the same way.
	if !isFilesystemRoot(root + string(filepath.Separator)) {
		t.Fatalf("trailing separator should still be root")
	}
}

func TestListDir_EmptyDirJustHasParentEntry(t *testing.T) {
	dir := t.TempDir()
	got := labels(t, dir, false)
	want := []string{"../"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("empty dir:\n got %v\nwant %v", got, want)
	}
}

func TestListDir_ErrorOnMissingDir(t *testing.T) {
	dir := t.TempDir()
	_, err := listDir(filepath.Join(dir, "does-not-exist"), NewIgnoreFilter(dir, false, true))
	if err == nil {
		t.Fatal("expected error for missing directory")
	}
}

func TestResolveQueryPath(t *testing.T) {
	sep := string(filepath.Separator)
	cur := "/tmp/here"
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{"relative file", "x.txt", "/tmp/here/x.txt"},
		{"relative subdir without slash", "sub", "/tmp/here/sub"},
		{"relative subdir with slash", "sub" + sep, "/tmp/here/sub" + sep},
		{"parent shorthand", ".." + sep, "/tmp" + sep},
		{"absolute path", "/etc/hosts", "/etc/hosts"},
		{"dot-dot in middle", "a/../b", "/tmp/here/b"},
	}
	for _, c := range cases {
		// Skip absolute-path case on Windows — POSIX-shaped fixtures
		// are not portable, and the helper itself is platform-agnostic.
		if filepath.VolumeName(c.query) != "" {
			continue
		}
		if got := resolveQueryPath(cur, c.query); got != c.want {
			t.Errorf("resolveQueryPath(%q, %q) = %q, want %q",
				cur, c.query, got, c.want)
		}
	}
}

func TestIndexOfLabel(t *testing.T) {
	dir := t.TempDir()
	makeFile(t, filepath.Join(dir, "a.txt"))
	makeFile(t, filepath.Join(dir, "b.txt"))
	makeDir(t, filepath.Join(dir, "sub"))

	items, err := listDir(dir, NewIgnoreFilter(dir, false, true))
	if err != nil {
		t.Fatal(err)
	}
	// Listing is "../", "sub/", "a.txt", "b.txt" — dirs first, files
	// next, both case-insensitive sorted.
	cases := []struct {
		label string
		want  int
	}{
		{"../", 0},
		{"sub/", 1},
		{"a.txt", 2},
		{"b.txt", 3},
		{"missing", -1},
		// Files do not have a trailing slash; "a.txt/" must miss.
		{"a.txt/", -1},
	}
	for _, c := range cases {
		if got := indexOfLabel(items, c.label); got != c.want {
			t.Errorf("indexOfLabel(%q) = %d, want %d", c.label, got, c.want)
		}
	}
}
