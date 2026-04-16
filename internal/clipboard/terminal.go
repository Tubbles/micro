package clipboard

import (
	"errors"
	"time"

	"github.com/micro-editor/micro/v2/internal/screen"
	"github.com/micro-editor/tcell/v2"
)

type terminalClipboard struct{}

var terminal terminalClipboard

func (t terminalClipboard) read(reg string) (string, error) {
	screen.Screen.GetClipboard()
	// wait at most 200ms for response
	for {
		select {
		case event := <-screen.Events:
			if e, ok := event.(*tcell.EventClipboard); ok {
				return string(e.Data()), nil
			}
		case <-time.After(200 * time.Millisecond):
			return "", errors.New("No clipboard received from terminal")
		}
	}
}

func (t terminalClipboard) write(text, reg string) error {
	screen.Screen.SetClipboard([]byte(text))
	return nil
}
