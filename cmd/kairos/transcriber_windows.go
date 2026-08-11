//go:build windows

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

// newProductionTranscriber wires the real Paraformer transcriber. CUDA is
// requested by default — asr.NewParaformerTranscriber falls back to CPU
// internally on CUDA init failure (ticket 03), so this doesn't need its own
// fallback branch.
func newProductionTranscriber() core.Transcriber {
	dir, err := modelDirPath()
	if err != nil {
		return failingTranscriber{err: fmt.Errorf("asr: %w", err)}
	}
	t, err := asr.NewParaformerTranscriber(dir, true)
	if err != nil {
		// Model files missing/corrupt at startup — surface it the same way
		// as the non-Windows "unsupported platform" stub does: defer the
		// error to the first Transcribe() call rather than crashing app
		// startup, so the rest of the GUI (file picking, history, settings)
		// stays usable and the error shows up through the same
		// RunHighlightExtraction -> userMessage() path as every other
		// failure mode.
		return failingTranscriber{err: err}
	}
	return t
}

type failingTranscriber struct{ err error }

func (f failingTranscriber) Transcribe(audioPath string) ([]core.Sentence, error) {
	return nil, f.err
}
