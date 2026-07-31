# 桌面安装包打包与分发

**Status:** resolved
**Created:** 2026-07-23 (repurposed)
**Label:** wayfinder:grilling
**Parent:** docs/scratch/short-drama-highlight-clip/map.md
**Assignee:** none

## Context

桌面软件不再涉及服务端部署/CI-CD，但需要一套 Windows 安装包打包与分发方案。

## Question

如何将 Go 原生桌面应用打包为 Windows 安装包并推送给用户？需要确定：
- 打包工具：Go 生态 `go-msi`（包一层 WiX，出 MSI）、直接用 WiX Toolset 打包编译好的 exe、或 Inno Setup / NSIS 脚本打包——`go-msi` 对 Go 项目集成最直接（YAML 配置声明文件清单），WiX Toolset 需要单独安装；Inno Setup 更通用但需手工维护打包脚本
- 安装包内嵌哪些文件：Go 编译的单文件二进制、FFmpeg 可执行文件、Paraformer ONNX 模型文件（~ 几百 MB）、VAD 模型文件（Silero VAD，几十 MB）、CUDA/cuDNN 运行时 DLL——这些文件的打包策略直接影响安装包体积
- FFmpeg 依赖策略：安装包内置（增加 ~ 100+ MB）、引导用户自行下载（首次运行检查并引导）、使用系统已安装的 FFmpeg（如用户已装则跳过）
- 模型文件部署策略：安装包内置（安装即用，但体积大）or 首次运行时后台下载（安装包体积小，但首次运行有等待从几秒到几分不等）
- GPU 硬件加速策略：NVIDIA-only（团队实测显卡为 T1000）。FFmpeg 解码走 `-hwaccel cuda`，编码走 `h264_nvenc`，不可用时退化 `libx264`；Paraformer ONNX 推理走 CUDA execution provider。首次运行检测 CUDA 运行时可用性，不满足时降级到 CPU 推理
- CUDA/cuDNN 运行时 DLL 打包：ONNX Runtime 的 CUDA execution provider 需要匹配版本的 `cudart64_*.dll`/`cublas64_*.dll`/`cublasLt64_*.dll`/`cufft64_*.dll`/`curand64_*.dll`/`cudnn*.dll`，能否打进安装包、版本怎么锁定
- 数字签名是否需要：MSI/exe 无签名会触发 Windows SmartScreen 警告，需要代码签名证书（OV 或 EV）
- 操作系统版本覆盖：Win10 和 Win11 都要支持

## Key decisions to make

1. **打包工具**（已确认）：`go-msi` 出 MSI——YAML 配置声明式管理文件清单，跟 Go 单文件二进制的产出模式契合。Inno Setup 作为备选方案（如果 `go-msi` 的配置复杂性超过收益）。
2. **FFmpeg 策略**（已确认）：安装包内置 pre-built Windows FFmpeg 二进制（官方 `ffmpeg.exe`），核心功能不依赖用户环境。
3. **模型文件部署**（已确认）：安装包内置，不走首次运行下载。首次运行时下载模型文件对用户体验的伤害（等待 + 网络问题 + 失败率）大于安装包增大的伤害。从减少分发体积的角度，可考虑使用量化版本（q8 量化的 GGUF/ONNX 模型约为原始体积的一半，精度损失极小），但"内置"这个决定本身不变。
4. **GPU 检测**（已确认，NVIDIA-only）：首次启动检测 CUDA 运行时可用性；不可用时自动回退到 CPU 推理并提示用户性能损失。FFmpeg 编码器优先 `h264_nvenc`，不可用时退化到 `libx264` 软编码。
5. **CUDA/cuDNN 运行时 DLL 打包**（已确认，技术核实过）：把匹配版本的 `cudart64_*.dll`/`cublas64_*.dll`/`cublasLt64_*.dll`/`cufft64_*.dll`/`curand64_*.dll`/`cudnn*.dll` 直接打进安装包，放在 exe 同目录即可被加载，不需要用户系统装完整 CUDA Toolkit——NVIDIA 官方允许这些运行时 DLL 按其许可条款重新分发（需在安装包里附带 NVIDIA 许可声明）。**用户仍需自行安装匹配的 NVIDIA 显卡驱动**（驱动本身不能塞进安装包），但 T1000 这类工作站显卡出厂/IT 部署时通常已装好驱动，属正常预期。ORT/CUDA/cuDNN 版本要严格匹配（cuDNN 8.x 和 9.x 不能混用），打包阶段需锁定具体版本号。
6. **数字签名**（已确认）：不购买代码签名证书。用户安装时会看到 Windows SmartScreen"未知发布者"警告，需点击"更多信息→仍要运行"继续——个人工具场景接受这个一次性摩擦，不为此花钱买 OV/EV 证书年费。
7. **操作系统版本覆盖**（已确认，Win10 1903+ / Win11）：CUDA/cuDNN 运行时 DLL 跟 Windows 版本无关，两个系统均支持，不需要像 DirectML 那样考虑系统内置版本问题——这条随语言从 Rust 换 Go、GPU 策略从 DirectML 换 CUDA 后自动简化，不再需要单独打包"DirectML redistributable"。

## Blocked by

None — 不依赖其他文档，模型选型（Paraformer-large）已定后可决定具体模型文件清单与体积。
