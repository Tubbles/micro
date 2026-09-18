package workspace

import (
	"encoding/json"
	"os"

	"github.com/micro-editor/micro/v2/internal/util"
)

// Recent is the most-recently-used list of dir-backed workspace
// directories, most-recent-first.
type Recent struct {
	Dirs []string `json:"dirs"`
}

// LoadRecent reads the MRU list for configDir. A missing file is not
// an error, it just means no workspace has been opened yet.
func LoadRecent(configDir string) (*Recent, error) {
	data, err := os.ReadFile(RecentPath(configDir))
	if err != nil {
		if os.IsNotExist(err) {
			return &Recent{}, nil
		}
		return nil, err
	}
	r := new(Recent)
	if err := json.Unmarshal(data, r); err != nil {
		return nil, err
	}
	return r, nil
}

// Touch moves dir to the front of the list, adding it if new and
// dropping any earlier occurrence so each directory appears once.
func (r *Recent) Touch(dir string) {
	dirs := make([]string, 0, len(r.Dirs)+1)
	dirs = append(dirs, dir)
	for _, d := range r.Dirs {
		if d != dir {
			dirs = append(dirs, d)
		}
	}
	r.Dirs = dirs
}

// Save persists the MRU list under configDir.
func (r *Recent) Save(configDir string) error {
	if err := os.MkdirAll(Dir(configDir), os.ModePerm); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return util.SafeWrite(RecentPath(configDir), data, true)
}
