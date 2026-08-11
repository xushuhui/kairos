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

	"kairos/internal/apppath"
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

// setupLogging wires slog's default output to two files under
// <exe 所在目录>/logs/, degrading gracefully (Warn + fall back to stderr
// only) when the app directory can't be resolved or a file can't be
// opened — never crashes the app over a logging problem:
//
//   - logs/app.log — full log (every level), implementation-plan.md「日志」
//     一节已确认的主日志：启动/退出、RunHighlightExtraction 各阶段、CUDA
//     检测结果、panic 兜底都记这里.
//   - logs/error.log — Error 级别的独立副本，方便直接肉眼查看，不用在
//     app.log 的大量 Info/Warn 里翻。ASSUMPTION（用户明确要求，已知晓风险）：
//     exe 装在 Program Files 等受保护目录时通常没有写权限，两个文件都会打开
//     失败；按设计静默跳过（打一条 Warn），不影响程序运行。
//
// Returns the files the caller must Close() on exit (0, 1, or 2 of them,
// depending on which destinations actually opened).
func setupLogging() []*os.File {
	dir, dirErr := apppath.Dir()
	if dirErr != nil {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
		slog.Warn("kairos: failed to resolve app directory, logging to stderr only", "error", dirErr)
		return nil
	}
	logDir := filepath.Join(dir, "logs")

	var handlers []slog.Handler
	var files []*os.File

	if f, err := openLogFile(logDir, "app.log"); err != nil {
		slog.Warn("kairos: app.log disabled", "error", err)
	} else {
		handlers = append(handlers, slog.NewTextHandler(io.MultiWriter(f, os.Stderr), nil))
		files = append(files, f)
	}

	if f, err := openLogFile(logDir, "error.log"); err != nil {
		slog.Warn("kairos: error.log disabled", "error", err)
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

// openLogFile opens (creating logDir as needed) logDir/name for append.
func openLogFile(logDir, name string) (*os.File, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(logDir, name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
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
