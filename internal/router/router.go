package router

import (
	"github.com/gin-gonic/gin"

	"pr-guard-agent/internal/config"
	"pr-guard-agent/internal/database"
	"pr-guard-agent/internal/handler"
	"pr-guard-agent/internal/service"
	reportcache "pr-guard-agent/pkg/cache"
	"pr-guard-agent/pkg/embedding"
	"pr-guard-agent/pkg/llm"
)

func SetupRouter(cfg *config.Config, reportCache *reportcache.ReportCache) *gin.Engine {
	r := gin.Default()
	embeddingClient := embedding.NewClient(cfg.Embedding)
	llmClient := llm.NewClient(cfg.LLM)
	embeddingHandler := handler.NewEmbeddingHandler(
		service.NewEmbeddingService(embeddingClient),
	)
	vectorHandler := handler.NewVectorHandler(
		service.NewVectorService(cfg.Qdrant, embeddingClient),
	)
	indexHandler := handler.NewIndexHandler(
		service.NewIndexService(database.DB, cfg.Qdrant, embeddingClient),
	)
	ragHandler := handler.NewRAGHandler(
		service.NewRAGService(database.DB, cfg.Qdrant, embeddingClient),
	)
	llmHandler := handler.NewLLMHandler(
		service.NewLLMService(llmClient),
	)
	reportHandler := handler.NewReportHandler(
		service.NewReportService(
			database.DB,
			service.NewRAGService(database.DB, cfg.Qdrant, embeddingClient),
			llmClient,
			reportCache,
		),
	)

	r.GET("/health", handler.Health)
	r.POST("/projects/upload", handler.UploadProject)
	r.POST("/projects/:id/chunks/ast", handler.GenerateASTChunks)
	r.POST("/projects/:id/index", indexHandler.IndexProject)
	r.POST("/projects/:id/diffs", handler.UploadDiff)
	r.POST("/projects/:id/diffs/:diff_id/retrieve", ragHandler.RetrieveRelatedChunks)
	r.POST("/projects/:id/diffs/:diff_id/analyze", reportHandler.AnalyzeDiff)
	r.POST("/embedding/test", embeddingHandler.Test)
	r.POST("/vector/collection/init", vectorHandler.InitCollection)
	r.POST("/vectoe/collection/init", vectorHandler.InitCollection)
	r.POST("/vector/test/upsert", vectorHandler.TestUpsert)
	r.POST("/vector/test/search", vectorHandler.TestSearch)
	r.POST("/llm/risk/test", llmHandler.RiskTest)

	return r
}
