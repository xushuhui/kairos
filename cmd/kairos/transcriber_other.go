//go:build !windows

package main

import (
	"kairos/internal/core"
)

// unsupportedTranscriber is the stub Transcriber used on any non-Windows
// build. Kairos is a Windows-only product (map.md「Out of scope」:
// macOS/Linux 支持已确认不支持) — its ASR backend
// (github.com/k2-fsa/sherpa-onnx-go-windows) only compiles on
// windows/amd64|386, so there is no real transcriber to construct here.
//
// The rest of the GUI (file picking, output selection, history, settings)
// is plain Fyne and builds/runs fine cross-platform, which is exactly what
// lets this app be developed and smoke-tested on this machine — only the
// actual transcription call fails, cleanly, through the same error path
// every other RunHighlightExtraction failure uses. ErrUnsupportedPlatform
// itself lives in errors.go (unconstrained) since userMessage() needs it
// on every platform.

type unsupportedTranscriber struct{}

func (unsupportedTranscriber) Transcribe(audioPath string) ([]core.Sentence, error) {
	return nil, ErrUnsupportedPlatform
}

func newProductionTranscriber() core.Transcriber {
	return unsupportedTranscriber{}
}
