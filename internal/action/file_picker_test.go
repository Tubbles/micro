package action

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

func TestWalkFilesCollectsRelPaths(t *testing.T) {
	root := t.TempDir()
	mustTouch(t, filepath.Join(root, "a.go"))
	mustMkdir(t, root, "src")
	mustTouch(t, filepath.Join(root, "src", "b.go"))
	mustTouch(t, filepath.Join(root, "src", "c.go"))

	filter := NewIgnoreFilter(root, false, true)
	items, truncated, err := walkFiles(root, filter, 100)
	if err != nil {
		t.Fatal(err)
	}
	if truncated {
		t.Errorf("did not expect truncation")
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

func TestWalkFilesRespectsCap(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 10; i++ {
		mustTouch(t, filepath.Join(root, "f"+string(rune('a'+i))+".txt"))
	}
	filter := NewIgnoreFilter(root, false, true)
	items, truncated, err := walkFiles(root, filter, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated {
		t.Errorf("expected truncated=true")
	}
	if len(items) != 3 {
		t.Errorf("cap not enforced: got %d items, want 3", len(items))
	}
}

func TestWalkFilesPrunesIgnoredDirs(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, ".gitignore", "vendor/\n")
	mustMkdir(t, root, "vendor")
	mustTouch(t, filepath.Join(root, "vendor", "huge.bin"))
	mustTouch(t, filepath.Join(root, "main.go"))

	filter := NewIgnoreFilter(root, true, false)
	items, _, err := walkFiles(root, filter, 100)
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
	items, _, err := walkFiles(root, filter, 100)
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
	itemsHidden, _, err := walkFiles(root, noHidden, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range itemsHidden {
		if it.Label == ".hidden" {
			t.Errorf("hidden file should be filtered: %v", itemsHidden)
		}
	}

	withHidden := NewIgnoreFilter(root, true, true)
	itemsAll, _, err := walkFiles(root, withHidden, 100)
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
	hidden, _, err := walkFiles(root, hideIgnored, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range hidden {
		if it.Label == "app.log" {
			t.Errorf("ignored entry should be filtered: %v", hidden)
		}
	}

	showIgnored := NewIgnoreFilter(root, true, true)
	all, _, err := walkFiles(root, showIgnored, 100)
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
	items, _, err := walkFiles(root, filter, 100)
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
