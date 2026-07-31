# 07 — internal/history：JSON 旁路历史记录

**What to build:** JSON 旁路文件写入/扫描（04-task-state-schema-retry.md 定的方案，不用 SQLite）——每次处理完在 `%APPDATA%/kairos/history/` 目录下写一个 `{时间戳}_{源文件名}.json`，字段包含 source_path/highlight_path/asr_raw_result/llm_raw_response/status/created_at 等。

**Blocked by:** None — can start immediately.

**Status:** ready-for-agent

- [ ] `WriteRecord()` 成功写入一条包含完整 `asr_raw_result`/`llm_raw_response` 的 JSON 文件
- [ ] `ListRecords()` 扫描目录、解析全部 JSON 文件、按 `created_at` 倒序返回
- [ ] 目录不存在时自动创建（`%APPDATA%/kairos/history/`）
- [ ] 不实现删除/导出功能（已确认不做，见 04 文档）
