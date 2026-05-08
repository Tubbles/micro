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
	items, err := listDir(dir, showHidden)
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
	items, err := listDir(root, false)
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
	_, err := listDir(filepath.Join(t.TempDir(), "does-not-exist"), false)
	if err == nil {
		t.Fatal("expected error for missing directory")
	}
}
