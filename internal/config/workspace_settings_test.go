package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/micro-editor/micro/v2/internal/workspace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// workspaceDirWith writes the given JSON contents into a fresh temp
// dir's .ide/micro/ tree (D-50) and returns the dir. An empty string
// means do not write that file.
func workspaceDirWith(t *testing.T, settingsJSON, localJSON string) string {
	t.Helper()
	dir := t.TempDir()
	ideDir := workspace.IdeMicroDir(dir)
	require.NoError(t, os.MkdirAll(ideDir, 0o755))
	if settingsJSON != "" {
		require.NoError(t, os.WriteFile(workspace.SettingsPath(dir),
			[]byte(settingsJSON), 0o644))
	}
	if localJSON != "" {
		require.NoError(t, os.WriteFile(workspace.LocalSettingsPath(dir),
			[]byte(localJSON), 0o644))
	}
	return dir
}

func TestWorkspaceSettingsOverrideLocal(t *testing.T) {
	resetSettings()
	// settings.json says ruler false, settings.local.json overrides to
	// true, and the workspace's settings.json overrides again to
	// false: the workspace should win (D-51).
	configDirWith(t, `{"ruler": false}`, `{"ruler": true}`)
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())

	wsDir := workspaceDirWith(t, `{"ruler": false}`, "")
	require.NoError(t, ReadWorkspaceSettings(wsDir))
	require.NoError(t, ReadWorkspaceLocalSettings(wsDir))
	require.NoError(t, InitGlobalSettings())

	assert.Equal(t, false, GlobalSettings["ruler"], "workspace settings.json should win over the user's settings.local.json")
}

func TestWorkspaceLocalSettingsOverrideWorkspaceSettings(t *testing.T) {
	resetSettings()
	configDirWith(t, `{"ruler": false}`, `{"ruler": true}`)
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())

	wsDir := workspaceDirWith(t, `{"ruler": false}`, `{"ruler": true}`)
	require.NoError(t, ReadWorkspaceSettings(wsDir))
	require.NoError(t, ReadWorkspaceLocalSettings(wsDir))
	require.NoError(t, InitGlobalSettings())

	assert.Equal(t, true, GlobalSettings["ruler"], "workspace settings.local.json should win over workspace settings.json")
}

func TestMissingWorkspaceSettingsFilesAreNotAnError(t *testing.T) {
	resetSettings()
	configDirWith(t, `{"ruler": false}`, "")
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())

	wsDir := t.TempDir() // no .ide/micro tree at all
	require.NoError(t, ReadWorkspaceSettings(wsDir))
	require.NoError(t, ReadWorkspaceLocalSettings(wsDir))
	require.NoError(t, InitGlobalSettings())

	assert.Empty(t, parsedWorkspaceSettings)
	assert.Empty(t, parsedWorkspaceLocalSettings)
	assert.Equal(t, false, GlobalSettings["ruler"])
}

func TestWorkspaceSettingsAreValidated(t *testing.T) {
	resetSettings()
	configDirWith(t, `{}`, "")
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())

	// tabsize must be positive; -1 is invalid and should be corrected
	// back to the default, exactly like an invalid settings.json.
	wsDir := workspaceDirWith(t, `{"tabsize": -1}`, "")
	err := ReadWorkspaceSettings(wsDir)
	assert.Error(t, err, "invalid workspace settings.json value should be reported")
	assert.Equal(t, DefaultAllSettings()["tabsize"], parsedWorkspaceSettings["tabsize"],
		"invalid value should be corrected to the default, like validateParsedSettings does for settings.json")
}

func TestClearWorkspaceSettingsRemovesActiveLayers(t *testing.T) {
	resetSettings()
	configDirWith(t, `{}`, "")
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())

	wsDir := workspaceDirWith(t, `{"ruler": false}`, "")
	require.NoError(t, ReadWorkspaceSettings(wsDir))
	require.NoError(t, InitGlobalSettings())
	assert.Equal(t, false, GlobalSettings["ruler"])

	ClearWorkspaceSettings()
	RebuildGlobalSettings()
	assert.Empty(t, parsedWorkspaceSettings)
	assert.Empty(t, parsedWorkspaceLocalSettings)
	assert.Equal(t, true, GlobalSettings["ruler"], "clearing the workspace layer should fall back to the default")
}

func TestSwitchingWorkspacesReplacesRatherThanMerges(t *testing.T) {
	resetSettings()
	configDirWith(t, `{}`, "")
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())

	dirA := workspaceDirWith(t, `{"ruler": false, "tabsize": 8}`, "")
	require.NoError(t, ReadWorkspaceSettings(dirA))
	require.NoError(t, ReadWorkspaceLocalSettings(dirA))
	require.NoError(t, InitGlobalSettings())
	assert.Equal(t, false, GlobalSettings["ruler"])
	assert.Equal(t, float64(8), GlobalSettings["tabsize"])

	dirB := workspaceDirWith(t, `{"ruler": false}`, "")
	require.NoError(t, ReadWorkspaceSettings(dirB))
	require.NoError(t, ReadWorkspaceLocalSettings(dirB))
	RebuildGlobalSettings()
	assert.Equal(t, false, GlobalSettings["ruler"], "B's own setting should still apply")
	assert.Equal(t, DefaultAllSettings()["tabsize"], GlobalSettings["tabsize"],
		"A's tabsize must not leak into B: switching replaces the workspace layer, it does not merge")
}

func TestUpdateParsedWorkspaceSettingRoundTrip(t *testing.T) {
	resetSettings()
	configDirWith(t, `{}`, "")
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())

	wsDir := t.TempDir()
	require.NoError(t, ReadWorkspaceSettings(wsDir))
	require.NoError(t, ReadWorkspaceLocalSettings(wsDir))

	UpdateParsedWorkspaceSetting("tabsize", float64(8))
	require.NoError(t, WriteWorkspaceSettings(wsDir))

	data, err := os.ReadFile(filepath.Join(wsDir, ".ide", "micro", "settings.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), `"tabsize": 8`)

	// Setting back to the default should drop the key.
	UpdateParsedWorkspaceSetting("tabsize", DefaultAllSettings()["tabsize"])
	require.NoError(t, WriteWorkspaceSettings(wsDir))
	data, err = os.ReadFile(filepath.Join(wsDir, ".ide", "micro", "settings.json"))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "tabsize")
}
