package auth

import (
	"sync"
	"time"
)

// LoginLimiter is a fixed-window rate limiter keyed by client IP, guarding the
// login endpoint against password guessing.
type LoginLimiter struct {
	mu     sync.Mutex
	hits   map[string][]time.Time
	limit  int
	window time.Duration
	now    func() time.Time
	lastGC time.Time
}

// NewLoginLimiter allows limit attempts per window per key.
func NewLoginLimiter(limit int, window time.Duration) *LoginLimiter {
	return &LoginLimiter{
		hits:   make(map[string][]time.Time),
		limit:  limit,
		window: window,
		now:    time.Now,
	}
}

// Allow records an attempt for key and reports whether it is within the limit.
func (l *LoginLimiter) Allow(key string) bool {
	t := l.now()
	cut := t.Add(-l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	if t.Sub(l.lastGC) > l.window {
		for k, ts := range l.hits {
			if len(ts) == 0 || ts[len(ts)-1].Before(cut) {
				delete(l.hits, k)
			}
		}
		l.lastGC = t
	}

	kept := l.hits[key][:0]
	for _, ts := range l.hits[key] {
		if ts.After(cut) {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= l.limit {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, t)
	return true
}

// Reset clears the attempt history for key (call on a successful login).
func (l *LoginLimiter) Reset(key string) {
	l.mu.Lock()
	delete(l.hits, key)
	l.mu.Unlock()
}
