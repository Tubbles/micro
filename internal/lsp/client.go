package lsp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/creachadair/jrpc2"
	"github.com/creachadair/jrpc2/channel"

	"github.com/micro-editor/micro/v2/internal/lsp/protocol"
)

// clientState tracks where a Client is in its lifecycle.
type clientState int

const (
	stateStarting clientState = iota
	stateInitializing
	stateReady
	stateShuttingDown
	stateStopped
)

// Client is a connection to a single language server, for one (server
// definition, workspace root) pair. It owns the child process (or, in
// tests, a direct in-memory channel), the jrpc2.Client, and the
// negotiated capabilities.
//
// Call and Notify issued before the initialize handshake completes are
// queued and flushed once the server has replied to initialize and
// micro has sent the initialized notification (the "writer buffers
// until initialize completes" rule; see D-40). Callers of Call/Notify
// never block: the jrpc2 request runs on its own goroutine and results
// are delivered by posting a closure onto Events.
type Client struct {
	Name string // server definition name, e.g. "go"
	Root string // workspace root this client was started for

	rpc *jrpc2.Client
	cmd *exec.Cmd // nil for clients built directly over a test channel

	mu           sync.Mutex
	state        clientState
	pending      []func() // Call/Notify closures queued while state != stateReady
	capabilities protocol.ServerCapabilities
	posEncoding  protocol.PositionEncodingKind
}

// NewClient starts def.Command as a child process rooted at root and
// wires its stdio to a jrpc2 client using LSP framing. It does not send
// the initialize request; call Initialize for that.
func NewClient(name string, def ServerDefinition, root string) (*Client, error) {
	cmd := exec.Command(def.Command, def.Args...)
	cmd.Dir = root
	// Servers report crashes, tracing, and misconfiguration on stderr.
	// Mirror it into the log buffer line by line; exec's copier feeds
	// the writer and Wait in Shutdown drains it before returning.
	cmd.Stderr = &serverStderr{emit: func(line string) {
		post(func() { logServerLine(name, line) })
	}}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	c := newClientOverChannel(name, root, channel.LSP(stdout, stdin))
	c.cmd = cmd
	return c, nil
}

// newClientOverChannel builds a Client around an already-established
// jrpc2 channel. Production code reaches this via NewClient; tests wire
// ch directly to a fake in-process server via channel.Direct(), so the
// initialize handshake can be exercised without spawning a real
// language server.
func newClientOverChannel(name, root string, ch channel.Channel) *Client {
	c := &Client{Name: name, Root: root, state: stateStarting}
	c.rpc = jrpc2.NewClient(ch, &jrpc2.ClientOptions{
		OnStop: c.handleStop,
		// handleNotify routes textDocument/publishDiagnostics (see
		// diagnostics.go); it is the only server->client notification
		// v1 acts on. OnCallback (server->client requests such as
		// workspace/configuration) is unwired: jrpc2's default applies
		// (callbacks logged and discarded) until a later chunk needs it.
		OnNotify: c.handleNotify,
	})
	return c
}

func (c *Client) handleStop(_ *jrpc2.Client, _ error) {
	c.mu.Lock()
	c.state = stateStopped
	c.mu.Unlock()
}

// whenReady runs fn on its own goroutine once the client has finished
// initializing, or immediately if it already has. If the client is
// still starting or initializing, fn is queued and run (still on its
// own goroutine) when Initialize completes. If the client has stopped
// or is shutting down, fn is dropped.
func (c *Client) whenReady(fn func()) {
	c.mu.Lock()
	switch c.state {
	case stateReady:
		c.mu.Unlock()
		go fn()
	case stateStopped, stateShuttingDown:
		c.mu.Unlock()
	default:
		c.pending = append(c.pending, fn)
		c.mu.Unlock()
	}
}

// InitializeOptions carries the caller-supplied parts of the initialize
// request. Client.Initialize fills in Capabilities itself: D-44 always
// advertises "utf-8" and "utf-16" position encodings.
type InitializeOptions struct {
	RootURI               protocol.DocumentURI
	InitializationOptions []byte
}

// Initialize sends the initialize request, and on success sends the
// initialized notification and marks the client ready, flushing any
// Call/Notify invocations that were queued in the meantime. onDone runs
// on the main goroutine (via Events) with the negotiated
// InitializeResult, or an error if the handshake failed.
func (c *Client) Initialize(ctx context.Context, opts InitializeOptions, onDone func(protocol.InitializeResult, error)) {
	c.mu.Lock()
	if c.state != stateStarting {
		c.mu.Unlock()
		post(func() {
			onDone(protocol.InitializeResult{}, fmt.Errorf("lsp: client %s is already initializing or initialized", c.Name))
		})
		return
	}
	c.state = stateInitializing
	c.mu.Unlock()

	pid := int32(os.Getpid())
	params := protocol.InitializeParams{
		ProcessID:             &pid,
		RootURI:               opts.RootURI,
		InitializationOptions: opts.InitializationOptions,
		Capabilities: protocol.ClientCapabilities{
			General: &protocol.GeneralClientCapabilities{
				PositionEncodings: []protocol.PositionEncodingKind{
					protocol.PositionEncodingUTF8,
					protocol.PositionEncodingUTF16,
				},
			},
		},
	}

	go func() {
		var result protocol.InitializeResult
		if err := c.rpc.CallResult(ctx, "initialize", params, &result); err != nil {
			c.mu.Lock()
			c.state = stateStopped
			c.mu.Unlock()
			post(func() { onDone(protocol.InitializeResult{}, err) })
			return
		}

		if err := c.rpc.Notify(ctx, "initialized", protocol.InitializedParams{}); err != nil {
			c.mu.Lock()
			c.state = stateStopped
			c.mu.Unlock()
			post(func() { onDone(protocol.InitializeResult{}, err) })
			return
		}

		posEncoding := result.Capabilities.PositionEncoding
		if posEncoding == "" {
			posEncoding = protocol.PositionEncodingUTF16
		}

		c.mu.Lock()
		c.capabilities = result.Capabilities
		c.posEncoding = posEncoding
		c.state = stateReady
		queued := c.pending
		c.pending = nil
		c.mu.Unlock()

		for _, fn := range queued {
			go fn()
		}

		post(func() { onDone(result, nil) })
	}()
}

// Call issues method with params, queuing it if the client has not yet
// finished initializing (see whenReady). callback runs on the main
// goroutine (via Events) with the response or error; Call itself never
// blocks the caller.
func (c *Client) Call(ctx context.Context, method string, params any, callback func(*jrpc2.Response, error)) {
	c.whenReady(func() {
		rsp, err := c.rpc.Call(ctx, method, params)
		post(func() { callback(rsp, err) })
	})
}

// Notify sends a fire-and-forget notification, queued the same way as
// Call if the client is not yet ready. Notifications have no response
// in the JSON-RPC/LSP protocol, so transport errors are not reported
// back to the caller.
func (c *Client) Notify(ctx context.Context, method string, params any) {
	c.whenReady(func() {
		c.rpc.Notify(ctx, method, params)
	})
}

// Shutdown sends the shutdown request followed by the exit
// notification per the LSP lifecycle, closes the transport, reaps the
// child process (if any), and marks the client stopped. onDone runs on
// the main goroutine (via Events).
func (c *Client) Shutdown(onDone func(error)) {
	c.mu.Lock()
	if c.state == stateStopped || c.state == stateShuttingDown {
		c.mu.Unlock()
		post(func() { onDone(nil) })
		return
	}
	c.state = stateShuttingDown
	c.mu.Unlock()

	go func() {
		ctx := context.Background()
		_, err := c.rpc.Call(ctx, "shutdown", nil)
		if err == nil {
			err = c.rpc.Notify(ctx, "exit", nil)
		}
		c.rpc.Close()
		if c.cmd != nil {
			c.cmd.Wait()
		}

		c.mu.Lock()
		c.state = stateStopped
		c.mu.Unlock()

		post(func() { onDone(err) })
	}()
}

// Capabilities returns the capabilities the server advertised during
// initialize. It is the zero value until Initialize completes.
func (c *Client) Capabilities() protocol.ServerCapabilities {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.capabilities
}

// PositionEncoding returns the position encoding negotiated during
// initialize, defaulting to UTF-16 (the LSP default) until Initialize
// completes.
func (c *Client) PositionEncoding() protocol.PositionEncodingKind {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.posEncoding == "" {
		return protocol.PositionEncodingUTF16
	}
	return c.posEncoding
}

// State returns a human-readable summary of the client's lifecycle
// state, for `> lsp status`.
func (c *Client) State() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch c.state {
	case stateStarting:
		return "starting"
	case stateInitializing:
		return "initializing"
	case stateReady:
		return "ready"
	case stateShuttingDown:
		return "shutting down"
	case stateStopped:
		return "stopped"
	default:
		return "unknown"
	}
}
