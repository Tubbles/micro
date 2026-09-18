package buffer

import (
	"os"
	"testing"

	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/workspace"
)

// TestSetLocalBeatsWorkspaceSettings proves the top of the D-51
// precedence chain: a setlocal on a buffer outranks even the
// workspace's own settings.json. This exercises the real read/rebuild
// path (config.ReadWorkspaceSettings + RebuildGlobalSettings) rather
// than poking package-private maps directly, so it doubles as
// coverage that Buffer.ReloadSettings (used on every workspace
// switch, see internal/action/workspace_open.go's
// applyWorkspaceConfig) picks up the workspace layer via
// config.ParsedSettings.
func TestSetLocalBeatsWorkspaceSettings(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(workspace.IdeMicroDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workspace.SettingsPath(dir), []byte(`{"tabsize": 8}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.ReadWorkspaceSettings(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(config.ClearWorkspaceSettings)
	config.RebuildGlobalSettings()

	b := NewBufferFromString("", "", BTDefault)
	if got := b.Settings["tabsize"]; got != float64(8) {
		t.Fatalf("buffer should pick up workspace tabsize at creation: got %v, want 8", got)
	}

	if err := b.SetOptionNative("tabsize", float64(2)); err != nil {
		t.Fatal(err)
	}
	b.ReloadSettings(true)

	if got := b.Settings["tabsize"]; got != float64(2) {
		t.Errorf("setlocal should beat workspace settings.json: got %v, want 2", got)
	}
}
