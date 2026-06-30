package router

import (
	"github.com/gin-gonic/gin"

	"pr-guard-agent/internal/handler"
)

func SetupRouter() *gin.Engine {
	r := gin.Default()

	r.GET("/health", handler.Health)
	r.POST("/projects/upload", handler.UploadProject)
	r.POST("/projects/:id/chunks/ast", handler.GenerateASTChunks)

	return r
}
