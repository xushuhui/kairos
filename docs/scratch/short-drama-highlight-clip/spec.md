# 短剧高光片段自动提取 - 技术规格文档

**Status:** ready-for-agent
**Created:** 2026-07-28
**Label:** spec
**Context:** docs/scratch/short-drama-highlight-clip/map.md

## Problem Statement

短剧内容运营需要在单集短剧（1-3 分钟）中快速提取约 1 分钟的"高光片段"，作为广告素材投放到视频平台，给短剧引流更多用户。手动剪辑效率低、主观性强、难以批量。

目前只能靠人工筛选，没有工具能从台词语义层面自动判定哪些片段适合做广告钩子。我们需要一套工具，让剪辑同事能输入单集视频，自动产出适合投放的高光片段，且不需要依赖云端视频处理基础设施——因为高光判定涉及的内容理解最好就近运行以利用本地硬件性能。

## Solution

一个 Windows 原生桌面软件（Go，Fyne 原生 GUI，不内嵌浏览器引擎），用户选取或拖入本机短剧视频文件后，全自动走通以下链路：

1. **FFmpeg 提取音轨** — 转为 16kHz 单声道 WAV
2. **本地 ASR（FunASR Paraformer-large，ONNX GPU）** — 转写台词，输出带字级毫秒时间戳的分句文本
3. **云端 LLM（DeepSeek V4-flash）** — 语义理解台词，判定冲突/反转/情绪高点最集中的连续窗口，返回起始/结束句子 ID
4. **代码查表映射时间戳** — 将句子 ID 映射回毫秒级时间戳，锚定高潮在片段结尾，双 clamp 处理边界
5. **FFmpeg 硬件加速截取** — 用检测到的 GPU 编码器（NVENC/AMF/QSV/软编码保底）输出高光片段到用户指定本机路径

全流程特点：ASR 完全本地化（仅 LLM 判定需网络，但只传纯文本不带视频），GPU 跨厂商（不锁定 NVIDIA），输入输出均在本机磁盘、不接入任何云存储。

## User Stories

### 核心链路

1. As a 短剧内容运营, I want to 点击"选择视频"按钮打开系统文件对话框选取本地视频文件，也可以直接拖拽文件进窗口, so that 我可以开始剪辑流程。
2. As a 短剧内容运营, I want to 选定输出目录后处理自动开始（不需要额外点击"开始"按钮）, so that 操作步骤最少。
3. As a 短剧内容运营, I want to 看到处理进度（音轨提取中→识别台词中→判定高光中→剪辑中）, so that 我知道软件没卡死。
4. As a 短剧内容运营, I want to 处理完成后自动打开预览窗口播放高光片段, so that 我可以快速确认效果。
5. As a 短剧内容运营, I want to 选完输入文件后立即看到输出目录选择项（默认预填源文件同目录，可点击"更改"打开系统文件夹对话框自定义位置）, so that 我知道产物在哪里、也能按需指定其他位置。
6. As a 短剧内容运营, I want to 支持常见视频格式（MP4/MOV/AVI/WEBM）, so that 我不需要预先转码。
7. As a 短剧内容运营, I want to 软件自动使用我的独立显卡做加速（不挑显卡品牌）, so that 处理速度比纯软件编码快几倍。

### 高光判定

8. As a 短剧内容运营, I want to 软件自动识别台词中的冲突/反转/情绪高点，精确定位到句子边界, so that 高光判定不依赖我的主观经验。
9. As a 短剧内容运营, I want to 高光约 60 秒左右（45-75 秒区间都可接受）, so that 片段长度适合社媒发布。
10. As a 短剧内容运营, I want to 如果整段视频短于 60 秒，直接使用整段不做截取, so that 不会出现"比原视频还短"的怪异输出。
11. As a 短剧内容运营, I want to 如果精彩高潮靠近视频开头（不足 60 秒），自动从视频起点开始并尽可能向后延伸, so that 不会因为边界限制导致片段丢帧。
12. As a 短剧内容运营, I want to 高光片段以情绪高潮点结尾，形成悬念式收尾, so that 符合短剧分发节奏。
13. As a 短剧内容运营, I want to 能在设置中看到 LLM 判定理由（为什么这段是"最精彩"）, so that 我可以信任或调优自动化判定。

### 错误与边界

14. As a 短剧内容运营, I want to 如果视频文件在开始处理后被外部删除/移动，软件优雅报错而不是崩溃, so that 我收到明确的错误而非诡异的程序闪退。
15. As a 短剧内容运营, I want to 如果磁盘空间不足，处理前就检测并报错, so that 我不等到中途才失败。
16. As a 短剧内容运营, I want to 如果用户显卡不支持硬件加速，自动降级到 CPU 处理并提示性能影响, so that 老机器也能用（只是慢一点）。
17. As a 短剧内容运营, I want to 如果 LLM 调用超时或网络不通，显示明确的网络错误提示, so that 我知道去检查 API Key 或网络。
18. As a 短剧内容运营, I want to 如果 DeepSeek API Key 未配置或无效，在首次运行时引导输入, so that 不会卡在"空转"状态。

### 历史与二次使用

19. As a 短剧内容运营, I want to 查看之前处理过的历史记录（视频名、处理时间、片段时长）, so that 我可以复盘之前剪过什么。
20. As a 短剧内容运营, I want to 在历史记录中查看某次剪辑的完整 ASR 转写文本, so that 我可以回看台词有没有识别错误。
21. As a 短剧内容运营, I want to 在历史记录中查看某次剪辑的 LLM 判定理由, so that 我可以理解算法决策。
22. As a 短剧内容运营, I want to 对已处理过的源视频重新运行剪辑（使用不同的参数）, so that 我不需要重新选取文件。
23. As a 短剧内容运营, I want to 删除不再需要的历史记录, so that 保持历史列表干净。
24. As a 短剧内容运营, I want to 历史记录在软件重启后仍然保留, so that 处理过的视频记录不丢失。

### 配置与首次运行

25. As a 短剧内容运营, I want to 首次运行时软件引导我输入 DeepSeek API Key, so that 不用去翻配置文件格式。
26. As a 短剧内容运营, I want to 可选择将 API Key 存入 Windows 凭据管理器加密存储（代替默认的明文配置文件）, so that 更安全地保存密钥。
27. As a 短剧内容运营, I want to 安装完成后软件直接可用（模型文件、FFmpeg 均已内置）, so that 我不需要额外下载任何东西。
28. As a 短剧内容运营, I want to 软件检测我的显卡驱动是否满足 GPU 加速要求，不满足时给出安装指引, so that 我知道缺少什么依赖。

## Implementation Decisions

### 架构总览

桌面软件使用 Go 作为唯一开发语言（已确认，2026-07-30；备选并认真比较过 **Rust** 和 **C#**）——团队实际只熟悉 Go，Rust/C# 均为团队零基础，"团队能否长期维护这份代码"是比技术指标更根本的要求：Rust 的零成本抽象/内存安全优势在这个 I/O 驱动、重活委托给 FFmpeg/ONNX 原生库的场景里基本兑现不了，C# 虽然原生 GUI 更成熟，但团队同样没有基础。UI 框架选定 **Fyne**（Go 生态最成熟、维护最活跃的 GUI 库；`Walk`——曾经的真原生 Win32 包装方案——已停止维护，排除）。不内嵌浏览器引擎，通过 `os/exec` 启动 FFmpeg 子进程。

### Pipeline 编排

```go
func RunHighlightExtraction(videoPath string, config Config) (HighlightOutput, error)
```

内部依赖两个 interface，生产和测试环境通过不同的实现注入（Go interface 是结构化类型，不需要显式 `impl` 声明）：

- `Transcriber` interface — `Transcribe(audioPath string) ([]Sentence, error)`。生产实现封装 FunASR Paraformer-large ONNX 推理；测试实现返回固定的句子列表。
- `HighlightJudge` interface — `Judge(sentences []Sentence) (HighlightWindow, error)`。生产实现调用 DeepSeek V4-flash OpenAI 兼容接口；测试实现返回固定的 `{start_id, end_id, reason}`。

编排自身覆盖的职责：验证源文件存在、调用 ffmpeg 提取音轨、调用 Transcriber、格式化句子列表送给 HighlightJudge、拿 LLM 输出查表格句子 ID 映射为毫秒时间戳、调用 `ComputeWindow()` 做边界 clamp、调用 ffmpeg 截取视频、清理临时文件。

### 窗口算法（已原型验证）

```go
func ComputeWindow(peakEndMs, targetLenMs, videoLenMs uint64) (startMs, endMs uint64)
```

- 默认以高潮窗口结尾锚定片段末尾，向回倒推目标时长
- peak_end_ms 来自 LLM 输出 `end_id` 在 ASR 句子表中对应的 `EndMs`
- 当 start < 0 时 clamp 到 0，end 取 min(target_len_ms, video_len_ms)——接受此边界下"高潮不在末尾"的妥协
- 当 video_len_ms ≤ target_len_ms 时直接返回 (0, video_len_ms)
- 目标时长约 60 秒，45-75 秒均视为合格

### LLM Prompt 结构

- **System prompt**：固定文本"你是短剧广告投放剪辑师，从带编号的台词列表中先识别开端/发展/高潮/结局的叙事结构作为背景参考，再挑出一段最适合做广告投放钩子的连续片段——目标不是'剧情上最重要的一段'，而是'从未看过这部剧的路人观众刷到后会忍不住点进去看正片'的片段，目标时长约 60 秒"
- **User message**：动态拼接每句 `id, start_ms, end_ms, text`，带列标题；一次性输入（远低于 DeepSeek 1M token 上下文限制）
- **Response format**：`response_format: {type: "json_object"}`。**注意**：DeepSeek 官方 API 只支持 `response_format.type` 为 `text` 或 `json_object`，不支持 OpenAI 的 `json_schema` 严格模式——schema 约束通过在 prompt 里显式写出目标 JSON 形状达成，不是靠 API 参数强制。目标形状：
  ```json
  { "narrative_structure": { "setup": [id,id], "rising_action": [id,id], "climax": [id,id], "resolution": [id,id] },
    "start_id": integer, "end_id": integer, "reason": string,
    "candidate_sentences": [{ "start_id": integer, "end_id": integer, "label": string }] }
  ```
  `narrative_structure` 不参与后续代码逻辑，只用于让模型先建立全剧故事上下文再挑窗口，兼作 prompt 调试用的中间产物（详见 06 文档）。拿到响应后用 `encoding/json` 反序列化到 Go struct 做本地校验，字段类型/必填项不对会在反序列化阶段直接报错。
- LLM 不直接输出时间戳，只输出句子 ID，由代码查 ASR 表映射（消除 LLM 时间戳幻觉风险）
- `candidate_sentences` 为可选字段，供代码做二次排序补偿单次判定可能的遗漏
- prompt 内嵌输出前自检清单（路人观众能否看懂冲击点、是否制造悬念/信息缺口而非讲透冲突、结尾是否卡在高点不给答案、时长是否落在 45-75 秒），判定标准是广告钩子/引流转化而非单纯叙事重要性，参考真实生产项目 NarratoAI 的短剧混剪 prompt 设计改写（详见 06 文档）
- Few-shot 样例暂不加入 prompt，先试零 shot，效果不够再加

### 本地 ASR 实现

- **模型**：FunASR Paraformer-large（阿里达摩院）
- **运行时**：ONNX Runtime CUDA execution provider（团队实测显卡 NVIDIA T1000，NVIDIA-only，不做跨厂商）——`sherpa-onnx-go` 官方 Go 绑定本身也只支持 `cpu`/`cuda`/`coreml`，不支持 DirectML，这条限制跟硬件确认结果一致，不构成问题
- **依赖**：Paraformer ONNX 模型文件 + Silero VAD ONNX 模型文件（原方案 FSMN-VAD，2026-07-31 实现阶段修正——`k2-fsa/sherpa-onnx-go-windows` 官方绑定的 `VadModelConfig` 只支持 `SileroVad`/`TenVad`，无 FSMN 选项，见 issues/03-kairos-asr-paraformer.md）+ 标点恢复模型
- **输出**：带句级和字级毫秒时间戳的句子列表 `[{id, start_ms, end_ms, text, words: [{char, start_ms, end_ms}]}]`
- **许可证**：代码 Apache-2.0；模型权重 FunASR Model License（商用需标注来源 Alibaba/FunAudioLLM）
- **部署**：模型文件打包在安装包内，首次运行不下载

### 云端 LLM 接入

- **供应商**：DeepSeek V4-flash
- **接入方式**：Go 通过 `github.com/sashabaranov/go-openai` 调用 OpenAI 兼容接口（`BaseURL` 指向 `https://api.deepseek.com`，`model` 设为 `deepseek-v4-flash`）；`fm-kafka`（同源 Go 项目）已用同一个库对接过 LLM，有先例可循
- **API Key 管理**：默认存本地 JSON 配置文件（`%APPDATA%`，标准库 `encoding/json`），可选 Windows 凭据管理器加密存储
- **成本**：DeepSeek V4-flash 输入 $0.14/M token、输出 $0.28/M token，短剧单集调用几乎可忽略

### GPU 硬件加速策略

- **解码**：FFmpeg 走 `-hwaccel cuda`（NVIDIA-only，团队实测显卡为 T1000）
- **编码**：`h264_nvenc`，不可用时退化到 `libx264` 软编码
- **ASR 推理**：ONNX Runtime CUDA execution provider
- **显存检测**：Paraformer 约 220M 参数，显存占用预计 < 1 GB，T1000 的 4GB/8GB 显存无资源争夺风险
- **运行时检测**：首次启动检测 CUDA 运行时可用性，不满足时降级到 CPU 推理

### FFmpeg 依赖

- 安装包内置官方 Windows pre-built 二进制
- 核心功能不依赖用户环境安装 FFmpeg
- 调用方式：`os/exec` 启动子进程

### 本地存储

- **历史记录**（已确认为 JSON 旁路文件，不用 SQLite——数据量小，扫目录足够，省一个 cgo 依赖）：每次处理完在 `%APPDATA%/kairos/history/` 写一个 `{时间戳}_{源文件名}.json`，字段包含 `source_path, source_name, highlight_path, highlight_start_ms, highlight_end_ms, asr_raw_result, llm_raw_response, status, error_message, created_at`
- **临时文件**：`os.MkdirTemp()`，`defer os.RemoveAll(tmpDir)` 保证失败路径也清理（Go 没有 RAII/`Drop`，`defer` 是等价惯用法）
- **输出策略**：用户未指定时默认输出到源文件同目录下 `{source}_highlight.mp4`
- **配置文件**：DeepSeek API Key 等设置存 JSON 文件到标准配置目录

### 打包与分发

- **打包工具**：`go-msi` 出 MSI 安装包
- **安装包内容**：Go 编译的单文件二进制、FFmpeg 可执行文件、Paraformer ONNX 模型文件、Silero VAD 模型文件、CUDA/cuDNN 运行时 DLL
- **模型量化选项**：可考虑 q8 量化 GGUF/ONNX 版本（约原始模型一半体积，精度损失极小），但"内置"决定不变
- **GPU 驱动检测**：安装后首次启动检测 CUDA 运行时可用性，不满足时降级 CPU 推理并提示用户

## Testing Decisions

### 测试哲学

只测试外部行为，不测试实现细节。对于桌面软件来说，外部行为就是：**给定一个输入视频文件和配置，产出正确的高光片段或正确的错误提示**。

### 测试缝合点（1 个入口点 + 2 个注入接口）

`RunHighlightExtraction()` 是唯一端到端测试入口，通过 `Transcriber` 和 `HighlightJudge` 两个 interface 的 mock 实现将其余依赖隔离：

```
[Test: run_highlight_extraction()]
  ├─ 注入 MockTranscriber（返回固定句子列表）
  ├─ 注入 MockJudge（返回固定 start_id/end_id）
  ├─ 使用真实 FFmpeg 子进程提取音轨 + 截取（唯一不 mock 的真实 I/O）
  └─ 验证：输出文件存在 / 时长接近 targetLen / 临时文件被清理 / 历史记录 JSON 文件已写入
```

为什么 FFmpeg 子进程不 mock：它已经是编排层的真实 I/O 边界，mock 后测试只能测到"发命令"而测不到"产出了可播放的视频"。短剧源文件可以预置一个 5 秒的测试 MP4（静音 + 固定色块），放进 CI。

### `ComputeWindow()` 纯函数测试

无 I/O、零 mock、直接单元测试：

| 测试场景 | peak_end_ms | target_len_ms | video_len_ms | 期望 (start, end) |
|---|---|---|---|---|
| 正常 | 75,000 | 60,000 | 120,000 | (15,000, 75,000) |
| 高潮在开头附近 | 30,000 | 60,000 | 120,000 | (0, 60,000) |
| 高潮在开头附近（短视频） | 30,000 | 60,000 | 50,000 | (0, 50,000) |
| 视频短于目标时长 | 20,000 | 60,000 | 45,000 | (0, 45,000) |
| 精确边界：start == 0 | 60,000 | 60,000 | 120,000 | (0, 60,000) |
| 精确边界：start == 0（视频等于目标时长） | 60,000 | 60,000 | 60,000 | (0, 60,000) |

这项测试覆盖所有边界条件，是成本最低、价值最高的测试点。

### 真实适配器集成测试

- `Transcriber` 真实实现：用预置的 5 秒测试音频 WAV 文件运行一次 ONNX 推理，验证输出句子结构正确（有 id、时间戳、文本字段）
- `HighlightJudge` 真实实现：用预置的短台词文本调用一次真实 DeepSeek API，验证返回 JSON 结构合法（start_id ≤ end_id，都有 reason 字段）
- 这两类集成测试**不进常规 CI**，标记为 manual-run，因为需要 GPU+模型文件（Transcriber）和有效 API Key + 网络（Judge）

### 已有项目参考

无——`kairos` 是新项目，没有前例。但 Go 生态 `go test` 是标准做法，测试模式上用表驱动测试（`ComputeWindow` 的表驱动用例是 Go 测试的惯用写法，不需要额外的第三方参数化测试库），mock 策略用手写 fake struct 实现 `Transcriber`/`HighlightJudge` interface（Go interface 天然可 mock，通常不需要 `testify/mock` 这类框架，除非 mock 逻辑变得复杂）。

## Out of Scope

- 云端部署/CI-CD/HTTP API —— 桌面软件不是后端服务，整套服务器端命题不适用
- 自动发布高光片段到抖音/小红书等平台 —— 只产出片段文件到本机磁盘
- 人工审核环节 —— 全自动直接交付
- 多集合集/整部剧的高光挖掘 —— 只处理单集视频文件（1-3 分钟）输入
- 纯音频/画面信号的高光算法 —— 选定台词语义路线为主，此项留待主线跑通后再评估是否叠加辅助信号
- 多语言支持（如英语/日语短剧）—— 本轮中文短剧专用
- 批量处理/队列 —— 单机单文件串行操作

## Further Notes

- 本 spec 对应的领域决策记录详见 `docs/scratch/short-drama-highlight-clip/map.md`，具体选型调研详见该目录下的 01-06 号文档
- `01-research-findings.md` 中的 ASR 调研结论（腾讯云 ASR）已被本地 FunASR Paraformer-large 替换；LLM 调研结论（DeepSeek V4-flash）保留有效
- ASR 模型许可证为 FunASR Model License，要求在下游产品中标注来源（Alibaba/FunAudioLLM），需在软件关于页面或 README 中体现
- DeepSeek V4-flash 命名/定价已发生过变更（V3.2→V4），需持续关注官方定价页与模型下线公告
