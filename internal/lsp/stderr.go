package lsp

import (
	"bytes"

	"github.com/micro-editor/micro/v2/internal/buffer"
)

// serverStderr receives a language server's stderr as exec's copier
// goroutine delivers it and hands each complete line to emit. Chunks
// arrive at arbitrary boundaries, so a trailing fragment waits for the
// next write. emit runs on the copier goroutine and must not touch
// editor state itself; NewClient's emit posts to Events for that.
type serverStderr struct {
	emit    func(line string)
	pending []byte
}

func (s *serverStderr) Write(data []byte) (int, error) {
	s.pending = append(s.pending, data...)
	for {
		newline := bytes.IndexByte(s.pending, '\n')
		if newline < 0 {
			return len(data), nil
		}
		s.emit(string(s.pending[:newline]))
		s.pending = s.pending[newline+1:]
	}
}

// logServerLine writes one line of a server's stderr to the log buffer
// (> log), prefixed with the server name so several servers stay
// readable side by side. It runs on the main goroutine via post, which
// drops lines rather than stalling the server when the main loop is
// busy.
func logServerLine(name, line string) {
	if buffer.LogBuf == nil {
		return
	}
	buffer.WriteLog("[lsp:" + name + "] " + line + "\n")
}
