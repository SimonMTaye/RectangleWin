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

package main

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/ahmetb/RectangleWin/config"
	"github.com/ahmetb/RectangleWin/keyboard"
	"github.com/gonutz/w32/v2"
)

type shortcutAction struct {
	name       string
	window     w32.HWND
	generation uint64
}

// keyboardHookEvent mirrors KBDLLHOOKSTRUCT, including pointer-size alignment.
type keyboardHookEvent struct {
	vkCode, scanCode, flags, time uint32
	extraInfo                     uintptr
}

// installKeyboardHook gives the hook its own message-pump thread. Window actions
// run serially on a separate goroutine: neither slow windows nor the tray's modal
// menu loop may block hook delivery or consume action messages.
func installKeyboardHook(cfg config.Keyboard, callbacks map[string]func(w32.HWND)) (func(), error) {
	for action := range cfg.Bindings {
		if callbacks[action] == nil {
			return nil, fmt.Errorf("no handler for configured action %q", action)
		}
	}
	actions := make(chan shortcutAction, 64)
	generation := new(uint64)
	ready := make(chan error, 1)
	done := make(chan struct{})
	var threadID uintptr
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(done)
		kernel32 := syscall.NewLazyDLL("kernel32.dll")
		threadID, _, _ = kernel32.NewProc("GetCurrentThreadId").Call()
		stop, err := runKeyboardHook(cfg, actions, generation)
		ready <- err
		if err != nil {
			return
		}
		defer stop()
		if err := msgLoop(); err != nil {
			fmt.Printf("warn: keyboard message loop: %v\n", err)
		}
	}()
	if err := <-ready; err != nil {
		return nil, err
	}
	go func() {
		// Window operations include GetDC/ReleaseDC, which must share a thread.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		for action := range actions {
			// Do not apply a delayed shortcut to a newly focused window or
			// replay queued actions after returning from the secure desktop.
			if action.generation == atomic.LoadUint64(generation) && action.window != 0 && action.window == w32.GetForegroundWindow() {
				callbacks[action.name](action.window)
			}
		}
	}()
	return func() {
		post := syscall.NewLazyDLL("user32.dll").NewProc("PostThreadMessageW")
		if ok, _, err := post.Call(threadID, w32.WM_QUIT, 0, 0); ok == 0 {
			fmt.Printf("warn: stop keyboard thread: %v\n", err)
			return
		}
		<-done
		close(actions)
	}, nil
}

func runKeyboardHook(cfg config.Keyboard, actions chan<- shortcutAction, generation *uint64) (func(), error) {
	user32 := syscall.NewLazyDLL("user32.dll")
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	setHook := user32.NewProc("SetWindowsHookExW")
	unhook := user32.NewProc("UnhookWindowsHookEx")
	callNext := user32.NewProc("CallNextHookEx")

	getKeyState := user32.NewProc("GetAsyncKeyState")

	module, _, err := kernel32.NewProc("GetModuleHandleW").Call(0)
	if module == 0 {
		return nil, fmt.Errorf("GetModuleHandleW: %w", err)
	}
	// Ensure the thread's message queue exists before the hook can post actions.
	var msg w32.MSG
	user32.NewProc("PeekMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 0)

	matcher := keyboard.New(cfg)
	// Key releases on the secure desktop are not delivered to our keyboard
	// hook. Desktop-switch notifications run on this same message-pump thread.
	desktopCallback := syscall.NewCallback(func(hook uintptr, event uint32, hwnd uintptr, objectID, childID int32, eventThread, eventTime uint32) uintptr {
		matcher = keyboard.New(cfg)
		atomic.AddUint64(generation, 1)
		return 0
	})
	desktopHook, _, err := user32.NewProc("SetWinEventHook").Call(
		0x20, 0x20, 0, desktopCallback, 0, 0, 0) // EVENT_SYSTEM_DESKTOPSWITCH, WINEVENT_OUTOFCONTEXT
	if desktopHook == 0 {
		return nil, fmt.Errorf("watch desktop switches: %w", err)
	}
	unhookDesktop := user32.NewProc("UnhookWinEvent")
	callback := syscall.NewCallback(func(code int32, event uintptr, data *keyboardHookEvent) uintptr {
		if code == 0 && (event == w32.WM_KEYDOWN || event == w32.WM_SYSKEYDOWN || event == w32.WM_KEYUP || event == w32.WM_SYSKEYUP) {
			// Resync modifiers
			// to recover from missed releases (e.g. after the secure desktop).
			for _, vk := range []int{0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5, 0x5B, 0x5C} {
				state, _, _ := getKeyState.Call(uintptr(vk))
				matcher.SetKeyDown(vk, state&0x8000 != 0)
			}
			vk := int(data.vkCode)
			down := event == w32.WM_KEYDOWN || event == w32.WM_SYSKEYDOWN
			action, suppress := matcher.Handle(vk, down)
			if action != "" {
				// Never block keyboard input if window operations stall. Excess
				// actions are discarded rather than building an unbounded backlog.
				select {
				case actions <- shortcutAction{action, w32.GetForegroundWindow(), atomic.LoadUint64(generation)}:
				default:
				}
			}
			if suppress {
				return 1
			}
		}
		result, _, _ := callNext.Call(0, uintptr(code), event, uintptr(unsafe.Pointer(data)))
		return result
	})
	hook, _, err := setHook.Call(13 /* WH_KEYBOARD_LL */, callback, module, 0)
	if hook == 0 {
		unhookDesktop.Call(desktopHook)
		return nil, fmt.Errorf("install keyboard hook: %w", err)
	}
	return func() {
		if ok, _, err := unhookDesktop.Call(desktopHook); ok == 0 {
			fmt.Printf("warn: remove desktop switch hook: %v\n", err)
		}
		if ok, _, err := unhook.Call(hook); ok == 0 {
			fmt.Printf("warn: remove keyboard hook: %v\n", err)
		}
	}, nil
}

func msgLoop() error {
	defer fmt.Println("event loop finished")
	for {
		var m w32.MSG
		c := w32.GetMessage(&m, 0, 0, 0)
		if c == -1 {
			return fmt.Errorf("GetMessage failed: %d", c)
		} else if c == 0 {
			return nil
		}
		w32.TranslateMessage(&m)
		w32.DispatchMessage(&m)
	}
}
