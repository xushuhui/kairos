# 09 — 打包分发

**What to build:** `go-msi` 出 MSI 安装包，内置 FFmpeg 可执行文件 + Paraformer/Silero VAD 模型文件 + CUDA/cuDNN 运行时 DLL（`cudart64_*.dll`/`cublas64_*.dll`/`cublasLt64_*.dll`/`cufft64_*.dll`/`curand64_*.dll`/`cudnn*.dll`），目标系统 Windows 10 1903+ / Windows 11。

**Blocked by:** 08

**Status:** ready-for-agent

- [ ] MSI 在全新 Windows 环境（无预装 Go/FFmpeg/任何依赖，但已装 NVIDIA 显卡驱动）上能正常安装
- [ ] 安装后直接可用，不需要用户额外下载 FFmpeg、模型文件或 CUDA Toolkit
- [ ] 在 Windows 10（1903 以上）和 Windows 11 两个系统上分别验证安装+运行
- [ ] CUDA/cuDNN 运行时 DLL 走安装包内置，不依赖用户系统单独安装 CUDA Toolkit；版本号锁定，跟打包时选用的 ONNX Runtime 版本匹配
- [ ] 不购买代码签名证书（已确认），SmartScreen 警告接受为已知摩擦
- [ ] 首次启动检测 CUDA 运行时可用性，不满足时降级 CPU 推理并提示用户
