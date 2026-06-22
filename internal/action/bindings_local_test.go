package action

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/micro-editor/micro/v2/internal/config"
)

// withTempConfigDir points config.ConfigDir at a fresh temp dir for
// the duration of the test and writes the given files into it.
func withTempConfigDir(t *testing.T, files map[string]string) {
	t.Helper()
	dir := t.TempDir()
	old := config.ConfigDir
	config.ConfigDir = dir
	t.Cleanup(func() { config.ConfigDir = old })
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInitBindingsLocalOverridesUserFile(t *testing.T) {
	withTempConfigDir(t, map[string]string{
		"bindings.json":       `{"Alt-y": "Undo", "Alt-u": "Save"}`,
		"bindings.local.json": `{"Alt-y": "Redo", "command": {"Alt-p": "HistoryUp"}}`,
	})

	InitBindings()

	if got := config.Bindings["buffer"]["Alt-y"]; got != "Redo" {
		t.Errorf("local override: buffer[Alt-y]=%q, want Redo", got)
	}
	if got := config.Bindings["buffer"]["Alt-u"]; got != "Save" {
		t.Errorf("non-overridden binding: buffer[Alt-u]=%q, want Save", got)
	}
	if got := config.Bindings["command"]["Alt-p"]; got != "HistoryUp" {
		t.Errorf("pane map in local file: command[Alt-p]=%q, want HistoryUp", got)
	}
}

func TestInitBindingsWithoutLocalFile(t *testing.T) {
	withTempConfigDir(t, map[string]string{
		"bindings.json": `{"Alt-y": "Undo"}`,
	})

	InitBindings()

	if got := config.Bindings["buffer"]["Alt-y"]; got != "Undo" {
		t.Errorf("missing local file: buffer[Alt-y]=%q, want Undo", got)
	}
}

func TestInitBindingsLocalOverridesDefault(t *testing.T) {
	// Ctrl-z is Undo in the stock buffer bindings; the local layer
	// must win over defaults even with no bindings.json entry for it.
	withTempConfigDir(t, map[string]string{
		"bindings.json":       `{}`,
		"bindings.local.json": `{"Ctrl-z": "Redo"}`,
	})

	InitBindings()

	if got := config.Bindings["buffer"]["Ctrl-z"]; got != "Redo" {
		t.Errorf("local over default: buffer[Ctrl-z]=%q, want Redo", got)
	}
}
