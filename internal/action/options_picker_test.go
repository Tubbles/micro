package action

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
)

// resetOptionsTestConfig points config.ConfigDir at a fresh temp dir
// with no settings.json/settings.local.json, and rebuilds
// GlobalSettings/VolatileSettings from a clean slate. Mirrors the
// config package's own resetSettings/configDirWith test harness
// (internal/config/settings_local_test.go); this package can't reuse
// that helper directly since it operates on config's unexported
// parsedSettings/parsedLocalSettings.
func resetOptionsTestConfig(t *testing.T) {
	t.Helper()
	config.ConfigDir = t.TempDir()
	config.VolatileSettings = make(map[string]bool)
	require.NoError(t, config.ReadSettings())
	require.NoError(t, config.ReadLocalSettings())
	require.NoError(t, config.InitGlobalSettings())
}

func valuesAsStrings(t *testing.T, values []any) []string {
	t.Helper()
	out := make([]string, len(values))
	for i, v := range values {
		s, ok := v.(string)
		require.True(t, ok, "value %d is not a string: %#v", i, v)
		out[i] = s
	}
	return out
}

func TestSortedOptionNamesIsSorted(t *testing.T) {
	names := sortedOptionNames()
	require.NotEmpty(t, names)
	assert.True(t, sort.StringsAreSorted(names), "sortedOptionNames must be sorted")
	// A common (per-buffer) option and a global-only option should
	// both be present: DefaultAllSettings merges both maps.
	assert.Contains(t, names, "tabsize")
	assert.Contains(t, names, "colorscheme")
}

func TestOptionsListItemsDefaultLayer(t *testing.T) {
	resetOptionsTestConfig(t)
	defaults := config.DefaultAllSettings()

	items := optionsListItems([]string{"ruler"}, defaults, nil)

	require.Len(t, items, 1)
	assert.Equal(t, "ruler  =  true", items[0].Label)
	assert.Equal(t, "[default] default: true", items[0].Aux)
}

func TestOptionsListItemsGlobalLayer(t *testing.T) {
	resetOptionsTestConfig(t)
	require.NoError(t, SetGlobalOptionNative("ruler", false, true))
	defaults := config.DefaultAllSettings()

	items := optionsListItems([]string{"ruler"}, defaults, nil)

	require.Len(t, items, 1)
	assert.Equal(t, "ruler  =  false", items[0].Label)
	assert.Equal(t, "[global] default: true", items[0].Aux)
}

func TestOptionsListItemsBufferLocalLayer(t *testing.T) {
	resetOptionsTestConfig(t)
	defaults := config.DefaultAllSettings()

	// A minimal buffer.Buffer literal is enough here: optionsListItems
	// only reads the Settings/LocalSettings fields directly (promoted
	// from the embedded *SharedBuffer), it never calls a Buffer
	// method, so this stays hermetic (no screen, no
	// NewBufferFromString).
	buf := &buffer.Buffer{
		SharedBuffer: &buffer.SharedBuffer{
			Settings:      map[string]any{"ruler": false},
			LocalSettings: map[string]bool{"ruler": true},
		},
	}

	items := optionsListItems([]string{"ruler"}, defaults, buf)

	require.Len(t, items, 1)
	assert.Equal(t, "ruler  =  false", items[0].Label)
	assert.Equal(t, "[buffer-local] default: true", items[0].Aux)
}

func TestOptionsValueItemsBool(t *testing.T) {
	defaults := config.DefaultAllSettings()

	items, values, hint := optionsValueItems("ruler", defaults)

	require.Len(t, items, 2)
	assert.Equal(t, "true", items[0].Label)
	assert.Equal(t, "false", items[1].Label)
	assert.Equal(t, []any{true, false}, values)
	assert.Equal(t, optionsChoiceHint, hint)
}

func TestOptionsValueItemsChoice(t *testing.T) {
	defaults := config.DefaultAllSettings()
	wantChoices := config.OptionChoices["fileformat"]
	require.NotEmpty(t, wantChoices, "test assumes fileformat has predefined choices")

	items, values, hint := optionsValueItems("fileformat", defaults)

	require.Len(t, items, len(wantChoices))
	for i, c := range wantChoices {
		assert.Equal(t, c, items[i].Label)
		assert.Equal(t, c, values[i])
	}
	assert.Equal(t, optionsChoiceHint, hint)
}

func TestOptionsValueItemsColorscheme(t *testing.T) {
	defaults := config.DefaultAllSettings()
	_, wantNames := colorschemeComplete("")

	items, values, hint := optionsValueItems("colorscheme", defaults)

	require.Len(t, items, len(wantNames))
	assert.ElementsMatch(t, wantNames, valuesAsStrings(t, values))
	assert.Equal(t, optionsChoiceHint, hint)
}

func TestOptionsValueItemsFiletype(t *testing.T) {
	defaults := config.DefaultAllSettings()
	_, wantNames := filetypeComplete("")
	// filetypeComplete("") always includes "off" regardless of which
	// syntax runtime files happen to be registered in this test
	// binary, so this list is never empty.
	require.NotEmpty(t, wantNames)

	items, values, hint := optionsValueItems("filetype", defaults)

	require.Len(t, items, len(wantNames))
	assert.ElementsMatch(t, wantNames, valuesAsStrings(t, values))
	assert.Equal(t, optionsChoiceHint, hint)
}

func TestOptionsValueItemsFreeform(t *testing.T) {
	defaults := config.DefaultAllSettings()

	items, values, hint := optionsValueItems("tabsize", defaults)

	assert.Nil(t, items)
	assert.Nil(t, values)
	assert.Equal(t, optionsFreeformHint, hint)
}

func TestApplyOptionValueTemporary(t *testing.T) {
	resetOptionsTestConfig(t)

	require.NoError(t, applyOptionValue("tabsize", float64(8), false))

	assert.Equal(t, float64(8), config.GlobalSettings["tabsize"])
	assert.True(t, config.VolatileSettings["tabsize"],
		"temporary apply should mark the option volatile so reload keeps it")

	_, err := os.Stat(filepath.Join(config.ConfigDir, "settings.json"))
	assert.True(t, os.IsNotExist(err), "temporary apply must not write settings.json")
}

func TestApplyOptionValuePermanent(t *testing.T) {
	resetOptionsTestConfig(t)

	require.NoError(t, applyOptionValue("tabsize", float64(8), true))

	assert.Equal(t, float64(8), config.GlobalSettings["tabsize"])
	assert.False(t, config.VolatileSettings["tabsize"],
		"permanent apply must not leave the option marked volatile")

	data, err := os.ReadFile(filepath.Join(config.ConfigDir, "settings.json"))
	require.NoError(t, err, "permanent apply must write settings.json")

	var disk map[string]any
	require.NoError(t, json.Unmarshal(data, &disk))
	assert.Equal(t, float64(8), disk["tabsize"])
}

func TestApplyOptionValuePermanentClearsPriorVolatileMark(t *testing.T) {
	resetOptionsTestConfig(t)
	require.NoError(t, applyOptionValue("tabsize", float64(8), false))
	require.True(t, config.VolatileSettings["tabsize"])

	require.NoError(t, applyOptionValue("tabsize", float64(2), true))

	assert.False(t, config.VolatileSettings["tabsize"],
		"a later permanent apply should clear an earlier volatile mark")
}
