package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"pr-guard-agent/internal/middleware"
	"pr-guard-agent/internal/model"
	"pr-guard-agent/internal/service"
)

type AnalysisTaskHandler struct {
	service analysisTaskService
}

type analysisTaskService interface {
	Submit(ctx context.Context, projectID uint, diffID uint, topK int, submitRequestID string) (*service.SubmitAnalysisTaskResult, error)
	Get(ctx context.Context, taskID uint64) (*model.AnalysisTask, error)
}

func NewAnalysisTaskHandler(service analysisTaskService) *AnalysisTaskHandler {
	return &AnalysisTaskHandler{service: service}
}

func (h *AnalysisTaskHandler) Submit(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "msg": "analysis task service is not initialized"})
		return
	}
	projectID, ok := parsePositiveUintParam(c, "id", "invalid project id")
	if !ok {
		return
	}
	diffID, ok := parsePositiveUintParam(c, "diff_id", "invalid diff id")
	if !ok {
		return
	}
	topK, err := parseAnalysisTaskTopK(c.Query("top_k"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": err.Error()})
		return
	}

	result, err := h.service.Submit(
		c.Request.Context(),
		projectID,
		diffID,
		topK,
		middleware.FromGin(c),
	)
	if err != nil {
		status, message := analysisTaskErrorResponse(err)
		c.JSON(status, gin.H{"code": 1, "msg": message})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"code": 0,
		"msg":  "analysis task accepted",
		"data": gin.H{
			"task_id":  result.Task.ID,
			"status":   result.Task.Status,
			"reused":   result.Reused,
			"retried":  result.Retried,
			"poll_url": "/analysis-tasks/" + strconv.FormatUint(result.Task.ID, 10),
		},
	})
}

func (h *AnalysisTaskHandler) Get(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "msg": "analysis task service is not initialized"})
		return
	}
	taskID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || taskID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": "invalid analysis task id"})
		return
	}
	task, err := h.service.Get(c.Request.Context(), taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": 1, "msg": "analysis task not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "msg": "query analysis task failed"})
		return
	}

	data := gin.H{
		"task_id":         task.ID,
		"status":          task.Status,
		"attempt_count":   task.AttemptCount,
		"max_attempts":    task.MaxAttempts,
		"next_run_at":     task.NextRunAt,
		"retry_scheduled": task.Status == model.AnalysisTaskStatusPending && task.AttemptCount > 0 && task.NextRunAt != nil,
	}
	switch task.Status {
	case model.AnalysisTaskStatusPending:
		data["created_at"] = task.CreatedAt
		if task.AttemptCount > 0 && task.NextRunAt != nil {
			data["last_error_code"] = task.ErrorCode
			data["last_error_message"] = task.ErrorMessage
		}
	case model.AnalysisTaskStatusRunning:
		data["started_at"] = task.StartedAt
	case model.AnalysisTaskStatusSucceeded:
		data["report_id"] = task.ReportID
		data["degraded"] = task.Degraded
		data["finished_at"] = task.FinishedAt
		var result json.RawMessage
		if task.ResultJSON == "" || json.Unmarshal([]byte(task.ResultJSON), &result) != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "msg": "analysis task result is unavailable"})
			return
		}
		data["result"] = result
	case model.AnalysisTaskStatusFailed:
		data["last_error_code"] = task.ErrorCode
		data["last_error_message"] = task.ErrorMessage
		data["finished_at"] = task.FinishedAt
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "msg": "analysis task status is invalid"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": data})
}

func parseAnalysisTaskTopK(raw string) (int, error) {
	if raw == "" {
		return 5, nil
	}
	topK, err := strconv.Atoi(raw)
	if err != nil || topK < 1 || topK > 20 {
		return 0, service.ErrInvalidAnalysisTopK
	}
	return topK, nil
}

func parsePositiveUintParam(c *gin.Context, name, message string) (uint, bool) {
	value, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || value == 0 || uint64(uint(value)) != value {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": message})
		return 0, false
	}
	return uint(value), true
}

func analysisTaskErrorResponse(err error) (int, string) {
	switch {
	case errors.Is(err, service.ErrInvalidAnalysisTopK),
		errors.Is(err, service.ErrDiffProjectMismatch):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, service.ErrProjectNotFound), errors.Is(err, service.ErrDiffNotFound):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, service.ErrAnalysisTaskRetryExhausted):
		return http.StatusConflict, err.Error()
	default:
		return http.StatusInternalServerError, "submit analysis task failed"
	}
}
