package middleware

import (
	"net/http"
	"sync"
	"time"
)

type tokenBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64
	lastRefill time.Time
}

func (tb *tokenBucket) allow() bool {
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
	tb.lastRefill = now

	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}
	return false
}

type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*tokenBucket
	rps     float64
	burst   float64
}

func NewRateLimiter(rps, burst float64) *RateLimiter {
	rl := &RateLimiter{
		buckets: make(map[string]*tokenBucket),
		rps:     rps,
		burst:   burst,
	}
	// Cleanup old buckets periodically
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		for key, bucket := range rl.buckets {
			if time.Since(bucket.lastRefill) > 10*time.Minute {
				delete(rl.buckets, key)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	bucket, exists := rl.buckets[key]
	if !exists {
		bucket = &tokenBucket{
			tokens:     rl.burst,
			maxTokens:  rl.burst,
			refillRate: rl.rps,
			lastRefill: time.Now(),
		}
		rl.buckets[key] = bucket
	}

	return bucket.allow()
}

func RateLimitMiddleware(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Use user ID if authenticated, otherwise use IP
			key := GetUserID(r.Context())
			if key == "" {
				key = r.RemoteAddr
			}

			if !limiter.Allow(key) {
				http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
