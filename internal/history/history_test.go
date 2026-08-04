package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// withTempConfigDir 把 userConfigDir 替换为一个测试临时目录，避免测试写入
// 本机真实的 %APPDATA%；调用结束后自动恢复。
func withTempConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := userConfigDir
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = orig })
	return dir
}

func sampleRecord(sourceName string, createdAt time.Time) Record {
	return Record{
		SourcePath:       "/videos/" + sourceName,
		SourceName:       sourceName,
		HighlightPath:    "/videos/" + sourceName + "_highlight.mp4",
		HighlightStartMs: 15_000,
		HighlightEndMs:   75_000,
		ASRRawResult: json.RawMessage(`[` +
			`{"id":0,"start_ms":0,"end_ms":1200,"text":"第一句台词"},` +
			`{"id":1,"start_ms":1200,"end_ms":2400,"text":"第二句台词"}]`),
		LLMRawResponse: json.RawMessage(`{` +
			`"narrative_structure":{"setup":[0,1],"rising_action":[2,3],"climax":[4,5]},` +
			`"start_id":4,"end_id":5,"reason":"冲突集中，适合做广告钩子"}`),
		Status:    "success",
		CreatedAt: createdAt,
	}
}

func TestWriteRecordRoundTrip(t *testing.T) {
	dir := withTempConfigDir(t)

	rec := sampleRecord("ep01.mp4", time.Date(2026, 7, 30, 15, 30, 0, 0, time.UTC))
	if err := WriteRecord(rec); err != nil {
		t.Fatalf("WriteRecord() error = %v", err)
	}

	histDir := filepath.Join(dir, "kairos", "history")
	entries, err := os.ReadDir(histDir)
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", histDir, err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d files in history dir, want 1", len(entries))
	}

	name := entries[0].Name()
	if !strings.Contains(name, "ep01.mp4") || !strings.HasPrefix(name, "20260730-153000.000_") {
		t.Errorf("history file name = %q, want timestamp+source-name pattern", name)
	}

	raw, err := os.ReadFile(filepath.Join(histDir, name))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	rawStr := string(raw)

	for _, want := range []string{
		`"asr_raw_result"`, "第一句台词", "第二句台词",
		`"llm_raw_response"`, "冲突集中，适合做广告钩子",
	} {
		if !strings.Contains(rawStr, want) {
			t.Errorf("raw history file missing %q; content:\n%s", want, rawStr)
		}
	}
}

func TestListRecordsSortedNewestFirst(t *testing.T) {
	withTempConfigDir(t)

	t0 := time.Date(2026, 7, 28, 10, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 7, 30, 10, 0, 0, 0, time.UTC)

	// 故意打乱写入顺序，验证 ListRecords 是按 CreatedAt 排序而非写入顺序。
	for _, rec := range []Record{
		sampleRecord("ep02.mp4", t1),
		sampleRecord("ep03.mp4", t2),
		sampleRecord("ep01.mp4", t0),
	} {
		if err := WriteRecord(rec); err != nil {
			t.Fatalf("WriteRecord() error = %v", err)
		}
	}

	records, err := ListRecords()
	if err != nil {
		t.Fatalf("ListRecords() error = %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("got %d records, want 3", len(records))
	}

	wantOrder := []string{"ep03.mp4", "ep02.mp4", "ep01.mp4"}
	for i, want := range wantOrder {
		if records[i].SourceName != want {
			t.Errorf("records[%d].SourceName = %q, want %q (full order: %v)", i, records[i].SourceName, want, sourceNames(records))
		}
	}
	if !(records[0].CreatedAt.After(records[1].CreatedAt) && records[1].CreatedAt.After(records[2].CreatedAt)) {
		t.Errorf("records not strictly descending by CreatedAt: %v", sourceNames(records))
	}
}

func sourceNames(records []Record) []string {
	names := make([]string, len(records))
	for i, r := range records {
		names[i] = r.SourceName
	}
	return names
}

func TestWriteAndListRecordsAutoCreateHistoryDir(t *testing.T) {
	dir := withTempConfigDir(t)
	histDir := filepath.Join(dir, "kairos", "history")

	if _, err := os.Stat(histDir); !os.IsNotExist(err) {
		t.Fatalf("history dir unexpectedly pre-exists before test: %v", err)
	}

	// ListRecords 先跑：必须在目录不存在时自动创建而不是报错。
	records, err := ListRecords()
	if err != nil {
		t.Fatalf("ListRecords() on missing dir error = %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("ListRecords() on empty dir = %v, want empty", records)
	}
	if info, err := os.Stat(histDir); err != nil || !info.IsDir() {
		t.Fatalf("history dir not auto-created by ListRecords(): stat error = %v", err)
	}

	// 再用一个全新的临时目录验证 WriteRecord 同样能自动创建。
	dir2 := t.TempDir()
	userConfigDir = func() (string, error) { return dir2, nil }

	if err := WriteRecord(sampleRecord("ep01.mp4", time.Now())); err != nil {
		t.Fatalf("WriteRecord() on missing dir error = %v", err)
	}
	if info, err := os.Stat(filepath.Join(dir2, "kairos", "history")); err != nil || !info.IsDir() {
		t.Fatalf("history dir not auto-created by WriteRecord(): stat error = %v", err)
	}
}

func TestListRecordsSkipsMalformedFile(t *testing.T) {
	dir := withTempConfigDir(t)

	if err := WriteRecord(sampleRecord("ep01.mp4", time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC))); err != nil {
		t.Fatalf("WriteRecord() error = %v", err)
	}

	histDir := filepath.Join(dir, "kairos", "history")
	badPath := filepath.Join(histDir, "20260730-100000.000_corrupt.json")
	if err := os.WriteFile(badPath, []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("WriteFile(bad record) error = %v", err)
	}

	records, err := ListRecords()
	if err != nil {
		t.Fatalf("ListRecords() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1 (malformed file should be skipped): %v", len(records), sourceNames(records))
	}
	if records[0].SourceName != "ep01.mp4" {
		t.Errorf("records[0].SourceName = %q, want ep01.mp4", records[0].SourceName)
	}
}
