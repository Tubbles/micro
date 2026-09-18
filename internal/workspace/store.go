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

// LoadScratch reads the persistent scratch workspace's saved state,
// if any (D-55). The second return value reports whether scratch.json
// existed; when false the caller should fall back to an empty
// workspace, same convention as Load.
func LoadScratch(configDir string) (*State, bool, error) {
	data, err := os.ReadFile(ScratchStatePath(configDir))
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

// SaveScratch persists s as the persistent scratch workspace's state
// (scratch.json, D-55). Callers are responsible for only calling this
// from the instance that holds the scratch lock (see ClaimScratchLock)
// so the single shared file is not written by two instances at once.
func SaveScratch(configDir string, s *State) error {
	if err := os.MkdirAll(Dir(configDir), os.ModePerm); err != nil {
		return err
	}
	data, err := Encode(s)
	if err != nil {
		return err
	}
	return util.SafeWrite(ScratchStatePath(configDir), data, true)
}
