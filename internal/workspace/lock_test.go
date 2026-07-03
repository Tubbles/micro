package workspace

import (
	"os"
	"testing"
)

func alwaysAlive(int) bool { return true }
func alwaysDead(int) bool  { return false }
func neverProbed(int) bool { panic("isPidAlive must not be called for a foreign-hostname lock") }

func readLock(t *testing.T, configDir string) []byte {
	t.Helper()
	data, err := os.ReadFile(ScratchLockPath(configDir))
	if err != nil {
		t.Fatalf("reading scratch.lock: %v", err)
	}
	return data
}

// TestClaimScratchLockFreeSucceeds proves the base case: no lock file
// yet means this instance claims it outright and scratch.lock ends up
// holding this instance's own pid and hostname.
func TestClaimScratchLockFreeSucceeds(t *testing.T) {
	configDir := t.TempDir()

	outcome, err := ClaimScratchLock(configDir, 4242, "myhost", neverProbed)
	if err != nil {
		t.Fatalf("ClaimScratchLock: %v", err)
	}
	if outcome != ScratchLockPersisting {
		t.Fatalf("outcome = %v, want ScratchLockPersisting", outcome)
	}

	pid, hostname, ok := parseLockContent(readLock(t, configDir))
	if !ok || pid != 4242 || hostname != "myhost" {
		t.Fatalf("scratch.lock content = %q, want \"4242 myhost\"", readLock(t, configDir))
	}
}

// TestClaimScratchLockLiveSameHostIsEphemeral proves a same-host lock
// naming a still-running pid is honored: the claim goes ephemeral and
// the lock file is left exactly as it was.
func TestClaimScratchLockLiveSameHostIsEphemeral(t *testing.T) {
	configDir := t.TempDir()
	if err := os.MkdirAll(Dir(configDir), os.ModePerm); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ScratchLockPath(configDir), lockContent(1111, "myhost"), 0o644); err != nil {
		t.Fatal(err)
	}

	outcome, err := ClaimScratchLock(configDir, 2222, "myhost", alwaysAlive)
	if err != nil {
		t.Fatalf("ClaimScratchLock: %v", err)
	}
	if outcome != ScratchLockEphemeral {
		t.Fatalf("outcome = %v, want ScratchLockEphemeral", outcome)
	}

	pid, hostname, ok := parseLockContent(readLock(t, configDir))
	if !ok || pid != 1111 || hostname != "myhost" {
		t.Fatalf("scratch.lock content changed, got %q, want the original owner untouched", readLock(t, configDir))
	}
}

// TestClaimScratchLockDeadSameHostStealsOnce proves the recovery path:
// a same-host lock naming a pid that is no longer running is stolen,
// and scratch.lock is overwritten with this instance's own identity.
func TestClaimScratchLockDeadSameHostStealsOnce(t *testing.T) {
	configDir := t.TempDir()
	if err := os.MkdirAll(Dir(configDir), os.ModePerm); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ScratchLockPath(configDir), lockContent(1111, "myhost"), 0o644); err != nil {
		t.Fatal(err)
	}

	outcome, err := ClaimScratchLock(configDir, 2222, "myhost", alwaysDead)
	if err != nil {
		t.Fatalf("ClaimScratchLock: %v", err)
	}
	if outcome != ScratchLockPersisting {
		t.Fatalf("outcome = %v, want ScratchLockPersisting", outcome)
	}

	pid, hostname, ok := parseLockContent(readLock(t, configDir))
	if !ok || pid != 2222 || hostname != "myhost" {
		t.Fatalf("scratch.lock content = %q, want the stolen lock to name the new owner (2222 myhost)", readLock(t, configDir))
	}
}

// TestClaimScratchLockForeignHostNeverSteals proves the D-55 rule that
// a foreign-hostname lock is always honored, even when the pid it
// names would report as dead: ConfigDir is synced across machines, so
// pid liveness only means anything on the same host. isPidAlive is
// neverProbed here specifically to prove it is never even consulted
// once the hostname mismatch is seen.
func TestClaimScratchLockForeignHostNeverSteals(t *testing.T) {
	configDir := t.TempDir()
	if err := os.MkdirAll(Dir(configDir), os.ModePerm); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ScratchLockPath(configDir), lockContent(1111, "otherhost"), 0o644); err != nil {
		t.Fatal(err)
	}

	outcome, err := ClaimScratchLock(configDir, 2222, "myhost", neverProbed)
	if err != nil {
		t.Fatalf("ClaimScratchLock: %v", err)
	}
	if outcome != ScratchLockEphemeral {
		t.Fatalf("outcome = %v, want ScratchLockEphemeral", outcome)
	}

	pid, hostname, ok := parseLockContent(readLock(t, configDir))
	if !ok || pid != 1111 || hostname != "otherhost" {
		t.Fatalf("scratch.lock content changed, got %q, want the foreign owner untouched", readLock(t, configDir))
	}
}

// TestReleaseScratchLockRemovesFile proves the persisting instance's
// clean-exit cleanup: releasing a claimed lock removes the file, and
// releasing an already-absent lock is a harmless no-op.
func TestReleaseScratchLockRemovesFile(t *testing.T) {
	configDir := t.TempDir()

	if _, err := ClaimScratchLock(configDir, 4242, "myhost", neverProbed); err != nil {
		t.Fatal(err)
	}
	if err := ReleaseScratchLock(configDir); err != nil {
		t.Fatalf("ReleaseScratchLock: %v", err)
	}
	if _, err := os.Stat(ScratchLockPath(configDir)); !os.IsNotExist(err) {
		t.Fatalf("expected scratch.lock to be removed, stat err = %v", err)
	}

	if err := ReleaseScratchLock(configDir); err != nil {
		t.Fatalf("ReleaseScratchLock on an absent lock: %v", err)
	}
}
