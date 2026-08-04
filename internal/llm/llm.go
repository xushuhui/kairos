// Package llm 封装 DeepSeek V4-flash 客户端（github.com/sashabaranov/go-openai），
// 实现 core.HighlightJudge。
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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

	// jsonShapeInstruction 显式写出目标 JSON 形状，追加在 systemPrompt 之后发给
	// 模型——spec.md「### LLM Prompt 结构」：「DeepSeek 官方 API 只支持
	// response_format.type 为 text 或 json_object，不支持 OpenAI 的 json_schema
	// 严格模式——schema 约束通过在 prompt 里显式写出目标 JSON 形状达成」。
	//
	// 2026-08-03 用真实 API Key 实测发现：这条本来只被当成"写代码时参考的
	// 文档"，从未真的拼进发给模型的 prompt 里——mock server 测试测不出这个
	// 遗漏（httptest 不会校验 DeepSeek 的这条硬性要求），直到真打一次真实
	// DeepSeek 端点才报错：「Prompt must contain the word 'json' in some form
	// to use response_format of type json_object」。这个字段既补上了 spec 本来
	// 就要求的 JSON 形状说明，也顺带满足了 DeepSeek 这条"prompt 里必须出现
	// json 字样"的硬性要求。
	jsonShapeInstruction = "请以 json 格式输出，形状为：" +
		`{"narrative_structure":{"setup":[开始句子id,结束句子id],"rising_action":[开始句子id,结束句子id],` +
		`"climax":[开始句子id,结束句子id],"resolution":[开始句子id,结束句子id]或null},` +
		`"start_id":整数,"end_id":整数,"reason":字符串,` +
		`"candidate_sentences":[{"start_id":整数,"end_id":整数,"label":字符串}]}`
)

// ErrLlmTimeout 在 DeepSeek 请求超过超时上限时返回。
var ErrLlmTimeout = errors.New("llm: DeepSeek 请求超时")

// ErrInvalidAPIKey 在 DeepSeek 因 API Key 无效/未授权（HTTP 401）拒绝请求时
// 返回——GUI 层（cmd/kairos）据此跟其余"高光判定失败"场景区分开，提示用户
// 去设置里重新输入 Key，而不是笼统地说"判定失败，稍后重试"。
var ErrInvalidAPIKey = errors.New("llm: DeepSeek API Key 无效")

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
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt + "\n\n" + jsonShapeInstruction},
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
		var apiErr *openai.APIError
		if errors.As(err, &apiErr) && apiErr.HTTPStatusCode == http.StatusUnauthorized {
			return core.HighlightWindow{}, ErrInvalidAPIKey
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

// SaveAPIKey 把 apiKey 写入标准配置目录的 config.json（deepseek.api_key
// 字段），供 GUI 首次运行引导流程调用（ticket 08）。已存在的配置文件会先
// 读出来再改写对应字段，不清空文件里其余尚未使用到的字段（如
// output.default_dir）——用 map[string]json.RawMessage 做通用合并，不需要
// 为配置文件的完整形状单独建模。
func SaveAPIKey(apiKey string) error {
	if strings.TrimSpace(apiKey) == "" {
		return errors.New("llm: api key 不能为空")
	}

	dir, err := userConfigDir()
	if err != nil {
		return fmt.Errorf("llm: 无法定位配置目录: %w", err)
	}
	confDir := filepath.Join(dir, "kairos")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		return fmt.Errorf("llm: 创建配置目录失败: %w", err)
	}
	path := filepath.Join(confDir, "config.json")

	raw := map[string]json.RawMessage{}
	if data, readErr := os.ReadFile(path); readErr == nil {
		if uerr := json.Unmarshal(data, &raw); uerr != nil {
			return fmt.Errorf("llm: 解析已有配置文件失败: %w", uerr)
		}
	} else if !os.IsNotExist(readErr) {
		return fmt.Errorf("llm: 读取配置文件失败: %w", readErr)
	}

	deepseek := map[string]json.RawMessage{}
	if existing, ok := raw["deepseek"]; ok {
		if uerr := json.Unmarshal(existing, &deepseek); uerr != nil {
			return fmt.Errorf("llm: 解析已有 deepseek 配置失败: %w", uerr)
		}
	}
	apiKeyJSON, err := json.Marshal(apiKey)
	if err != nil {
		return fmt.Errorf("llm: 序列化 api_key 失败: %w", err)
	}
	deepseek["api_key"] = apiKeyJSON
	deepseekJSON, err := json.Marshal(deepseek)
	if err != nil {
		return fmt.Errorf("llm: 序列化 deepseek 配置失败: %w", err)
	}
	raw["deepseek"] = deepseekJSON

	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return fmt.Errorf("llm: 序列化配置文件失败: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("llm: 写入配置文件失败: %w", err)
	}
	return nil
}
