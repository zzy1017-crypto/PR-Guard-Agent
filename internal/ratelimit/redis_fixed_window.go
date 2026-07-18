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

const KeyPrefix = "prguard:ratelimit:" // Redis键前缀，用于区分不同的限流器和用途

// Redis固定窗口限流器，使用Redis的INCR和EXPIRE命令实现固定时间窗口的请求计数。
var fixedWindowScript = redis.NewScript(`
local current = redis.call("INCR", KEYS[1])
if current == 1 then
  redis.call("EXPIRE", KEYS[1], ARGV[1])
end
local ttl = redis.call("TTL", KEYS[1])
return {current, ttl}
`)

// Result 表示限流结果，包括是否允许请求、当前计数、限制数、剩余请求数和重试等待时间。
type Result struct {
	Allowed    bool
	Current    int64
	Limit      int64
	Remaining  int64
	RetryAfter int64
}

// FixedWindowLimiter 使用Redis实现的固定窗口限流器，支持启用/禁用、失败策略和日志记录。
type FixedWindowLimiter struct {
	redisClient *redis.Client
	limit       int64
	window      time.Duration
	enabled     bool
	failOpen    bool
	logger      *zap.Logger
}

// 保存Redis、额度、窗口、启用及fail-open参数。
func NewFixedWindowLimiter(redisClient *redis.Client, limit int64, window time.Duration, enabled bool, failOpen bool, logger *zap.Logger) *FixedWindowLimiter {
	if logger == nil {
		logger = zap.NewNop()
	}
	//返回一个新的FixedWindowLimiter实例，初始化时保存Redis客户端、请求限制、时间窗口、启用状态、失败策略和日志记录器。
	return &FixedWindowLimiter{
		redisClient: redisClient,
		limit:       limit,
		window:      window,
		enabled:     enabled,
		failOpen:    failOpen,
		logger:      logger,
	}
}

// Enabled 返回限流器是否启用。
func (l *FixedWindowLimiter) Enabled() bool { return l != nil && l.enabled }

// FailOpen 返回限流器是否配置为失败时允许请求继续。
func (l *FixedWindowLimiter) FailOpen() bool { return l != nil && l.failOpen }

// Limit 返回限流器的请求限制数。
func (l *FixedWindowLimiter) Limit() int64 {
	if l == nil {
		return 0
	}
	return l.limit
}

// 校验依赖和key通过Lua脚本原子执行INCR+首次EXPIRE+TTL，计算Allowed/Remaining/RetryAfter，返回Result或错误；Redis错误按fail-open/fail-closed决定继续或503。
func (l *FixedWindowLimiter) Allow(ctx context.Context, key string) (*Result, error) {
	// 如果限流器为nil，返回错误。
	if l == nil {
		return nil, errors.New("rate limiter is nil")
	}
	// 如果限流器未启用，则直接返回允许请求的结果，Remaining为limit。
	if !l.enabled {
		return &Result{Allowed: true, Limit: l.limit, Remaining: max64(l.limit, 0)}, nil
	}
	//如果ctx为nil，返回错误。
	if ctx == nil {
		return nil, errors.New("rate limit context is nil")
	}
	// 如果Redis客户端为nil，返回错误。
	if l.redisClient == nil {
		return nil, errors.New("rate limit redis client is nil")
	}
	// 如果limit或window为非正数，返回错误。
	if l.limit <= 0 || l.window <= 0 {
		return nil, errors.New("rate limit and window must be positive")
	}

	key = strings.TrimSpace(key) //key去除前后空格
	// 如果key为空，返回错误。
	if key == "" {
		return nil, errors.New("rate limit key is empty")
	}
	// 如果key不以KeyPrefix开头，则添加前缀，以确保Redis键的唯一性和一致性。
	if !strings.HasPrefix(key, KeyPrefix) {
		key = KeyPrefix + key
	}
	// 计算窗口秒数，向上取整为整数秒，以便在Lua脚本中使用。
	windowSeconds := int64((l.window + time.Second - 1) / time.Second)
	// 调用Redis Lua脚本执行固定窗口限流逻辑，传入key和窗口秒数，返回当前计数和TTL。
	values, err := fixedWindowScript.Run(ctx, l.redisClient, []string{key}, windowSeconds).Int64Slice()
	// 如果执行Lua脚本出错，返回错误。
	if err != nil {
		return nil, fmt.Errorf("execute fixed window rate limit: %w", err)
	}
	// 如果返回的值长度不为2，说明Lua脚本返回结果异常，返回错误。
	if len(values) != 2 {
		return nil, fmt.Errorf("unexpected fixed window result length: %d", len(values))
	}

	// 解析Lua脚本返回的当前计数和TTL，计算剩余请求数和重试等待时间，并返回限流结果。
	current, ttl := values[0], values[1]
	// 如果TTL小于0，说明Redis键不存在或已过期，将TTL设置为窗口秒数，以便客户端知道需要等待的时间。
	if ttl < 0 {
		ttl = windowSeconds
	}
	// 计算剩余请求数，如果当前计数超过限制，则剩余为0，否则为limit-current。
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

// max64 返回两个int64中的较大值，用于计算剩余请求数时确保不为负数。
func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
