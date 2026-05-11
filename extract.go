package main

import "strings"

// extractFullSlackMessage builds a comprehensive plain-text from a Slack message event,
// including top-level text, blocks (section text/fields), and attachments.
func extractFullSlackMessage(event map[string]any) string {
	var parts []string
	// Top-level text
	if t, ok := event["text"].(string); ok && strings.TrimSpace(t) != "" {
		parts = append(parts, t)
	}
	// Blocks (best effort for "section" blocks)
	if bs, ok := event["blocks"].([]any); ok {
		for _, bi := range bs {
			bm, ok := bi.(map[string]any)
			if !ok {
				continue
			}
			btype, _ := bm["type"].(string)
			if btype == "section" {
				if txtObj, ok := bm["text"].(map[string]any); ok {
					if tt, ok := txtObj["text"].(string); ok && strings.TrimSpace(tt) != "" {
						parts = append(parts, tt)
					}
				}
				if fields, ok := bm["fields"].([]any); ok {
					for _, fi := range fields {
						if fm, ok := fi.(map[string]any); ok {
							if ft, ok := fm["text"].(string); ok && strings.TrimSpace(ft) != "" {
								parts = append(parts, ft)
							}
						}
					}
				}
			}
			// Other block types (rich_text, context, etc.) are ignored for now.
		}
	}
	// Attachments (classic)
	if atts, ok := event["attachments"].([]any); ok {
		for _, ai := range atts {
			am, ok := ai.(map[string]any)
			if !ok {
				continue
			}
			if v, ok := am["pretext"].(string); ok && strings.TrimSpace(v) != "" {
				parts = append(parts, v)
			}
			if v, ok := am["title"].(string); ok && strings.TrimSpace(v) != "" {
				parts = append(parts, v)
			}
			if v, ok := am["text"].(string); ok && strings.TrimSpace(v) != "" {
				parts = append(parts, v)
			}
			if fields, ok := am["fields"].([]any); ok {
				for _, fi := range fields {
					if fm, ok := fi.(map[string]any); ok {
						name, _ := fm["title"].(string)
						val, _ := fm["value"].(string)
						s := strings.TrimSpace(strings.TrimSpace(name) + ": " + strings.TrimSpace(val))
						if s != ":" && strings.TrimSpace(s) != "" {
							parts = append(parts, s)
						}
					}
				}
			}
		}
	}
	// Join and lightly cap to avoid extremely large prompts
	joined := strings.Join(parts, "\n")
	const maxLen = 20000
	if len(joined) > maxLen {
		joined = joined[:maxLen-12] + "\n... [truncated]"
	}
	return joined
}
