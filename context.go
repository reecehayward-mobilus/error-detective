package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// gatherContext extracts file:line references from an error message and returns
// small code snippets (a few lines of context) from matching files under any
// of the configured REPO_ROOTS (colon or comma separated absolute paths).
func gatherContext(msg string) string {
	rootsEnv := os.Getenv("REPO_ROOTS")
	if rootsEnv == "" {
		return ""
	}
	roots := strings.FieldsFunc(rootsEnv, func(r rune) bool { return r == ':' || r == ',' })
	for i, r := range roots {
		roots[i] = strings.TrimSpace(r)
	}

	fileLineRe := regexp.MustCompile(`([\w./\\_-]+\.go):(\d+)`)
	matches := fileLineRe.FindAllStringSubmatch(msg, -1)
	if len(matches) == 0 {
		return ""
	}
	maxFiles := 3
	var snippets []string
	seen := make(map[string]struct{})
	for _, m := range matches {
		if len(snippets) >= maxFiles {
			break
		}
		rel := m[1]
		lnStr := m[2]
		key := rel + ":" + lnStr
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		lineNum, _ := strconv.Atoi(lnStr)

		// Build candidate file paths:
		// 1) the path as-is
		// 2) if absolute like /go/src/bitbucket.org/... try to strip up to
		//    '/src/' or VCS host and rebase under each REPO_ROOTS.
		candidatePaths := []string{rel}
		if filepath.IsAbs(rel) {
			rebased := false
			// Try stripping up to '/src/'
			if idx := strings.Index(rel, "/src/"); idx >= 0 && idx+5 < len(rel) {
				relFromSrc := rel[idx+5:]
				for _, root := range roots {
					candidatePaths = append(candidatePaths, filepath.Join(root, relFromSrc))
				}
				rebased = true
			}
			// Try common VCS hosts
			if !rebased {
				for _, host := range []string{"bitbucket.org", "github.com", "gitlab.com"} {
					marker := "/" + host + "/"
					if idx := strings.Index(rel, marker); idx >= 0 {
						relFromHost := rel[idx+1:] // drop leading '/'
						for _, root := range roots {
							candidatePaths = append(candidatePaths, filepath.Join(root, relFromHost))
						}
						break
					}
				}
			}
		} else {
			for _, root := range roots {
				candidatePaths = append(candidatePaths, filepath.Join(root, rel))
			}
		}
		var chosen string
		for _, c := range candidatePaths {
			if info, err := os.Stat(c); err == nil && !info.IsDir() {
				chosen = c
				break
			}
		}
		if chosen == "" {
			continue
		}
		data, err := os.ReadFile(chosen)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		start := lineNum - 4
		if start < 1 {
			start = 1
		}
		end := lineNum + 4
		if end > len(lines) {
			end = len(lines)
		}
		var b strings.Builder
		b.WriteString(fmt.Sprintf("// %s:%d\n", chosen, lineNum))
		for i := start; i <= end; i++ {
			b.WriteString(fmt.Sprintf("%4d | %s\n", i, lines[i-1]))
		}
		snippets = append(snippets, b.String())
	}
	if len(snippets) == 0 {
		return ""
	}
	joined := strings.Join(snippets, "\n")
	if len(joined) > 4000 { // cap size
		joined = joined[:4000] + "\n... (truncated)"
	}
	return joined
}
