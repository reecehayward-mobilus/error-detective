# Product Summary

Error Detective is a Slack bot that monitors messages from a designated error-reporting bot. When it detects an error message, it uses an AI provider (OpenAI or Google Gemini) to generate a concise root-cause analysis and fix steps in Japanese, then posts the explanation as a threaded reply.

It also supports one-click Backlog ticket creation: a button is attached to the AI explanation, and clicking it creates a bug ticket in Backlog with the error context and a link back to the Slack thread.

## Key Capabilities

- Listens to Slack Events API for messages from a specific bot (identified by `ERROR_BOT_ID`)
- Extracts error details from Slack message text, blocks, and attachments
- Gathers source code context from local repos when file:line references are found in stack traces
- Generates AI-powered explanations via OpenAI or Gemini (configurable)
- Posts threaded replies formatted in Slack mrkdwn (Japanese)
- One-click Backlog issue creation with auto-generated summary and description
- In-memory event deduplication to handle Slack retries
