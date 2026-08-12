//go:build windows || darwin

package main

import (
	"fmt"
	"path/filepath"

	"kairos/internal/apppath"
	"kairos/internal/asr"
	"kairos/internal/core"
)

// modelDirPath resolves where ticket 09's installer places the bundled
// Paraformer-large + Silero VAD + punctuation model files, relative to the
// running executable's own directory (apppath.Dir()) — NOT the process's
// current working directory. A relative "models" constant would break the
// moment kairos.exe is launched with a different cwd (double-click from a
// shortcut with no explicit "start in" dir, `go run`, a console opened
// elsewhere), surfacing only as a confusing mid-run "加载失败" with no path
// hint (found during pre-flight audit before this had ever been exercised
// on real Windows).
func modelDirPath() (string, error) {
	dir, err := apppath.Dir()
	if err != nil {
		return "", fmt.Errorf("定位可执行文件路径失败: %w", err)
	}
	return filepath.Join(dir, "models"), nil
}

// newProductionTranscriber wires the real Paraformer transcriber, CPU
// provider only (user-directed decision: CUDA execution provider caused
// too many real-machine failure modes — see internal/asr/paraformer_windows.go's
// NewParaformerTranscriber doc comment for the specific crash that tipped
// this decision). Builds on both windows and darwin — internal/asr ships a
// mirrored implementation per platform (paraformer_windows.go /
// paraformer_darwin.go) behind the identical asr.NewParaformerTranscriber
// signature, so this call site needs no platform branching of its own.
// darwin support exists purely as a local dev/test convenience (user-
// directed: verify end to end on macOS before installing on the actual
// Windows target) — map.md's "Windows-only, macOS/Linux out of scope"
// deployment decision hasn't changed.
func newProductionTranscriber() core.Transcriber {
	dir, err := modelDirPath()
	if err != nil {
		return failingTranscriber{err: fmt.Errorf("asr: %w", err)}
	}
	t, err := asr.NewParaformerTranscriber(dir)
	if err != nil {
		// Model files missing/corrupt at startup — surface it the same way
		// as the unsupported-platform stub does: defer the error to the
		// first Transcribe() call rather than crashing app startup, so the
		// rest of the GUI (file picking, history, settings) stays usable
		// and the error shows up through the same RunHighlightExtraction
		// -> userMessage() path as every other failure mode.
		return failingTranscriber{err: err}
	}
	return t
}

type failingTranscriber struct{ err error }

func (f failingTranscriber) Transcribe(audioPath string) ([]core.Sentence, error) {
	return nil, f.err
}
