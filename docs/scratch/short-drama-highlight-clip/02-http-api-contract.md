# DeepSeek 云端 API 客户端集成设计

**Status:** resolved
**Created:** 2026-07-23 (repurposed)
**Label:** wayfinder:grilling
**Parent:** docs/scratch/short-drama-highlight-clip/map.md
**Assignee:** none

## Question

桌面软件中，调用云端 DeepSeek V4-flash LLM 作高光窗口判定的客户端应如何集成？需要确定：
- Go 侧接入方式：直接用标准库 `net/http` 拼请求，还是用 `sashabaranov/go-openai` 等 OpenAI Go SDK 封装
- DeepSeek 的 OpenAI 兼容 base URL（`https://api.deepseek.com`）与模型名（`deepseek-v4-flash`），桌面软件需要独立的、指向真实 DeepSeek 云端的客户端
- API Key 的来源与生命周期：用户在 UI 中输入 API Key 后，存于本地配置文件（JSON 到 `%APPDATA%`）还是 Windows 凭据管理器（Credential Manager）——后者更安全但需要额外系统 API 调用
- 请求结构：system prompt、含完整台词文本 + 句子 ID 的 user message、`response_format: json_object` + prompt 内嵌 schema 说明，约束输出 `{start_id, end_id, reason}` 格式
- 超时与重试策略：桌面场景不同于服务端，用户可接受等待几秒，但需有明确的超时上限和失败提示 UI

## Key decisions to make

1. **Go 客户端**（已确认）：使用 `github.com/sashabaranov/go-openai`——直接支持 `response_format`、OpenAI 全协议格式、含自定义 `BaseURL` 切换供应商。DeepSeek 官方声明 OpenAI 兼容，`go-openai` 换个 `BaseURL` 配置即可直接对接，不需要自己拼请求体或处理流式解析。`fm-kafka`（同源 Go 项目）的 `utils/deepseek.go` 已经用同一个库对接过（目前指向本地 ollama，需要改指向真实 DeepSeek 云端点，不能直接复用现有连接配置，但库选型和使用模式有先例可循）。

2. **API Key 存储位置**（已确认，JSON 不用 TOML）：默认写本地配置文件（JSON，`%APPDATA%`，标准库 `encoding/json`，不引入 TOML 依赖——配置内容极简且项目别处已用 JSON），可选勾选启用 Windows 凭据管理器（`github.com/danieljoos/wincred` 封装 CredWriteW/CredReadW）加密存储。

3. **Prompt 结构**（已确认，含一处重要修正）：system prompt 固定（你是短剧广告投放剪辑师...，见 06 文档广告钩子标准），user message 动态填充台词列表（带句子 ID + 起止毫秒时间戳）。**DeepSeek 官方 API 的 `response_format` 只支持 `text`/`json_object` 两种取值，不支持 OpenAI 的 `json_schema` 严格模式**——正确做法是 `response_format: {type: "json_object"}` + 在 system/user prompt 里显式写出目标 JSON 形状 `{narrative_structure, start_id, end_id, reason, candidate_sentences}`，拿到响应后用 `encoding/json` 反序列化到 Go struct，字段类型不对会在反序列化阶段直接报错，相当于本地做二次校验。`candidate_sentences` 字段可选，用于 LLM 在扫描段落时标记多个候选高光区域，由代码做二次排序，以补偿 LLM 单次判定可能漏掉次要候选窗口的风险。

## Blocked by

None — LLM 供应商选型（DeepSeek V4-flash）已确认，可直接开始。
