// Package core 是整个项目唯一的编排与纯逻辑节点：领域类型、两个测试缝合点
// interface（Transcriber / HighlightJudge）、以及零 I/O 的 ComputeWindow() 都定义在这里。
// 其余包（video/asr/llm/history）只依赖 core 暴露的类型，互相之间零依赖。
package core

// Sentence 是 ASR 转写输出的一句台词，带句级毫秒时间戳。
type Sentence struct {
	ID      int
	StartMs uint64
	EndMs   uint64
	Text    string
}

// NarrativeStructure 是 LLM 判定高光窗口前先给出的全剧叙事结构定位，
// 只作为 prompt 设计里的中间产物，不参与后续代码逻辑。
type NarrativeStructure struct {
	Setup        [2]int
	RisingAction [2]int
	Climax       [2]int
	Resolution   *[2]int // nil 表示无明显结局
}

// CandidateWindow 是 LLM 判定过程中扫描到的候选片段，供代码做二次排序补偿。
type CandidateWindow struct {
	StartID int
	EndID   int
	Label   string
}

// HighlightWindow 是 HighlightJudge 的判定结果：以句子 ID（不是毫秒时间戳）
// 表达的高光窗口边界，时间戳映射由 core 编排层查 ASR 句子表完成。
type HighlightWindow struct {
	NarrativeStructure NarrativeStructure
	StartID            int
	EndID              int
	Reason             string
	Candidates         []CandidateWindow
}
