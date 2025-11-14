package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	genai "github.com/google/generative-ai-go/genai"
	"github.com/joho/godotenv"
	openai "github.com/sashabaranov/go-openai"
	"google.golang.org/api/option"
)

// simple in-memory deduplication for Slack event_ids
var processed = struct {
	mu sync.Mutex
	m  map[string]time.Time
}{m: make(map[string]time.Time)}

func hasProcessed(id string) bool {
	processed.mu.Lock()
	defer processed.mu.Unlock()
	if t, ok := processed.m[id]; ok {
		// expire after 15 minutes
		if time.Since(t) < 15*time.Minute {
			return true
		}
		delete(processed.m, id)
	}
	return false
}

func markProcessed(id string) {
	processed.mu.Lock()
	processed.m[id] = time.Now()
	// light cleanup to avoid unbounded growth
	if len(processed.m) > 1000 {
		cutoff := time.Now().Add(-15 * time.Minute)
		for k, v := range processed.m {
			if v.Before(cutoff) {
				delete(processed.m, k)
			}
		}
	}
	processed.mu.Unlock()
}

// One-click ticket creation cache
type ticketContext struct {
	Channel     string
	ThreadTS    string
	ErrorText   string
	Explanation string
}

type cacheItem struct {
	Ctx    ticketContext
	Expire time.Time
}

var ticketCache = struct {
	mu sync.Mutex
	m  map[string]cacheItem
}{m: make(map[string]cacheItem)}

func getBacklogTokenTTL() time.Duration {
	v := strings.TrimSpace(os.Getenv("BACKLOG_TOKEN_TTL_HOURS"))
	if v == "" {
		return 24 * time.Hour
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 24 * time.Hour
	}
	return time.Duration(n) * time.Hour
}

func genToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func putTicketContext(ctx ticketContext, ttl time.Duration) (string, error) {
	tok, err := genToken()
	if err != nil {
		return "", err
	}
	ticketCache.mu.Lock()
	ticketCache.m[tok] = cacheItem{Ctx: ctx, Expire: time.Now().Add(ttl)}
	// cleanup
	for k, v := range ticketCache.m {
		if time.Now().After(v.Expire) {
			delete(ticketCache.m, k)
		}
	}
	ticketCache.mu.Unlock()
	return tok, nil
}

func takeTicketContext(tok string) (ticketContext, bool) {
	ticketCache.mu.Lock()
	defer ticketCache.mu.Unlock()
	it, ok := ticketCache.m[tok]
	if !ok || time.Now().After(it.Expire) {
		delete(ticketCache.m, tok)
		return ticketContext{}, false
	}
	// one-time use
	delete(ticketCache.m, tok)
	return it.Ctx, true
}

func backlogConfigured() bool {
	return os.Getenv("BACKLOG_BASE_URL") != "" && os.Getenv("BACKLOG_API_KEY") != "" && os.Getenv("BACKLOG_PROJECT_KEY") != ""
}

func generateIssueSummary(_ string, explanation string) string {
	// Simplified: take first meaningful line after the 【原因】 heading.
	maxLen := 120
	if explanation == "" {
		return "エラー通知botによるバグ"
	}
	lines := strings.Split(explanation, "\n")
	foundHeading := false
	for i, raw := range lines {
		l := strings.TrimSpace(raw)
		if l == "" {
			continue
		}
		if strings.Contains(l, "【原因】") {
			// scan forward for first bullet/non-empty line
			for _, next := range lines[i+1:] {
				nextTrim := strings.TrimSpace(next)
				if nextTrim == "" {
					continue
				}
				// remove markdown bullet markers
				nextTrim = strings.TrimLeft(nextTrim, "-•*")
				nextTrim = strings.TrimSpace(nextTrim)
				if nextTrim == "" {
					continue
				}
				if len(nextTrim) > maxLen {
					nextTrim = nextTrim[:maxLen-3] + "..."
				}
				return nextTrim
			}
			foundHeading = true
			break
		}
	}
	if foundHeading {
		return "原因不明のエラー"
	}
	// Fallback to first non-empty trimmed line
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			if len(l) > maxLen {
				l = l[:maxLen-3] + "..."
			}
			return l
		}
	}
	return "エラー通知botによるバグ"
}

func buildBacklogIssue(ctx ticketContext) (summary string, description string) {
	summary = generateIssueSummary(ctx.ErrorText, ctx.Explanation)
	fmt.Println("Generated issue summary:", summary)
	var b strings.Builder
	b.WriteString("[自動作成]\n")
	b.WriteString("Slackスレッド: (後で追加)\n\n")
	b.WriteString("【説明】\n")
	b.WriteString(ctx.Explanation)
	b.WriteString("\n\n【エラーログ】\n")
	b.WriteString(ctx.ErrorText)
	description = b.String()
	return
}

// Resolve Backlog project ID from projectKey (cached).
var backlogProjectIDCache = struct {
	mu sync.Mutex
	m  map[string]int64
}{m: make(map[string]int64)}

func resolveProjectID(baseURL, apiKey, projectKey string) (int64, error) {
	// Allow override via BACKLOG_PROJECT_ID env (string -> int64)
	if override := strings.TrimSpace(os.Getenv("BACKLOG_PROJECT_ID")); override != "" {
		if id, err := strconv.ParseInt(override, 10, 64); err == nil {
			return id, nil
		}
	}
	backlogProjectIDCache.mu.Lock()
	if id, ok := backlogProjectIDCache.m[projectKey]; ok {
		backlogProjectIDCache.mu.Unlock()
		return id, nil
	}
	backlogProjectIDCache.mu.Unlock()

	endpoint := strings.TrimRight(baseURL, "/") + "/api/v2/projects?apiKey=" + url.QueryEscape(apiKey)
	req, _ := http.NewRequest("GET", endpoint, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("projects list failed: %s", string(body))
	}
	var projects []struct {
		ID         int64  `json:"id"`
		ProjectKey string `json:"projectKey"`
	}
	if err := json.Unmarshal(body, &projects); err != nil {
		return 0, err
	}
	for _, p := range projects {
		if strings.EqualFold(p.ProjectKey, projectKey) {
			backlogProjectIDCache.mu.Lock()
			backlogProjectIDCache.m[projectKey] = p.ID
			backlogProjectIDCache.mu.Unlock()
			return p.ID, nil
		}
	}
	return 0, fmt.Errorf("projectKey '%s' not found", projectKey)
}

func createBacklogIssue(projectKey, baseURL, apiKey, summary, description string) (issueKey, issueURL string, err error) {
	projectID, err := resolveProjectID(baseURL, apiKey, projectKey)
	if err != nil {
		return "", "", err
	}
	// Required IDs: issueTypeId & priorityId must exist; we surface clearer errors.
	issueTypeID := strings.TrimSpace(os.Getenv("BACKLOG_ISSUE_TYPE_ID"))
	if issueTypeID == "" {
		return "", "", fmt.Errorf("BACKLOG_ISSUE_TYPE_ID not set (Backlog API requires issueTypeId)")
	}
	priorityID := strings.TrimSpace(os.Getenv("BACKLOG_PRIORITY_ID"))
	if priorityID == "" {
		return "", "", fmt.Errorf("BACKLOG_PRIORITY_ID not set (Backlog API requires priorityId)")
	}

	form := url.Values{}
	form.Set("projectId", strconv.FormatInt(projectID, 10))
	form.Set("summary", summary)
	form.Set("issueTypeId", issueTypeID)
	form.Set("priorityId", priorityID)
	form.Set("description", description)

	// Optional assignee
	if assignee := strings.TrimSpace(os.Getenv("BACKLOG_ASSIGNEE_ID")); assignee != "" {
		form.Set("assigneeId", assignee)
	}
	// Optional category/version/milestone lists (comma separated IDs)
	for _, pair := range []struct{ env, key string }{
		{"BACKLOG_CATEGORY_IDS", "categoryId[]"},
		{"BACKLOG_VERSION_IDS", "versionId[]"},
		{"BACKLOG_MILESTONE_IDS", "milestoneId[]"},
		{"BACKLOG_NOTIFIED_USER_IDS", "notifiedUserId[]"},
	} {
		raw := strings.TrimSpace(os.Getenv(pair.env))
		if raw == "" {
			continue
		}
		for _, id := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' }) {
			id = strings.TrimSpace(id)
			if id != "" {
				form.Add(pair.key, id)
			}
		}
	}
	// Custom fields: BACKLOG_CUSTOM_FIELDS format: fieldId=value;fieldId=value
	cfRaw := strings.TrimSpace(os.Getenv("BACKLOG_CUSTOM_FIELDS"))
	if cfRaw != "" {
		parts := strings.FieldsFunc(cfRaw, func(r rune) bool { return r == ';' || r == ',' })
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			kv := strings.SplitN(p, "=", 2)
			if len(kv) != 2 {
				continue
			}
			fieldID := strings.TrimSpace(kv[0])
			value := strings.TrimSpace(kv[1])
			if fieldID != "" && value != "" {
				form.Set("customField_"+fieldID, value)
			}
		}
	}

	endpoint := strings.TrimRight(baseURL, "/") + "/api/v2/issues?apiKey=" + url.QueryEscape(apiKey)
	req, _ := http.NewRequest("POST", endpoint, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("backlog create failed: %s", string(body))
	}
	var res struct {
		IssueKey string `json:"issueKey"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return "", "", err
	}
	if res.IssueKey == "" {
		return "", "", fmt.Errorf("backlog create returned no key: %s", string(body))
	}
	issueURL = strings.TrimRight(baseURL, "/") + "/view/" + res.IssueKey
	return res.IssueKey, issueURL, nil
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
	form := url.Values{}
	form.Set("channel", channel)
	form.Set("ts", threadTS)
	req, _ := http.NewRequest("POST", "https://slack.com/api/conversations.replies", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+os.Getenv("SLACK_BOT_TOKEN"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		OK       bool `json:"ok"`
		Messages []struct {
			Text     string `json:"text"`
			TS       string `json:"ts"`
			ThreadTS string `json:"thread_ts"`
		} `json:"messages"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if !out.OK {
		return "", fmt.Errorf("conversations.replies error: %s", out.Error)
	}
	if len(out.Messages) == 0 {
		return "", fmt.Errorf("no messages in thread")
	}
	// First message is the parent
	return out.Messages[0].Text, nil
}

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

// gatherContext extracts file:line references from an error message and returns
// small code snippets (a few lines of context) from matching files under any
// of the configured REPO_ROOTS (colon or comma separated absolute paths).
func gatherContext(msg string) string {
	rootsEnv := os.Getenv("REPO_ROOTS")
	if rootsEnv == "" {
		return ""
	}
	roots := strings.FieldsFunc(rootsEnv, func(r rune) bool { return r == ':' || r == ',' })
	for i, r := range roots {
		roots[i] = strings.TrimSpace(r)
	}

	fileLineRe := regexp.MustCompile(`([\w./\\_-]+\.go):(\d+)`)
	matches := fileLineRe.FindAllStringSubmatch(msg, -1)
	if len(matches) == 0 {
		return ""
	}
	maxFiles := 3
	var snippets []string
	seen := make(map[string]struct{})
	for _, m := range matches {
		if len(snippets) >= maxFiles {
			break
		}
		rel := m[1]
		lnStr := m[2]
		key := rel + ":" + lnStr
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		lineNum, _ := strconv.Atoi(lnStr)

		// Build candidate file paths:
		// 1) the path as-is
		// 2) if absolute like /go/src/bitbucket.org/... try to strip up to
		//    '/src/' or VCS host and rebase under each REPO_ROOTS.
		candidatePaths := []string{rel}
		if filepath.IsAbs(rel) {
			rebased := false
			// Try stripping up to '/src/'
			if idx := strings.Index(rel, "/src/"); idx >= 0 && idx+5 < len(rel) {
				relFromSrc := rel[idx+5:]
				for _, root := range roots {
					candidatePaths = append(candidatePaths, filepath.Join(root, relFromSrc))
				}
				rebased = true
			}
			// Try common VCS hosts
			if !rebased {
				for _, host := range []string{"bitbucket.org", "github.com", "gitlab.com"} {
					marker := "/" + host + "/"
					if idx := strings.Index(rel, marker); idx >= 0 {
						relFromHost := rel[idx+1:] // drop leading '/'
						for _, root := range roots {
							candidatePaths = append(candidatePaths, filepath.Join(root, relFromHost))
						}
						break
					}
				}
			}
		} else {
			for _, root := range roots {
				candidatePaths = append(candidatePaths, filepath.Join(root, rel))
			}
		}
		var chosen string
		for _, c := range candidatePaths {
			if info, err := os.Stat(c); err == nil && !info.IsDir() {
				chosen = c
				break
			}
		}
		if chosen == "" {
			continue
		}
		data, err := os.ReadFile(chosen)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		start := lineNum - 4
		if start < 1 {
			start = 1
		}
		end := lineNum + 4
		if end > len(lines) {
			end = len(lines)
		}
		var b strings.Builder
		b.WriteString(fmt.Sprintf("// %s:%d\n", chosen, lineNum))
		for i := start; i <= end; i++ {
			b.WriteString(fmt.Sprintf("%4d | %s\n", i, lines[i-1]))
		}
		snippets = append(snippets, b.String())
	}
	if len(snippets) == 0 {
		return ""
	}
	joined := strings.Join(snippets, "\n")
	if len(joined) > 4000 { // cap size
		joined = joined[:4000] + "\n... (truncated)"
	}
	return joined
}

// explainError delegates to the configured AI provider (OpenAI or Gemini) to generate an explanation.
func explainError(msg string) string {
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("AI_PROVIDER")))
	if provider == "" {
		provider = "openai" // default
	}
	model := strings.TrimSpace(os.Getenv("AI_MODEL"))
	if model == "" {
		if provider == "openai" {
			model = "gpt-4o-mini"
		} else {
			model = "gemini-1.5-flash"
		}
	}
	ctxSnippet := gatherContext(msg)
	if ctxSnippet != "" && os.Getenv("DEBUG_CONTEXT") == "1" {
		preview := ctxSnippet
		if len(preview) > 800 {
			preview = preview[:800] + "\n... (truncated for debug output)"
		}
		fmt.Printf("[debug] gathered context (%d bytes) preview:\n%s\n", len(ctxSnippet), preview)
	}

	systemPrompt := "You are a senior Go engineer. Given an error log, produce: 1) concise cause (1-2 short bullet points) 2) prioritized fix steps. Be direct. Reply in Japanese. Use Slack mrkdwn: use '-' for bullets, '1.' for ordered lists, and single '*' for bold (NOT **). Use single ticks for key tech words or variable names eg. `variableName`. Keep it short (<= 10 lines). Format strictly as:\n*【原因】*\n- <bullet>\n*【修正手順】*\n1. <step>\n2. <step>"

	switch provider {
	case "openai":
		key := os.Getenv("OPENAI_API_KEY")
		if key == "" {
			return "OPENAI_API_KEY not set"
		}
		client := openai.NewClient(key)
		messages := []openai.ChatCompletionMessage{{Role: "system", Content: systemPrompt}, {Role: "user", Content: msg}}
		if ctxSnippet != "" {
			messages = append(messages, openai.ChatCompletionMessage{Role: "system", Content: "Relevant code context:\n" + ctxSnippet})
		}
		resp, err := client.CreateChatCompletion(context.Background(), openai.ChatCompletionRequest{Model: model, Messages: messages})
		if err != nil {
			return fmt.Sprintf("Error analyzing (openai): %v", err)
		}
		if len(resp.Choices) == 0 {
			return "No response from OpenAI"
		}
		return resp.Choices[0].Message.Content

	case "gemini":
		key := os.Getenv("GEMINI_API_KEY")
		if key == "" {
			return "GEMINI_API_KEY not set"
		}
		ctx := context.Background()
		client, err := genai.NewClient(ctx, option.WithAPIKey(key))
		if err != nil {
			return fmt.Sprintf("Error creating Gemini client: %v", err)
		}
		modelHandle := client.GenerativeModel(model)
		modelHandle.SystemInstruction = &genai.Content{Parts: []genai.Part{genai.Text(systemPrompt)}}
		parts := []genai.Part{genai.Text(msg)}
		if ctxSnippet != "" {
			parts = append(parts, genai.Text("Relevant code context:\n"+ctxSnippet))
		}
		resp, err := modelHandle.GenerateContent(ctx, parts...)
		if err != nil {
			return fmt.Sprintf("Error analyzing (gemini): %v", err)
		}
		if len(resp.Candidates) == 0 {
			return "No response from Gemini"
		}
		var b strings.Builder
		for _, part := range resp.Candidates[0].Content.Parts {
			if txt, ok := part.(genai.Text); ok {
				b.WriteString(string(txt))
			}
		}
		return b.String()

	default:
		return "Unknown AI_PROVIDER; use 'openai' or 'gemini'"
	}
}

func postToSlack(channel, threadTS, text string) {
	// Use Block Kit section with mrkdwn to ensure list/bold rendering
	fallback := text
	if len(fallback) > 300 {
		fallback = fallback[:300] + "…"
	}
	blocks := []map[string]interface{}{
		{
			"type": "section",
			"text": map[string]interface{}{
				"type": "mrkdwn",
				"text": text,
			},
		},
	}

	// Note: create-button is added by postToSlackWithCreateButton wrapper when configured.

	payload := map[string]interface{}{
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

	var slackResponse map[string]interface{}
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

// postToSlackWithCreateButton posts the explanation and appends an Actions block with a one-click Backlog button.
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

	// Build payload with button
	fallback := explanation
	if len(fallback) > 300 {
		fallback = fallback[:300] + "…"
	}
	blocks := []map[string]interface{}{
		{
			"type": "section",
			"text": map[string]interface{}{
				"type": "mrkdwn",
				"text": explanation,
			},
		},
		{
			"type": "actions",
			"elements": []map[string]interface{}{
				{
					"type":      "button",
					"text":      map[string]string{"type": "plain_text", "text": "Backlogに起票"},
					"style":     "primary",
					"value":     tok,
					"action_id": "create_backlog_ticket",
				},
			},
		},
	}
	payload := map[string]interface{}{
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
	var slackResponse map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&slackResponse); err != nil {
		fmt.Printf("Error decoding Slack response: %v\n", err)
		return
	}
	if ok, _ := slackResponse["ok"].(bool); !ok {
		fmt.Printf("Slack API Error: %s\n", slackResponse["error"])
	}
}

// postToSlackSimple posts a simple mrkdwn message in thread without actions.
func postToSlackSimple(channel, threadTS, text string) {
	fallback := text
	if len(fallback) > 300 {
		fallback = fallback[:300] + "…"
	}
	payload := map[string]interface{}{
		"channel":   channel,
		"thread_ts": threadTS,
		"text":      fallback,
		"blocks": []map[string]interface{}{
			{"type": "section", "text": map[string]interface{}{"type": "mrkdwn", "text": text}},
		},
	}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+os.Getenv("SLACK_BOT_TOKEN"))
	http.DefaultClient.Do(req)
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found, relying on environment variables.")
	}

	r := gin.Default()

	r.POST("/slack/events", func(c *gin.Context) {
		var payload map[string]interface{}
		c.BindJSON(&payload)

		// Slack verification challenge
		if payload["type"] == "url_verification" {
			c.JSON(http.StatusOK, gin.H{"challenge": payload["challenge"]})
			return
		}

		event, _ := payload["event"].(map[string]interface{})
		eventID, _ := payload["event_id"].(string)
		// We ignore Slack retry headers and deduplicate via event_id.
		if event["type"] == "message" {
			// Dedup retry deliveries
			if eventID != "" && hasProcessed(eventID) {
				c.Status(http.StatusOK)
				return
			}

			botID, _ := event["bot_id"].(string)
			channel, _ := event["channel"].(string)
			threadTS, _ := event["ts"].(string)
			text, _ := event["text"].(string)
			subtype, _ := event["subtype"].(string)

			// Only process messages posted by the configured error bot.
			if botID == "" || botID != os.Getenv("ERROR_BOT_ID") {
				c.Status(http.StatusOK)
				return
			}
			// Optional extra safety: ignore this app's own bot messages to prevent loops if misconfigured.
			if self := strings.TrimSpace(os.Getenv("SELF_BOT_ID")); self != "" && botID == self {
				c.Status(http.StatusOK)
				return
			}
			// Ignore edited/deleted/etc. subtypes, but allow normal bot posts (subtype=="bot_message").
			if subtype != "" && subtype != "bot_message" {
				c.Status(http.StatusOK)
				return
			}
			if strings.TrimSpace(text) == "" {
				c.Status(http.StatusOK)
				return
			}

			// Ack immediately to stop further retries
			c.Status(http.StatusOK)
			if eventID != "" {
				markProcessed(eventID)
			}

			go func(ch, ts, message string) {
				fmt.Println("Processing error bot message:", message)
				explanation := explainError(message)
				postToSlackWithCreateButton(ch, ts, explanation, message)
			}(channel, threadTS, text)
			return
		}

		c.Status(http.StatusOK)
	})

	// Slack Interactivity endpoint for button clicks
	r.POST("/slack/interactions", func(c *gin.Context) {
		// Read raw body for signature
		raw, _ := io.ReadAll(c.Request.Body)
		// Reassign body so we can parse form
		c.Request.Body = io.NopCloser(bytes.NewReader(raw))
		if !verifySlackSignature(c, raw) {
			c.Status(http.StatusUnauthorized)
			return
		}
		// Parse payload from form
		if err := c.Request.ParseForm(); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		payloadStr := c.PostForm("payload")
		var p struct {
			Type string `json:"type"`
			User struct {
				ID string `json:"id"`
			} `json:"user"`
			Channel struct {
				ID string `json:"id"`
			} `json:"channel"`
			Message struct {
				TS       string `json:"ts"`
				ThreadTS string `json:"thread_ts"`
				Blocks   []struct {
					Type string `json:"type"`
					Text struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"text"`
				} `json:"blocks"`
			} `json:"message"`
			Actions []struct {
				ActionID string `json:"action_id"`
				Value    string `json:"value"`
			} `json:"actions"`
		}
		if err := json.Unmarshal([]byte(payloadStr), &p); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		if p.Type != "block_actions" || len(p.Actions) == 0 || p.Actions[0].ActionID != "create_backlog_ticket" {
			c.Status(http.StatusOK)
			return
		}
		token := p.Actions[0].Value
		ctx, ok := takeTicketContext(token)
		// If cache expired or missing, reconstruct from payload and Slack API
		if !ok {
			threadTS := p.Message.ThreadTS
			if threadTS == "" {
				threadTS = p.Message.TS
			}
			// Extract explanation from the section block
			explanation := ""
			for _, b := range p.Message.Blocks {
				if b.Type == "section" && b.Text.Text != "" {
					explanation = b.Text.Text
					break
				}
			}
			// Fetch original error text from thread parent
			errText, err := getParentMessageText(p.Channel.ID, threadTS)
			if err != nil {
				// As a last resort, continue with minimal context
				errText = ""
			}
			ctx = ticketContext{Channel: p.Channel.ID, ThreadTS: threadTS, ErrorText: errText, Explanation: explanation}
		}
		// Ack immediately
		c.Status(http.StatusOK)

		go func(tc ticketContext) {
			// Add permalink into description
			permalink, err := getSlackPermalink(tc.Channel, tc.ThreadTS)
			summary, description := buildBacklogIssue(tc)
			if err == nil && permalink != "" {
				description = strings.Replace(description, "Slackスレッド: (後で追加)", "Slackスレッド: "+permalink, 1)
			}
			issueKey, issueURL, err := createBacklogIssue(
				os.Getenv("BACKLOG_PROJECT_KEY"),
				os.Getenv("BACKLOG_BASE_URL"),
				os.Getenv("BACKLOG_API_KEY"),
				summary, description,
			)
			if err != nil {
				postToSlackSimple(tc.Channel, tc.ThreadTS, fmt.Sprintf(":x: Backlog作成に失敗しました: %v", err))
				return
			}
			postToSlackSimple(tc.Channel, tc.ThreadTS, fmt.Sprintf(":white_check_mark: Backlogチケットを作成しました: %s\n%s", issueKey, issueURL))
		}(ctx)
	})

	r.Run(":8080")
}
