# Kairos

一个 Windows 桌面工具：把一集短剧（1-3 分钟 MP4/MOV/AVI/WEBM）自动剪辑成一段约 60 秒的广告钩子高光片段——本地 ASR 转写台词，云端 LLM 判定最适合做广告投放的连续窗口，FFmpeg 截取输出。全流程自动，不需要人工剪辑，不上传视频到云端。

English version: [README.md](./README.md).

## 处理链路

```
源视频 ──▶ FFmpeg 提取 16kHz 单声道 WAV
              │
              ▼
     本地 ASR（FunASR Paraformer-large，ONNX，CPU）
              │  带句级毫秒时间戳的分句文本
              ▼
     云端 LLM（DeepSeek V4-flash）—— 判定最适合做广告钩子的窗口，
     返回起始/结束句子 ID + 判定理由
              │
              ▼
     代码把句子 ID 查表映射为毫秒时间戳，锚定高潮在片段结尾，
     做边界 clamp 处理
              │
              ▼
     FFmpeg 硬件加速截取（NVENC，不可用时退化 libx264）
              │
              ▼
     ~60 秒高光片段落盘
```

全流程单机单用户，不依赖服务端基础设施。唯一的网络调用是 LLM 判定请求，只传纯文本台词，不传视频本身。

## 当前状态

目标平台（正式部署）是 Windows 10（1903+）/ Windows 11——这个决定没有变（见 `map.md` 的"Out of scope"）。`internal/asr` 另外镜像了一份 macOS（darwin）后端，纯粹是**本地开发/测试的便利**，用户明确要求：先在这台机器上把真实流程跑通，再装到 Windows 目标机器，而不是走"改一行 Go 代码 → 找人在 Windows 上重新编译 → 贴日志回来"这种慢循环。Windows 和 macOS 两个后端是同一份 Go 逻辑，分别对接两个平台专属的 sherpa-onnx 绑定——已经逐字段 diff 过确认 API 表面完全一致（`sherpa-onnx-go-windows@v1.13.4` vs `sherpa-onnx-go-macos@v1.13.5`）。在 macOS 上验证过能给 Windows 那边很高的信心，但不能替代一次真实的 Windows 实跑。

| # | 包 | 做什么 | 状态 |
|---|---|---|---|
| 01 | `internal/core` | 领域类型、`Transcriber`/`HighlightJudge` interface、`ComputeWindow()` | ✅ 完成，已测试 |
| 02 | `internal/video` | FFmpeg 子进程封装：音轨提取、CUDA 检测、剪辑、时长探测 | ✅ 完成，已测试 |
| 03 | `internal/asr` | 本地 Paraformer-large + Silero VAD 转写，走 `sherpa-onnx-go-windows`/`-macos`（固定用 CPU provider） | ✅ 已在 macOS 上端到端验证（真实模型文件，`scripts/download-models.sh` 下载；真实 TTS 生成的中文语音音频）——VAD 切分、Paraformer 解码、标点恢复全部产出正确结果。Windows 版是对接平台匹配绑定的同一份 Go 逻辑，没有在真实 Windows 硬件上独立跑过 |
| 04 | `internal/llm` | DeepSeek V4-flash 客户端，实现 `HighlightJudge` | ✅ 完成，已用假 HTTP server 测试；真实调用需要一个真实的 `DEEPSEEK_API_KEY`，尚未验证 |
| 05 | `internal/core` | `RunHighlightExtraction()`——编排入口 | ✅ 完成，用手写 fake 实现两个注入接口测试通过 |
| 06 | — | 真实素材端到端验证 | ⚠️ ASR 本身已验证（见 03）；完整的 GUI → DeepSeek → 剪辑链路还需要一次真实 Windows 实跑 + 真实 API Key |
| 07 | `internal/history` | JSON 旁路历史记录 | ✅ 完成，已测试 |
| 08 | `cmd/kairos` | Fyne GUI | ✅ 已实现，Windows 和 macOS 均能编译；用 Fyne 测试驱动配合 fake `Transcriber`/`HighlightJudge` 做过无头测试，还没在真实硬件上以真实窗口跑过 |
| 09 | — | MSI 打包 | ⬜ 未开始——仅有脚手架，需要真实 Windows + WiX Toolset 主机 |

完整的 ticket 定义和设计决策在 [`docs/scratch/short-drama-highlight-clip/`](./docs/scratch/short-drama-highlight-clip/) 下——`spec.md` 是产品决策，`implementation-plan.md` 是工程设计，`map.md` 是决策记录。

## 依赖要求

- Go 1.26+
- Windows 上：需要一个支持 cgo 的 C 编译器（MinGW-w64 gcc），用来编译 `fyne.io/fyne/v2`（`go-gl/gl`）和 `github.com/k2-fsa/sherpa-onnx-go-windows`。一次性设置：先 `Set-ExecutionPolicy -Scope CurrentUser -ExecutionPolicy RemoteSigned`（允许本地脚本运行），再跑 `.\scripts\setup-cgo-toolchain.ps1`。
- macOS 上（仅开发/测试，见上方"当前状态"）：Xcode Command Line Tools 提供 cgo 支持（`xcode-select --install`），一般已经装好，不需要额外的工具链步骤——`sherpa-onnx-go-macos` 的 cgo 链接用的是编译期就写死的 rpath，不像 Windows 那样需要额外拷 DLL 这一步。
- `ffmpeg`/`ffprobe` 在 `PATH` 上（仅开发/测试需要——正式 Windows 安装包会内置）
- 要在 Windows 上真正跑起来：一张支持 CUDA 的 NVIDIA 显卡，给 FFmpeg 的 `-hwaccel cuda`/`h264_nvenc` 用（不可用时退化到 CPU/`libx264`——`internal/asr` 本身在两个平台上都固定用 CPU，见"当前状态"），以及 Paraformer-large + Silero VAD 模型文件
- 一个 DeepSeek API Key（`internal/llm` 需要）

## 构建与测试

```sh
go build ./...
go test ./...
```

大多数测试会真的起 FFmpeg 子进程，对一个自动生成的小夹具（5 秒静音+纯色 MP4）跑，而不是 mock FFmpeg——原因见 `spec.md` 的 Testing Decisions 一节。`ffmpeg`/`ffprobe` 不在 `PATH` 上时相关测试会优雅跳过。

### 真实 ASR 流程测试（macOS 或 Windows）

`internal/asr` 真正对接 sherpa-onnx 那部分需要真实模型文件（约 500MB，不随仓库分发，见 `packaging/README.md`）和真实语音音频，所以用两个环境变量挡住，不在上面的常规测试套件里跑：

```sh
./scripts/download-models.sh          # 下载一次 models/，幂等（Windows 用 scripts/download-models.ps1）
say -v Tingting -o /tmp/speech.aiff "随便一句中文测试语音"   # 仅 macOS，任何真实语音 WAV 都行
ffmpeg -y -i /tmp/speech.aiff -vn -ac 1 -ar 16000 -f wav /tmp/speech.wav

KAIROS_TEST_MODEL_DIR="$(pwd)/models" KAIROS_TEST_AUDIO_PATH=/tmp/speech.wav \
  go test ./internal/asr/... -run TestParaformerTranscriber_RealModels_ManualRun -v
```

## 项目结构

```
kairos/
├── cmd/kairos/          二进制入口（Fyne GUI）
├── internal/
│   ├── core/            编排、领域类型、ComputeWindow()、两个测试缝合点
│   ├── video/            FFmpeg 子进程封装
│   ├── asr/              Paraformer 转写——Windows + macOS（开发/测试）都有真实后端，Go 逻辑按平台镜像
│   ├── llm/               DeepSeek 客户端
│   ├── history/           JSON 旁路历史记录
│   ├── apppath/           解析 config/history/logs/models 统一所在的 exe 同目录
│   └── testutil/          FFmpeg 测试夹具共用工具
└── docs/scratch/short-drama-highlight-clip/   完整规格与设计文档
```

## 配置

Kairos 自包含/可移植：配置、历史记录、日志都放在可执行文件同目录，不写进系统用户目录。以 `kairos.exe` 所在目录为例，布局是：

```
<exe 目录>/
├── kairos.exe
├── config.json      DeepSeek API Key + 输出设置
├── history/         每次处理（成功或失败）一个 JSON 文件
├── logs/
│   ├── app.log        全量日志（所有级别）
│   └── error.log       仅 Error 级别，方便快速肉眼查看
└── models/           内置的 Paraformer/Silero-VAD/标点恢复 ONNX 模型
```

`config.json`：

```json
{
  "deepseek": {
    "api_key": ""
  },
  "output": {
    "default_dir": ""
  }
}
```

## 第三方许可证与标注

- ASR：FunASR Paraformer-large 模型权重——**FunASR Model License**，商用需标注来源 Alibaba / FunAudioLLM。这份说明暂时满足这条要求，正式的应用内"关于"页面见 08 号 ticket。
- `github.com/sashabaranov/go-openai`、`github.com/k2-fsa/sherpa-onnx-go-windows`、`github.com/k2-fsa/sherpa-onnx-go-macos`（仅开发/测试）——许可证条款见各自仓库。

本仓库自身的开源许可证尚未选定。
