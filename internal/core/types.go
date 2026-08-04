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

// Stage 标识 RunHighlightExtraction() 处理流程中的一个阶段，供 GUI 层做
// 分阶段进度展示（ticket 08：「处理进度分阶段展示：音轨提取中→识别台词中→
// 判定高光中→剪辑中」）。
type Stage string

const (
	StageExtractingAudio Stage = "音轨提取中"
	StageTranscribing    Stage = "识别台词中"
	StageJudging         Stage = "判定高光中"
	StageCutting         Stage = "剪辑中"
)

// Config 是 RunHighlightExtraction() 的可选运行参数。零值字段一律有明确的
// 默认值（见 defaultTargetLenMs / defaultOutputPath），调用方不需要每次都填全。
type Config struct {
	// OutputPath 是高光片段的输出文件路径。留空时默认输出到源文件同目录下
	// {source}_highlight.mp4（spec.md「输出策略」）。
	OutputPath string
	// TargetLenMs 是高光片段的目标时长（毫秒）。留空（0）时默认 60000ms
	// （spec.md「目标时长约 60 秒」）。
	TargetLenMs uint64
	// OnProgress 在每个阶段开始前被调用（同步、阻塞调用方所在的 goroutine，
	// 不新开 goroutine——GUI 层需要自己决定要不要切回主线程更新界面）。
	// nil 表示不关心进度，RunHighlightExtraction 不会因此报错。
	OnProgress func(Stage)
}

// reportProgress 是 OnProgress 的 nil-safe 调用点。
func (c Config) reportProgress(stage Stage) {
	if c.OnProgress != nil {
		c.OnProgress(stage)
	}
}

// HighlightOutput 是 RunHighlightExtraction() 成功时的产出摘要。
type HighlightOutput struct {
	OutputPath  string
	StartMs     uint64
	EndMs       uint64
	Sentences   []Sentence
	JudgeReason string
}
