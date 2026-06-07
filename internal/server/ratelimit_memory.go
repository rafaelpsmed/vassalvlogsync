package server

import (
	"net/http"
	"sync"
	"time"
)

const (
	defaultRate  = 30
	defaultBurst = 60
	cleanupEvery = 5 * time.Minute
	staleAfter   = 10 * time.Minute
)

type visitor struct {
	tokens   float64
	lastSeen time.Time
}

type MemoryRateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     float64
	burst    int
}

func newMemoryRateLimiter(ratePerSec float64, burst int) *MemoryRateLimiter {
	if ratePerSec <= 0 {
		ratePerSec = defaultRate
	}
	if burst <= 0 {
		burst = defaultBurst
	}
	rl := &MemoryRateLimiter{
		visitors: make(map[string]*visitor),
		rate:     ratePerSec,
		burst:    burst,
	}
	go rl.cleanupLoop()
	return rl
}

func (rl *MemoryRateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[key]
	now := time.Now()
	if !exists {
		v = &visitor{tokens: float64(rl.burst) - 1, lastSeen: now}
		rl.visitors[key] = v
		return true
	}

	elapsed := now.Sub(v.lastSeen).Seconds()
	v.tokens += elapsed * rl.rate
	if v.tokens > float64(rl.burst) {
		v.tokens = float64(rl.burst)
	}
	v.lastSeen = now

	if v.tokens < 1 {
		return false
	}
	v.tokens--
	return true
}

func (rl *MemoryRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !rl.Allow(ip) {
			http.Error(w, "muitas requisições — aguarde", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (rl *MemoryRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(cleanupEvery)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-staleAfter)
		for ip, v := range rl.visitors {
			if v.lastSeen.Before(cutoff) {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}
