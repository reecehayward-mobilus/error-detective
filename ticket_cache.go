package main

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// One-click ticket creation cache
type ticketContext struct {
	Channel     string
	ThreadTS    string
	ErrorText   string
	Explanation string
}

type cacheItem struct {
	Ctx    ticketContext
	Expire time.Time
}

var ticketCache = struct {
	mu sync.Mutex
	m  map[string]cacheItem
}{m: make(map[string]cacheItem)}

func getBacklogTokenTTL() time.Duration {
	v := strings.TrimSpace(os.Getenv("BACKLOG_TOKEN_TTL_HOURS"))
	if v == "" {
		return 24 * time.Hour
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 24 * time.Hour
	}
	return time.Duration(n) * time.Hour
}

func genToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func putTicketContext(ctx ticketContext, ttl time.Duration) (string, error) {
	tok, err := genToken()
	if err != nil {
		return "", err
	}
	ticketCache.mu.Lock()
	ticketCache.m[tok] = cacheItem{Ctx: ctx, Expire: time.Now().Add(ttl)}
	// cleanup
	for k, v := range ticketCache.m {
		if time.Now().After(v.Expire) {
			delete(ticketCache.m, k)
		}
	}
	ticketCache.mu.Unlock()
	return tok, nil
}

func takeTicketContext(tok string) (ticketContext, bool) {
	ticketCache.mu.Lock()
	defer ticketCache.mu.Unlock()
	it, ok := ticketCache.m[tok]
	if !ok || time.Now().After(it.Expire) {
		delete(ticketCache.m, tok)
		return ticketContext{}, false
	}
	// one-time use
	delete(ticketCache.m, tok)
	return it.Ctx, true
}
