package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Health 是一个处理健康检查请求的函数，当用户访问/health路径时，返回一个JSON格式的响应，表示服务的健康状态
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "pr-guard-agent is running",
	})
}
