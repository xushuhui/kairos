// Package llm 封装 DeepSeek V4-flash 客户端（github.com/sashabaranov/go-openai），
// 实现 core.HighlightJudge。
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"

	"kairos/internal/core"
)

const (
	deepSeekBaseURL = "https://api.deepseek.com"
	deepSeekModel   = "deepseek-v4-flash"

	// defaultTimeout 是单次 DeepSeek 请求的超时上限，见 spec.md「云端 LLM 接入」。
	defaultTimeout = 30 * time.Second

	// systemPrompt 是固定角色设定，逐字取自 spec.md「### LLM Prompt 结构」，不得改写。
	systemPrompt = "你是短剧广告投放剪辑师，从带编号的台词列表中先识别开端/发展/高潮/结局的叙事结构作为背景参考，" +
		"再挑出一段最适合做广告投放钩子的连续片段——目标不是'剧情上最重要的一段'，" +
		"而是'从未看过这部剧的路人观众刷到后会忍不住点进去看正片'的片段，目标时长约 60 秒"
)

// ErrLlmTimeout 在 DeepSeek 请求超过超时上限时返回。
var ErrLlmTimeout = errors.New("llm: DeepSeek 请求超时")

// userConfigDir 是 os.UserConfigDir 的可替换指向，供测试注入临时目录。
var userConfigDir = os.UserConfigDir

// DeepSeekJudge 通过 DeepSeek V4-flash（OpenAI 兼容接口）实现 core.HighlightJudge。
type DeepSeekJudge struct {
	client  *openai.Client
	model   string
	timeout time.Duration
}

var _ core.HighlightJudge = (*DeepSeekJudge)(nil)

// NewDeepSeekJudge 构造指向真实 DeepSeek 端点的生产用 DeepSeekJudge。
func NewDeepSeekJudge(apiKey string) *DeepSeekJudge {
	cfg := openai.DefaultConfig(apiKey)
	cfg.BaseURL = deepSeekBaseURL
	return newDeepSeekJudge(cfg)
}

// newDeepSeekJudge 是未导出的注入点：接受任意 openai.ClientConfig，
// 供包内测试指向 httptest.Server 而不必访问真实 DeepSeek。
func newDeepSeekJudge(cfg openai.ClientConfig) *DeepSeekJudge {
	return &DeepSeekJudge{
		client:  openai.NewClientWithConfig(cfg),
		model:   deepSeekModel,
		timeout: defaultTimeout,
	}
}

// Judge 把分句台词交给 DeepSeek 判定最适合做广告钩子的连续窗口。
func (j *DeepSeekJudge) Judge(sentences []core.Sentence) (core.HighlightWindow, error) {
	ctx, cancel := context.WithTimeout(context.Background(), j.timeout)
	defer cancel()

	req := openai.ChatCompletionRequest{
		Model: j.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: buildUserMessage(sentences)},
		},
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONObject,
		},
	}

	resp, err := j.client.CreateChatCompletion(ctx, req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return core.HighlightWindow{}, ErrLlmTimeout
		}
		return core.HighlightWindow{}, fmt.Errorf("llm: DeepSeek 请求失败: %w", err)
	}
	if len(resp.Choices) == 0 {
		return core.HighlightWindow{}, errors.New("llm: DeepSeek 响应不含任何 choice")
	}

	return parseHighlightWindow(resp.Choices[0].Message.Content)
}

// buildUserMessage 把分句台词拼成 `id, start_ms, end_ms, text` 每行一句的纯文本，带列标题。
func buildUserMessage(sentences []core.Sentence) string {
	var b strings.Builder
	b.WriteString("id, start_ms, end_ms, text\n")
	for _, s := range sentences {
		fmt.Fprintf(&b, "%d, %d, %d, %s\n", s.ID, s.StartMs, s.EndMs, s.Text)
	}
	return b.String()
}

// wireHighlightWindow 镜像 spec.md/06 文档里约定的 LLM 响应 JSON 形状。
type wireHighlightWindow struct {
	NarrativeStructure struct {
		Setup        [2]int  `json:"setup"`
		RisingAction [2]int  `json:"rising_action"`
		Climax       [2]int  `json:"climax"`
		Resolution   *[2]int `json:"resolution"`
	} `json:"narrative_structure"`
	StartID            int    `json:"start_id"`
	EndID              int    `json:"end_id"`
	Reason             string `json:"reason"`
	CandidateSentences []struct {
		StartID int    `json:"start_id"`
		EndID   int    `json:"end_id"`
		Label   string `json:"label"`
	} `json:"candidate_sentences"`
}

// parseHighlightWindow 把 DeepSeek 响应的 content 字段反序列化为 core.HighlightWindow。
// 字段类型不对时 encoding/json 会直接返回 *json.UnmarshalTypeError，不做额外校验。
func parseHighlightWindow(content string) (core.HighlightWindow, error) {
	var wire wireHighlightWindow
	if err := json.Unmarshal([]byte(content), &wire); err != nil {
		return core.HighlightWindow{}, fmt.Errorf("llm: 解析 DeepSeek 响应失败: %w", err)
	}

	candidates := make([]core.CandidateWindow, len(wire.CandidateSentences))
	for i, c := range wire.CandidateSentences {
		candidates[i] = core.CandidateWindow{StartID: c.StartID, EndID: c.EndID, Label: c.Label}
	}

	return core.HighlightWindow{
		NarrativeStructure: core.NarrativeStructure{
			Setup:        wire.NarrativeStructure.Setup,
			RisingAction: wire.NarrativeStructure.RisingAction,
			Climax:       wire.NarrativeStructure.Climax,
			Resolution:   wire.NarrativeStructure.Resolution,
		},
		StartID:    wire.StartID,
		EndID:      wire.EndID,
		Reason:     wire.Reason,
		Candidates: candidates,
	}, nil
}

// apiKeyConfig 镜像 spec.md「### 云端 LLM 接入」约定的配置文件 JSON 形状。
type apiKeyConfig struct {
	DeepSeek struct {
		APIKey string `json:"api_key"`
	} `json:"deepseek"`
}

// LoadAPIKey 从标准配置目录（os.UserConfigDir()/kairos/config.json）读取 DeepSeek API Key。
func LoadAPIKey() (string, error) {
	dir, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("llm: 无法定位配置目录: %w", err)
	}

	path := filepath.Join(dir, "kairos", "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("llm: 读取配置文件失败: %w", err)
	}

	var cfg apiKeyConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("llm: 解析配置文件失败: %w", err)
	}
	if cfg.DeepSeek.APIKey == "" {
		return "", errors.New("llm: 配置文件中未设置 deepseek.api_key")
	}

	return cfg.DeepSeek.APIKey, nil
}
