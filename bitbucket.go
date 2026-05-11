package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// bitbucketBaseURL is the base URL for the Bitbucket Cloud API.
// Tests can override this to point to httptest.NewServer.
var bitbucketBaseURL = "https://api.bitbucket.org"

// bitbucketHTTPClient is used for Bitbucket API calls with a timeout.
var bitbucketHTTPClient = &http.Client{Timeout: 30 * time.Second}

const (
	maxFileRefs       = 5
	contextLinesAbove = 10
	contextLinesBelow = 10
	maxContextChars   = 8000
)

// bitbucketConfigured returns true when Bitbucket credentials are available
// (BITBUCKET_APP_PASSWORD is set and non-empty after trimming).
func bitbucketConfigured() bool {
	return strings.TrimSpace(os.Getenv("BITBUCKET_APP_PASSWORD")) != ""
}

// FileRef represents a parsed file path and line number from an error message.
type FileRef struct {
	Workspace string // Bitbucket workspace (e.g., "mobilusall")
	RepoSlug  string // Repository slug (e.g., "mbv-api" or "mbv-agi")
	Path      string // Repository-relative file path
	Line      int    // 1-based line number
}

// normalizePath parses a full file path containing bitbucket.org/{workspace}/{repo}/
// and returns the workspace, repo slug, and repository-relative path.
// If the path does not contain the expected pattern, all return values are empty.
func normalizePath(path string) (workspace, repoSlug, relativePath string) {
	const marker = "bitbucket.org/"
	idx := strings.Index(path, marker)
	if idx < 0 {
		return "", "", ""
	}
	// Everything after "bitbucket.org/"
	rest := path[idx+len(marker):]
	// Expected: {workspace}/{repo}/{relative-path...}
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 3 {
		return "", "", ""
	}
	workspace = parts[0]
	repoSlug = parts[1]
	relativePath = parts[2]
	if workspace == "" || repoSlug == "" || relativePath == "" {
		return "", "", ""
	}
	return workspace, repoSlug, relativePath
}

// fileLineRe matches .go:<line> patterns in error messages.
// It captures the full path and the line number.
var fileLineRe = regexp.MustCompile(`([\w./\\:_-]+\.go):(\d+)`)

// parseFileRefs extracts up to maxRefs file:line references from an error
// message. Parses workspace and repo slug from the path itself, normalizes
// paths to be repository-relative, deduplicates, and caps at maxRefs.
func parseFileRefs(msg string, maxRefs int) []FileRef {
	matches := fileLineRe.FindAllStringSubmatch(msg, -1)
	if len(matches) == 0 {
		return nil
	}

	var refs []FileRef
	seen := make(map[string]struct{})

	for _, m := range matches {
		if len(refs) >= maxRefs {
			break
		}
		fullPath := m[1]
		lineStr := m[2]

		workspace, repoSlug, relPath := normalizePath(fullPath)
		if workspace == "" {
			continue
		}

		line, err := strconv.Atoi(lineStr)
		if err != nil || line <= 0 {
			continue
		}

		// Deduplicate by workspace+repo+path+line
		key := workspace + "/" + repoSlug + "/" + relPath + ":" + lineStr
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		refs = append(refs, FileRef{
			Workspace: workspace,
			RepoSlug:  repoSlug,
			Path:      relPath,
			Line:      line,
		})
	}

	return refs
}

// fetchFileFromBitbucket fetches raw file content from Bitbucket Cloud API
// for the given workspace, repo slug, and path.
// Uses BITBUCKET_BRANCH env var for the branch (default "master").
// If BITBUCKET_USERNAME and BITBUCKET_APP_PASSWORD are set, uses Basic Auth.
// Returns the file content as a string, or an error on non-2xx status.
func fetchFileFromBitbucket(workspace, repoSlug, path string) (string, error) {
	branch := os.Getenv("BITBUCKET_BRANCH")
	if branch == "" {
		branch = "master"
	}

	url := fmt.Sprintf("%s/2.0/repositories/%s/%s/src/%s/%s",
		bitbucketBaseURL, workspace, repoSlug, branch, path)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	username := os.Getenv("BITBUCKET_USERNAME")
	appPassword := os.Getenv("BITBUCKET_APP_PASSWORD")
	if appPassword != "" {
		if username == "" {
			// Repository/Workspace/Project access token: use Bearer auth
			req.Header.Set("Authorization", "Bearer "+appPassword)
		} else {
			// Personal API token or app password: use Basic auth (email:token)
			creds := base64.StdEncoding.EncodeToString([]byte(username + ":" + appPassword))
			req.Header.Set("Authorization", "Basic "+creds)
		}
	}

	resp, err := bitbucketHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("bitbucket API returned status %d for %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// extractLines extracts a window of context lines around the target line.
// Returns lines from max(1, targetLine-above) to min(totalLines, targetLine+below),
// clamped to file boundaries. Lines are 1-indexed.
func extractLines(content string, targetLine, above, below int) string {
	if content == "" {
		return ""
	}

	lines := strings.Split(content, "\n")
	totalLines := len(lines)

	if targetLine < 1 {
		targetLine = 1
	}
	if targetLine > totalLines {
		targetLine = totalLines
	}

	start := targetLine - above
	if start < 1 {
		start = 1
	}

	end := targetLine + below
	if end > totalLines {
		end = totalLines
	}

	// Convert to 0-indexed for slice
	return strings.Join(lines[start-1:end], "\n")
}

// fetchBitbucketContext fetches code snippets from Bitbucket Cloud API for
// the given file references. Returns formatted context string capped at
// maxTotalChars. Skips files that return errors.
func fetchBitbucketContext(refs []FileRef, maxTotalChars int) string {
	var result strings.Builder

	for _, ref := range refs {
		content, err := fetchFileFromBitbucket(ref.Workspace, ref.RepoSlug, ref.Path)
		if err != nil {
			fmt.Printf("[dig-deeper] fetch failed for %s/%s/%s: %v\n", ref.Workspace, ref.RepoSlug, ref.Path, err)
			continue
		}

		extracted := extractLines(content, ref.Line, contextLinesAbove, contextLinesBelow)
		snippet := fmt.Sprintf("// %s/%s/%s:%d\n%s\n", ref.Workspace, ref.RepoSlug, ref.Path, ref.Line, extracted)

		if result.Len()+len(snippet) > maxTotalChars {
			break
		}

		result.WriteString(snippet)
	}

	return result.String()
}
