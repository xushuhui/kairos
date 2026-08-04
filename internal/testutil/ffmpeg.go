// Package testutil holds tiny FFmpeg test-fixture helpers shared by
// internal/video and internal/core's tests — both packages exercise real
// FFmpeg subprocesses in their tests (spec.md Testing Decisions: FFmpeg
// I/O is deliberately not mocked), so both need the same throwaway test
// clip. This package only exists to avoid maintaining two copies of the
// same lavfi recipe; it has no production callers.
package testutil

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// RequireFfmpeg skips the test if ffmpeg/ffprobe aren't on PATH — the test
// fixtures are generated with ffmpeg itself, so without it there's nothing
// to test against.
func RequireFfmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not found in PATH, skipping")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not found in PATH, skipping")
	}
}

// MakeTestMp4 generates a 5s synthetic MP4 (silent audio + static color
// block video) via ffmpeg lavfi sources, per spec.md Testing Decisions:
// "短剧源文件可以预置一个 5 秒的测试 MP4（静音 + 固定色块）".
func MakeTestMp4(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "fixture.mp4")
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi", "-i", "color=c=black:size=64x64:rate=10:duration=5",
		"-f", "lavfi", "-i", "anullsrc=r=48000:cl=mono:duration=5",
		"-c:v", "libx264", "-c:a", "aac",
		"-y", out,
	)
	if outBytes, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to generate test fixture mp4: %v\n%s", err, outBytes)
	}
	return out
}
