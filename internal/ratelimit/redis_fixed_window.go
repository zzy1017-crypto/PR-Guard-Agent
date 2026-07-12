package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

const KeyPrefix = "prguard:ratelimit:"

var fixedWindowScript = redis.NewScript(`
local current = redis.call("INCR", KEYS[1])
if current == 1 then
  redis.call("EXPIRE", KEYS[1], ARGV[1])
end
local ttl = redis.call("TTL", KEYS[1])
return {current, ttl}
`)

type Result struct {
	Allowed    bool
	Current    int64
	Limit      int64
	Remaining  int64
	RetryAfter int64
}

type FixedWindowLimiter struct {
	redisClient *redis.Client
	limit       int64
	window      time.Duration
	enabled     bool
	failOpen    bool
	logger      *zap.Logger
}

func NewFixedWindowLimiter(redisClient *redis.Client, limit int64, window time.Duration, enabled bool, failOpen bool, logger *zap.Logger) *FixedWindowLimiter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &FixedWindowLimiter{
		redisClient: redisClient,
		limit:       limit,
		window:      window,
		enabled:     enabled,
		failOpen:    failOpen,
		logger:      logger,
	}
}

func (l *FixedWindowLimiter) Enabled() bool  { return l != nil && l.enabled }
func (l *FixedWindowLimiter) FailOpen() bool { return l != nil && l.failOpen }
func (l *FixedWindowLimiter) Limit() int64 {
	if l == nil {
		return 0
	}
	return l.limit
}

func (l *FixedWindowLimiter) Allow(ctx context.Context, key string) (*Result, error) {
	if l == nil {
		return nil, errors.New("rate limiter is nil")
	}
	if !l.enabled {
		return &Result{Allowed: true, Limit: l.limit, Remaining: max64(l.limit, 0)}, nil
	}
	if ctx == nil {
		return nil, errors.New("rate limit context is nil")
	}
	if l.redisClient == nil {
		return nil, errors.New("rate limit redis client is nil")
	}
	if l.limit <= 0 || l.window <= 0 {
		return nil, errors.New("rate limit and window must be positive")
	}

	key = strings.TrimSpace(key)
	if key == "" {
		return nil, errors.New("rate limit key is empty")
	}
	if !strings.HasPrefix(key, KeyPrefix) {
		key = KeyPrefix + key
	}
	windowSeconds := int64((l.window + time.Second - 1) / time.Second)
	values, err := fixedWindowScript.Run(ctx, l.redisClient, []string{key}, windowSeconds).Int64Slice()
	if err != nil {
		return nil, fmt.Errorf("execute fixed window rate limit: %w", err)
	}
	if len(values) != 2 {
		return nil, fmt.Errorf("unexpected fixed window result length: %d", len(values))
	}

	current, ttl := values[0], values[1]
	if ttl < 0 {
		ttl = windowSeconds
	}
	remaining := l.limit - current
	if remaining < 0 {
		remaining = 0
	}
	return &Result{
		Allowed:    current <= l.limit,
		Current:    current,
		Limit:      l.limit,
		Remaining:  remaining,
		RetryAfter: ttl,
	}, nil
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
