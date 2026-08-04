package core

import "errors"

// 哨兵错误：RunHighlightExtraction() 按 implementation-plan.md「错误处理策略」
// 一节声明的分类返回，供 GUI 层用 errors.Is() 判定具体错误类型做用户可读的
// 提示，不把底层 ffmpeg/go-openai 报错原文直接展示。
//
// 每一层用 fmt.Errorf("阶段描述: %w", err) 包装错误往上传，%w 保留原始哨兵
// 错误不丢失——对于封装了下层具体实现（video/asr/llm）的阶段，这里额外用
// Go 1.20+ 多重 %w 把这层自己的分类错误和下层原始错误一起链进结果，两者都能
// 被 errors.Is() 判定到（前者做粗粒度的"哪个阶段失败了"分类，后者留给知道
// 具体注入实现的调用方做细粒度诊断）。
var (
	// ErrSourceFileMissing 表示源视频文件不存在或无法访问。
	ErrSourceFileMissing = errors.New("source file missing")
	// ErrInsufficientDiskSpace 表示处理前检测到磁盘可用空间不足。
	ErrInsufficientDiskSpace = errors.New("insufficient disk space")
	// ErrAudioExtractionFailed 表示音轨提取阶段失败。
	ErrAudioExtractionFailed = errors.New("audio extraction failed")
	// ErrTranscriptionFailed 表示 ASR 转写阶段失败。
	ErrTranscriptionFailed = errors.New("transcription failed")
	// ErrLlmTimeout 预留给 LLM 请求超时场景——core 本身不直接判定超时（避免
	// 反向依赖 internal/llm 造成循环 import），但 Judge() 失败的原始错误会
	// 经由 %w 链一路透传，知道具体注入实现的调用方（如未来的 cmd/kairos）
	// 仍可以对结果 errors.Is(err, llm.ErrLlmTimeout) 判定到。
	ErrLlmTimeout = errors.New("llm request timeout")
	// ErrLlmInvalidResponse 表示 HighlightJudge 返回了不可用的判定结果——
	// 包括 Judge() 本身报错，以及返回的 StartID/EndID 越界（查不到对应的
	// ASR 句子）这种 core 自己能判定出的"响应不合法"场景。
	ErrLlmInvalidResponse = errors.New("llm returned invalid response")
	// ErrClipExtractionFailed 表示最终剪辑阶段失败。
	ErrClipExtractionFailed = errors.New("clip extraction failed")
	// ErrVideoInspectionFailed 表示探测源视频元信息（目前是 video.Duration()
	// 探测总时长，用于喂给 ComputeWindow() 的 videoLenMs 参数）失败——
	// implementation-plan.md 原始的 8 个哨兵没有覆盖这个阶段（时长探测是
	// 编排 RunHighlightExtraction 时才发现的必要前置步骤，不在原设计枚举
	// 里），这里补一个保持跟其余阶段一致的 errors.Is() 判定粒度。
	ErrVideoInspectionFailed = errors.New("video inspection failed")
	// ErrGpuUnavailable 表示 GPU 硬件加速不可用——仅作为日志/降级提示的分类
	// 标记，不阻断处理：video 包内部已经透明处理 CUDA 不可用时的 CPU/软编码
	// 降级，RunHighlightExtraction 不会因为这个原因失败。
	ErrGpuUnavailable = errors.New("gpu unavailable")
)
