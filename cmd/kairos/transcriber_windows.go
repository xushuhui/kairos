//go:build windows

package main

import (
	"kairos/internal/asr"
	"kairos/internal/core"
)

// modelDir is where ticket 09's installer places the bundled
// Paraformer-large + Silero VAD + punctuation model files, alongside the
// executable (spec.md「本地 ASR 实现」：模型文件打包在安装包内).
const modelDir = "models"

// newProductionTranscriber wires the real Paraformer transcriber. CUDA is
// requested by default — asr.NewParaformerTranscriber falls back to CPU
// internally on CUDA init failure (ticket 03), so this doesn't need its own
// fallback branch.
func newProductionTranscriber() core.Transcriber {
	t, err := asr.NewParaformerTranscriber(modelDir, true)
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
