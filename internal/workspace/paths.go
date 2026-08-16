package workspace

import (
	"path/filepath"

	"github.com/micro-editor/micro/v2/internal/util"
)

// storeDirName is the ConfigDir subdirectory that holds machine-
// written workspace state. It is kept separate from the human-edited
// ${dir}/.ide/micro/ tree (D-50): layout/cursor churn should never
// show up as noise in the project's own git status.
const storeDirName = "workspaces"

// Dir returns the directory under configDir that holds workspace
// state files.
func Dir(configDir string) string {
	return filepath.Join(configDir, storeDirName)
}

// StatePath returns the path a dir-backed workspace's state should be
// read from or written to, keying it by escaping dir the same way
// micro already escapes buffer paths for its own per-file state (see
// util.DetermineEscapePath). Plain path escaping is deliberate: it
// avoids VS Code's path+inode hash, whose documented failure mode is
// losing state when the inode is recycled.
//
// The second return value, when non-empty, is a sidecar file that
// must be written alongside path recording the original dir string;
// it is only set when the escaped name would exceed the filesystem's
// name length limit and had to be replaced by a hash.
func StatePath(configDir, dir string) (path string, resolvePath string) {
	return util.DetermineEscapePath(Dir(configDir), dir)
}

// RecentPath returns the path to the MRU list of dir-backed
// workspaces.
func RecentPath(configDir string) string {
	return filepath.Join(Dir(configDir), "recent.json")
}

// ScratchStatePath returns the path the persistent scratch
// workspace's state is saved to (D-55). Unlike StatePath, this is a
// single fixed name: there is exactly one scratch workspace, so it
// needs no escaped-dir key.
func ScratchStatePath(configDir string) string {
	return filepath.Join(Dir(configDir), "scratch.json")
}

// IdeMicroDir returns the human-edited config directory for a dir-
// backed workspace: ${dir}/.ide/micro/ (D-50). Unlike Dir/StatePath/
// RecentPath, which live under configDir, this tree lives inside the
// workspace directory itself and is meant to be checked into the
// project's own version control.
func IdeMicroDir(dir string) string {
	return filepath.Join(dir, ".ide", "micro")
}

// SettingsPath returns the workspace-level settings.json path for
// dir.
func SettingsPath(dir string) string {
	return filepath.Join(IdeMicroDir(dir), "settings.json")
}

// LocalSettingsPath returns the workspace-level settings.local.json
// path for dir.
func LocalSettingsPath(dir string) string {
	return filepath.Join(IdeMicroDir(dir), "settings.local.json")
}

// BindingsLocalPath returns the workspace-level bindings.local.json
// path for dir.
func BindingsLocalPath(dir string) string {
	return filepath.Join(IdeMicroDir(dir), "bindings.local.json")
}
