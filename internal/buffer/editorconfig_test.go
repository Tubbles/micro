package buffer

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/editorconfig/editorconfig-core-go/v2"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/stretchr/testify/assert"
)

func boolPtr(b bool) *bool { return &b }

// messagerFunc adapts a func to the buffer.Messager interface, for
// intercepting the messages warnEditorConfigOnce sends to prompt.
type messagerFunc func(msg ...any)

func (f messagerFunc) Message(msg ...any) { f(msg...) }

func writeFixtureFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// withEditorConfigEnabled points config.ConfigDir at a fresh settings.json
// (written via writeSettingsJSON, or absent) and turns the global
// `editorconfig` option on, restoring both plus parsedSettings afterward.
func withEditorConfigEnabled(t *testing.T) {
	t.Helper()

	oldConfigDir := config.ConfigDir
	oldEditorConfig := config.GlobalSettings["editorconfig"]

	config.ConfigDir = t.TempDir()
	config.GlobalSettings["editorconfig"] = true

	t.Cleanup(func() {
		config.ConfigDir = oldConfigDir
		config.GlobalSettings["editorconfig"] = oldEditorConfig
		// ReadSettings unconditionally resets parsedSettings to a fresh
		// empty map as its first step, flushing whatever this test wrote.
		config.ReadSettings()
	})
}

func writeSettingsJSON(t *testing.T, content string) {
	t.Helper()
	writeFixtureFile(t, filepath.Join(config.ConfigDir, "settings.json"), content)
	if err := config.ReadSettings(); err != nil {
		t.Fatal(err)
	}
}

// -- pure mapping tests --

func TestMapEditorConfigSettings(t *testing.T) {
	tests := []struct {
		name         string
		def          *editorconfig.Definition
		wantValues   map[string]any
		wantWarnings []string
	}{
		{
			name:       "all properties unset leaves everything untouched",
			def:        &editorconfig.Definition{Raw: map[string]string{}},
			wantValues: map[string]any{},
		},
		{
			name:       "indent_style space maps to tabstospaces true",
			def:        &editorconfig.Definition{IndentStyle: "space", Raw: map[string]string{}},
			wantValues: map[string]any{"tabstospaces": true},
		},
		{
			name:       "indent_style tab maps to tabstospaces false",
			def:        &editorconfig.Definition{IndentStyle: "tab", Raw: map[string]string{}},
			wantValues: map[string]any{"tabstospaces": false},
		},
		{
			name:       "numeric indent_size maps to tabsize",
			def:        &editorconfig.Definition{IndentSize: "2", TabWidth: 2, Raw: map[string]string{}},
			wantValues: map[string]any{"tabsize": float64(2)},
		},
		{
			name:       "indent_size=tab with tab_width set uses tab_width",
			def:        &editorconfig.Definition{IndentSize: "tab", TabWidth: 8, Raw: map[string]string{}},
			wantValues: map[string]any{"tabsize": float64(8)},
		},
		{
			name:       "indent_size=tab with tab_width unset leaves tabsize untouched",
			def:        &editorconfig.Definition{IndentSize: "tab", TabWidth: 0, Raw: map[string]string{}},
			wantValues: map[string]any{},
		},
		{
			name:       "tab_width alone (indent_size unset) still maps to tabsize",
			def:        &editorconfig.Definition{TabWidth: 4, Raw: map[string]string{}},
			wantValues: map[string]any{"tabsize": float64(4)},
		},
		{
			name:       "end_of_line lf maps to fileformat unix",
			def:        &editorconfig.Definition{EndOfLine: "lf", Raw: map[string]string{}},
			wantValues: map[string]any{"fileformat": "unix"},
		},
		{
			name:       "end_of_line crlf maps to fileformat dos",
			def:        &editorconfig.Definition{EndOfLine: "crlf", Raw: map[string]string{}},
			wantValues: map[string]any{"fileformat": "dos"},
		},
		{
			name:         "end_of_line cr is skipped with a warning",
			def:          &editorconfig.Definition{EndOfLine: "cr", Raw: map[string]string{}},
			wantValues:   map[string]any{},
			wantWarnings: []string{"editorconfig: end_of_line = cr is not supported, skipping"},
		},
		{
			name:       "charset utf-8 maps to encoding utf-8",
			def:        &editorconfig.Definition{Charset: "utf-8", Raw: map[string]string{}},
			wantValues: map[string]any{"encoding": "utf-8"},
		},
		{
			name:       "charset latin1 maps to encoding windows-1252",
			def:        &editorconfig.Definition{Charset: "latin1", Raw: map[string]string{}},
			wantValues: map[string]any{"encoding": "windows-1252"},
		},
		{
			name:         "charset utf-8-bom is skipped with a warning",
			def:          &editorconfig.Definition{Charset: "utf-8-bom", Raw: map[string]string{}},
			wantValues:   map[string]any{},
			wantWarnings: []string{"editorconfig: charset = utf-8-bom is not supported (micro has no BOM handling), skipping"},
		},
		{
			name:         "unknown charset is skipped with a warning",
			def:          &editorconfig.Definition{Charset: "bogus", Raw: map[string]string{}},
			wantValues:   map[string]any{},
			wantWarnings: []string{"editorconfig: unknown charset \"bogus\", skipping"},
		},
		{
			name:       "trim_trailing_whitespace true maps to rmtrailingws",
			def:        &editorconfig.Definition{TrimTrailingWhitespace: boolPtr(true), Raw: map[string]string{}},
			wantValues: map[string]any{"rmtrailingws": true},
		},
		{
			name:       "insert_final_newline false maps to eofnewline",
			def:        &editorconfig.Definition{InsertFinalNewline: boolPtr(false), Raw: map[string]string{}},
			wantValues: map[string]any{"eofnewline": false},
		},
		{
			name:       "numeric max_line_length maps to colorcolumn",
			def:        &editorconfig.Definition{Raw: map[string]string{"max_line_length": "80"}},
			wantValues: map[string]any{"colorcolumn": float64(80)},
		},
		{
			name:       "unset max_line_length leaves colorcolumn untouched",
			def:        &editorconfig.Definition{Raw: map[string]string{}},
			wantValues: map[string]any{},
		},
		{
			name:       "non-numeric max_line_length leaves colorcolumn untouched",
			def:        &editorconfig.Definition{Raw: map[string]string{"max_line_length": "off"}},
			wantValues: map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values, warnings := mapEditorConfigSettings(tt.def)
			assert.Equal(t, tt.wantValues, values)
			assert.Equal(t, tt.wantWarnings, warnings)
		})
	}
}

func TestWarnEditorConfigOnceDedupes(t *testing.T) {
	oldWarned := editorConfigWarned
	editorConfigWarned = make(map[string]bool)
	oldPrompt := prompt
	defer func() {
		editorConfigWarned = oldWarned
		prompt = oldPrompt
	}()

	var messages []string
	prompt = messagerFunc(func(msg ...any) {
		messages = append(messages, fmt.Sprint(msg...))
	})

	warnEditorConfigOnce("hello")
	warnEditorConfigOnce("hello")
	warnEditorConfigOnce("world")

	assert.Equal(t, []string{"hello", "world"}, messages)
}

// -- resolveEditorConfig / fixture tree tests --

func TestResolveEditorConfigEmptyPath(t *testing.T) {
	def, err := resolveEditorConfig("")
	assert.NoError(t, err)
	assert.Nil(t, def)
}

func TestResolveEditorConfigBasicResolution(t *testing.T) {
	dir := t.TempDir()
	writeFixtureFile(t, filepath.Join(dir, ".editorconfig"),
		"root = true\n[*.go]\nindent_style = tab\nindent_size = 4\n")
	target := filepath.Join(dir, "main.go")
	writeFixtureFile(t, target, "package main\n")

	def, err := resolveEditorConfig(target)
	assert.NoError(t, err)
	assert.Equal(t, "tab", def.IndentStyle)
	assert.Equal(t, "4", def.IndentSize)
	// the library defaults tab_width from a numeric indent_size
	assert.Equal(t, 4, def.TabWidth)
}

func TestResolveEditorConfigRootStopsUpwardDiscovery(t *testing.T) {
	outer := t.TempDir()
	writeFixtureFile(t, filepath.Join(outer, ".editorconfig"),
		"[*.go]\nmax_line_length = 120\n")

	inner := filepath.Join(outer, "project")
	writeFixtureFile(t, filepath.Join(inner, ".editorconfig"),
		"root = true\n[*.go]\nindent_size = 2\n")
	target := filepath.Join(inner, "main.go")
	writeFixtureFile(t, target, "package main\n")

	def, err := resolveEditorConfig(target)
	assert.NoError(t, err)
	assert.Equal(t, "2", def.IndentSize)
	_, hasMaxLineLength := def.Raw["max_line_length"]
	assert.False(t, hasMaxLineLength,
		"root = true in the inner .editorconfig must stop upward discovery")
}

// -- applyEditorConfig precedence tests, without going through NewBuffer --

func TestApplyEditorConfigOverridesOpenHookValue(t *testing.T) {
	b := NewBufferFromString("package main\n", "", BTDefault)
	defer b.Close()

	// Simulate an onBufferOpen plugin such as ftoptions calling
	// b:SetOption("tabstospaces", "off"): that Lua call goes through the
	// same b.SetOptionNative path used here, marking the flag local.
	if err := b.SetOptionNative("tabstospaces", false); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, true, b.LocalSettings["tabstospaces"])

	def := &editorconfig.Definition{IndentStyle: "space", Raw: map[string]string{}}
	applyEditorConfig(b, def, true)

	assert.Equal(t, true, b.Settings["tabstospaces"],
		"editorconfig should outrank the open-hook-set local value")
	_, stillLocal := b.LocalSettings["tabstospaces"]
	assert.False(t, stillLocal, "editorconfig must clear the LocalSettings flag it overrides")
}

func TestApplyEditorConfigRespectsLocalWhenNotOverriding(t *testing.T) {
	b := NewBufferFromString("package main\n", "", BTDefault)
	defer b.Close()

	if err := b.SetOptionNative("tabstospaces", false); err != nil {
		t.Fatal(err)
	}

	def := &editorconfig.Definition{IndentStyle: "space", Raw: map[string]string{}}
	applyEditorConfig(b, def, false)

	assert.Equal(t, false, b.Settings["tabstospaces"],
		"a local value must not be touched when overrideLocal is false")
	assert.Equal(t, true, b.LocalSettings["tabstospaces"])
}

func TestApplyEditorConfigNilDefinitionIsNoop(t *testing.T) {
	b := NewBufferFromString("package main\n", "", BTDefault)
	defer b.Close()

	before := b.Settings["tabstospaces"]
	applyEditorConfig(b, nil, true)
	assert.Equal(t, before, b.Settings["tabstospaces"])

	applyEditorConfigEncoding(b, nil)
	assert.Equal(t, "utf-8", b.Settings["encoding"])
}

// -- end-to-end tests through NewBufferFromFile / ReloadSettings --

func TestNewBufferFromFileEditorConfigBeatsFtOverlay(t *testing.T) {
	withEditorConfigEnabled(t)
	writeSettingsJSON(t, `{"ft:go": {"tabstospaces": false}}`)

	dir := t.TempDir()
	writeFixtureFile(t, filepath.Join(dir, ".editorconfig"),
		"root = true\n[*.go]\nindent_style = space\n")
	target := filepath.Join(dir, "main.go")
	writeFixtureFile(t, target, "package main\n")

	b, err := NewBufferFromFile(target, BTDefault)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	assert.Equal(t, "go", b.Settings["filetype"])
	assert.Equal(t, true, b.Settings["tabstospaces"],
		"editorconfig's indent_style = space should win over the ft:go overlay's tabstospaces = false")
}

func TestNewBufferFromFileEditorConfigEncodingAppliesBeforeRead(t *testing.T) {
	withEditorConfigEnabled(t)

	dir := t.TempDir()
	writeFixtureFile(t, filepath.Join(dir, ".editorconfig"),
		"root = true\n[*.txt]\ncharset = latin1\n")
	target := filepath.Join(dir, "sample.txt")
	// 'é' encoded as windows-1252/latin1 is the single byte 0xE9. Decoded
	// as UTF-8 instead, a lone 0xE9 is an invalid lead byte and would
	// become U+FFFD, so a correct decode proves charset was applied
	// before the file was read, not after.
	if err := os.WriteFile(target, []byte("caf\xe9\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	b, err := NewBufferFromFile(target, BTDefault)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	assert.Equal(t, "windows-1252", b.Settings["encoding"])
	assert.Equal(t, "café", b.Line(0))
}

func TestReloadSettingsEditorConfigYieldsToSetlocal(t *testing.T) {
	withEditorConfigEnabled(t)

	dir := t.TempDir()
	writeFixtureFile(t, filepath.Join(dir, ".editorconfig"),
		"root = true\n[*.go]\nindent_style = space\n")
	target := filepath.Join(dir, "main.go")
	writeFixtureFile(t, target, "package main\n")

	b, err := NewBufferFromFile(target, BTDefault)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	assert.Equal(t, true, b.Settings["tabstospaces"], "editorconfig should apply at open")

	// simulate an interactive `setlocal tabstospaces off`
	if err := b.SetOptionNative("tabstospaces", false); err != nil {
		t.Fatal(err)
	}

	b.ReloadSettings(true)

	assert.Equal(t, false, b.Settings["tabstospaces"],
		"a subsequent setlocal must keep winning across ReloadSettings, even though editorconfig still says space")
}
