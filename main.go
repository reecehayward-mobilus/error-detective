package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found, relying on environment variables.")
	}

	r := gin.Default()

	r.POST("/slack/events", func(c *gin.Context) {
		var payload map[string]any
		c.BindJSON(&payload)

		// Slack verification challenge
		if payload["type"] == "url_verification" {
			c.JSON(http.StatusOK, gin.H{"challenge": payload["challenge"]})
			return
		}

		event, _ := payload["event"].(map[string]any)
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
			// Build a comprehensive message body from text + blocks + attachments
			fullMsg := extractFullSlackMessage(event)
			subtype, _ := event["subtype"].(string)

			// Only process messages posted by the configured error bot.
			// if botID == "" || botID != os.Getenv("ERROR_BOT_ID") {
			// 	c.Status(http.StatusOK)
			// 	return
			// }
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
			if strings.TrimSpace(fullMsg) == "" {
				c.Status(http.StatusOK)
				return
			}

			// Ack immediately to stop further retries
			c.Status(http.StatusOK)
			if eventID != "" {
				markProcessed(eventID)
			}

			go func(ch, ts, message string) {
				logPreview := message
				if len(logPreview) > 500 {
					logPreview = logPreview[:500] + "…"
				}
				fmt.Println("Processing error bot message:", logPreview)
				explanation := explainError(message)
				postToSlackWithCreateButton(ch, ts, explanation, message)
			}(channel, threadTS, fullMsg)
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
