package llm

import (
	"os"
	"testing"
	"time"

	"kairos/internal/core"
)

// TestDeepSeekJudge_Judge_RealAPI is the "真实适配器集成测试" spec.md's
// Testing Decisions describes for HighlightJudge: "用预置的短台词文本调用一
// 次真实 DeepSeek API，验证返回 JSON 结构合法（start_id ≤ end_id，都有
// reason 字段）". Per spec.md: "这两类集成测试不进常规 CI，标记为
// manual-run，因为需要...有效 API Key + 网络" — gated behind DEEPSEEK_API_KEY
// being set, skipped otherwise (never runs in the normal go test ./... suite
// on a machine without a real key).
func TestDeepSeekJudge_Judge_RealAPI(t *testing.T) {
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		t.Skip("DEEPSEEK_API_KEY not set, skipping manual-run real API integration test")
	}

	// 固定台词文本：一段短剧式对白，带一个转折/冲突点，用于验证 LLM 能不能
	// 挑出合理的连续窗口——不是随便找几句话，要有真实的"高光候选"可判定。
	sentences := []core.Sentence{
		{ID: 0, StartMs: 0, EndMs: 2000, Text: "妈，我跟你说了多少次，我跟阿哲是真心的。"},
		{ID: 1, StartMs: 2000, EndMs: 4500, Text: "真心？他连自己姓什么都是假的，你还敢说真心！"},
		{ID: 2, StartMs: 4500, EndMs: 6000, Text: "什么意思？"},
		{ID: 3, StartMs: 6000, EndMs: 9000, Text: "你以为他叫陈哲？他本名姓顾，十年前那场大火，就是他放的。"},
		{ID: 4, StartMs: 9000, EndMs: 10500, Text: "不可能，你在骗我。"},
		{ID: 5, StartMs: 10500, EndMs: 13500, Text: "我骗你干什么，那场火烧死的，是你亲舅舅。"},
		{ID: 6, StartMs: 13500, EndMs: 15000, Text: "……"},
		{ID: 7, StartMs: 15000, EndMs: 18000, Text: "所以这些年他一直在你身边，你猜他图什么？"},
		{ID: 8, StartMs: 18000, EndMs: 19500, Text: "妈，你先冷静，我们坐下说。"},
		{ID: 9, StartMs: 19500, EndMs: 22000, Text: "冷静？他今晚就要跟你去领证了！"},
	}

	judge := NewDeepSeekJudge(apiKey)
	// 2026-08-03 实测：DeepSeek 当前负载下（"Service is too busy"）单次请求
	// 实测耗时可达 45s+。2026-08-11 生产默认超时已从 30s 调到 60s（用户
	// 真机实测触发过 ErrLlmTimeout 后明确要求），这里仍显式设成比生产默认
	// 更宽松的 90s，给这条真实网络的人工验证留够余量，不依赖会变的默认值。
	judge.timeout = 90 * time.Second
	got, err := judge.Judge(sentences)
	if err != nil {
		t.Fatalf("Judge() error = %v", err)
	}

	if got.StartID > got.EndID {
		t.Errorf("StartID=%d > EndID=%d, want StartID <= EndID", got.StartID, got.EndID)
	}
	if got.StartID < 0 || got.EndID >= len(sentences) {
		t.Errorf("window [%d,%d] out of sentence table range [0,%d)", got.StartID, got.EndID, len(sentences))
	}
	if got.Reason == "" {
		t.Error("Reason is empty, want a non-empty judging rationale")
	}

	t.Logf("real DeepSeek judge result: window=[%d,%d] reason=%q narrative_structure=%+v",
		got.StartID, got.EndID, got.Reason, got.NarrativeStructure)
}
