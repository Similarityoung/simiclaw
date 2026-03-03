package gateway

import (
	"math"
	"sync"
	"time"
)

type limiter struct {
	mu        sync.Mutex
	rate      float64
	burst     float64
	buckets   map[string]*bucket
	gcCounter int
}

type bucket struct {
	tokens float64
	last   time.Time
}

const gcInterval = 256
const bucketExpiry = 5 * time.Minute

func newLimiter(rate, burst float64) *limiter {
	return &limiter{rate: rate, burst: burst, buckets: map[string]*bucket{}}
}

func (l *limiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.gcCounter++
	if l.gcCounter >= gcInterval {
		l.gcCounter = 0
		for k, b := range l.buckets {
			if now.Sub(b.last) > bucketExpiry {
				delete(l.buckets, k)
			}
		}
	}
	b, ok := l.buckets[key]
	if !ok {
		l.buckets[key] = &bucket{tokens: l.burst - 1, last: now}
		return true
	}
	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens = math.Min(l.burst, b.tokens+elapsed*l.rate)
		b.last = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
