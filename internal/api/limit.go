package api

import (
	"sync"
	"time"
)

type bucket struct {
	tokens  float64
	updated time.Time
}

// Limiter is an in-process token bucket. One agent's limit cannot spend another's.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	now     func() time.Time
}

func NewLimiter() *Limiter {
	return &Limiter{buckets: map[string]*bucket{}, now: time.Now}
}

// sweepThreshold bounds the bucket map: keys are per-client (including real
// client IPs from X-Forwarded-For in public mode), so the map would grow
// without bound. Above the threshold, buckets idle for over a minute are
// dropped — an evicted client simply starts with a full bucket again.
const sweepThreshold = 1 << 16

func (l *Limiter) Allow(key string, perMinute float64) bool {
	if l == nil || perMinute <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.buckets) > sweepThreshold {
		for k, b := range l.buckets {
			if now.Sub(b.updated) > time.Minute {
				delete(l.buckets, k)
			}
		}
	}
	b := l.buckets[key]
	if b == nil {
		l.buckets[key] = &bucket{tokens: perMinute - 1, updated: now}
		return true
	}
	b.tokens += now.Sub(b.updated).Seconds() * (perMinute / 60)
	if b.tokens > perMinute {
		b.tokens = perMinute
	}
	b.updated = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
