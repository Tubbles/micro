package shell

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// collectJob spawns a job and runs its queued callbacks on this
// goroutine, the way the main loop does, until the exit callback has run.
func collectJob(t *testing.T, cmdName string, cmdArgs []string) (stderr []string, exitOutput string) {
	t.Helper()
	exited := false
	onStderr := func(out string, _ []any) { stderr = append(stderr, out) }
	onExit := func(out string, _ []any) {
		exitOutput = out
		exited = true
	}
	JobSpawn(cmdName, cmdArgs, nil, onStderr, onExit)
	for !exited {
		select {
		case job := <-Jobs:
			job.Function(job.Output, job.Args)
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for the job to exit")
		}
	}
	return stderr, exitOutput
}

func TestJobSpawnReportsStartFailureOnStderr(t *testing.T) {
	stderr, _ := collectJob(t, "micro-test-no-such-executable", nil)
	if assert.Len(t, stderr, 1) {
		assert.Contains(t, stderr[0], "executable file not found")
		assert.True(t, strings.HasSuffix(stderr[0], "\n"))
	}
}

func TestJobSpawnLeavesStderrAloneWhenTheProcessRan(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs sh")
	}
	stderr, _ := collectJob(t, "sh", []string{"-c", "echo real >&2; exit 3"})
	assert.Equal(t, []string{"real\n"}, stderr)
}
