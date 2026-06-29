package router

import (
	"github.com/gin-gonic/gin"

	"pr-guard-agent/internal/handler"
)

// SetupRouter 创建一个新的Gin引擎实例，并定义路由规则，返回这个引擎实例供main函数使用
func SetupRouter() *gin.Engine {
	r := gin.Default()

	r.GET("/health", handler.Health) //用户访问/health路径时，调用handler包中的Health函数来处理请求，返回一个JSON格式的响应，表示服务的健康状态

	r.POST("/projects/upload", handler.UploadProject) //用户访问/projects/upload路径时，调用handler包中的UploadProject函数来处理上传项目的请求，接收一个zip文件和项目名称，进行验证和处理，并返回相应的JSON响应

	return r
}
