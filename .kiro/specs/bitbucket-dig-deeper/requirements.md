# Requirements Document

## Introduction

This feature adds a "深掘りする" (Dig Deeper) button to the Slack bot's AI explanation replies, positioned alongside the existing "Backlogに起票" button. When clicked, the app fetches relevant source code from Bitbucket Cloud via its REST API (replacing the broken local `REPO_ROOTS` file-reading approach that fails on render.io), then performs a follow-up AI analysis with the code context to provide a more accurate debugging solution.

## Glossary

- **Error_Detective**: The Slack bot application that monitors error messages and provides AI-powered analysis
- **Dig_Deeper_Button**: A Slack Block Kit button labeled "深掘りする" that triggers Bitbucket code fetching and re-analysis
- **Bitbucket_Client**: The module responsible for communicating with the Bitbucket Cloud REST API to fetch file contents
- **Code_Context_Fetcher**: The component that parses file:line references from error messages and retrieves corresponding source code from Bitbucket
- **AI_Analyzer**: The component that sends error messages and code context to an AI provider (OpenAI or Gemini) for explanation
- **Interaction_Handler**: The HTTP handler at `/slack/interactions` that processes Slack button click payloads

## Requirements

### Requirement 1: Display Dig Deeper Button

**User Story:** As a developer viewing an error analysis in Slack, I want to see a "深掘りする" button next to the "Backlogに起票" button, so that I can request a deeper code-aware analysis.

#### Acceptance Criteria

1. WHEN the AI_Analyzer produces an explanation for an error message, THE Error_Detective SHALL display a "深掘りする" button in the actions block alongside the existing "Backlogに起票" button
2. THE Dig_Deeper_Button SHALL use the action_id "dig_deeper_bitbucket"
3. THE Dig_Deeper_Button SHALL be rendered with Slack Block Kit button type using plain_text label "深掘りする"
4. WHILE Bitbucket configuration environment variables are not set, THE Error_Detective SHALL omit the Dig_Deeper_Button from the actions block

### Requirement 2: Parse File References from Error Messages

**User Story:** As a developer, I want the system to identify file paths and line numbers from error stack traces, so that it can fetch the relevant source code from Bitbucket.

#### Acceptance Criteria

1. WHEN the Dig_Deeper_Button is clicked, THE Code_Context_Fetcher SHALL extract file path and line number references matching the pattern `<path>.go:<line>` from the original error message
2. THE Code_Context_Fetcher SHALL parse the Bitbucket workspace and repository slug from the file path itself (e.g., from `/go/src/bitbucket.org/mobilusall/mbv-api/interfaces/controllers/call_flow.go:269`, extract workspace `mobilusall` and repo slug `mbv-api`)
3. THE Code_Context_Fetcher SHALL normalize absolute file paths by stripping the prefix up to and including the repo slug (e.g., `/go/src/bitbucket.org/mobilusall/mbv-api/`) to produce repository-relative paths
4. THE Code_Context_Fetcher SHALL correctly handle file references from multiple repositories (e.g., `mbv-api` and `mbv-agi`) within the same error message, fetching from the appropriate repo for each reference
5. THE Code_Context_Fetcher SHALL extract a maximum of 5 file references from a single error message
6. IF no file references are found in the error message, THEN THE Error_Detective SHALL post a message indicating no code references were found to analyze

### Requirement 3: Fetch Source Code from Bitbucket Cloud API

**User Story:** As a developer, I want the app to retrieve source code from our Bitbucket repositories when deployed on render.io, so that the AI can provide code-aware debugging suggestions without requiring local file system access.

#### Acceptance Criteria

1. WHEN file references are extracted, THE Bitbucket_Client SHALL fetch file contents from the Bitbucket Cloud REST API v2.0 endpoint (`/2.0/repositories/{workspace}/{repo_slug}/src/{commit_or_branch}/{path}`), using the workspace and repo slug parsed from each file reference
2. THE Bitbucket_Client SHALL authenticate using either an App Password or a Repository Access Token configured via environment variables
3. THE Bitbucket_Client SHALL use the branch or commit reference specified in the `BITBUCKET_BRANCH` environment variable, defaulting to `master` when not set
4. IF the Bitbucket API returns a non-success status code for a file, THEN THE Bitbucket_Client SHALL skip that file and continue processing remaining references
5. THE Bitbucket_Client SHALL extract a context window of 10 lines above and 10 lines below the referenced line number from the fetched file content
6. THE Bitbucket_Client SHALL enforce a total code context size limit of 8000 characters across all fetched snippets

### Requirement 4: Perform Follow-Up AI Analysis with Code Context

**User Story:** As a developer, I want the AI to re-analyze the error with actual source code context, so that I receive a more accurate and actionable debugging solution.

#### Acceptance Criteria

1. WHEN code context is successfully fetched from Bitbucket, THE AI_Analyzer SHALL generate a new explanation using both the original error message and the fetched code snippets
2. THE AI_Analyzer SHALL use a specialized system prompt that instructs the AI to provide code-level root cause analysis and specific fix suggestions referencing the actual source code
3. THE AI_Analyzer SHALL post the enhanced analysis as a new threaded reply in the same Slack thread
4. THE AI_Analyzer SHALL format the enhanced analysis response in Japanese using Slack mrkdwn syntax
5. WHILE the AI analysis is in progress, THE Error_Detective SHALL post an interim "分析中..." (Analyzing...) message in the thread

### Requirement 5: Handle Dig Deeper Button Interaction

**User Story:** As a developer, I want the button click to be processed reliably, so that I always receive feedback after clicking "深掘りする".

#### Acceptance Criteria

1. WHEN the Interaction_Handler receives a block_actions payload with action_id "dig_deeper_bitbucket", THE Error_Detective SHALL acknowledge the request with HTTP 200 within 3 seconds
2. THE Error_Detective SHALL process the Bitbucket fetch and AI re-analysis asynchronously after acknowledging the Slack interaction
3. THE Error_Detective SHALL retrieve the original error message text from the cached ticket context or by fetching the thread parent message via Slack API
4. IF the Bitbucket API is unreachable or returns errors for all file references, THEN THE Error_Detective SHALL post an error message in the thread indicating code fetching failed

### Requirement 6: Bitbucket Configuration

**User Story:** As a system administrator, I want to configure Bitbucket access via environment variables, so that the deployment can connect to the correct repositories without code changes.

#### Acceptance Criteria

1. THE Error_Detective SHALL read Bitbucket connection settings from the following environment variables: `BITBUCKET_USERNAME`, `BITBUCKET_APP_PASSWORD`, and `BITBUCKET_BRANCH`
2. THE Error_Detective SHALL extract the workspace and repository slug dynamically from file paths in the error message (e.g., `bitbucket.org/mobilusall/mbv-api/` → workspace: `mobilusall`, repo: `mbv-api`)
3. THE Error_Detective SHALL consider Bitbucket integration configured when `BITBUCKET_APP_PASSWORD` is set to a non-empty value (or when unauthenticated access is acceptable for public repos)
4. IF `BITBUCKET_APP_PASSWORD` is not set, THEN THE Bitbucket_Client SHALL attempt unauthenticated access (for public repositories only)

### Requirement 7: Replace Local Context Gathering Entirely

**User Story:** As a developer, I want the system to use Bitbucket API exclusively for code context, so that the code context feature works reliably in cloud deployments without depending on local file access.

#### Acceptance Criteria

1. THE Error_Detective SHALL use the Bitbucket_Client as the sole mechanism for code context gathering, replacing the existing `REPO_ROOTS` local file reading entirely
2. THE Error_Detective SHALL remove the `REPO_ROOTS` environment variable handling and the `gatherContext()` function from `context.go`
3. IF Bitbucket is not configured, THE Error_Detective SHALL skip code context gathering entirely (no fallback to local files)
