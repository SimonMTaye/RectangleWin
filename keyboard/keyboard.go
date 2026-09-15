// Package keyboard matches configured shortcuts without platform dependencies.
package keyboard

import (
	"strconv"
	"strings"

	"github.com/ahmetb/RectangleWin/config"
)

// Modifier masks use bits 1 and 2 for required sides, or 4 for either/both.
type modifiers [4]uint8

var modifierKeys = [4][2]int{
	{0xA2, 0xA3}, // Ctrl
	{0xA4, 0xA5}, // Alt
	{0xA0, 0xA1}, // Shift
	{0x5B, 0x5C}, // Win
}

type chord struct {
	action string
	mods   modifiers
}

type keyState struct {
	down      bool
	swallowed bool
}

// Matcher tracks physical key transitions and consumed shortcut presses.
// Calls to its methods must be serialized by the caller.
type Matcher struct {
	bindings map[int][]chord
	keys     map[int]keyState
}

// New compiles an already validated keyboard configuration into an independent
// matcher. All physical keys are initially considered released.
func New(c config.Keyboard) *Matcher {
	m := &Matcher{bindings: make(map[int][]chord), keys: make(map[int]keyState)}
	super := parseModifiers(c.Super)
	for action, binding := range c.Bindings {
		mods := parseModifiers(binding.Modifiers)
		for i := range mods {
			mods[i] |= super[i]
		}
		vk := keyCode(binding.Key)
		m.bindings[vk] = append(m.bindings[vk], chord{action, mods})
	}
	return m
}

// SetKeyDown initializes or resynchronizes a sided physical modifier state.
// Non-modifier keys are ignored; this never triggers or consumes an action.
func (m *Matcher) SetKeyDown(vk int, down bool) {
	if isModifier(vk) {
		if down {
			m.keys[vk] = keyState{down: true}
		} else {
			delete(m.keys, vk)
		}
	}
}

// Handle processes a physical key event. An action is emitted only on a fresh
// matching key-down. Its repeats and eventual key-up remain suppressed even if
// modifiers change. Modifier events always pass through.
func (m *Matcher) Handle(vk int, down bool) (action string, suppress bool) {
	if isModifier(vk) {
		m.SetKeyDown(vk, down)
		return "", false
	}
	state := m.keys[vk]
	if !down {
		delete(m.keys, vk)
		return "", state.swallowed
	}
	if state.down {
		return "", state.swallowed
	}
	state.down = true
	for _, binding := range m.bindings[vk] {
		if m.matches(binding.mods) {
			action = binding.action
			state.swallowed = true
			break
		}
	}
	m.keys[vk] = state
	return action, state.swallowed
}

func (m *Matcher) matches(want modifiers) bool {
	for family, sides := range modifierKeys {
		var actual uint8
		for side, vk := range sides {
			if m.keys[vk].down {
				actual |= 1 << uint(side)
			}
		}
		if want[family] == 4 {
			if actual == 0 {
				return false
			}
		} else if want[family] != actual {
			return false
		}
	}
	return true
}

func isModifier(vk int) bool {
	for _, sides := range modifierKeys {
		if vk == sides[0] || vk == sides[1] {
			return true
		}
	}
	return false
}

func parseModifiers(names []string) modifiers {
	var result modifiers
	for _, name := range names {
		var side uint8 = 4
		if strings.HasPrefix(name, "left_") {
			name, side = strings.TrimPrefix(name, "left_"), 1
		} else if strings.HasPrefix(name, "right_") {
			name, side = strings.TrimPrefix(name, "right_"), 2
		}
		for family, candidate := range [...]string{"ctrl", "alt", "shift", "win"} {
			if name == candidate {
				result[family] |= side
				break
			}
		}
	}
	return result
}

func keyCode(key string) int {
	if len(key) == 1 {
		return int(key[0])
	}
	if strings.HasPrefix(key, "F") {
		n, _ := strconv.Atoi(key[1:])
		return 0x70 + n - 1
	}
	return map[string]int{
		"Left": 0x25, "Right": 0x27, "Up": 0x26, "Down": 0x28,
		"Space": 0x20, "Enter": 0x0D, "Tab": 0x09, "Escape": 0x1B,
		"Home": 0x24, "End": 0x23, "PageUp": 0x21, "PageDown": 0x22,
		"Insert": 0x2D, "Delete": 0x2E, "Backspace": 0x08,
	}[key]
}
