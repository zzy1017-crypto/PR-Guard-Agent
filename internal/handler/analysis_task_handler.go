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

// AnalysisTaskHandler 处理分析任务相关的HTTP请求。
type AnalysisTaskHandler struct {
	service analysisTaskService
}

// 定义一个接口analysisTaskService，包含Submit和Get方法，用于提交和获取分析任务。
type analysisTaskService interface {
	Submit(ctx context.Context, projectID uint, diffID uint, topK int, submitRequestID string) (*service.SubmitAnalysisTaskResult, error)
	Get(ctx context.Context, taskID uint64) (*model.AnalysisTask, error)
}

// 创建一个新的AnalysisTaskHandler实例，使用提供的analysisTaskService进行初始化。
func NewAnalysisTaskHandler(service analysisTaskService) *AnalysisTaskHandler {
	return &AnalysisTaskHandler{service: service}
}

// Submit解析项目、diff、TopK和RequestID，调用异步提交并返回202、任务ID和轮询URL；提交只入队，不等待分析完成。
func (h *AnalysisTaskHandler) Submit(c *gin.Context) {
	//检查handler和service是否已初始化，如果未初始化则返回500错误。
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "msg": "analysis task service is not initialized"})
		return
	}
	//解析项目ID、diffID和TopK参数，如果解析失败则返回400错误。
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

	//调用service.Submit方法提交分析任务，并处理返回结果或错误。
	result, err := h.service.Submit(
		c.Request.Context(),
		projectID,
		diffID,
		topK,
		middleware.FromGin(c),
	)

	//如果提交分析任务失败，根据错误类型返回相应的HTTP状态码和错误信息。
	if err != nil {
		status, message := analysisTaskErrorResponse(err)
		c.JSON(status, gin.H{"code": 1, "msg": message})
		return
	}

	//如果提交成功，返回202状态码，并包含任务ID、状态、是否重用、是否重试以及轮询URL等信息。
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

// Get查询任务并按pending/running/succeeded/failed返回不同字段；避免对未完成的任务暴露无意义结果，并验证成功结果JSON。
func (h *AnalysisTaskHandler) Get(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "msg": "analysis task service is not initialized"})
		return
	}
	//解析任务ID参数，如果解析失败则返回400错误。
	taskID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || taskID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": "invalid analysis task id"})
		return
	}
	//调用service.Get方法获取分析任务，如果任务不存在则返回404错误，如果查询失败则返回500错误。
	task, err := h.service.Get(c.Request.Context(), taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": 1, "msg": "analysis task not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "msg": "query analysis task failed"})
		return
	}

	//根据任务状态构建响应数据，包含任务ID、状态、尝试次数、最大尝试次数、下一次运行时间等信息。
	data := gin.H{
		"task_id":         task.ID,
		"status":          task.Status,
		"attempt_count":   task.AttemptCount,
		"max_attempts":    task.MaxAttempts,
		"next_run_at":     task.NextRunAt,
		"retry_scheduled": task.Status == model.AnalysisTaskStatusPending && task.AttemptCount > 0 && task.NextRunAt != nil,
	}
	//根据任务状态返回不同的字段，避免对未完成的任务暴露无意义结果，并验证成功结果JSON。
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

// parseAnalysisTaskTopK解析TopK参数，如果为空则返回默认值5，只接受1-20的值；如果无效则返回错误。
func parseAnalysisTaskTopK(raw string) (int, error) {
	if raw == "" {
		return 5, nil
	}
	//将TopK参数解析为整数，并检查其范围是否在1到20之间，如果无效则返回错误。
	topK, err := strconv.Atoi(raw)
	if err != nil || topK < 1 || topK > 20 {
		return 0, service.ErrInvalidAnalysisTopK
	}
	return topK, nil
}

// 解析正整数路径参数并防止uint64->uint溢出；如果无效则返回400错误。
func parsePositiveUintParam(c *gin.Context, name, message string) (uint, bool) {
	value, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || value == 0 || uint64(uint(value)) != value {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": message})
		return 0, false
	}
	return uint(value), true
}

// analysisTaskErrorResponse根据不同的错误类型返回相应的HTTP状态码和错误信息，用于处理分析任务提交失败的情况。
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
