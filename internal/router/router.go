package router

import (
	"github.com/gin-gonic/gin"

	"pr-guard-agent/internal/config"
	"pr-guard-agent/internal/handler"
	"pr-guard-agent/internal/service"
	"pr-guard-agent/pkg/embedding"
)

func SetupRouter(cfg *config.Config) *gin.Engine {
	r := gin.Default()
	embeddingHandler := handler.NewEmbeddingHandler(
		service.NewEmbeddingService(embedding.NewClient(cfg.Embedding)),
	)

	r.GET("/health", handler.Health)
	r.POST("/projects/upload", handler.UploadProject)
	r.POST("/projects/:id/chunks/ast", handler.GenerateASTChunks)
	r.POST("/projects/:id/index", handler.IndexProject)
	r.POST("/projects/:id/diffs", handler.UploadDiff)
	r.POST("/embedding/test", embeddingHandler.Test)

	return r
}
