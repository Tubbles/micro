package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func withResolverState(t *testing.T, settings map[string]any, theme SystemTheme, fn func()) {
	t.Helper()
	savedSettings := GlobalSettings
	savedDetector := DetectSystemTheme
	t.Cleanup(func() {
		GlobalSettings = savedSettings
		DetectSystemTheme = savedDetector
	})
	GlobalSettings = make(map[string]any, len(settings))
	for k, v := range settings {
		GlobalSettings[k] = v
	}
	DetectSystemTheme = func() SystemTheme { return theme }
	fn()
}

func TestResolveManualWhenFollowSystemOff(t *testing.T) {
	withResolverState(t, map[string]any{
		"colorscheme":               "monokai",
		"colorscheme.dark":          "darcula-tc",
		"colorscheme.light":         "sunny-day",
		"colorscheme.follow-system": false,
	}, SystemThemeDark, func() {
		name, source := ResolveEffectiveColorscheme()
		assert.Equal(t, "monokai", name)
		assert.Equal(t, "manual", source)
	})
}

func TestResolveDarkSlotWhenSystemDark(t *testing.T) {
	withResolverState(t, map[string]any{
		"colorscheme":               "monokai",
		"colorscheme.dark":          "darcula-tc",
		"colorscheme.light":         "sunny-day",
		"colorscheme.follow-system": true,
	}, SystemThemeDark, func() {
		name, source := ResolveEffectiveColorscheme()
		assert.Equal(t, "darcula-tc", name)
		assert.Equal(t, "dark", source)
	})
}

func TestResolveLightSlotWhenSystemLight(t *testing.T) {
	withResolverState(t, map[string]any{
		"colorscheme":               "monokai",
		"colorscheme.dark":          "darcula-tc",
		"colorscheme.light":         "sunny-day",
		"colorscheme.follow-system": true,
	}, SystemThemeLight, func() {
		name, source := ResolveEffectiveColorscheme()
		assert.Equal(t, "sunny-day", name)
		assert.Equal(t, "light", source)
	})
}

func TestResolveFallbackWhenSystemUnknown(t *testing.T) {
	withResolverState(t, map[string]any{
		"colorscheme":               "monokai",
		"colorscheme.dark":          "darcula-tc",
		"colorscheme.light":         "sunny-day",
		"colorscheme.follow-system": true,
	}, SystemThemeUnknown, func() {
		name, source := ResolveEffectiveColorscheme()
		assert.Equal(t, "monokai", name)
		assert.Equal(t, "fallback", source)
	})
}

func TestResolveFallbackWhenDarkSlotEmpty(t *testing.T) {
	withResolverState(t, map[string]any{
		"colorscheme":               "monokai",
		"colorscheme.dark":          "",
		"colorscheme.light":         "sunny-day",
		"colorscheme.follow-system": true,
	}, SystemThemeDark, func() {
		name, source := ResolveEffectiveColorscheme()
		assert.Equal(t, "monokai", name)
		assert.Equal(t, "fallback", source)
	})
}

func TestResolveFallbackWhenLightSlotEmpty(t *testing.T) {
	withResolverState(t, map[string]any{
		"colorscheme":               "monokai",
		"colorscheme.dark":          "darcula-tc",
		"colorscheme.light":         "",
		"colorscheme.follow-system": true,
	}, SystemThemeLight, func() {
		name, source := ResolveEffectiveColorscheme()
		assert.Equal(t, "monokai", name)
		assert.Equal(t, "fallback", source)
	})
}

func TestValidateColorschemeOrEmptyAcceptsEmpty(t *testing.T) {
	err := validateColorschemeOrEmpty("colorscheme.dark", "")
	assert.NoError(t, err)
}

func TestValidateColorschemeOrEmptyRejectsNonString(t *testing.T) {
	err := validateColorschemeOrEmpty("colorscheme.dark", 42)
	assert.Error(t, err)
}

func TestValidateColorschemeOrEmptyRejectsUnknownName(t *testing.T) {
	err := validateColorschemeOrEmpty("colorscheme.dark", "definitely-not-a-real-scheme-xyz")
	assert.Error(t, err)
}
