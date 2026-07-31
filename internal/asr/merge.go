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
