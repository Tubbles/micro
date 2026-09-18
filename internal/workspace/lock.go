package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/micro-editor/micro/v2/internal/util"
)

// ScratchLockPath returns the path of the persistent scratch
// workspace's single-instance concurrency lock (D-55).
func ScratchLockPath(configDir string) string {
	return filepath.Join(Dir(configDir), "scratch.lock")
}

// ScratchLockOutcome is the result of attempting to claim the
// persistent scratch workspace's single-instance lock (D-55).
type ScratchLockOutcome int

const (
	// ScratchLockPersisting means this instance now owns scratch.json:
	// either the lock did not exist yet, or it named a same-host
	// process that is no longer running and was stolen.
	ScratchLockPersisting ScratchLockOutcome = iota
	// ScratchLockEphemeral means another instance already owns
	// scratch.json: the scratch workspace is still fully usable this
	// session, but it is never persisted, and scratch.lock is left
	// untouched.
	ScratchLockEphemeral
)

// lockContent formats scratch.lock's payload: "<pid> <hostname>".
func lockContent(pid int, hostname string) []byte {
	return []byte(fmt.Sprintf("%d %s", pid, hostname))
}

// parseLockContent parses scratch.lock's payload. ok is false for
// anything that doesn't look like "<pid> <hostname>", which
// ClaimScratchLock treats the same as a foreign lock: not safe to
// steal.
func parseLockContent(data []byte) (pid int, hostname string, ok bool) {
	fields := strings.Fields(string(data))
	if len(fields) != 2 {
		return 0, "", false
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, "", false
	}
	return pid, fields[1], true
}

// createLockExclusive creates configDir's scratch.lock with
// O_CREATE|O_EXCL, so the create itself fails if another instance won
// the race. It is also reused for the single post-steal-race retry in
// ClaimScratchLock.
func createLockExclusive(path string, pid int, hostname string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, util.FileMode)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(lockContent(pid, hostname))
	return err
}

// ClaimScratchLock attempts to claim configDir's persistent scratch
// workspace lock (scratch.lock) for (selfPid, selfHostname), following
// the check-and-set protocol from D-55.
//
// isPidAlive reports whether a pid names a running process ON THIS
// HOST (see IsPidAlive). It is threaded through as a parameter,
// rather than this function calling IsPidAlive itself, so the whole
// protocol is unit-testable without spawning or killing real
// processes.
//
// Outcomes:
//   - scratch.lock does not exist: claim it (O_CREATE|O_EXCL) and
//     return ScratchLockPersisting.
//   - it names selfHostname and a pid isPidAlive reports as dead:
//     steal it (overwrite in place) once and return
//     ScratchLockPersisting. This never loops: if the overwrite race
//     is lost too, the caller gets ScratchLockEphemeral rather than
//     this function retrying.
//   - anything else (a live same-host pid, a foreign hostname, or a
//     lock file that doesn't parse as "<pid> <hostname>") is left
//     untouched and this instance runs ScratchLockEphemeral. A
//     foreign-hostname lock is never stolen, live pid or not:
//     ConfigDir is synced across machines, so a foreign pid's
//     liveness is unknowable and meaningless here (D-55).
//
// A non-nil error means a genuine I/O problem unrelated to normal
// claim contention (for example a permission error creating the
// workspaces directory); the outcome is always ScratchLockEphemeral
// in that case, the safe fallback, so callers can treat the returned
// outcome as authoritative even when also surfacing the error.
func ClaimScratchLock(configDir string, selfPid int, selfHostname string, isPidAlive func(int) bool) (ScratchLockOutcome, error) {
	if err := os.MkdirAll(Dir(configDir), os.ModePerm); err != nil {
		return ScratchLockEphemeral, err
	}

	path := ScratchLockPath(configDir)

	err := createLockExclusive(path, selfPid, selfHostname)
	if err == nil {
		return ScratchLockPersisting, nil
	}
	if !os.IsExist(err) {
		return ScratchLockEphemeral, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return ScratchLockEphemeral, err
		}
		// The lock vanished between our failed claim and this read
		// (its owner exited cleanly in between): one more claim
		// attempt is the same single try a fresh start would make,
		// not a retry loop. If it also fails, something else won the
		// race in the meantime; fall through to ephemeral.
		if err := createLockExclusive(path, selfPid, selfHostname); err == nil {
			return ScratchLockPersisting, nil
		}
		return ScratchLockEphemeral, nil
	}

	pid, hostname, ok := parseLockContent(data)
	if !ok || hostname != selfHostname {
		// Unparseable, or a foreign host: never steal (D-55).
		return ScratchLockEphemeral, nil
	}
	if isPidAlive(pid) {
		return ScratchLockEphemeral, nil
	}

	// Same host, dead pid: steal once.
	if err := os.WriteFile(path, lockContent(selfPid, selfHostname), util.FileMode); err != nil {
		return ScratchLockEphemeral, err
	}
	return ScratchLockPersisting, nil
}

// ReleaseScratchLock removes configDir's scratch.lock. Only the
// persisting instance (the one ClaimScratchLock returned
// ScratchLockPersisting to) should ever call this, on its own clean
// exit: removing it on behalf of another live instance would let a
// third instance claim persistence while the second one still
// believes it owns it.
func ReleaseScratchLock(configDir string) error {
	err := os.Remove(ScratchLockPath(configDir))
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}
