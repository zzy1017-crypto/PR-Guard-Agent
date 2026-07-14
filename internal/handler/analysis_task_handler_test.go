package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"pr-guard-agent/internal/model"
	"pr-guard-agent/internal/service"
)

type fakeAnalysisTaskService struct{}

func (fakeAnalysisTaskService) Submit(context.Context, uint, uint, int, string) (*service.SubmitAnalysisTaskResult, error) {
	return &service.SubmitAnalysisTaskResult{Task: &model.AnalysisTask{ID: 1, Status: model.AnalysisTaskStatusPending}}, nil
}

type getAnalysisTaskService struct {
	task *model.AnalysisTask
}

func (s getAnalysisTaskService) Submit(context.Context, uint, uint, int, string) (*service.SubmitAnalysisTaskResult, error) {
	return nil, errors.New("not implemented")
}

func (s getAnalysisTaskService) Get(context.Context, uint64) (*model.AnalysisTask, error) {
	return s.task, nil
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

func TestGetAnalysisTaskReturnsRetryScheduleAndSafeLastError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	nextRunAt := time.Now().Add(time.Minute).UTC().Truncate(time.Millisecond)
	handler := NewAnalysisTaskHandler(getAnalysisTaskService{task: &model.AnalysisTask{
		ID: 25, Status: model.AnalysisTaskStatusPending, AttemptCount: 1, MaxAttempts: 3,
		NextRunAt: &nextRunAt, ErrorCode: "qdrant_unavailable",
		ErrorMessage: "code retrieval service is temporarily unavailable",
	}})
	router := gin.New()
	router.GET("/analysis-tasks/:id", handler.Get)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/analysis-tasks/25", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data["retry_scheduled"] != true || body.Data["last_error_code"] != "qdrant_unavailable" || body.Data["max_attempts"] != float64(3) {
		t.Fatalf("unexpected response: %s", response.Body.String())
	}
}
