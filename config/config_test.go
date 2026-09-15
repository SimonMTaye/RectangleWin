package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestDefaults(t *testing.T) {
	c := Defaults()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.Keyboard.Super, []string{"left_ctrl", "left_alt", "right_shift"}) {
		t.Fatalf("unexpected super: %v", c.Keyboard.Super)
	}
	want := map[string]Binding{
		"edge_left": {"Left", []string{}}, "edge_right": {"Right", []string{}},
		"edge_top": {"Up", []string{}}, "edge_bottom": {"Down", []string{}},
		"corner_top_left": {"Left", []string{"win"}}, "corner_top_right": {"Up", []string{"win"}},
		"corner_bottom_left": {"Down", []string{"win"}}, "corner_bottom_right": {"Right", []string{"win"}},
		"maximize": {"F", []string{}}, "center": {"C", []string{}}, "always_on_top": {"A", []string{}},
	}
	if !reflect.DeepEqual(c.Keyboard.Bindings, want) {
		t.Fatalf("unexpected bindings: %v", c.Keyboard.Bindings)
	}
	c.Keyboard.Super[0] = "win"
	c.Keyboard.Bindings["corner_top_left"].Modifiers[0] = "alt"
	delete(c.Keyboard.Bindings, "center")
	if !reflect.DeepEqual(Defaults().Keyboard.Bindings, want) || Defaults().Keyboard.Super[0] != "left_ctrl" {
		t.Fatal("Defaults shares mutable state")
	}
}

func TestLoadFileCreateAndRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	c, err := LoadFile(path)
	if err != nil || !reflect.DeepEqual(c, Defaults()) {
		t.Fatalf("load: %v, %v", c, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(defaultsJSON) {
		t.Fatalf("created file differs from embedded defaults: %v", err)
	}
	custom := `{"keyboard":{"bindings":{"center":{"key":"Z"}}}}`
	writeConfig(t, path, custom)
	c, err = LoadFile(path)
	if err != nil || c.Keyboard.Bindings["center"].Key != "Z" {
		t.Fatalf("read custom config: %v, %v", c, err)
	}
	data, err = os.ReadFile(path)
	if err != nil || string(data) != custom {
		t.Fatal("existing file was modified", err)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("AppData", dir)
	case "darwin":
		t.Setenv("HOME", dir)
	default:
		t.Setenv("XDG_CONFIG_HOME", dir)
	}
	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	c, path, err := Load()
	if err != nil || path != filepath.Join(base, "RectangleWin", "config.json") || !reflect.DeepEqual(c, Defaults()) {
		t.Fatalf("Load: %v, %q, %v", c, path, err)
	}
	writeConfig(t, path, "bad JSON")
	_, failedPath, err := Load()
	if failedPath != path || err == nil || !strings.Contains(err.Error(), path) {
		// Quoted Windows paths escape backslashes in the error.
		if failedPath != path || err == nil || !strings.Contains(err.Error(), strconv.Quote(path)) {
			t.Fatalf("Load failure: %q, %v", failedPath, err)
		}
	}
}

func TestInheritance(t *testing.T) {
	for _, input := range []string{`{}`, `{"keyboard":{}}`, `{"keyboard":{"bindings":{}}}`, `{"keyboard":{"bindings":{"center":{}}}}`} {
		t.Run(input, func(t *testing.T) {
			c, err := loadText(t, input)
			if err != nil || !reflect.DeepEqual(c, Defaults()) {
				t.Fatalf("inherit defaults: %v, %v", c, err)
			}
		})
	}
	c, err := loadText(t, `{"keyboard":{"super":["ctrl"],"bindings":{"corner_top_left":{"key":"Z"},"corner_top_right":{"modifiers":["alt"]}}}}`)
	if err != nil {
		t.Fatal(err)
	}
	want := Defaults()
	want.Keyboard.Super = []string{"ctrl"}
	want.Keyboard.Bindings["corner_top_left"] = Binding{"Z", []string{"win"}}
	want.Keyboard.Bindings["corner_top_right"] = Binding{"Up", []string{"alt"}}
	if !reflect.DeepEqual(c, want) {
		t.Fatalf("partial overrides: got %v, want %v", c, want)
	}
	c, err = loadText(t, `{"keyboard":{"bindings":{"corner_top_left":{"key":"Z","modifiers":[]}}}}`)
	if err != nil || len(c.Keyboard.Bindings["corner_top_left"].Modifiers) != 0 {
		t.Fatalf("explicit empty array should replace defaults: %v", err)
	}
}

func TestLoadErrors(t *testing.T) {
	cases := []struct{ input, message string }{
		{"", "decode JSON"}, {"{", "decode JSON"}, {`[]`, "decode JSON"},
		{`{} {}`, "trailing document"}, {`{} garbage`, "trailing JSON"},
		{`{"unknown":true}`, "unknown field"}, {`{"keyboard":{"unknown":true}}`, "unknown field"},
		{`{"keyboard":{"bindings":{"center":{"unknown":true}}}}`, "unknown field"},
		{`{"keyboard":{"bindings":{"typo":{"key":"Z"}}}}`, "unknown action"},
		{`{"keyboard":{"super":[]}}`, "keyboard.super"},
		{`{"keyboard":{"super":"ctrl"}}`, "decode configuration fields"},
		{`{"keyboard":{"bindings":{"center":{"key":123}}}}`, "decode configuration fields"},
		{`{"keyboard":{"bindings":{"center":{"key":""}}}}`, "unsupported key"},
		{`null`, "null"}, {`{"keyboard":null}`, "null"},
		{`{"keyboard":{"super":null}}`, "null"}, {`{"keyboard":{"bindings":null}}`, "null"},
		{`{"keyboard":{"bindings":{"center":null}}}`, "null"},
		{`{"keyboard":{"bindings":{"center":{"key":null}}}}`, "null"},
		{`{"keyboard":{"bindings":{"center":{"modifiers":[null]}}}}`, "null"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			writeConfig(t, path, tc.input)
			_, err := LoadFile(path)
			if err == nil || !strings.Contains(err.Error(), tc.message) || !strings.Contains(err.Error(), strconv.Quote(path)) {
				t.Fatalf("expected path and %q: %v", tc.message, err)
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil || string(data) != tc.input {
				t.Fatalf("invalid configuration was changed: %v", readErr)
			}
		})
	}
	path := t.TempDir()
	if _, err := LoadFile(path); err == nil || !strings.Contains(err.Error(), strconv.Quote(path)) {
		t.Fatalf("directory read: %v", err)
	}
	blocker := filepath.Join(t.TempDir(), "file")
	writeConfig(t, blocker, "blocked")
	path = filepath.Join(blocker, "config.json")
	if _, err := LoadFile(path); err == nil || !strings.Contains(err.Error(), strconv.Quote(path)) {
		t.Fatalf("non-directory parent: %v", err)
	}
}

func TestKeyValidation(t *testing.T) {
	keys := []string{"Left", "Right", "Up", "Down", "Space", "Enter", "Tab", "Escape", "Home", "End", "PageUp", "PageDown", "Insert", "Delete", "Backspace"}
	for ch := 'A'; ch <= 'Z'; ch++ {
		keys = append(keys, string(ch))
	}
	for ch := '0'; ch <= '9'; ch++ {
		keys = append(keys, string(ch))
	}
	for i := 1; i <= 24; i++ {
		keys = append(keys, "F"+strconv.Itoa(i))
	}
	for _, key := range keys {
		if !validKey(key) {
			t.Errorf("rejected supported key %q", key)
		}
	}
	for _, key := range []string{"", "a", "left", "CTRL", "Esc", "F0", "F25", "F01", "F+1", " F", "AA", "é", "Numpad1"} {
		c := Defaults()
		c.Keyboard.Bindings["center"] = Binding{Key: key}
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "unsupported key") {
			t.Errorf("invalid key %q: %v", key, err)
		}
	}
}

func TestModifierValidation(t *testing.T) {
	for _, family := range modifierFamilies {
		for _, names := range [][]string{{family}, {"left_" + family}, {"right_" + family}, {"left_" + family, "right_" + family}} {
			if _, err := parseModifiers(names, "test"); err != nil {
				t.Errorf("valid modifiers %v: %v", names, err)
			}
		}
		for _, names := range [][]string{{family, family}, {family, "left_" + family}, {"right_" + family, family}, {"left_" + family, "left_" + family}} {
			c := Defaults()
			c.Keyboard.Super = names
			if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "redundant") {
				t.Errorf("invalid modifiers %v: %v", names, err)
			}
		}
	}
	for _, name := range []string{"", "Ctrl", "control", "super", "left_left_ctrl"} {
		c := Defaults()
		c.Keyboard.Bindings["center"] = Binding{"C", []string{name}}
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "unknown modifier") {
			t.Errorf("invalid modifier %q: %v", name, err)
		}
	}
	c := Defaults()
	c.Keyboard.Super = nil
	if err := c.Validate(); err == nil {
		t.Fatal("empty super accepted")
	}
	c = Defaults()
	delete(c.Keyboard.Bindings, "center")
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "missing action") {
		t.Fatalf("missing action: %v", err)
	}
}

func TestSuperOverlap(t *testing.T) {
	for _, tc := range []struct {
		super, extra []string
		valid        bool
	}{
		{[]string{"ctrl"}, []string{"left_ctrl"}, false},
		{[]string{"left_ctrl"}, []string{"ctrl"}, false},
		{[]string{"left_ctrl"}, []string{"left_ctrl"}, false},
		{[]string{"left_ctrl"}, []string{"right_ctrl"}, true},
		{[]string{"left_ctrl", "right_ctrl"}, []string{"ctrl"}, false},
		{[]string{"ctrl"}, []string{"left_shift", "right_shift"}, true},
		{[]string{"ctrl"}, []string{"shift", "left_shift"}, false},
	} {
		c := Defaults()
		c.Keyboard.Super = tc.super
		c.Keyboard.Bindings["center"] = Binding{"C", tc.extra}
		err := c.Validate()
		if (err == nil) != tc.valid {
			t.Errorf("super %v extra %v: %v", tc.super, tc.extra, err)
		}
	}
}

func TestShortcutConflicts(t *testing.T) {
	for _, tc := range []struct {
		name     string
		a, b     []string
		conflict bool
	}{
		{"identical", nil, nil, true},
		{"order", []string{"alt", "shift"}, []string{"shift", "alt"}, true},
		{"generic-left", []string{"shift"}, []string{"left_shift"}, true},
		{"generic-right", []string{"right_shift"}, []string{"shift"}, true},
		{"generic-both", []string{"shift"}, []string{"left_shift", "right_shift"}, true},
		{"distinct-sides", []string{"left_shift"}, []string{"right_shift"}, false},
		{"both-versus-left", []string{"left_shift", "right_shift"}, []string{"left_shift"}, false},
		{"extra-family", nil, []string{"shift"}, false},
		{"different-family", []string{"alt"}, []string{"shift"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Defaults()
			c.Keyboard.Super = []string{"ctrl"}
			c.Keyboard.Bindings["center"] = Binding{"Z", append([]string{}, tc.a...)}
			c.Keyboard.Bindings["maximize"] = Binding{"Z", append([]string{}, tc.b...)}
			err := c.Validate()
			if (err != nil) != tc.conflict {
				t.Fatalf("conflict=%v: %v", tc.conflict, err)
			}
			if err != nil && (!strings.Contains(err.Error(), "center") || !strings.Contains(err.Error(), "maximize")) {
				t.Fatalf("conflict must identify both actions: %v", err)
			}
			data, err := json.Marshal(c)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := loadText(t, string(data)); (err != nil) != tc.conflict {
				t.Fatalf("LoadFile conflict=%v: %v", tc.conflict, err)
			}
		})
	}
}

func writeConfig(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func loadText(t *testing.T, text string) (Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfig(t, path, text)
	return LoadFile(path)
}
