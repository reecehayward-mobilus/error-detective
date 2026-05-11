# Tasks: Bitbucket Dig Deeper

## Task 1: Create bitbucket.go with configuration and file reference parsing

- [x] 1.1 Create `bitbucket.go` with `bitbucketConfigured()` function that returns true when `BITBUCKET_APP_PASSWORD` env var is non-empty (or unauthenticated access is acceptable)
- [x] 1.2 Implement `FileRef` struct with `Workspace` (string), `RepoSlug` (string), `Path` (string), and `Line` (int) fields
- [x] 1.3 Implement `normalizePath()` function that parses `bitbucket.org/{workspace}/{repo}/` from file paths and returns workspace, repo slug, and repository-relative path
- [x] 1.4 Implement `parseFileRefs(msg string, maxRefs int) []FileRef` that extracts `.go:<line>` references from error messages, parses workspace/repo from the path, normalizes to repo-relative paths, deduplicates, and caps at maxRefs (5)
- [x] 1.5 Write property-based tests for `parseFileRefs` and `normalizePath` (Properties 3, 4) and unit tests for `bitbucketConfigured` (Property 7)

## Task 2: Implement Bitbucket Cloud API file fetching

- [x] 2.1 Implement `fetchFileFromBitbucket(workspace, repoSlug, path string) (string, error)` that calls `GET /2.0/repositories/{workspace}/{repo_slug}/src/{branch}/{path}` with optional Basic Auth
- [x] 2.2 Implement `extractLines(content string, targetLine, above, below int) string` that extracts a context window clamped to file boundaries
- [x] 2.3 Implement `fetchBitbucketContext(refs []FileRef, maxTotalChars int) string` that fetches all refs, extracts context windows, skips failures, and enforces the 8000-char total limit
- [x] 2.4 Write property-based tests for `extractLines` (Property 5) and `fetchBitbucketContext` size limit (Property 6), plus unit tests for API URL construction and error skipping

## Task 3: Add "深掘りする" button to Slack messages

- [x] 3.1 Modify `postToSlackWithCreateButton` in `slack.go` to add a "深掘りする" button (action_id: "dig_deeper_bitbucket", plain_text label) to the actions block when `bitbucketConfigured()` returns true
- [x] 3.2 Store the ticket context token as the button value (reuse existing cache pattern) so the dig-deeper handler can retrieve the original error text
- [x] 3.3 Write property-based tests for button presence/omission (Properties 1, 2) and unit tests for button attributes (action_id, label)

## Task 4: Handle "dig_deeper_bitbucket" interaction in main.go

- [x] 4.1 Extend the `/slack/interactions` handler to recognize `action_id == "dig_deeper_bitbucket"` and route to a new handler function
- [x] 4.2 Implement the dig-deeper handler: acknowledge with HTTP 200, retrieve error text from cache (or fallback to `getParentMessageText`), then process asynchronously
- [x] 4.3 In the async goroutine: post "分析中..." interim message, call `parseFileRefs`, call `fetchBitbucketContext`, handle the "no references found" and "all fetches failed" error cases
- [x] 4.4 Write unit tests for interaction routing and error case messages

## Task 5: Implement enhanced AI analysis with code context

- [x] 5.1 Add `explainErrorWithContext(msg string, codeContext string) string` to `ai.go` that uses a specialized system prompt for code-level root cause analysis
- [x] 5.2 The specialized prompt should instruct the AI to reference actual source code lines, provide specific fix suggestions, and respond in Japanese with Slack mrkdwn
- [x] 5.3 In the dig-deeper async handler, call `explainErrorWithContext` and post the result as a threaded reply
- [x] 5.4 Write unit tests verifying the specialized prompt is used and both error message and code context are passed to the AI provider

## Task 6: Remove old gatherContext and REPO_ROOTS logic

- [x] 6.1 Delete the `gatherContext()` function and all `REPO_ROOTS` handling from `context.go` (or remove `context.go` entirely if it has no other purpose)
- [x] 6.2 Remove any references to `REPO_ROOTS` from the codebase (env loading, documentation comments, etc.)
- [x] 6.3 Ensure the initial AI explanation flow no longer calls `gatherContext()` — code context is only provided via the dig-deeper button using Bitbucket
- [x] 6.4 Write a test verifying that when Bitbucket is not configured, code context gathering is skipped entirely (no errors, no fallback)

## Task 7: Final integration testing and cleanup

- [x] 7.1 Run `go build ./...` to verify compilation
- [x] 7.2 Run `go test ./...` to verify all tests pass
- [x] 7.3 Run `go mod tidy` and `go mod vendor` if any new dependencies were added
- [x] 7.4 Verify the complete flow works end-to-end with mocked external services
