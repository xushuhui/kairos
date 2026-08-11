// Command kairos 是 Windows 桌面 GUI 入口（Fyne），组装 core/video/asr/llm/history
// 完成"选视频 → 自动出高光片段"的全流程。
package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2/app"

	"kairos/internal/llm"
)

func main() {
	logFiles := setupLogging()
	defer func() {
		for _, f := range logFiles {
			f.Close()
		}
	}()
	slog.Info("kairos: starting")

	fyneApp := app.New()
	a := newApp(fyneApp)
	a.win.SetOnClosed(func() {
		slog.Info("kairos: shutting down")
		a.closeTranscriber()
	})
	a.win.Show()

	if _, err := llm.LoadAPIKey(); err != nil {
		a.promptForAPIKey(nil)
	}

	fyneApp.Run()
}

// setupLogging wires slog's default output to two independent
// destinations, degrading gracefully (Warn + skip, never crash) when either
// is unavailable:
//
//   - %APPDATA%/kairos/app.log — full log (every level), implementation-plan.md
//     「日志」一节已确认的主日志：启动/退出、RunHighlightExtraction 各阶段、CUDA
//     检测结果、panic 兜底都记这里.
//   - <exe 所在目录>/logs/error.log — Error 级别的独立副本，方便在程序安装目录
//     直接肉眼查看，不用去 %APPDATA% 翻。ASSUMPTION（用户明确要求，已知晓风险）：
//     exe 装在 Program Files 等受保护目录时通常没有写权限，这个文件会打开失败；
//     按设计静默跳过（打一条 Warn），不影响 app.log 或程序运行。
//
// Returns the files the caller must Close() on exit (0, 1, or 2 of them,
// depending on which destinations actually opened).
func setupLogging() []*os.File {
	var handlers []slog.Handler
	var files []*os.File

	if f, err := openAppLog(); err != nil {
		slog.Warn("kairos: app.log disabled", "error", err)
	} else {
		handlers = append(handlers, slog.NewTextHandler(io.MultiWriter(f, os.Stderr), nil))
		files = append(files, f)
	}

	if f, err := openErrorLog(); err != nil {
		slog.Warn("kairos: logs/error.log disabled", "error", err)
	} else {
		handlers = append(handlers, slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelError}))
		files = append(files, f)
	}

	switch len(handlers) {
	case 0:
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	case 1:
		slog.SetDefault(slog.New(handlers[0]))
	default:
		slog.SetDefault(slog.New(multiHandler{handlers: handlers}))
	}
	return files
}

// openAppLog opens (creating dirs as needed) %APPDATA%/kairos/app.log.
func openAppLog() (*os.File, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	logDir := filepath.Join(dir, "kairos")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(logDir, "app.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}

// openErrorLog opens (creating dirs as needed) <exe 所在目录>/logs/error.log.
func openErrorLog() (*os.File, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	logDir := filepath.Join(filepath.Dir(exe), "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(logDir, "error.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}

// multiHandler fans a slog.Record out to every wrapped handler that accepts
// its level. log/slog has no built-in multi-destination handler; Handle's
// use of record.Clone() per sub-handler is the pattern the stdlib docs
// recommend for exactly this fan-out case (a shared Record must not be
// mutated by more than one handler).
type multiHandler struct {
	handlers []slog.Handler
}

func (m multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m multiHandler) Handle(ctx context.Context, record slog.Record) error {
	var errs error
	for _, h := range m.handlers {
		if !h.Enabled(ctx, record.Level) {
			continue
		}
		if err := h.Handle(ctx, record.Clone()); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	return errs
}

func (m multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		next[i] = h.WithAttrs(attrs)
	}
	return multiHandler{handlers: next}
}

func (m multiHandler) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		next[i] = h.WithGroup(name)
	}
	return multiHandler{handlers: next}
}
