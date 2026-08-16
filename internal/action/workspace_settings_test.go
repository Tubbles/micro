package action

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/workspace"
)

// withGlobalSettings points config.ConfigDir at a fresh temp dir and
// initializes GlobalSettings from it, the way cmd/micro/micro.go does
// at startup. SetWorkspaceOption needs GlobalSettings populated
// because config.GetNativeValue infers an option's type from the
// current global value.
func withGlobalSettings(t *testing.T) {
	t.Helper()
	old := config.ConfigDir
	config.ConfigDir = t.TempDir()
	t.Cleanup(func() { config.ConfigDir = old })

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
}

func TestSetWorkspaceOptionErrorsWithoutActiveWorkspace(t *testing.T) {
	withGlobalSettings(t)
	old := currentWorkspaceDir
	currentWorkspaceDir = ""
	defer func() { currentWorkspaceDir = old }()

	err := SetWorkspaceOption("tabsize", "8")
	if err != errNoActiveWorkspace {
		t.Fatalf("SetWorkspaceOption with no active workspace: got %v, want errNoActiveWorkspace", err)
	}
}

func TestSetWorkspaceOptionWritesFileAndUpdatesGlobalSettings(t *testing.T) {
	withGlobalSettings(t)
	dir := withActiveWorkspace(t)

	if err := SetWorkspaceOption("tabsize", "8"); err != nil {
		t.Fatalf("SetWorkspaceOption: %v", err)
	}

	if got := config.GlobalSettings["tabsize"]; got != float64(8) {
		t.Errorf("GlobalSettings[tabsize] = %v, want 8", got)
	}

	data, err := os.ReadFile(workspace.SettingsPath(dir))
	if err != nil {
		t.Fatalf("reading workspace settings.json: %v", err)
	}
	var onDisk map[string]any
	if err := json.Unmarshal(data, &onDisk); err != nil {
		t.Fatalf("parsing workspace settings.json: %v", err)
	}
	if onDisk["tabsize"] != float64(8) {
		t.Errorf("workspace settings.json tabsize = %v, want 8", onDisk["tabsize"])
	}
}

func TestSetWorkspaceOptionRejectsLocalOnlyOption(t *testing.T) {
	withGlobalSettings(t)
	withActiveWorkspace(t)

	err := SetWorkspaceOption("readonly", "true")
	if err != errWorkspaceOptionIsLocalOnly {
		t.Fatalf("SetWorkspaceOption(readonly): got %v, want errWorkspaceOptionIsLocalOnly", err)
	}
}
