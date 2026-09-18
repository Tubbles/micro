package action

import (
	"errors"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
)

var (
	errNoActiveWorkspace          = errors.New("No workspace is active")
	errWorkspaceOptionIsLocalOnly = errors.New("Option must be set with setlocal, not setworkspace")
)

// SetWorkspaceCmd implements `> setworkspace <option> <value>`: set
// an option in the active dir-backed workspace's own settings.json
// (${dir}/.ide/micro/settings.json, D-50), analogous to `> set` but
// targeting the workspace layer instead of the user's settings.json.
// Only valid when a dir-backed workspace is active.
func (h *BufPane) SetWorkspaceCmd(args []string) {
	if len(args) < 2 {
		InfoBar.Error("Not enough arguments")
		return
	}

	if err := SetWorkspaceOption(args[0], args[1]); err != nil {
		InfoBar.Error(err)
	}
}

// SetWorkspaceOption parses value for option (same rules as
// SetGlobalOption) and, if valid, records it via
// SetWorkspaceOptionNative. Returns an error, without writing
// anything, when no dir-backed workspace is active.
func SetWorkspaceOption(option, value string) error {
	if currentWorkspaceDir == "" {
		return errNoActiveWorkspace
	}

	nativeValue, err := config.GetNativeValue(option, value)
	if err != nil {
		return err
	}

	return SetWorkspaceOptionNative(option, nativeValue)
}

// SetWorkspaceOptionNative validates nativeValue for option, then
// records it in the active workspace's settings.json
// (config.parsedWorkspaceSettings via UpdateParsedWorkspaceSetting),
// rebuilds GlobalSettings and every open buffer's settings so the
// change takes effect immediately, and persists the workspace's
// settings.json. filetype/readonly (config.LocalSettings) are
// rejected: SetGlobalOptionNative redirects those to the current
// buffer for `> set`, but there is no "current buffer" a workspace-
// wide default should mean; `setlocal` is the only way to set them.
func SetWorkspaceOptionNative(option string, nativeValue any) error {
	if err := config.OptionIsValid(option, nativeValue); err != nil {
		return err
	}

	for _, s := range config.LocalSettings {
		if s == option {
			return errWorkspaceOptionIsLocalOnly
		}
	}

	config.UpdateParsedWorkspaceSetting(option, nativeValue)
	config.RebuildGlobalSettings()
	for _, b := range buffer.OpenBuffers {
		b.ReloadSettings(true)
	}

	return config.WriteWorkspaceSettings(currentWorkspaceDir)
}
