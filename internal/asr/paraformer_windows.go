//go:build windows

package asr

import (
	"fmt"
	"path/filepath"

	sherpa "github.com/k2-fsa/sherpa-onnx-go-windows"

	"kairos/internal/core"
)

// 模型文件在 modelDir 下的固定相对路径约定，由本项目的模型打包脚本负责
// 按此结构放置模型文件（不是 sherpa-onnx 官方强制约定，是本项目自己的布局）。
//
// 注意：implementation-plan.md 里写的 VAD 是 "FSMN-VAD"，但
// github.com/k2-fsa/sherpa-onnx-go-windows（截至 v1.13.4，2025 年最新发布版）
// 的 VadModelConfig 只暴露 SileroVad / TenVad 两种模型配置，没有 FSMN 选项——
// 上游没有对应绑定，不是本实现遗漏。这里改用该绑定官方支持、同样是
// sherpa-onnx 生态标准配置的 Silero-VAD 模型，这是本次实现对 ticket 文字描述
// 的一处必要偏离，请见最终报告。
const (
	paraformerModelFile  = "paraformer/model.int8.onnx"
	paraformerTokensFile = "paraformer/tokens.txt"
	vadModelFile         = "vad/silero_vad.onnx"
	punctuationModelFile = "punctuation/model.onnx"

	vadSampleRate        = 16000
	vadBufferSizeSeconds = 30
)

// ParaformerTranscriber 用 sherpa-onnx 加载本地 Paraformer-large 离线识别模型
// + Silero-VAD + 标点恢复模型，实现 core.Transcriber。
//
// 本文件只在 windows 下编译（sherpa-onnx-go-windows 的 Go 源码本身也是
// //go:build windows && (amd64 || 386) 门控的，在其它平台上不存在可编译的
// 实现），且依赖真实 ONNX Runtime + CUDA + 模型文件才能跑通，因此本文件的
// 正确性无法在非 Windows 开发机上验证，只能靠人工代码走查。
type ParaformerTranscriber struct {
	recognizer  *sherpa.OfflineRecognizer
	vad         *sherpa.VoiceActivityDetector
	punctuation *sherpa.OfflinePunctuation
}

// NewParaformerTranscriber 加载 modelDir 下的 Paraformer-large + Silero-VAD +
// 标点恢复模型。useCuda=true 时优先用 CUDA execution provider 初始化识别器；
// 初始化失败（驱动/运行时缺失等）时自动降级为 CPU provider 重试，不直接报错，
// 只有 CPU 也失败才返回 error（VAD、标点模型不参与 CUDA/CPU 降级，
// 它们的推理开销本身很小，官方绑定固定用 CPU provider 即可）。
func NewParaformerTranscriber(modelDir string, useCuda bool) (*ParaformerTranscriber, error) {
	provider := "cpu"
	if useCuda {
		provider = "cuda"
	}

	recognizer, err := newOfflineRecognizer(modelDir, provider)
	if err != nil && provider == "cuda" {
		recognizer, err = newOfflineRecognizer(modelDir, "cpu")
	}
	if err != nil {
		return nil, fmt.Errorf("asr: 加载 Paraformer 识别模型失败: %w", err)
	}

	vad := sherpa.NewVoiceActivityDetector(&sherpa.VadModelConfig{
		SileroVad: sherpa.SileroVadModelConfig{
			Model:              filepath.Join(modelDir, vadModelFile),
			Threshold:          0.5,
			MinSilenceDuration: 0.5,
			MinSpeechDuration:  0.25,
			WindowSize:         512,
			MaxSpeechDuration:  20,
		},
		SampleRate: vadSampleRate,
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

// newOfflineRecognizer 用给定 execution provider 初始化一次 Paraformer-large
// 离线识别器；nil 返回值时 sherpa-onnx 官方绑定不暴露具体失败原因，
// 只能返回一个笼统的 error。
func newOfflineRecognizer(modelDir, provider string) (*sherpa.OfflineRecognizer, error) {
	recognizer := sherpa.NewOfflineRecognizer(&sherpa.OfflineRecognizerConfig{
		ModelConfig: sherpa.OfflineModelConfig{
			Paraformer: sherpa.OfflineParaformerModelConfig{
				Model: filepath.Join(modelDir, paraformerModelFile),
			},
			Tokens:     filepath.Join(modelDir, paraformerTokensFile),
			NumThreads: 1,
			Provider:   provider,
		},
		DecodingMethod: "greedy_search",
	})
	if recognizer == nil {
		return nil, fmt.Errorf("provider=%s 初始化失败", provider)
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
func (p *ParaformerTranscriber) Transcribe(audioPath string) ([]core.Sentence, error) {
	wave := sherpa.ReadWave(audioPath)
	if wave == nil {
		return nil, fmt.Errorf("asr: 读取音频文件失败: %s", audioPath)
	}

	p.vad.Reset()
	p.vad.AcceptWaveform(wave.Samples)
	p.vad.Flush()

	var words []WordToken
	for !p.vad.IsEmpty() {
		segment := p.vad.Front()
		p.vad.Pop()

		stream := sherpa.NewOfflineStream(p.recognizer)
		stream.AcceptWaveform(wave.SampleRate, segment.Samples)
		p.recognizer.Decode(stream)
		result := stream.GetResult()
		sherpa.DeleteOfflineStream(stream)

		segmentStartMs := uint64(float64(segment.Start) / float64(wave.SampleRate) * 1000)
		segmentEndMs := segmentStartMs + uint64(float64(len(segment.Samples))/float64(wave.SampleRate)*1000)
		words = append(words, tokensToWords(result, segmentStartMs, segmentEndMs)...)
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

// sentenceEndIndicesFromPunctuation 把标点恢复模型输出的带标点文本，
// 对齐回原始 words 序列，定位每个句末标点对应的 word 下标。
//
// 对齐方式：punctuated 除标点符号外，字符序列应与 words 拼接出的纯文本一一
// 对应（CT-Transformer 标点恢复模型只插入标点、不改写原文），逐字符扫描时
// 按当前 word 的 rune 长度推进 word 下标，遇到句末标点（。！？…）就记录
// "当前已消费完的 word 下标" 为一个句子边界。
func sentenceEndIndicesFromPunctuation(words []WordToken, punctuated string) []int {
	sentenceEndPunct := map[rune]bool{
		'。': true, '！': true, '？': true, '…': true,
		'.': true, '!': true, '?': true,
	}

	var boundaries []int
	wordIdx := 0
	runesLeftInWord := 0
	if len(words) > 0 {
		runesLeftInWord = len([]rune(words[0].Text))
	}

	for _, r := range punctuated {
		if sentenceEndPunct[r] {
			if wordIdx > 0 && wordIdx-1 < len(words) {
				boundaries = append(boundaries, wordIdx-1)
			}
			continue
		}
		if wordIdx >= len(words) {
			continue
		}
		runesLeftInWord--
		if runesLeftInWord <= 0 {
			wordIdx++
			if wordIdx < len(words) {
				runesLeftInWord = len([]rune(words[wordIdx].Text))
			}
		}
	}

	return boundaries
}
