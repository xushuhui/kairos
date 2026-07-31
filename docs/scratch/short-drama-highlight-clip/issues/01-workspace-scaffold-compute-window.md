# 01 — Go module 骨架 + ComputeWindow()

**What to build:** Go module 骨架，按 implementation-plan.md 的 `cmd/` + `internal/` 目录结构搭好（`internal/core` / `internal/video` / `internal/asr` / `internal/llm` / `internal/history`，`cmd/kairos`）；在 `internal/core` 里实现 `ComputeWindow()` 纯函数（锚定高潮在结尾 + 边界 clamp 算法，见 06-highlight-window-algorithm.md）。

**Blocked by:** None — can start immediately.

**Status:** ready-for-agent

- [ ] `go build ./...` 能编译整个 module（其余包允许是空壳，只有 `internal/core` 需要实质内容）
- [ ] `func ComputeWindow(peakEndMs, targetLenMs, videoLenMs uint64) (startMs, endMs uint64)` 实现完成
- [ ] spec.md Testing Decisions 表里的 6 条边界用例（正常、高潮贴开头、视频短于目标时长等）全部通过 `go test`（表驱动测试写法）
