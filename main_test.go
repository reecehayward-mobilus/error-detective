package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
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
