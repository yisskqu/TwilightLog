package auth

import (
	"strings"
	"sync"
	"time"
)

type loginLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	entries map[string]limiterEntry
}

type limiterEntry struct {
	count   int
	resetAt time.Time
}

func newLoginLimiter(limit int, window time.Duration) *loginLimiter {
	return &loginLimiter{
		limit:   limit,
		window:  window,
		entries: map[string]limiterEntry{},
	}
}

func (l *loginLimiter) allow(keys ...string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, key := range keys {
		entry, ok := l.entries[key]
		if ok && now.Before(entry.resetAt) && entry.count >= l.limit {
			return false
		}
	}
	return true
}

func (l *loginLimiter) fail(keys ...string) {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, key := range keys {
		entry := l.entries[key]
		if !now.Before(entry.resetAt) {
			entry = limiterEntry{count: 0, resetAt: now.Add(l.window)}
		}
		entry.count++
		l.entries[key] = entry
	}
}

func (l *loginLimiter) reset(keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, key := range keys {
		delete(l.entries, key)
	}
}

func loginKeys(ip, username string) []string {
	username = strings.ToLower(strings.TrimSpace(username))
	return []string{"ip:" + ip, "ip-user:" + ip + ":" + username}
}
