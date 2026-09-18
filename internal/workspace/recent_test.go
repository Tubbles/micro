package workspace

import "testing"

func equalDirs(a, b []string) bool {
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

// TestRecentTouchOrdersMostRecentFirstAndDedups exercises the MRU
// invariant directly: re-touching an existing entry moves it to the
// front instead of appending a duplicate.
func TestRecentTouchOrdersMostRecentFirstAndDedups(t *testing.T) {
	r := &Recent{}
	r.Touch("/a")
	r.Touch("/b")
	r.Touch("/c")
	r.Touch("/a")

	want := []string{"/a", "/c", "/b"}
	if !equalDirs(r.Dirs, want) {
		t.Fatalf("got %v, want %v", r.Dirs, want)
	}
}

func TestRecentSaveLoadRoundTrip(t *testing.T) {
	configDir := t.TempDir()

	r := &Recent{}
	r.Touch("/project/a")
	r.Touch("/project/b")
	if err := r.Save(configDir); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadRecent(configDir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/project/b", "/project/a"}
	if !equalDirs(loaded.Dirs, want) {
		t.Fatalf("got %v, want %v", loaded.Dirs, want)
	}
}

func TestLoadRecentMissingFileIsEmpty(t *testing.T) {
	configDir := t.TempDir()
	r, err := LoadRecent(configDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Dirs) != 0 {
		t.Fatalf("expected empty MRU, got %v", r.Dirs)
	}
}
