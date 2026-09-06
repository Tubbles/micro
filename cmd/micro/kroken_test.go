package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Tubbles/tcell/v3"
	"github.com/micro-editor/micro/v2/internal/action"
	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/shell"
	"github.com/stretchr/testify/assert"
)

// fakeKrokenScript stands in for the kroken executable. It blocks until
// the file named by KROKEN_TEST_WAIT_FOR exists (when set), so a test can
// edit the buffer while the "run" is in flight, then either fails like
// kroken does or echoes the selection wrapped in markers.
const fakeKrokenScript = `#!/bin/sh
while [ -n "$KROKEN_TEST_WAIT_FOR" ] && [ ! -e "$KROKEN_TEST_WAIT_FOR" ]; do
	sleep 0.01
done
selection=""
while [ $# -gt 0 ]; do
	if [ "$1" = "--selection-file" ]; then
		selection="$2"
		shift
	fi
	shift
done
if [ -n "$KROKEN_TEST_FAIL" ]; then
	echo "kroken: claude failed (test): boom" >&2
	exit 1
fi
printf 'BEGIN['
cat "$selection"
printf ']END'
echo "kroken: done in 0.0 s, 1 turns, \$0.0000" >&2
exit 0
`

func installFakeKroken(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake kroken is a shell script")
	}
	binDir := t.TempDir()
	err := os.WriteFile(filepath.Join(binDir, "kroken"), []byte(fakeKrokenScript), 0755)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("KROKEN_TEST_WAIT_FOR", "")
	t.Setenv("KROKEN_TEST_FAIL", "")
}

func openWithSelection(t *testing.T, content string, start, end buffer.Loc) *buffer.Buffer {
	file := createTestFile(t, content)
	openFile(file)
	buf := findBuffer(file)
	if buf == nil {
		t.Fatalf("Could not find buffer %s", file)
	}
	// A modified buffer makes the next test's "> open" stop at the
	// save-changes prompt, so leave the buffer clean.
	t.Cleanup(func() {
		if buf.Modified() {
			if err := buf.Save(); err != nil {
				t.Log(err)
			}
		}
	})
	cursor := buf.GetActiveCursor()
	cursor.SetSelectionStart(start)
	cursor.SetSelectionEnd(end)
	return buf
}

func runKrokenCommand() {
	action.InfoBar.HasError = false
	injectKey(tcell.KeyCtrlE, rune(tcell.KeyCtrlE), tcell.ModCtrl)
	injectString("kroken")
	injectKey(tcell.KeyEnter, rune(tcell.KeyEnter), tcell.ModNone)
}

// pumpJobsUntil runs pending job callbacks on the main loop until done
// reports true.
func pumpJobsUntil(t *testing.T, done func() bool) {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		for len(shell.Jobs) > 0 {
			DoEvent()
		}
		if done() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for the kroken job")
}

func TestKrokenReplacesSelectionAfterEditsAbove(t *testing.T) {
	installFakeKroken(t)
	goFile := filepath.Join(t.TempDir(), "go")
	t.Setenv("KROKEN_TEST_WAIT_FOR", goFile)

	buf := openWithSelection(t, "one\ntwo\nthree\nfour\n", buffer.Loc{0, 1}, buffer.Loc{0, 3})
	runKrokenCommand()

	buf.Insert(buffer.Loc{0, 0}, "zero\n")
	if err := os.WriteFile(goFile, nil, 0644); err != nil {
		t.Fatal(err)
	}

	pumpJobsUntil(t, func() bool { return strings.Contains(string(buf.Bytes()), "BEGIN[") })
	assert.Equal(t, "zero\none\nBEGIN[two\nthree\n]ENDfour\n", string(buf.Bytes()))
	assert.False(t, action.InfoBar.HasError)
	assert.Equal(t, "kroken: done in 0.0 s, 1 turns, $0.0000", action.InfoBar.Msg)
	log := string(buffer.LogBuf.Bytes())
	assert.Contains(t, log, "[kroken] kroken complete --file ")
	assert.Contains(t, log, "[kroken] kroken: done in 0.0 s, 1 turns, $0.0000\n")
}

func TestKrokenFailureLeavesBufferUntouched(t *testing.T) {
	installFakeKroken(t)
	t.Setenv("KROKEN_TEST_FAIL", "1")

	buf := openWithSelection(t, "one\ntwo\n", buffer.Loc{0, 0}, buffer.Loc{0, 1})
	runKrokenCommand()

	pumpJobsUntil(t, func() bool { return action.InfoBar.HasError })
	assert.Equal(t, "one\ntwo\n", string(buf.Bytes()))
	assert.Equal(t, "kroken: claude failed (test): boom", action.InfoBar.Msg)
	log := string(buffer.LogBuf.Bytes())
	assert.Contains(t, log, "[kroken] kroken: claude failed (test): boom\n")
	assert.Contains(t, log, "[kroken] exit status 1\n")
}

func TestKrokenRefusesWhenSelectionChanged(t *testing.T) {
	installFakeKroken(t)
	goFile := filepath.Join(t.TempDir(), "go")
	t.Setenv("KROKEN_TEST_WAIT_FOR", goFile)

	buf := openWithSelection(t, "one\ntwo\nthree\n", buffer.Loc{0, 1}, buffer.Loc{0, 2})
	runKrokenCommand()

	buf.Insert(buffer.Loc{1, 1}, "X")
	if err := os.WriteFile(goFile, nil, 0644); err != nil {
		t.Fatal(err)
	}

	pumpJobsUntil(t, func() bool { return action.InfoBar.HasError })
	assert.Equal(t, "one\ntXwo\nthree\n", string(buf.Bytes()))
	assert.Contains(t, action.InfoBar.Msg, "selection changed")
}

func TestKrokenMissingExecutableIsReported(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	buf := openWithSelection(t, "one\ntwo\n", buffer.Loc{0, 0}, buffer.Loc{0, 1})
	runKrokenCommand()

	pumpJobsUntil(t, func() bool { return action.InfoBar.HasError })
	assert.Equal(t, "one\ntwo\n", string(buf.Bytes()))
	assert.Equal(t, "kroken: could not start kroken, see > log", action.InfoBar.Msg)
	assert.Contains(t, string(buffer.LogBuf.Bytes()), "[kroken] exec: \"kroken\": executable file not found")
}
