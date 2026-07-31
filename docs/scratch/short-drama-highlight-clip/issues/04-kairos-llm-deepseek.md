# 04 — internal/llm：DeepSeek 客户端

**What to build:** 用 `github.com/sashabaranov/go-openai` 接 DeepSeek V4-flash（`BaseURL` 指向 `https://api.deepseek.com`），实现 `core.HighlightJudge` interface；API Key 从本地 JSON 配置文件读取（可选 Windows 凭据管理器）。

**Blocked by:** None — can start immediately.

**Status:** ready-for-agent

- [ ] 用固定台词文本调用真实 DeepSeek API，`response_format` 用 `json_object`（不是 `json_schema`，DeepSeek 官方不支持后者）
- [ ] 响应能反序列化为含 `narrative_structure`/`start_id`/`end_id`/`reason`/`candidate_sentences` 字段的 Go struct，字段类型不对时报错
- [ ] API Key 从 `%APPDATA%/kairos/config.json` 正确读取
- [ ] 请求超时上限 30s（`context.WithTimeout`），超时返回明确的 `ErrLlmTimeout`
