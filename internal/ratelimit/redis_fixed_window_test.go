package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

func TestFixedWindowAllowsThenRejectsAtLimit(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	limiter := NewFixedWindowLimiter(client, 2, time.Minute, true, true, zap.NewNop())

	first, err := limiter.Allow(context.Background(), "analyze:127.0.0.1")
	if err != nil {
		t.Fatalf("first Allow() error = %v", err)
	}
	if !first.Allowed || first.Current != 1 || first.Remaining != 1 || first.RetryAfter <= 0 {
		t.Fatalf("unexpected first result: %#v", first)
	}

	second, err := limiter.Allow(context.Background(), "analyze:127.0.0.1")
	if err != nil || !second.Allowed || second.Remaining != 0 {
		t.Fatalf("unexpected second result: result=%#v err=%v", second, err)
	}
	third, err := limiter.Allow(context.Background(), "analyze:127.0.0.1")
	if err != nil {
		t.Fatalf("third Allow() error = %v", err)
	}
	if third.Allowed || third.Current != 3 || third.Remaining != 0 || third.RetryAfter <= 0 {
		t.Fatalf("unexpected rejected result: %#v", third)
	}
	if !server.Exists(KeyPrefix + "analyze:127.0.0.1") {
		t.Fatal("rate limit key does not use required prefix")
	}
}
