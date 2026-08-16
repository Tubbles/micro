package action

import (
	"fmt"
	"sort"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/widget"
)

const (
	optionsListHint     = "<Enter> edit - <Esc> cancel"
	optionsChoiceHint   = "<Up>/<Down> move - <Enter> apply temporarily - <Ctrl-Enter> apply permanently - <Esc> back"
	optionsFreeformHint = "Type a value - <Enter> apply temporarily - <Ctrl-Enter> apply permanently - <Esc> back"
)

// sortedOptionNames returns every option name from DefaultAllSettings
// (which includes plugin-registered options via RegisterCommonOption/
// RegisterGlobalOption), sorted for a deterministic, browsable list.
func sortedOptionNames() []string {
	all := config.DefaultAllSettings()
	names := make([]string, 0, len(all))
	for name := range all {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// optionsListItems builds one picker row per option name: the label
// shows "<name>  =  <effective value>", the aux column shows the
// layer that supplied the value and the default, e.g.
// "[local] default: 4". buf may be nil (no active buffer), in which
// case every option falls back to the global/local/default layers;
// GetSettingOrigin handles a nil buffer context directly.
func optionsListItems(names []string, defaults map[string]any, buf *buffer.Buffer) []widget.PickerItem {
	var bufSettings map[string]any
	var bufLocalSettings map[string]bool
	if buf != nil {
		bufSettings = buf.Settings
		bufLocalSettings = buf.LocalSettings
	}

	items := make([]widget.PickerItem, len(names))
	for i, name := range names {
		value, origin := config.GetSettingOrigin(name, bufSettings, bufLocalSettings)
		items[i] = widget.PickerItem{
			Label: fmt.Sprintf("%s  =  %v", name, value),
			Aux:   fmt.Sprintf("[%s] default: %v", origin, defaults[name]),
		}
	}
	return items
}

// optionsValueItems builds the value-mode picker rows for option:
// bool options get two rows (true/false); choice options get one row
// per choice, with colorscheme and filetype special-cased to reuse
// their existing completers since neither is in config.OptionChoices;
// anything else (a number or free-form string) gets no rows at all,
// since its value comes from the query line rather than a highlighted
// row (see the OnSubmit/OnSubmitCtrl wiring in OptionsPicker).
// rawValues is index-parallel with items and holds the native value
// each row commits; it is nil in the no-rows case.
func optionsValueItems(option string, defaults map[string]any) (items []widget.PickerItem, rawValues []any, hint string) {
	if _, ok := defaults[option].(bool); ok {
		return boolValueItems()
	}
	switch option {
	case "colorscheme":
		_, names := colorschemeComplete("")
		return choiceValueItems(names)
	case "filetype":
		_, names := filetypeComplete("")
		return choiceValueItems(names)
	}
	if choices, ok := config.OptionChoices[option]; ok {
		return choiceValueItems(choices)
	}
	return nil, nil, optionsFreeformHint
}

func boolValueItems() ([]widget.PickerItem, []any, string) {
	items := []widget.PickerItem{{Label: "true"}, {Label: "false"}}
	values := []any{true, false}
	return items, values, optionsChoiceHint
}

func choiceValueItems(choices []string) ([]widget.PickerItem, []any, string) {
	items := make([]widget.PickerItem, len(choices))
	values := make([]any, len(choices))
	for i, c := range choices {
		items[i] = widget.PickerItem{Label: c}
		values[i] = c
	}
	return items, values, optionsChoiceHint
}

// applyOptionValue dispatches a value chosen in the options picker.
//
// permanent=false is the Enter path: SetGlobalOptionNative with
// writeToFile=false applies the value in RAM only (GlobalSettings and
// every open buffer's Settings), then the option is marked volatile
// so a later `> reload` keeps it instead of reverting to whatever is
// on disk. Marking happens AFTER the call because
// doSetGlobalOptionNative (which SetGlobalOptionNative always routes
// through) itself deletes any existing VolatileSettings entry for
// option as part of applying a value; marking first would be
// immediately undone.
//
// permanent=true is the Ctrl-Enter path: writeToFile=true additionally
// serializes parsedSettings to settings.json, and the option is left
// off VolatileSettings since it is no longer a session-only value.
func applyOptionValue(option string, value any, permanent bool) error {
	if err := SetGlobalOptionNative(option, value, permanent); err != nil {
		return err
	}
	if !permanent {
		config.VolatileSettings[option] = true
	}
	return nil
}

// OptionsPicker opens the > options picker: a browsable, editable
// list of every registered option (DefaultAllSettings, which includes
// plugin-registered options) with its effective value and the
// configuration layer that supplied it.
//
// The picker has two modes sharing a single widget.Picker instance;
// SetItems swaps the row set in place rather than pushing a second
// widget onto a stack (there is no stack, only one widget can be
// active at a time). List mode shows one row per option; Enter on a
// row switches to value mode for that option via showValue. Value
// mode shows bool/choice rows to pick from, or no rows at all for a
// number/free-form string option, where the hint explains typing the
// value into the query line instead.
//
// Esc in value mode calls showList, which restores the option list
// and the query the user had typed there before switching
// (widget.Picker.SetQuery, since SetItems always clears the query);
// Esc in list mode is not intercepted (OnEsc returns false) and falls
// through to the picker's normal close.
//
// Enter commits the highlighted row (or, for a number/free-form
// string, the typed query) temporarily; Ctrl-Enter commits the same
// value permanently. See applyOptionValue for the RAM-only-vs-disk
// distinction.
func (h *BufPane) OptionsPicker() {
	buf := h.Buf
	names := sortedOptionNames()
	defaults := config.DefaultAllSettings()

	var picker *widget.Picker
	editing := ""          // "" means list mode; else the option being edited
	listQuery := ""        // list-mode filter, preserved across a value-mode round trip
	var valueChoices []any // index-parallel with the value-mode Items; nil in free-form mode

	showList := func() {
		editing = ""
		picker.SetItems(optionsListItems(names, defaults, buf))
		if listQuery != "" {
			picker.SetQuery(listQuery)
		}
		picker.SetTitle("Options")
		picker.SetHint(optionsListHint)
	}

	showValue := func(option string) {
		editing = option
		items, values, hint := optionsValueItems(option, defaults)
		valueChoices = values
		picker.SetItems(items)
		picker.SetTitle("Options · " + option)
		picker.SetHint(hint)
	}

	commit := func(value any, permanent bool) {
		if err := applyOptionValue(editing, value, permanent); err != nil {
			InfoBar.Error(err)
			return
		}
		showList()
	}

	picker = widget.NewPicker(widget.PickerOptions{
		Title:    "Options",
		Hint:     optionsListHint,
		Query:    true,
		Items:    optionsListItems(names, defaults, buf),
		Geometry: widget.Geometry{Kind: widget.GeomScreenRect, Rect: widgetOverlayRect()},
		OnSelect: func(idx int) {
			if editing == "" {
				if idx < 0 || idx >= len(names) {
					return
				}
				listQuery = picker.Query()
				showValue(names[idx])
				return
			}
			if idx < 0 || idx >= len(valueChoices) {
				return
			}
			commit(valueChoices[idx], false)
		},
		OnSelectCtrl: func(idx int) {
			if editing == "" || idx < 0 || idx >= len(valueChoices) {
				return
			}
			commit(valueChoices[idx], true)
		},
		OnSubmit: func(query string) {
			if editing == "" {
				return
			}
			v, err := config.GetNativeValue(editing, query)
			if err != nil {
				InfoBar.Error(err)
				return
			}
			commit(v, false)
		},
		OnSubmitCtrl: func(query string) {
			if editing == "" {
				return
			}
			v, err := config.GetNativeValue(editing, query)
			if err != nil {
				InfoBar.Error(err)
				return
			}
			commit(v, true)
		},
		OnEsc: func() bool {
			if editing == "" {
				return false
			}
			showList()
			return true
		},
	})
	widget.Open(picker)
}

// OptionsCmd is the command-bar entry point for the options picker.
// Args are ignored.
func (h *BufPane) OptionsCmd(args []string) {
	h.OptionsPicker()
}
