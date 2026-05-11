package main

import (
	"strings"
	"testing"
	"testing/quick"
	"time"
)

// Feature: monolith-refactor, Property 1: Dedup store idempotence and isolation
// Validates: Requirements 3.3

func TestProperty_DedupStoreIdempotence(t *testing.T) {
	f := func(id string) bool {
		if id == "" {
			return true // skip empty strings
		}
		markProcessed(id)
		return hasProcessed(id)
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("idempotence property failed: %v", err)
	}
}

func TestProperty_DedupStoreIsolation(t *testing.T) {
	// Reset the dedup store before this test to ensure clean state
	processed.mu.Lock()
	processed.m = make(map[string]time.Time)
	processed.mu.Unlock()

	f := func(id1, id2 string) bool {
		if id1 == "" || id2 == "" || id1 == id2 {
			return true // skip trivial cases
		}

		// Reset state for each iteration to ensure isolation
		processed.mu.Lock()
		delete(processed.m, id1)
		delete(processed.m, id2)
		processed.mu.Unlock()

		markProcessed(id1)
		// id2 was never marked, so hasProcessed(id2) should be false
		return !hasProcessed(id2)
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("isolation property failed: %v", err)
	}
}

// Feature: monolith-refactor, Property 3: Ticket cache one-time-use semantics
// Validates: Requirements 3.8

func TestProperty_TicketCacheOneTimeUse(t *testing.T) {
	f := func(channel, threadTS, errorText, explanation string) bool {
		ctx := ticketContext{
			Channel:     channel,
			ThreadTS:    threadTS,
			ErrorText:   errorText,
			Explanation: explanation,
		}

		// Put the context and get a token
		token, err := putTicketContext(ctx, 1*time.Hour)
		if err != nil {
			t.Logf("putTicketContext error: %v", err)
			return false
		}

		// First take should return the original context and true
		got, ok := takeTicketContext(token)
		if !ok {
			return false
		}
		if got.Channel != ctx.Channel || got.ThreadTS != ctx.ThreadTS ||
			got.ErrorText != ctx.ErrorText || got.Explanation != ctx.Explanation {
			return false
		}

		// Second take with the same token should return false (one-time use)
		_, ok2 := takeTicketContext(token)
		return !ok2
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("ticket cache one-time-use property failed: %v", err)
	}
}

// Feature: monolith-refactor, Property 2: Message extraction completeness
// Validates: Requirements 3.4

func TestProperty_MessageExtractionCompleteness_TopLevelText(t *testing.T) {
	f := func(text string) bool {
		if text == "" {
			return true // skip empty strings
		}
		event := map[string]any{
			"text": text,
		}
		result := extractFullSlackMessage(event)
		return strings.Contains(result, text)
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("message extraction completeness (top-level text) property failed: %v", err)
	}
}

func TestProperty_MessageExtractionCompleteness_AttachmentFields(t *testing.T) {
	f := func(pretext, title, text string) bool {
		if pretext == "" && title == "" && text == "" {
			return true // skip all-empty case
		}

		attachment := map[string]any{}
		if pretext != "" {
			attachment["pretext"] = pretext
		}
		if title != "" {
			attachment["title"] = title
		}
		if text != "" {
			attachment["text"] = text
		}

		event := map[string]any{
			"attachments": []any{attachment},
		}
		result := extractFullSlackMessage(event)

		if pretext != "" && !strings.Contains(result, pretext) {
			return false
		}
		if title != "" && !strings.Contains(result, title) {
			return false
		}
		if text != "" && !strings.Contains(result, text) {
			return false
		}
		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Errorf("message extraction completeness (attachment fields) property failed: %v", err)
	}
}
