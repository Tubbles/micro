//go:build plan9 || nacl || windows

package workspace

import "os"

// IsPidAlive reports whether pid names a running process on this
// host. There is no portable signal-0 probe on this platform (see the
// posix implementation), so this relies on os.FindProcess itself: on
// Windows it opens a handle via OpenProcess and fails if pid does not
// name a running process, which is exactly the liveness check the
// scratch lock protocol needs (D-55).
func IsPidAlive(pid int) bool {
	_, err := os.FindProcess(pid)
	return err == nil
}
