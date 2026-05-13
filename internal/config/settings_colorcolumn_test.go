package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateColorcolumnScalar(t *testing.T) {
	assert.NoError(t, validateColorcolumn("colorcolumn", float64(0)))
	assert.NoError(t, validateColorcolumn("colorcolumn", float64(80)))
	assert.Error(t, validateColorcolumn("colorcolumn", float64(-1)))
}

func TestValidateColorcolumnList(t *testing.T) {
	assert.NoError(t, validateColorcolumn("colorcolumn", []any{}))
	assert.NoError(t, validateColorcolumn("colorcolumn", []any{float64(80)}))
	assert.NoError(t, validateColorcolumn("colorcolumn", []any{float64(72), float64(80), float64(120)}))
	assert.NoError(t, validateColorcolumn("colorcolumn", []any{float64(0), float64(80)}))

	assert.Error(t, validateColorcolumn("colorcolumn", []any{float64(-1)}))
	assert.Error(t, validateColorcolumn("colorcolumn", []any{float64(80), float64(-1)}))
	assert.Error(t, validateColorcolumn("colorcolumn", []any{"80"}))
	assert.Error(t, validateColorcolumn("colorcolumn", []any{float64(80), "bad"}))
}

func TestValidateColorcolumnRejectsOther(t *testing.T) {
	assert.Error(t, validateColorcolumn("colorcolumn", "80"))
	assert.Error(t, validateColorcolumn("colorcolumn", true))
	assert.Error(t, validateColorcolumn("colorcolumn", nil))
}

func TestVerifySettingColorcolumn(t *testing.T) {
	def := float64(0)
	assert.NoError(t, verifySetting("colorcolumn", float64(80), def))
	assert.NoError(t, verifySetting("colorcolumn", []any{float64(72), float64(80), float64(120)}, def))
	assert.NoError(t, verifySetting("colorcolumn", []any{}, def))
	assert.Error(t, verifySetting("colorcolumn", "80", def))
	assert.Error(t, verifySetting("colorcolumn", true, def))
	assert.Error(t, verifySetting("colorcolumn", float64(-1), def))
	assert.Error(t, verifySetting("colorcolumn", []any{float64(-1)}, def))
}

func TestGetNativeValueColorcolumn(t *testing.T) {
	// GetNativeValue reads GlobalSettings to discover the current kind.
	// Make sure the colorcolumn entry exists with its default float64(0).
	if GlobalSettings == nil {
		GlobalSettings = DefaultAllSettings()
	} else if _, ok := GlobalSettings["colorcolumn"]; !ok {
		GlobalSettings["colorcolumn"] = float64(0)
	}

	v, err := GetNativeValue("colorcolumn", "80")
	assert.NoError(t, err)
	assert.Equal(t, float64(80), v)

	v, err = GetNativeValue("colorcolumn", "72,80,120")
	assert.NoError(t, err)
	assert.Equal(t, []any{float64(72), float64(80), float64(120)}, v)

	v, err = GetNativeValue("colorcolumn", "72, 80, 120")
	assert.NoError(t, err)
	assert.Equal(t, []any{float64(72), float64(80), float64(120)}, v)

	_, err = GetNativeValue("colorcolumn", "80,bad")
	assert.Error(t, err)

	_, err = GetNativeValue("colorcolumn", "bad")
	assert.Error(t, err)
}
