package action

import (
	"os"

	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/workspace"
)

// scratchIsPersisting records whether this process claimed the
// persistent scratch workspace's lock at startup (see
// ClaimScratchLockAtStartup). Only the persisting instance may write
// scratch.json (D-55); every other instance still runs a fully
// functional scratch workspace, it just never saves it.
var scratchIsPersisting bool

// ClaimScratchLockAtStartup attempts to claim the persistent scratch
// workspace's single-instance lock (scratch.lock, D-55) for this
// process, using config.ConfigDir, this process's pid and hostname,
// and workspace.IsPidAlive as the same-host liveness probe. It
// records the outcome so SaveActiveScratchWorkspace knows whether
// this instance may write scratch.json, and shows a one-line InfoBar
// notice the moment the claim comes back ephemeral (another instance
// already owns persistence).
//
// It is only meaningful once per process, at startup, before any
// dir-backed workspace might be opened (see cmd/micro's main()): a
// later `> opendir` does not re-run this, it only decides whether
// SaveActiveWorkspace's scratch branch is reachable again once the
// session returns to no-dir.
func ClaimScratchLockAtStartup() (workspace.ScratchLockOutcome, error) {
	hostname, _ := os.Hostname()

	outcome, err := workspace.ClaimScratchLock(config.ConfigDir, os.Getpid(), hostname, workspace.IsPidAlive)
	scratchIsPersisting = outcome == workspace.ScratchLockPersisting

	if outcome == workspace.ScratchLockEphemeral {
		InfoBar.Message("Scratch workspace persistence disabled: another instance already owns it")
	}

	return outcome, err
}

// ReleaseScratchLockIfPersisting removes scratch.lock on a clean
// exit, but only if this instance is the one holding it (the
// persisting instance from ClaimScratchLockAtStartup). An ephemeral
// instance never touched the lock and must not remove it out from
// under whichever instance does hold it.
func ReleaseScratchLockIfPersisting() error {
	if !scratchIsPersisting {
		return nil
	}
	return workspace.ReleaseScratchLock(config.ConfigDir)
}

// SaveActiveScratchWorkspace persists the scratch workspace's current
// session to scratch.json, but only if this instance claimed the
// persisting role at startup (D-55); an ephemeral instance no-ops.
// SaveActiveWorkspace calls this whenever no dir-backed workspace is
// active (currentWorkspaceDir == "").
func SaveActiveScratchWorkspace() error {
	if !scratchIsPersisting {
		return nil
	}
	return SaveScratchWorkspace()
}
