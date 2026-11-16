package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"pemira-backend/pkg/redis"

	"github.com/gin-gonic/gin"
)

func TestRateLimiterLocalFallbackRejectsBurst(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rl := NewRateLimiter(&redis.RedisService{Client: nil})

	r := gin.New()
	r.GET("/limited", rl.Limit("test", 2, time.Minute), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	for i := 1; i <= 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/limited", nil)
		req.RemoteAddr = "203.0.113.10:12345"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d should be allowed, got %d", i, w.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/limited", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 on third request, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header on rate limit response")
	}
}

func TestRateLimiterSeparatesClients(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rl := NewRateLimiter(&redis.RedisService{Client: nil})

	r := gin.New()
	r.GET("/limited", rl.Limit("test", 1, time.Minute), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req1 := httptest.NewRequest(http.MethodGet, "/limited", nil)
	req1.RemoteAddr = "203.0.113.10:1111"
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)

	req2 := httptest.NewRequest(http.MethodGet, "/limited", nil)
	req2.RemoteAddr = "203.0.113.11:2222"
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w1.Code != http.StatusOK || w2.Code != http.StatusOK {
		t.Fatalf("expected independent clients to be allowed, got %d and %d", w1.Code, w2.Code)
	}
}
