package lsp

import (
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestLspAttachArgs(t *testing.T) {
	args := lspAttachArgs("/fake/main.go", "gopls")
	if len(args) != 2 {
		t.Fatalf("len(args) = %d, want 2", len(args))
	}
	if got, ok := args[0].(lua.LString); !ok || string(got) != "/fake/main.go" {
		t.Errorf("args[0] = %#v, want LString(/fake/main.go)", args[0])
	}
	if got, ok := args[1].(lua.LString); !ok || string(got) != "gopls" {
		t.Errorf("args[1] = %#v, want LString(gopls)", args[1])
	}
}

func TestLspDetachArgs(t *testing.T) {
	args := lspDetachArgs("/fake/main.go", "gopls")
	if len(args) != 2 {
		t.Fatalf("len(args) = %d, want 2", len(args))
	}
	if got, ok := args[0].(lua.LString); !ok || string(got) != "/fake/main.go" {
		t.Errorf("args[0] = %#v, want LString(/fake/main.go)", args[0])
	}
	if got, ok := args[1].(lua.LString); !ok || string(got) != "gopls" {
		t.Errorf("args[1] = %#v, want LString(gopls)", args[1])
	}
}

func TestOnDiagnosticsArgs(t *testing.T) {
	args := onDiagnosticsArgs("/fake/main.go", 3)
	if len(args) != 2 {
		t.Fatalf("len(args) = %d, want 2", len(args))
	}
	if got, ok := args[0].(lua.LString); !ok || string(got) != "/fake/main.go" {
		t.Errorf("args[0] = %#v, want LString(/fake/main.go)", args[0])
	}
	if got, ok := args[1].(lua.LNumber); !ok || float64(got) != 3 {
		t.Errorf("args[1] = %#v, want LNumber(3)", args[1])
	}
}
