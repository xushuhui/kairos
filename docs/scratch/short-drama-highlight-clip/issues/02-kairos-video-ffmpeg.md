# 02 — internal/video：FFmpeg 封装

**What to build:** FFmpeg 子进程封装——音轨提取（16kHz 单声道 WAV）、CUDA 检测（团队实测显卡 NVIDIA T1000，NVIDIA-only）、视频剪辑截取。

**Blocked by:** None — can start immediately.

**Status:** ready-for-agent

- [ ] `ExtractAudio()` 对一个预置 5 秒测试 MP4 产出合法 16kHz mono WAV
- [ ] `CudaAvailable()` 正确检测本机 CUDA 运行时可用性
- [ ] `CutClip()` 用 `h264_nvenc` 对测试 MP4 产出可播放的输出文件
- [ ] CUDA 不可用或检测失败时，自动退化到 `libx264` 软编码，不崩溃
