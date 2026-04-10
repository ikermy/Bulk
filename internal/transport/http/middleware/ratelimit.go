package middleware

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	apperr "github.com/ikermy/Bulk/internal/transport/http/apperror"
	"github.com/redis/go-redis/v9"
)

// ── In-memory token bucket (fallback при отсутствии Redis) ────────────────────

type limiter struct {
	mu     sync.Mutex
	tokens float64
	last   time.Time
}

var (
	buckets   = map[string]*limiter{}
	bucketsMu sync.Mutex
)

func getLimiter(key string, rps float64) *limiter {
	bucketsMu.Lock()
	defer bucketsMu.Unlock()
	b, ok := buckets[key]
	if !ok {
		b = &limiter{tokens: rps, last: time.Now()}
		buckets[key] = b
	}
	return b
}

// ── Redis client (singleton, инициализируется один раз) ───────────────────────

var (
	globalRedisClient *redis.Client
	redisOnce         sync.Once
)

// getRedisClient возвращает Redis-клиент если REDIS_URL задан и Redis доступен.
// При ошибке подключения возвращает nil — используется in-memory fallback.
func getRedisClient() *redis.Client {
	redisOnce.Do(func() {
		url := os.Getenv("REDIS_URL")
		if url == "" {
			return
		}
		opt, err := redis.ParseURL(url)
		if err != nil {
			return
		}
		c := redis.NewClient(opt)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if c.Ping(ctx).Err() == nil {
			globalRedisClient = c
		} else {
			_ = c.Close()
		}
	})
	return globalRedisClient
}

// ── Middleware ─────────────────────────────────────────────────────────────────

// RateLimitMiddleware ограничивает запросы по IP.
// При наличии Redis (REDIS_URL) — distributed fixed-window rate limiting:
//
//	каждый под инкрементирует общий счётчик, ключ живёт 1 секунду.
//
// При отсутствии Redis — in-memory token bucket (только для dev/single-instance).
// Лимит задаётся через RATE_LIMIT_RPS (запросов/сек, default 10).
func RateLimitMiddleware() gin.HandlerFunc {
	rps := 10.0
	if v := os.Getenv("RATE_LIMIT_RPS"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			rps = f
		}
	}
	redisLimit := int64(rps)
	window := time.Second

	return func(c *gin.Context) {
		ip := c.ClientIP()

		if rdb := getRedisClient(); rdb != nil {
			// Distributed: Redis fixed-window (INCR + EXPIRE)
			ctx := context.Background()
			key := "ratelimit:" + ip
			count, err := rdb.Incr(ctx, key).Result()
			if err == nil {
				// Установить TTL при первом запросе в окне
				if count == 1 {
					rdb.Expire(ctx, key, window)
				}
				if count > redisLimit {
					apperr.WriteError(c, http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED", "rate limit exceeded", nil)
					c.Abort()
					return
				}
				c.Next()
				return
			}
			// При ошибке Redis — деградируем до in-memory (не блокируем запрос)
		}

		// In-memory token bucket (fallback)
		b := getLimiter(ip, rps)
		b.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(b.last).Seconds()
		b.tokens += elapsed * rps
		if b.tokens > rps {
			b.tokens = rps
		}
		b.last = now
		if b.tokens < 1.0 {
			b.mu.Unlock()
			apperr.WriteError(c, http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED", "rate limit exceeded", nil)
			c.Abort()
			return
		}
		b.tokens -= 1.0
		b.mu.Unlock()
		c.Next()
	}
}
