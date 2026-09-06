package lsp

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/micro-editor/micro/v2/internal/buffer"
)

func TestServerStderrEmitsCompleteLinesOnly(t *testing.T) {
	var lines []string
	writer := &serverStderr{emit: func(line string) { lines = append(lines, line) }}

	written, err := writer.Write([]byte("first\nsec"))
	assert.NoError(t, err)
	assert.Equal(t, 9, written)
	assert.Equal(t, []string{"first"}, lines)

	writer.Write([]byte("ond\n\nlast"))
	assert.Equal(t, []string{"first", "second", ""}, lines)

	writer.Write([]byte("\n"))
	assert.Equal(t, []string{"first", "second", "", "last"}, lines)
}

// TestNewClientMirrorsServerStderrIntoLog starts a "server" that only
// writes to stderr and exits, then runs the posted Events the way the
// main loop would until the line shows up in the log buffer.
func TestNewClientMirrorsServerStderrIntoLog(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs sh")
	}
	buffer.LogBuf = buffer.NewBufferFromString("", "", buffer.BTLog)
	t.Cleanup(func() { buffer.LogBuf = nil })

	def := ServerDefinition{Command: "sh", Args: []string{"-c", "echo boom >&2"}}
	client, err := NewClient("fake", def, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.cmd.Wait() })

	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(string(buffer.LogBuf.Bytes()), "[lsp:fake] boom\n") {
		if time.Now().After(deadline) {
			t.Fatalf("stderr line never reached the log buffer, log is %q", buffer.LogBuf.Bytes())
		}
		drainEvents(t, time.Second)
	}
}
