# Requirements Document

## Introduction

Refactor the Error Detective Slack bot from a single-file monolith (`main.go`, ~600 lines) into a well-organized multi-file structure within the same `package main`. The goal is improved maintainability, readability, and separation of concerns — without changing any external behavior or adding new features.

## Glossary

- **Application**: The Error Detective Slack bot as a whole
- **Module**: A logical grouping of related functions placed in a dedicated `.go` file
- **Deduplication_Store**: The in-memory event deduplication mechanism (map + mutex)
- **Ticket_Cache**: The in-memory one-time-use token cache for Backlog ticket creation context
- **AI_Provider**: The abstraction over OpenAI and Gemini that generates error explanations
- **Slack_Client**: Functions responsible for posting messages and retrieving data from the Slack Web API
- **Backlog_Client**: Functions responsible for creating issues via the Backlog REST API v2
- **Message_Extractor**: Logic that parses Slack event payloads into plain-text error messages
- **Context_Gatherer**: Logic that resolves file:line references from stack traces and extracts code snippets
- **Handler**: An HTTP endpoint handler registered with the Gin router

## Requirements

### Requirement 1: File Decomposition by Concern

**User Story:** As a developer, I want the monolith split into separate files by logical concern, so that I can navigate and modify each subsystem independently.

#### Acceptance Criteria

1. THE Application SHALL be organized into separate `.go` files where each file contains functions belonging to a single logical concern
2. THE Application SHALL place event deduplication logic (Deduplication_Store) in a dedicated file
3. THE Application SHALL place Backlog REST API integration (Backlog_Client) in a dedicated file
4. THE Application SHALL place Slack Web API interactions (Slack_Client) in a dedicated file
5. THE Application SHALL place AI provider logic (AI_Provider) in a dedicated file
6. THE Application SHALL place Slack message extraction (Message_Extractor) in a dedicated file
7. THE Application SHALL place code context gathering (Context_Gatherer) in a dedicated file
8. THE Application SHALL place ticket context caching (Ticket_Cache) in a dedicated file
9. THE Application SHALL keep HTTP handler registration and server startup in `main.go`

### Requirement 2: Preserve Package Structure

**User Story:** As a developer, I want the refactored code to remain in `package main` with no new sub-packages, so that the build and deployment process stays unchanged.

#### Acceptance Criteria

1. THE Application SHALL keep all `.go` files in `package main` at the repository root
2. THE Application SHALL not introduce any new Go sub-packages or directories for source code
3. THE Application SHALL continue to produce a single binary via `go build -o error-detective .`

### Requirement 3: Preserve External Behavior

**User Story:** As a developer, I want the refactored application to behave identically to the current monolith, so that no regressions are introduced.

#### Acceptance Criteria

1. THE Application SHALL expose the same HTTP endpoints (`/slack/events` and `/slack/interactions`) on port 8080
2. THE Application SHALL handle Slack URL verification challenges identically
3. THE Application SHALL deduplicate Slack events using the same 15-minute TTL mechanism
4. THE Application SHALL extract messages from text, blocks, and attachments in the same manner
5. THE Application SHALL gather code context from `REPO_ROOTS` using the same file:line resolution logic
6. THE Application SHALL generate AI explanations via the same OpenAI and Gemini integration
7. THE Application SHALL post threaded Slack replies with Block Kit formatting identically
8. THE Application SHALL create Backlog issues with the same field mapping and one-time-use token behavior
9. THE Application SHALL read all configuration from the same environment variables

### Requirement 4: Preserve Test Compatibility

**User Story:** As a developer, I want existing tests to continue passing without modification, so that I have confidence the refactoring is safe.

#### Acceptance Criteria

1. THE Application SHALL keep existing test functions accessible and passing via `go test ./...`
2. WHEN tests reference internal functions (e.g., `extractFullSlackMessage`), THE Application SHALL ensure those functions remain exported or accessible within `package main`
3. THE Application SHALL not require changes to `main_test.go` for existing tests to pass

### Requirement 5: Consistent Code Style

**User Story:** As a developer, I want the refactored files to follow consistent Go conventions, so that the codebase is idiomatic and easy to read.

#### Acceptance Criteria

1. THE Application SHALL use `any` instead of `interface{}` in all refactored code
2. THE Application SHALL compile without errors using Go 1.25
3. THE Application SHALL pass `go vet ./...` without warnings

### Requirement 6: Minimal Main Entry Point

**User Story:** As a developer, I want `main.go` to serve only as the entry point and router setup, so that it provides a clear overview of the application's structure.

#### Acceptance Criteria

1. THE Application SHALL limit `main.go` to the `main()` function, Gin router setup, and handler registration
2. THE Application SHALL keep `main.go` under 80 lines of code (excluding blank lines and comments)
3. WHEN reading `main.go`, a developer SHALL be able to identify all HTTP endpoints and their handler locations without scrolling

### Requirement 7: Dependency and Vendor Compatibility

**User Story:** As a developer, I want the refactored project to work with the existing vendored dependencies, so that no network access is needed to build.

#### Acceptance Criteria

1. THE Application SHALL not add or remove any dependencies in `go.mod`
2. THE Application SHALL build successfully using the existing `vendor/` directory without running `go mod vendor`
3. THE Application SHALL not modify `go.mod` or `go.sum`
