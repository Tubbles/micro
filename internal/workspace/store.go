package workspace

import (
	"os"

	"github.com/micro-editor/micro/v2/internal/util"
)

// Load reads the saved state for dir, if any. The second return
// value reports whether a state file existed; when false the caller
// should fall back to an empty workspace rather than treating it as
// an error.
func Load(configDir, dir string) (*State, bool, error) {
	path, _ := StatePath(configDir, dir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	state, err := Decode(data)
	if err != nil {
		return nil, false, err
	}
	return state, true, nil
}

// Save persists s under configDir, keyed by s.Dir. It creates the
// workspaces directory if necessary. Like buffer state (see
// internal/buffer/serialize.go), this is machine-only state, so it
// is written with SafeWrite's atomic rename rather than the
// overwrite-in-place mode used for user-editable files.
func Save(configDir string, s *State) error {
	if err := os.MkdirAll(Dir(configDir), os.ModePerm); err != nil {
		return err
	}
	data, err := Encode(s)
	if err != nil {
		return err
	}
	path, resolvePath := StatePath(configDir, s.Dir)
	if err := util.SafeWrite(path, data, true); err != nil {
		return err
	}
	if resolvePath != "" {
		if err := util.SafeWrite(resolvePath, []byte(s.Dir), true); err != nil {
			return err
		}
	}
	return nil
}
