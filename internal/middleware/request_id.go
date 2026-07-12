package middleware

import (
	"github.com/gin-gonic/gin"

	"pr-guard-agent/pkg/requestid"
)

const ginRequestIDKey = "request_id"

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		value := c.GetHeader(requestid.HeaderName)
		if !requestid.IsValid(value) {
			value = requestid.New()
		}

		c.Set(ginRequestIDKey, value)
		c.Request = c.Request.WithContext(requestid.WithContext(c.Request.Context(), value))
		c.Header(requestid.HeaderName, value)
		c.Next()
	}
}

func FromGin(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if value, ok := c.Get(ginRequestIDKey); ok {
		if id, ok := value.(string); ok {
			return id
		}
	}
	return requestid.FromContext(c.Request.Context())
}
