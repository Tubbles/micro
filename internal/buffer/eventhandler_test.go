package buffer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplyDiffOnDosBufferKeepsLineStructure(t *testing.T) {
	b := NewBufferFromString("a\r\nb\r\nc\r\n", "", BTDefault)
	if b.Endings != FFDos {
		t.Fatal("DOS line endings were not detected from the string")
	}

	b.ApplyDiff("a\r\nB\r\nc\r\n")

	assert.Equal(t, 4, b.LinesNum())
	assert.Equal(t, "B", b.Line(1))
	assert.Equal(t, "a\r\nB\r\nc\r\n", string(b.Bytes()))
}

func TestApplyDiffFoldsCRLFIntoStoredLines(t *testing.T) {
	b := NewBufferFromString("a\nb\n", "", BTDefault)

	b.ApplyDiff("a\r\nB\r\n")

	assert.Equal(t, []byte("a\nB\n"), b.joinedLines())
	assert.Equal(t, "a\nB\n", string(b.Bytes()))
}

func TestReOpenRedetectsLineEndings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "endings.txt")
	if err := os.WriteFile(path, []byte("a\nb\n"), 0644); err != nil {
		t.Fatal(err)
	}
	b, err := NewBufferFromFile(path, BTDefault)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, FileFormat(FFUnix), b.Endings)

	if err := os.WriteFile(path, []byte("a\r\nB\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := b.ReOpen(); err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "B", b.Line(1))
	assert.Equal(t, FileFormat(FFDos), b.Endings)
	assert.Equal(t, "dos", b.Settings["fileformat"])
	assert.Equal(t, "a\r\nB\r\n", string(b.Bytes()))
}
