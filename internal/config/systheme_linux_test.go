//go:build linux

package config

import (
	"os"
	"testing"
)

// TestDetectSystemThemeLive hits the real XDG desktop portal on the
// machine running the test. It is opt-in via MICRO_SYSTHEME_LIVE so
// CI without a desktop session does not see spurious failures, and so
// developers can run it explicitly to verify the wiring against
// whatever portal implementation is on the box.
//
// The test only checks that the call completes; the returned theme
// depends on the running session and is reported as a log line for
// the developer's benefit.
func TestDetectSystemThemeLive(t *testing.T) {
	if os.Getenv("MICRO_SYSTHEME_LIVE") == "" {
		t.Skip("set MICRO_SYSTHEME_LIVE=1 to run against the live D-Bus portal")
	}
	theme := detectSystemTheme()
	switch theme {
	case SystemThemeDark:
		t.Logf("portal reports: prefer dark")
	case SystemThemeLight:
		t.Logf("portal reports: prefer light")
	case SystemThemeUnknown:
		t.Logf("portal reports: no preference or no portal running")
	default:
		t.Fatalf("unexpected SystemTheme value: %d", theme)
	}
}
