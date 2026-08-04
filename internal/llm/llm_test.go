package llm

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"

	"kairos/internal/core"
)

// newTestJudge 起一个 httptest.Server 顶替真实 DeepSeek 端点，返回指向它的 DeepSeekJudge。
func newTestJudge(t *testing.T, handler http.HandlerFunc) *DeepSeekJudge {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	cfg := openai.DefaultConfig("test-key")
	cfg.BaseURL = server.URL
	return newDeepSeekJudge(cfg)
}

// chatCompletionBody 把 content 包成一个合法的 openai.ChatCompletionResponse JSON 报文。
func chatCompletionBody(content string) []byte {
	resp := openai.ChatCompletionResponse{
		ID:      "chatcmpl-test",
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   deepSeekModel,
		Choices: []openai.ChatCompletionChoice{
			{
				Index:        0,
				Message:      openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: content},
				FinishReason: openai.FinishReasonStop,
			},
		},
	}
	b, err := json.Marshal(resp)
	if err != nil {
		panic(err)
	}
	return b
}

var testSentences = []core.Sentence{
	{ID: 1, StartMs: 0, EndMs: 1000, Text: "从前有座山"},
	{ID: 2, StartMs: 1000, EndMs: 2000, Text: "山里有座庙"},
}

func TestDeepSeekJudge_Judge(t *testing.T) {
	t.Run("valid response populates HighlightWindow", func(t *testing.T) {
		wireJSON := `{
			"narrative_structure": {
				"setup": [1, 2],
				"rising_action": [3, 4],
				"climax": [5, 6],
				"resolution": [7, 8]
			},
			"start_id": 5,
			"end_id": 6,
			"reason": "冲突在此刻爆发，路人观众能立刻抓住冲击点",
			"candidate_sentences": [
				{"start_id": 1, "end_id": 2, "label": "备选窗口一"},
				{"start_id": 3, "end_id": 4, "label": "备选窗口二"}
			]
		}`

		var gotReq openai.ChatCompletionRequest
		judge := newTestJudge(t, func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write(chatCompletionBody(wireJSON))
		})

		got, err := judge.Judge(testSentences)
		if err != nil {
			t.Fatalf("Judge() error = %v", err)
		}

		want := core.HighlightWindow{
			NarrativeStructure: core.NarrativeStructure{
				Setup:        [2]int{1, 2},
				RisingAction: [2]int{3, 4},
				Climax:       [2]int{5, 6},
				Resolution:   &[2]int{7, 8},
			},
			StartID: 5,
			EndID:   6,
			Reason:  "冲突在此刻爆发，路人观众能立刻抓住冲击点",
			Candidates: []core.CandidateWindow{
				{StartID: 1, EndID: 2, Label: "备选窗口一"},
				{StartID: 3, EndID: 4, Label: "备选窗口二"},
			},
		}

		if got.StartID != want.StartID || got.EndID != want.EndID || got.Reason != want.Reason {
			t.Errorf("Judge() = %+v, want %+v", got, want)
		}
		if got.NarrativeStructure.Resolution == nil || *got.NarrativeStructure.Resolution != *want.NarrativeStructure.Resolution {
			t.Errorf("NarrativeStructure.Resolution = %v, want %v", got.NarrativeStructure.Resolution, want.NarrativeStructure.Resolution)
		}
		if got.NarrativeStructure.Setup != want.NarrativeStructure.Setup ||
			got.NarrativeStructure.RisingAction != want.NarrativeStructure.RisingAction ||
			got.NarrativeStructure.Climax != want.NarrativeStructure.Climax {
			t.Errorf("NarrativeStructure = %+v, want %+v", got.NarrativeStructure, want.NarrativeStructure)
		}
		if !reflect.DeepEqual(got.Candidates, want.Candidates) {
			t.Errorf("Candidates = %+v, want %+v", got.Candidates, want.Candidates)
		}

		// system prompt 与 user message 都应正确送达，验证请求构造本身。
		if len(gotReq.Messages) != 2 {
			t.Fatalf("request has %d messages, want 2", len(gotReq.Messages))
		}
		wantSystem := systemPrompt + "\n\n" + jsonShapeInstruction
		if gotReq.Messages[0].Role != openai.ChatMessageRoleSystem || gotReq.Messages[0].Content != wantSystem {
			t.Errorf("system message = %+v, want fixed systemPrompt + jsonShapeInstruction", gotReq.Messages[0])
		}
		// DeepSeek 要求 response_format=json_object 时 prompt 里必须出现
		// "json" 字样，否则直接 400（2026-08-03 真实 API 调用时发现）。
		if !strings.Contains(strings.ToLower(gotReq.Messages[0].Content), "json") {
			t.Error("system message doesn't contain \"json\", DeepSeek's json_object response_format requires it")
		}
		wantUser := "id, start_ms, end_ms, text\n1, 0, 1000, 从前有座山\n2, 1000, 2000, 山里有座庙\n"
		if gotReq.Messages[1].Role != openai.ChatMessageRoleUser || gotReq.Messages[1].Content != wantUser {
			t.Errorf("user message = %q, want %q", gotReq.Messages[1].Content, wantUser)
		}
		if gotReq.ResponseFormat == nil || gotReq.ResponseFormat.Type != openai.ChatCompletionResponseFormatTypeJSONObject {
			t.Errorf("ResponseFormat = %+v, want json_object", gotReq.ResponseFormat)
		}
	})

	t.Run("resolution 为 nil 时保持 nil", func(t *testing.T) {
		wireJSON := `{
			"narrative_structure": {"setup": [1,2], "rising_action": [2,3], "climax": [3,4], "resolution": null},
			"start_id": 3, "end_id": 4, "reason": "无明显结局", "candidate_sentences": []
		}`
		judge := newTestJudge(t, func(w http.ResponseWriter, r *http.Request) {
			w.Write(chatCompletionBody(wireJSON))
		})

		got, err := judge.Judge(testSentences)
		if err != nil {
			t.Fatalf("Judge() error = %v", err)
		}
		if got.NarrativeStructure.Resolution != nil {
			t.Errorf("Resolution = %v, want nil", got.NarrativeStructure.Resolution)
		}
		if len(got.Candidates) != 0 {
			t.Errorf("Candidates = %v, want empty", got.Candidates)
		}
	})

	t.Run("字段类型不对时返回 unmarshal 类型错误", func(t *testing.T) {
		// start_id 本该是 integer，这里给成字符串，触发 encoding/json 原生类型检查。
		wireJSON := `{"start_id": "not-an-int", "end_id": 6, "reason": "x"}`
		judge := newTestJudge(t, func(w http.ResponseWriter, r *http.Request) {
			w.Write(chatCompletionBody(wireJSON))
		})

		_, err := judge.Judge(testSentences)
		if err == nil {
			t.Fatal("Judge() error = nil, want non-nil")
		}
		var typeErr *json.UnmarshalTypeError
		if !errors.As(err, &typeErr) {
			t.Errorf("Judge() error = %v (%T), want *json.UnmarshalTypeError in chain", err, err)
		}
	})

	t.Run("content 不是合法 JSON 时返回错误", func(t *testing.T) {
		judge := newTestJudge(t, func(w http.ResponseWriter, r *http.Request) {
			w.Write(chatCompletionBody("这不是 JSON"))
		})

		_, err := judge.Judge(testSentences)
		if err == nil {
			t.Fatal("Judge() error = nil, want non-nil")
		}
	})

	t.Run("超时返回 ErrLlmTimeout", func(t *testing.T) {
		judge := newTestJudge(t, func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-time.After(500 * time.Millisecond):
			case <-r.Context().Done():
			}
			w.Write(chatCompletionBody(`{"start_id":1,"end_id":2,"reason":"x"}`))
		})
		judge.timeout = 30 * time.Millisecond

		_, err := judge.Judge(testSentences)
		if !errors.Is(err, ErrLlmTimeout) {
			t.Errorf("Judge() error = %v, want ErrLlmTimeout", err)
		}
	})

	t.Run("API_Key_无效（401）返回_ErrInvalidAPIKey", func(t *testing.T) {
		judge := newTestJudge(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":{"message":"Incorrect API key provided","type":"invalid_request_error"}}`))
		})

		_, err := judge.Judge(testSentences)
		if !errors.Is(err, ErrInvalidAPIKey) {
			t.Errorf("Judge() error = %v, want ErrInvalidAPIKey", err)
		}
	})
}

func TestLoadAPIKey(t *testing.T) {
	t.Run("从配置目录正确读取 api_key", func(t *testing.T) {
		dir := t.TempDir()
		origUserConfigDir := userConfigDir
		userConfigDir = func() (string, error) { return dir, nil }
		t.Cleanup(func() { userConfigDir = origUserConfigDir })

		kairosDir := filepath.Join(dir, "kairos")
		if err := os.MkdirAll(kairosDir, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		cfgJSON := `{"deepseek": {"api_key": "sk-test-abc123"}}`
		if err := os.WriteFile(filepath.Join(kairosDir, "config.json"), []byte(cfgJSON), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		got, err := LoadAPIKey()
		if err != nil {
			t.Fatalf("LoadAPIKey() error = %v", err)
		}
		if got != "sk-test-abc123" {
			t.Errorf("LoadAPIKey() = %q, want %q", got, "sk-test-abc123")
		}
	})

	t.Run("配置文件不存在时报错", func(t *testing.T) {
		dir := t.TempDir()
		origUserConfigDir := userConfigDir
		userConfigDir = func() (string, error) { return dir, nil }
		t.Cleanup(func() { userConfigDir = origUserConfigDir })

		_, err := LoadAPIKey()
		if err == nil {
			t.Fatal("LoadAPIKey() error = nil, want non-nil")
		}
	})

	t.Run("api_key 为空时报错", func(t *testing.T) {
		dir := t.TempDir()
		origUserConfigDir := userConfigDir
		userConfigDir = func() (string, error) { return dir, nil }
		t.Cleanup(func() { userConfigDir = origUserConfigDir })

		kairosDir := filepath.Join(dir, "kairos")
		if err := os.MkdirAll(kairosDir, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(filepath.Join(kairosDir, "config.json"), []byte(`{"deepseek": {"api_key": ""}}`), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		_, err := LoadAPIKey()
		if err == nil {
			t.Fatal("LoadAPIKey() error = nil, want non-nil")
		}
	})
}

// withTempConfigDir 把 userConfigDir 换成一个测试临时目录，避免真的写进
// 这台机器的 %APPDATA%/kairos/。
func withTempConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := userConfigDir
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = orig })
	return dir
}

func TestSaveAPIKey(t *testing.T) {
	t.Run("写入全新配置文件，LoadAPIKey 能读回", func(t *testing.T) {
		withTempConfigDir(t)

		if err := SaveAPIKey("sk-new-key"); err != nil {
			t.Fatalf("SaveAPIKey() error = %v", err)
		}
		got, err := LoadAPIKey()
		if err != nil {
			t.Fatalf("LoadAPIKey() error = %v", err)
		}
		if got != "sk-new-key" {
			t.Errorf("LoadAPIKey() = %q, want %q", got, "sk-new-key")
		}
	})

	t.Run("不清空已有配置文件里其余字段", func(t *testing.T) {
		dir := withTempConfigDir(t)
		kairosDir := filepath.Join(dir, "kairos")
		if err := os.MkdirAll(kairosDir, 0o755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		existing := `{"deepseek":{"api_key":"sk-old","use_credential_manager":true},"output":{"default_dir":"/videos"}}`
		if err := os.WriteFile(filepath.Join(kairosDir, "config.json"), []byte(existing), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}

		if err := SaveAPIKey("sk-new"); err != nil {
			t.Fatalf("SaveAPIKey() error = %v", err)
		}

		raw, err := os.ReadFile(filepath.Join(kairosDir, "config.json"))
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}

		deepseek, _ := got["deepseek"].(map[string]any)
		if deepseek["api_key"] != "sk-new" {
			t.Errorf("deepseek.api_key = %v, want sk-new", deepseek["api_key"])
		}
		if deepseek["use_credential_manager"] != true {
			t.Errorf("deepseek.use_credential_manager = %v, want true (preserved)", deepseek["use_credential_manager"])
		}
		output, _ := got["output"].(map[string]any)
		if output["default_dir"] != "/videos" {
			t.Errorf("output.default_dir = %v, want /videos (preserved)", output["default_dir"])
		}
	})

	t.Run("空字符串报错", func(t *testing.T) {
		withTempConfigDir(t)
		if err := SaveAPIKey(""); err == nil {
			t.Fatal("SaveAPIKey(\"\") error = nil, want non-nil")
		}
		if err := SaveAPIKey("   "); err == nil {
			t.Fatal("SaveAPIKey(\"   \") error = nil, want non-nil")
		}
	})

	t.Run("重复保存覆盖为最新值", func(t *testing.T) {
		withTempConfigDir(t)
		if err := SaveAPIKey("sk-first"); err != nil {
			t.Fatalf("SaveAPIKey() error = %v", err)
		}
		if err := SaveAPIKey("sk-second"); err != nil {
			t.Fatalf("SaveAPIKey() error = %v", err)
		}
		got, err := LoadAPIKey()
		if err != nil {
			t.Fatalf("LoadAPIKey() error = %v", err)
		}
		if got != "sk-second" {
			t.Errorf("LoadAPIKey() = %q, want %q", got, "sk-second")
		}
	})
}
