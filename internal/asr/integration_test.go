//go:build windows || darwin

package asr

import (
	"os"
	"testing"
)

// TestParaformerTranscriber_RealModels_ManualRun is the real end-to-end
// smoke test for the actual ASR pipeline (VAD + Paraformer + punctuation)
// against real model files and real audio — gated behind two env vars so
// it never runs in the normal `go test ./...` suite (needs ~500MB of model
// files fetched via scripts/download-models.sh/.ps1, not vendored in this
// repo, see packaging/README.md), matching internal/llm/integration_test.go's
// existing pattern for real-dependency manual-run tests.
func TestParaformerTranscriber_RealModels_ManualRun(t *testing.T) {
	modelDir := os.Getenv("KAIROS_TEST_MODEL_DIR")
	audioPath := os.Getenv("KAIROS_TEST_AUDIO_PATH")
	if modelDir == "" || audioPath == "" {
		t.Skip("KAIROS_TEST_MODEL_DIR/KAIROS_TEST_AUDIO_PATH not set, skipping manual-run real ASR pipeline test")
	}

	transcriber, err := NewParaformerTranscriber(modelDir)
	if err != nil {
		t.Fatalf("NewParaformerTranscriber() error = %v", err)
	}
	defer transcriber.Close()

	sentences, err := transcriber.Transcribe(audioPath)
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if len(sentences) == 0 {
		t.Fatal("Transcribe() returned 0 sentences, want at least 1 for real speech audio")
	}

	for _, s := range sentences {
		t.Logf("sentence[%d] %dms-%dms: %q", s.ID, s.StartMs, s.EndMs, s.Text)
	}
}
