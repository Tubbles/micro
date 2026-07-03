package action

import (
	"os"
	"testing"

	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/workspace"
)

// writeWorkspaceBindingsLocal writes bindings.local.json into dir's
// .ide/micro/ tree (D-50).
func writeWorkspaceBindingsLocal(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(workspace.IdeMicroDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workspace.BindingsLocalPath(dir), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeWorkspaceSettings writes settings.json into dir's .ide/micro/
// tree (D-50).
func writeWorkspaceSettings(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(workspace.IdeMicroDir(dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workspace.SettingsPath(dir), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// withActiveWorkspace sets currentWorkspaceDir to a fresh temp dir
// for the duration of the test and restores it afterward.
func withActiveWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	old := currentWorkspaceDir
	currentWorkspaceDir = dir
	t.Cleanup(func() { currentWorkspaceDir = old })
	return dir
}

func TestInitBindingsWorkspaceLocalOverridesUserLocal(t *testing.T) {
	withTempConfigDir(t, map[string]string{
		"bindings.json":       `{"Alt-y": "Undo"}`,
		"bindings.local.json": `{"Alt-y": "Redo"}`,
	})
	dir := withActiveWorkspace(t)
	writeWorkspaceBindingsLocal(t, dir, `{"Alt-y": "Save"}`)

	InitBindings()

	if got := config.Bindings["buffer"]["Alt-y"]; got != "Save" {
		t.Errorf("workspace bindings.local.json should win: buffer[Alt-y]=%q, want Save", got)
	}
}

func TestInitBindingsSkipsWorkspaceLayerWithoutActiveWorkspace(t *testing.T) {
	withTempConfigDir(t, map[string]string{
		"bindings.json":       `{"Alt-y": "Undo"}`,
		"bindings.local.json": `{"Alt-y": "Redo"}`,
	})
	old := currentWorkspaceDir
	currentWorkspaceDir = ""
	defer func() { currentWorkspaceDir = old }()

	InitBindings()

	if got := config.Bindings["buffer"]["Alt-y"]; got != "Redo" {
		t.Errorf("no active workspace: buffer[Alt-y]=%q, want Redo (user local.json)", got)
	}
}

// TestApplyWorkspaceConfigSwapsOnSwitch exercises applyWorkspaceConfig
// directly (no live screen/Tabs needed: it only touches
// GlobalSettings, config.Bindings and buffer.OpenBuffers) to prove
// that switching from workspace A to workspace B replaces both the
// settings and bindings layers rather than merging them.
//
// The bindings assertion compares two workspaces that both bind
// Alt-y, rather than A binding it and B leaving it unbound: InitBindings
// only ever adds/overwrites entries in config.Bindings, it never
// clears keys absent from the newly loaded files (true of the
// pre-existing bindings.json/bindings.local.json layers too, not
// something this feature changes), so "unbind on switch" is not a
// real guarantee to test for.
func TestApplyWorkspaceConfigSwapsOnSwitch(t *testing.T) {
	withTempConfigDir(t, map[string]string{
		"bindings.json": `{}`,
	})

	if err := config.ReadSettings(); err != nil {
		t.Fatal(err)
	}
	if err := config.ReadLocalSettings(); err != nil {
		t.Fatal(err)
	}
	if err := config.InitGlobalSettings(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(config.ClearWorkspaceSettings)

	oldDir := currentWorkspaceDir
	t.Cleanup(func() { currentWorkspaceDir = oldDir })

	dirA := t.TempDir()
	writeWorkspaceSettings(t, dirA, `{"tabsize": 8}`)
	writeWorkspaceBindingsLocal(t, dirA, `{"Alt-y": "Save"}`)

	currentWorkspaceDir = dirA
	applyWorkspaceConfig(dirA)

	if got := config.GlobalSettings["tabsize"]; got != float64(8) {
		t.Fatalf("after opening A: GlobalSettings[tabsize]=%v, want 8", got)
	}
	if got := config.Bindings["buffer"]["Alt-y"]; got != "Save" {
		t.Fatalf("after opening A: buffer[Alt-y]=%q, want Save", got)
	}

	dirB := t.TempDir()
	// B leaves tabsize unset (should revert to default) and binds
	// Alt-y to a different action (should replace A's, not merge).
	writeWorkspaceBindingsLocal(t, dirB, `{"Alt-y": "Redo"}`)

	currentWorkspaceDir = dirB
	applyWorkspaceConfig(dirB)

	if got := config.GlobalSettings["tabsize"]; got != config.DefaultAllSettings()["tabsize"] {
		t.Errorf("after switching to B: GlobalSettings[tabsize]=%v, want default (A's value must not leak)", got)
	}
	if got := config.Bindings["buffer"]["Alt-y"]; got != "Redo" {
		t.Errorf("after switching to B: buffer[Alt-y]=%q, want Redo (B's own binding, not A's Save)", got)
	}
}
