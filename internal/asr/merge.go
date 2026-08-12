package asr

import "kairos/internal/core"

// WordToken 是 ASR 字级/词级识别输出的一个 token，带独立时间戳，
// 尚未按标点恢复模型给出的句子边界合并为句级 core.Sentence。
type WordToken struct {
	Text           string
	StartMs, EndMs uint64
}

// mergeWordsToSentences 把字级/词级 ASR token 序列，按 sentenceEndIndices
// 标出的句末边界（每个元素是 words 里"作为某句最后一个 token"的下标，
// 来自标点恢复模型输出的句末标点位置），合并为句级 core.Sentence 列表。
//
// 合并规则：
//   - Sentence.ID 从 0 开始按输出顺序连续编号；
//   - Sentence.StartMs 取该句第一个 token 的 StartMs，EndMs 取最后一个 token 的 EndMs；
//   - Sentence.Text 是该句所有 token.Text 的顺序拼接（中文 token 间无需分隔符）；
//   - words 的最后一个 token 总是某一句的句末，即使 sentenceEndIndices 没有
//     显式标出（标点恢复模型漏标句尾标点时的兜底，避免丢词）；
//   - 越界（<0 或 >= len(words)）或重复的边界下标会被忽略/去重，不影响结果。
func mergeWordsToSentences(words []WordToken, sentenceEndIndices []int) []core.Sentence {
	if len(words) == 0 {
		return nil
	}

	isEnd := make(map[int]bool, len(sentenceEndIndices))
	for _, idx := range sentenceEndIndices {
		if idx >= 0 && idx < len(words) {
			isEnd[idx] = true
		}
	}

	sentences := make([]core.Sentence, 0, len(isEnd)+1)
	start := 0
	for i := range words {
		if !isEnd[i] && i != len(words)-1 {
			continue
		}

		text := concatWordTexts(words[start : i+1])

		sentences = append(sentences, core.Sentence{
			ID:      len(sentences),
			StartMs: words[start].StartMs,
			EndMs:   words[i].EndMs,
			Text:    text,
		})
		start = i + 1
	}

	return sentences
}

// concatWordTexts 把 words 的文本按顺序拼接成不带标点的整段纯文本——供
// mergeWordsToSentences 拼句用，也供 paraformer_windows.go 的标点恢复模型
// AddPunct() 调用前拼接输入。放在这个不带 build tag 的文件里，让两处调用方
// （跨平台的合并逻辑 + windows-only 的 sherpa 集成）共用同一份实现。
func concatWordTexts(words []WordToken) string {
	text := ""
	for _, w := range words {
		text += w.Text
	}
	return text
}

// 模型文件在 modelDir 下的固定相对路径约定，由本项目的模型打包脚本负责
// 按此结构放置模型文件（不是 sherpa-onnx 官方强制约定，是本项目自己的布局）。
// 跟下面的 VAD/特征提取调参一样，是 sherpa-onnx 生态的共同约定，windows/darwin
// 两个平台的 ParaformerTranscriber 实现共用同一份。
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

	// audioSampleRate 是本项目音轨提取环节固定输出的 16kHz 单声道 WAV
	// （implementation-plan.md「音轨提取」），也是 sherpa-onnx 所有官方预训练
	// 模型（VAD + Paraformer 特征提取）期望的采样率，两处复用同一个常量。
	audioSampleRate = 16000
	// featureDim 是 sherpa-onnx 官方预训练模型固定期望的特征维度
	// （sherpa.FeatureConfig 文档原话："It is 80 for all pre-trained models
	// provided by us"）。
	featureDim = 80
	// vadBufferSizeSeconds 覆盖 spec.md 约定的源视频时长上限（1-3 分钟）
	// 再留出余量——原值 30 太小，真机验证时对 1 分钟以上素材稳定触发
	// sherpa-onnx 内部 circular-buffer 的 "Overflow...Increase capacity to"
	// 日志（自动扩容、sherpa-onnx 自己保证 "No data loss"，不是正确性 bug，
	// 但每次都要重新分配+拷贝，且日志噪音掩盖真正需要关注的问题）。
	vadBufferSizeSeconds = 300
	// vadWindowSize 是喂给 Silero-VAD 每次 AcceptWaveform 调用的样本数
	// （16kHz 下 512 样本 = 32ms）——必须跟 SileroVadModelConfig.WindowSize
	// 保持一致，Transcribe() 按这个粒度分块喂音频，不能改成一次性喂整段
	// （见各平台 Transcribe() 的 doc comment，这是真机排查定位到的根因）。
	vadWindowSize = 512
	// minSegmentDurationSec 是丢弃 VAD 切出的过短片段的阈值，跟 sherpa-onnx
	// 官方参考实现（sherpa-onnx-vad-with-offline-asr.cc）用的 0.1 秒一致。
	minSegmentDurationSec = 0.1
)

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
