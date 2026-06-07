package server

import (
	"net/http"
	"os"
	"strconv"
)

type RateLimiter interface {
	Allow(key string) bool
	Middleware(next http.Handler) http.Handler
}

func NewRateLimiterFromEnv() RateLimiter {
	driver := envOr("RATE_LIMIT_DRIVER", "memory")
	switch driver {
	case "redis":
		rl := newRedisRateLimiter()
		if rl != nil {
			return rl
		}
	}
	rate := parseEnvFloat("RATE_LIMIT_RATE", defaultRate)
	burst := parseEnvInt("RATE_LIMIT_BURST", defaultBurst)
	return newMemoryRateLimiter(rate, burst)
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		for i := len(fwd) - 1; i >= 0; i-- {
			if fwd[i] == ',' || fwd[i] == ' ' {
				return fwd[i+1:]
			}
		}
		return fwd
	}
	host := r.RemoteAddr
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			return host[:i]
		}
	}
	return host
}

func parseEnvFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f
		}
	}
	return fallback
}

func parseEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i > 0 {
			return i
		}
	}
	return fallback
}
