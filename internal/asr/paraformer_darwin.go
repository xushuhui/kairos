//go:build darwin

package asr

import (
	"fmt"
	"log/slog"
	"path/filepath"

	sherpa "github.com/k2-fsa/sherpa-onnx-go-macos"

	"kairos/internal/core"
)

// ParaformerTranscriber 用 sherpa-onnx 加载本地 Paraformer-large 离线识别模型
// + Silero-VAD + 标点恢复模型，实现 core.Transcriber。
//
// 本文件只在 darwin 下编译，是 paraformer_windows.go 的镜像实现——两个平台
// 用的都是 sherpa-onnx 官方 Go 绑定，API 表面逐字段核对过完全一致（截至
// sherpa-onnx-go-windows@v1.13.4 / sherpa-onnx-go-macos@v1.13.5），差别只在
// import 的具体平台包。没有 sherpa 类型依赖的公共逻辑已经提到不带 build tag
// 的 merge.go 里共用（模型文件路径约定、VAD 调参常量、标点边界对齐）。
//
// 加这份 darwin 实现是用户明确要求的开发便利，不是要把 macOS 变成正式支持
// 的部署平台——map.md「Out of scope」里"只做 Windows"的决定没有变，
// 这份实现只是为了能在本机（而不是来回在 Windows 机器上贴日志）直接跑通
// 音轨提取→VAD→Paraformer→标点恢复整条链路，验证 Go 侧逻辑（尤其是 VAD
// 分块喂入这类跟平台无关的调用方式问题）本身是对的，减少 Windows 那边的
// 排查轮次。
type ParaformerTranscriber struct {
	recognizer  *sherpa.OfflineRecognizer
	vad         *sherpa.VoiceActivityDetector
	punctuation *sherpa.OfflinePunctuation
}

// NewParaformerTranscriber 加载 modelDir 下的 Paraformer-large + Silero-VAD +
// 标点恢复模型，固定用 CPU provider——跟 Windows 侧保持一致的决策（用户
// 明确要求去掉 CUDA execution provider，见 paraformer_windows.go 的
// NewParaformerTranscriber doc comment），CPU provider 没有外部运行时依赖，
// 只要模型文件本身正确就能稳定跑起来。
func NewParaformerTranscriber(modelDir string) (*ParaformerTranscriber, error) {
	recognizer, err := newOfflineRecognizer(modelDir)
	if err != nil {
		return nil, fmt.Errorf("asr: 加载 Paraformer 识别模型失败: %w", err)
	}
	slog.Info("asr: paraformer recognizer initialized", "provider", "cpu")

	vad := sherpa.NewVoiceActivityDetector(&sherpa.VadModelConfig{
		SileroVad: sherpa.SileroVadModelConfig{
			Model:              filepath.Join(modelDir, vadModelFile),
			Threshold:          0.5,
			MinSilenceDuration: 0.5,
			MinSpeechDuration:  0.25,
			WindowSize:         vadWindowSize,
			MaxSpeechDuration:  20,
		},
		SampleRate: audioSampleRate,
		NumThreads: 1,
		Provider:   "cpu",
	}, vadBufferSizeSeconds)
	if vad == nil {
		sherpa.DeleteOfflineRecognizer(recognizer)
		return nil, fmt.Errorf("asr: 加载 VAD 模型失败: %s", filepath.Join(modelDir, vadModelFile))
	}

	punctuation := sherpa.NewOfflinePunctuation(&sherpa.OfflinePunctuationConfig{
		Model: sherpa.OfflinePunctuationModelConfig{
			CtTransformer: filepath.Join(modelDir, punctuationModelFile),
			NumThreads:    1,
			Provider:      "cpu",
		},
	})
	if punctuation == nil {
		sherpa.DeleteOfflineRecognizer(recognizer)
		sherpa.DeleteVoiceActivityDetector(vad)
		return nil, fmt.Errorf("asr: 加载标点恢复模型失败: %s", filepath.Join(modelDir, punctuationModelFile))
	}

	return &ParaformerTranscriber{recognizer: recognizer, vad: vad, punctuation: punctuation}, nil
}

// newOfflineRecognizer 用 CPU provider 初始化一次 Paraformer-large 离线
// 识别器；nil 返回值时 sherpa-onnx 官方绑定不暴露具体失败原因，只能返回
// 一个笼统的 error。
//
// FeatConfig 是必填项——sherpa.FeatureConfig{} 零值（SampleRate=0,
// FeatureDim=0）会导致底层 C API 直接拒绝构造识别器（真机排查定位到的
// bug，见 paraformer_windows.go 对应函数的 doc comment）。
func newOfflineRecognizer(modelDir string) (*sherpa.OfflineRecognizer, error) {
	recognizer := sherpa.NewOfflineRecognizer(&sherpa.OfflineRecognizerConfig{
		FeatConfig: sherpa.FeatureConfig{
			SampleRate: audioSampleRate,
			FeatureDim: featureDim,
		},
		ModelConfig: sherpa.OfflineModelConfig{
			Paraformer: sherpa.OfflineParaformerModelConfig{
				Model: filepath.Join(modelDir, paraformerModelFile),
			},
			Tokens:     filepath.Join(modelDir, paraformerTokensFile),
			NumThreads: 1,
			Provider:   "cpu",
		},
		DecodingMethod: "greedy_search",
	})
	if recognizer == nil {
		return nil, fmt.Errorf("provider=cpu 初始化失败")
	}
	return recognizer, nil
}

// Close 释放底层 C 对象持有的内存，调用方在用完 ParaformerTranscriber 后必须调用。
func (p *ParaformerTranscriber) Close() {
	if p.recognizer != nil {
		sherpa.DeleteOfflineRecognizer(p.recognizer)
	}
	if p.vad != nil {
		sherpa.DeleteVoiceActivityDetector(p.vad)
	}
	if p.punctuation != nil {
		sherpa.DeleteOfflinePunc(p.punctuation)
	}
}

// Transcribe 读取 audioPath 指向的 WAV 音频，先跑 VAD 切出语音片段，
// 再对每个片段跑 Paraformer 推理拿字级 token + 时间戳，最后用标点恢复模型
// 给拼接文本加标点、定位句末边界，调 mergeWordsToSentences 合并为句级 core.Sentence。
//
// VAD 必须按 vadWindowSize（512 样本 = 32ms）分块喂，每块喂完立刻排空
// p.vad 里已经切好的片段——不能一次性把整段音频丢给 AcceptWaveform() 再
// 统一排空，这条是跨平台的，跟 Windows 侧真机排查出的根因完全一样（详见
// paraformer_windows.go 对应函数的 doc comment），照抄 sherpa-onnx 官方
// C++ 参考实现（sherpa-onnx-vad-with-offline-asr.cc 的 main 循环）。
func (p *ParaformerTranscriber) Transcribe(audioPath string) ([]core.Sentence, error) {
	wave := sherpa.ReadWave(audioPath)
	if wave == nil {
		return nil, fmt.Errorf("asr: 读取音频文件失败: %s", audioPath)
	}

	p.vad.Reset()

	var words []WordToken
	samples := wave.Samples
	for i := 0; i < len(samples); i += vadWindowSize {
		if end := i + vadWindowSize; end <= len(samples) {
			p.vad.AcceptWaveform(samples[i:end])
		} else {
			// 剩余样本不足一整个窗口——Flush() 强制处理这段尾巴，避免
			// 贴着音频结束边界的最后一段对白因为凑不满一个完整窗口而丢失。
			p.vad.Flush()
		}

		for !p.vad.IsEmpty() {
			segment := p.vad.Front()
			segmentDurationSec := float64(len(segment.Samples)) / float64(wave.SampleRate)
			if segmentDurationSec < minSegmentDurationSec {
				p.vad.Pop()
				continue
			}

			stream := sherpa.NewOfflineStream(p.recognizer)
			stream.AcceptWaveform(wave.SampleRate, segment.Samples)
			p.recognizer.Decode(stream)
			result := stream.GetResult()
			sherpa.DeleteOfflineStream(stream)

			segmentStartMs := uint64(float64(segment.Start) / float64(wave.SampleRate) * 1000)
			segmentEndMs := segmentStartMs + uint64(segmentDurationSec*1000)
			words = append(words, tokensToWords(result, segmentStartMs, segmentEndMs)...)
			p.vad.Pop()
		}
	}

	if len(words) == 0 {
		return nil, nil
	}

	plainText := concatWordTexts(words)
	punctuated := p.punctuation.AddPunct(plainText)
	boundaries := sentenceEndIndicesFromPunctuation(words, punctuated)

	return mergeWordsToSentences(words, boundaries), nil
}

// tokensToWords 把一个语音片段的 Paraformer 识别结果（字级 token + 相对该
// 片段起点的秒级时间戳）转换为绝对毫秒时间戳的 WordToken，segmentStartMs/
// segmentEndMs 是该片段在整段音频里的绝对起止毫秒（来自 VAD 切分）。
func tokensToWords(result *sherpa.OfflineRecognizerResult, segmentStartMs, segmentEndMs uint64) []WordToken {
	if result == nil || len(result.Tokens) == 0 {
		return nil
	}

	hasDurations := len(result.Durations) == len(result.Tokens)
	hasTimestamps := len(result.Timestamps) == len(result.Tokens)

	words := make([]WordToken, len(result.Tokens))
	for i, text := range result.Tokens {
		start := segmentStartMs
		if hasTimestamps {
			start = segmentStartMs + uint64(result.Timestamps[i]*1000)
		}

		end := segmentEndMs
		switch {
		case hasDurations:
			end = start + uint64(result.Durations[i]*1000)
		case i+1 < len(result.Tokens) && hasTimestamps:
			end = segmentStartMs + uint64(result.Timestamps[i+1]*1000)
		}
		if end < start {
			end = start
		}

		words[i] = WordToken{Text: text, StartMs: start, EndMs: end}
	}
	return words
}
