//go:build linux

package config

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"
)

// detectSystemTheme queries the XDG desktop portal for the
// freedesktop color-scheme preference. The portal is the modern
// cross-desktop signal (GNOME 44+, KDE Plasma 5.27+, sway, etc.) and
// is the single source micro consults on Linux. Sessions without a
// portal service running return SystemThemeUnknown, which the
// resolver treats as "fall back to the colorscheme option".
//
// The freedesktop spec for org.freedesktop.appearance/color-scheme
// (https://flatpak.github.io/xdg-desktop-portal/) maps the uint32
// reply: 0 = no preference, 1 = prefer dark, 2 = prefer light.
func detectSystemTheme() SystemTheme {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return SystemThemeUnknown
	}
	defer conn.Close()

	obj := conn.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop")
	var reply dbus.Variant
	call := obj.CallWithContext(ctx, "org.freedesktop.portal.Settings.Read", 0,
		"org.freedesktop.appearance", "color-scheme")
	if err := call.Store(&reply); err != nil {
		return SystemThemeUnknown
	}

	// The portal double-wraps the value in a variant on some
	// implementations; unwrap one extra layer when we see it.
	value := reply.Value()
	if inner, ok := value.(dbus.Variant); ok {
		value = inner.Value()
	}

	preference, ok := value.(uint32)
	if !ok {
		return SystemThemeUnknown
	}
	switch preference {
	case 1:
		return SystemThemeDark
	case 2:
		return SystemThemeLight
	default:
		return SystemThemeUnknown
	}
}
