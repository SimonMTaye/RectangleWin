package main

import "testing"

func TestAutoRunCommand(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"plain path", `C:\RectangleWin\RectangleWin.exe`, `"C:\RectangleWin\RectangleWin.exe"`},
		{"spaces", `C:\Program Files\RectangleWin\RectangleWin.exe`, `"C:\Program Files\RectangleWin\RectangleWin.exe"`},
		{"unicode", `C:\Users\Renée\RectangleWin.exe`, `"C:\Users\Renée\RectangleWin.exe"`},
		{"UNC path", `\\server\share\RectangleWin.exe`, `"\\server\share\RectangleWin.exe"`},
		{"UNC path with spaces", `\\server\shared apps\RectangleWin.exe`, `"\\server\shared apps\RectangleWin.exe"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := autoRunCommand(tt.path); got != tt.want {
				t.Errorf("autoRunCommand(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestAutoRunCommandMatches(t *testing.T) {
	for _, path := range []string{
		`C:\RectangleWin\RectangleWin.exe`,
		`C:\Program Files\RectangleWin\RectangleWin.exe`,
		`\\server\shared apps\RectangleWin.exe`,
	} {
		t.Run(path, func(t *testing.T) {
			tests := []struct {
				name    string
				command string
				want    bool
			}{
				{"quoted", `"` + path + `"`, true},
				{"legacy unquoted", path, true},
				{"empty", "", false},
				{"different executable", `"C:\Other\RectangleWin.exe"`, false},
				{"relative executable", `RectangleWin.exe`, false},
				{"quoted with arguments", `"` + path + `" --extra`, false},
				{"unquoted with arguments", path + ` --extra`, false},
				{"prefix only", path + `.other`, false},
				{"missing closing quote", `"` + path, false},
				{"missing opening quote", path + `"`, false},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					if got := autoRunCommandMatches(tt.command, path); got != tt.want {
						t.Errorf("autoRunCommandMatches(%q, %q) = %v, want %v", tt.command, path, got, tt.want)
					}
				})
			}
		})
	}
}
