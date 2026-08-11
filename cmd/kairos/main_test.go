package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// multiHandler 是 setupLogging() 引入的核心新逻辑——验证它按每个子 handler
// 自己的 level 过滤分流，而不是无差别广播（error.log 只应收到 Error 级别，
// app.log 风格的 handler 应收到全部级别）。
func TestMultiHandler_RoutesByLevel(t *testing.T) {
	var everything, errorsOnly bytes.Buffer
	h := multiHandler{handlers: []slog.Handler{
		slog.NewTextHandler(&everything, nil),
		slog.NewTextHandler(&errorsOnly, &slog.HandlerOptions{Level: slog.LevelError}),
	}}
	logger := slog.New(h)

	logger.Info("info message")
	logger.Error("error message")

	if !strings.Contains(everything.String(), "info message") {
		t.Errorf("all-levels handler missing info message, got: %s", everything.String())
	}
	if !strings.Contains(everything.String(), "error message") {
		t.Errorf("all-levels handler missing error message, got: %s", everything.String())
	}
	if strings.Contains(errorsOnly.String(), "info message") {
		t.Errorf("error-only handler should not receive info message, got: %s", errorsOnly.String())
	}
	if !strings.Contains(errorsOnly.String(), "error message") {
		t.Errorf("error-only handler missing error message, got: %s", errorsOnly.String())
	}
}

// WithAttrs/WithGroup must propagate to every wrapped handler — a logger
// built with slog.New(h).With("k", "v") needs both destinations to carry
// the attribute, not just the first one.
func TestMultiHandler_WithAttrsPropagatesToAll(t *testing.T) {
	var a, b bytes.Buffer
	h := multiHandler{handlers: []slog.Handler{
		slog.NewTextHandler(&a, nil),
		slog.NewTextHandler(&b, nil),
	}}
	logger := slog.New(h).With("request_id", "abc123")
	logger.Info("hello")

	if !strings.Contains(a.String(), "request_id=abc123") {
		t.Errorf("handler a missing propagated attr, got: %s", a.String())
	}
	if !strings.Contains(b.String(), "request_id=abc123") {
		t.Errorf("handler b missing propagated attr, got: %s", b.String())
	}
}
