package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"pr-guard-agent/internal/config"
	"pr-guard-agent/internal/repository"
	"pr-guard-agent/internal/service"
)

type fakeOpsHandlerService struct {
	filter      repository.TaskListFilter
	windowHours int
	listResult  *service.TaskListResult
	listErr     error
}

func (s *fakeOpsHandlerService) ListTasks(_ context.Context, filter repository.TaskListFilter) (*service.TaskListResult, error) {
	s.filter = filter
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.listResult != nil {
		return s.listResult, nil
	}
	return &service.TaskListResult{Page: filter.Page, PageSize: filter.PageSize, Items: []service.TaskListItem{}}, nil
}

func (s *fakeOpsHandlerService) GetMetrics(_ context.Context, windowHours int) (*service.TaskMetricsResult, error) {
	s.windowHours = windowHours
	return &service.TaskMetricsResult{
		WindowHours:  windowHours,
		Current:      service.CurrentTaskMetrics{},
		Window:       service.WindowTaskMetrics{},
		ErrorsByCode: []repository.ErrorCodeMetric{},
	}, nil
}

func (s *fakeOpsHandlerService) GetWorkers(context.Context) (*service.WorkerStatusResult, error) {
	return &service.WorkerStatusResult{Workers: []service.WorkerStatusItem{}}, nil
}

func TestOpsTaskListValidation(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"page_below_one", "?page=0"},
		{"page_size_above_max", "?page_size=101"},
		{"invalid_status", "?status=queued"},
		{"invalid_degraded", "?degraded=1"},
		{"created_from_after_created_to", "?created_from=2026-07-17T00:00:00Z&created_to=2026-07-16T00:00:00Z"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router, _ := newOpsHandlerRouter()
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ops/analysis-tasks"+test.query, nil))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestOpsTaskListParsesFiltersAndDefaults(t *testing.T) {
	router, fake := newOpsHandlerRouter()
	path := "/ops/analysis-tasks?status=failed&project_id=2&diff_id=3&error_code=qdrant_unavailable&degraded=false" +
		"&created_from=2026-07-15T00:00:00Z&created_to=2026-07-16T00:00:00Z&page=2"
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", response.Code, response.Body.String())
	}
	filter := fake.filter
	if filter.Page != 2 || filter.PageSize != 20 || filter.Status == nil || *filter.Status != "failed" ||
		filter.ProjectID == nil || *filter.ProjectID != 2 || filter.DiffID == nil || *filter.DiffID != 3 ||
		filter.ErrorCode == nil || *filter.ErrorCode != "qdrant_unavailable" ||
		filter.Degraded == nil || *filter.Degraded ||
		filter.CreatedFrom == nil || filter.CreatedTo == nil {
		t.Fatalf("unexpected filter: %#v", filter)
	}
}

func TestOpsMetricsUsesDefaultWindowAndRejectsExcess(t *testing.T) {
	router, fake := newOpsHandlerRouter()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ops/analysis-tasks/metrics", nil))
	if response.Code != http.StatusOK || fake.windowHours != 24 {
		t.Fatalf("default window status=%d window=%d body=%s", response.Code, fake.windowHours, response.Body.String())
	}

	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ops/analysis-tasks/metrics?window_hours=169", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", response.Code, response.Body.String())
	}
}

func TestOpsTaskListResponseDoesNotContainResultJSON(t *testing.T) {
	router, fake := newOpsHandlerRouter()
	now := time.Now()
	fake.listResult = &service.TaskListResult{
		Total: 1, Page: 1, PageSize: 20,
		Items: []service.TaskListItem{{
			TaskID: 1, ProjectID: 2, DiffID: 3, Status: "succeeded",
			CreatedAt: now, UpdateAt: now,
		}},
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ops/analysis-tasks", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if containsJSONKey(body, "result_json") || containsJSONKey(body, "task_key") || containsJSONKey(body, "result") {
		t.Fatalf("sensitive key leaked: %s", response.Body.String())
	}
}

func TestOpsQueryTimeoutReturnsSafeGatewayTimeout(t *testing.T) {
	router, fake := newOpsHandlerRouter()
	fake.listErr = context.DeadlineExceeded
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/ops/analysis-tasks", nil))
	if response.Code != http.StatusGatewayTimeout ||
		response.Body.String() != `{"code":1,"msg":"operations query timed out"}` {
		t.Fatalf("unexpected timeout response: status=%d body=%s", response.Code, response.Body.String())
	}
}

func newOpsHandlerRouter() (*gin.Engine, *fakeOpsHandlerService) {
	gin.SetMode(gin.TestMode)
	fake := &fakeOpsHandlerService{}
	handler := NewAnalysisTaskOpsHandler(fake, config.OpsConfig{
		Enabled: true, DefaultPageSize: 20, MaxPageSize: 100,
		DefaultMetricsWindowHours: 24, MaxMetricsWindowHours: 168,
		QueryTimeoutSeconds: 3,
	}, nil)
	router := gin.New()
	router.GET("/ops/analysis-tasks", handler.ListTasks)
	router.GET("/ops/analysis-tasks/metrics", handler.GetMetrics)
	router.GET("/ops/workers", handler.GetWorkers)
	return router, fake
}

func containsJSONKey(value any, key string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for currentKey, item := range typed {
			if currentKey == key || containsJSONKey(item, key) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if containsJSONKey(item, key) {
				return true
			}
		}
	}
	return false
}
