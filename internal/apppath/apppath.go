// Package apppath resolves the one directory kairos treats as its own —
// config.json, history/, logs/, and models/ (ticket 09's installer layout)
// all live next to the running executable, not in a per-OS user profile
// directory (%APPDATA% on Windows). The app is meant to be fully
// self-contained/portable in one folder: copy the folder, the whole app
// (config, history, logs) moves with it — no leftover state in the user's
// profile after an uninstall, and no surprise about which %APPDATA%\kairos\
// an install is actually reading from when multiple copies of the exe exist
// on the same machine (real friction hit during Windows bring-up).
package apppath

import (
	"os"
	"path/filepath"
)

// Dir returns the directory containing the running executable — NOT the
// process's current working directory, which can be anything (double-click
// shortcut with no explicit "start in" dir, `go run`, a console opened
// elsewhere). Replaceable so tests can point it at a throwaway temp
// directory instead of wherever `go test`'s compiled test binary happens to
// live.
var Dir = func() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}
