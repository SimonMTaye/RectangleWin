# Configuration

Pure Go configuration package with no third-party dependencies. All settings live
under the public `Config` root, currently containing `Keyboard`. Canonical defaults
are checked in as `defaults.json` and embedded in the binary.

## API

- `Load() (Config, string, error)` resolves
  `os.UserConfigDir()/RectangleWin/config.json` and loads it. The resolved path is
  returned on failure too, unless resolving the user configuration directory fails.
- `LoadFile(path string) (Config, error)` loads an explicit path. Both loaders create
  missing parents and initialize missing files with the exact embedded defaults.
  Existing files, including invalid ones, are never rewritten.
- `Defaults() Config` returns an independent, mutable copy of the defaults.
- `(Config).Validate() error` validates a complete configuration, including all eleven
  supported actions. Loaders call this automatically and include the file path in
  errors.

The public data types are `Config`, `Keyboard`, and `Binding`; their JSON fields are
`keyboard`, `super`, `bindings`, `key`, and `modifiers`.

## Overrides

Only specified fields change. Objects merge recursively, including each binding;
arrays replace their defaults. `{}` therefore loads the full default configuration.
An empty modifiers array removes a binding's extra modifiers. An empty super array
is invalid. Explicit `null`, unknown fields/actions, incorrect types, malformed JSON,
and trailing JSON documents are rejected. Use the canonical spelling and case for
key and modifier names.

For example, this changes only the center key, preserving all other defaults:

```json
{"keyboard":{"bindings":{"center":{"key":"Z"}}}}
```

Keys: `A`–`Z`, `0`–`9`, `F1`–`F24`, `Left`, `Right`, `Up`, `Down`, `Space`, `Enter`,
`Tab`, `Escape`, `Home`, `End`, `PageUp`, `PageDown`, `Insert`, `Delete`, `Backspace`.
Modifiers: `ctrl`, `alt`, `shift`, `win`, and their `left_`/`right_` variants.

A shortcut combines super with its binding's extra modifiers. Repeated modifiers
and generic-plus-specific modifiers within one family are redundant and rejected.
Extra modifiers must not overlap super; adding the opposite side is allowed.

Conflict detection uses exact modifier chords: extra modifier families distinguish
shortcuts (as with default edges versus corners). Left-only and right-only chords
are distinct, and requiring both sides is supported. A generic modifier accepts
left, right, or both sides, so it conflicts with any of these on the same key when
the other modifier families also overlap. Consumers should preserve these matching
semantics rather than collapsing side-specific modifiers.

## Tests

Run `go test ./config` and `go vet ./config` from the repository root. Tests cover
canonical defaults and copy isolation, file creation/preservation, user-path
resolution, nested inheritance, strict decoding, I/O errors, all supported keys,
modifier validation, super overlap, and effective shortcut conflicts. The package
can be tested on Linux independently of the Windows application.
