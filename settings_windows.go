package main

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/ahmetb/RectangleWin/config"
	"github.com/gonutz/w32/v2"
)

const (
	settingsClass    = "RectangleWin.Settings"
	settingsActivate = w32.WM_APP + 1
	settingsSave     = 1 // Dialog navigation sends IDOK/IDCANCEL for Enter/Escape.
	settingsCancel   = 2
)

// Serialize registry transactions and systray's unsynchronized checkbox state.
var autoRunSettingsMu sync.Mutex

var settingsInstance struct {
	sync.Mutex
	window *settingsWindow
	hwnd   w32.HWND
}

// Allocate the callback once: syscall callbacks cannot be freed on window close.
var settingsProc = syscall.NewCallback(settingsWindowProc)
var settingsUser32 = syscall.NewLazyDLL("user32.dll")
var settingsLastActivePopup = settingsUser32.NewProc("GetLastActivePopup")
var settingsIsIconic = settingsUser32.NewProc("IsIconic")

type settingsBindingControls struct {
	action         string
	key, modifiers w32.HWND
}

type settingsWindow struct {
	path                 string
	onAutoRunChanged     func(bool)
	hwnd, super, autorun w32.HWND
	bindings             []settingsBindingControls
	initialAutoRun       bool
	closed, saving       bool
}

// showSettings is nonblocking and may be called from the tray goroutine.
// All HWND operations and the message pump run on one dedicated OS thread.
func showSettings(configPath string, onAutoRunChanged func(bool)) {
	settingsInstance.Lock()
	defer settingsInstance.Unlock()
	if settingsInstance.window != nil {
		if settingsInstance.hwnd != 0 {
			w32.PostMessage(settingsInstance.hwnd, settingsActivate, 0, 0)
		}
		return
	}
	d := &settingsWindow{path: configPath, onAutoRunChanged: onAutoRunChanged}
	settingsInstance.window = d
	go d.run()
}

func (d *settingsWindow) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer func() {
		settingsInstance.Lock()
		settingsInstance.window = nil
		settingsInstance.hwnd = 0
		settingsInstance.Unlock()
	}()

	c, err := config.LoadFile(d.path)
	if err != nil {
		d.error(fmt.Sprintf("Could not load settings:\n%v", err))
		return
	}
	d.initialAutoRun, err = AutoRunEnabled()
	if err != nil {
		d.error(fmt.Sprintf("Could not read start-on-login status:\n%v", err))
		return
	}

	instance := w32.GetModuleHandle("")
	wc := w32.WNDCLASSEX{
		WndProc:    settingsProc,
		Instance:   instance,
		Cursor:     w32.LoadCursor(0, w32.MakeIntResource(w32.IDC_ARROW)),
		Background: w32.HBRUSH(w32.COLOR_BTNFACE + 1),
		ClassName:  syscall.StringToUTF16Ptr(settingsClass),
	}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if w32.RegisterClassEx(&wc) == 0 {
		d.error(fmt.Sprintf("Could not register settings window (Windows error %d).", w32.GetLastError()))
		return
	}
	defer w32.UnregisterClass(settingsClass, instance)

	dc := w32.GetDC(0)
	dpi := w32.GetDeviceCaps(dc, w32.LOGPIXELSY)
	w32.ReleaseDC(0, dc)
	if dpi <= 0 {
		dpi = 96
	}
	scale := func(n int) int { return n * dpi / 96 }
	lf := w32.LOGFONT{Height: -int32(9 * dpi / 72), Weight: 400}
	copy(lf.FaceName[:], syscall.StringToUTF16("Segoe UI"))
	font := w32.CreateFontIndirect(&lf)
	if font == 0 {
		d.error("Could not create the settings font.")
		return
	}
	defer w32.DeleteObject(w32.HGDIOBJ(font))

	actions := make([]string, 0, len(c.Keyboard.Bindings))
	for action := range c.Keyboard.Bindings {
		actions = append(actions, action)
	}
	sort.Strings(actions)
	const width = 660
	height := 210 + 28*len(actions)
	style := uint(w32.WS_CAPTION | w32.WS_SYSMENU | w32.WS_MINIMIZEBOX)
	exStyle := uint(w32.WS_EX_CONTROLPARENT)
	rect := w32.RECT{Right: int32(scale(width)), Bottom: int32(scale(height))}
	if !w32.AdjustWindowRectEx(&rect, style, false, exStyle) {
		d.error("Could not calculate settings window size.")
		return
	}
	d.hwnd = w32.CreateWindowExStr(exStyle, settingsClass, "RectangleWin Settings", style,
		w32.CW_USEDEFAULT, w32.CW_USEDEFAULT, int(rect.Width()), int(rect.Height()), 0, 0, instance, nil)
	if d.hwnd == 0 {
		d.error(fmt.Sprintf("Could not create settings window (Windows error %d).", w32.GetLastError()))
		return
	}
	defer func() {
		if !d.closed {
			w32.DestroyWindow(d.hwnd)
			var msg w32.MSG
			w32.PeekMessage(&msg, 0, w32.WM_QUIT, w32.WM_QUIT, w32.PM_REMOVE)
		}
	}()

	var controlErr error
	control := func(class, text string, extra uint, id, x, y, width, height int) w32.HWND {
		if controlErr != nil {
			return 0
		}
		ex := uint(0)
		if class == "EDIT" {
			ex = w32.WS_EX_CLIENTEDGE
		}
		h := w32.CreateWindowExStr(ex, class, text, w32.WS_CHILD|w32.WS_VISIBLE|extra,
			scale(x), scale(y), scale(width), scale(height), d.hwnd, w32.HMENU(id), instance, nil)
		if h == 0 {
			controlErr = fmt.Errorf("could not create %s control (Windows error %d)", class, w32.GetLastError())
			return 0
		}
		w32.SendMessage(h, w32.WM_SETFONT, uintptr(font), 1)
		return h
	}
	label := func(text string, x, y, width int) {
		control("STATIC", text, 0, 0, x, y, width, 22)
	}
	edit := func(text string, id, x, y, width int) w32.HWND {
		return control("EDIT", text, w32.WS_TABSTOP|w32.ES_AUTOHSCROLL, id, x, y, width, 24)
	}
	label("&Super modifiers:", 16, 18, 170)
	d.super = edit(strings.Join(c.Keyboard.Super, ", "), 100, 190, 14, 454)
	label("Modifiers: ctrl, alt, shift, win; optional left_ / right_ prefix. Separate with commas.", 16, 46, 628)
	label("Action", 16, 78, 230)
	label("Key (e.g. A, Left, F1)", 250, 78, 170)
	label("Additional modifiers (or blank)", 430, 78, 214)
	y := 104
	for i, action := range actions {
		b := c.Keyboard.Bindings[action]
		label(action, 16, y+2, 230)
		key := edit(b.Key, 101+2*i, 250, y, 166)
		modifiers := edit(strings.Join(b.Modifiers, ", "), 102+2*i, 430, y, 214)
		d.bindings = append(d.bindings, settingsBindingControls{action, key, modifiers})
		y += 28
	}
	d.autorun = control("BUTTON", "Start on &login", w32.WS_TABSTOP|w32.BS_AUTOCHECKBOX,
		200, 16, y+6, 240, 24)
	if d.initialAutoRun && d.autorun != 0 {
		w32.SendMessage(d.autorun, w32.BM_SETCHECK, w32.BST_CHECKED, 0)
	}
	label("Restart RectangleWin after saving to apply shortcut changes.", 16, y+36, 628)
	control("BUTTON", "&Save", w32.WS_TABSTOP|w32.BS_DEFPUSHBUTTON, settingsSave, 448, y+66, 94, 28)
	control("BUTTON", "Cancel", w32.WS_TABSTOP|w32.BS_PUSHBUTTON, settingsCancel, 550, y+66, 94, 28)
	if controlErr != nil {
		d.error(controlErr.Error())
		return
	}

	settingsInstance.Lock()
	settingsInstance.hwnd = d.hwnd
	settingsInstance.Unlock()
	w32.ShowWindow(d.hwnd, w32.SW_SHOWNORMAL)
	w32.SetForegroundWindow(d.hwnd)
	w32.SetFocus(d.super)
	var msg w32.MSG
	for {
		switch w32.GetMessage(&msg, 0, 0, 0) {
		case -1:
			d.error(fmt.Sprintf("Settings message loop failed (Windows error %d).", w32.GetLastError()))
			return
		case 0:
			return
		}
		if !w32.IsDialogMessage(d.hwnd, &msg) {
			w32.TranslateMessage(&msg)
			w32.DispatchMessage(&msg)
		}
	}
}

func settingsWindowProc(hwnd w32.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	settingsInstance.Lock()
	d := settingsInstance.window
	settingsInstance.Unlock()
	if d == nil || d.hwnd != hwnd {
		return w32.DefWindowProc(hwnd, msg, wParam, lParam)
	}
	switch msg {
	case settingsActivate:
		if iconic, _, _ := settingsIsIconic.Call(uintptr(hwnd)); iconic != 0 {
			w32.ShowWindow(hwnd, w32.SW_RESTORE)
		}
		// A validation/error message box may currently own keyboard focus.
		popup, _, _ := settingsLastActivePopup.Call(uintptr(hwnd))
		w32.SetForegroundWindow(w32.HWND(popup))
		return 0
	case w32.DM_GETDEFID:
		return uintptr(0x534b<<16 | settingsSave) // DC_HASDEFID
	case w32.WM_COMMAND:
		switch int(wParam & 0xffff) {
		case settingsSave:
			if !d.saving {
				d.save()
			}
			return 0
		case settingsCancel:
			if !d.saving {
				w32.DestroyWindow(hwnd)
			}
			return 0
		}
	case w32.WM_CLOSE:
		if !d.saving {
			w32.DestroyWindow(hwnd)
		}
		return 0
	case w32.WM_DESTROY:
		d.closed = true
		settingsInstance.Lock()
		settingsInstance.hwnd = 0
		settingsInstance.Unlock()
		// GetMessage must wake even when destruction occurs via a sent message.
		w32.PostQuitMessage(0)
		return 0
	}
	return w32.DefWindowProc(hwnd, msg, wParam, lParam)
}

func settingsModifiers(text string) []string {
	if strings.TrimSpace(text) == "" {
		return []string{} // Empty arrays, never JSON null.
	}
	parts := strings.Split(text, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	// Keep empty elements so validation rejects accidental double/trailing commas.
	return parts
}

func (d *settingsWindow) error(text string) {
	w32.MessageBox(d.hwnd, text, "RectangleWin Settings", w32.MB_OK|w32.MB_ICONERROR)
}

func settingsSetAutoRun(enabled bool) error {
	if enabled {
		return AutoRunEnable()
	}
	return AutoRunDisable()
}

func (d *settingsWindow) save() {
	d.saving = true
	defer func() { d.saving = false }()
	c := config.Config{Keyboard: config.Keyboard{
		Super:    settingsModifiers(w32.GetWindowText(d.super)),
		Bindings: make(map[string]config.Binding, len(d.bindings)),
	}}
	for _, controls := range d.bindings {
		c.Keyboard.Bindings[controls.action] = config.Binding{
			Key:       strings.TrimSpace(w32.GetWindowText(controls.key)),
			Modifiers: settingsModifiers(w32.GetWindowText(controls.modifiers)),
		}
	}
	if err := c.Validate(); err != nil {
		d.error(fmt.Sprintf("Please correct the settings before saving:\n%v", err))
		return
	}
	autoRunSettingsMu.Lock()
	defer autoRunSettingsMu.Unlock()
	wanted := w32.SendMessage(d.autorun, w32.BM_GETCHECK, 0, 0) == w32.BST_CHECKED
	// Re-read because the tray can change autorun while this modeless window is open.
	before, err := AutoRunEnabled()
	if err != nil {
		d.error(fmt.Sprintf("Could not read start-on-login status:\n%v", err))
		return
	}
	// An untouched checkbox must not overwrite a newer tray selection.
	if wanted == d.initialAutoRun {
		wanted = before
	}
	changed := wanted != before
	var snapshot autoRunSnapshot
	if changed {
		snapshot, err = snapshotAutoRun()
		if err != nil {
			d.error(fmt.Sprintf("Could not back up start-on-login registration:\n%v", err))
			return
		}
		if err := settingsSetAutoRun(wanted); err != nil {
			d.error(fmt.Sprintf("Could not change start-on-login:\n%v", err))
			return
		}
	}
	if err := config.SaveFile(d.path, c); err != nil {
		message := fmt.Sprintf("Could not save settings to %q:\n%v", d.path, err)
		if changed {
			if rollbackErr := snapshot.restore(); rollbackErr != nil {
				message += fmt.Sprintf("\n\nCould not restore start-on-login:\n%v\nCheck its status before retrying.", rollbackErr)
				if actual, readErr := AutoRunEnabled(); readErr == nil {
					if d.onAutoRunChanged != nil {
						d.onAutoRunChanged(actual)
					}
				} else {
					message += fmt.Sprintf("\nCould not read current start-on-login status: %v", readErr)
				}
			}
		}
		d.error(message)
		return
	}
	if d.onAutoRunChanged != nil {
		d.onAutoRunChanged(wanted)
	}
	w32.DestroyWindow(d.hwnd)
}
