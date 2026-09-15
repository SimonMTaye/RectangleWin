package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// SaveFile validates and replaces a configuration using a temporary file in the
// same directory. Failed validation or writes leave the existing file untouched.
func SaveFile(path string, c Config) error {
	if err := c.Validate(); err != nil {
		return fmt.Errorf("configuration %q: %w", path, err)
	}
	// Nil modifier slices are valid in memory but null is not valid configuration JSON.
	bindings := make(map[string]Binding, len(c.Keyboard.Bindings))
	for action, b := range c.Keyboard.Bindings {
		if b.Modifiers == nil {
			b.Modifiers = []string{}
		}
		bindings[action] = b
	}
	c.Keyboard.Bindings = bindings
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("encode configuration %q: %w", path, err)
	}
	data = append(data, '\n')
	f, err := os.CreateTemp(filepath.Dir(path), ".rectanglewin-*.json")
	if err != nil {
		return fmt.Errorf("create temporary configuration %q: %w", path, err)
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("write configuration %q: %w", path, err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("sync configuration %q: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close configuration %q: %w", path, err)
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return fmt.Errorf("replace configuration %q: %w", path, err)
	}
	return nil
}
