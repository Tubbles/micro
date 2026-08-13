package action

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSearchTestFile creates a file for index-loading tests and
// returns its path. (Deliberately not named writeTestFile: the
// jump-list branch defines that name in this package, and integration
// merges both branches.)
func writeSearchTestFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "target.txt")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func searchTestFiles() []workspaceSearchFile {
	return []workspaceSearchFile{
		{rel: "a.go", abs: "/w/a.go", lines: []string{"package main", "func Alpha() {}", "// alpha helper"}},
		{rel: "b/b.go", abs: "/w/b/b.go", lines: []string{"package b", "var alphabet = 26"}},
	}
}

func TestSearchWorkspaceSmartCaseInsensitive(t *testing.T) {
	hits := searchWorkspace(searchTestFiles(), "alpha")
	// Lowercase needle matches Alpha, alpha, and alphabet: one hit per
	// matching line.
	if len(hits) != 3 {
		t.Fatalf("got %d hits, want 3: %+v", len(hits), hits)
	}
	first := hits[0]
	if first.fileIndex != 0 || first.line != 1 || first.col != 5 {
		t.Errorf("first hit = %+v, want file 0 line 1 col 5 (Alpha in func Alpha)", first)
	}
}

func TestSearchWorkspaceSmartCaseSensitive(t *testing.T) {
	hits := searchWorkspace(searchTestFiles(), "Alpha")
	if len(hits) != 1 {
		t.Fatalf("got %d hits, want 1 (uppercase needle is exact): %+v", len(hits), hits)
	}
	if hits[0].fileIndex != 0 || hits[0].line != 1 {
		t.Errorf("hit = %+v, want file 0 line 1", hits[0])
	}
}

func TestSearchWorkspaceMinQueryLength(t *testing.T) {
	if hits := searchWorkspace(searchTestFiles(), "a"); hits != nil {
		t.Errorf("single-rune query returned %d hits, want none (below min length)", len(hits))
	}
}

func TestSearchWorkspaceHitCap(t *testing.T) {
	lines := make([]string, workspaceSearchMaxHits+100)
	for i := range lines {
		lines[i] = "needle here"
	}
	files := []workspaceSearchFile{{rel: "big.txt", abs: "/w/big.txt", lines: lines}}
	hits := searchWorkspace(files, "needle")
	if len(hits) != workspaceSearchMaxHits {
		t.Errorf("got %d hits, want capped at %d", len(hits), workspaceSearchMaxHits)
	}
}

func TestWorkspaceSearchItemsFormat(t *testing.T) {
	files := searchTestFiles()
	hits := []workspaceSearchHit{{fileIndex: 1, line: 1, col: 4}}
	items := workspaceSearchItems(files, hits)
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	want := "b/b.go:2:5  var alphabet = 26"
	if items[0].Label != want {
		t.Errorf("label = %q, want %q", items[0].Label, want)
	}
}

func TestLoadSearchLinesSkipsBinary(t *testing.T) {
	path := writeSearchTestFile(t, "text\x00binary")
	if _, ok := loadSearchLines(path); ok {
		t.Error("NUL-containing file was indexed; want skipped")
	}
}

func TestLoadSearchLinesReadsText(t *testing.T) {
	path := writeSearchTestFile(t, "one\ntwo\n")
	lines, ok := loadSearchLines(path)
	if !ok {
		t.Fatal("text file was not indexed")
	}
	if strings.Join(lines, "|") != "one|two|" {
		t.Errorf("lines = %q", lines)
	}
}
