package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	genai "github.com/google/generative-ai-go/genai"
	openai "github.com/sashabaranov/go-openai"
	"google.golang.org/api/option"
)

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

	systemPrompt := "You are a senior Go engineer. Given an error log, produce: 1) concise cause (1-2 short bullet points) 2) prioritized debugging steps. Be direct. Reply in Japanese. Use Slack mrkdwn: use '•' for bullets, '1.' for ordered lists, and single '*' for bold (NOT **). Use single ticks for key tech words or variable names eg. `variableName`. Keep it short (<= 10 lines). Format strictly as:\n*【原因】*\n• <bullet>\n• <bullet>\n\n*【対策提案】*\n1. <step>\n2. <step>"

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
