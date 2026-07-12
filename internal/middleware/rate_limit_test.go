package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"pr-guard-agent/internal/ratelimit"
)

func TestRateLimitDisabledPassesThrough(t *testing.T) {
	limiter := ratelimit.NewFixedWindowLimiter(nil, 1, time.Minute, false, false, zap.NewNop())
	response, called := performRateLimitRequest(t, limiter)
	if response.Code != http.StatusNoContent || !called {
		t.Fatalf("status=%d called=%t, want 204/true", response.Code, called)
	}
}

func TestRateLimitRedisErrorFailOpenPassesThrough(t *testing.T) {
	limiter := failingLimiter(true)
	response, called := performRateLimitRequest(t, limiter)
	if response.Code != http.StatusNoContent || !called {
		t.Fatalf("status=%d called=%t, want 204/true", response.Code, called)
	}
}

func TestRateLimitRedisErrorFailClosedReturns503(t *testing.T) {
	limiter := failingLimiter(false)
	response, called := performRateLimitRequest(t, limiter)
	if response.Code != http.StatusServiceUnavailable || called {
		t.Fatalf("status=%d called=%t, want 503/false", response.Code, called)
	}
}

func TestRateLimitRejectsWith429AndHeaders(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	limiter := ratelimit.NewFixedWindowLimiter(client, 1, time.Minute, true, true, zap.NewNop())

	first, called := performRateLimitRequest(t, limiter)
	if first.Code != http.StatusNoContent || !called {
		t.Fatalf("first request status=%d called=%t", first.Code, called)
	}
	second, called := performRateLimitRequest(t, limiter)
	if second.Code != http.StatusTooManyRequests || called {
		t.Fatalf("second request status=%d called=%t, want 429/false", second.Code, called)
	}
	if second.Header().Get("X-RateLimit-Limit") != "1" || second.Header().Get("X-RateLimit-Remaining") != "0" || second.Header().Get("Retry-After") == "" {
		t.Fatalf("unexpected rate limit headers: %#v", second.Header())
	}
	var body map[string]any
	if err := json.Unmarshal(second.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode 429 response: %v", err)
	}
	if body["request_id"] == "" || body["retry_after_seconds"] == nil {
		t.Fatalf("unexpected 429 response: %#v", body)
	}
}

func failingLimiter(failOpen bool) *ratelimit.FixedWindowLimiter {
	client := redis.NewClient(&redis.Options{
		Addr:         "127.0.0.1:1",
		DialTimeout:  10 * time.Millisecond,
		ReadTimeout:  10 * time.Millisecond,
		WriteTimeout: 10 * time.Millisecond,
		MaxRetries:   -1,
	})
	return ratelimit.NewFixedWindowLimiter(client, 1, time.Minute, true, failOpen, zap.NewNop())
}

func performRateLimitRequest(t *testing.T, limiter *ratelimit.FixedWindowLimiter) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	called := false
	r := gin.New()
	r.Use(RequestID())
	r.POST("/projects/:id/diffs/:diff_id/analyze", RateLimit(limiter, zap.NewNop()), func(c *gin.Context) {
		called = true
		c.Status(http.StatusNoContent)
	})
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/projects/1/diffs/2/analyze", nil))
	return response, called
}
