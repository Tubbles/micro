package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	"github.com/micro-editor/json5"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/lsp/protocol"
	rt "github.com/micro-editor/micro/v2/runtime"
)

// ServerDefinition describes how to launch a language server and which
// files it applies to. Definitions are loaded from the bundled
// runtime/lspservers.json and overlaid by ConfigDir/lspservers.json, so
// users can add or override servers without rebuilding micro.
type ServerDefinition struct {
	Command   string          `json:"command"`
	Args      []string        `json:"args,omitempty"`
	Roots     []string        `json:"roots,omitempty"`
	Filetypes []string        `json:"filetypes,omitempty"`
	Init      json.RawMessage `json:"init,omitempty"`
}

// registryKey identifies a running client by (server name, workspace
// root), the unit micro shares a Client for.
type registryKey struct {
	name string
	root string
}

// Registry holds the named server definitions and the live clients
// started from them, keyed by (server name, workspace root).
type Registry struct {
	definitions map[string]ServerDefinition

	mu      sync.Mutex
	clients map[registryKey]*Client

	// starter creates and returns an unstarted-but-connected Client for
	// (name, def, root). It defaults to spawning def.Command via
	// NewClient; tests replace it with a stub wired to a fake in-process
	// server so GetOrStart is testable without launching a real
	// language server.
	starter func(name string, def ServerDefinition, root string) (*Client, error)
}

// NewRegistry loads server definitions from the bundled
// runtime/lspservers.json, overlaid by ConfigDir/lspservers.json if it
// exists, and returns a Registry ready for Get/GetOrStart.
func NewRegistry() (*Registry, error) {
	defs, err := loadDefinitions()
	if err != nil {
		return nil, err
	}
	r := &Registry{
		definitions: defs,
		clients:     make(map[registryKey]*Client),
	}
	r.starter = func(name string, def ServerDefinition, root string) (*Client, error) {
		return NewClient(name, def, root)
	}
	return r, nil
}

func loadDefinitions() (map[string]ServerDefinition, error) {
	defs := make(map[string]ServerDefinition)

	bundled, err := rt.Asset("lspservers.json")
	if err != nil {
		return nil, fmt.Errorf("lsp: reading bundled lspservers.json: %w", err)
	}
	if err := json5.Unmarshal(bundled, &defs); err != nil {
		return nil, fmt.Errorf("lsp: parsing bundled lspservers.json: %w", err)
	}

	userPath := filepath.Join(config.ConfigDir, "lspservers.json")
	data, err := os.ReadFile(userPath)
	if err != nil {
		if os.IsNotExist(err) {
			return defs, nil
		}
		return nil, fmt.Errorf("lsp: reading %s: %w", userPath, err)
	}

	var overlay map[string]ServerDefinition
	if err := json5.Unmarshal(data, &overlay); err != nil {
		return nil, fmt.Errorf("lsp: parsing %s: %w", userPath, err)
	}
	for name, def := range overlay {
		defs[name] = def
	}

	return defs, nil
}

// DefinitionForFiletype returns the first server definition whose
// Filetypes list contains filetype, along with the name it is
// registered under. ok is false if no definition claims filetype. If
// more than one definition claims the same filetype, which one is
// returned is unspecified.
func (r *Registry) DefinitionForFiletype(filetype string) (name string, def ServerDefinition, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for n, d := range r.definitions {
		for _, ft := range d.Filetypes {
			if ft == filetype {
				return n, d, true
			}
		}
	}
	return "", ServerDefinition{}, false
}

// Get returns the already-running client for (name, root), if any.
func (r *Registry) Get(name, root string) (*Client, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.clients[registryKey{name, root}]
	return c, ok
}

// GetOrStart returns the running client for (name, root), starting and
// initializing one via r.starter if it doesn't exist yet. onReady runs
// on the main goroutine (via Events) with the client once it is ready
// (or already was), or with an error if starting or initializing
// failed.
func (r *Registry) GetOrStart(ctx context.Context, name, root string, onReady func(*Client, error)) {
	r.mu.Lock()
	def, known := r.definitions[name]
	if !known {
		r.mu.Unlock()
		post(func() { onReady(nil, fmt.Errorf("lsp: unknown server %q", name)) })
		return
	}
	if c, ok := r.clients[registryKey{name, root}]; ok {
		r.mu.Unlock()
		post(func() { onReady(c, nil) })
		return
	}
	r.mu.Unlock()

	go func() {
		c, err := r.starter(name, def, root)
		if err != nil {
			post(func() { onReady(nil, err) })
			return
		}

		r.mu.Lock()
		r.clients[registryKey{name, root}] = c
		r.mu.Unlock()

		c.Initialize(ctx, InitializeOptions{RootURI: fileURI(root)}, func(_ protocol.InitializeResult, err error) {
			onReady(c, err)
		})
	}()
}

// RootFor walks up from the directory containing file, looking for any
// of def's root markers (e.g. "go.mod", ".git"). It returns the first
// matching directory, or the current working directory if none is
// found or file is empty.
func RootFor(def ServerDefinition, file string) string {
	cwd, _ := os.Getwd()
	if file == "" {
		return cwd
	}

	dir := filepath.Dir(file)
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(cwd, dir)
	}

	for {
		for _, marker := range def.Roots {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return cwd
}

// fileURI converts a filesystem path to a "file://" DocumentURI,
// percent-encoding characters (such as spaces) that aren't valid
// unescaped in a URI path.
func fileURI(path string) protocol.DocumentURI {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	return protocol.DocumentURI(u.String())
}
