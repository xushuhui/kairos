# ASR + LLM 供应商选型调研

**Status:** closed (ASR recommendation superseded by local deployment; LLM conclusion retained)
**Created:** 2026-07-23
**Label:** wayfinder:research
**Parent:** docs/scratch/short-drama-highlight-clip/map.md
**Assignee:** wayfinder-session (resolved via research subagent)

## Question

面向中文短剧台词转写（ASR）与剧情语义理解（LLM，用于判定"最精彩连续片段"）需要哪个技术方案组合？候选至少包括：阶跃云、OpenAI Whisper API、华为云语音、腾讯云语音、阿里云智能语音等 ASR 供应商，以及用于高光判定的 LLM（可与 ASR 同供应商或独立选型）。

需要给出的调研结论：
- 各候选的中文（尤其是短剧强方言/口语化台词）识别准确率、是否原生返回分句时间戳
- 延迟与是否支持异步/批量任务
- 计价方式与单分钟成本量级
- 是否有官方 Go SDK 或稳定 HTTP/REST 接口（技术栈已定为 Go，见地图 Notes）
- 数据安全/合规限制（是否允许短剧类版权内容上传）

## Blocked by

None — can start immediately.

## Resolution

推荐主方案：**腾讯云语音识别（大模型2.0引擎，录音文件识别标准版）+ DeepSeek V4-flash**。理由：腾讯云 ASR 是唯一有第三方检测机构背书准确率数字（97.40%字准率）、官方 Go SDK 完整覆盖所需接口、句+词级时间戳齐全、成本适中（¥0.013/分钟）、音频不留存；DeepSeek V4-flash 中文理解基准最高（C-Eval 92.1/CMMLU 90.4）、成本最低（$0.14/$0.28 每百万token）、官方 OpenAI 兼容可直接用 Go SDK 接入。备选：极致成本优先用阶跃云组合；合规优先用阿里云 DashScope Paraformer-v2 + 通义千问。
两个核心证据缺口需选型定案前做小规模 POC 验证：(1) 六家 ASR 均无短剧口语化+BGM 场景专项准确率数据；(2) 五个 LLM 均无"剧情理解/高光判定"专项评测。详见完整调研报告：[01-research-findings.md](./01-research-findings.md)。


## Supplementary — 2026-07-28

**ASR 结论被覆盖**：后续决策将 ASR 从云端腾讯云切换为本地阿里达摩院 FunASR Paraformer-large（ONNX 运行时，GPU 加速，Apache-2.0 + FunASR Model License），原因是桌面软件定位不再需要走云端 ASR，利用本机 GPU 硬件加速可更低延迟、无调用费、无隐私上传隐患。详见 [01-research-findings.md](./01-research-findings.md) 中 Paraformer 相关基准数据（中文 CER 10.18%，字级时间戳原生支持），以及 map.md Notes 中的选型说明。

**LLM 结论保留**：DeepSeek V4-flash 选型不变，仍然是云端调用（OpenAI 兼容接口），Rust 侧通过 `reqwest` 或 OpenAI Rust SDK 接入。