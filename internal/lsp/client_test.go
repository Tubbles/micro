package lsp

import (
	"context"
	"testing"
	"time"

	"github.com/creachadair/jrpc2"
	"github.com/creachadair/jrpc2/channel"
	"github.com/creachadair/jrpc2/handler"

	"github.com/micro-editor/micro/v2/internal/lsp/protocol"
)

// newFakeServer starts a jrpc2 server implementing handlers over an
// in-memory channel.Direct() pair, and returns the client-side end of
// that pair. It stands in for a real language server process so Client
// lifecycle behaviour can be tested without launching one.
func newFakeServer(t *testing.T, handlers handler.Map) channel.Channel {
	t.Helper()
	clientSide, serverSide := channel.Direct()
	srv := jrpc2.NewServer(handlers, nil).Start(serverSide)
	t.Cleanup(func() {
		srv.Stop()
		srv.Wait()
	})
	return clientSide
}

// drainEvents runs the next func queued on Events, the same thing
// cmd/micro/micro.go's DoEvent select does for the "case f :=
// <-lsp.Events" arm. Tests use it to simulate the main loop noticing a
// posted effect.
func drainEvents(t *testing.T, timeout time.Duration) {
	t.Helper()
	select {
	case fn := <-Events:
		fn()
	case <-time.After(timeout):
		t.Fatal("timed out waiting for a posted lsp event")
	}
}

func waitForSignal(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s was never called", what)
	}
}

// lifecycleHandlers returns a handler.Map with stub "initialize" and
// "initialized" handlers (overridable via extra), plus a channel that
// receives once the fake server has actually finished processing the
// "initialized" notification.
//
// Waiting on that channel before a test ends matters because
// newFakeServer's cleanup calls Server.Stop(), and jrpc2's Stop races
// benignly-but-fatally with a read() that has already dequeued a
// message but not yet signalled the work queue: Stop closes that queue
// out from under it, and the pending signal panics with "send on
// closed channel". Confirming the notification was fully handled (not
// just handed off through the channel) avoids tripping that race.
func lifecycleHandlers(extra handler.Map) (handler.Map, <-chan struct{}) {
	initialized := make(chan struct{}, 1)
	handlers := handler.Map{
		"initialize": handler.New(func(_ context.Context, _ protocol.InitializeParams) (protocol.InitializeResult, error) {
			return protocol.InitializeResult{}, nil
		}),
		// The initialized notification carries "params":{} (an empty
		// object per the LSP spec), not absent params, so the handler
		// must declare a (here, ignored) argument: a zero-argument
		// handler form rejects any request that has params at all.
		"initialized": handler.New(func(_ context.Context, _ protocol.InitializedParams) error {
			initialized <- struct{}{}
			return nil
		}),
	}
	for method, h := range extra {
		handlers[method] = h
	}
	return handlers, initialized
}

func TestClientInitializeStoresCapabilitiesAndNegotiatedPositionEncoding(t *testing.T) {
	var gotParams protocol.InitializeParams
	initializedCh := make(chan struct{}, 1)
	ch := newFakeServer(t, handler.Map{
		"initialize": handler.New(func(_ context.Context, params protocol.InitializeParams) (protocol.InitializeResult, error) {
			gotParams = params
			return protocol.InitializeResult{
				Capabilities: protocol.ServerCapabilities{
					PositionEncoding: protocol.PositionEncodingUTF8,
					HoverProvider:    []byte("true"),
				},
			}, nil
		}),
		"initialized": handler.New(func(_ context.Context, _ protocol.InitializedParams) error {
			initializedCh <- struct{}{}
			return nil
		}),
	})

	c := newClientOverChannel("go", "/root", ch)

	var gotResult protocol.InitializeResult
	var gotErr error
	done := make(chan struct{})
	c.Initialize(context.Background(), InitializeOptions{RootURI: "file:///root"}, func(result protocol.InitializeResult, err error) {
		gotResult, gotErr = result, err
		close(done)
	})

	drainEvents(t, 2*time.Second)
	<-done
	waitForSignal(t, initializedCh, "initialized")

	if gotErr != nil {
		t.Fatalf("Initialize failed: %v", gotErr)
	}
	if gotResult.Capabilities.PositionEncoding != protocol.PositionEncodingUTF8 {
		t.Errorf("InitializeResult.Capabilities.PositionEncoding = %q, want %q", gotResult.Capabilities.PositionEncoding, protocol.PositionEncodingUTF8)
	}
	if got := c.PositionEncoding(); got != protocol.PositionEncodingUTF8 {
		t.Errorf("c.PositionEncoding() = %q, want %q", got, protocol.PositionEncodingUTF8)
	}
	if got := string(c.Capabilities().HoverProvider); got != "true" {
		t.Errorf("c.Capabilities().HoverProvider = %s, want true", got)
	}

	want := []protocol.PositionEncodingKind{protocol.PositionEncodingUTF8, protocol.PositionEncodingUTF16}
	got := gotParams.Capabilities.General.PositionEncodings
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("advertised PositionEncodings = %v, want %v", got, want)
	}
}

func TestClientInitializeDefaultsPositionEncodingToUTF16(t *testing.T) {
	handlers, initializedCh := lifecycleHandlers(nil)
	ch := newFakeServer(t, handlers)
	c := newClientOverChannel("go", "/root", ch)

	done := make(chan struct{})
	var gotErr error
	c.Initialize(context.Background(), InitializeOptions{}, func(_ protocol.InitializeResult, err error) {
		gotErr = err
		close(done)
	})

	drainEvents(t, 2*time.Second)
	<-done
	waitForSignal(t, initializedCh, "initialized")

	if gotErr != nil {
		t.Fatalf("Initialize failed: %v", gotErr)
	}
	if got := c.PositionEncoding(); got != protocol.PositionEncodingUTF16 {
		t.Errorf("c.PositionEncoding() = %q, want %q (the LSP default)", got, protocol.PositionEncodingUTF16)
	}
}

// TestClientBuffersRequestsUntilInitializeCompletes exercises D-40's
// "writer buffers until initialize completes" rule: a Call issued while
// initialize is still in flight must not reach the server until the
// handshake finishes, at which point it is flushed.
func TestClientBuffersRequestsUntilInitializeCompletes(t *testing.T) {
	release := make(chan struct{})
	someMethodCalled := make(chan struct{}, 1)

	handlers, initializedCh := lifecycleHandlers(handler.Map{
		"initialize": handler.New(func(_ context.Context, _ protocol.InitializeParams) (protocol.InitializeResult, error) {
			<-release
			return protocol.InitializeResult{}, nil
		}),
		"someMethod": handler.New(func(_ context.Context) error {
			someMethodCalled <- struct{}{}
			return nil
		}),
	})
	ch := newFakeServer(t, handlers)
	c := newClientOverChannel("go", "/root", ch)

	initDone := make(chan struct{})
	c.Initialize(context.Background(), InitializeOptions{}, func(_ protocol.InitializeResult, err error) {
		if err != nil {
			t.Errorf("Initialize failed: %v", err)
		}
		close(initDone)
	})

	// Initialize sets state = stateInitializing synchronously before it
	// returns (the handshake itself runs on its own goroutine, blocked
	// on <-release), so this Call is guaranteed to queue rather than
	// dispatch: no sleep or race needed to observe that.
	callDone := make(chan struct{})
	c.Call(context.Background(), "someMethod", nil, func(_ *jrpc2.Response, err error) {
		if err != nil {
			t.Errorf("queued someMethod call failed: %v", err)
		}
		close(callDone)
	})

	c.mu.Lock()
	queued := len(c.pending)
	c.mu.Unlock()
	if queued != 1 {
		t.Fatalf("queued pending calls = %d, want 1 (someMethod should be buffered, not sent)", queued)
	}

	select {
	case <-someMethodCalled:
		t.Fatal("someMethod reached the server before initialize completed")
	default:
	}

	close(release) // let the fake server's initialize handler return

	drainEvents(t, 2*time.Second) // runs Initialize's onDone, which flushes c.pending
	<-initDone

	waitForSignal(t, someMethodCalled, "someMethod")
	waitForSignal(t, initializedCh, "initialized")

	drainEvents(t, 2*time.Second) // runs the flushed someMethod call's callback
	<-callDone
}

func TestClientShutdownSendsShutdownThenExit(t *testing.T) {
	shutdownCalled := make(chan struct{}, 1)
	exitCalled := make(chan struct{}, 1)

	handlers, initializedCh := lifecycleHandlers(handler.Map{
		"shutdown": handler.New(func(_ context.Context) error {
			shutdownCalled <- struct{}{}
			return nil
		}),
		"exit": handler.New(func(_ context.Context) error {
			exitCalled <- struct{}{}
			return nil
		}),
	})
	ch := newFakeServer(t, handlers)
	c := newClientOverChannel("go", "/root", ch)

	initDone := make(chan struct{})
	c.Initialize(context.Background(), InitializeOptions{}, func(_ protocol.InitializeResult, err error) {
		if err != nil {
			t.Errorf("Initialize failed: %v", err)
		}
		close(initDone)
	})
	drainEvents(t, 2*time.Second)
	<-initDone
	waitForSignal(t, initializedCh, "initialized")

	shutdownDone := make(chan error, 1)
	c.Shutdown(func(err error) { shutdownDone <- err })
	drainEvents(t, 2*time.Second)

	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("Shutdown callback error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown never completed")
	}

	waitForSignal(t, shutdownCalled, "shutdown")
	waitForSignal(t, exitCalled, "exit")
}
