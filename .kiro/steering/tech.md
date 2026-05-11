# Tech Stack

## Language & Runtime

- Go 1.25
- Single-binary application (all logic in `main.go`)

## Key Libraries

| Library                                    | Purpose                         |
| ------------------------------------------ | ------------------------------- |
| `github.com/gin-gonic/gin`                 | HTTP router and web framework   |
| `github.com/sashabaranov/go-openai`        | OpenAI API client               |
| `github.com/google/generative-ai-go/genai` | Google Gemini API client        |
| `github.com/joho/godotenv`                 | Load `.env` files for local dev |

## External Services

- Slack Events API & Web API (messages, interactions, permalinks)
- OpenAI ChatCompletion API (default provider)
- Google Gemini GenerateContent API (alternative provider)
- Backlog REST API v2 (issue creation)

## Dependency Management

- Go modules (`go.mod` / `go.sum`)
- Vendored dependencies (`vendor/` directory committed)

## Common Commands

```bash
# Install / tidy dependencies
go mod tidy

# Run the server (starts on :8080)
go run .

# Run tests
go test ./...

# Build binary
go build -o error-detective .

# Vendor dependencies after adding new ones
go mod vendor
```

## Configuration

All configuration is via environment variables (loaded from `.env` in local dev via godotenv). Key variables:

- `AI_PROVIDER` — `openai` or `gemini`
- `AI_MODEL` — model name (defaults: `gpt-4o-mini` / `gemini-1.5-flash`)
- `SLACK_BOT_TOKEN`, `SLACK_SIGNING_SECRET`, `ERROR_BOT_ID`
- `OPENAI_API_KEY` or `GEMINI_API_KEY`
- `BACKLOG_BASE_URL`, `BACKLOG_API_KEY`, `BACKLOG_PROJECT_KEY`, `BACKLOG_ISSUE_TYPE_ID`, `BACKLOG_PRIORITY_ID`
- `BITBUCKET_USERNAME`, `BITBUCKET_APP_PASSWORD`, `BITBUCKET_BRANCH` — Bitbucket Cloud API access for code context

## Notes

- The `.env` file is gitignored; never commit secrets.
- The server must be publicly reachable for Slack events (use ngrok for local dev).
- AI responses are formatted in Japanese using Slack mrkdwn syntax.
