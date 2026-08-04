package main

import (
	"errors"
	"fmt"

	"kairos/internal/core"
	"kairos/internal/llm"
)

// userMessage 把 RunHighlightExtraction()（或启动期检查）返回的 error 映射
// 成用户可读的中文提示，不把底层 ffmpeg/go-openai 报错原文直接展示给用户
// （implementation-plan.md「错误处理策略」）。覆盖 ticket 08 要求的全部错误
// 场景：源文件被删除/移动、磁盘空间不足、CUDA 不可用、LLM 超时、API Key 无效。
func userMessage(err error) string {
	if err == nil {
		return ""
	}

	switch {
	case errors.Is(err, core.ErrSourceFileMissing):
		return "源文件不存在或处理过程中被移动/删除，请重新选择视频文件。"
	case errors.Is(err, core.ErrInsufficientDiskSpace):
		return "磁盘空间不足，请清理磁盘后重试。"
	case errors.Is(err, ErrUnsupportedPlatform):
		return "本地语音识别依赖 Windows 专属组件，当前系统不支持。"
	case errors.Is(err, llm.ErrLlmTimeout):
		return "DeepSeek 请求超时，请检查网络连接后重试。"
	case errors.Is(err, llm.ErrInvalidAPIKey):
		return "DeepSeek API Key 无效，请在设置中重新输入。"
	case errors.Is(err, core.ErrLlmInvalidResponse):
		return "高光判定失败，请稍后重试；如持续失败请检查 API Key 或网络。"
	case errors.Is(err, core.ErrAudioExtractionFailed):
		return "音轨提取失败，请确认视频文件未损坏。"
	case errors.Is(err, core.ErrTranscriptionFailed):
		return "台词识别失败，请检查显卡驱动是否正常或稍后重试。"
	case errors.Is(err, core.ErrVideoInspectionFailed):
		return "读取视频信息失败，请确认视频文件未损坏。"
	case errors.Is(err, core.ErrClipExtractionFailed):
		return "剪辑高光片段失败，请确认磁盘空间充足、视频文件未损坏。"
	default:
		return fmt.Sprintf("处理失败：%v", err)
	}
}
