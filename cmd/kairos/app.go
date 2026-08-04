package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"kairos/internal/core"
	"kairos/internal/history"
	"kairos/internal/llm"
	"kairos/internal/video"
)

// acceptedExtensions is the set of source video formats spec.md user story 6
// requires supporting.
var acceptedExtensions = []string{".mp4", ".mov", ".avi", ".webm"}

// stageLabel maps a core.Stage to the exact 4-phase progress text ticket 08
// asks for: "音轨提取中→识别台词中→判定高光中→剪辑中".
var stageProgressValue = map[core.Stage]float64{
	core.StageExtractingAudio: 0.15,
	core.StageTranscribing:    0.45,
	core.StageJudging:         0.70,
	core.StageCutting:         0.90,
}

// App holds all GUI state for Kairos's single window.
type App struct {
	fyneApp fyne.App
	win     fyne.Window

	newTranscriber func() core.Transcriber // production wiring, platform-split (transcriber_windows.go / transcriber_other.go)
	transcriber    core.Transcriber        // lazily constructed once, reused across runs (see getTranscriber)

	dropLabel     *widget.Label
	pathLabel     *widget.Label
	outputSection *fyne.Container // hidden until a video is selected (spec.md 用户故事 5)
	outputEntry   *widget.Entry
	statusLabel   *widget.Label
	progress      *widget.ProgressBar
	cudaLabel     *widget.Label
	historyList   *widget.List

	videoPath  string
	processing bool
	records    []history.Record
}

func newApp(fyneApp fyne.App) *App {
	a := &App{
		fyneApp:        fyneApp,
		newTranscriber: newProductionTranscriber,
	}
	a.win = fyneApp.NewWindow("Kairos — 短剧高光片段提取器")
	a.win.SetOnDropped(a.onDropped)
	a.build()
	return a
}

// isVideoFile checks path's extension against spec.md's supported formats
// (MP4/MOV/AVI/WEBM), case-insensitively.
func isVideoFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, accepted := range acceptedExtensions {
		if ext == accepted {
			return true
		}
	}
	return false
}

func (a *App) build() {
	a.dropLabel = widget.NewLabel("拖入视频文件，或点击下方按钮选择")
	browseBtn := widget.NewButton("选择视频文件…", a.browseForFile)
	a.pathLabel = widget.NewLabel("")

	a.outputEntry = widget.NewEntry()
	a.outputEntry.PlaceHolder = "输出目录（默认与源文件同目录）"
	changeBtn := widget.NewButton("更改…", a.browseForOutputDir)
	// "使用此目录" 而不是"开始处理"——这个按钮代表"确定输出目录"这个选择动作
	// 本身，不是一个独立于该选择之外的"开始"按钮（ticket 08：「没有独立的
	// '开始'按钮——第 2 步的选择动作本身就是触发点」），跟"更改…"打开
	// dialog.ShowFolderOpen() 完成后自动触发处理是同一类动作，只是默认值
	// 这条路径需要一次点击来表达"确定用这个"。
	confirmBtn := widget.NewButton("使用此目录", a.startProcessing)
	outputRow := container.NewBorder(nil, nil, nil, container.NewHBox(changeBtn, confirmBtn), a.outputEntry)
	a.outputSection = container.NewVBox(widget.NewLabel("输出目录："), outputRow)
	a.outputSection.Hide() // 选定输入文件后才出现（spec.md 用户故事 5）

	a.statusLabel = widget.NewLabel("")
	a.progress = widget.NewProgressBar()
	a.progress.Hide()

	a.cudaLabel = widget.NewLabel(cudaStatusText())

	a.historyList = widget.NewList(
		func() int { return len(a.records) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < 0 || id >= len(a.records) {
				return
			}
			obj.(*widget.Label).SetText(historyRowText(a.records[id]))
		},
	)
	a.historyList.OnSelected = func(id widget.ListItemID) {
		if id >= 0 && id < len(a.records) {
			a.showHistoryDetail(a.records[id])
		}
	}

	top := container.NewVBox(
		a.cudaLabel,
		a.dropLabel,
		browseBtn,
		a.pathLabel,
		widget.NewSeparator(),
		a.outputSection,
		a.statusLabel,
		a.progress,
		widget.NewSeparator(),
		widget.NewLabel("历史记录"),
	)

	content := container.NewBorder(top, nil, nil, nil, container.NewVScroll(a.historyList))
	a.win.SetContent(content)
	a.win.Resize(fyne.NewSize(640, 760))

	a.refreshHistory()
}

// cudaStatusText is an informational (non-blocking) notice about hardware
// acceleration — CUDA unavailable is not an error condition, video/asr
// already degrade to CPU/libx264 transparently, so this is a status line,
// not a dialog (ticket 08's "CUDA 不可用...均有明确错误提示" is satisfied by
// always showing this, not by blocking processing on it).
func cudaStatusText() string {
	if video.CudaAvailable() {
		return "已检测到 NVIDIA 显卡加速"
	}
	return "未检测到可用的 NVIDIA 显卡，将使用 CPU 处理（速度较慢）"
}

func (a *App) onDropped(_ fyne.Position, uris []fyne.URI) {
	if len(uris) == 0 {
		return
	}
	a.selectVideo(uris[0].Path())
}

func (a *App) browseForFile() {
	fd := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, a.win)
			return
		}
		if r == nil {
			return // user cancelled
		}
		defer r.Close()
		a.selectVideo(r.URI().Path())
	}, a.win)
	fd.SetFilter(storage.NewExtensionFileFilter(acceptedExtensions))
	fd.Show()
}

// selectVideo is the single entry point both drag-drop and the file dialog
// funnel through (implementation-plan.md：「拖放和点击选择走同一个内部事件，
// 逻辑不分叉」).
func (a *App) selectVideo(path string) {
	if !isVideoFile(path) {
		dialog.ShowInformation("不支持的文件类型", "请选择 MP4/MOV/AVI/WEBM 格式的视频文件。", a.win)
		return
	}
	a.videoPath = path
	a.pathLabel.SetText("已选择：" + filepath.Base(path))
	a.outputEntry.SetText(filepath.Dir(path))
	a.outputSection.Show()
	a.statusLabel.SetText("")
}

func (a *App) browseForOutputDir() {
	dialog.ShowFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil {
			dialog.ShowError(err, a.win)
			return
		}
		if uri == nil {
			return // user cancelled
		}
		a.outputEntry.SetText(uri.Path())
		a.startProcessing() // completing the folder picker IS the confirmation
	}, a.win)
}

// startProcessing is the sole trigger point for RunHighlightExtraction —
// reached either by clicking "使用此目录" (accepting whatever is currently
// in outputEntry, which defaults to the source directory) or by completing
// the folder-change dialog. There is no separate "开始" button beyond this
// (ticket 08).
func (a *App) startProcessing() {
	if a.processing {
		return
	}
	if a.videoPath == "" {
		dialog.ShowInformation("请先选择视频文件", "还没有选择要处理的视频文件。", a.win)
		return
	}

	apiKey, keyErr := llm.LoadAPIKey()
	if keyErr != nil {
		dialog.ShowInformation("需要配置 API Key", "请先设置 DeepSeek API Key 才能开始处理。", a.win)
		a.promptForAPIKey(nil)
		return
	}

	videoPath := a.videoPath
	outputDir := strings.TrimSpace(a.outputEntry.Text)
	if outputDir == "" {
		outputDir = filepath.Dir(videoPath)
	}
	ext := filepath.Ext(videoPath)
	base := strings.TrimSuffix(filepath.Base(videoPath), ext)
	outputPath := filepath.Join(outputDir, base+"_highlight.mp4")

	a.processing = true
	a.progress.Show()
	a.progress.SetValue(0)
	a.statusLabel.SetText(string(core.StageExtractingAudio))

	judge := llm.NewDeepSeekJudge(apiKey)
	transcriber := a.getTranscriber()

	stageStart := time.Now()
	runStart := stageStart
	cfg := core.Config{
		OutputPath: outputPath,
		OnProgress: func(stage core.Stage) {
			now := time.Now()
			slog.Info("kairos: stage started", "stage", stage, "since_previous", now.Sub(stageStart))
			stageStart = now
			fyne.Do(func() {
				a.statusLabel.SetText(string(stage))
				if v, ok := stageProgressValue[stage]; ok {
					a.progress.SetValue(v)
				}
			})
		},
	}

	go a.runExtraction(videoPath, cfg, transcriber, judge, runStart)
}

// runExtraction runs RunHighlightExtraction on a background goroutine with a
// panic-recover safety net (implementation-plan.md「Panic 兜底」) and marshals
// all UI updates back onto the Fyne main thread via fyne.Do.
func (a *App) runExtraction(videoPath string, cfg core.Config, transcriber core.Transcriber, judge core.HighlightJudge, runStart time.Time) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("kairos: panic during RunHighlightExtraction", "recover", r, "stack", string(debug.Stack()))
			fyne.Do(func() {
				a.processing = false
				a.progress.Hide()
				a.statusLabel.SetText("处理异常")
				dialog.ShowInformation("处理异常", "处理过程中出现未预期的错误，请查看日志。", a.win)
			})
		}
	}()

	output, err := core.RunHighlightExtraction(videoPath, cfg, transcriber, judge)
	elapsed := time.Since(runStart)

	fyne.Do(func() {
		a.processing = false
		a.progress.Hide()
		a.refreshHistory()

		if err != nil {
			slog.Warn("kairos: RunHighlightExtraction failed", "source", videoPath, "elapsed", elapsed, "error", err)
			a.statusLabel.SetText("处理失败")
			dialog.ShowInformation("处理失败", userMessage(err), a.win)
			return
		}

		slog.Info("kairos: RunHighlightExtraction succeeded", "source", videoPath, "output", output.OutputPath, "elapsed", elapsed)
		a.statusLabel.SetText("完成：" + output.OutputPath)
		if openErr := openPreview(output.OutputPath); openErr != nil {
			slog.Warn("kairos: failed to launch preview player", "path", output.OutputPath, "error", openErr)
			dialog.ShowInformation("已完成", "高光片段已生成："+output.OutputPath, a.win)
		}
	})
}

// openPreview launches the OS's default video player on path — Fyne has no
// built-in video-playback widget, so "自动弹出预览窗口播放高光片段" (ticket
// 08) is satisfied by handing off to the system's own player in its own
// window, rather than embedding a player in-app.
//
// ASSUMPTION: ticket 08 doesn't mandate an embedded player, just "弹出预览
// 窗口播放" — interpreting that as "hand off to the system default player"
// since Fyne has no video widget to build one with; flag if a truly embedded
// preview is required instead.
func openPreview(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// 直接调 explorer.exe 打开文件本身的默认关联程序，不走 cmd.exe：
		// exec.Command 不经过 shell，参数按原样传给 CreateProcess，避免
		// 之前 `cmd /c start "" path` 里 cmd.exe 自己的元字符解析
		// （&、%、^、|、<、>）在文件名恰好包含这些字符时把命令解析错——
		// 预检查发现的问题，本机是 macOS 走不到这条分支，无法直接跑
		// TestOpenPreview_BuildsCommand 之外的真实验证。
		cmd = exec.Command("explorer", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}

// closeTranscriber releases the cached ASR model/GPU session on app
// shutdown. core.Transcriber doesn't declare a Close() method (not every
// implementation needs one — the mock/unsupported-platform ones don't hold
// any real resource), so this uses an optional-interface type assertion to
// call it only when the concrete type actually has one (production's
// *asr.ParaformerTranscriber does).
func (a *App) closeTranscriber() {
	if closer, ok := a.transcriber.(interface{ Close() }); ok {
		closer.Close()
	}
}

// getTranscriber returns the cached transcriber, constructing it via
// newTranscriber on first use — see the App.transcriber field's doc comment
// for why this is cached rather than rebuilt per run. Extracted as its own
// method (rather than inlined in startProcessing) so the caching behavior is
// directly unit-testable without going through the full async processing
// flow.
func (a *App) getTranscriber() core.Transcriber {
	if a.transcriber == nil {
		a.transcriber = a.newTranscriber()
	}
	return a.transcriber
}

// promptForAPIKey shows the first-run / missing-key onboarding dialog
// (ticket 08：「首次运行引导用户输入 DeepSeek API Key，存本地配置文件」).
// onSaved (may be nil) runs after a successful save.
func (a *App) promptForAPIKey(onSaved func()) {
	entry := widget.NewPasswordEntry()
	entry.Validator = func(s string) error {
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("API Key 不能为空")
		}
		return nil
	}
	item := widget.NewFormItem("DeepSeek API Key", entry)
	dialog.ShowForm("设置 DeepSeek API Key", "保存", "取消", []*widget.FormItem{item}, func(ok bool) {
		if !ok {
			return
		}
		if err := llm.SaveAPIKey(entry.Text); err != nil {
			dialog.ShowError(err, a.win)
			return
		}
		if onSaved != nil {
			onSaved()
		}
	}, a.win)
}

// refreshHistory reloads the history list from disk (ticket 08：「历史列表
// 能查看过往记录的转写文本和判定理由」).
func (a *App) refreshHistory() {
	records, err := history.ListRecords()
	if err != nil {
		slog.Warn("kairos: failed to list history records", "error", err)
		return
	}
	a.records = records
	a.historyList.Refresh()
}

// historyRowText renders one history.Record as a single list-row line.
func historyRowText(rec history.Record) string {
	status := "成功"
	if rec.Status != "success" {
		status = "失败"
	}
	return fmt.Sprintf("%s · %s · %s", rec.CreatedAt.Format("2006-01-02 15:04"), rec.SourceName, status)
}

// showHistoryDetail opens a dialog showing the full ASR transcript and LLM
// judging reason for one record (spec.md 用户故事 20/21).
func (a *App) showHistoryDetail(rec history.Record) {
	body := fmt.Sprintf("转写文本：\n%s\n\n判定理由：\n%s", formatTranscript(rec.ASRRawResult), formatJudgeReason(rec.LLMRawResponse))
	if rec.Status != "success" && rec.ErrorMessage != "" {
		body = "错误信息：\n" + rec.ErrorMessage + "\n\n" + body
	}
	text := widget.NewLabel(body)
	text.Wrapping = fyne.TextWrapWord
	scroll := container.NewVScroll(text)
	scroll.SetMinSize(fyne.NewSize(480, 360))
	dialog.ShowCustom(rec.SourceName, "关闭", scroll, a.win)
}

// formatTranscript decodes a history.Record.ASRRawResult (json.Marshal of
// []core.Sentence, written by internal/core's writeHistoryRecord) back into
// plain readable dialogue lines.
func formatTranscript(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "（无转写记录）"
	}
	var sentences []core.Sentence
	if err := json.Unmarshal(raw, &sentences); err != nil {
		return "（转写记录解析失败）"
	}
	if len(sentences) == 0 {
		return "（无转写记录）"
	}
	var b strings.Builder
	for _, s := range sentences {
		b.WriteString(s.Text)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// formatJudgeReason decodes a history.Record.LLMRawResponse (json.Marshal of
// core.HighlightWindow) back into the LLM's judging reason text.
func formatJudgeReason(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "（无判定记录）"
	}
	var window core.HighlightWindow
	if err := json.Unmarshal(raw, &window); err != nil {
		return "（判定记录解析失败）"
	}
	if window.Reason == "" {
		return "（无判定理由）"
	}
	return window.Reason
}
