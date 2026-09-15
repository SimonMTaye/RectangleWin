package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSaveFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	c := Defaults()
	c.Keyboard.Super = []string{"alt", "ctrl"}
	c.Keyboard.Bindings["center"] = Binding{Key: "Z", Modifiers: []string{}}
	if err := SaveFile(path, c); err != nil {
		t.Fatal(err)
	}
	got, err := LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, c) {
		t.Fatalf("round trip: got %#v, want %#v", got, c)
	}
	c.Keyboard.Bindings["center"] = Binding{Key: "X", Modifiers: []string{}}
	if err := SaveFile(path, c); err != nil {
		t.Fatal(err)
	}
	got, err = LoadFile(path)
	if err != nil || !reflect.DeepEqual(got, c) {
		t.Fatalf("replace: got %#v, error %v", got, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v, %v", entries, err)
	}
}

func TestSaveFileRejectsInvalidWithoutChangingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	original := []byte(`{"keyboard":{"bindings":{"center":{"key":"Z"}}}}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	c := Defaults()
	c.Keyboard.Super = nil
	if err := SaveFile(path, c); err == nil {
		t.Fatal("expected validation error")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(original) {
		t.Fatalf("existing file changed: %s, %v", got, err)
	}
}

func TestSaveFileWriteFailure(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := SaveFile(target, Defaults()); err == nil {
		t.Fatal("expected replacement error")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v, %v", entries, err)
	}
}

func TestSaveFileNormalizesNilModifiers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	c := Defaults()
	c.Keyboard.Bindings["center"] = Binding{Key: "Z"}
	if err := SaveFile(path, c); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(path); err != nil {
		t.Fatalf("saved config cannot be loaded: %v", err)
	}
}
