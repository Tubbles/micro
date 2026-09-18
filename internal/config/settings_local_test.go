package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetSettings restores the package globals to their startup state.
// Each test that exercises ReadSettings/ReadLocalSettings/InitGlobalSettings
// must call this in advance to keep tests independent.
func resetSettings() {
	parsedSettings = nil
	parsedLocalSettings = nil
	parsedWorkspaceSettings = nil
	parsedWorkspaceLocalSettings = nil
	GlobalSettings = nil
	settingsParseError = false
	localSettingsParseError = false
	workspaceSettingsParseError = false
	workspaceLocalSettingsParseError = false
	VolatileSettings = make(map[string]bool)
}

// configDirWith writes the given JSON contents into a fresh temp dir.
// An empty string means do not write that file.
func configDirWith(t *testing.T, settingsJSON, localJSON string) {
	t.Helper()
	dir := t.TempDir()
	if settingsJSON != "" {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "settings.json"),
			[]byte(settingsJSON), 0o644))
	}
	if localJSON != "" {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "settings.local.json"),
			[]byte(localJSON), 0o644))
	}
	ConfigDir = dir
}

func readSettingsJSON(t *testing.T) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(ConfigDir, "settings.json"))
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(data, &out))
	return out
}

func TestLocalScalarOverridesGlobal(t *testing.T) {
	resetSettings()
	// settings.json says ruler false; settings.local.json overrides to true.
	configDirWith(t,
		`{"ruler": false}`,
		`{"ruler": true}`,
	)
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())
	require.NoError(t, InitGlobalSettings())

	assert.Equal(t, true, GlobalSettings["ruler"], "local should win over global")
}

func TestFileTypeLocalOnly(t *testing.T) {
	resetSettings()
	// No ft:go in settings.json; settings.local.json provides one.
	configDirWith(t,
		`{}`,
		`{"ft:go": {"tabsize": 2}}`,
	)
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())
	require.NoError(t, InitGlobalSettings())

	settings := map[string]any{"tabsize": float64(4)}
	UpdateFileTypeLocals(settings, "go")
	assert.Equal(t, float64(2), settings["tabsize"], "local ft:go should apply when settings.json has none")
}

func TestFileTypeMergePreservesNonCollidingKeys(t *testing.T) {
	resetSettings()
	configDirWith(t,
		`{"ft:go": {"tabsize": 8, "ruler": true}}`,
		`{"ft:go": {"tabsize": 2}}`,
	)
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())
	require.NoError(t, InitGlobalSettings())

	settings := map[string]any{}
	UpdateFileTypeLocals(settings, "go")
	assert.Equal(t, float64(2), settings["tabsize"], "local key wins on collision")
	assert.Equal(t, true, settings["ruler"], "non-colliding settings.json key is preserved")
}

func TestPathGlobLocalOnly(t *testing.T) {
	resetSettings()
	configDirWith(t,
		`{}`,
		`{"glob:*.py": {"tabsize": 2}}`,
	)
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())
	require.NoError(t, InitGlobalSettings())

	settings := map[string]any{"tabsize": float64(4)}
	UpdatePathGlobLocals(settings, "foo.py")
	assert.Equal(t, float64(2), settings["tabsize"])
}

func TestPathGlobMergePreservesNonCollidingKeys(t *testing.T) {
	resetSettings()
	configDirWith(t,
		`{"glob:*.py": {"tabsize": 4, "ruler": true}}`,
		`{"glob:*.py": {"tabsize": 2}}`,
	)
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())
	require.NoError(t, InitGlobalSettings())

	settings := map[string]any{}
	UpdatePathGlobLocals(settings, "foo.py")
	assert.Equal(t, float64(2), settings["tabsize"], "local key wins on collision")
	assert.Equal(t, true, settings["ruler"], "non-colliding settings.json key is preserved")
}

func TestGlobNormalisationInLocal(t *testing.T) {
	resetSettings()
	// Non-prefixed glob in local file should be rewritten to glob:*.py.
	configDirWith(t,
		`{}`,
		`{"*.py": {"tabsize": 2}}`,
	)
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())

	_, raw := parsedLocalSettings["*.py"]
	_, prefixed := parsedLocalSettings["glob:*.py"]
	assert.False(t, raw, "non-prefixed glob should be removed by validate")
	assert.True(t, prefixed, "non-prefixed glob should be re-keyed under glob:")
}

func TestSetRoundTripWritesOnlyTheChangedKey(t *testing.T) {
	resetSettings()
	configDirWith(t, `{}`, `{"ruler": true}`)
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())
	require.NoError(t, InitGlobalSettings())

	// Simulate :set tabsize 8. tabsize default is 4.
	UpdateParsedSetting("tabsize", float64(8))
	require.NoError(t, WriteSettings(filepath.Join(ConfigDir, "settings.json")))

	disk := readSettingsJSON(t)
	assert.Equal(t, map[string]any{"tabsize": float64(8)}, disk,
		"settings.json should contain exactly the :set'd key; no leak from local or defaults")
}

func TestSetToDefaultDeletesFromSettingsJSON(t *testing.T) {
	resetSettings()
	// ruler default is true, so false is the non-default value to start with.
	configDirWith(t, `{"ruler": false}`, ``)
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())
	require.NoError(t, InitGlobalSettings())

	// :set ruler false (non-default, no change).
	UpdateParsedSetting("ruler", false)
	_, present := parsedSettings["ruler"]
	assert.True(t, present, "non-default value should stay in parsedSettings")

	// :set ruler true (the default for ruler).
	defs := DefaultAllSettings()
	UpdateParsedSetting("ruler", defs["ruler"])
	_, present = parsedSettings["ruler"]
	assert.False(t, present, ":set to default should drop the key from parsedSettings")

	require.NoError(t, WriteSettings(filepath.Join(ConfigDir, "settings.json")))
	disk := readSettingsJSON(t)
	_, present = disk["ruler"]
	assert.False(t, present, ":set to default should not appear in settings.json")
}

func TestSetUnrelatedKeyLeavesLocallyShadowedEntryAlone(t *testing.T) {
	resetSettings()
	// settings.json has a legitimate non-default colorscheme.
	// settings.local.json overrides colorscheme to a value that happens to
	// equal the default ("default"). A :set on an unrelated key (tabsize)
	// must not cause the colorscheme entry to leave settings.json.
	configDirWith(t,
		`{"colorscheme": "monokai"}`,
		`{"colorscheme": "default"}`,
	)
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())
	require.NoError(t, InitGlobalSettings())

	UpdateParsedSetting("tabsize", float64(8))
	require.NoError(t, WriteSettings(filepath.Join(ConfigDir, "settings.json")))

	disk := readSettingsJSON(t)
	assert.Equal(t, "monokai", disk["colorscheme"],
		"unrelated :set must not delete a settings.json entry shadowed by local")
	assert.Equal(t, float64(8), disk["tabsize"])
}

func TestHandEditedDefaultValuedEntryPersists(t *testing.T) {
	resetSettings()
	// User hand-wrote `"ruler": true` (which is the default) into
	// settings.json. The pure serializer must not silently drop it.
	configDirWith(t, `{"ruler": true}`, ``)
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())
	require.NoError(t, InitGlobalSettings())

	// :set tabsize 8 (unrelated, triggers WriteSettings).
	UpdateParsedSetting("tabsize", float64(8))
	require.NoError(t, WriteSettings(filepath.Join(ConfigDir, "settings.json")))

	disk := readSettingsJSON(t)
	assert.Equal(t, true, disk["ruler"], "hand-edited default-valued entry must survive WriteSettings")
}

func TestRebuildAfterLocalChangeReflectsNewValue(t *testing.T) {
	resetSettings()
	configDirWith(t, `{"ruler": false}`, `{"ruler": true}`)
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())
	require.NoError(t, InitGlobalSettings())
	assert.Equal(t, true, GlobalSettings["ruler"])

	// Simulate the user hand-editing settings.local.json to flip the override.
	parsedLocalSettings["ruler"] = false
	RebuildGlobalSettings()
	assert.Equal(t, false, GlobalSettings["ruler"], "rebuild should pick up the new local value")
}

func TestRebuildPreservesVolatile(t *testing.T) {
	resetSettings()
	configDirWith(t, `{"ruler": true}`, ``)
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())
	require.NoError(t, InitGlobalSettings())

	// CLI flag set ruler=false; mark volatile.
	GlobalSettings["ruler"] = false
	VolatileSettings["ruler"] = true

	RebuildGlobalSettings()
	assert.Equal(t, false, GlobalSettings["ruler"],
		"volatile value should outrank both parsedSettings and defaults across rebuild")
}

func TestMissingLocalFileIsNotAnError(t *testing.T) {
	resetSettings()
	configDirWith(t, `{"ruler": false}`, ``)
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())
	require.NoError(t, InitGlobalSettings())

	assert.Equal(t, false, GlobalSettings["ruler"])
	assert.Empty(t, parsedLocalSettings,
		"missing settings.local.json should yield empty parsedLocalSettings")
}

func TestParsedSettingsReturnsEffectiveScalarView(t *testing.T) {
	resetSettings()
	// settings.json has tabsize 4 and ft:go (nested map should be excluded).
	// settings.local.json overrides tabsize and adds ruler.
	configDirWith(t,
		`{"tabsize": 4, "ft:go": {"tabsize": 2}}`,
		`{"tabsize": 8, "ruler": false}`,
	)
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())

	view := ParsedSettings()
	assert.Equal(t, float64(8), view["tabsize"], "local overlays parsed")
	assert.Equal(t, false, view["ruler"], "local-only scalar is included")
	_, hasFtGo := view["ft:go"]
	assert.False(t, hasFtGo, "ft:/glob: maps must not appear in scalar view")
}
