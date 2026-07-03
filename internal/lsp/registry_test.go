package lsp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/creachadair/jrpc2/handler"

	"github.com/micro-editor/micro/v2/internal/config"
)

// withTempConfigDir points config.ConfigDir at a fresh temp directory
// for the duration of the test, restoring the previous value on
// cleanup. loadDefinitions and NewRegistry read config.ConfigDir to
// find the user's lspservers.json overlay.
func withTempConfigDir(t *testing.T) string {
	t.Helper()
	old := config.ConfigDir
	dir := t.TempDir()
	config.ConfigDir = dir
	t.Cleanup(func() { config.ConfigDir = old })
	return dir
}

func TestLoadDefinitionsBundledOnly(t *testing.T) {
	withTempConfigDir(t)

	defs, err := loadDefinitions()
	if err != nil {
		t.Fatalf("loadDefinitions: %v", err)
	}

	goDef, ok := defs["go"]
	if !ok {
		t.Fatal(`bundled runtime/lspservers.json is missing a "go" entry`)
	}
	if goDef.Command != "gopls" {
		t.Errorf(`bundled "go" server command = %q, want "gopls"`, goDef.Command)
	}
}

func TestLoadDefinitionsUserOverlayMergesAndOverrides(t *testing.T) {
	dir := withTempConfigDir(t)

	overlay := `{
		"go": {"command": "my-custom-gopls", "args": ["-v"], "roots": ["go.mod"], "filetypes": ["go"]},
		"python": {"command": "pyright-langserver", "args": ["--stdio"], "roots": ["pyproject.toml"], "filetypes": ["python"]}
	}`
	if err := os.WriteFile(filepath.Join(dir, "lspservers.json"), []byte(overlay), 0644); err != nil {
		t.Fatal(err)
	}

	defs, err := loadDefinitions()
	if err != nil {
		t.Fatalf("loadDefinitions: %v", err)
	}

	if got := defs["go"].Command; got != "my-custom-gopls" {
		t.Errorf(`overlay did not override "go" server command: got %q`, got)
	}

	py, ok := defs["python"]
	if !ok {
		t.Fatal("overlay did not add a new \"python\" server definition")
	}
	if py.Command != "pyright-langserver" {
		t.Errorf(`"python" server command = %q, want "pyright-langserver"`, py.Command)
	}
}

func TestRootForWalksUpToMarker(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(sub, "main.go")

	def := ServerDefinition{Roots: []string{"go.mod", ".git"}}
	got := RootFor(def, file)

	wantResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	gotResolved, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatal(err)
	}
	if gotResolved != wantResolved {
		t.Errorf("RootFor = %q, want %q", got, root)
	}
}

func TestRootForFallsBackToCwdWhenNoMarkerFound(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "orphan.go")
	def := ServerDefinition{Roots: []string{"go.mod", ".git"}}

	got := RootFor(def, file)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cwdResolved, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		t.Fatal(err)
	}
	gotResolved, err := filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatal(err)
	}
	if gotResolved != cwdResolved {
		t.Errorf("RootFor fallback = %q, want cwd %q", got, cwd)
	}
}

// TestRegistryGetOrStartUsesStarterAndCachesClient exercises GetOrStart
// through the starter stub (see Registry.starter's doc comment), so it
// doesn't need to launch a real language server: the stub hands back a
// Client wired to a fake in-process server, the same trick client_test.go
// uses for lifecycle tests.
func TestRegistryGetOrStartUsesStarterAndCachesClient(t *testing.T) {
	withTempConfigDir(t)

	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	handlers, initializedCh := lifecycleHandlers(nil)
	ch := newFakeServer(t, handlers)

	var startedName, startedRoot string
	r.starter = func(name string, def ServerDefinition, root string) (*Client, error) {
		startedName, startedRoot = name, root
		return newClientOverChannel(name, root, ch), nil
	}

	root := t.TempDir()
	done := make(chan struct{})
	var gotClient *Client
	var gotErr error
	r.GetOrStart(context.Background(), "go", root, func(c *Client, err error) {
		gotClient, gotErr = c, err
		close(done)
	})

	drainEvents(t, 2*time.Second)
	<-done
	waitForSignal(t, initializedCh, "initialized")

	if gotErr != nil {
		t.Fatalf("GetOrStart: %v", gotErr)
	}
	if gotClient == nil {
		t.Fatal("GetOrStart returned a nil client with no error")
	}
	if startedName != "go" || startedRoot != root {
		t.Errorf("starter called with (%q, %q), want (%q, %q)", startedName, startedRoot, "go", root)
	}

	got, ok := r.Get("go", root)
	if !ok || got != gotClient {
		t.Errorf("Get(%q, %q) = (%v, %v), want (%v, true)", "go", root, got, ok, gotClient)
	}
}

func TestRegistryStopShutsDownAndForgetsClient(t *testing.T) {
	withTempConfigDir(t)

	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

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
	r.starter = func(name string, def ServerDefinition, root string) (*Client, error) {
		return newClientOverChannel(name, root, ch), nil
	}

	root := t.TempDir()
	started := make(chan struct{})
	r.GetOrStart(context.Background(), "go", root, func(_ *Client, err error) {
		if err != nil {
			t.Errorf("GetOrStart: %v", err)
		}
		close(started)
	})
	drainEvents(t, 2*time.Second)
	<-started
	waitForSignal(t, initializedCh, "initialized")

	stopDone := make(chan error, 1)
	r.Stop("go", root, func(err error) { stopDone <- err })
	drainEvents(t, 2*time.Second)

	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatalf("Stop callback error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stop never completed")
	}
	waitForSignal(t, shutdownCalled, "shutdown")
	waitForSignal(t, exitCalled, "exit")

	if _, ok := r.Get("go", root); ok {
		t.Error("Get after Stop still finds a client, want it removed from the registry")
	}
}

func TestRegistryStopOnNothingRunningReportsNilError(t *testing.T) {
	withTempConfigDir(t)

	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	done := make(chan struct{})
	var gotErr error
	r.Stop("go", "/nowhere", func(err error) {
		gotErr = err
		close(done)
	})
	drainEvents(t, 2*time.Second)
	<-done

	if gotErr != nil {
		t.Errorf("Stop on nothing running: got error %v, want nil", gotErr)
	}
}

// TestRegistryRestartStopsThenStartsANewClient exercises `> lsp
// restart`'s registry-level mechanics: Restart must shut down the
// running client for (name, root) before invoking the starter again,
// and must report a client distinct from the one that was stopped
// (the same fake-starter substitution seam GetOrStart's own tests
// use, so no real language server is launched).
func TestRegistryRestartStopsThenStartsANewClient(t *testing.T) {
	withTempConfigDir(t)

	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	handlers, initializedCh := lifecycleHandlers(handler.Map{
		"shutdown": handler.New(func(_ context.Context) error { return nil }),
		"exit":     handler.New(func(_ context.Context) error { return nil }),
	})

	var startedName, startedRoot string
	starterCalls := 0
	r.starter = func(name string, def ServerDefinition, root string) (*Client, error) {
		starterCalls++
		startedName, startedRoot = name, root
		return newClientOverChannel(name, root, newFakeServer(t, handlers)), nil
	}

	root := t.TempDir()
	firstStarted := make(chan struct{})
	var firstClient *Client
	r.GetOrStart(context.Background(), "go", root, func(c *Client, err error) {
		if err != nil {
			t.Errorf("initial GetOrStart: %v", err)
		}
		firstClient = c
		close(firstStarted)
	})
	drainEvents(t, 2*time.Second)
	<-firstStarted
	waitForSignal(t, initializedCh, "initialized")

	if starterCalls != 1 {
		t.Fatalf("starter calls before Restart = %d, want 1", starterCalls)
	}

	restartDone := make(chan struct{})
	var restartedClient *Client
	var restartErr error
	r.Restart(context.Background(), "go", root, func(c *Client, err error) {
		restartedClient, restartErr = c, err
		close(restartDone)
	})

	drainEvents(t, 2*time.Second) // Stop's onDone, which calls GetOrStart
	waitForSignal(t, initializedCh, "initialized")
	drainEvents(t, 2*time.Second) // the new client's Initialize onDone
	<-restartDone

	if restartErr != nil {
		t.Fatalf("Restart: %v", restartErr)
	}
	if starterCalls != 2 {
		t.Fatalf("starter calls after Restart = %d, want 2", starterCalls)
	}
	if startedName != "go" || startedRoot != root {
		t.Errorf("starter called with (%q, %q), want (%q, %q)", startedName, startedRoot, "go", root)
	}
	if restartedClient == firstClient {
		t.Error("Restart returned the same client that was stopped, want a fresh one")
	}

	got, ok := r.Get("go", root)
	if !ok || got != restartedClient {
		t.Errorf("Get(%q, %q) = (%v, %v), want (%v, true)", "go", root, got, ok, restartedClient)
	}
}

func TestRegistryGetOrStartUnknownServerReportsError(t *testing.T) {
	withTempConfigDir(t)

	r, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	done := make(chan struct{})
	var gotErr error
	r.GetOrStart(context.Background(), "nonexistent", "/root", func(_ *Client, err error) {
		gotErr = err
		close(done)
	})

	drainEvents(t, 2*time.Second)
	<-done

	if gotErr == nil {
		t.Fatal("want an error for an unknown server name, got nil")
	}
}
