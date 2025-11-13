# Error Detective Slack Bot Instructions

This document provides guidance for AI agents working on the `error-detective` codebase.

## 1. Project Overview

The `error-detective` is a Go-based Slack bot that listens for messages posted by a specific error-reporting bot. When it detects an error message, it uses the OpenAI API (specifically, `gpt-4o-mini`) to generate a concise explanation of the error and a likely fix. It then posts this explanation as a threaded reply to the original error message in Slack.

The entire application is contained within a single file, `main.go`.

## 2. Core Components & Data Flow

1.  **Gin Web Server (`main.go`):** An HTTP server listens for incoming POST requests from Slack on the `/slack/events` endpoint.
2.  **Slack Event Handling:**
    -   It handles Slack's `url_verification` challenge.
    -   It processes `message` events. It checks if the message's `bot_id` matches the `ERROR_BOT_ID` environment variable.
3.  **OpenAI Integration (`explainError` function):** If the message is from the target error bot, its text is passed to the `explainError` function. This function calls the OpenAI ChatCompletion API to get an analysis of the error.
4.  **Slack Message Posting (`postToSlack` function):** The explanation from OpenAI is posted back to the original channel as a threaded reply using the `chat.postMessage` Slack API method.

## 3. Configuration

The application is configured exclusively through environment variables. All three are required for the application to function correctly.

-   `OPENAI_API_KEY`: Your API key for the OpenAI service.
-   `SLACK_BOT_TOKEN`: A Slack bot token with the necessary permissions (e.g., `chat:write`, `channels:history`, `groups:history`).
-   `ERROR_BOT_ID`: The unique `bot_id` of the Slack bot whose messages you want this service to analyze.

These are loaded at runtime using `os.Getenv()`. For local development, you can create a `.env` file and use a tool like `godotenv` or simply `export` them in your shell.

## 4. Developer Workflow

### Running the Application

1.  **Install Dependencies:**
    ```sh
    go mod tidy
    ```
2.  **Set Environment Variables:**
    ```sh
    export OPENAI_API_KEY="sk-..."
    export SLACK_BOT_TOKEN="xoxb-..."
    export ERROR_BOT_ID="B01234567"
    ```
3.  **Run the Server:**
    The server starts on port 8080.
    ```sh
    go run .
    ```

### Testing with Slack

To receive live events from Slack, your local server must be accessible from the public internet.

1.  **Expose Localhost:** Use a tool like `ngrok`.
    ```sh
    ngrok http 8080
    ```
2.  **Configure Slack App:**
    -   Take the public URL provided by ngrok (e.g., `https://<your-id>.ngrok-free.app`).
    -   In your Slack App's configuration, go to "Event Subscriptions".
    -   Set the "Request URL" to `https://<your-id>.ngrok-free.app/slack/events`.
    -   Ensure your bot is subscribed to the appropriate `message` events (e.g., `message.channels`).
