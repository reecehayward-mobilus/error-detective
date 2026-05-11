package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
)

func backlogConfigured() bool {
	return os.Getenv("BACKLOG_BASE_URL") != "" && os.Getenv("BACKLOG_API_KEY") != "" && os.Getenv("BACKLOG_PROJECT_KEY") != ""
}

func generateIssueSummary(_ string, explanation string) string {
	// Simplified: take first meaningful line after the 【原因】 heading.
	maxLen := 120
	if explanation == "" {
		return "エラー通知botによるバグ"
	}
	lines := strings.Split(explanation, "\n")
	foundHeading := false
	for i, raw := range lines {
		l := strings.TrimSpace(raw)
		if l == "" {
			continue
		}
		if strings.Contains(l, "【原因】") {
			// scan forward for first bullet/non-empty line
			for _, next := range lines[i+1:] {
				nextTrim := strings.TrimSpace(next)
				if nextTrim == "" {
					continue
				}
				// remove markdown bullet markers
				nextTrim = strings.TrimLeft(nextTrim, "-•*")
				nextTrim = strings.TrimSpace(nextTrim)
				if nextTrim == "" {
					continue
				}
				if len(nextTrim) > maxLen {
					nextTrim = nextTrim[:maxLen-3] + "..."
				}
				return nextTrim
			}
			foundHeading = true
			break
		}
	}
	if foundHeading {
		return "原因不明のエラー"
	}
	// Fallback to first non-empty trimmed line
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			if len(l) > maxLen {
				l = l[:maxLen-3] + "..."
			}
			return l
		}
	}
	return "エラー通知botによるバグ"
}

func buildBacklogIssue(ctx ticketContext) (summary string, description string) {
	summary = generateIssueSummary(ctx.ErrorText, ctx.Explanation)
	fmt.Println("Generated issue summary:", summary)
	var b strings.Builder
	b.WriteString("[自動作成]\n")
	b.WriteString("Slackスレッド: (後で追加)\n\n")
	b.WriteString("【説明】\n")
	b.WriteString(ctx.Explanation)
	b.WriteString("\n\n【エラーログ】\n")
	b.WriteString(ctx.ErrorText)
	description = b.String()
	return
}

// Resolve Backlog project ID from projectKey (cached).
var backlogProjectIDCache = struct {
	mu sync.Mutex
	m  map[string]int64
}{m: make(map[string]int64)}

func resolveProjectID(baseURL, apiKey, projectKey string) (int64, error) {
	// Allow override via BACKLOG_PROJECT_ID env (string -> int64)
	if override := strings.TrimSpace(os.Getenv("BACKLOG_PROJECT_ID")); override != "" {
		if id, err := strconv.ParseInt(override, 10, 64); err == nil {
			return id, nil
		}
	}
	backlogProjectIDCache.mu.Lock()
	if id, ok := backlogProjectIDCache.m[projectKey]; ok {
		backlogProjectIDCache.mu.Unlock()
		return id, nil
	}
	backlogProjectIDCache.mu.Unlock()

	endpoint := strings.TrimRight(baseURL, "/") + "/api/v2/projects?apiKey=" + url.QueryEscape(apiKey)
	req, _ := http.NewRequest("GET", endpoint, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("projects list failed: %s", string(body))
	}
	var projects []struct {
		ID         int64  `json:"id"`
		ProjectKey string `json:"projectKey"`
	}
	if err := json.Unmarshal(body, &projects); err != nil {
		return 0, err
	}
	for _, p := range projects {
		if strings.EqualFold(p.ProjectKey, projectKey) {
			backlogProjectIDCache.mu.Lock()
			backlogProjectIDCache.m[projectKey] = p.ID
			backlogProjectIDCache.mu.Unlock()
			return p.ID, nil
		}
	}
	return 0, fmt.Errorf("projectKey '%s' not found", projectKey)
}

func createBacklogIssue(projectKey, baseURL, apiKey, summary, description string) (issueKey, issueURL string, err error) {
	projectID, err := resolveProjectID(baseURL, apiKey, projectKey)
	if err != nil {
		return "", "", err
	}
	// Required IDs: issueTypeId & priorityId must exist; we surface clearer errors.
	issueTypeID := strings.TrimSpace(os.Getenv("BACKLOG_ISSUE_TYPE_ID"))
	if issueTypeID == "" {
		return "", "", fmt.Errorf("BACKLOG_ISSUE_TYPE_ID not set (Backlog API requires issueTypeId)")
	}
	priorityID := strings.TrimSpace(os.Getenv("BACKLOG_PRIORITY_ID"))
	if priorityID == "" {
		return "", "", fmt.Errorf("BACKLOG_PRIORITY_ID not set (Backlog API requires priorityId)")
	}

	form := url.Values{}
	form.Set("projectId", strconv.FormatInt(projectID, 10))
	form.Set("summary", summary)
	form.Set("issueTypeId", issueTypeID)
	form.Set("priorityId", priorityID)
	form.Set("description", description)

	// Optional assignee
	if assignee := strings.TrimSpace(os.Getenv("BACKLOG_ASSIGNEE_ID")); assignee != "" {
		form.Set("assigneeId", assignee)
	}
	// Optional category/version/milestone lists (comma separated IDs)
	for _, pair := range []struct{ env, key string }{
		{"BACKLOG_CATEGORY_IDS", "categoryId[]"},
		{"BACKLOG_VERSION_IDS", "versionId[]"},
		{"BACKLOG_MILESTONE_IDS", "milestoneId[]"},
		{"BACKLOG_NOTIFIED_USER_IDS", "notifiedUserId[]"},
	} {
		raw := strings.TrimSpace(os.Getenv(pair.env))
		if raw == "" {
			continue
		}
		for _, id := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' }) {
			id = strings.TrimSpace(id)
			if id != "" {
				form.Add(pair.key, id)
			}
		}
	}
	// Custom fields: BACKLOG_CUSTOM_FIELDS format: fieldId=value;fieldId=value
	cfRaw := strings.TrimSpace(os.Getenv("BACKLOG_CUSTOM_FIELDS"))
	if cfRaw != "" {
		parts := strings.FieldsFunc(cfRaw, func(r rune) bool { return r == ';' || r == ',' })
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			kv := strings.SplitN(p, "=", 2)
			if len(kv) != 2 {
				continue
			}
			fieldID := strings.TrimSpace(kv[0])
			value := strings.TrimSpace(kv[1])
			if fieldID != "" && value != "" {
				form.Set("customField_"+fieldID, value)
			}
		}
	}

	endpoint := strings.TrimRight(baseURL, "/") + "/api/v2/issues?apiKey=" + url.QueryEscape(apiKey)
	req, _ := http.NewRequest("POST", endpoint, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("backlog create failed: %s", string(body))
	}
	var res struct {
		IssueKey string `json:"issueKey"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return "", "", err
	}
	if res.IssueKey == "" {
		return "", "", fmt.Errorf("backlog create returned no key: %s", string(body))
	}
	issueURL = strings.TrimRight(baseURL, "/") + "/view/" + res.IssueKey
	return res.IssueKey, issueURL, nil
}
