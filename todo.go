package main

import (
	"fmt"
	"image"

	"github.com/ahmetb/RectangleWin/config"
	"github.com/ahmetb/RectangleWin/layout"
	"github.com/gonutz/w32/v2"
)

// Access is serialized by the keyboard action executor; shutdown waits for it.
type windowManager struct {
	settings config.Todo
	todos    map[w32.HMONITOR]*todoWindow
}

type todoWindow struct {
	hwnd      w32.HWND
	pid       w32.DWORD
	placement w32.WINDOWPLACEMENT
	topmost   bool
}

func (t *todoWindow) alive() bool {
	_, pid := w32.GetWindowThreadProcessId(t.hwnd)
	return w32.IsWindow(t.hwnd) && pid == t.pid
}

func setTopmost(hwnd w32.HWND, topmost bool) error {
	order := w32.HWND_NOTOPMOST
	if topmost {
		order = w32.HWND_TOPMOST
	}
	if !w32.SetWindowPos(hwnd, order, 0, 0, 0, 0, w32.SWP_NOMOVE|w32.SWP_NOSIZE|w32.SWP_NOACTIVATE) {
		return fmt.Errorf("set always on top: %d", w32.GetLastError())
	}
	return nil
}

func (m *windowManager) release(mon w32.HMONITOR, restorePlacement bool) error {
	t := m.todos[mon]
	if t == nil {
		return nil
	}
	if t.alive() {
		if err := setTopmost(t.hwnd, t.topmost); err != nil {
			return err
		}
		if restorePlacement && !w32.SetWindowPlacement(t.hwnd, &t.placement) {
			return fmt.Errorf("restore TODO window placement: %d", w32.GetLastError())
		}
	}
	delete(m.todos, mon)
	return nil
}

func (m *windowManager) clear() {
	for mon := range m.todos {
		if err := m.release(mon, true); err != nil {
			fmt.Printf("warn: %v\n", err)
		}
	}
}

// A closed window or a window moved to another monitor no longer reserves space.
func (m *windowManager) prune() {
	for mon, t := range m.todos {
		if !t.alive() {
			delete(m.todos, mon)
		} else if w32.MonitorFromWindow(t.hwnd, w32.MONITOR_DEFAULTTONEAREST) != mon {
			if err := m.release(mon, false); err != nil {
				fmt.Printf("warn: %v\n", err)
			}
		}
	}
}

func splitTodo(work w32.RECT, settings config.Todo) (w32.RECT, w32.RECT) {
	a, b := layout.Split(image.Rect(int(work.Left), int(work.Top), int(work.Right), int(work.Bottom)), settings.Side, settings.Fraction)
	rect := func(r image.Rectangle) w32.RECT {
		return w32.RECT{Left: int32(r.Min.X), Top: int32(r.Min.Y), Right: int32(r.Max.X), Bottom: int32(r.Max.Y)}
	}
	return rect(a), rect(b)
}

func monitorWork(mon w32.HMONITOR) (w32.RECT, error) {
	var info w32.MONITORINFO
	if !w32.GetMonitorInfo(mon, &info) {
		return w32.RECT{}, fmt.Errorf("get monitor work area: %d", w32.GetLastError())
	}
	return info.RcWork, nil
}

func fillArea(work, _ w32.RECT) w32.RECT { return work }

func (m *windowManager) toggleTodo(hwnd w32.HWND) error {
	m.prune()
	for mon, t := range m.todos {
		if t.hwnd == hwnd {
			return m.release(mon, true)
		}
	}
	if !isZonableWindow(hwnd) {
		return fmt.Errorf("TODO window is not zonable")
	}
	mon := w32.MonitorFromWindow(hwnd, w32.MONITOR_DEFAULTTONEAREST)
	work, err := monitorWork(mon)
	if err != nil {
		return err
	}
	t := &todoWindow{hwnd: hwnd, topmost: w32.GetWindowLong(hwnd, w32.GWL_EXSTYLE)&w32.WS_EX_TOPMOST != 0}
	_, t.pid = w32.GetWindowThreadProcessId(hwnd)
	if !w32.GetWindowPlacement(hwnd, &t.placement) {
		return fmt.Errorf("save TODO window placement: %d", w32.GetLastError())
	}
	if err := m.release(mon, true); err != nil {
		return err
	}
	reserved, _ := splitTodo(work, m.settings)
	if _, err := resizeInArea(hwnd, fillArea, &reserved); err != nil {
		w32.SetWindowPlacement(hwnd, &t.placement)
		return err
	}
	if err := setTopmost(hwnd, true); err != nil {
		w32.SetWindowPlacement(hwnd, &t.placement)
		return err
	}
	m.todos[mon] = t
	return nil
}

func (m *windowManager) area(hwnd w32.HWND) (*w32.RECT, bool, error) {
	m.prune()
	mon := w32.MonitorFromWindow(hwnd, w32.MONITOR_DEFAULTTONEAREST)
	t := m.todos[mon]
	if t == nil {
		return nil, false, nil
	}
	work, err := monitorWork(mon)
	if err != nil {
		return nil, false, err
	}
	reserved, available := splitTodo(work, m.settings)
	if t.hwnd == hwnd {
		return &reserved, true, nil
	}
	return &available, false, nil
}

func (m *windowManager) resize(hwnd w32.HWND, f resizeFunc) (bool, error) {
	area, pinned, err := m.area(hwnd)
	if err != nil {
		return false, err
	}
	if pinned {
		f = fillArea
	}
	return resizeInArea(hwnd, f, area)
}

func (m *windowManager) maximize(hwnd w32.HWND) error {
	area, _, err := m.area(hwnd)
	if err != nil {
		return err
	}
	if area == nil {
		return maximize(hwnd)
	}
	_, err = resizeInArea(hwnd, fillArea, area)
	return err
}

func (m *windowManager) toggleAlwaysOnTop(hwnd w32.HWND) error {
	m.prune()
	for _, t := range m.todos {
		if t.hwnd == hwnd {
			return setTopmost(hwnd, true)
		}
	}
	return toggleAlwaysOnTop(hwnd)
}
