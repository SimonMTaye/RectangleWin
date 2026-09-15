package config

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Validate checks action names, canonical key/modifier names and shortcut conflicts.
// Chords match exact modifier states: left and right variants are distinct, while
// a generic modifier accepts either or both sides. Requiring both sides is valid.
func (c Config) Validate() error {
	if len(c.Keyboard.Super) == 0 {
		return fmt.Errorf("keyboard.super: at least one modifier is required")
	}
	super, err := parseModifiers(c.Keyboard.Super, "keyboard.super")
	if err != nil {
		return err
	}
	defaults := Defaults()
	actions := make([]string, 0, len(c.Keyboard.Bindings))
	for action := range c.Keyboard.Bindings {
		actions = append(actions, action)
	}
	sort.Strings(actions)
	chords := make(map[string]modifierSet, len(actions))
	for _, action := range actions {
		field := "keyboard.bindings." + action
		if _, ok := defaults.Keyboard.Bindings[action]; !ok {
			return fmt.Errorf("%s: unknown action", field)
		}
		binding := c.Keyboard.Bindings[action]
		if !validKey(binding.Key) {
			return fmt.Errorf("%s.key: unsupported key %q; use A-Z, 0-9, F1-F24, arrows or a supported navigation key", field, binding.Key)
		}
		extra, err := parseModifiers(binding.Modifiers, field+".modifiers")
		if err != nil {
			return err
		}
		chord := super
		for family, mod := range extra {
			if super[family] != 0 && mod != 0 && (super[family]&mod != 0 || super[family] == 4 || mod == 4) {
				return fmt.Errorf("%s.modifiers: %s modifier overlaps keyboard.super", field, modifierFamilies[family])
			}
			chord[family] |= mod
		}
		chords[action] = chord
	}
	for _, action := range sortedDefaultActions(defaults) {
		if _, ok := c.Keyboard.Bindings[action]; !ok {
			return fmt.Errorf("keyboard.bindings.%s: missing action", action)
		}
	}
	for i, action := range actions {
		for _, other := range actions[:i] {
			if c.Keyboard.Bindings[action].Key == c.Keyboard.Bindings[other].Key && chordsOverlap(chords[action], chords[other]) {
				return fmt.Errorf("keyboard.bindings.%s: shortcut overlaps keyboard.bindings.%s (key %q)", action, other, c.Keyboard.Bindings[action].Key)
			}
		}
	}
	return nil
}

func sortedDefaultActions(c Config) []string {
	actions := make([]string, 0, len(c.Keyboard.Bindings))
	for action := range c.Keyboard.Bindings {
		actions = append(actions, action)
	}
	sort.Strings(actions)
	return actions
}

var modifierFamilies = [...]string{"ctrl", "alt", "shift", "win"}

// Bits 1 and 2 require the respective sides; bit 4 means either side.
type modifierSet [4]uint8

func parseModifiers(names []string, field string) (modifierSet, error) {
	var result modifierSet
	for _, name := range names {
		familyName := name
		var side uint8 = 4
		if strings.HasPrefix(name, "left_") {
			familyName, side = strings.TrimPrefix(name, "left_"), 1
		} else if strings.HasPrefix(name, "right_") {
			familyName, side = strings.TrimPrefix(name, "right_"), 2
		}
		family := -1
		for i, candidate := range modifierFamilies {
			if familyName == candidate {
				family = i
				break
			}
		}
		if family == -1 {
			return result, fmt.Errorf("%s: unknown modifier %q; use ctrl, alt, shift, win or their left_/right_ variants", field, name)
		}
		old := result[family]
		if old != 0 && (old&side != 0 || old == 4 || side == 4) {
			return result, fmt.Errorf("%s: duplicate or redundant modifier %q", field, name)
		}
		result[family] |= side
	}
	return result, nil
}

func chordsOverlap(a, b modifierSet) bool {
	for i := range a {
		if a[i] == b[i] {
			continue
		}
		if a[i] == 0 || b[i] == 0 || (a[i] != 4 && b[i] != 4) {
			return false
		}
	}
	return true
}

func validKey(key string) bool {
	if len(key) == 1 && (key[0] >= 'A' && key[0] <= 'Z' || key[0] >= '0' && key[0] <= '9') {
		return true
	}
	if strings.HasPrefix(key, "F") {
		n, err := strconv.Atoi(key[1:])
		if err == nil && n >= 1 && n <= 24 && key == "F"+strconv.Itoa(n) {
			return true
		}
	}
	switch key {
	case "Left", "Right", "Up", "Down", "Space", "Enter", "Tab", "Escape", "Home", "End", "PageUp", "PageDown", "Insert", "Delete", "Backspace":
		return true
	}
	return false
}
