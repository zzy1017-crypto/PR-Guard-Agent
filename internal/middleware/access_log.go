package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// 记录请求ID、方法、路径、路由、状态码、延迟时间、客户端IP、用户代理和错误计数的中间件。根据状态码选择Info、Warn或Error。
func AccessLog(logger *zap.Logger) gin.HandlerFunc {

	// 如果传入的logger为nil，则使用zap.NewNop()创建一个不执行任何操作的Logger。
	if logger == nil {
		logger = zap.NewNop()
	}
	return func(c *gin.Context) {
		// 记录请求开始时间
		startedAt := time.Now()
		c.Next()

		// 记录请求结束时间，并计算延迟时间
		route := c.FullPath()
		// 记录请求的相关信息，包括请求ID、方法、路径、路由、状态码、延迟时间、客户端IP、用户代理和错误计数。
		fields := []zap.Field{
			zap.String("request_id", FromGin(c)),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("route", route),
			zap.Int("status", c.Writer.Status()),
			zap.Int64("latency_ms", time.Since(startedAt).Milliseconds()),
			zap.String("client_ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
			zap.Int("error_count", len(c.Errors)),
		}
		// 根据HTTP状态码选择日志级别：500及以上为Error，400及以上为Warn，其余为Info。
		status := c.Writer.Status()
		switch {
		case status >= 500:
			logger.Error("http_request", fields...)
		case status >= 400:
			logger.Warn("http_request", fields...)
		default:
			logger.Info("http_request", fields...)
		}
	}
}
