# Implementation Plan: Monolith Refactor

## Overview

Decompose `main.go` (~600 lines) into 8 focused files within `package main` at the repository root. Each file encapsulates a single logical concern. No behavioral changes, no new dependencies, no new packages. Replace all `interface{}` with `any`. Existing tests must pass without modification.

## Tasks

- [x]   1. Create dedup.go — Event deduplication store
    - [x] 1.1 Extract deduplication logic into dedup.go
        - Create `dedup.go` with `package main`
        - Move the `processed` variable (struct with mutex and map), `hasProcessed()`, and `markProcessed()` functions from `main.go` into `dedup.go`
        - Add only the imports needed by this file (`sync`, `time`)
        - Remove the moved code from `main.go`
        - Verify `go build ./...` succeeds
        - _Requirements: 1.2, 2.1, 2.2, 3.3_

    - [x] 1.2 Write property test for dedup store idempotence and isolation
        - **Property 1: Dedup store idempotence and isolation**
        - Use `testing/quick` with minimum 100 iterations
        - Test that marking an event ID as processed causes `hasProcessed` to return `true`
        - Test that marking one event ID does not affect `hasProcessed` for a different event ID
        - Add comment tag: `// Feature: monolith-refactor, Property 1: Dedup store idempotence and isolation`
        - **Validates: Requirements 3.3**

- [x]   2. Create ticket_cache.go — One-time-use token cache
    - [x] 2.1 Extract ticket cache logic into ticket_cache.go
        - Create `ticket_cache.go` with `package main`
        - Move `ticketContext` struct, `cacheItem` struct, `ticketCache` variable, `getBacklogTokenTTL()`, `genToken()`, `putTicketContext()`, and `takeTicketContext()` from `main.go`
        - Add only the imports needed (`crypto/rand`, `encoding/hex`, `os`, `strconv`, `strings`, `sync`, `time`)
        - Remove the moved code from `main.go`
        - Verify `go build ./...` succeeds
        - _Requirements: 1.8, 2.1, 2.2, 3.8_

    - [x] 2.2 Write property test for ticket cache one-time-use semantics
        - **Property 3: Ticket cache one-time-use semantics**
        - Use `testing/quick` with minimum 100 iterations
        - Test that `putTicketContext` produces a token, and `takeTicketContext` returns the original context exactly once
        - Test that a second `takeTicketContext` with the same token returns `false`
        - Add comment tag: `// Feature: monolith-refactor, Property 3: Ticket cache one-time-use semantics`
        - **Validates: Requirements 3.8**

- [x]   3. Create backlog.go — Backlog REST API client
    - [x] 3.1 Extract Backlog integration into backlog.go
        - Create `backlog.go` with `package main`
        - Move `backlogConfigured()`, `generateIssueSummary()`, `buildBacklogIssue()`, `backlogProjectIDCache` variable, `resolveProjectID()`, and `createBacklogIssue()` from `main.go`
        - Add only the imports needed (`encoding/json`, `fmt`, `io`, `net/http`, `net/url`, `os`, `strconv`, `strings`, `sync`)
        - Remove the moved code from `main.go`
        - Verify `go build ./...` succeeds
        - _Requirements: 1.3, 2.1, 2.2, 3.8_

- [x]   4. Create slack.go — Slack Web API helpers
    - [x] 4.1 Extract Slack helpers into slack.go
        - Create `slack.go` with `package main`
        - Move `verifySlackSignature()`, `postToSlack()`, `postToSlackWithCreateButton()`, `postToSlackSimple()`, `getSlackPermalink()`, and `getParentMessageText()` from `main.go`
        - Replace all `interface{}` with `any` in the moved code
        - Add only the imports needed (`bytes`, `crypto/hmac`, `crypto/sha256`, `encoding/hex`, `encoding/json`, `fmt`, `net/http`, `net/url`, `os`, `strings`)
        - Remove the moved code from `main.go`
        - Verify `go build ./...` succeeds
        - _Requirements: 1.4, 2.1, 2.2, 3.7_

- [x]   5. Create ai.go — AI provider delegation
    - [x] 5.1 Extract AI logic into ai.go
        - Create `ai.go` with `package main`
        - Move `explainError()` from `main.go`
        - Add only the imports needed (`context`, `fmt`, `os`, `strings`, plus the `genai`, `openai`, and `option` packages)
        - Remove the moved code from `main.go`
        - Verify `go build ./...` succeeds
        - _Requirements: 1.5, 2.1, 2.2, 3.6_

- [x]   6. Create extract.go — Slack message extraction
    - [x] 6.1 Extract message parsing into extract.go
        - Create `extract.go` with `package main`
        - Move `extractFullSlackMessage()` from `main.go`
        - Replace all `interface{}` with `any` in the moved code
        - Add only the imports needed (`strings`)
        - Remove the moved code from `main.go`
        - Verify `go build ./...` succeeds
        - _Requirements: 1.6, 2.1, 2.2, 3.4_

    - [x] 6.2 Write property test for message extraction completeness
        - **Property 2: Message extraction completeness**
        - Use `testing/quick` with minimum 100 iterations
        - Test that for any event map with non-empty top-level `text`, the output contains that text
        - Test that for any event map with attachment `pretext`, `title`, or `text` fields, those values appear in the output
        - Add comment tag: `// Feature: monolith-refactor, Property 2: Message extraction completeness`
        - **Validates: Requirements 3.4**

- [x]   7. Create context.go — Code context gathering
    - [x] 7.1 Extract context gathering into context.go
        - Create `context.go` with `package main`
        - Move `gatherContext()` from `main.go`
        - Add only the imports needed (`fmt`, `os`, `path/filepath`, `regexp`, `strconv`, `strings`)
        - Remove the moved code from `main.go`
        - Verify `go build ./...` succeeds
        - _Requirements: 1.7, 2.1, 2.2, 3.5_

- [x]   8. Checkpoint — Verify build and tests pass after extraction
    - Ensure `go build ./...` succeeds
    - Ensure `go vet ./...` produces no warnings
    - Ensure `go test ./...` passes (existing `TestExtractFullSlackMessage_ExamplePayload` must pass without modification)
    - Ensure all tests pass, ask the user if questions arise.
    - _Requirements: 4.1, 4.2, 4.3, 5.2, 5.3_

- [x]   9. Finalize main.go — Minimal entry point
    - [x] 9.1 Clean up main.go to contain only router setup and handlers
        - Ensure `main.go` contains only: `package main`, necessary imports, `main()` function with Gin router setup, handler registration, and `r.Run(":8080")`
        - Replace all remaining `interface{}` with `any` in handler code
        - Remove any unused imports
        - Verify `main.go` is under 80 lines of code (excluding blank lines and comments)
        - Verify `go build ./...` succeeds
        - _Requirements: 6.1, 6.2, 6.3, 5.1_

- [x]   10. Replace all interface{} with any across all files
    - [x] 10.1 Global interface{} → any replacement
        - Search all `.go` files for remaining `interface{}` occurrences
        - Replace with `any` (Go 1.18+ type alias)
        - Verify `go build ./...` succeeds
        - Verify `go vet ./...` produces no warnings
        - _Requirements: 5.1, 5.2, 5.3_

- [x]   11. Final checkpoint — Full verification
    - Run `go build -o error-detective .` and confirm it succeeds
    - Run `go vet ./...` and confirm no warnings
    - Run `go test ./...` and confirm all tests pass
    - Verify `main.go` is under 80 lines (excluding blank lines and comments)
    - Verify no `interface{}` occurrences remain in any `.go` file
    - Verify `go.mod` and `go.sum` are unchanged
    - Ensure all tests pass, ask the user if questions arise.
    - _Requirements: 2.3, 3.1, 4.1, 4.2, 4.3, 5.1, 5.2, 5.3, 6.2, 7.1, 7.2, 7.3_

## Notes

- Tasks marked with `*` are optional and can be skipped for faster MVP
- Each task references specific requirements for traceability
- Checkpoints ensure incremental validation after each extraction step
- Property tests use Go's standard `testing/quick` package (no new dependencies)
- All files remain in `package main` at the repository root — no sub-packages
- Handler closures stay inline in `main.go` per design decision
- The existing `main_test.go` must not be modified
