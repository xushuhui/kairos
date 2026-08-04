# Kairos

一个 Windows 桌面工具：把一集短剧（1-3 分钟 MP4/MOV/AVI/WEBM）自动剪辑成一段约 60 秒的广告钩子高光片段——本地 ASR 转写台词，云端 LLM 判定最适合做广告投放的连续窗口，FFmpeg 截取输出。全流程自动，不需要人工剪辑，不上传视频到云端。

English version: [README.md](./README.md).

## 处理链路

```
源视频 ──▶ FFmpeg 提取 16kHz 单声道 WAV
              │
              ▼
     本地 ASR（FunASR Paraformer-large，ONNX + CUDA）
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

目标平台是 Windows 10（1903+）/ Windows 11 + NVIDIA 显卡。这个仓库目前在 macOS 上开发/测试，所以任何锁定 CUDA/Windows-only 绑定的部分都已实现但**未经验证**——下面明确标注出来，不含糊带过。

| # | 包 | 做什么 | 状态 |
|---|---|---|---|
| 01 | `internal/core` | 领域类型、`Transcriber`/`HighlightJudge` interface、`ComputeWindow()` | ✅ 完成，已测试 |
| 02 | `internal/video` | FFmpeg 子进程封装：音轨提取、CUDA 检测、剪辑、时长探测 | ✅ 完成，已测试 |
| 03 | `internal/asr` | 本地 Paraformer-large + Silero VAD 转写，走 `sherpa-onnx-go-windows` | ⚠️ 字级转句级的合并逻辑跨平台且已测试；真正的 sherpa-onnx 集成锁在 `//go:build windows`，**这台机器上无法验证**（没有 Windows 主机、没有 CUDA、没有模型文件） |
| 04 | `internal/llm` | DeepSeek V4-flash 客户端，实现 `HighlightJudge` | ✅ 完成，已用假 HTTP server 测试；真实调用需要一个真实的 `DEEPSEEK_API_KEY`，尚未验证 |
| 05 | `internal/core` | `RunHighlightExtraction()`——编排入口 | ✅ 完成，用手写 fake 实现两个注入接口测试通过 |
| 06 | — | 真实素材端到端验证 | ⬜ 未开始——卡在需要 Windows 主机、NVIDIA 显卡、真实 ASR 模型文件、DeepSeek API Key |
| 07 | `internal/history` | JSON 旁路历史记录 | ✅ 完成，已测试 |
| 08 | `cmd/kairos` | Fyne GUI | ⬜ 未开始——阻塞于 06 |
| 09 | — | MSI 打包 | ⬜ 未开始——阻塞于 08 |

完整的 ticket 定义和设计决策在 [`docs/scratch/short-drama-highlight-clip/`](./docs/scratch/short-drama-highlight-clip/) 下——`spec.md` 是产品决策，`implementation-plan.md` 是工程设计，`map.md` 是决策记录。

## 依赖要求

- Go 1.26+
- `ffmpeg`/`ffprobe` 在 `PATH` 上（仅开发/测试需要——正式安装包会内置）
- 要在 Windows 上真正跑起来：一张支持 CUDA 的 NVIDIA 显卡（不可用时退化到 CPU/`libx264`），以及 Paraformer-large + Silero VAD 模型文件
- 一个 DeepSeek API Key（`internal/llm` 需要）

## 构建与测试

```sh
go build ./...
go test ./...
```

大多数测试会真的起 FFmpeg 子进程，对一个自动生成的小夹具（5 秒静音+纯色 MP4）跑，而不是 mock FFmpeg——原因见 `spec.md` 的 Testing Decisions 一节。`ffmpeg`/`ffprobe` 不在 `PATH` 上时相关测试会优雅跳过。

## 项目结构

```
kairos/
├── cmd/kairos/          二进制入口（Fyne GUI，尚未实现）
├── internal/
│   ├── core/            编排、领域类型、ComputeWindow()、两个测试缝合点
│   ├── video/            FFmpeg 子进程封装
│   ├── asr/              Paraformer 转写（真实后端 Windows-only）
│   ├── llm/               DeepSeek 客户端
│   ├── history/           JSON 旁路历史记录
│   └── testutil/          FFmpeg 测试夹具共用工具
└── docs/scratch/short-drama-highlight-clip/   完整规格与设计文档
```

## 配置

API Key 和输出设置存在本地 JSON 文件里（`os.UserConfigDir()/kairos/config.json`，Windows 上对应 `%APPDATA%\kairos\config.json`）：

```json
{
  "deepseek": {
    "api_key": "",
    "use_credential_manager": false
  },
  "output": {
    "default_dir": ""
  }
}
```

## 第三方许可证与标注

- ASR：FunASR Paraformer-large 模型权重——**FunASR Model License**，商用需标注来源 Alibaba / FunAudioLLM。这份说明暂时满足这条要求，正式的应用内"关于"页面见 08 号 ticket。
- `github.com/sashabaranov/go-openai`、`github.com/k2-fsa/sherpa-onnx-go-windows`——许可证条款见各自仓库。

本仓库自身的开源许可证尚未选定。
