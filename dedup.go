package main

import (
	"sync"
	"time"
)

// simple in-memory deduplication for Slack event_ids
var processed = struct {
	mu sync.Mutex
	m  map[string]time.Time
}{m: make(map[string]time.Time)}

func hasProcessed(id string) bool {
	processed.mu.Lock()
	defer processed.mu.Unlock()
	if t, ok := processed.m[id]; ok {
		// expire after 15 minutes
		if time.Since(t) < 15*time.Minute {
			return true
		}
		delete(processed.m, id)
	}
	return false
}

func markProcessed(id string) {
	processed.mu.Lock()
	processed.m[id] = time.Now()
	// light cleanup to avoid unbounded growth
	if len(processed.m) > 1000 {
		cutoff := time.Now().Add(-15 * time.Minute)
		for k, v := range processed.m {
			if v.Before(cutoff) {
				delete(processed.m, k)
			}
		}
	}
	processed.mu.Unlock()
}
