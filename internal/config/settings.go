package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"

	"github.com/micro-editor/json5"
	"github.com/micro-editor/micro/v2/internal/util"
	"github.com/zyedidia/glob"
	"golang.org/x/text/encoding/htmlindex"
)

type optionValidator func(string, any) error

// a list of settings that need option validators
var optionValidators = map[string]optionValidator{
	"autosave":                   validateNonNegativeValue,
	"clipboard":                  validateChoice,
	"colorcolumn":                validateNonNegativeValue,
	"colorscheme":                validateColorscheme,
	"commandpalette.historysize": validateNonNegativeValue,
	"detectlimit":                validateNonNegativeValue,
	"encoding":                   validateEncoding,
	"fileformat":                 validateChoice,
	"helpsplit":                  validateChoice,
	"matchbracestyle":            validateChoice,
	"multiopen":                  validateChoice,
	"pageoverlap":                validateNonNegativeValue,
	"reload":                     validateChoice,
	"scrollmargin":               validateNonNegativeValue,
	"scrollspeed":                validateNonNegativeValue,
	"tabsize":                    validatePositiveValue,
	"truecolor":                  validateChoice,
}

// a list of settings with pre-defined choices
var OptionChoices = map[string][]string{
	"clipboard":       {"internal", "external", "terminal"},
	"fileformat":      {"unix", "dos"},
	"helpsplit":       {"hsplit", "vsplit"},
	"matchbracestyle": {"underline", "highlight"},
	"multiopen":       {"tab", "hsplit", "vsplit"},
	"reload":          {"prompt", "auto", "disabled"},
	"truecolor":       {"auto", "on", "off"},
}

// a list of settings that can be globally and locally modified and their
// default values
var defaultCommonSettings = map[string]any{
	"autoindent":              true,
	"autosu":                  false,
	"backup":                  true,
	"backupdir":               "",
	"basename":                false,
	"colorcolumn":             float64(0),
	"cursorline":              true,
	"detectlimit":             float64(100),
	"diffgutter":              false,
	"encoding":                "utf-8",
	"eofnewline":              true,
	"fastdirty":               false,
	"fileformat":              defaultFileFormat(),
	"filemanager.showhidden":  false,
	"filemanager.showignored": false,
	"filetype":                "unknown",
	"hlsearch":                false,
	"hlselection":             false,
	"hltaberrors":             false,
	"hltrailingws":            false,
	"ignorecase":              true,
	"incsearch":               true,
	"indentchar":              " ", // Deprecated
	"keepautoindent":          false,
	"matchbrace":              true,
	"matchbraceleft":          true,
	"matchbracestyle":         "underline",
	"mkparents":               false,
	"pageoverlap":             float64(2),
	"permbackup":              false,
	"readonly":                false,
	"relativeruler":           false,
	"reload":                  "prompt",
	"rmtrailingws":            false,
	"ruler":                   true,
	"savecursor":              false,
	"saveundo":                false,
	"scrollbar":               false,
	"scrollmargin":            float64(3),
	"scrollspeed":             float64(2),
	"showchars":               "",
	"smartpaste":              true,
	"softwrap":                false,
	"splitbottom":             true,
	"splitright":              true,
	"statusformatl":           "$(filename) $(modified)$(overwrite)($(line),$(col)) $(status.paste)| ft:$(opt:filetype) | $(opt:fileformat) | $(opt:encoding)",
	"statusformatr":           "$(bind:ToggleKeyMenu): bindings, $(bind:ToggleHelp): help",
	"statusline":              true,
	"syntax":                  true,
	"tabmovement":             false,
	"tabsize":                 float64(4),
	"tabstospaces":            false,
	"truecolor":               "auto",
	"useprimary":              true,
	"wordwrap":                false,
}

// a list of settings that should only be globally modified and their
// default values
var DefaultGlobalOnlySettings = map[string]any{
	"autosave":                   float64(0),
	"clipboard":                  "external",
	"colorscheme":                "default",
	"commandpalette.actions":     true,
	"commandpalette.commands":    true,
	"commandpalette.historysize": float64(20),
	"commandpalette.lua":         true,
	"divchars":                   "|-",
	"divreverse":                 true,
	"fakecursor":                 defaultFakeCursor(),
	"helpsplit":                  "hsplit",
	"infobar":                    true,
	"keymenu":                    false,
	"lockbindings":               false,
	"mouse":                      true,
	"multiopen":                  "tab",
	"parsecursor":                false,
	"paste":                      false,
	"pluginchannels":             []string{"https://raw.githubusercontent.com/micro-editor/plugin-channel/master/channel.json"},
	"pluginrepos":                []string{},
	"quittoscratch":              false,
	"savehistory":                true,
	"scrollbarchar":              "|",
	"sucmd":                      "sudo",
	"tabhighlight":               false,
	"tabreverse":                 true,
	"xterm":                      false,
}

// a list of settings that should never be globally modified
var LocalSettings = []string{
	"filetype",
	"readonly",
}

var (
	ErrInvalidOption    = errors.New("Invalid option")
	ErrInvalidValue     = errors.New("Invalid value")
	ErrOptNotToggleable = errors.New("Option not toggleable")

	// The options that the user can set
	GlobalSettings map[string]any

	// This is the raw parsed json. parsedSettings is the in-memory mirror of
	// settings.json: :set writes directly into it, WriteSettings serializes it
	// to disk verbatim.
	parsedSettings     map[string]any
	settingsParseError bool

	// parsedLocalSettings is the in-memory view of settings.local.json.
	// It is read-only at runtime: the editor never writes to it; users edit
	// the file by hand. Values here override parsedSettings during rebuild
	// of GlobalSettings, providing a per-machine override layer for a
	// settings.json that the user wants to keep in version control.
	parsedLocalSettings     map[string]any
	localSettingsParseError bool

	// VolatileSettings is a map of settings which should not be written to disk
	// because they have been temporarily set for this session only
	VolatileSettings map[string]bool
)

func writeFile(name string, txt []byte) error {
	return util.SafeWrite(name, txt, false)
}

func init() {
	VolatileSettings = make(map[string]bool)
}

// validateParsedSettings normalises and validates a parsed settings map
// in place. It is used for both the global parsedSettings (settings.json)
// and the per-machine parsedLocalSettings (settings.local.json); the rules
// are identical for both files.
func validateParsedSettings(m map[string]any) error {
	var err error
	defaults := DefaultAllSettings()
	for k, v := range m {
		if strings.HasPrefix(reflect.TypeOf(v).String(), "map") {
			if strings.HasPrefix(k, "ft:") {
				for k1, v1 := range v.(map[string]any) {
					if _, ok := defaults[k1]; ok {
						if e := verifySetting(k1, v1, defaults[k1]); e != nil {
							err = e
							m[k].(map[string]any)[k1] = defaults[k1]
							continue
						}
					}
				}
			} else {
				tk := strings.TrimPrefix(k, "glob:")
				if _, e := glob.Compile(tk); e != nil {
					err = errors.New("Error with glob setting " + tk + ": " + e.Error())
					delete(m, k)
					continue
				}
				if !strings.HasPrefix(k, "glob:") {
					// Support non-prefixed glob settings but internally convert
					// them to prefixed ones for simplicity.
					delete(m, k)
					k = "glob:" + k
					m[k] = v
				}
				for k1, v1 := range v.(map[string]any) {
					if _, ok := defaults[k1]; ok {
						if e := verifySetting(k1, v1, defaults[k1]); e != nil {
							err = e
							m[k].(map[string]any)[k1] = defaults[k1]
							continue
						}
					}
				}
			}
			continue
		}

		if k == "autosave" {
			// if autosave is a boolean convert it to float
			s, ok := v.(bool)
			if ok {
				if s {
					m["autosave"] = 8.0
				} else {
					m["autosave"] = 0.0
				}
			}
			continue
		}

		if _, ok := defaults[k]; ok {
			if e := verifySetting(k, v, defaults[k]); e != nil {
				err = e
				m[k] = defaults[k]
				continue
			}
		}
	}
	return err
}

func ReadSettings() error {
	parsedSettings = make(map[string]any)
	filename := filepath.Join(ConfigDir, "settings.json")
	if _, e := os.Stat(filename); e == nil {
		input, err := os.ReadFile(filename)
		if err != nil {
			settingsParseError = true
			return errors.New("Error reading settings.json file: " + err.Error())
		}
		if !strings.HasPrefix(string(input), "null") {
			// Unmarshal the input into the parsed map
			err = json5.Unmarshal(input, &parsedSettings)
			if err != nil {
				settingsParseError = true
				return errors.New("Error reading settings.json: " + err.Error())
			}
			err = validateParsedSettings(parsedSettings)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// ReadLocalSettings populates parsedLocalSettings from settings.local.json
// in ConfigDir. A missing file is not an error. The local file is an
// optional per-machine override layer for settings.json: any scalar key
// present here wins over the same key in settings.json, and ft:/glob:
// nested maps deep-merge on a per-key basis. The editor never writes to
// settings.local.json; users edit it by hand.
func ReadLocalSettings() error {
	parsedLocalSettings = make(map[string]any)
	filename := filepath.Join(ConfigDir, "settings.local.json")
	if _, e := os.Stat(filename); e != nil {
		// Missing local file is the common case; not an error.
		return nil
	}
	input, err := os.ReadFile(filename)
	if err != nil {
		localSettingsParseError = true
		return errors.New("Error reading settings.local.json file: " + err.Error())
	}
	if strings.HasPrefix(string(input), "null") {
		return nil
	}
	err = json5.Unmarshal(input, &parsedLocalSettings)
	if err != nil {
		localSettingsParseError = true
		return errors.New("Error reading settings.local.json: " + err.Error())
	}
	return validateParsedSettings(parsedLocalSettings)
}

// ParsedSettings returns the effective scalar view of the user's
// configuration: keys in parsedSettings overlaid by keys in
// parsedLocalSettings. ft:/glob: nested maps are excluded because they
// apply per-buffer via UpdateFileTypeLocals / UpdatePathGlobLocals.
// Used by the :reload settings path so it picks up local overrides.
func ParsedSettings() map[string]any {
	s := make(map[string]any)
	for k, v := range parsedSettings {
		if strings.HasPrefix(reflect.TypeOf(v).String(), "map") {
			continue
		}
		s[k] = v
	}
	for k, v := range parsedLocalSettings {
		if strings.HasPrefix(reflect.TypeOf(v).String(), "map") {
			continue
		}
		s[k] = v
	}
	return s
}

func verifySetting(option string, value any, def any) error {
	var interfaceArr []any
	valType := reflect.TypeOf(value)
	defType := reflect.TypeOf(def)
	assignable := false

	switch option {
	case "pluginrepos", "pluginchannels":
		assignable = valType.AssignableTo(reflect.TypeOf(interfaceArr))
	default:
		assignable = defType.AssignableTo(valType)
	}
	if !assignable {
		return fmt.Errorf("Error: setting '%s' has incorrect type (%s), using default value: %v (%s)", option, valType, def, defType)
	}

	if option == "colorscheme" {
		// Plugins are not initialized yet, so do not verify if the colorscheme
		// exists yet, since the colorscheme may be added by a plugin later.
		return nil
	}

	if err := OptionIsValid(option, value); err != nil {
		return err
	}

	return nil
}

// InitGlobalSettings initialises the GlobalSettings map.
// Must be called after ReadSettings and ReadLocalSettings.
func InitGlobalSettings() error {
	GlobalSettings = make(map[string]any)
	RebuildGlobalSettings()
	return nil
}

// RebuildGlobalSettings recomputes GlobalSettings from the source layers:
// defaults are laid down first, then parsedSettings (settings.json) overlays
// them, then parsedLocalSettings (settings.local.json) overlays again. Keys
// flagged in VolatileSettings (typically set from command-line flags) keep
// their current GlobalSettings value across the rebuild so transient
// overrides survive reloads.
//
// Only scalar (non-map) values participate in the global rebuild. The
// ft:<filetype> and glob:<pattern> nested maps from either source apply
// per-buffer via UpdateFileTypeLocals / UpdatePathGlobLocals, not here.
func RebuildGlobalSettings() {
	// Preserve volatile values across the reset.
	volatileValues := make(map[string]any, len(VolatileSettings))
	for k := range VolatileSettings {
		if v, ok := GlobalSettings[k]; ok {
			volatileValues[k] = v
		}
	}

	GlobalSettings = DefaultAllSettings()

	for k, v := range parsedSettings {
		if strings.HasPrefix(reflect.TypeOf(v).String(), "map") {
			continue
		}
		GlobalSettings[k] = v
	}
	for k, v := range parsedLocalSettings {
		if strings.HasPrefix(reflect.TypeOf(v).String(), "map") {
			continue
		}
		GlobalSettings[k] = v
	}

	// Restore volatile values last so they outrank everything from disk.
	for k, v := range volatileValues {
		GlobalSettings[k] = v
	}
}

// applyPathGlobLocals overlays any glob:<pattern> nested maps in src
// whose pattern matches path into settings.
func applyPathGlobLocals(src map[string]any, settings map[string]any, path string) {
	for k, v := range src {
		if !strings.HasPrefix(reflect.TypeOf(v).String(), "map") || !strings.HasPrefix(k, "glob:") {
			continue
		}
		tk := strings.TrimPrefix(k, "glob:")
		g, _ := glob.Compile(tk)
		if g.MatchString(path) {
			for k1, v1 := range v.(map[string]any) {
				settings[k1] = v1
			}
		}
	}
}

// applyFileTypeLocals overlays any ft:<filetype> nested map in src
// matching filetype into settings.
func applyFileTypeLocals(src map[string]any, settings map[string]any, filetype string) {
	for k, v := range src {
		if !strings.HasPrefix(reflect.TypeOf(v).String(), "map") || !strings.HasPrefix(k, "ft:") {
			continue
		}
		if filetype != k[3:] {
			continue
		}
		for k1, v1 := range v.(map[string]any) {
			if k1 != "filetype" {
				settings[k1] = v1
			}
		}
	}
}

// UpdatePathGlobLocals applies glob:<pattern> nested maps from settings.json
// first, then from settings.local.json, so local values win on key collision
// while non-colliding settings.json keys persist (per-key deep merge).
// Must be called after ReadSettings and ReadLocalSettings.
func UpdatePathGlobLocals(settings map[string]any, path string) {
	applyPathGlobLocals(parsedSettings, settings, path)
	applyPathGlobLocals(parsedLocalSettings, settings, path)
}

// UpdateFileTypeLocals applies ft:<filetype> nested maps from settings.json
// first, then from settings.local.json, so local values win on key collision
// while non-colliding settings.json keys persist (per-key deep merge).
// Must be called after ReadSettings and ReadLocalSettings.
func UpdateFileTypeLocals(settings map[string]any, filetype string) {
	applyFileTypeLocals(parsedSettings, settings, filetype)
	applyFileTypeLocals(parsedLocalSettings, settings, filetype)
}

// UpdateParsedSetting records a user-driven :set in parsedSettings.
// If value equals the default for option, the key is removed (so it won't
// be serialized into settings.json); otherwise the key is set to value.
// This keeps parsedSettings as the authoritative mirror of settings.json
// so WriteSettings can remain a plain serializer.
func UpdateParsedSetting(option string, value any) {
	defaults := DefaultAllSettings()
	if def, ok := defaults[option]; ok && reflect.DeepEqual(value, def) {
		delete(parsedSettings, option)
	} else {
		parsedSettings[option] = value
	}
}

// DeleteParsedSetting removes option from parsedSettings so it will not be
// emitted by the next WriteSettings call. Used by the clean tool to drop
// keys that are no longer recognised by micro or its plugins.
func DeleteParsedSetting(option string) {
	delete(parsedSettings, option)
}

// WriteSettings writes parsedSettings to the specified filename as JSON.
//
// parsedSettings is the authoritative in-memory mirror of settings.json:
// :set mutates it directly (delete-if-default, else assign), so by the
// time WriteSettings runs the map already contains exactly the keys that
// should appear on disk. WriteSettings is a plain serializer with no
// reconciliation against GlobalSettings.
//
// Behavior note: this function does NOT prune scalar entries whose value
// equals the default. Hand-edited default-valued entries in settings.json
// (e.g. "ruler": false when false is the default) are preserved verbatim.
// The only way an entry leaves settings.json is :set <key> <default-value>
// at runtime, which deletes the key from parsedSettings.
func WriteSettings(filename string) error {
	if settingsParseError {
		// Don't write settings if there was a parse error
		// because this will delete the settings.json if it
		// is invalid. Instead we should allow the user to fix
		// it manually.
		return nil
	}

	var err error
	if _, e := os.Stat(ConfigDir); e == nil {
		txt, _ := json.MarshalIndent(parsedSettings, "", "    ")
		txt = append(txt, '\n')
		err = writeFile(filename, txt)
	}
	return err
}

// RegisterCommonOptionPlug creates a new option (called pl.name). This is meant to be called by plugins to add options.
func RegisterCommonOptionPlug(pl string, name string, defaultvalue any) error {
	return RegisterCommonOption(pl+"."+name, defaultvalue)
}

// RegisterGlobalOptionPlug creates a new global-only option (named pl.name)
func RegisterGlobalOptionPlug(pl string, name string, defaultvalue any) error {
	return RegisterGlobalOption(pl+"."+name, defaultvalue)
}

// RegisterCommonOption creates a new option
func RegisterCommonOption(name string, defaultvalue any) error {
	if _, ok := GlobalSettings[name]; !ok {
		GlobalSettings[name] = defaultvalue
	}
	defaultCommonSettings[name] = defaultvalue
	return nil
}

// RegisterGlobalOption creates a new global-only option
func RegisterGlobalOption(name string, defaultvalue any) error {
	if _, ok := GlobalSettings[name]; !ok {
		GlobalSettings[name] = defaultvalue
	}
	DefaultGlobalOnlySettings[name] = defaultvalue
	return nil
}

// GetGlobalOption returns the global value of the given option
func GetGlobalOption(name string) any {
	return GlobalSettings[name]
}

func defaultFileFormat() string {
	if runtime.GOOS == "windows" {
		return "dos"
	}
	return "unix"
}

func defaultFakeCursor() bool {
	_, wt := os.LookupEnv("WT_SESSION")
	if runtime.GOOS == "windows" && !wt {
		// enabled for windows consoles where the cursor is slow
		return true
	}
	return false
}

func GetInfoBarOffset() int {
	offset := 0
	if GetGlobalOption("infobar").(bool) {
		offset++
	}
	if GetGlobalOption("keymenu").(bool) {
		offset += 2
	}
	return offset
}

// DefaultCommonSettings returns a map of all common buffer settings
// and their default values
func DefaultCommonSettings() map[string]any {
	commonsettings := make(map[string]any)
	for k, v := range defaultCommonSettings {
		commonsettings[k] = v
	}
	return commonsettings
}

// DefaultAllSettings returns a map of all common buffer & global-only settings
// and their default values
func DefaultAllSettings() map[string]any {
	allsettings := make(map[string]any)
	for k, v := range defaultCommonSettings {
		allsettings[k] = v
	}
	for k, v := range DefaultGlobalOnlySettings {
		allsettings[k] = v
	}
	return allsettings
}

// GetNativeValue parses and validates a value for a given option
func GetNativeValue(option, value string) (any, error) {
	curVal := GetGlobalOption(option)
	if curVal == nil {
		return nil, ErrInvalidOption
	}

	switch kind := reflect.TypeOf(curVal).Kind(); kind {
	case reflect.Bool:
		b, err := util.ParseBool(value)
		if err != nil {
			return nil, ErrInvalidValue
		}
		return b, nil
	case reflect.String:
		return value, nil
	case reflect.Float64:
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, ErrInvalidValue
		}
		return f, nil
	default:
		return nil, ErrInvalidValue
	}
}

// OptionIsValid checks if a value is valid for a certain option
func OptionIsValid(option string, value any) error {
	if validator, ok := optionValidators[option]; ok {
		return validator(option, value)
	}

	return nil
}

// Option validators

func validatePositiveValue(option string, value any) error {
	nativeValue, ok := value.(float64)

	if !ok {
		return errors.New("Expected numeric type for " + option)
	}

	if nativeValue < 1 {
		return errors.New(option + " must be greater than 0")
	}

	return nil
}

func validateNonNegativeValue(option string, value any) error {
	nativeValue, ok := value.(float64)

	if !ok {
		return errors.New("Expected numeric type for " + option)
	}

	if nativeValue < 0 {
		return errors.New(option + " must be non-negative")
	}

	return nil
}

func validateChoice(option string, value any) error {
	if choices, ok := OptionChoices[option]; ok {
		val, ok := value.(string)
		if !ok {
			return errors.New("Expected string type for " + option)
		}

		for _, v := range choices {
			if val == v {
				return nil
			}
		}

		choicesStr := strings.Join(choices, ", ")
		return errors.New(option + " must be one of: " + choicesStr)
	}

	return errors.New("Option has no pre-defined choices")
}

func validateColorscheme(option string, value any) error {
	colorscheme, ok := value.(string)

	if !ok {
		return errors.New("Expected string type for colorscheme")
	}

	if !ColorschemeExists(colorscheme) {
		return errors.New(colorscheme + " is not a valid colorscheme")
	}

	return nil
}

func validateEncoding(option string, value any) error {
	_, err := htmlindex.Get(value.(string))
	return err
}
