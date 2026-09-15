// Package config loads the unified RectangleWin configuration without platform dependencies.
package config

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

//go:embed defaults.json
var defaultsJSON []byte

// Config holds all application settings.
type Config struct {
	Keyboard Keyboard `json:"keyboard"`
}

// Keyboard defines the common modifiers and per-action shortcuts.
type Keyboard struct {
	Super    []string           `json:"super"`
	Bindings map[string]Binding `json:"bindings"`
}

// Binding adds Modifiers to Keyboard.Super when matching Key.
type Binding struct {
	Key       string   `json:"key"`
	Modifiers []string `json:"modifiers"`
}

// Defaults returns an independent copy of the embedded canonical defaults.
func Defaults() Config {
	var c Config
	if err := json.Unmarshal(defaultsJSON, &c); err != nil {
		panic(fmt.Sprintf("config: invalid embedded defaults: %v", err))
	}
	return c
}

// Load loads the user's RectangleWin/config.json, creating it if missing.
// The resolved path is returned even when loading fails (unless resolution fails).
func Load() (Config, string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return Config{}, "", fmt.Errorf("resolve RectangleWin configuration directory: %w", err)
	}
	path := filepath.Join(dir, "RectangleWin", "config.json")
	c, err := LoadFile(path)
	return c, path, err
}

// LoadFile creates a missing parent directory and file using the embedded defaults,
// then loads and validates path. Existing files are never rewritten. Omitted object
// fields inherit defaults, including fields within individual bindings; arrays
// replace defaults. Explicit nulls are not supported.
func LoadFile(path string) (Config, error) {
	c, err := loadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("configuration %q: %w", path, err)
	}
	return c, nil
}

func loadFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return Config{}, fmt.Errorf("create parent directory: %w", err)
		}
		// Exclusive creation protects an existing configuration from replacement.
		f, createErr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if createErr == nil {
			_, writeErr := f.Write(defaultsJSON)
			closeErr := f.Close()
			if writeErr != nil {
				return Config{}, fmt.Errorf("write defaults: %w", writeErr)
			}
			if closeErr != nil {
				return Config{}, fmt.Errorf("close defaults file: %w", closeErr)
			}
		} else if !os.IsExist(createErr) {
			return Config{}, fmt.Errorf("create defaults file: %w", createErr)
		}
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return Config{}, fmt.Errorf("read file: %w", err)
	}

	var overrides map[string]interface{}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&overrides); err != nil {
		return Config{}, fmt.Errorf("decode JSON: %w", err)
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Config{}, fmt.Errorf("decode JSON: trailing document; expected one object")
		}
		return Config{}, fmt.Errorf("decode trailing JSON: %w", err)
	}
	if err := rejectNull(overrides, "config"); err != nil {
		return Config{}, err
	}
	var merged map[string]interface{}
	if err := json.Unmarshal(defaultsJSON, &merged); err != nil {
		return Config{}, fmt.Errorf("decode embedded defaults: %w", err)
	}
	mergeObjects(merged, overrides)
	data, err = json.Marshal(merged)
	if err != nil {
		return Config{}, fmt.Errorf("merge defaults: %w", err)
	}
	var c Config
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("decode configuration fields: %w", err)
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func rejectNull(value interface{}, field string) error {
	switch v := value.(type) {
	case nil:
		return fmt.Errorf("%s: null is not allowed; omit the field to inherit defaults", field)
	case map[string]interface{}:
		if v == nil {
			return fmt.Errorf("%s: expected a JSON object, not null", field)
		}
		for key, child := range v {
			if err := rejectNull(child, field+"."+key); err != nil {
				return err
			}
		}
	case []interface{}:
		for i, child := range v {
			if err := rejectNull(child, fmt.Sprintf("%s[%d]", field, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func mergeObjects(dst, src map[string]interface{}) {
	for key, value := range src {
		oldObject, oldOK := dst[key].(map[string]interface{})
		newObject, newOK := value.(map[string]interface{})
		if oldOK && newOK {
			mergeObjects(oldObject, newObject)
		} else {
			dst[key] = value
		}
	}
}
