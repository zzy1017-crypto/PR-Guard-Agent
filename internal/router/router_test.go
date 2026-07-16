package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"pr-guard-agent/internal/config"
)

func TestOpsRoutesAreNotRegisteredWhenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{
		Server:    config.ServerConfig{Mode: gin.TestMode},
		RateLimit: config.RateLimitConfig{Enabled: false},
		Qdrant: config.QdrantConfig{
			Host: "localhost", Port: 6334, CollectionName: "test",
			VectorSize: 3, Distance: "Cosine", TimeoutSeconds: 1,
		},
		Embedding: config.EmbeddingConfig{Provider: "mock", Dimension: 3, BatchSize: 1, TimeoutSeconds: 1},
		LLM:       config.LLMConfig{Provider: "mock", MockMode: "normal", TimeoutSeconds: 1},
		AnalysisWorker: config.AnalysisWorkerConfig{
			Enabled: false, WorkerCount: 1, PollIntervalMS: 10,
			TaskTimeoutSeconds: 1, StaleAfterSeconds: 60, MaxAttempts: 3,
			RetryBaseSeconds: 1, RetryMaxSeconds: 30,
		},
		Ops: config.OpsConfig{
			Enabled: false, DefaultPageSize: 20, MaxPageSize: 100,
			DefaultMetricsWindowHours: 24, MaxMetricsWindowHours: 168,
			QueryTimeoutSeconds: 3,
		},
	}
	router := SetupRouter(cfg, nil)
	for _, path := range []string{
		"/ops/analysis-tasks",
		"/ops/analysis-tasks/metrics",
		"/ops/workers",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404; body=%s", path, response.Code, response.Body.String())
		}
	}
}
