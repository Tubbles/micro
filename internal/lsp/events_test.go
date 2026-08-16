package lsp

import (
	"testing"
	"time"
)

func TestPostedFuncRunsWhenEventsIsDrained(t *testing.T) {
	ran := make(chan struct{})
	post(func() { close(ran) })

	drainEvents(t, 2*time.Second)

	select {
	case <-ran:
	case <-time.After(2 * time.Second):
		t.Fatal("posted func did not run after being drained from Events")
	}
}
