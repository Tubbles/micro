//go:build !linux

package config

// detectSystemTheme returns SystemThemeUnknown on every platform
// other than Linux. The resolver treats unknown as "fall back to the
// colorscheme option", so the follow-system switch silently degrades
// to the manual setting on platforms where micro has no detector
// implementation yet.
func detectSystemTheme() SystemTheme {
	return SystemThemeUnknown
}
