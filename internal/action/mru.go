package action

import "sync"

// MRUList tracks pane split-IDs (BufPane.ID()) in most-recently-used
// order, front-first. Production code touches it at the two SetActive
// chokepoints the jump list already instruments (TabList.SetActive,
// Tab.SetActive) and at pane creation (NewTabFromBuffer,
// NewTabFromPane, VSplitIndex, HSplitIndex): a pane can become the
// zero-value "active" pane of a brand-new Tab without SetActive ever
// running on it (e.g. tab 0 at editor startup), so creation-time
// touch is what keeps such a pane from being invisible to the MRU
// order entirely.
//
// Shaped after JumpList: a package-level instance (MRU) for
// production use, with the alive check threaded through the read
// path as a callback rather than a hard dependency on the global
// Tabs, so tests can swap in their own alive func without wiring up a
// real Tab/BufPane tree.
type MRUList struct {
	mu  sync.Mutex
	ids []uint64
}

// MRU is the global MRU list. Lives for the duration of the editor
// session; not persisted across restarts.
var MRU = &MRUList{}

// Touch moves id to the front, inserting it if not already present.
func (m *MRUList) Touch(id uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, existing := range m.ids {
		if existing == id {
			copy(m.ids[1:i+1], m.ids[:i])
			m.ids[0] = id
			return
		}
	}
	m.ids = append([]uint64{id}, m.ids...)
}

// List returns the MRU list, most-recently-used first, after dropping
// any id the alive callback rejects. Pruning mutates the stored list
// (mirroring JumpList.Back/Forward's dead-entry pruning), so a dead
// pane is dropped once rather than re-checked on every read.
func (m *MRUList) List(alive func(uint64) bool) []uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	live := m.ids[:0]
	for _, id := range m.ids {
		if alive(id) {
			live = append(live, id)
		}
	}
	m.ids = live
	out := make([]uint64, len(live))
	copy(out, live)
	return out
}
