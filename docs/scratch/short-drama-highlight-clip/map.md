# 短剧高光片段自动剪辑 - 技术方案地图

**Status:** ready-for-agent
**Created:** 2026-07-23
**Label:** wayfinder:map

## Destination

产出一份端到端技术方案 Spec：设计一个 Windows 原生桌面软件（Go 技术栈，Fyne 原生 GUI，不内嵌浏览器引擎，目标系统 Windows 10 1903+ / Windows 11），供剪辑同事在自己电脑上安装使用——打开软件、选取或拖入本地短剧视频文件（MP4，1-3 分钟），通过本地 ASR（FunASR Paraformer-large，NVIDIA CUDA 硬件加速转写台词）+ 云端 LLM（DeepSeek V4-flash 判定最适合做广告投放钩子的连续片段），自动截取剪辑产出约 1 分钟高光片段，输出到用户指定本机路径，供同事人工复核后手动上传到视频平台做广告素材、给短剧引流。全流程单机单用户，不依赖服务端基础设施，不写入云存储，软件本身不含发布/审核环节。方案写完交给后续开发实现，本地图本身不产出可运行代码。

## Notes

- 本地 ASR 选用阿里达摩院 FunASR Paraformer-large（ONNX 运行时，GPU 加速），许可证 Apache-2.0（代码）+ FunASR Model License（模型权重，商用需标注来源 Alibaba/FunAudioLLM）。详见 [01-asr-llm-vendor-research.md](./01-asr-llm-vendor-research.md) 的 ASR 本地化补充说明。
- 云端 LLM 继续使用 DeepSeek V4-flash（OpenAI 兼容接口，Go 侧用 `sashabaranov/go-openai` 接入，已确认——fm-kafka 已有同库先例）。
- 涉及决策优先用 `/grilling` + `/domain-modeling`；涉及供应商/技术选型调研的用 research 类型 ticket。
- 本地图未声明 carry execution —— 只产出决策与规格文档，不写实现代码。

## Decisions so far

- [目的地确认（Windows 原生桌面软件 → Go + Fyne → 本地 ASR（FunASR Paraformer-large）+ 云端 LLM（DeepSeek V4-flash）→ FFmpeg 硬件加速剪辑 → 仅存本机磁盘）](./map.md) — 见上方 Destination 与 Notes。
- [ASR 本地化选定 + LLM 云端保留确认](./map.md) — ASR 改用阿里达摩院 FunASR Paraformer-large（字级时间戳原生支持，非自回归架构 GPU 推理 119.6× 实时，中文 CER 10.18%，商用需标注来源）；LLM 保留 DeepSeek V4-flash 云端方案。详见 [01-asr-llm-vendor-research.md](./01-asr-llm-vendor-research.md) 补充说明。
- **开发语言：Go，不是 Rust（2026-07-30 确认）**——团队实际只熟悉 Go，Rust/C# 均为团队零基础；"团队能否长期维护这份代码"是比技术指标更根本的要求，Rust 的性能/安全优势在这个 I/O 驱动、重活委托给 FFmpeg/ONNX 原生库的场景里基本兑现不了。GUI 框架同步确认为 **Fyne**（Go 生态最成熟的 GUI 库；`Walk` 真原生方案已停止维护，排除）。详见 [implementation-plan.md](./implementation-plan.md)。
- **GPU 硬件加速策略：NVIDIA-only，不做跨厂商（2026-07-30 修正）**——团队实测显卡为 NVIDIA T1000（Turing 架构，CUDA compute capability 7.5，支持 NVENC H.264/HEVC 硬编）。原定的"跨厂商 DirectML/NVENC/AMF/QSV"设计是为未知硬件做的保险，硬件确认后这层复杂度不再需要——且 Go 版 `sherpa-onnx-go` 绑定本身也只支持 CUDA，不支持 DirectML，两个理由都指向同一个简化方向。FFmpeg 走 `-hwaccel cuda` 解码 + `h264_nvenc` 编码，`libx264` 仅作驱动异常时的安全网。详见 [03](./03-source-video-fetch-clip-cleanup.md)、[05](./05-deployment-cicd.md)、[implementation-plan.md](./implementation-plan.md)。
- **DeepSeek API Key 存储方案（已确认，JSON 不用 TOML）**——默认写本地配置文件（JSON，`%APPDATA%`，标准库 `encoding/json`），可选勾选启用 Windows 凭据管理器加密存储。详见 [02-http-api-contract.md](./02-http-api-contract.md)。
- **ASR 模型文件部署方式（已确认）**——安装包内置，不走首次运行下载。详见 [05-deployment-cicd.md](./05-deployment-cicd.md)。
- **项目定名 Kairos（2026-07-30 确认）**——希腊神话"时机之神"，专司稍纵即逝的关键瞬间（区别于代表线性时间的 Chronos），对应本项目"从长视频中精准捕捉那个决定广告钩子成败的高光瞬间"这一核心产品动作。Go module、`cmd/` 二进制目录、`%APPDATA%` 配置/历史/日志路径均已从 `fm-cut` 改为 `kairos`。

## Not yet specified

- 次要高光信号（音频能量峰值、镜头切换、运动幅度）是否以及如何作为辅助信号叠加到台词语义判定上——本轮方案主线是台词语义，此项留待主线跑通后再评估。

## Out of scope

- 云端部署/CI-CD/HTTP API 相关设计——桌面软件不是后端服务，整套服务器端命题不适用。
- 自动发布/分发高光片段到抖音/小红书等平台——用户只要求"产出"片段文件到本机磁盘，不含发布环节。
- 人工审核环节——本轮明确先做全自动直接交付版本。
- 多集合集/整部剧的高光挖掘——本轮只处理单集短剧视频文件（1-3 分钟）输入。
- 纯音频/画面低层信号的高光算法（音频能量峰值、镜头切换、运动幅度）——这类信号跟"精彩"语义弱相关，本轮选定台词语义路线（本地 ASR FunASR Paraformer + 云端 DeepSeek V4-flash LLM）为方案主线，不纳入。
- 视觉理解大模型（识别沉默反应镜头、视觉揭示、屏幕文字等纯台词覆盖不到的高潮时刻）——跟上一条低层信号不同，这是真实存在的结构性盲区（台词稀疏/零台词的画面高潮，ASR+LLM 流水线完全看不见），但现有 LLM（DeepSeek V4-flash）不支持视觉输入，引入意味着重新做供应商选型 + 显著更高的多模态调用成本，缺乏证据前不预先构建。**触发条件（不是"以后再评估"这种模糊说法）**：implementation-plan.md 第 6 步"真实素材端到端验证"时，如果 LLM 从台词判定出的窗口明显漏掉了一个更精彩但台词稀疏/沉默的画面时刻，即为引入信号。
- macOS/Linux 支持——已确认不支持，只做 Windows（10 1903+ / 11）。Go + Fyne 本身具备跨平台能力，但已经确认的核心硬件加速链路是 Windows 专属技术（FFmpeg `-hwaccel cuda`、Windows 凭据管理器、`go-msi` MSI 打包）——要跨平台意味着给每个系统单独维护一套硬件加速路径（macOS 对应 VideoToolbox，Linux 对应 VAAPI/VDPAU），维护成本超出个人工具的合理范围，不做。
