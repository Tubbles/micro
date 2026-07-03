package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetSettingOriginDefault covers a common option with no override
// anywhere: the classification should fall all the way through to
// the default layer.
func TestGetSettingOriginDefault(t *testing.T) {
	resetSettings()
	configDirWith(t, `{}`, ``)
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())
	require.NoError(t, InitGlobalSettings())

	value, layer := GetSettingOrigin("ruler", nil, nil)
	assert.Equal(t, true, value, "ruler default is true")
	assert.Equal(t, SettingOriginDefault, layer)
}

// TestGetSettingOriginGlobal covers a scalar override living only in
// settings.json.
func TestGetSettingOriginGlobal(t *testing.T) {
	resetSettings()
	configDirWith(t, `{"ruler": false}`, ``)
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())
	require.NoError(t, InitGlobalSettings())

	value, layer := GetSettingOrigin("ruler", nil, nil)
	assert.Equal(t, false, value)
	assert.Equal(t, SettingOriginGlobal, layer)
}

// TestGetSettingOriginLocal covers settings.local.json overriding
// settings.json for the same key.
func TestGetSettingOriginLocal(t *testing.T) {
	resetSettings()
	configDirWith(t, `{"ruler": false}`, `{"ruler": true}`)
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())
	require.NoError(t, InitGlobalSettings())

	value, layer := GetSettingOrigin("ruler", nil, nil)
	assert.Equal(t, true, value)
	assert.Equal(t, SettingOriginLocal, layer)
}

// TestGetSettingOriginVolatile covers a session-only override (e.g. a
// CLI flag) recorded in VolatileSettings. The value comes from
// GlobalSettings, not from either parsed file.
func TestGetSettingOriginVolatile(t *testing.T) {
	resetSettings()
	configDirWith(t, `{"ruler": true}`, ``)
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())
	require.NoError(t, InitGlobalSettings())

	GlobalSettings["ruler"] = false
	VolatileSettings["ruler"] = true

	value, layer := GetSettingOrigin("ruler", nil, nil)
	assert.Equal(t, false, value)
	assert.Equal(t, SettingOriginVolatile, layer)
}

// TestGetSettingOriginBufferLocal covers a per-buffer `setlocal`
// override, expressed as the caller's own bufSettings/bufLocalSettings
// maps (mirroring buffer.Buffer.Settings/LocalSettings) since this
// package cannot import *buffer.Buffer.
func TestGetSettingOriginBufferLocal(t *testing.T) {
	resetSettings()
	configDirWith(t, `{"ruler": false}`, ``)
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())
	require.NoError(t, InitGlobalSettings())

	bufSettings := map[string]any{"ruler": true}
	bufLocalSettings := map[string]bool{"ruler": true}

	value, layer := GetSettingOrigin("ruler", bufSettings, bufLocalSettings)
	assert.Equal(t, true, value)
	assert.Equal(t, SettingOriginBufferLocal, layer)
}

// TestGetSettingOriginBufferLocalOutranksVolatile documents the tie-
// break for the rare case where a `setlocal` on this buffer runs
// after an earlier session-wide volatile set: the more specific,
// more recent per-buffer value wins the classification.
func TestGetSettingOriginBufferLocalOutranksVolatile(t *testing.T) {
	resetSettings()
	configDirWith(t, `{}`, ``)
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())
	require.NoError(t, InitGlobalSettings())

	GlobalSettings["ruler"] = false
	VolatileSettings["ruler"] = true

	bufSettings := map[string]any{"ruler": true}
	bufLocalSettings := map[string]bool{"ruler": true}

	value, layer := GetSettingOrigin("ruler", bufSettings, bufLocalSettings)
	assert.Equal(t, true, value)
	assert.Equal(t, SettingOriginBufferLocal, layer)
}

// TestGetSettingOriginGlobalOnlyOption covers an option that has no
// per-buffer representation at all (colorscheme lives only in
// GlobalSettings/parsedSettings). Passing nil buffer maps must not
// panic and must still classify correctly.
func TestGetSettingOriginGlobalOnlyOption(t *testing.T) {
	resetSettings()
	configDirWith(t, `{"colorscheme": "monokai"}`, ``)
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())
	require.NoError(t, InitGlobalSettings())

	value, layer := GetSettingOrigin("colorscheme", nil, nil)
	assert.Equal(t, "monokai", value)
	assert.Equal(t, SettingOriginGlobal, layer)
}

// TestGetSettingOriginIgnoresNestedOverrides ensures a ft:/glob:
// nested map in either parsed file is never mistaken for a scalar
// override of a same-named option (there is no such collision by
// construction, but this pins the isNestedSetting guard against a
// key that happens to shadow a real option name inside a nested map
// without polluting the scalar read).
func TestGetSettingOriginIgnoresNestedOverrides(t *testing.T) {
	resetSettings()
	configDirWith(t, `{"ft:go": {"tabsize": 2}}`, ``)
	require.NoError(t, ReadSettings())
	require.NoError(t, ReadLocalSettings())
	require.NoError(t, InitGlobalSettings())

	value, layer := GetSettingOrigin("tabsize", nil, nil)
	assert.Equal(t, float64(4), value, "ft:go's tabsize must not leak into the scalar classification")
	assert.Equal(t, SettingOriginDefault, layer)
}
