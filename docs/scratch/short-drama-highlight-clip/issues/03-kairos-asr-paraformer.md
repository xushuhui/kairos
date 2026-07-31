# 03 — internal/asr：Paraformer 集成

**What to build:** 本地 FunASR Paraformer-large 语音识别集成，走 `github.com/k2-fsa/sherpa-onnx-go-windows`（官方绑定，CUDA execution provider），实现 `core.Transcriber` interface。

**Blocked by:** None — can start immediately.

**Status:** ready-for-agent

- [ ] Paraformer-large + Silero VAD + 标点恢复模型加载成功（模型文件路径可配置）
- [ ] 对测试音频产出结构正确的 `[]core.Sentence`（`ID`/`StartMs`/`EndMs`/`Text` 字段齐全，字级时间戳正确合并为句级）
- [ ] CUDA 推理路径可用；初始化失败时自动降级 CPU 推理
- [ ] 本阶段验收目标是"跑通"，不是"准确率达标"——准确率验证在 ticket 06
