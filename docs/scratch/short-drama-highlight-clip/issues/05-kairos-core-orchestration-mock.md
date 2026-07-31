# 05 — internal/core 编排（mock 测试）

**What to build:** `RunHighlightExtraction()` 编排函数——串联源文件校验、磁盘空间检查、调用 FFmpeg 提取音轨、调用 Transcriber、拼装句子列表送 HighlightJudge、拿 LLM 输出查表映射时间戳、调用 `ComputeWindow()`、调用 FFmpeg 剪辑、清理临时文件、写历史记录。

**Blocked by:** 01（需要 `ComputeWindow()` 和 `Transcriber`/`HighlightJudge` interface 定义）

**Status:** ready-for-agent

- [ ] 用 mock `Transcriber`/`HighlightJudge` 实现（手写 fake struct）跑通全流程测试，不依赖真实 GPU/网络/API Key
- [ ] 源文件不存在时返回 `ErrSourceFileMissing`，不 panic
- [ ] 磁盘空间不足时处理前报错，不中途失败
- [ ] 临时 WAV 文件在成功和失败路径下都被清理（`defer os.RemoveAll(tmpDir)`）
- [ ] LLM 输出的 `EndID` 正确查表映射为毫秒时间戳，传给 `ComputeWindow()`
