package middleware

import (
	"github.com/gin-gonic/gin"

	"pr-guard-agent/pkg/requestid"
)

const ginRequestIDKey = "request_id" // Gin上下文中存储请求ID的键

// 复用合法X-Request-ID或生成新的请求ID，存储在Gin上下文和HTTP请求上下文中，并设置响应头.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取请求头中的X-Request-ID，如果不存在或不合法，则生成一个新的请求ID。
		value := c.GetHeader(requestid.HeaderName)
		if !requestid.IsValid(value) {
			value = requestid.New()
		}

		// 将请求ID存储在Gin上下文中，以便在后续处理中可以方便地访问。
		c.Set(ginRequestIDKey, value)
		// 将请求ID存储在HTTP请求的上下文中，以便在其他中间件或处理函数中可以访问。
		c.Request = c.Request.WithContext(requestid.WithContext(c.Request.Context(), value))
		// 设置响应头中的X-Request-ID，以便客户端可以获取到请求ID。
		c.Header(requestid.HeaderName, value)
		c.Next()
	}
}

// 优先从Gin上下文中获取请求ID，如果不存在则从HTTP请求上下文中获取；如果都不存在则返回空字符串。
func FromGin(c *gin.Context) string {
	if c == nil {
		return ""
	}
	// 尝试从Gin上下文中获取请求ID，如果存在则返回。
	if value, ok := c.Get(ginRequestIDKey); ok {
		if id, ok := value.(string); ok {
			return id
		}
	}
	// 如果Gin上下文中不存在请求ID，则尝试从HTTP请求的上下文中获取。
	return requestid.FromContext(c.Request.Context())
}
