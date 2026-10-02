// Copyright 2022 Ahmet Alp Balkan
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// TODO make it possible to "go generate" on Windows (https://github.com/josephspurrier/goversioninfo/issues/52).
//go:generate /bin/bash -c "go run github.com/josephspurrier/goversioninfo/cmd/goversioninfo@latest -arm -64 -icon=assets/icon.ico - <<< '{}'"

package main

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"reflect"
	"runtime"

	"github.com/getlantern/systray"
	"github.com/gonutz/w32/v2"

	"github.com/ahmetb/RectangleWin/config"
	"github.com/ahmetb/RectangleWin/w32ex"
)

func main() {
	runtime.LockOSThread() // since we bind hotkeys etc that need to dispatch their message here
	cfg, configPath, err := config.Load()
	if err != nil {
		showMessageBox(err.Error())
		return
	}
	fmt.Printf("configuration: %s\n", configPath)
	if !w32ex.SetProcessPerMonitorDPIAware() {
		panic("failed to set per-monitor DPI awareness")
	}

	autorun, err := AutoRunEnabled()
	if err != nil {
		panic(err)
	}
	fmt.Printf("autorun enabled=%v\n", autorun)
	printMonitors()
	windows := &windowManager{settings: cfg.Todo, todos: make(map[w32.HMONITOR]*todoWindow)}
	defer windows.clear()
	resize := windows.resize
	maximize := windows.maximize
	toggleAlwaysOnTop := windows.toggleAlwaysOnTop

	edgeFuncs := [][]resizeFunc{
		{leftHalf, leftTwoThirds, leftOneThirds},
		{rightHalf, rightTwoThirds, rightOneThirds},
		{topHalf, topTwoThirds, topOneThirds},
		{bottomHalf, bottomTwoThirds, bottomOneThirds}}
	cornerFuncs := [][]resizeFunc{
		{topLeftHalf, topLeftTwoThirds, topLeftOneThirds},
		{topRightHalf, topRightTwoThirds, topRightOneThirds},
		{bottomLeftHalf, bottomLeftTwoThirds, bottomLeftOneThirds},
		{bottomRightHalf, bottomRightTwoThirds, bottomRightOneThirds}}
	var lastCycleWindow w32.HWND
	var lastCycleAction string
	cycleTurn := 0
	cycleFuncs := func(hwnd w32.HWND, action string, funcs []resizeFunc) {
		if hwnd == 0 {
			panic("foreground window is NULL")
		}
		if lastCycleWindow != hwnd || lastCycleAction != action {
			cycleTurn = 0
		}
		if _, err := resize(hwnd, funcs[cycleTurn%len(funcs)]); err != nil {
			fmt.Printf("warn: resize: %v\n", err)
			return
		}
		lastCycleWindow = hwnd
		lastCycleAction = action
		cycleTurn++
	}
	resizeDirect := func(hwnd w32.HWND, f resizeFunc) {
		if _, err := resize(hwnd, f); err != nil {
			fmt.Printf("warn: resize: %v\n", err)
		}
		lastCycleWindow = 0 // the next edge or corner shortcut starts at a half
	}
	maximizeDirect := func(hwnd w32.HWND) {
		lastCycleWindow = 0
		if err := maximize(hwnd); err != nil {
			fmt.Printf("warn: maximize: %v\n", err)
		}
	}

	actions := map[string]func(w32.HWND){
		"edge_left":           func(hwnd w32.HWND) { cycleFuncs(hwnd, "edge_left", edgeFuncs[0]) },
		"edge_right":          func(hwnd w32.HWND) { cycleFuncs(hwnd, "edge_right", edgeFuncs[1]) },
		"edge_top":            func(hwnd w32.HWND) { cycleFuncs(hwnd, "edge_top", edgeFuncs[2]) },
		"edge_bottom":         func(hwnd w32.HWND) { cycleFuncs(hwnd, "edge_bottom", edgeFuncs[3]) },
		"third_left":          func(hwnd w32.HWND) { resizeDirect(hwnd, leftOneThirds) },
		"third_middle_width":  func(hwnd w32.HWND) { resizeDirect(hwnd, middleWidthThird) },
		"third_right":         func(hwnd w32.HWND) { resizeDirect(hwnd, rightOneThirds) },
		"third_top":           func(hwnd w32.HWND) { resizeDirect(hwnd, topOneThirds) },
		"third_middle_height": func(hwnd w32.HWND) { resizeDirect(hwnd, middleHeightThird) },
		"third_bottom":        func(hwnd w32.HWND) { resizeDirect(hwnd, bottomOneThirds) },
		"corner_top_left":     func(hwnd w32.HWND) { cycleFuncs(hwnd, "corner_top_left", cornerFuncs[0]) },
		"corner_top_right":    func(hwnd w32.HWND) { cycleFuncs(hwnd, "corner_top_right", cornerFuncs[1]) },
		"corner_bottom_left":  func(hwnd w32.HWND) { cycleFuncs(hwnd, "corner_bottom_left", cornerFuncs[2]) },
		"corner_bottom_right": func(hwnd w32.HWND) { cycleFuncs(hwnd, "corner_bottom_right", cornerFuncs[3]) },
		"maximize":            maximizeDirect,
		"maximize_space":      maximizeDirect,
		"todo": func(hwnd w32.HWND) {
			lastCycleWindow = 0
			if err := windows.toggleTodo(hwnd); err != nil {
				showMessageBox(err.Error())
			}
		},
		"center": func(hwnd w32.HWND) {
			resizeDirect(hwnd, center)
		},
		"always_on_top": func(hwnd w32.HWND) {
			if err := toggleAlwaysOnTop(hwnd); err != nil {
				fmt.Printf("warn: toggleAlwaysOnTop: %v\n", err)
				return
			}
			fmt.Printf("> toggled always on top: %v\n", hwnd)
		},
	}

	stopKeyboardHook, err := installKeyboardHook(cfg.Keyboard, actions)
	if err != nil {
		showMessageBox(err.Error())
		return
	}
	defer stopKeyboardHook()

	exitCh := make(chan os.Signal, 1)
	signal.Notify(exitCh, os.Interrupt)
	go func() {
		<-exitCh
		fmt.Println("exit signal received")
		systray.Quit() // causes WM_CLOSE, WM_QUIT, not sure if a side-effect
	}()

	// TODO systray/systray.go already locks the OS thread in init()
	// however it's not clear if GetMessage(0,0) will continue to work
	// as we run "go initTray()" and not pin the thread that initializes the
	// tray.
	initTray(configPath)
	if err := msgLoop(); err != nil {
		panic(err)
	}
}

func showMessageBox(text string) {
	w32.MessageBox(w32.GetActiveWindow(), text, "RectangleWin", w32.MB_ICONWARNING|w32.MB_OK)
}

type resizeFunc func(disp, cur w32.RECT) w32.RECT

func center(disp, cur w32.RECT) w32.RECT {
	if cur.Width() > disp.Width() {
		cur.Right = cur.Left + disp.Width()
	}
	if cur.Height() > disp.Height() {
		cur.Bottom = cur.Top + disp.Height()
	}
	// TODO find a way to round up divisions consistently as it causes multiple runs to shift by 1px
	w := (disp.Width() - cur.Width()) / 2
	h := (disp.Height() - cur.Height()) / 2
	return w32.RECT{
		Left:   disp.Left + w,
		Right:  disp.Left + w + cur.Width(),
		Top:    disp.Top + h,
		Bottom: disp.Top + h + cur.Height()}
}

func resize(hwnd w32.HWND, f resizeFunc) (bool, error) {
	return resizeInArea(hwnd, f, nil)
}

func resizeInArea(hwnd w32.HWND, f resizeFunc, area *w32.RECT) (bool, error) {
	if !isZonableWindow(hwnd) {
		fmt.Printf("warn: non-zonable window: %s\n", w32.GetWindowText(hwnd))
		return false, nil
	}
	// ShowWindow returns the window's previous visibility, not whether this
	// call succeeded. Restore first so measurements and frame offsets describe
	// the normal window, even if the shortcut began from a maximized state.
	w32.ShowWindow(hwnd, w32.SW_SHOWNORMAL)
	rect := w32.GetWindowRect(hwnd)
	if rect == nil {
		return false, fmt.Errorf("failed to GetWindowRect:%d", w32.GetLastError())
	}
	mon := w32.MonitorFromWindow(hwnd, w32.MONITOR_DEFAULTTONEAREST)
	var monInfo w32.MONITORINFO
	if !w32.GetMonitorInfo(mon, &monInfo) {
		return false, fmt.Errorf("failed to GetMonitorInfo:%d", w32.GetLastError())
	}

	ok, frame := w32.DwmGetWindowAttributeEXTENDED_FRAME_BOUNDS(hwnd)
	if !ok {
		return false, fmt.Errorf("failed to DwmGetWindowAttributeEXTENDED_FRAME_BOUNDS:%d", w32.GetLastError())
	}
	fmt.Printf("> window: 0x%x %#v (w:%d,h:%d) mon=0x%X\n", hwnd, rect, rect.Width(), rect.Height(), mon)
	fmt.Printf("> DWM frame: %#v (W:%d,H:%d)\n", frame, frame.Width(), frame.Height())

	work := monInfo.RcWork
	if area != nil {
		work = *area
	}
	// DWM already returns physical pixels. Scaling its screen coordinates by
	// the target app's DPI corrupts border sizes (and depends on its position).
	target := f(work, frame)
	newPos := windowBoundsForFrame(target, *rect, frame)

	if sameRect(rect, &newPos) {
		fmt.Println("no resize")
		return false, nil
	}

	fmt.Printf("> resizing to: %#v (W:%d,H:%d)\n", newPos, newPos.Width(), newPos.Height())
	if !w32.SetWindowPos(hwnd, 0, int(newPos.Left), int(newPos.Top), int(newPos.Width()), int(newPos.Height()), w32.SWP_NOZORDER|w32.SWP_NOACTIVATE) {
		return false, fmt.Errorf("failed to SetWindowPos:%d", w32.GetLastError())
	}
	rect = w32.GetWindowRect(hwnd)
	if rect == nil {
		return false, fmt.Errorf("failed to GetWindowRect after resize:%d", w32.GetLastError())
	}
	fmt.Printf("> post-resize: %#v(W:%d,H:%d)\n", rect, rect.Width(), rect.Height())
	if rect.Width() > newPos.Width() || rect.Height() > newPos.Height() {
		fmt.Printf("warn: window is larger than requested: requested %dx%d, actual %dx%d; the app may enforce a minimum size\n",
			newPos.Width(), newPos.Height(), rect.Width(), rect.Height())
	}
	return true, nil
}

func maximize(hwnd w32.HWND) error {
	if !isZonableWindow(hwnd) {
		return errors.New("foreground window is not zonable")
	}
	w32.ShowWindow(hwnd, w32.SW_MAXIMIZE)
	return nil
}

func toggleAlwaysOnTop(hwnd w32.HWND) error {
	if !isZonableWindow(hwnd) {
		return errors.New("foreground window is not zonable")
	}

	if w32.GetWindowLong(hwnd, w32.GWL_EXSTYLE)&w32.WS_EX_TOPMOST != 0 {
		if !w32.SetWindowPos(hwnd, w32.HWND_NOTOPMOST, 0, 0, 0, 0, w32.SWP_NOMOVE|w32.SWP_NOSIZE) {
			return fmt.Errorf("failed to SetWindowPos(HWND_NOTOPMOST): %v", w32.GetLastError())
		}
	} else {
		if !w32.SetWindowPos(hwnd, w32.HWND_TOPMOST, 0, 0, 0, 0, w32.SWP_NOMOVE|w32.SWP_NOSIZE) {
			return fmt.Errorf("failed to SetWindowPos(HWND_TOPMOST) :%v", w32.GetLastError())
		}
	}
	return nil
}

// windowBoundsForFrame expands a desired visible frame by the measured invisible
// borders. All three rectangles must be in physical screen coordinates.
func windowBoundsForFrame(target, window, frame w32.RECT) w32.RECT {
	return w32.RECT{
		Left:   target.Left + window.Left - frame.Left,
		Right:  target.Right + window.Right - frame.Right,
		Top:    target.Top + window.Top - frame.Top,
		Bottom: target.Bottom + window.Bottom - frame.Bottom,
	}
}

func sameRect(a, b *w32.RECT) bool {
	return a != nil && b != nil && reflect.DeepEqual(*a, *b)
}
