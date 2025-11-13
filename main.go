package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
	payload := map[string]interface{}{
		"channel":   channel,
		"thread_ts": threadTS,
		"text":      fallback,
		"blocks": []map[string]interface{}{
			{
				"type": "section",
				"text": map[string]interface{}{
					"type": "mrkdwn",
					"text": text,
				},
			},
		},
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
		retryNum := c.GetHeader("X-Slack-Retry-Num")
		if event["type"] == "message" {
			// drop edits, bot messages, and Slack retry duplicates
			if eventID != "" && hasProcessed(eventID) {
				c.Status(http.StatusOK)
				return
			}

			botID, _ := event["bot_id"].(string)
			channel, _ := event["channel"].(string)
			threadTS, _ := event["ts"].(string)
			text, _ := event["text"].(string)
			subtype, _ := event["subtype"].(string)

			// Ignore messages from bots to prevent loops, and messages with no text.
			_, isBot := event["bot_id"]
			if isBot || text == "" || subtype != "" {
				c.Status(http.StatusOK)
				return
			}

			// Always ack immediately to avoid Slack retries
			c.Status(http.StatusOK)

			// Only process once
			if eventID != "" {
				markProcessed(eventID)
			}

			// process asynchronously
			go func(ch, ts, message, retry string) {
				fmt.Println("Received message from user:", message)
				explanation := explainError(message)
				postToSlack(ch, ts, explanation)
			}(channel, threadTS, text, retryNum)
			// Respond only if message came from the error-reporting bot
			if botID == os.Getenv("ERROR_BOT_ID") {
				fmt.Println("Received message from error bot:", text)
				explanation := explainError(text)
				postToSlack(channel, threadTS, explanation)
			}
			return
		}

		c.Status(http.StatusOK)
	})

	r.Run(":8080")
}
