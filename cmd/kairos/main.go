// Command kairos 是 Windows 桌面 GUI 入口（Fyne），组装 core/video/asr/llm/history
// 完成"选视频 → 自动出高光片段"的全流程。
package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2/app"

	"kairos/internal/llm"
)

func main() {
	logFile := setupLogging()
	if logFile != nil {
		defer logFile.Close()
	}
	slog.Info("kairos: starting")

	fyneApp := app.New()
	a := newApp(fyneApp)
	a.win.SetOnClosed(func() { slog.Info("kairos: shutting down") })
	a.win.Show()

	if _, err := llm.LoadAPIKey(); err != nil {
		a.promptForAPIKey(nil)
	}

	fyneApp.Run()
}

// setupLogging opens (creating if needed) %APPDATA%/kairos/app.log and makes
// it the default slog output (implementation-plan.md「日志」：软件启动/退出、
// RunHighlightExtraction 各阶段、CUDA 检测结果、panic 兜底都要记进这个文件，
// 不做日志轮转——内部工具日志量级小，暂不需要）。On failure it falls back to
// stderr rather than crashing the app over a logging problem.
func setupLogging() *os.File {
	dir, err := os.UserConfigDir()
	if err != nil {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
		slog.Warn("kairos: failed to resolve user config dir, logging to stderr", "error", err)
		return nil
	}
	logDir := filepath.Join(dir, "kairos")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
		slog.Warn("kairos: failed to create log dir, logging to stderr", "error", err)
		return nil
	}
	f, err := os.OpenFile(filepath.Join(logDir, "app.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
		slog.Warn("kairos: failed to open log file, logging to stderr", "error", err)
		return nil
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(f, os.Stderr), nil)))
	return f
}
