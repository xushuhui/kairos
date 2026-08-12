//go:build !windows && !darwin

package main

import (
	"kairos/internal/core"
)

// unsupportedTranscriber is the stub Transcriber used on any platform other
// than Windows or macOS. Kairos's actual deployment target is
// Windows-only (map.md「Out of scope」: macOS/Linux 支持已确认不支持) — the
// darwin build in transcriber_supported.go exists purely as a local
// dev/test convenience (user-directed: verify end to end on macOS before
// installing on the real Windows target), not a second shipped platform.
// Linux and everything else still has no real transcriber to construct.
//
// The rest of the GUI (file picking, output selection, history, settings)
// is plain Fyne and builds/runs fine cross-platform, which is exactly what
// lets this app be developed and smoke-tested on unsupported platforms too
// — only the actual transcription call fails, cleanly, through the same
// error path every other RunHighlightExtraction failure uses.
// ErrUnsupportedPlatform itself lives in errors.go (unconstrained) since
// userMessage() needs it on every platform.

type unsupportedTranscriber struct{}

func (unsupportedTranscriber) Transcribe(audioPath string) ([]core.Sentence, error) {
	return nil, ErrUnsupportedPlatform
}

func newProductionTranscriber() core.Transcriber {
	return unsupportedTranscriber{}
}
