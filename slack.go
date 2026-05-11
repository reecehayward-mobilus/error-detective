package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func verifySlackSignature(c *gin.Context, rawBody []byte) bool {
	secret := os.Getenv("SLACK_SIGNING_SECRET")
	if secret == "" {
		// If not set, skip verification (not recommended in prod)
		return true
	}
	ts := c.GetHeader("X-Slack-Request-Timestamp")
	sig := c.GetHeader("X-Slack-Signature")
	base := "v0:" + ts + ":" + string(rawBody)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(base))
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(sig))
}

func postToSlack(channel, threadTS, text string) {
	// Use Block Kit section with mrkdwn to ensure list/bold rendering
	fallback := text
	if len(fallback) > 300 {
		fallback = fallback[:300] + "…"
	}
	blocks := []map[string]any{
		{
			"type": "section",
			"text": map[string]any{
				"type": "mrkdwn",
				"text": text,
			},
		},
	}

	// Note: create-button is added by postToSlackWithCreateButton wrapper when configured.

	payload := map[string]any{
		"channel":         channel,
		"thread_ts":       threadTS,
		"text":            fallback,
		"blocks":          blocks,
		"unfurl_links":    false,
		"unfurl_media":    false,
		"reply_broadcast": false,
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", bytes.NewBuffer(body))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+os.Getenv("SLACK_BOT_TOKEN"))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("Error posting to Slack: %v\n", err)
		return
	}
	defer resp.Body.Close()

	var slackResponse map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&slackResponse); err != nil {
		fmt.Printf("Error decoding Slack response: %v\n", err)
		return
	}

	if ok, _ := slackResponse["ok"].(bool); !ok {
		fmt.Printf("Slack API Error: %s\n", slackResponse["error"])
	} else {
		fmt.Println("Successfully posted message to Slack.")
	}
}

// buildSlackPayloadWithButtons builds the Slack Block Kit payload for an explanation message.
// Returns the blocks array. Used by postToSlackWithCreateButton and tests.
func buildSlackPayloadWithButtons(explanation, token string, includeBitbucket bool) []map[string]any {
	elements := []map[string]any{
		{
			"type":      "button",
			"text":      map[string]string{"type": "plain_text", "text": "Backlogに起票"},
			"style":     "primary",
			"value":     token,
			"action_id": "create_backlog_ticket",
		},
	}

	if includeBitbucket {
		elements = append(elements, map[string]any{
			"type":      "button",
			"text":      map[string]string{"type": "plain_text", "text": "深掘りする"},
			"value":     token,
			"action_id": "dig_deeper_bitbucket",
		})
	}

	return []map[string]any{
		{
			"type": "section",
			"text": map[string]any{
				"type": "mrkdwn",
				"text": explanation,
			},
		},
		{
			"type":     "actions",
			"elements": elements,
		},
	}
}

// postToSlackWithCreateButton posts the explanation and appends an Actions block with a one-click Backlog button.
// When Bitbucket is configured, a "深掘りする" button is also included.
func postToSlackWithCreateButton(channel, threadTS, explanation, errorText string) {
	if !backlogConfigured() {
		postToSlack(channel, threadTS, explanation)
		return
	}
	// Create token and cache context
	tok, err := putTicketContext(ticketContext{Channel: channel, ThreadTS: threadTS, ErrorText: errorText, Explanation: explanation}, getBacklogTokenTTL())
	if err != nil {
		postToSlack(channel, threadTS, explanation)
		return
	}

	// Build payload with buttons
	fallback := explanation
	if len(fallback) > 300 {
		fallback = fallback[:300] + "…"
	}
	blocks := buildSlackPayloadWithButtons(explanation, tok, bitbucketConfigured())
	payload := map[string]any{
		"channel":      channel,
		"thread_ts":    threadTS,
		"text":         fallback,
		"blocks":       blocks,
		"unfurl_links": false,
		"unfurl_media": false,
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", bytes.NewBuffer(body))
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer "+os.Getenv("SLACK_BOT_TOKEN"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("Error posting to Slack: %v\n", err)
		return
	}
	defer resp.Body.Close()
	var slackResponse map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&slackResponse); err != nil {
		fmt.Printf("Error decoding Slack response: %v\n", err)
		return
	}
	if ok, _ := slackResponse["ok"].(bool); !ok {
		fmt.Printf("Slack API Error: %s\n", slackResponse["error"])
	}
}

// postToSlackSimple posts a simple mrkdwn message in thread without actions.
// Handles Slack's 3000-char section block limit by splitting into multiple blocks.
func postToSlackSimple(channel, threadTS, text string) {
	fallback := text
	if len(fallback) > 300 {
		fallback = fallback[:300] + "…"
	}

	// Slack section blocks have a 3000-char text limit.
	// Split long messages into multiple section blocks.
	const maxBlockText = 3000
	var blocks []map[string]any
	remaining := text
	for remaining != "" {
		chunk := remaining
		if len(chunk) > maxBlockText {
			// Try to split at a newline to avoid breaking mid-line
			cutAt := maxBlockText
			if idx := strings.LastIndex(chunk[:cutAt], "\n"); idx > cutAt/2 {
				cutAt = idx + 1
			}
			chunk = remaining[:cutAt]
			remaining = remaining[cutAt:]
		} else {
			remaining = ""
		}
		blocks = append(blocks, map[string]any{
			"type": "section",
			"text": map[string]any{"type": "mrkdwn", "text": chunk},
		})
	}

	payload := map[string]any{
		"channel":   channel,
		"thread_ts": threadTS,
		"text":      fallback,
		"blocks":    blocks,
	}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+os.Getenv("SLACK_BOT_TOKEN"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("[slack] postToSlackSimple error: %v\n", err)
		return
	}
	defer resp.Body.Close()
	var slackResp map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&slackResp); err != nil {
		fmt.Printf("[slack] decode error: %v\n", err)
		return
	}
	if ok, _ := slackResp["ok"].(bool); !ok {
		fmt.Printf("[slack] postToSlackSimple API error: %s\n", slackResp["error"])
	}
}

func getSlackPermalink(channel, ts string) (string, error) {
	reqBody := map[string]string{"channel": channel, "message_ts": ts}
	b, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "https://slack.com/api/chat.getPermalink", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+os.Getenv("SLACK_BOT_TOKEN"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		OK        bool   `json:"ok"`
		Permalink string `json:"permalink"`
		Error     string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if !out.OK {
		return "", fmt.Errorf("chat.getPermalink error: %s", out.Error)
	}
	return out.Permalink, nil
}

func getParentMessageText(channel, threadTS string) (string, error) {
	msg, err := getParentMessageFull(channel, threadTS)
	if err != nil {
		return "", err
	}
	text, _ := msg["text"].(string)
	return text, nil
}

// getParentMessageFull fetches the thread parent message as a raw map,
// including text, blocks, and attachments. This allows extractFullSlackMessage
// to be used on the result to get the complete error text with stack traces.
func getParentMessageFull(channel, threadTS string) (map[string]any, error) {
	form := url.Values{}
	form.Set("channel", channel)
	form.Set("ts", threadTS)
	form.Set("limit", "1")
	form.Set("inclusive", "true")
	req, _ := http.NewRequest("POST", "https://slack.com/api/conversations.replies", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+os.Getenv("SLACK_BOT_TOKEN"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		OK       bool             `json:"ok"`
		Messages []map[string]any `json:"messages"`
		Error    string           `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if !out.OK {
		return nil, fmt.Errorf("conversations.replies error: %s", out.Error)
	}
	if len(out.Messages) == 0 {
		return nil, fmt.Errorf("no messages in thread")
	}
	return out.Messages[0], nil
}
