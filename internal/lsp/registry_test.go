package lsp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

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
