package widget

import (
	"time"

	"github.com/micro-editor/tcell/v2"

	"github.com/micro-editor/micro/v2/internal/screen"
)

// screenSize is overridable for tests so the picker can be exercised
// without a real tcell.Screen.
var screenSize = func() (int, int) {
	if screen.Screen == nil {
		return 0, 0
	}
	return screen.Screen.Size()
}

// PickerItem is one row in a picker. Aux is rendered right-aligned
// (e.g. a file size or the trailing "/" on a directory).
type PickerItem struct {
	Label string
	Aux   string
}

// PickerOptions configures a Picker at construction time. After
// construction the items can be replaced with SetItems, etc.
type PickerOptions struct {
	Title    string
	Hint     string
	Items    []PickerItem
	Geometry Geometry
	// OnSelect fires when the user activates a row (Enter,
	// double-click). The picker is NOT auto-closed; the callback
	// decides (it might want to refresh the items in place).
	OnSelect func(index int)
	// OnClose fires when the user dismisses the picker (Esc, click
	// outside). The callback should treat the picker as gone; the
	// active slot is cleared by the dispatcher first.
	OnClose func()
}

// Picker is a generic list-of-rows overlay widget.
type Picker struct {
	opts    PickerOptions
	current int
	top     int

	lastClickTime time.Time
	lastClickRow  int

	// nowFn is used by tests to inject a fake clock for the
	// double-click window.
	nowFn func() time.Time
}

const doubleClickWindow = 500 * time.Millisecond

// NewPicker builds a picker. The picker is not yet active; pass it
// to widget.Open to make it the current overlay.
func NewPicker(opts PickerOptions) *Picker {
	return &Picker{
		opts:         opts,
		lastClickRow: -1,
		nowFn:        time.Now,
	}
}

// SetItems replaces the row list. The current row is clamped to the
// new length (resetting to 0 if the list is empty); top is reset.
func (p *Picker) SetItems(items []PickerItem) {
	p.opts.Items = items
	if len(items) == 0 {
		p.current = 0
	} else if p.current >= len(items) {
		p.current = len(items) - 1
	}
	p.top = 0
	p.scrollIntoView()
}

// SetTitle replaces the title shown on the top border.
func (p *Picker) SetTitle(t string) { p.opts.Title = t }

// SetHint replaces the hint line shown above the bottom border. Callers
// rebuild the hint when in-picker state changes (e.g. a visibility
// toggle flips) so the user can see the new state without dismissing
// the picker.
func (p *Picker) SetHint(s string) { p.opts.Hint = s }

// SetCurrent moves the highlight to index i, clamped to the item
// range. Top is adjusted so the row is visible.
func (p *Picker) SetCurrent(i int) {
	if len(p.opts.Items) == 0 {
		p.current = 0
		return
	}
	if i < 0 {
		i = 0
	}
	if i >= len(p.opts.Items) {
		i = len(p.opts.Items) - 1
	}
	p.current = i
	p.scrollIntoView()
}

// Current returns the highlight index.
func (p *Picker) Current() int { return p.current }

// Geometry returns the picker's geometry, recomputed at draw time
// when buffer-anchored.
func (p *Picker) Geometry() Geometry { return p.opts.Geometry }

// Close fires the OnClose callback once. Idempotent: subsequent
// calls do nothing.
func (p *Picker) Close() {
	cb := p.opts.OnClose
	p.opts.OnClose = nil
	if cb != nil {
		cb()
	}
}

// HandleEvent absorbs the event stream while the picker is active.
// Always returns true so events don't leak to the buffer below.
func (p *Picker) HandleEvent(ev tcell.Event) bool {
	switch e := ev.(type) {
	case *tcell.EventKey:
		p.handleKey(e)
	case *tcell.EventMouse:
		p.handleMouse(e)
	}
	return true
}

func (p *Picker) handleKey(e *tcell.EventKey) {
	switch e.Key() {
	case tcell.KeyUp:
		p.move(-1)
	case tcell.KeyDown:
		p.move(1)
	case tcell.KeyPgUp:
		p.move(-p.bodyHeight())
	case tcell.KeyPgDn:
		p.move(p.bodyHeight())
	case tcell.KeyHome:
		p.SetCurrent(0)
	case tcell.KeyEnd:
		p.SetCurrent(len(p.opts.Items) - 1)
	case tcell.KeyEnter:
		p.activate()
	case tcell.KeyEsc:
		CloseActive()
	}
	// All other keys (incl. typed runes) are silently swallowed.
}

func (p *Picker) handleMouse(e *tcell.EventMouse) {
	mx, my := e.Position()
	sw, sh := screenSize()
	rect := Resolve(p.opts.Geometry, sw, sh)
	btn := e.Buttons()

	switch {
	case btn&tcell.WheelUp != 0:
		p.move(-1)
		return
	case btn&tcell.WheelDown != 0:
		p.move(1)
		return
	}

	// Button presses only — releases would double-fire on each
	// click. ButtonNone is the release sentinel in tcell.
	if btn == tcell.ButtonNone {
		return
	}

	if !inRect(mx, my, rect) {
		CloseActive()
		return
	}

	bodyY0 := rect.Y + 1
	bodyH := rect.H - 2
	if my < bodyY0 || my >= bodyY0+bodyH {
		// click on top border, bottom border, or hint line: ignore
		return
	}
	idx := p.top + (my - bodyY0)
	if idx < 0 || idx >= len(p.opts.Items) {
		return
	}

	now := p.nowFn()
	if idx == p.lastClickRow && now.Sub(p.lastClickTime) <= doubleClickWindow {
		p.current = idx
		p.lastClickTime = time.Time{}
		p.lastClickRow = -1
		p.activate()
		return
	}

	p.current = idx
	p.scrollIntoView()
	p.lastClickTime = now
	p.lastClickRow = idx
}

func (p *Picker) move(delta int) {
	if len(p.opts.Items) == 0 {
		return
	}
	n := p.current + delta
	if n < 0 {
		n = 0
	}
	if n >= len(p.opts.Items) {
		n = len(p.opts.Items) - 1
	}
	p.current = n
	p.scrollIntoView()
}

func (p *Picker) activate() {
	if len(p.opts.Items) == 0 {
		return
	}
	if p.opts.OnSelect != nil {
		p.opts.OnSelect(p.current)
	}
}

func (p *Picker) bodyHeight() int {
	sw, sh := screenSize()
	rect := Resolve(p.opts.Geometry, sw, sh)
	h := rect.H - 2
	if h < 1 {
		h = 1
	}
	return h
}

func (p *Picker) scrollIntoView() {
	bh := p.bodyHeight()
	if p.current < p.top {
		p.top = p.current
	} else if p.current >= p.top+bh {
		p.top = p.current - bh + 1
	}
	if p.top < 0 {
		p.top = 0
	}
}

func inRect(x, y int, r ScreenRect) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}
