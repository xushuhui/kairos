package core

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kairos/internal/history"
	"kairos/internal/video"
)

// defaultTargetLenMs 是高光片段的默认目标时长（约 60 秒），
// 见 spec.md「窗口算法」。
const defaultTargetLenMs uint64 = 60_000

// diskSpaceSafetyFactor 把源文件大小放大这么多倍作为磁盘空间检查的估算
// 需求量——覆盖临时 WAV（通常远小于源文件）+ 输出高光片段（编码参数相近时
// 体积量级跟源文件相当）+ 一点余量，不追求精确，只求"处理到一半才发现
// 磁盘满了"这种情况不会发生。
const diskSpaceSafetyFactor = 2

// RunHighlightExtraction 是整个项目唯一的编排入口：校验源文件、检查磁盘
// 空间、提取音轨、转写台词、判定高光窗口、查表映射时间戳、剪辑输出、写
// 历史记录。transcriber/judge 是两个测试缝合点（spec.md Testing Decisions），
// 生产环境注入 internal/asr.ParaformerTranscriber 和 internal/llm.DeepSeekJudge，
// 测试环境注入手写 fake struct——FFmpeg 子进程（internal/video）和历史记录
// （internal/history）不走 mock，是编排层唯一的真实 I/O 边界。
func RunHighlightExtraction(videoPath string, config Config, transcriber Transcriber, judge HighlightJudge) (output HighlightOutput, err error) {
	info, statErr := os.Stat(videoPath)
	if statErr != nil {
		return HighlightOutput{}, fmt.Errorf("源文件不存在: %w", ErrSourceFileMissing)
	}

	outputPath := config.OutputPath
	if outputPath == "" {
		outputPath = defaultOutputPath(videoPath)
	}
	targetLenMs := config.TargetLenMs
	if targetLenMs == 0 {
		targetLenMs = defaultTargetLenMs
	}

	requiredBytes := uint64(info.Size()) * diskSpaceSafetyFactor
	if spaceErr := checkDiskSpace(outputPath, requiredBytes); spaceErr != nil {
		return HighlightOutput{}, spaceErr
	}

	tmpDir, tmpErr := os.MkdirTemp("", "kairos-*")
	if tmpErr != nil {
		return HighlightOutput{}, fmt.Errorf("创建临时目录失败: %w", tmpErr)
	}
	defer os.RemoveAll(tmpDir)

	var (
		sentences []Sentence
		window    HighlightWindow
		judged    bool
	)
	defer func() {
		writeHistoryRecord(videoPath, output, sentences, window, judged, err)
	}()

	audioPath := filepath.Join(tmpDir, "audio.wav")
	config.reportProgress(StageExtractingAudio)
	if extractErr := video.ExtractAudio(videoPath, audioPath); extractErr != nil {
		return HighlightOutput{}, fmt.Errorf("提取音轨失败: %w: %w", ErrAudioExtractionFailed, extractErr)
	}

	var transcribeErr error
	config.reportProgress(StageTranscribing)
	sentences, transcribeErr = transcriber.Transcribe(audioPath)
	if transcribeErr != nil {
		return HighlightOutput{}, fmt.Errorf("转写台词失败: %w: %w", ErrTranscriptionFailed, transcribeErr)
	}

	var judgeErr error
	config.reportProgress(StageJudging)
	window, judgeErr = judge.Judge(sentences)
	if judgeErr != nil {
		return HighlightOutput{}, fmt.Errorf("判定高光失败: %w: %w", ErrLlmInvalidResponse, judgeErr)
	}
	judged = true

	if window.EndID < 0 || window.EndID >= len(sentences) {
		// judged 在这里已经是 true——即使窗口越界，也让 writeHistoryRecord
		// 把这个（不合法的）LLM 原始响应写进历史记录，方便事后排查"LLM 到底
		// 返回了什么导致查表失败"（spec.md 用户故事 20/21：历史记录要能看到
		// 判定理由/原始响应，即使这次处理最终失败）。
		return HighlightOutput{}, fmt.Errorf("LLM 返回的 end_id=%d 超出句子表范围 [0,%d): %w",
			window.EndID, len(sentences), ErrLlmInvalidResponse)
	}
	peakEndMs := sentences[window.EndID].EndMs

	videoLenMs, durationErr := video.Duration(videoPath)
	if durationErr != nil {
		return HighlightOutput{}, fmt.Errorf("探测视频时长失败: %w: %w", ErrVideoInspectionFailed, durationErr)
	}

	startMs, endMs := ComputeWindow(peakEndMs, targetLenMs, videoLenMs)

	config.reportProgress(StageCutting)
	if cutErr := video.CutClip(videoPath, startMs, endMs, outputPath); cutErr != nil {
		return HighlightOutput{}, fmt.Errorf("剪辑高光片段失败: %w: %w", ErrClipExtractionFailed, cutErr)
	}

	return HighlightOutput{
		OutputPath:  outputPath,
		StartMs:     startMs,
		EndMs:       endMs,
		Sentences:   sentences,
		JudgeReason: window.Reason,
	}, nil
}

// defaultOutputPath 是用户未指定输出路径时的默认值：源文件同目录下
// {source}_highlight.mp4（spec.md「输出策略」，字面示例即 .mp4）。
//
// 输出扩展名固定为 .mp4，不沿用源文件扩展名——CutClip() 固定用 H.264
// 视频编码（h264_nvenc/libx264）+ AAC 音频编码，这个编码组合能封进
// mp4/mov/mkv 容器，但封不进 webm（webm 只接受 VP8/VP9/AV1 + Vorbis/Opus）。
// 而 spec.md 用户故事 6 明确要支持 webm 源文件输入，如果输出扩展名跟随
// 源文件扩展名，webm 源文件走默认输出路径时 CutClip 会失败。
func defaultOutputPath(videoPath string) string {
	ext := filepath.Ext(videoPath)
	base := strings.TrimSuffix(filepath.Base(videoPath), ext)
	return filepath.Join(filepath.Dir(videoPath), base+"_highlight.mp4")
}

// diskFreeBytes 是 freeBytes 的可替换点，测试用它注入假的可用空间数值，
// 不依赖真的把磁盘写满来触发 ErrInsufficientDiskSpace 分支。
var diskFreeBytes = freeBytes

// checkDiskSpace 校验临时文件目录（os.TempDir()）和输出目录都至少有
// requiredBytes 可用空间——处理前一次性检查完，避免中途才因为磁盘写满失败
// （ticket 05：「磁盘空间不足时处理前报错，不中途失败」）。
func checkDiskSpace(outputPath string, requiredBytes uint64) error {
	for _, dir := range []string{os.TempDir(), filepath.Dir(outputPath)} {
		free, err := diskFreeBytes(dir)
		if err != nil {
			return fmt.Errorf("检测磁盘可用空间失败 (%s): %w", dir, err)
		}
		if free < requiredBytes {
			return fmt.Errorf("%s 可用空间 %d 字节，处理至少需要 %d 字节: %w",
				dir, free, requiredBytes, ErrInsufficientDiskSpace)
		}
	}
	return nil
}

// writeHistoryRecord 在成功和失败路径下都写一条历史记录（04-task-state-
// schema-retry.md：「每次处理完（成功或失败）...写一个 JSON 文件」）。写入
// 失败只记日志，不覆盖/掩盖 procErr——历史记录是旁路产物，它自己写失败不该
// 让整个 RunHighlightExtraction 的结果变成"写历史记录失败"而不是真正的处理
// 结果。
func writeHistoryRecord(videoPath string, output HighlightOutput, sentences []Sentence, window HighlightWindow, judged bool, procErr error) {
	rec := history.Record{
		SourcePath: videoPath,
		SourceName: filepath.Base(videoPath),
		CreatedAt:  time.Now(),
	}
	if procErr != nil {
		rec.Status = "failed"
		rec.ErrorMessage = procErr.Error()
	} else {
		rec.Status = "success"
		rec.HighlightPath = output.OutputPath
		rec.HighlightStartMs = output.StartMs
		rec.HighlightEndMs = output.EndMs
	}
	if len(sentences) > 0 {
		if raw, marshalErr := json.Marshal(sentences); marshalErr == nil {
			rec.ASRRawResult = raw
		} else {
			slog.Warn("core: failed to marshal ASR result for history record", "error", marshalErr)
		}
	}
	if judged {
		if raw, marshalErr := json.Marshal(window); marshalErr == nil {
			rec.LLMRawResponse = raw
		} else {
			slog.Warn("core: failed to marshal LLM response for history record", "error", marshalErr)
		}
	}

	if writeErr := history.WriteRecord(rec); writeErr != nil {
		slog.Warn("core: failed to write history record", "source", videoPath, "error", writeErr)
	}
}
