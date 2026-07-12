package middleware

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"pr-guard-agent/internal/ratelimit"
)

func RateLimit(limiter *ratelimit.FixedWindowLimiter, logger *zap.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = zap.NewNop()
	}
	return func(c *gin.Context) {
		if limiter == nil || !limiter.Enabled() {
			c.Next()
			return
		}

		clientIP := c.ClientIP()
		key := ratelimit.KeyPrefix + "analyze:" + clientIP
		result, err := limiter.Allow(c.Request.Context(), key)
		if err != nil {
			logger.Warn("rate_limit_redis_error",
				zap.String("request_id", FromGin(c)),
				zap.String("client_ip", clientIP),
				zap.String("route", c.FullPath()),
				zap.Bool("fail_open", limiter.FailOpen()),
				zap.Error(err),
			)
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

		c.Header("X-RateLimit-Limit", strconv.FormatInt(result.Limit, 10))
		c.Header("X-RateLimit-Remaining", strconv.FormatInt(result.Remaining, 10))
		if result.Allowed {
			c.Next()
			return
		}

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
