package core

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kairos/internal/apppath"
	"kairos/internal/history"
	"kairos/internal/testutil"
)

// withTempHome 把 apppath.Dir（history.WriteRecord/ListRecords 用它定位
// history/ 目录）重定向到一个测试临时目录，避免 RunHighlightExtraction
// 内部调用 history.WriteRecord() 时真的写进这台机器上编译出的测试二进制
// 所在目录。
func withTempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := apppath.Dir
	apppath.Dir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { apppath.Dir = orig })
	return dir
}

// fakeTranscriber 和 fakeJudge 是 spec.md「两个测试缝合点」——手写 fake
// struct 实现 Transcriber/HighlightJudge，不依赖真实 GPU/网络/API Key。
type fakeTranscriber struct {
	sentences []Sentence
	err       error
}

func (f *fakeTranscriber) Transcribe(audioPath string) ([]Sentence, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.sentences, nil
}

type fakeJudge struct {
	window HighlightWindow
	err    error
	called bool
}

func (f *fakeJudge) Judge(sentences []Sentence) (HighlightWindow, error) {
	f.called = true
	if f.err != nil {
		return HighlightWindow{}, f.err
	}
	return f.window, nil
}

// testSentences 是三句台词，句末时间戳固定，供多个用例复用。
func testSentences() []Sentence {
	return []Sentence{
		{ID: 0, StartMs: 0, EndMs: 1_000, Text: "开场白"},
		{ID: 1, StartMs: 1_000, EndMs: 2_500, Text: "冲突升级"},
		{ID: 2, StartMs: 2_500, EndMs: 4_000, Text: "高潮反转"},
	}
}

func TestRunHighlightExtraction_MockFullFlow(t *testing.T) {
	testutil.RequireFfmpeg(t)
	withTempHome(t)

	src := testutil.MakeTestMp4(t) // 5s fixture
	outDir := t.TempDir()

	transcriber := &fakeTranscriber{sentences: testSentences()}
	judge := &fakeJudge{window: HighlightWindow{StartID: 0, EndID: 2, Reason: "测试判定理由"}}

	// targetLenMs=2000 且 fixture 时长 5000ms > targetLenMs，走 ComputeWindow
	// 的正常分支（不触发"视频短于目标时长"提前返回），能验证 EndID 到毫秒
	// 时间戳的查表映射是否正确：peakEndMs = sentences[2].EndMs = 4000，
	// start = 4000-2000 = 2000，end = 4000。
	cfg := Config{
		OutputPath:  filepath.Join(outDir, "clip.mp4"),
		TargetLenMs: 2_000,
	}

	output, err := RunHighlightExtraction(src, cfg, transcriber, judge)
	if err != nil {
		t.Fatalf("RunHighlightExtraction() error = %v", err)
	}
	if output.StartMs != 2_000 || output.EndMs != 4_000 {
		t.Errorf("output = {StartMs:%d EndMs:%d}, want {StartMs:2000 EndMs:4000} (end_id=2 -> EndMs=4000 correctly mapped)",
			output.StartMs, output.EndMs)
	}
	if output.JudgeReason != "测试判定理由" {
		t.Errorf("output.JudgeReason = %q, want %q", output.JudgeReason, "测试判定理由")
	}
	if output.OutputPath != cfg.OutputPath {
		t.Errorf("output.OutputPath = %q, want %q", output.OutputPath, cfg.OutputPath)
	}
	if _, statErr := os.Stat(output.OutputPath); statErr != nil {
		t.Errorf("expected output clip at %s to exist: %v", output.OutputPath, statErr)
	}

	records, listErr := history.ListRecords()
	if listErr != nil {
		t.Fatalf("history.ListRecords() error = %v", listErr)
	}
	if len(records) != 1 {
		t.Fatalf("got %d history records, want 1", len(records))
	}
	if records[0].Status != "success" {
		t.Errorf("history record Status = %q, want %q", records[0].Status, "success")
	}
	if len(records[0].ASRRawResult) == 0 {
		t.Error("history record ASRRawResult is empty, want marshaled sentences")
	}
	if len(records[0].LLMRawResponse) == 0 {
		t.Error("history record LLMRawResponse is empty, want marshaled judge window")
	}
}

func TestRunHighlightExtraction_CreatesOutputDirIfMissing(t *testing.T) {
	testutil.RequireFfmpeg(t)
	withTempHome(t)

	src := testutil.MakeTestMp4(t)
	// nested/missing 目录不预先创建——RunHighlightExtraction 自己必须建出来，
	// 否则 CutClip 的 ffmpeg 子进程写不进一个不存在的目录会直接失败
	// （预检查发现的问题：之前隐含依赖调用方/GUI 层保证输出目录已存在）。
	outputPath := filepath.Join(t.TempDir(), "nested", "missing", "clip.mp4")
	cfg := Config{OutputPath: outputPath, TargetLenMs: 2_000}

	output, err := RunHighlightExtraction(src, cfg,
		&fakeTranscriber{sentences: testSentences()},
		&fakeJudge{window: HighlightWindow{EndID: 2}},
	)
	if err != nil {
		t.Fatalf("RunHighlightExtraction() error = %v, want the missing output dir to be created automatically", err)
	}
	if _, statErr := os.Stat(output.OutputPath); statErr != nil {
		t.Errorf("expected output clip at %s to exist: %v", output.OutputPath, statErr)
	}
}

func TestRunHighlightExtraction_OnProgressReportsStagesInOrder(t *testing.T) {
	testutil.RequireFfmpeg(t)
	withTempHome(t)

	src := testutil.MakeTestMp4(t)
	var stages []Stage
	cfg := Config{
		OutputPath:  filepath.Join(t.TempDir(), "clip.mp4"),
		TargetLenMs: 2_000,
		OnProgress:  func(s Stage) { stages = append(stages, s) },
	}

	if _, err := RunHighlightExtraction(src, cfg,
		&fakeTranscriber{sentences: testSentences()},
		&fakeJudge{window: HighlightWindow{EndID: 2}},
	); err != nil {
		t.Fatalf("RunHighlightExtraction() error = %v", err)
	}

	want := []Stage{StageExtractingAudio, StageTranscribing, StageJudging, StageCutting}
	if len(stages) != len(want) {
		t.Fatalf("got %d progress callbacks %v, want %d %v", len(stages), stages, len(want), want)
	}
	for i, s := range want {
		if stages[i] != s {
			t.Errorf("stage[%d] = %q, want %q", i, stages[i], s)
		}
	}
}

func TestRunHighlightExtraction_OnProgressNilIsSafe(t *testing.T) {
	testutil.RequireFfmpeg(t)
	withTempHome(t)

	src := testutil.MakeTestMp4(t)
	cfg := Config{OutputPath: filepath.Join(t.TempDir(), "clip.mp4"), TargetLenMs: 2_000} // OnProgress left nil
	if _, err := RunHighlightExtraction(src, cfg,
		&fakeTranscriber{sentences: testSentences()},
		&fakeJudge{window: HighlightWindow{EndID: 2}},
	); err != nil {
		t.Fatalf("RunHighlightExtraction() error = %v, want nil OnProgress to be a no-op, not a crash", err)
	}
}

func TestRunHighlightExtraction_SourceFileMissing(t *testing.T) {
	withTempHome(t)

	_, err := RunHighlightExtraction(
		filepath.Join(t.TempDir(), "does-not-exist.mp4"),
		Config{},
		&fakeTranscriber{},
		&fakeJudge{},
	)
	if !errors.Is(err, ErrSourceFileMissing) {
		t.Fatalf("RunHighlightExtraction() error = %v, want errors.Is(err, ErrSourceFileMissing)", err)
	}

	// 校验前置校验失败不 panic、也不写历史记录（还没开始真正的处理）。
	if records, listErr := history.ListRecords(); listErr == nil && len(records) != 0 {
		t.Errorf("got %d history records for a source-file-missing failure, want 0", len(records))
	}
}

func TestRunHighlightExtraction_InsufficientDiskSpace(t *testing.T) {
	testutil.RequireFfmpeg(t)
	withTempHome(t)

	orig := diskFreeBytes
	diskFreeBytes = func(dir string) (uint64, error) { return 1, nil } // 1 字节，必然不够
	t.Cleanup(func() { diskFreeBytes = orig })

	src := testutil.MakeTestMp4(t)
	_, err := RunHighlightExtraction(src, Config{}, &fakeTranscriber{}, &fakeJudge{})
	if !errors.Is(err, ErrInsufficientDiskSpace) {
		t.Fatalf("RunHighlightExtraction() error = %v, want errors.Is(err, ErrInsufficientDiskSpace)", err)
	}
}

func TestRunHighlightExtraction_TempDirCleanedUpOnSuccessAndFailure(t *testing.T) {
	testutil.RequireFfmpeg(t)
	withTempHome(t)

	countLeftoverTmpDirs := func() int {
		matches, _ := filepath.Glob(filepath.Join(os.TempDir(), "kairos-*"))
		return len(matches)
	}

	src := testutil.MakeTestMp4(t)
	before := countLeftoverTmpDirs()

	// success path
	cfg := Config{OutputPath: filepath.Join(t.TempDir(), "clip.mp4"), TargetLenMs: 2_000}
	if _, err := RunHighlightExtraction(src, cfg,
		&fakeTranscriber{sentences: testSentences()},
		&fakeJudge{window: HighlightWindow{EndID: 2}},
	); err != nil {
		t.Fatalf("success-path RunHighlightExtraction() error = %v", err)
	}
	if got := countLeftoverTmpDirs(); got != before {
		t.Errorf("after success path: %d leftover kairos-* temp dirs, want %d (cleaned up)", got, before)
	}

	// failure path (transcriber fails after tmpDir is created)
	if _, err := RunHighlightExtraction(src, cfg,
		&fakeTranscriber{err: errors.New("boom")},
		&fakeJudge{},
	); err == nil {
		t.Fatal("failure-path RunHighlightExtraction() error = nil, want error")
	}
	if got := countLeftoverTmpDirs(); got != before {
		t.Errorf("after failure path: %d leftover kairos-* temp dirs, want %d (cleaned up)", got, before)
	}
}

func TestRunHighlightExtraction_TranscribeFailure(t *testing.T) {
	testutil.RequireFfmpeg(t)
	withTempHome(t)

	src := testutil.MakeTestMp4(t)
	wantErr := errors.New("asr blew up")
	_, err := RunHighlightExtraction(src, Config{}, &fakeTranscriber{err: wantErr}, &fakeJudge{})
	if !errors.Is(err, ErrTranscriptionFailed) || !errors.Is(err, wantErr) {
		t.Fatalf("RunHighlightExtraction() error = %v, want wrapping both ErrTranscriptionFailed and %v", err, wantErr)
	}

	records, _ := history.ListRecords()
	if len(records) != 1 || records[0].Status != "failed" {
		t.Errorf("history record = %+v, want 1 record with Status=failed", records)
	}
}

// 转写"成功"但一句台词都没识别出来（VAD 没切出语音）——不该继续送空台词表
// 给 judge.Judge()（真机验证遇到过 DeepSeek 对着空输入瞎猜、返回越界
// end_id 的情况），也不该写出一个空的台词文件（用户会误以为是 bug）。
func TestRunHighlightExtraction_NoSpeechDetected(t *testing.T) {
	testutil.RequireFfmpeg(t)
	withTempHome(t)

	src := testutil.MakeTestMp4(t)
	judge := &fakeJudge{}
	_, err := RunHighlightExtraction(src, Config{}, &fakeTranscriber{sentences: nil}, judge)
	if !errors.Is(err, ErrNoSpeechDetected) {
		t.Fatalf("RunHighlightExtraction() error = %v, want wrapping ErrNoSpeechDetected", err)
	}
	if judge.called {
		t.Error("judge.Judge() was called with an empty sentence table, want it skipped entirely")
	}
	if _, statErr := os.Stat(transcriptFilePath(src)); !os.IsNotExist(statErr) {
		t.Errorf("transcript file unexpectedly written for empty transcription result: stat error = %v", statErr)
	}

	records, _ := history.ListRecords()
	if len(records) != 1 || records[0].Status != "failed" {
		t.Errorf("history record = %+v, want 1 record with Status=failed", records)
	}
}

// 用户明确要求「必须保证台词文件生成并且有内容再进行下一步」：写入台词
// 文件失败必须硬性中止，不能进入 judge.Judge()。预先在目标路径造一个同名
// 目录，让 os.WriteFile 必然因为"目标是目录"失败——这个手法跟平台/权限
// 模型无关（哪怕以 root/管理员跑，往一个目录路径当文件写都会失败），
// 比 chmod 只读目录更可靠。
func TestRunHighlightExtraction_TranscriptFileWriteFailure(t *testing.T) {
	testutil.RequireFfmpeg(t)
	withTempHome(t)

	src := testutil.MakeTestMp4(t)
	if err := os.MkdirAll(transcriptFilePath(src), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	judge := &fakeJudge{}
	_, err := RunHighlightExtraction(src, Config{}, &fakeTranscriber{sentences: testSentences()}, judge)
	if !errors.Is(err, ErrTranscriptFileWriteFailed) {
		t.Fatalf("RunHighlightExtraction() error = %v, want wrapping ErrTranscriptFileWriteFailed", err)
	}
	if judge.called {
		t.Error("judge.Judge() was called despite transcript file write failure, want it skipped")
	}
}

func TestRunHighlightExtraction_JudgeFailure(t *testing.T) {
	testutil.RequireFfmpeg(t)
	withTempHome(t)

	src := testutil.MakeTestMp4(t)
	wantErr := errors.New("llm blew up")
	_, err := RunHighlightExtraction(src, Config{}, &fakeTranscriber{sentences: testSentences()}, &fakeJudge{err: wantErr})
	if !errors.Is(err, ErrLlmInvalidResponse) || !errors.Is(err, wantErr) {
		t.Fatalf("RunHighlightExtraction() error = %v, want wrapping both ErrLlmInvalidResponse and %v", err, wantErr)
	}
}

// 用户明确要求「转写台词步骤和发给 DeepSeek 分开，台词放到视频同一个目录」：
// 即使 judge.Judge() 失败（比如 DeepSeek 超时），转写产物也已经落盘，不随
// LLM 调用失败一起丢失。
func TestRunHighlightExtraction_TranscriptFileWrittenEvenIfJudgingFails(t *testing.T) {
	testutil.RequireFfmpeg(t)
	withTempHome(t)

	src := testutil.MakeTestMp4(t)
	_, err := RunHighlightExtraction(src, Config{}, &fakeTranscriber{sentences: testSentences()}, &fakeJudge{err: errors.New("deepseek 超时")})
	if err == nil {
		t.Fatal("RunHighlightExtraction() error = nil, want non-nil (judge configured to fail)")
	}

	wantPath := transcriptFilePath(src)
	raw, readErr := os.ReadFile(wantPath)
	if readErr != nil {
		t.Fatalf("transcript file not written at %s: %v", wantPath, readErr)
	}
	got := string(raw)
	for _, want := range []string{"开场白", "冲突升级", "高潮反转"} {
		if !strings.Contains(got, want) {
			t.Errorf("transcript file content = %q, want it to contain %q", got, want)
		}
	}

	if filepath.Dir(wantPath) != filepath.Dir(src) {
		t.Errorf("transcript file dir = %q, want same dir as source video %q", filepath.Dir(wantPath), filepath.Dir(src))
	}
}

func TestRunHighlightExtraction_EndIDOutOfRange(t *testing.T) {
	testutil.RequireFfmpeg(t)
	withTempHome(t)

	src := testutil.MakeTestMp4(t)
	transcriber := &fakeTranscriber{sentences: testSentences()} // 3 sentences, valid ids 0-2
	judge := &fakeJudge{window: HighlightWindow{EndID: 99}}

	_, err := RunHighlightExtraction(src, Config{}, transcriber, judge)
	if !errors.Is(err, ErrLlmInvalidResponse) {
		t.Fatalf("RunHighlightExtraction() error = %v, want errors.Is(err, ErrLlmInvalidResponse)", err)
	}
}
