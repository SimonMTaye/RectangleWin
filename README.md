# RectangleWin

Fork of [RectangleWin](https://github.com/ahmetb/RectangleWin)

- As per the author, I wanted rectangle-like keyboard bindings on Windows for snapping and navigating in general
- This fork adds a few modifications to the original
    - Configurable shortcuts

A minimalistic Windows rewrite of macOS
[Rectangle.app](https://rectangleapp.com)/[Spectacle.app](https://www.spectacleapp.com/).
([Why?](#why))

A hotkey-oriented window snapping and resizing tool for Windows.

This animation illustrates how RectangleWin helps me move windows to edges
and corners (and cycle through half, one-thirds or two thirds width or height)
only using hotkeys:

![RectangleWin demo](./assets/RectangleWin-demo.gif)

## Install

1. Clone repo and run build command described below.


2. Launch the `.exe` file. Now the program icon should be visible on system
   tray!

3. Optionally enable **Start on login** in the tray menu or **Settings...** to
   launch RectangleWin automatically when you sign in.

## Keyboard Bindings

**Super = Left Ctrl + Left Alt + Right Shift** by default. Hold these modifiers,
then press the action key. This is an application shortcut prefix, not a system-wide
remapping of the Windows key. The specified sides matter; additional modifiers
prevent a match unless that binding requires them.

| Action | Default shortcut |
| --- | --- |
| Snap left / right / top / bottom | Super + Left / Right / Up / Down |
| Left / middle / right third (full height) | Super + 1 / 2 / 3 |
| Top / middle / bottom third (full width) | Super + Win + 1 / 2 / 3 |
| Snap top-left | Super + Win + Left |
| Snap top-right | Super + Win + Up |
| Snap bottom-left | Super + Win + Down |
| Snap bottom-right | Super + Win + Right |
| Center | Super + C |
| Maximize | Super + F / Space |
| Toggle always on top | Super + A |
| Toggle TODO mode for the current window | Super + T |

## Configuration

### TODO mode

Super + T pins the current window above other windows in the left quarter of its
monitor's usable area. RectangleWin's maximize (including Super + Space), snaps,
thirds, and center then use the remaining area on that monitor. Other monitors
keep their full area. For example, on a 1920-pixel-wide work area, TODO takes 480
pixels and each remaining third is 480 pixels wide, starting at x=480.

Press Super + T on the TODO window again to release it and restore its previous
placement and always-on-top state. Selecting another TODO window on the same
monitor replaces the old one; separate monitors can each have one. RectangleWin
layout shortcuts keep the TODO window in its reserved region, and Super + A keeps
it on top until TODO mode is disabled. Closing it or moving it to another monitor
releases its reservation on the next RectangleWin action. Minimizing it keeps the
reservation. Quitting RectangleWin restores TODO windows; mode is not persisted.

Add a top-level setting to your config to change the reserved side and amount:

```json
"todo": {"side": "left", "fraction": 0.25}
```

Sides are `left`, `right`, `top`, or `bottom`; fraction must be greater than 0
and less than 1. Restart RectangleWin after changes. The shortcut is configurable
under `keyboard.bindings.todo`. Existing configs inherit these defaults.

The reservation applies to RectangleWin commands. Native Windows maximize,
Win+Arrow, and manual dragging do not honor it. Windows with an application-enforced
minimum size may not fit a small region.

### Configuration file

RectangleWin creates `%AppData%\RectangleWin\config.json` on first launch. Choose
**Settings...** from the tray menu for an optional native settings pop-up. Edit
Super modifiers, action keys, and additional modifiers; separate modifiers with
commas. **Save** validates shortcuts before writing; **Cancel** discards edits.
The window only appears when requested, never automatically at startup.

**Start on login** is optional and applies immediately when saved. It uses the
current user's Windows Run registry entry (no administrator access required),
not a second value in JSON. The tray checkbox controls the same setting.
Keep the executable at a stable location; re-enable this option if you move it.

**Open configuration** remains available for editing JSON directly in Notepad.
**Quit and restart RectangleWin** to apply shortcut changes from either editor.
The configuration path is independent of the executable location and working
directory, including when launched at login.

Repository defaults live in [`config/defaults.json`](config/defaults.json) and are
embedded in the executable; no separate defaults file needs to be distributed.
The user file overrides those defaults. Omitted fields inherit defaults, objects
merge, and arrays replace defaults. Loading never overwrites existing files;
explicitly saving in Settings writes the complete validated configuration. Invalid
settings produce an error dialog with the file path and prevent startup.


An example configuration:

```json
{
  "keyboard": {
    "super": ["left_ctrl", "left_alt", "right_shift"],
    "bindings": {
      "center": { "key": "Z", "modifiers": [] }
    }
  }
}
```

Each binding combines `keyboard.super`, its own extra `modifiers`, and its `key`.
Modifiers support `ctrl`, `alt`, `shift`, `win` (either side), or their `left_` and
`right_` variants. Key names are case-sensitive: `A`–`Z`, `0`–`9`, `F1`–`F24`,
`Left`, `Right`, `Up`, `Down`, `Space`, `Enter`, `Tab`, `Escape`, `Home`, `End`,
`PageUp`, `PageDown`, `Insert`, `Delete`, `Backspace`.


### Keyboard Shortcut Handling

Shortcuts use a low-level keyboard hook to distinguish modifier sides. Only matched
action keys are consumed; modifiers and unmatched keys pass through normally.
Consequently, OS shortcuts assigned to the modifiers themselves (such as an
Alt+Shift input-language switch) can still run; disable conflicting Windows bindings
if necessary. Unlike `RegisterHotKey`, hooks do not reserve shortcuts exclusively or
detect conflicts with other utilities. Actions are queued in order; if window
operations stall and all 64 queue slots fill, further actions are discarded to keep
keyboard input responsive. Queued actions are discarded if their target is no
longer foreground or a desktop switch has occurred. Desktop switches also reset
held-key tracking to recover from releases missed on the lock screen.

### Architecture and future settings

- [`config/`](config/README.md): the shared typed `Config`, canonical defaults,
  file loading, default merging, and validation. No Windows dependencies.
- `main.go`: loads configuration once at startup and passes settings to consumers.
  New components should receive their relevant settings, not read files themselves.
- `keyboard/`: platform-independent shortcut matching and key-state handling.
- `hotkey.go`: Windows hook on a dedicated message-pump thread, with a separate
  serialized action executor so resizing and tray menus cannot block the hook.
- `tray.go`: exposes Settings, JSON editing, and the start-on-login toggle.
- `settings_windows.go`: on-demand native settings window with its own message pump.
- `autorun.go`: per-user Windows login registration; independent of shortcut JSON.

To add a setting, extend `config.Config` (or a nested settings type), add its default
in `config/defaults.json`, add validation/tests, and pass it to the consuming component.
Existing partial configurations inherit newly introduced defaults.

## Why?

I like Rectangle on macOS and want to make use of the muscle memory I have developed already. Fork was mainly to add configuration / rebinding
of keys, particularly to allow `CapsLock` to be the leader key after rebinding `CapsLock` to `Left Ctrl + Left Alt + Right Shift`

## Development (Install from source)

> **NOTE: My changes were made using Codex for my own use only. I will only edit this repo to fit my own needs; please clone and customize for yourself if necessary!**

With Go 1.17+ installed, clone this repository and run:

```sh
go generate
GOOS=windows go build -ldflags -H=windowsgui .
```

The `RectangleWin.exe` will be available in the same directory.

Platform-independent configuration and matching tests can run on any supported Go host:

```sh
go test ./config ./keyboard ./layout
go vet ./config ./keyboard ./layout
```

Test actual keyboard input on Windows, including wrong modifier sides, key repeats,
modifier-first releases, the tray menu, and switching away to the lock screen and back.
On Windows, also test Settings Save/Cancel, invalid/conflicting shortcuts, reopening
and keyboard navigation, and synchronization with the tray login toggle. Verify
login launch with the executable in a path containing spaces and no launch after
disabling it. Native UI and registry integration require a Windows smoke test.

## License

This project is distributed as-is under the Apache 2.0 license.
See [LICENSE](./LICENSE).

If you see bugs, please open issues. I can't promise any fixes.
