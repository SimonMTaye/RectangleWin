package keyboard_test

import (
	"fmt"
	"sort"
	"testing"

	"github.com/ahmetb/RectangleWin/config"
	"github.com/ahmetb/RectangleWin/keyboard"
)

var sides = [4][2]int{{0xA2, 0xA3}, {0xA4, 0xA5}, {0xA0, 0xA1}, {0x5B, 0x5C}}

func event(t *testing.T, m *keyboard.Matcher, vk int, down bool, action string, suppress bool) {
	t.Helper()
	gotAction, gotSuppress := m.Handle(vk, down)
	if gotAction != action || gotSuppress != suppress {
		t.Fatalf("Handle(%#x, %t) = (%q, %t), want (%q, %t)", vk, down, gotAction, gotSuppress, action, suppress)
	}
}

func newMatcher(t *testing.T, c config.Keyboard) *keyboard.Matcher {
	t.Helper()
	if err := (config.Config{Keyboard: c}).Validate(); err != nil {
		t.Fatal(err)
	}
	return keyboard.New(c)
}

// Give other actions distinct keys so tests can freely override modifiers.
func custom(super, extra []string, key string) config.Keyboard {
	c := config.Defaults().Keyboard
	c.Super = super
	actions := make([]string, 0, len(c.Bindings))
	for action := range c.Bindings {
		actions = append(actions, action)
	}
	sort.Strings(actions)
	for i, action := range actions {
		c.Bindings[action] = config.Binding{Key: fmt.Sprintf("F%d", i+1)}
	}
	c.Bindings["edge_left"] = config.Binding{Key: key, Modifiers: extra}
	return c
}

func TestDesktopSwitchReinitialization(t *testing.T) {
	for _, consumed := range []bool{false, true} {
		t.Run(fmt.Sprintf("consumed=%t", consumed), func(t *testing.T) {
			cfg := config.Defaults().Keyboard
			m := newMatcher(t, cfg)
			if consumed {
				for _, vk := range []int{0xA2, 0xA4, 0xA1} {
					m.SetKeyDown(vk, true)
				}
				event(t, m, 0x25, true, "edge_left", true)
			} else {
				event(t, m, 0x25, true, "", false)
			}
			// The platform adapter replaces the matcher on a desktop switch:
			// releases delivered on another desktop must not leave held state.
			m = keyboard.New(cfg)
			event(t, m, 0x25, true, "", false)
			event(t, m, 0x25, false, "", false)
			for _, vk := range []int{0xA2, 0xA4, 0xA1} {
				m.SetKeyDown(vk, true)
			}
			event(t, m, 0x25, true, "edge_left", true)
			event(t, m, 0x25, false, "", true)
		})
	}
}

func TestDefaults(t *testing.T) {
	cases := []struct {
		action string
		vk     int
		corner bool
	}{
		{"edge_left", 0x25, false}, {"edge_right", 0x27, false},
		{"edge_top", 0x26, false}, {"edge_bottom", 0x28, false},
		{"corner_top_left", 0x25, true}, {"corner_top_right", 0x26, true},
		{"corner_bottom_left", 0x28, true}, {"corner_bottom_right", 0x27, true},
		{"maximize", 'F', false}, {"center", 'C', false}, {"always_on_top", 'A', false},
	}
	for _, tc := range cases {
		for win := 0; win < 4; win++ {
			t.Run(fmt.Sprintf("%s/win%d", tc.action, win), func(t *testing.T) {
				m := newMatcher(t, config.Defaults().Keyboard)
				for _, vk := range []int{0xA2, 0xA4, 0xA1} {
					event(t, m, vk, true, "", false)
				}
				for side, vk := range sides[3] {
					m.SetKeyDown(vk, win&(1<<uint(side)) != 0)
				}
				want := tc.action
				if tc.corner && win == 0 {
					want = map[int]string{0x25: "edge_left", 0x27: "edge_right", 0x26: "edge_top", 0x28: "edge_bottom"}[tc.vk]
				} else if !tc.corner && win != 0 {
					want = map[int]string{0x25: "corner_top_left", 0x27: "corner_bottom_right", 0x26: "corner_top_right", 0x28: "corner_bottom_left"}[tc.vk]
				}
				event(t, m, tc.vk, true, want, want != "")
				event(t, m, tc.vk, true, "", want != "")
				event(t, m, tc.vk, false, "", want != "")
			})
		}
	}
}

func TestExactModifierStates(t *testing.T) {
	for family, name := range []string{"ctrl", "alt", "shift", "win"} {
		for _, requirement := range []struct {
			label string
			super []string
			extra []string
			mask  int
		}{
			{"generic", []string{name}, nil, 4},
			{"left", []string{"left_" + name}, nil, 1},
			{"right", []string{"right_" + name}, nil, 2},
			{"both", []string{"left_" + name, "right_" + name}, nil, 3},
			{"split", []string{"left_" + name}, []string{"right_" + name}, 3},
		} {
			t.Run(name+"/"+requirement.label, func(t *testing.T) {
				m := newMatcher(t, custom(requirement.super, requirement.extra, "Z"))
				// Enumerate every physical combination, including unrelated families.
				for physical := 0; physical < 256; physical++ {
					for f, keys := range sides {
						for side, vk := range keys {
							m.SetKeyDown(vk, physical&(1<<uint(f*2+side)) != 0)
						}
					}
					actual := (physical >> uint(family*2)) & 3
					match := physical == actual<<uint(family*2) && (actual == requirement.mask || requirement.mask == 4 && actual != 0)
					want := ""
					if match {
						want = "edge_left"
					}
					event(t, m, 'Z', true, want, match)
					event(t, m, 'Z', false, "", match)
				}
			})
		}
	}
}

func TestHeldKeysAndReleaseOrdering(t *testing.T) {
	m := newMatcher(t, config.Defaults().Keyboard)
	event(t, m, 0x25, true, "", false)
	for _, vk := range []int{0xA2, 0xA4, 0xA1} {
		event(t, m, vk, true, "", false)
		event(t, m, vk, true, "", false)
	}
	// A previously passed key cannot become a shortcut on an autorepeat.
	m.SetKeyDown(0x25, false)
	event(t, m, 0x25, true, "", false)
	event(t, m, 0x25, false, "", false)
	event(t, m, 0x25, true, "edge_left", true)
	event(t, m, 0x27, true, "edge_right", true)
	// Changing modifiers must neither retrigger nor forget consumed keys.
	event(t, m, 0x5B, true, "", false)
	event(t, m, 0x25, true, "", true)
	for _, vk := range []int{0xA2, 0xA4, 0xA1, 0x5B} {
		event(t, m, vk, false, "", false)
	}
	m.SetKeyDown(0x25, false)
	event(t, m, 0x25, true, "", true)
	event(t, m, 0x25, false, "", true)
	event(t, m, 0x25, false, "", false)
	event(t, m, 0x27, true, "", true)
	event(t, m, 0x27, false, "", true)
	event(t, m, 0x25, true, "", false)
	event(t, m, 0x25, false, "", false)
	// Resync affects modifier state but does not reset held action keys.
	for _, vk := range []int{0xA2, 0xA4, 0xA1} {
		m.SetKeyDown(vk, true)
	}
	event(t, m, 0x25, true, "edge_left", true)
	m.SetKeyDown(0xA2, false)
	event(t, m, 0x25, true, "", true)
	event(t, m, 0x25, false, "", true)
	event(t, m, 0x25, true, "", false)
	m.SetKeyDown(0xA2, true)
	event(t, m, 0x25, true, "", false)
	event(t, m, 0x25, false, "", false)
	event(t, m, 0x25, true, "edge_left", true)
	event(t, m, 0x25, false, "", true)
	event(t, m, 0x25, true, "edge_left", true)
}

func TestPassthroughAndModifierOnlyInitialization(t *testing.T) {
	m := newMatcher(t, custom([]string{"ctrl"}, nil, "Z"))
	m.SetKeyDown(0xA2, true)
	m.SetKeyDown('Z', true) // Non-modifiers must not seed held state.
	event(t, m, 'Z', true, "edge_left", true)
	for _, vk := range []int{'Q', 0, -1, 0x100, 0xBA} {
		event(t, m, vk, false, "", false)
		event(t, m, vk, true, "", false)
		event(t, m, vk, true, "", false)
		event(t, m, vk, false, "", false)
	}
	for _, keys := range sides {
		for _, vk := range keys {
			event(t, m, vk, true, "", false)
			event(t, m, vk, true, "", false)
			event(t, m, vk, false, "", false)
		}
	}
	event(t, m, 'Z', false, "", true)
}

func TestSupportedKeysAndOverrides(t *testing.T) {
	keys := map[string]int{
		"Left": 0x25, "Right": 0x27, "Up": 0x26, "Down": 0x28,
		"Space": 0x20, "Enter": 0x0D, "Tab": 0x09, "Escape": 0x1B,
		"Home": 0x24, "End": 0x23, "PageUp": 0x21, "PageDown": 0x22,
		"Insert": 0x2D, "Delete": 0x2E, "Backspace": 0x08,
	}
	for key := 'A'; key <= 'Z'; key++ {
		keys[string(key)] = int(key)
	}
	for key := '0'; key <= '9'; key++ {
		keys[string(key)] = int(key)
	}
	for n := 1; n <= 24; n++ {
		keys[fmt.Sprintf("F%d", n)] = 0x6F + n
	}
	for key, vk := range keys {
		t.Run(key, func(t *testing.T) {
			c := custom([]string{"right_ctrl"}, []string{"left_alt"}, key)
			m := newMatcher(t, c)
			// Construction must not retain mutable config slices or maps.
			c.Super[0] = "win"
			c.Bindings["edge_left"].Modifiers[0] = "shift"
			delete(c.Bindings, "edge_left")
			m.SetKeyDown(0xA2, true)
			m.SetKeyDown(0xA4, true)
			event(t, m, vk, true, "", false)
			event(t, m, vk, false, "", false)
			m.SetKeyDown(0xA2, false)
			m.SetKeyDown(0xA3, true)
			event(t, m, vk, true, "edge_left", true)
			event(t, m, vk, false, "", true)
		})
	}
}
