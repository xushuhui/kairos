// Package asr 封装本地 FunASR Paraformer-large ONNX 推理（sherpa-onnx-go-windows，
// CPU execution provider），实现 core.Transcriber。
//
// 本包按平台拆成两部分：
//   - merge.go：字级/词级 token + 标点恢复句末边界 → 句级 core.Sentence 的纯合并
//     逻辑（mergeWordsToSentences），不依赖任何 OS/硬件/模型文件，跨平台可编译、
//     有单元测试（merge_test.go）。
//   - paraformer_windows.go：真正加载 Paraformer-large + Silero-VAD + 标点恢复
//     模型、跑 sherpa-onnx 推理的 ParaformerTranscriber，`//go:build windows`
//     门控——sherpa-onnx-go-windows 本身的 Go 源码只在 windows/(amd64|386) 下
//     可编译，本文件在其它平台上被排除在构建之外，是预期行为，不是遗漏。
package asr
