package middleware

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"pr-guard-agent/internal/ratelimit"
)

// 根据客户端IP使用Redis固定窗口限流；设置限流响应头，Redis错误按fail-open/fail-closed决定继续或503，超限返回429并设置Retry-After
func RateLimit(limiter *ratelimit.FixedWindowLimiter, logger *zap.Logger) gin.HandlerFunc {
	// 如果传入的logger为nil，则使用zap.NewNop()创建一个不执行任何操作的Logger。
	if logger == nil {
		logger = zap.NewNop()
	}
	return func(c *gin.Context) {
		// 如果limiter为nil或未启用，则直接继续处理请求。
		if limiter == nil || !limiter.Enabled() {
			c.Next()
			return
		}

		clientIP := c.ClientIP()                           // 获取客户端IP地址
		key := ratelimit.KeyPrefix + "analyze:" + clientIP // 构建限流键，使用固定前缀和客户端IP

		// 调用limiter的Allow方法检查请求是否被允许，如果发生错误则根据fail-open/fail-closed策略处理。
		result, err := limiter.Allow(c.Request.Context(), key)
		if err != nil {
			logger.Warn("rate_limit_redis_error",
				zap.String("request_id", FromGin(c)),
				zap.String("client_ip", clientIP),
				zap.String("route", c.FullPath()),
				zap.Bool("fail_open", limiter.FailOpen()),
				zap.Error(err),
			)
			// 如果limiter配置为fail-open，则继续处理请求，否则返回503服务不可用。
			if limiter.FailOpen() {
				c.Next()
				return
			}
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"code":       http.StatusServiceUnavailable,
				"msg":        "rate limit service unavailable",
				"request_id": FromGin(c),
			})
			return
		}

		// 设置响应头，告知客户端当前的限流状态，包括总限制数和剩余请求数。
		c.Header("X-RateLimit-Limit", strconv.FormatInt(result.Limit, 10))
		c.Header("X-RateLimit-Remaining", strconv.FormatInt(result.Remaining, 10))
		if result.Allowed {
			c.Next()
			return
		}

		// 如果请求被拒绝，则设置Retry-After响应头，告知客户端需要等待的秒数，并记录日志。
		c.Header("Retry-After", strconv.FormatInt(result.RetryAfter, 10))
		logger.Warn("rate_limit_rejected",
			zap.String("request_id", FromGin(c)),
			zap.String("client_ip", clientIP),
			zap.String("route", c.FullPath()),
			zap.Int64("limit", result.Limit),
			zap.Int64("retry_after", result.RetryAfter),
		)
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"code":                http.StatusTooManyRequests,
			"msg":                 "too many analyze requests",
			"request_id":          FromGin(c),
			"retry_after_seconds": result.RetryAfter,
		})
	}
}
