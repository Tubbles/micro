package action

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"testing"
)

func TestWalkFilesCollectsRelPaths(t *testing.T) {
	root := t.TempDir()
	mustTouch(t, filepath.Join(root, "a.go"))
	mustMkdir(t, root, "src")
	mustTouch(t, filepath.Join(root, "src", "b.go"))
	mustTouch(t, filepath.Join(root, "src", "c.go"))

	filter := NewIgnoreFilter(root, false, true)
	items, err := walkFiles(root, filter)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, it := range items {
		got = append(got, it.Label)
	}
	sort.Strings(got)
	want := []string{"a.go", "src/b.go", "src/c.go"}
	if !equal(got, want) {
		t.Errorf("walkFiles labels: got %v, want %v", got, want)
	}
}

// TestWalkFilesNoCap exercises the unbounded walk. The cap used to
// stop collection at 100 entries; removing it lets the fuzzy filter
// operate across the full tree, which is the only useful behaviour
// for projects with more files than the cap.
func TestWalkFilesNoCap(t *testing.T) {
	root := t.TempDir()
	const n = 250
	for i := 0; i < n; i++ {
		mustTouch(t, filepath.Join(root, "f"+strconv.Itoa(i)+".txt"))
	}
	filter := NewIgnoreFilter(root, false, true)
	items, err := walkFiles(root, filter)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != n {
		t.Errorf("walkFiles collected %d items, want %d", len(items), n)
	}
}

func TestWalkFilesPrunesIgnoredDirs(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, ".gitignore", "vendor/\n")
	mustMkdir(t, root, "vendor")
	mustTouch(t, filepath.Join(root, "vendor", "huge.bin"))
	mustTouch(t, filepath.Join(root, "main.go"))

	filter := NewIgnoreFilter(root, true, false)
	items, err := walkFiles(root, filter)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Label == "vendor/huge.bin" {
			t.Errorf("vendored file should be pruned: %v", items)
		}
	}
}

func TestWalkFilesPrunesGitDir(t *testing.T) {
	root := t.TempDir()
	mustMkdir(t, root, ".git")
	mustTouch(t, filepath.Join(root, ".git", "HEAD"))
	mustTouch(t, filepath.Join(root, "main.go"))

	filter := NewIgnoreFilter(root, true, false)
	items, err := walkFiles(root, filter)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Label == ".git/HEAD" {
			t.Errorf(".git/HEAD should not be in listing: %v", items)
		}
	}
}

func TestWalkFilesHiddenToggle(t *testing.T) {
	root := t.TempDir()
	mustTouch(t, filepath.Join(root, "visible.go"))
	mustTouch(t, filepath.Join(root, ".hidden"))

	noHidden := NewIgnoreFilter(root, false, true)
	itemsHidden, err := walkFiles(root, noHidden)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range itemsHidden {
		if it.Label == ".hidden" {
			t.Errorf("hidden file should be filtered: %v", itemsHidden)
		}
	}

	withHidden := NewIgnoreFilter(root, true, true)
	itemsAll, err := walkFiles(root, withHidden)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range itemsAll {
		if it.Label == ".hidden" {
			found = true
		}
	}
	if !found {
		t.Errorf("hidden file should appear when ShowHidden=true: %v", itemsAll)
	}
}

func TestWalkFilesIgnoredToggle(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, ".gitignore", "*.log\n")
	mustTouch(t, filepath.Join(root, "app.log"))
	mustTouch(t, filepath.Join(root, "main.go"))

	hideIgnored := NewIgnoreFilter(root, true, false)
	hidden, err := walkFiles(root, hideIgnored)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range hidden {
		if it.Label == "app.log" {
			t.Errorf("ignored entry should be filtered: %v", hidden)
		}
	}

	showIgnored := NewIgnoreFilter(root, true, true)
	all, err := walkFiles(root, showIgnored)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range all {
		if it.Label == "app.log" {
			found = true
		}
	}
	if !found {
		t.Errorf("ignored entry should appear when ShowIgnored=true: %v", all)
	}
}

func TestWalkFilesSkipsSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need Administrator on Windows")
	}
	root := t.TempDir()
	target := filepath.Join(root, "real.go")
	mustTouch(t, target)
	if err := os.Symlink(target, filepath.Join(root, "link.go")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	mustMkdir(t, root, "real-dir")
	mustTouch(t, filepath.Join(root, "real-dir", "x.go"))
	if err := os.Symlink(filepath.Join(root, "real-dir"), filepath.Join(root, "link-dir")); err != nil {
		t.Skipf("cannot create dir symlink: %v", err)
	}

	filter := NewIgnoreFilter(root, true, true)
	items, err := walkFiles(root, filter)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Label == "link.go" || it.Label == "link-dir/x.go" {
			t.Errorf("symlink should be skipped: got %v", items)
		}
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
