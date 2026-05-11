# Project Structure

```
.
├── main.go            # Entire application: HTTP server, Slack handlers, AI integration, Backlog client
├── main_test.go       # Unit tests
├── go.mod             # Module definition and dependencies
├── go.sum             # Dependency checksums
├── vendor/            # Vendored dependencies (committed)
├── .env               # Local environment variables (gitignored)
├── .github/
│   └── copilot-instructions.md
└── .kiro/
    └── steering/      # AI assistant steering rules
```

## Architecture

This is a single-file application (`main.go`). All logic lives in one package (`main`):

- **HTTP endpoints** — `/slack/events` (Slack Events API), `/slack/interactions` (button clicks)
- **Event deduplication** — in-memory map with 15-minute TTL
- **Message extraction** — parses Slack text, blocks, and attachments into plain text
- **Code context gathering** — fetches source code from Bitbucket Cloud API when "深掘りする" button is clicked
- **AI explanation** — delegates to OpenAI or Gemini based on `AI_PROVIDER`
- **Slack posting** — threaded replies with Block Kit (mrkdwn sections + action buttons)
- **Backlog integration** — one-time-use token cache, issue creation via REST API
- **Ticket context cache** — in-memory map linking button tokens to error context

## Conventions

- No sub-packages; everything is in `package main`
- Functions are organized by concern (dedup → ticket cache → Backlog client → Slack helpers → AI → HTTP handlers → main)
- Tests live in `main_test.go` alongside the source
- Vendor directory is committed — run `go mod vendor` after dependency changes
