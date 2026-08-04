# Windows 主机从零跑通操作手册

**Status:** ready-for-agent
**Created:** 2026-08-03
**目的:** 在一台真实 Windows + NVIDIA GPU 主机上，从空环境开始，跑通 ticket 03（ASR 真实推理）、04（DeepSeek 真实调用，已在 macOS 上部分验证）、06（真实素材端到端）、09（MSI 打包）——这四个是当前唯一卡在"没有 Windows+GPU 环境"这个前提上的 ticket，代码本身在 `1473555` 这次提交里已经写完，缺的是硬件验证。

按顺序走，每一步给出可验证的产出，不要跳步——后面的步骤依赖前面的产出。

---

## 0. 硬件与账号前提

- Windows 10（1903 及以上）或 Windows 11
- NVIDIA 显卡（spec 定的参考硬件是 T1000，其余支持 CUDA 的卡原则上也行）+ 已装好的显卡驱动
- 一条真实短剧素材（1-3 分钟，MP4/MOV/AVI/WEBM 均可）
- DeepSeek API Key（已有，`sk-a2d4...`，或重新生成一个）

---

## 1. 装基础工具链

```powershell
# Go（跟仓库 go.mod 声明的版本一致或更新）
winget install GoLang.Go

# Git
winget install Git.Git

# WiX Toolset（ticket 09 打包需要，>= 3.10，go-msi 的硬性依赖）
winget install WiXToolset.WiXToolset

# go-msi（打包工具本身）
go install github.com/mh-cbon/go-msi@latest
```

验证：
```powershell
go version
git --version
candle.exe -?    # WiX 装好后应该能找到
go-msi -h
```

---

## 2. 拉代码，确认能编译

```powershell
git clone git@github.com:xushuhui/kairos.git
cd kairos
go build ./...
```

**预期**：`cmd/kairos` 这次能编译出真正调用 `asr.NewParaformerTranscriber` 的版本（之前在 macOS 上编译时，`transcriber_windows.go` 因为 `//go:build windows` 直接被跳过，编译进去的是 `transcriber_other.go` 的"不支持"桩）。

---

## 3. 拿 FFmpeg（ticket 02 已经在写代码时确认走 PATH 查找）

```powershell
# 官方 pre-built Windows 版本，二选一来源都行：
# - https://www.gyan.dev/ffmpeg/builds/ （推荐 release essentials 或 full 版）
# - https://github.com/BtbN/FFmpeg-Builds/releases

# 解压后把 ffmpeg.exe / ffprobe.exe 所在目录加进 PATH，或者直接放进
# kairos.exe 同一个目录（internal/video 用 exec.Command("ffmpeg",...) 走
# PATH 查找，两种方式都能满足）
```

验证：
```powershell
ffmpeg -version
ffprobe -version
```

跑一遍 `internal/video` 的真实测试，确认这台机器上的 FFmpeg 行为符合预期（这些测试之前只在 macOS 上跑过）：
```powershell
go test ./internal/video/... -v
```

**这一步顺带能验证 `CudaAvailable()`**——在这台真机上应该返回 `true`（macOS 上因为没有 `nvidia-smi` 永远是 `false`），`TestCutClip_NvencRequiresRealHardware` 那条之前一直被跳过的用例，这次应该真正跑起来：
```powershell
go test ./internal/video/... -run TestCudaAvailable -v
go test ./internal/video/... -run TestCutClip -v
```
如果 `TestCutClip_NvencRequiresRealHardware` 还是显示 SKIP，说明这台机器没被正确识别出 CUDA 可用——先查显卡驱动，不要往下走。

---

## 4. 拿 ASR 模型文件（ticket 03 核心缺口）

`internal/asr/paraformer_windows.go` 硬编码了以下相对路径（相对 `modelDir`，`cmd/kairos` 里 `modelDir = "models"`，即跟 `kairos.exe` 同目录下的 `models/` 文件夹）：

```
models/
├── paraformer/
│   ├── model.int8.onnx
│   └── tokens.txt
├── vad/
│   └── silero_vad.onnx
└── punctuation/
    └── model.onnx
```

下载来源（sherpa-onnx 官方模型仓库，已核实的真实 URL，不是猜的）：

```powershell
# Paraformer-large 中文模型——推荐用 2023-09-14 这个版本，不是 03-28 那个：
# 官方文档标注 09-14 版本"支持时间戳"，本项目的 tokensToWords() 逐字依赖
# 模型输出的字级时间戳做句子合并，选错版本可能导致时间戳缺失或全零。
# 【这一点没有在真机上验证过，是根据官方文档描述做的推荐，第一次跑通时
# 请重点检查 Transcribe() 返回的 Sentence.StartMs/EndMs 是否合理】
Invoke-WebRequest -Uri "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/sherpa-onnx-paraformer-zh-2023-09-14.tar.bz2" -OutFile paraformer.tar.bz2
tar xvf paraformer.tar.bz2
# 解压出的目录里找 model.int8.onnx 和 tokens.txt，拷到 models/paraformer/

# Silero VAD：
Invoke-WebRequest -Uri "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/silero_vad.onnx" -OutFile models/vad/silero_vad.onnx

# 标点恢复模型——注意代码要的是 model.onnx（非 int8 版本），
# 对应下面这个非 int8 的压缩包，不是 -int8.tar.bz2 那个：
Invoke-WebRequest -Uri "https://github.com/k2-fsa/sherpa-onnx/releases/download/punctuation-models/sherpa-onnx-punct-ct-transformer-zh-en-vocab272727-2024-04-12.tar.bz2" -OutFile punct.tar.bz2
tar xvf punct.tar.bz2
# 解压出的目录里找 model.onnx，拷到 models/punctuation/
```

**许可证提醒**（spec.md 已经记录过，这里再提醒一次）：Paraformer 模型权重是 FunASR Model License，商用需要在软件里标注来源 Alibaba/FunAudioLLM——目前只在顶层 `README.md`/`README.zh.md` 里写了这条，`cmd/kairos` 还没有一个真正的"关于"页面来落地这个要求（ticket 08 没做这个，算一个后续小任务）。

---

## 5. 跑通 ticket 03：真实 ASR 推理

先准备一个测试音频（`internal/video` 的 `testutil.MakeTestMp4` 生成的是静音测试片段，没有台词，测不出 ASR 效果——用一段有真人说话的短音频/视频）：

```powershell
ffmpeg -i 你的真实素材.mp4 -vn -ac 1 -ar 16000 -f wav test_audio.wav
```

写一个一次性小程序或临时测试直接调用（比照 `internal/asr/merge_test.go` 的风格，但这次不 mock，真的初始化 `ParaformerTranscriber`）：

```go
transcriber, err := asr.NewParaformerTranscriber("models", true) // useCuda=true
// err != nil 就是模型文件路径不对，或者 CUDA provider 初始化失败——
// 看 NewParaformerTranscriber 的实现，CUDA 失败会自动退化 CPU 重试，
// 只有 CPU 也失败才会真的报错，所以这里 err != nil 基本可以排除是
// "只是没用上 CUDA" 这种情况，是真的模型加载失败
sentences, err := transcriber.Transcribe("test_audio.wav")
```

**这一步同时回答了 `packaging/README.md` 里那个悬而未决的问题**——`sherpa-onnx-go-windows@v1.13.4` 绑定的 `onnxruntime.dll` 到底支不支持 CUDA execution provider。加日志/断点确认 `newOfflineRecognizer(modelDir, "cuda")` 到底是成功了还是失败后自动降级到了 CPU——如果是后者，说明这个版本的绑定实际上是 CPU-only，`packaging/wix.json` 里那些 CUDA/cuDNN DLL 对 ASR 这条路径就是摆设（FFmpeg 的 NVENC 硬编码路径不受影响，那是完全独立的 CUDA 集成）。

**验收标准**（对应 ticket 03 checklist）：
- [ ] 模型加载成功（不管走 CUDA 还是降级 CPU）
- [ ] `Transcribe()` 返回结构正确的 `[]core.Sentence`——`ID`/`StartMs`/`EndMs`/`Text` 都有合理值，尤其检查时间戳不是全零
- [ ] 转写文本大致准确（人工核对，不追求完美）

---

## 6. 复核 ticket 04：真实 DeepSeek 调用

代码层面已经在 macOS 上用真实 Key 验证过一轮（见 `1473555` 提交），发现并修了一个 prompt 缺 "json" 字样导致 400 的 bug。当时卡在 DeepSeek 服务端自己不稳定（间歇性 503 "Service is too busy"），没拿到一次完整成功的响应。

在这台 Windows 机器上，网络环境可能不同，值得重跑一次确认：

```powershell
$env:DEEPSEEK_API_KEY = "你的key"
go test ./internal/llm/... -run TestDeepSeekJudge_Judge_RealAPI -v
```

如果还是遇到 503/超时，不是这台机器或代码的问题，是 DeepSeek 那边的服务状态——过一阵子再试，或者联系 DeepSeek 确认账号/配额状态。

---

## 7. 跑通 ticket 06：真实素材端到端验证

这一步不需要写代码，`cmd/kairos` 已经是完整可用的 GUI：

```powershell
$env:DEEPSEEK_API_KEY = "你的key"  # 或者走 GUI 首次运行引导输入，会存进 config.json
go run ./cmd/kairos
```

按 ticket 06 的验收清单人工过一遍：
- [ ] 选一条真实短剧 MP4（1-3 分钟），走完整 GUI 流程不崩溃
- [ ] 产出的高光片段时长落在 45-75 秒
- [ ] 转写文本基本准确
- [ ] LLM 判定理由讲得通，`narrative_structure` 四阶段划分跟真实剧情大致对得上
- [ ] 片段内容适合做广告钩子——路人观众零上下文能看懂冲击点、有悬念/信息缺口
- [ ] 片段结尾落在情绪/冲突高点附近，不是卡在台词中间
- [ ] 如果发现漏判（台词稀疏但更精彩的沉默画面时刻被漏掉），记录下来——这是 map.md Out of Scope 里"引入视觉理解模型"的触发条件，不是现在就要解决，但要记下证据

---

## 8. 跑通 ticket 09：真正出一个 MSI

```powershell
pwsh ./packaging/build.ps1 `
  -FfmpegDir  C:\path\to\ffmpeg-bin `
  -ModelsDir  C:\path\to\models `
  -CudaDir    C:\path\to\cuda-dlls `
  -Version    0.1.0
```

`-CudaDir` 需要你自己去 NVIDIA 官方下载匹配版本的 `cudart64_110.dll`/`cublas64_11.dll`/`cublasLt64_11.dll`/`cufft64_10.dll`/`curand64_10.dll`/`cudnn64_8.dll`——`packaging/wix.json` 里这些文件名是根据 sherpa-onnx 官方构建文档（CUDA 11.8）推断的，**没有验证过是否跟 v1.13.4 这个具体版本精确匹配**，如果 `go-msi make` 报错或者装完之后 ASR 用不了 CUDA，先来这里核对版本号。

第一次跑之前，还需要准备：
- `packaging/LICENSE.rtf`——项目还没选定开源许可证，这个文件目前不存在，先随便找个占位许可证文本转成 RTF（`go-msi to-rtf`）能让流程跑通，正式许可证定了之后再换
- `packaging/dist/NVIDIA_REDIST_LICENSE.txt`——NVIDIA 官方 CUDA/cuDNN 运行时 DLL 的重新分发许可声明文本，需要去 NVIDIA 官网找真实文本，不要瞎编

跑完之后按 ticket 09 checklist 逐条验收：全新环境安装、Win10/Win11 分别验证、CUDA 检测降级提示。

---

## 已知风险清单（提前看一遍，减少踩坑概率）

1. **CUDA execution provider 是否真的可用**——第 5 步会给出确定答案，是本清单里最重要的一个未知数。
2. **Paraformer 模型版本选择**（03-28 vs 09-14）——推荐用 09-14，但没有实测验证过时间戳输出，第 5 步验收标准里已经把这条列进去了。
3. **CUDA/cuDNN DLL 版本号**——`wix.json` 里的版本号是推断的，不是从真实 `onnxruntime.dll` 里读出来的。
4. **DeepSeek 服务稳定性**——不是这个项目的问题，但会影响你验证 04/06 时的体感，遇到 503 不要怀疑自己代码。
5. **`internal/asr` 的错误信息比较笼统**——`newOfflineRecognizer` 返回 nil 时，sherpa-onnx 官方绑定不暴露具体失败原因（代码注释里已经写了这条），模型加载失败时你只会看到一句"加载失败"，需要自己去确认是路径错、文件损坏还是版本不匹配。
