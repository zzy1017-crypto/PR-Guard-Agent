package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// defer/recover捕获Handler panic，服务端记录堆栈，客户端返回500+request_id；用于开发联调，不是业务分析接口。
func Recovery(logger *zap.Logger) gin.HandlerFunc {
	// 如果传入的logger为nil，则使用zap.NewNop()创建一个不执行任何操作的Logger。
	if logger == nil {
		logger = zap.NewNop()
	}
	return func(c *gin.Context) {
		defer func() {
			// 如果发生panic，捕获并记录日志，同时返回500错误给客户端。
			if recovered := recover(); recovered != nil {
				logger.Error("http_panic",
					zap.String("request_id", FromGin(c)),
					zap.String("method", c.Request.Method),
					zap.String("path", c.Request.URL.Path),
					zap.String("panic", fmt.Sprint(recovered)),
					zap.ByteString("stack", debug.Stack()),
				)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"code":       http.StatusInternalServerError,
					"msg":        "internal server error",
					"request_id": FromGin(c),
				})
			}
		}()
		c.Next()
	}
}
