# 源文件读取、剪辑与临时文件处理策略

**Status:** resolved
**Created:** 2026-07-23 (repurposed)
**Label:** wayfinder:grilling
**Parent:** docs/scratch/short-drama-highlight-clip/map.md
**Assignee:** none

## Question

桌面软件如何处理本机视频文件的读取、中间产物与清理？需要确定：
- 用户选择的源文件已在本机磁盘，不需要下载步骤。需要考虑的是文件被外部移动/删除后软件如何处理（路径失效检测 + 重新选择提示）
- ASR 使用本地 FunASR Paraformer-large，需要 16kHz 单声道 WAV 输入——因此 ffmpeg 提取音轨 `-vn -ac 1 -ar 16000 -f wav` 不可省略，不再是可选项
- 临时文件（提取的 16kHz WAV 音轨、剪辑中的中间产物）的存放路径选择 —— `std::env::temp_dir()`（系统临时目录随重启自动清理）还是软件自有数据目录（持久化便于后续复现/调试，但需手动清理）
- 剪辑成功后，用户指定输出路径后高光片段写到哪里；如果用户未指定路径，默认写入源文件同目录下 `{source}_highlight.mp4`
- 磁盘空间检查：处理前检查可用空间是否足够存放临时 WAV 文件（1 分钟视频约 10 MB WAV）和输出文件（~ 几 MB/mp4），空间不足时提前报错而非中途崩溃
- GPU 显存资源：FFmpeg 硬解（`-hwaccel cuda`）和 Paraformer ONNX GPU 推理（CUDA execution provider）共享同一块 GPU，显存耗尽后二者都会骤降性能。团队实测显卡为 NVIDIA T1000（4GB/8GB 两种显存版本），桌面场景单路串行处理，基本无资源争夺风险
- 清理时机：处理完成后立即删除临时 WAV 文件，不额外等待。失败路径也要确保已生成的中间文件被清理（Go 没有 RAII/`Drop`，用 `defer os.RemoveAll(tmpDir)` 是等价惯用法，保证所有返回路径包括 panic/recover 都会执行）

## Key decisions to make

1. **临时文件路径**（已确认）：使用 `os.MkdirTemp()` 即可——系统会在重启时自动清理，软件无积累膨胀问题。清理用 `defer os.RemoveAll(tmpDir)`，成功/失败路径都执行。
2. **GPU 显存调度**（已确认，NVIDIA-only）：桌面场景串行操作，单路 FFmpeg 硬解 + Paraformer ONNX GPU 推理，均走 CUDA 路径（FFmpeg `-hwaccel cuda` + ONNX Runtime CUDA EP，`sherpa-onnx-go` 绑定本身也只支持 CUDA，不支持 DirectML，这条限制对本项目不构成问题）。Paraformer 是非自回归轻量模型（约 220M 参数），显存占用预计 < 1 GB，T1000 的 4GB/8GB 显存无资源争夺风险。CUDA 初始化失败（驱动异常等）时自动降级到 CPU 推理。
3. **源文件失效检测**（已确认）：用户开始剪辑前检测一次 `os.Stat()`，如果后来文件被删除则在 ffmpeg 读取时报错并捕获，提示用户重新选择文件。

## Blocked by

None — can start immediately.
