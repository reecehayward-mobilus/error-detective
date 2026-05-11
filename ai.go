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

	systemPrompt := "You are a senior Go engineer. Given an error log, produce: 1) concise cause (1-2 short bullet points) 2) prioritized debugging steps. Be direct. Reply in Japanese. Use Slack mrkdwn: use '•' for bullets, '1.' for ordered lists, and single '*' for bold (NOT **). Use single ticks for key tech words or variable names eg. `variableName`. Keep it short (<= 10 lines). Format strictly as:\n*【原因】*\n• <bullet>\n• <bullet>\n\n*【対策提案】*\n1. <step>\n2. <step>"

	switch provider {
	case "openai":
		key := os.Getenv("OPENAI_API_KEY")
		if key == "" {
			return "OPENAI_API_KEY not set"
		}
		cfg := openai.DefaultConfig(key)
		if openaiBaseURL != "" {
			cfg.BaseURL = openaiBaseURL
		}
		client := openai.NewClientWithConfig(cfg)
		messages := []openai.ChatCompletionMessage{{Role: "system", Content: systemPrompt}, {Role: "user", Content: msg}}
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

// deepAnalysisSystemPrompt is the specialized system prompt for code-level
// root cause analysis used by explainErrorWithContext.
const deepAnalysisSystemPrompt = `You are a senior Go engineer performing deep code analysis. You have been given an error log AND the actual source code where the error occurred. Your task is to:
1) Identify the exact root cause by referencing specific lines in the provided code
2) Explain why this code causes the error
3) Provide a specific fix suggestion

Reference the actual source code lines in your analysis. Be specific about which function and which line.

IMPORTANT FORMATTING RULES:
- Keep the entire response under 2500 characters
- Do NOT include large code blocks. Use only short inline code references like ` + "`functionName()`" + ` or single-line snippets
- If you suggest a code fix, keep it to 3 lines maximum
- Be concise and direct — no lengthy explanations

Reply in Japanese. Use Slack mrkdwn: use '•' for bullets, '1.' for ordered lists, and single '*' for bold (NOT **). Use single backticks for code references. Do NOT use triple backtick code blocks. Format as:

*【根本原因】*
• <1-2 bullets referencing specific code lines>

*【コード分析】*
• <brief analysis, max 2-3 bullets>

*【修正提案】*
1. <specific fix, keep code snippets to 1 line>
2. <alternative if applicable>`

// explainErrorWithContext generates an AI explanation using both the error
// message and pre-fetched code context, with a specialized deep-analysis prompt.
func explainErrorWithContext(msg string, codeContext string) string {
	provider := strings.ToLower(strings.TrimSpace(os.Getenv("AI_PROVIDER")))
	if provider == "" {
		provider = "openai"
	}
	model := strings.TrimSpace(os.Getenv("AI_MODEL"))
	if model == "" {
		if provider == "openai" {
			model = "gpt-4o-mini"
		} else {
			model = "gemini-1.5-flash"
		}
	}

	switch provider {
	case "openai":
		key := os.Getenv("OPENAI_API_KEY")
		if key == "" {
			return "OPENAI_API_KEY not set"
		}
		cfg := openai.DefaultConfig(key)
		if openaiBaseURL != "" {
			cfg.BaseURL = openaiBaseURL
		}
		client := openai.NewClientWithConfig(cfg)
		messages := []openai.ChatCompletionMessage{
			{Role: "system", Content: deepAnalysisSystemPrompt},
			{Role: "user", Content: msg},
			{Role: "system", Content: "Relevant source code:\n" + codeContext},
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
		modelHandle.SystemInstruction = &genai.Content{Parts: []genai.Part{genai.Text(deepAnalysisSystemPrompt)}}
		parts := []genai.Part{
			genai.Text(msg),
			genai.Text("Relevant source code:\n" + codeContext),
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

// openaiBaseURL allows overriding the OpenAI API base URL for testing.
var openaiBaseURL = ""
