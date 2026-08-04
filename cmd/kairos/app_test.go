package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"kairos/internal/core"
	"kairos/internal/history"
	"kairos/internal/llm"
)

// withTempHome redirects os.UserConfigDir() (used by internal/history and
// internal/llm) to a throwaway temp dir, mirroring internal/core's own test
// helper — headless GUI tests must not touch this machine's real
// %APPDATA%/kairos/.
func withTempHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("AppData", dir)
}

func TestIsVideoFile(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"ep01.mp4", true},
		{"EP01.MP4", true}, // case-insensitive
		{"clip.mov", true},
		{"clip.avi", true},
		{"clip.webm", true},
		{"notes.txt", false},
		{"video.mkv", false}, // not in spec's supported list
		{"noext", false},
	}
	for _, c := range cases {
		if got := isVideoFile(c.path); got != c.want {
			t.Errorf("isVideoFile(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestApp_SelectVideo_UpdatesStateAndDefaultsOutputDir(t *testing.T) {
	withTempHome(t)
	a := newApp(test.NewApp())
	defer a.win.Close()

	dir := t.TempDir()
	videoPath := filepath.Join(dir, "ep01.mp4")
	if err := os.WriteFile(videoPath, []byte("fake"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	a.selectVideo(videoPath)

	if a.videoPath != videoPath {
		t.Errorf("videoPath = %q, want %q", a.videoPath, videoPath)
	}
	if got := a.outputEntry.Text; got != dir {
		t.Errorf("outputEntry.Text = %q, want %q (default = source dir)", got, dir)
	}
}

func TestApp_OutputSection_HiddenUntilVideoSelected(t *testing.T) {
	withTempHome(t)
	a := newApp(test.NewApp())
	defer a.win.Close()

	if a.outputSection.Visible() {
		t.Error("outputSection visible before any video is selected, want hidden (spec.md 用户故事 5)")
	}

	dir := t.TempDir()
	videoPath := filepath.Join(dir, "ep01.mp4")
	os.WriteFile(videoPath, []byte("fake"), 0o644)
	a.selectVideo(videoPath)

	if !a.outputSection.Visible() {
		t.Error("outputSection still hidden after selecting a video, want visible")
	}
}

func TestApp_SelectVideo_RejectsUnsupportedExtension(t *testing.T) {
	withTempHome(t)
	a := newApp(test.NewApp())
	defer a.win.Close()

	a.selectVideo("/tmp/not-a-video.txt")

	if a.videoPath != "" {
		t.Errorf("videoPath = %q after rejecting unsupported file, want empty", a.videoPath)
	}
}

func TestApp_StartProcessing_NoVideoSelected_DoesNotPanic(t *testing.T) {
	withTempHome(t)
	a := newApp(test.NewApp())
	defer a.win.Close()

	// No video selected yet — must show a hint and return, not crash.
	a.startProcessing()
	if a.processing {
		t.Error("processing = true with no video selected, want false")
	}
}

func TestApp_StartProcessing_NoAPIKey_PromptsInsteadOfProcessing(t *testing.T) {
	withTempHome(t) // fresh temp config dir => LoadAPIKey() fails => no key configured
	a := newApp(test.NewApp())
	defer a.win.Close()

	dir := t.TempDir()
	videoPath := filepath.Join(dir, "ep01.mp4")
	os.WriteFile(videoPath, []byte("fake"), 0o644)
	a.selectVideo(videoPath)

	a.startProcessing()

	if a.processing {
		t.Error("processing = true without a configured API key, want false (should prompt instead)")
	}
}

func TestHistoryRowText(t *testing.T) {
	rec := history.Record{
		SourceName: "ep01.mp4",
		Status:     "success",
		CreatedAt:  time.Date(2026, 7, 30, 15, 30, 0, 0, time.UTC),
	}
	got := historyRowText(rec)
	for _, want := range []string{"2026-07-30 15:30", "ep01.mp4", "成功"} {
		if !strings.Contains(got, want) {
			t.Errorf("historyRowText() = %q, want it to contain %q", got, want)
		}
	}

	rec.Status = "failed"
	got = historyRowText(rec)
	if !strings.Contains(got, "失败") {
		t.Errorf("historyRowText() for failed record = %q, want it to contain 失败", got)
	}
}

func TestFormatTranscript(t *testing.T) {
	t.Run("空记录", func(t *testing.T) {
		if got := formatTranscript(nil); got != "（无转写记录）" {
			t.Errorf("formatTranscript(nil) = %q", got)
		}
	})

	t.Run("正常解析", func(t *testing.T) {
		sentences := []core.Sentence{
			{ID: 0, StartMs: 0, EndMs: 1000, Text: "第一句"},
			{ID: 1, StartMs: 1000, EndMs: 2000, Text: "第二句"},
		}
		raw, err := json.Marshal(sentences)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		got := formatTranscript(raw)
		if !strings.Contains(got, "第一句") || !strings.Contains(got, "第二句") {
			t.Errorf("formatTranscript() = %q, want both sentences present", got)
		}
	})

	t.Run("解析失败时给出可读提示而不是报错", func(t *testing.T) {
		got := formatTranscript(json.RawMessage(`not json`))
		if got != "（转写记录解析失败）" {
			t.Errorf("formatTranscript(invalid) = %q", got)
		}
	})
}

func TestFormatJudgeReason(t *testing.T) {
	t.Run("空记录", func(t *testing.T) {
		if got := formatJudgeReason(nil); got != "（无判定记录）" {
			t.Errorf("formatJudgeReason(nil) = %q", got)
		}
	})

	t.Run("正常解析", func(t *testing.T) {
		window := core.HighlightWindow{Reason: "冲突集中，适合做广告钩子"}
		raw, err := json.Marshal(window)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if got := formatJudgeReason(raw); got != "冲突集中，适合做广告钩子" {
			t.Errorf("formatJudgeReason() = %q", got)
		}
	})
}

func TestUserMessage(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"源文件缺失", core.ErrSourceFileMissing, "源文件不存在或处理过程中被移动/删除，请重新选择视频文件。"},
		{"磁盘空间不足", core.ErrInsufficientDiskSpace, "磁盘空间不足，请清理磁盘后重试。"},
		{"不支持的平台", ErrUnsupportedPlatform, "本地语音识别依赖 Windows 专属组件，当前系统不支持。"},
		{"API_Key_无效", llm.ErrInvalidAPIKey, "DeepSeek API Key 无效，请在设置中重新输入。"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := userMessage(c.err); got != c.want {
				t.Errorf("userMessage(%v) = %q, want %q", c.err, got, c.want)
			}
		})
	}

	t.Run("多层 %w 包裹后仍能识别出具体哨兵错误", func(t *testing.T) {
		wrapped := fmt.Errorf("转写台词失败: %w: %w", core.ErrTranscriptionFailed, errors.New("底层 asr 报错"))
		if got := userMessage(wrapped); got != "台词识别失败，请检查显卡驱动是否正常或稍后重试。" {
			t.Errorf("userMessage(wrapped ErrTranscriptionFailed) = %q", got)
		}
	})

	t.Run("未知错误兜底展示原文", func(t *testing.T) {
		custom := errors.New("boom")
		if got := userMessage(custom); got != "处理失败：boom" {
			t.Errorf("userMessage(custom) = %q, want fallback with original text", got)
		}
	})
}
