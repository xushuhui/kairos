// Package history 用 JSON 旁路文件（%APPDATA%/kairos/history/）记录每次处理的
// 业务级摘要，不用 SQLite——数据量小（单机单团队，一天几十条记录量级），
// 文件系统扫描的性能跟数据库查询没有实质差别，换来的是省一个 cgo 依赖
// （mattn/go-sqlite3）。不提供删除/导出功能（已确认不做，见 spec.md 本地存储一节）。
package history

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"

	"kairos/internal/core"
)

// timestampLayout 是历史文件名里时间戳部分的格式：文件系统安全（无冒号等非法字符），
// 且按字典序排列即等价于按时间排列。
const timestampLayout = "20060102-150405.000"

// userConfigDir 是 os.UserConfigDir 的可替换点，测试用它注入临时目录，
// 避免真的写入本机的 %APPDATA%。
var userConfigDir = os.UserConfigDir

// Record 是一次处理（成功或失败）的业务级摘要，字段列表见
// docs/scratch/short-drama-highlight-clip/spec.md 本地存储一节。
// ASRRawResult/LLMRawResponse 直接复用 Transcriber/HighlightJudge 的输出类型，
// 供历史记录页面回看转写文本和 LLM 判定理由（spec.md 用户故事 20/21）。
type Record struct {
	SourcePath       string               `json:"source_path"`
	SourceName       string               `json:"source_name"`
	HighlightPath    string               `json:"highlight_path,omitempty"`
	HighlightStartMs uint64               `json:"highlight_start_ms,omitempty"`
	HighlightEndMs   uint64               `json:"highlight_end_ms,omitempty"`
	ASRRawResult     []core.Sentence      `json:"asr_raw_result,omitempty"`
	LLMRawResponse   core.HighlightWindow `json:"llm_raw_response,omitempty"`
	Status           string               `json:"status"` // "success" | "failed"
	ErrorMessage     string               `json:"error_message,omitempty"`
	CreatedAt        time.Time            `json:"created_at"`
}

// historyDir 解析 %APPDATA%/kairos/history/ 目录，不存在时自动创建，
// 供 WriteRecord/ListRecords 共用。
func historyDir() (string, error) {
	base, err := userConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}

	dir := filepath.Join(base, "kairos", "history")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create history dir: %w", err)
	}
	return dir, nil
}

// WriteRecord 把 rec 序列化为一个 JSON 文件写入历史目录，文件名
// {时间戳}_{源文件名}.json，天然按文件名排序即接近按时间排序。
func WriteRecord(rec Record) error {
	dir, err := historyDir()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal history record: %w", err)
	}

	name := fmt.Sprintf("%s_%s.json", rec.CreatedAt.Format(timestampLayout), filepath.Base(rec.SourcePath))
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write history record %s: %w", path, err)
	}
	return nil
}

// ListRecords 扫描历史目录下所有 .json 文件并解析为 Record，按 CreatedAt 倒序
// （最新的在前）返回。单个文件解析失败不中断整体扫描，只跳过并记录日志——
// 一条历史记录损坏不该让整个历史列表打不开。
func ListRecords() ([]Record, error) {
	dir, err := historyDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read history dir: %w", err)
	}

	records := make([]Record, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			slog.Warn("history: skip unreadable record", "path", path, "error", err)
			continue
		}

		var rec Record
		if err := json.Unmarshal(data, &rec); err != nil {
			slog.Warn("history: skip malformed record", "path", path, "error", err)
			continue
		}
		records = append(records, rec)
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})

	return records, nil
}
