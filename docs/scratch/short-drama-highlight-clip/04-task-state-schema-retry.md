# 本地处理历史记录方案

**Status:** resolved
**Created:** 2026-07-23 (repurposed)
**Label:** wayfinder:grilling
**Parent:** docs/scratch/short-drama-highlight-clip/map.md
**Assignee:** none

## Context

桌面软件是单机单用户场景，不需要"异步任务表 + 状态机 + 重试策略"这套服务端范式。用户打开软件后操作是即时交互式的：选取文件 → 开始剪辑 → 等待进度条走完 → 拿到高光片段。这里的「历史记录」仅用于：
- 查看之前剪过哪些视频
- 查看某次剪辑的 ASR 转写文本、LLM 判定理由
- 失败后查看错误原因，修好问题重试

## Question

本地历史记录的存储模型应如何设计？需要确定：
- 使用 SQLite 还是 JSON 旁路文件存储——这个内部工具单机使用，实际数据量小（几个剪辑同事，一天处理几个到几十个视频，不是高并发/大数据量场景），SQLite 换来的"按时间排序/搜索"查询能力在这个量级下扫目录也能做到同样效果，但要多背一个 cgo 依赖（`mattn/go-sqlite3`）和一个专门的存储包
- 表字段：`id (integer PK)`、`source_path (text)`、`source_name (text)`、`highlight_path (text)`、`highlight_start_ms (integer)`、`highlight_end_ms (integer)`、`asr_raw_result (text/json)`（中间产物便于调试和复现）、`llm_raw_response (text/json)`（包括判定理由）、`status (text: success/failed)`、`error_message (text)`、`created_at (text/datetime)`
- 是否需要在 asr_raw_result 中保留完整的 ASR 分句 + 时间戳数据？——对调试和复现非常重要（用户的某个源文件删除后，文本记录仍可供回看），但会增加存储至数百 KB/条
- 是否需要删除历史记录的功能（UI 侧清理旧记录）
- 是否需要导出历史记录的功能（便于共享/调试）

## Key decisions to make

1. **JSON 旁路文件，不用 SQLite**（已确认，2026-07-30 修正，原方案为 SQLite）：每次处理完（成功或失败）在 `%APPDATA%/kairos/history/` 目录下写一个 JSON 文件（文件名 `{时间戳}_{源文件名}.json`，天然按文件名排序即接近按时间排序），历史列表 UI 扫描这个目录、解析每个 JSON 文件展示。放弃 SQLite 的理由：这个数据量下（单机单团队，一天几十条记录量级）文件系统扫描的性能跟数据库查询没有实质差别，SQLite 换来的查询能力用不上，但要多背一个 cgo 依赖和一个专门的包——用不上的能力不该为它付维护成本。
2. **保留完整中间产物**（ASR 转写文本 + 时间戳 + LLM 原始响应），单条记录数百 KB 可接受（用户电脑没有几千条记录）。
3. **不需状态机**：单用户串行操作，不需要 pending/processing/succeeded/failed 状态流转。记录只在操作完成后写入（成功或失败），操作过程中不必记录。
4. **不需重试策略**：失败后用户 UI 中看到错误信息，手动重试比自动重试更符合桌面软件用户预期。
5. **不做删除功能**（已确认）：历史记录只增不减，不在 UI 里加清理旧记录的入口。个人工具场景数据量不大，JSON 文件本身可以让用户直接删（不是软件职责范围）。
6. **不做导出功能**（已确认）：不做导出记录的入口，不是当前需求范围。

## Blocked by

None — 历史记录方案不依赖其他文档。
