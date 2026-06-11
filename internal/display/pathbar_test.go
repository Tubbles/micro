package display

import (
	"path/filepath"
	"testing"
)

func TestPathBarText(t *testing.T) {
	sep := string(filepath.Separator)
	cwd := filepath.Join(sep, "home", "user", "project")

	tests := []struct {
		name    string
		absPath string
		want    string
	}{
		{
			name:    "file under cwd is relative",
			absPath: filepath.Join(cwd, "internal", "main.go"),
			want:    filepath.Join("internal", "main.go"),
		},
		{
			name:    "file directly in cwd is its basename",
			absPath: filepath.Join(cwd, "main.go"),
			want:    "main.go",
		},
		{
			name:    "file above cwd is absolute",
			absPath: filepath.Join(sep, "etc", "os-release"),
			want:    filepath.Join(sep, "etc", "os-release"),
		},
		{
			name:    "file in parent dir is absolute",
			absPath: filepath.Join(sep, "home", "user", "notes.txt"),
			want:    filepath.Join(sep, "home", "user", "notes.txt"),
		},
		{
			name:    "sibling dir sharing the cwd name prefix is absolute",
			absPath: filepath.Join(sep, "home", "user", "project2", "main.go"),
			want:    filepath.Join(sep, "home", "user", "project2", "main.go"),
		},
		{
			name:    "cwd itself is the .. parent edge case",
			absPath: filepath.Join(sep, "home", "user"),
			want:    filepath.Join(sep, "home", "user"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pathBarText(tt.absPath, cwd); got != tt.want {
				t.Errorf("pathBarText(%q, %q) = %q, want %q", tt.absPath, cwd, got, tt.want)
			}
		})
	}
}
