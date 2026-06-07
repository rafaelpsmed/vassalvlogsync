package server

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisRateLimiter struct {
	client *redis.Client
	rate   float64
	burst  int
}

func newRedisRateLimiter() *RedisRateLimiter {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		return nil
	}
	password := os.Getenv("REDIS_PASSWORD")
	db := 0
	if v := os.Getenv("REDIS_DB"); v != "" {
		db, _ = strconv.Atoi(v)
	}

	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("ratelimit redis: %v — usando memory", err)
		return nil
	}

	rate := parseEnvFloat("RATE_LIMIT_RATE", defaultRate)
	burst := parseEnvInt("RATE_LIMIT_BURST", defaultBurst)

	return &RedisRateLimiter{client: client, rate: rate, burst: burst}
}

func (rl *RedisRateLimiter) Allow(key string) bool {
	ctx := context.Background()
	now := time.Now().UnixMilli()
	windowMs := int64(1000) // 1 second sliding window

	pipe := rl.client.Pipeline()
	pipe.ZRemRangeByScore(ctx, "rl:"+key, "0", strconv.FormatInt(now-windowMs, 10))
	pipe.ZCard(ctx, "rl:"+key)
	pipe.ZAdd(ctx, "rl:"+key, redis.Z{Score: float64(now), Member: now})
	pipe.Expire(ctx, "rl:"+key, 2*time.Second)

	cmds, err := pipe.Exec(ctx)
	if err != nil {
		return true // fail open on Redis error
	}

	count, _ := cmds[1].(*redis.IntCmd).Result()
	return int(count) < rl.burst
}

func (rl *RedisRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !rl.Allow(ip) {
			http.Error(w, "muitas requisições — aguarde", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
