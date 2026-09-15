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
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	AutoRunName = `RectangleWin`
	regKey      = `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`
)

// autoRunSnapshot preserves the value even when it points to another executable
// or has a type that AutoRunEnabled does not recognize.
type autoRunSnapshot struct {
	exists  bool
	valtype uint32
	data    []byte
}

func snapshotAutoRun() (autoRunSnapshot, error) {
	rk, err := registry.OpenKey(registry.CURRENT_USER, regKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return autoRunSnapshot{}, nil
	}
	if err != nil {
		return autoRunSnapshot{}, err
	}
	defer rk.Close()

	// Keep the buffer nonempty so GetValue reads data rather than only its size.
	data := make([]byte, 64)
	for {
		n, valtype, err := rk.GetValue(AutoRunName, data)
		if errors.Is(err, registry.ErrNotExist) {
			return autoRunSnapshot{}, nil
		}
		if errors.Is(err, registry.ErrShortBuffer) && n > len(data) {
			data = make([]byte, n)
			continue
		}
		if err != nil {
			return autoRunSnapshot{}, err
		}
		return autoRunSnapshot{exists: true, valtype: valtype, data: data[:n]}, nil
	}
}

// The pinned x/sys version does not export a raw registry value setter.
var autoRunRegSetValueEx = windows.NewLazySystemDLL("advapi32.dll").NewProc("RegSetValueExW")

func (s autoRunSnapshot) restore() error {
	if !s.exists {
		return AutoRunDisable()
	}
	name, err := windows.UTF16PtrFromString(AutoRunName)
	if err != nil {
		return err
	}
	if err := autoRunRegSetValueEx.Find(); err != nil {
		return err
	}
	rk, _, err := registry.CreateKey(registry.CURRENT_USER, regKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer rk.Close()

	var data *byte
	if len(s.data) > 0 {
		data = &s.data[0]
	}
	status, _, _ := autoRunRegSetValueEx.Call(
		uintptr(rk), uintptr(unsafe.Pointer(name)), 0, uintptr(s.valtype),
		uintptr(unsafe.Pointer(data)), uintptr(len(s.data)),
	)
	// Registry APIs return an error code directly, not through GetLastError.
	if status != 0 {
		return syscall.Errno(status)
	}
	return nil
}

func self() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Abs(path)
}

func autoRunCommand(path string) string {
	command := syscall.EscapeArg(path)
	// EscapeArg leaves paths without whitespace unquoted; quote those too.
	if !strings.HasPrefix(command, `"`) {
		command = `"` + command + `"`
	}
	return command
}

func autoRunCommandMatches(command, path string) bool {
	// Older versions registered the executable path without quoting it.
	return command == path || command == autoRunCommand(path)
}

func AutoRunEnabled() (bool, error) {
	rk, err := registry.OpenKey(registry.CURRENT_USER, regKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer rk.Close()

	v, _, err := rk.GetStringValue(AutoRunName)
	if errors.Is(err, registry.ErrNotExist) || errors.Is(err, registry.ErrUnexpectedType) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	path, err := self()
	if err != nil {
		return false, err
	}
	return autoRunCommandMatches(v, path), nil
}

func AutoRunDisable() error {
	rk, err := registry.OpenKey(registry.CURRENT_USER, regKey, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer rk.Close()

	err = rk.DeleteValue(AutoRunName)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}

func AutoRunEnable() error {
	path, err := self()
	if err != nil {
		return err
	}
	rk, _, err := registry.CreateKey(registry.CURRENT_USER, regKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer rk.Close()
	return rk.SetStringValue(AutoRunName, autoRunCommand(path))
}
