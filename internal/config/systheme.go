package config

// detectSystemTheme is the production implementation behind the
// DetectSystemTheme variable. Commit 1 ships this stub so the
// resolver compiles standalone; commit 2 replaces this file with
// per-OS implementations (Linux uses the XDG portal D-Bus call,
// other platforms keep the stub).
func detectSystemTheme() SystemTheme {
	return SystemThemeUnknown
}
