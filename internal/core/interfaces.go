package core

// Transcriber 把一段音频转写为带时间戳的分句文本。
// 生产实现封装本地 Paraformer ONNX 推理（internal/asr）；
// 测试实现返回固定的句子列表，是 RunHighlightExtraction 端到端测试的两个注入缝合点之一。
type Transcriber interface {
	Transcribe(audioPath string) ([]Sentence, error)
}

// HighlightJudge 从分句文本中判定最适合做广告钩子的连续窗口。
// 生产实现调用云端 LLM（internal/llm）；
// 测试实现返回固定的 {StartID, EndID, Reason}，是另一个注入缝合点。
type HighlightJudge interface {
	Judge(sentences []Sentence) (HighlightWindow, error)
}
