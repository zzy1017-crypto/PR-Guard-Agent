package router

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"pr-guard-agent/internal/config"
	"pr-guard-agent/internal/database"
	"pr-guard-agent/internal/handler"
	"pr-guard-agent/internal/middleware"
	"pr-guard-agent/internal/ratelimit"
	"pr-guard-agent/internal/repository"
	"pr-guard-agent/internal/service"
	"pr-guard-agent/internal/worker"
	reportcache "pr-guard-agent/pkg/cache"
	"pr-guard-agent/pkg/embedding"
	"pr-guard-agent/pkg/llm"
)

func SetupRouter(cfg *config.Config, reportCache *reportcache.ReportCache, loggers ...*zap.Logger) *gin.Engine {
	r, _ := SetupRouterWithWorker(cfg, reportCache, loggers...)
	return r
}

func SetupRouterWithWorker(
	cfg *config.Config,
	reportCache *reportcache.ReportCache,
	loggers ...*zap.Logger,
) (*gin.Engine, *worker.AnalysisWorkerManager) {
	logger := zap.NewNop()
	if len(loggers) > 0 && loggers[0] != nil {
		logger = loggers[0]
	}
	r := gin.New()
	r.Use(
		middleware.RequestID(),
		middleware.AccessLog(logger),
		middleware.Recovery(logger),
	)
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
	reportService := service.NewReportService(
		database.DB,
		service.NewRAGService(database.DB, cfg.Qdrant, embeddingClient),
		llmClient,
		reportCache,
		logger,
	)
	reportHandler := handler.NewReportHandler(reportService)
	analysisTaskService := service.NewAnalysisTaskService(database.DB, cfg.AnalysisWorker.MaxAttempts, logger)
	analysisTaskHandler := handler.NewAnalysisTaskHandler(analysisTaskService)
	workerManager := worker.NewAnalysisWorkerManager(
		repository.NewAnalysisTaskRepository(database.DB),
		reportService,
		cfg.AnalysisWorker,
		logger,
	)
	limiter := ratelimit.NewFixedWindowLimiter(
		database.RDB,
		cfg.RateLimit.Limit,
		time.Duration(cfg.RateLimit.WindowSeconds)*time.Second,
		cfg.RateLimit.Enabled,
		cfg.RateLimit.FailOpen,
		logger,
	)

	r.GET("/health", handler.Health)
	r.POST("/projects/upload", handler.UploadProject)
	r.POST("/projects/:id/chunks/ast", handler.GenerateASTChunks)
	r.POST("/projects/:id/index", indexHandler.IndexProject)
	r.POST("/projects/:id/diffs", handler.UploadDiff)
	r.POST("/projects/:id/diffs/:diff_id/retrieve", ragHandler.RetrieveRelatedChunks)
	r.POST(
		"/projects/:id/diffs/:diff_id/analyze",
		middleware.RateLimit(limiter, logger),
		reportHandler.AnalyzeDiff,
	)
	r.POST(
		"/projects/:id/diffs/:diff_id/analysis-tasks",
		middleware.RateLimit(limiter, logger),
		analysisTaskHandler.Submit,
	)
	r.GET("/analysis-tasks/:id", analysisTaskHandler.Get)
	r.POST("/embedding/test", embeddingHandler.Test)
	r.POST("/vector/collection/init", vectorHandler.InitCollection)
	r.POST("/vectoe/collection/init", vectorHandler.InitCollection)
	r.POST("/vector/test/upsert", vectorHandler.TestUpsert)
	r.POST("/vector/test/search", vectorHandler.TestSearch)
	r.POST("/llm/risk/test", llmHandler.RiskTest)

	return r, workerManager
}
