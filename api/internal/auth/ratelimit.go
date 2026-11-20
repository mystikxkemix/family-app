package auth

import (
	"sync"
	"time"
)

// Limiter is a token bucket per key (an IP address, a username...). Each key
// starts with "burst" tokens, one request spends one token, and tokens come
// back at "rate" per second.
type Limiter struct {
	mu      sync.Mutex
	rate    float64
	burst   float64
	buckets map[string]*bucket
	now     func() time.Time
	// lastSweep is when idle buckets were last removed.
	lastSweep time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

// NewLimiter returns a limiter that allows "burst" requests at once, then
// "perMinute" requests per minute.
func NewLimiter(perMinute float64, burst int) *Limiter {
	return &Limiter{
		rate:    perMinute / 60,
		burst:   float64(burst),
		buckets: make(map[string]*bucket),
		now:     time.Now,
	}
}

// Allow spends one token for the key and reports whether one was left.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens = min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// sweep drops the buckets that are full again, so the map does not grow
// forever. It runs at most once a minute.
func (l *Limiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < time.Minute {
		return
	}
	l.lastSweep = now
	for key, b := range l.buckets {
		if b.tokens+now.Sub(b.last).Seconds()*l.rate >= l.burst {
			delete(l.buckets, key)
		}
	}
}
