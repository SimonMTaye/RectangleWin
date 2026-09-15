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
	_ "embed"
	"fmt"

	"github.com/getlantern/systray"
	"github.com/gonutz/w32/v2"
)

//go:embed assets/tray_icon.ico
var icon []byte

const repo = "https://github.com/ahmetb/RectangleWin"

func initTray(configPath string) {
	systray.Register(func() { onReady(configPath) }, onExit)
}

func onReady(configPath string) {
	systray.SetIcon(icon)
	systray.SetTitle("RectangleWin")
	systray.SetTooltip("RectangleWin")

	autorun, err := AutoRunEnabled()
	if err != nil {
		panic(err)
	}

	mSettings := systray.AddMenuItem("Settings...", "Configure keyboard shortcuts and start on login")
	mConfig := systray.AddMenuItem("Open configuration", "Restart RectangleWin after editing settings")
	go func() {
		for range mConfig.ClickedCh {
			if err := w32.ShellExecute(0, "open", "notepad.exe", "\""+configPath+"\"", "", w32.SW_SHOWNORMAL); err != nil {
				showMessageBox(fmt.Sprintf("Could not open configuration %q: %v", configPath, err))
			}
		}
	}()

	mRepo := systray.AddMenuItem("Documentation", "")
	go func() {
		for range mRepo.ClickedCh {
			if err := w32.ShellExecute(0, "open", repo, "", "", w32.SW_SHOWNORMAL); err != nil {
				fmt.Printf("failed to launch browser: (%d), %v\n", w32.GetLastError(), err)
			}
		}
	}()

	systray.AddSeparator()

	mAutoRun := systray.AddMenuItemCheckbox("Start on login", "", autorun)
	updateAutoRun := func(enabled bool) {
		if enabled {
			mAutoRun.Check()
		} else {
			mAutoRun.Uncheck()
		}
	}
	go func() {
		for range mSettings.ClickedCh {
			showSettings(configPath, updateAutoRun)
		}
	}()
	go func() {
		for range mAutoRun.ClickedCh {
			func() {
				autoRunSettingsMu.Lock()
				defer autoRunSettingsMu.Unlock()
				enabled, err := AutoRunEnabled()
				if err == nil {
					err = settingsSetAutoRun(!enabled)
				}
				if err != nil {
					showMessageBox(fmt.Sprintf("Could not change start on login: %v", err))
					return
				}
				updateAutoRun(!enabled)
			}()
		}
	}()

	systray.AddSeparator()

	mQuit := systray.AddMenuItem("Quit", "")
	go func() {
		<-mQuit.ClickedCh
		fmt.Println("clicked Quit")
		systray.Quit()
	}()

	fmt.Println("tray ready")
}

func onExit() {
	fmt.Println("onExit invoked")
}
