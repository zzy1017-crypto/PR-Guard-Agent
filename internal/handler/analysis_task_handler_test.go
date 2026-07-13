package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"pr-guard-agent/internal/model"
	"pr-guard-agent/internal/service"
)

type fakeAnalysisTaskService struct{}

func (fakeAnalysisTaskService) Submit(context.Context, uint, uint, int, string) (*service.SubmitAnalysisTaskResult, error) {
	return &service.SubmitAnalysisTaskResult{Task: &model.AnalysisTask{ID: 1, Status: model.AnalysisTaskStatusPending}}, nil
}

func (fakeAnalysisTaskService) Get(context.Context, uint64) (*model.AnalysisTask, error) {
	return nil, gorm.ErrRecordNotFound
}

func TestGetAnalysisTaskNotFoundReturns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewAnalysisTaskHandler(fakeAnalysisTaskService{})
	router := gin.New()
	router.GET("/analysis-tasks/:id", handler.Get)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/analysis-tasks/999", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", response.Code, response.Body.String())
	}
}

func TestSubmitAnalysisTaskRejectsOutOfRangeTopK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewAnalysisTaskHandler(fakeAnalysisTaskService{})
	router := gin.New()
	router.POST("/projects/:id/diffs/:diff_id/analysis-tasks", handler.Submit)

	for _, topK := range []string{"0", "21", "-1", "bad"} {
		t.Run(topK, func(t *testing.T) {
			response := httptest.NewRecorder()
			path := "/projects/1/diffs/1/analysis-tasks?top_k=" + topK
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", response.Code, response.Body.String())
			}
		})
	}
}
