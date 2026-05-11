package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/joho/godotenv"
)

// Sample payload captured from conversations.history (trimmed from user message)
const sampleEventJSON = `
{
  "user": "U0317LHBF7U",
  "type": "message",
  "ts": "1762234433.970879",
  "bot_id": "B031A00QFEG",
  "app_id": "A0300BS6CJ2",
  "text": "エラーが発生しています。\n日時:2025/11/04 14:21:50\nドメイン名:プラス少額短期保険株式会社\nエラー概要:sttSync: Google音声認識処理がタイムアウトにより最大リトライ回数に達しました\nエラー詳細:context deadline exceeded\nリンク:<https://mobi-work.slack.com/archives/CJT9BGWEA/p1762233710545119>",
  "team": "TGK1ZJQ8P",
  "bot_profile": {
    "id": "B031A00QFEG",
    "app_id": "A0300BS6CJ2",
    "user_id": "U0317LHBF7U",
    "name": "MBV商用エラー通知bot",
    "icons": {
      "image_36": "https://avatars.slack-edge.com/2022-02-14/3084991790119_6c3e0bfa89f6d03c44d0_36.png",
      "image_48": "https://avatars.slack-edge.com/2022-02-14/3084991790119_6c3e0bfa89f6d03c44d0_48.png",
      "image_72": "https://avatars.slack-edge.com/2022-02-14/3084991790119_6c3e0bfa89f6d03c44d0_72.png"
    },
    "deleted": false,
    "updated": 1644829352,
    "team_id": "TGK1ZJQ8P"
  },
  "attachments": [
    {
      "id": 1,
      "color": "a30200",
      "original_url": "https://mobi-work.slack.com/archives/CJT9BGWEA/p1762233710545119",
      "fallback": "sttSync: Google音声認識処理がタイムアウトにより最大リトライ回数に達しました",
      "text": "Message fields",
      "pretext": "sttSync: Google音声認識処理がタイムアウトにより最大リトライ回数に達しました",
      "fields": [
        {"value": "3080", "title": "durationMs", "short": true},
        {"value": "", "title": "scenarioHexID", "short": true},
        {"value": "1762233285.24069", "title": "uniqueCallID", "short": true},
        {"value": "0120-786-765", "title": "from", "short": true},
        {"value": "flow", "title": "botType", "short": true},
        {"value": "/go/src/bitbucket.org/mobilusall/mbv-agi/voices/stt/providers/google/speech.go:346 (*GoogleSpeech).sttSync", "title": "caller", "short": false},
        {"value": "1077925", "title": "domainID", "short": true},
        {"value": "3080", "title": "timeOutMs", "short": true},
        {"value": "3", "title": "timeOutCount", "short": true},
        {"value": "プラス少額短期保険株式会社", "title": "domainName", "short": false},
        {"value": "false", "title": "isOutbound", "short": true},
        {"value": "context deadline exceeded", "title": "error", "short": false},
        {"value": "/go/src/bitbucket.org/mobilusall/mbv-agi/voices/stt/providers/google/speech.go:346   (*GoogleSpeech).sttSync\n/go/src/bitbucket.org/mobilusall/mbv-agi/voices/stt/providers/google/speech.go:264   (*GoogleSpeech).SpeechToText\n/go/src/bitbucket.org/mobilusall/mbv-agi/voices/factory.go:462                       (*STT).SpeechToText\n/go/src/bitbucket.org/mobilusall/mbv-agi/voices/factory.go:441                       (*STT).ListenWithFile\n/go/src/bitbucket.org/mobilusall/mbv-agi/bots/interface.go:254                       (*VoiceSupportingBotBase).Listen\n/go/src/bitbucket.org/mobilusall/mbv-agi/types/inputType.go:29                       InputType.IsInputButton\n/go/src/bitbucket.org/mobilusall/mbv-agi/bots/interface.go:2655                      (*VoiceSupportingBotBase).ProcessQuestions\n/go/src/bitbucket.org/mobilusall/mbv-agi/use_cases/call_flows/call_flow_voice.go:259 (*CallFlowVoiceUseCase).CallFlowQuestions\n/go/src/bitbucket.org/mobilusall/mbv-agi/use_cases/call_flows/call_flow_voice.go:159 (*CallFlowVoiceUseCase).Exec\n/go/src/bitbucket.org/mobilusall/mbv-agi/use_cases/call_flows/call_flow.go:646       (*CallFlowUseCase).ScenarioExec\n/go/src/bitbucket.org/mobilusall/mbv-agi/use_cases/call_flows/call_flow.go:213       (*CallFlowUseCase).BranchExec\n/go/src/bitbucket.org/mobilusall/mbv-agi/use_cases/call_flows/call_flow.go:178       (*CallFlowUseCase).Exec\n/go/src/bitbucket.org/mobilusall/mbv-agi/controllers/inbound.go:256                  (*InboundController).InboundBot\n/go/src/bitbucket.org/mobilusall/mbv-agi/main.go:194                                 mobivoice\n/go/src/bitbucket.org/mobilusall/mbv-agi/main.go:150                                 handler\n/usr/local/go/src/runtime/asm_amd64.s:1700                                           goexit",
          "title": "stack",
          "short": false
        },
        {"value": "67105f23cda49cd12139061b", "title": "callFlowID", "short": false},
        {"value": "20251104141446-AOSAoc", "title": "receiptNumber", "short": false},
        {"value": "<tel:050-3205-1017|050-3205-1017>", "title": "to", "short": true}
      ]
    }
  ],
  "blocks": [
    {
      "type": "rich_text",
      "block_id": "Qro",
      "elements": [
        {
          "type": "rich_text_section",
          "elements": [
            {"type": "text", "text": "エラーが発生しています。\n日時:2025/11/04 14:21:50\nドメイン名:プラス少額短期保険株式会社\nエラー概要"},
            {"type": "emoji", "name": "sttSync"},
            {"type": "text", "text": " Google音声認識処理がタイムアウトにより最大リトライ回数に達しました\nエラー詳細:context deadline exceeded\nリンク:"},
            {"type": "link", "url": "https://mobi-work.slack.com/archives/CJT9BGWEA/p1762233710545119"}
          ]
        }
      ]
    }
  ]
}
`

func TestExtractFullSlackMessage_ExamplePayload(t *testing.T) {
	var event map[string]interface{}
	if err := json.Unmarshal([]byte(sampleEventJSON), &event); err != nil {
		t.Fatalf("failed to unmarshal sample event: %v", err)
	}

	got := extractFullSlackMessage(event)
	fmt.Println("Extracted Text:\n", got)
	if strings.TrimSpace(got) == "" {
		t.Fatalf("extracted text is empty")
	}

	// Does it contain the stacktrace from block fields?
	checks := []string{
		"/go/src/bitbucket.org/mobilusall/mbv-agi/use_cases/call_flows/call_flow.go:178",
	}
	for _, want := range checks {
		if !strings.Contains(got, want) {
			t.Errorf("extracted text missing expected fragment: %q\n--- got ---\n%s\n------------", want, got)
		}
	}

	// Ensure cap works (should be comfortably under 20k for this sample)
	if len(got) > 20000 {
		t.Errorf("extracted text too long: %d bytes", len(got))
	}
}

// TestDigDeeper_WithSampleEvent tests the full dig-deeper pipeline using the
// real sample event payload. It simulates what happens when the button is clicked:
// extractFullSlackMessage → parseFileRefs → fetchBitbucketContext → explainErrorWithContext
func TestDigDeeper_WithSampleEvent(t *testing.T) {
	// 1. Parse the sample event (same data that conversations.replies would return)
	var event map[string]any
	if err := json.Unmarshal([]byte(sampleEventJSON), &event); err != nil {
		t.Fatalf("failed to unmarshal sample event: %v", err)
	}

	// 2. Extract full message (this is what getParentMessageFull + extractFullSlackMessage does)
	errorText := extractFullSlackMessage(event)
	if errorText == "" {
		t.Fatal("extractFullSlackMessage returned empty string")
	}

	// 3. Parse file references — should find bitbucket.org paths from the stack trace
	refs := parseFileRefs(errorText, maxFileRefs)
	t.Logf("parseFileRefs found %d refs", len(refs))
	if len(refs) == 0 {
		t.Fatalf("parseFileRefs returned 0 refs from sample event.\nExtracted text:\n%s", errorText)
	}

	// Verify the refs look correct
	for i, ref := range refs {
		t.Logf("  ref[%d]: %s/%s/%s:%d", i, ref.Workspace, ref.RepoSlug, ref.Path, ref.Line)
		if ref.Workspace != "mobilusall" {
			t.Errorf("ref[%d] workspace = %q, want %q", i, ref.Workspace, "mobilusall")
		}
		if ref.RepoSlug != "mbv-agi" {
			t.Errorf("ref[%d] repoSlug = %q, want %q", i, ref.RepoSlug, "mbv-agi")
		}
		if ref.Line <= 0 {
			t.Errorf("ref[%d] line = %d, want > 0", i, ref.Line)
		}
	}

	// 4. Mock Bitbucket API — return fake Go source code
	bbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("  Bitbucket request: %s", r.URL.Path)
		// Generate fake source code with enough lines
		var lines []string
		for i := 1; i <= 500; i++ {
			lines = append(lines, fmt.Sprintf("// line %d: func doSomething() {}", i))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(strings.Join(lines, "\n")))
	}))
	defer bbServer.Close()

	oldBB := bitbucketBaseURL
	bitbucketBaseURL = bbServer.URL
	defer func() { bitbucketBaseURL = oldBB }()

	// 5. Fetch code context from mocked Bitbucket
	codeContext := fetchBitbucketContext(refs, maxContextChars)
	if codeContext == "" {
		t.Fatal("fetchBitbucketContext returned empty string")
	}
	t.Logf("fetchBitbucketContext returned %d chars", len(codeContext))

	if len(codeContext) > maxContextChars {
		t.Errorf("code context exceeds limit: %d > %d", len(codeContext), maxContextChars)
	}

	// 6. Mock OpenAI API — verify it receives both error and code context
	var aiMessages []any
	aiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		aiMessages, _ = body["messages"].([]any)
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "*【根本原因】*\n• `sttSync`でcontextのdeadlineを超過"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer aiServer.Close()

	oldAI := openaiBaseURL
	openaiBaseURL = aiServer.URL + "/v1"
	defer func() { openaiBaseURL = oldAI }()

	os.Setenv("AI_PROVIDER", "openai")
	os.Setenv("OPENAI_API_KEY", "test-key")
	defer os.Unsetenv("AI_PROVIDER")
	defer os.Unsetenv("OPENAI_API_KEY")

	// 7. Call explainErrorWithContext
	result := explainErrorWithContext(errorText, codeContext)
	if result == "" {
		t.Fatal("explainErrorWithContext returned empty string")
	}
	t.Logf("AI response: %s", result)

	// 8. Verify OpenAI received 3 messages: system prompt, error text, code context
	if len(aiMessages) != 3 {
		t.Fatalf("expected 3 messages to AI, got %d", len(aiMessages))
	}

	// System prompt should be the deep analysis one
	msg0, _ := aiMessages[0].(map[string]any)
	content0, _ := msg0["content"].(string)
	if !strings.Contains(content0, "根本原因") {
		t.Error("first message should be the deep analysis system prompt")
	}

	// User message should contain the error text
	msg1, _ := aiMessages[1].(map[string]any)
	content1, _ := msg1["content"].(string)
	if !strings.Contains(content1, "bitbucket.org/mobilusall/mbv-agi") {
		t.Error("user message should contain the original error with file paths")
	}

	// Code context message should contain fetched source
	msg2, _ := aiMessages[2].(map[string]any)
	content2, _ := msg2["content"].(string)
	if !strings.Contains(content2, "line") {
		t.Error("code context message should contain fetched source code")
	}

	t.Log("✓ Full dig-deeper pipeline works end-to-end with sample event data")
}

// TestDigDeeper_LiveEndToEnd tests the full dig-deeper pipeline against real
// Bitbucket and AI APIs using the sample event payload. Skips if credentials
// are not configured in .env.
//
// Run with: go test -run TestDigDeeper_LiveEndToEnd -v ./...
func TestDigDeeper_LiveEndToEnd(t *testing.T) {
	// Load .env for credentials
	_ = godotenv.Load()

	// Skip if Bitbucket not configured
	if !bitbucketConfigured() {
		t.Skip("Skipping: BITBUCKET_APP_PASSWORD not set (set in .env to run this test)")
	}
	// Skip if AI not configured
	provider := os.Getenv("AI_PROVIDER")
	if provider == "" {
		provider = "openai"
	}
	switch provider {
	case "openai":
		if os.Getenv("OPENAI_API_KEY") == "" {
			t.Skip("Skipping: OPENAI_API_KEY not set")
		}
	case "gemini":
		if os.Getenv("GEMINI_API_KEY") == "" {
			t.Skip("Skipping: GEMINI_API_KEY not set")
		}
	}

	// 1. Parse sample event
	var event map[string]any
	if err := json.Unmarshal([]byte(sampleEventJSON), &event); err != nil {
		t.Fatalf("failed to unmarshal sample event: %v", err)
	}

	// 2. Extract full message
	errorText := extractFullSlackMessage(event)
	t.Logf("Extracted message length: %d chars", len(errorText))
	if errorText == "" {
		t.Fatal("extractFullSlackMessage returned empty")
	}

	// 3. Parse file references
	refs := parseFileRefs(errorText, maxFileRefs)
	t.Logf("Found %d file references:", len(refs))
	for i, ref := range refs {
		t.Logf("  [%d] %s/%s/%s:%d", i, ref.Workspace, ref.RepoSlug, ref.Path, ref.Line)
	}
	if len(refs) == 0 {
		t.Fatal("No file references found")
	}

	// 4. Fetch code from REAL Bitbucket API
	t.Log("Fetching code from Bitbucket Cloud API...")
	codeContext := fetchBitbucketContext(refs, maxContextChars)
	t.Logf("Code context: %d chars", len(codeContext))
	if codeContext == "" {
		t.Fatal("fetchBitbucketContext returned empty — check credentials and repo access")
	}

	// Print a preview of what was fetched
	preview := codeContext
	if len(preview) > 1000 {
		preview = preview[:1000] + "\n... (truncated)"
	}
	t.Logf("Code context preview:\n%s", preview)

	// 5. Call REAL AI provider
	t.Logf("Calling %s for deep analysis...", provider)
	analysis := explainErrorWithContext(errorText, codeContext)
	if analysis == "" {
		t.Fatal("explainErrorWithContext returned empty")
	}
	if strings.HasPrefix(analysis, "Error") {
		t.Fatalf("AI returned error: %s", analysis)
	}

	t.Logf("\n=== AI Deep Analysis Result ===\n%s\n===============================", analysis)
	t.Log("✓ Full live end-to-end pipeline completed successfully")
}

// TestBitbucket_SingleFetch is a diagnostic test to check if Bitbucket API
// access works with current credentials. Run with:
//   go test -run TestBitbucket_SingleFetch -v ./...
func TestBitbucket_SingleFetch(t *testing.T) {
	_ = godotenv.Load()

	if !bitbucketConfigured() {
		t.Skip("Skipping: BITBUCKET_APP_PASSWORD not set")
	}

	t.Logf("BITBUCKET_USERNAME: %s", os.Getenv("BITBUCKET_USERNAME"))
	t.Logf("BITBUCKET_BRANCH: %s (default: master)", os.Getenv("BITBUCKET_BRANCH"))

	workspace := "mobilusall"
	repo := "mbv-agi"
	path := "main.go"

	// Try multiple branch names to find the right one
	branches := []string{os.Getenv("BITBUCKET_BRANCH"), "master", "main", "develop"}
	for _, branch := range branches {
		if branch == "" {
			continue
		}
		os.Setenv("BITBUCKET_BRANCH", branch)
		t.Logf("Trying branch %q: %s/%s/%s", branch, workspace, repo, path)
		content, err := fetchFileFromBitbucket(workspace, repo, path)
		if err != nil {
			t.Logf("  ✗ %v", err)
			continue
		}
		t.Logf("  ✓ Success with branch %q! Got %d bytes", branch, len(content))
		preview := content
		if len(preview) > 300 {
			preview = preview[:300] + "\n..."
		}
		t.Logf("Preview:\n%s", preview)
		return
	}
	t.Fatal("All branch attempts failed. Check workspace/repo name and permissions.")
}
