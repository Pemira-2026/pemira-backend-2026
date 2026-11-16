package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"pemira-backend/pkg/redis"

	"github.com/gin-gonic/gin"
)

const (
	rateLimitKeyPrefix = "ratelimit"
)

type localBucket struct {
	count   int
	resetAt time.Time
}

// RateLimiter implements a distributed fixed-window rate limiter backed by
// Redis. If Redis is unavailable (development / degraded mode) it falls back to
// an in-process limiter so the API still rejects obvious spam.
type RateLimiter struct {
	redisService *redis.RedisService

	mu     sync.Mutex
	bucket map[string]*localBucket
}

func NewRateLimiter(redisService *redis.RedisService) *RateLimiter {
	return &RateLimiter{
		redisService: redisService,
		bucket:       make(map[string]*localBucket),
	}
}

// Limit applies a per-client-IP rate limit for a named route.
func (rl *RateLimiter) Limit(name string, limit int, window time.Duration) gin.HandlerFunc {
	return rl.LimitBy(name, limit, window, func(c *gin.Context) string {
		return c.ClientIP()
	})
}

// LimitBy applies a rate limit using a custom identity function (for example,
// authenticated voter ID). This makes vote submission limiting independent of
// shared campus NAT/proxy IPs.
func (rl *RateLimiter) LimitBy(name string, limit int, window time.Duration, keyFn func(*gin.Context) string) gin.HandlerFunc {
	if limit <= 0 || window <= 0 {
		return func(c *gin.Context) { c.Next() }
	}

	return func(c *gin.Context) {
		identity := keyFn(c)
		if identity == "" {
			identity = c.ClientIP()
		}

		allowed, remaining, retryAfter := rl.allow(c, name, identity, limit, window)
		c.Header("X-RateLimit-Limit", strconv.Itoa(limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		if !allowed {
			if retryAfter > 0 {
				c.Header("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
			}
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "Terlalu banyak permintaan. Silakan coba lagi beberapa saat lagi.",
				"code":  "RATE_LIMITED",
			})
			return
		}

		c.Next()
	}
}

func (rl *RateLimiter) allow(c *gin.Context, name, identity string, limit int, window time.Duration) (bool, int, time.Duration) {
	key := fmt.Sprintf("%s:%s:%s", rateLimitKeyPrefix, name, identity)
	ctx := c.Request.Context()

	if rl.redisService != nil && rl.redisService.IsAvailable() {
		count, err := rl.redisService.IncrWithExpire(ctx, key, window)
		if err == nil {
			remaining := limit - int(count)
			if remaining < 0 {
				remaining = 0
			}
			return count <= int64(limit), remaining, window
		}
		// Redis error -> fail open to local fallback rather than blocking all
		// traffic. Vote submission itself still fails closed when Redis is
		// required by the service layer.
	}

	return rl.allowLocal(key, limit, window)
}

func (rl *RateLimiter) allowLocal(key string, limit int, window time.Duration) (bool, int, time.Duration) {
	now := time.Now()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Best-effort cleanup to prevent unbounded local map growth under attack.
	if len(rl.bucket) > 10000 {
		for k, entry := range rl.bucket {
			if now.After(entry.resetAt) {
				delete(rl.bucket, k)
			}
		}
	}

	b, ok := rl.bucket[key]
	if !ok || now.After(b.resetAt) {
		b = &localBucket{count: 0, resetAt: now.Add(window)}
		rl.bucket[key] = b
	}

	b.count++
	remaining := limit - b.count
	if remaining < 0 {
		remaining = 0
	}
	retryAfter := time.Until(b.resetAt)
	if retryAfter < 0 {
		retryAfter = 0
	}
	return b.count <= limit, remaining, retryAfter
}

// VoterKey is a helper identity function for authenticated vote endpoints.
func VoterKey(c *gin.Context) string {
	if v, exists := c.Get("voterId"); exists {
		if s, ok := v.(string); ok && s != "" {
			return "voter:" + s
		}
	}
	return c.ClientIP()
}
