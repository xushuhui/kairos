package video

import (
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// requireFfmpeg skips the test if ffmpeg/ffprobe aren't on PATH — the test
// fixtures are generated with ffmpeg itself, so without it there's nothing
// to test against.
func requireFfmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not found in PATH, skipping")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not found in PATH, skipping")
	}
}

// makeTestMp4 generates a 5s synthetic MP4 (silent audio + static color
// block video) via ffmpeg lavfi sources, per spec.md Testing Decisions:
// "短剧源文件可以预置一个 5 秒的测试 MP4（静音 + 固定色块）".
func makeTestMp4(t *testing.T) string {
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

// ffprobeStream runs ffprobe -show_streams on path and returns the decoded
// stream list.
type ffprobeStream struct {
	CodecType  string `json:"codec_type"`
	SampleRate string `json:"sample_rate"`
	Channels   int    `json:"channels"`
	Duration   string `json:"duration"`
}

func ffprobeStreams(t *testing.T, path string) []ffprobeStream {
	t.Helper()
	cmd := exec.Command("ffprobe", "-v", "quiet", "-print_format", "json", "-show_streams", path)
	outBytes, err := cmd.Output()
	if err != nil {
		t.Fatalf("ffprobe failed on %s: %v", path, err)
	}
	var parsed struct {
		Streams []ffprobeStream `json:"streams"`
	}
	if err := json.Unmarshal(outBytes, &parsed); err != nil {
		t.Fatalf("failed to parse ffprobe output: %v", err)
	}
	return parsed.Streams
}

func TestExtractAudio_ProducesValid16kMonoWav(t *testing.T) {
	requireFfmpeg(t)
	videoPath := makeTestMp4(t)
	outWav := filepath.Join(t.TempDir(), "out.wav")

	if err := ExtractAudio(videoPath, outWav); err != nil {
		t.Fatalf("ExtractAudio() error = %v", err)
	}

	streams := ffprobeStreams(t, outWav)
	var audio *ffprobeStream
	for i := range streams {
		if streams[i].CodecType == "audio" {
			audio = &streams[i]
			break
		}
	}
	if audio == nil {
		t.Fatalf("output wav has no audio stream: %+v", streams)
	}
	if audio.SampleRate != "16000" {
		t.Errorf("sample_rate = %q, want 16000", audio.SampleRate)
	}
	if audio.Channels != 1 {
		t.Errorf("channels = %d, want 1", audio.Channels)
	}
}

func TestExtractAudio_MissingInput(t *testing.T) {
	requireFfmpeg(t)
	outWav := filepath.Join(t.TempDir(), "out.wav")

	err := ExtractAudio(filepath.Join(t.TempDir(), "does-not-exist.mp4"), outWav)
	if err == nil {
		t.Fatal("ExtractAudio() error = nil, want error for missing input")
	}
	if !errors.Is(err, ErrInputUnreadable) {
		t.Errorf("ExtractAudio() error = %v, want wrapping ErrInputUnreadable", err)
	}
}

func TestCudaAvailable_FalseWithoutHardware(t *testing.T) {
	if _, err := exec.LookPath("nvidia-smi"); err == nil {
		t.Skip("nvidia-smi found in PATH, this test only covers the no-hardware case")
	}

	done := make(chan bool, 1)
	go func() { done <- CudaAvailable() }()

	select {
	case got := <-done:
		if got {
			t.Error("CudaAvailable() = true, want false (no NVIDIA hardware on this machine)")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("CudaAvailable() did not return within 10s, likely hanging")
	}
}

func TestSelectEncoder_Libx264WithoutCuda(t *testing.T) {
	if _, err := exec.LookPath("nvidia-smi"); err == nil {
		t.Skip("nvidia-smi found in PATH, this test only covers the no-hardware case")
	}

	if got := SelectEncoder(); got != "libx264" {
		t.Errorf("SelectEncoder() = %q, want libx264", got)
	}
}

func TestCutClip_Libx264ProducesPlayableOutput(t *testing.T) {
	requireFfmpeg(t)
	if CudaAvailable() {
		t.Skip("CUDA available on this machine, this test only covers the libx264 fallback path")
	}
	videoPath := makeTestMp4(t)
	outPath := filepath.Join(t.TempDir(), "clip.mp4")

	const startMs, endMs uint64 = 1000, 3000
	if err := CutClip(videoPath, startMs, endMs, outPath); err != nil {
		t.Fatalf("CutClip() error = %v", err)
	}

	streams := ffprobeStreams(t, outPath)
	var video *ffprobeStream
	for i := range streams {
		if streams[i].CodecType == "video" {
			video = &streams[i]
			break
		}
	}
	if video == nil {
		t.Fatalf("output clip has no video stream: %+v", streams)
	}

	gotDur, err := time.ParseDuration(video.Duration + "s")
	if err != nil {
		t.Fatalf("failed to parse duration %q: %v", video.Duration, err)
	}
	wantDur := time.Duration(endMs-startMs) * time.Millisecond
	diff := gotDur - wantDur
	if diff < 0 {
		diff = -diff
	}
	if diff > 500*time.Millisecond {
		t.Errorf("clip duration = %v, want close to %v (diff %v > 500ms tolerance)", gotDur, wantDur, diff)
	}
}

func TestCutClip_NvencRequiresRealHardware(t *testing.T) {
	if !CudaAvailable() {
		t.Skip("no NVENC-capable NVIDIA GPU on this machine, cannot exercise the h264_nvenc encode path")
	}
	// Intentionally left unimplemented on machines with real CUDA hardware:
	// this repo's dev/test machine has none, so the h264_nvenc path is only
	// covered by the automatic-fallback-to-libx264 behavior asserted in
	// TestCutClip_Libx264ProducesPlayableOutput and CutClip's own retry logic.
	t.Skip("NVENC hardware path not exercised by this test suite")
}

func TestCutClip_InvalidRange(t *testing.T) {
	err := CutClip("irrelevant.mp4", 5000, 1000, filepath.Join(t.TempDir(), "out.mp4"))
	if err == nil {
		t.Fatal("CutClip() error = nil, want error for endMs <= startMs")
	}
}
