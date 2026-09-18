//go:build linux || darwin || dragonfly || solaris || openbsd || netbsd || freebsd

package workspace

import (
	"errors"
	"os"
	"syscall"
)

// IsPidAlive reports whether pid names a running process on this
// host, via the standard signal-0 probe: os.FindProcess never fails
// on POSIX (it does not touch the OS), so liveness is determined by
// sending the null signal and checking whether the kernel could find
// the process to deliver it to.
//
// A process owned by a different user reports as alive too (the
// kernel returns EPERM, not ESRCH, because it exists but signaling it
// is not permitted); only "no such process" is treated as dead. This
// only matters for the same-host branch of the scratch lock protocol
// (D-55): a foreign-hostname lock is never probed at all.
func IsPidAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}
