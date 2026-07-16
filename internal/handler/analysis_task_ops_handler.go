package handler

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"pr-guard-agent/internal/config"
	"pr-guard-agent/internal/middleware"
	"pr-guard-agent/internal/model"
	"pr-guard-agent/internal/repository"
	"pr-guard-agent/internal/service"
)

type analysisTaskOpsService interface {
	ListTasks(ctx context.Context, filter repository.TaskListFilter) (*service.TaskListResult, error)
	GetMetrics(ctx context.Context, windowHours int) (*service.TaskMetricsResult, error)
	GetWorkers(ctx context.Context) (*service.WorkerStatusResult, error)
}

type AnalysisTaskOpsHandler struct {
	service analysisTaskOpsService
	config  config.OpsConfig
	logger  *zap.Logger
}

func NewAnalysisTaskOpsHandler(
	service analysisTaskOpsService,
	cfg config.OpsConfig,
	logger *zap.Logger,
) *AnalysisTaskOpsHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AnalysisTaskOpsHandler{service: service, config: cfg, logger: logger}
}

func (h *AnalysisTaskOpsHandler) ListTasks(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "msg": "operations service is not initialized"})
		return
	}
	filter, err := h.parseTaskListFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(h.config.QueryTimeoutSeconds)*time.Second)
	defer cancel()
	result, err := h.service.ListTasks(ctx, filter)
	if err != nil {
		h.logger.Error("ops_task_list_query_failed",
			zap.String("request_id", middleware.FromGin(c)),
			zap.Int("page", filter.Page),
			zap.Int("page_size", filter.PageSize),
			zap.Error(err),
		)
		h.writeOpsQueryError(c, ctx, err, "query analysis tasks failed")
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

func (h *AnalysisTaskOpsHandler) GetMetrics(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "msg": "operations service is not initialized"})
		return
	}
	windowHours, err := h.parseWindowHours(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(h.config.QueryTimeoutSeconds)*time.Second)
	defer cancel()
	result, err := h.service.GetMetrics(ctx, windowHours)
	if err != nil {
		h.logger.Error("ops_task_metrics_query_failed",
			zap.String("request_id", middleware.FromGin(c)),
			zap.Int("window_hours", windowHours),
			zap.Error(err),
		)
		h.writeOpsQueryError(c, ctx, err, "query analysis task metrics failed")
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

func (h *AnalysisTaskOpsHandler) GetWorkers(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "msg": "operations service is not initialized"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Duration(h.config.QueryTimeoutSeconds)*time.Second)
	defer cancel()
	result, err := h.service.GetWorkers(ctx)
	if err != nil {
		h.logger.Error("ops_worker_snapshot_failed",
			zap.String("request_id", middleware.FromGin(c)),
			zap.Error(err),
		)
		h.writeOpsQueryError(c, ctx, err, "query worker runtime status failed")
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

func (h *AnalysisTaskOpsHandler) parseTaskListFilter(c *gin.Context) (repository.TaskListFilter, error) {
	filter := repository.TaskListFilter{Page: 1, PageSize: h.config.DefaultPageSize}

	if raw, exists := c.GetQuery("page"); exists {
		page, err := strconv.Atoi(raw)
		if err != nil || page < 1 {
			return filter, errors.New("page must be greater than or equal to 1")
		}
		filter.Page = page
	}
	if raw, exists := c.GetQuery("page_size"); exists {
		pageSize, err := strconv.Atoi(raw)
		if err != nil || pageSize <= 0 {
			return filter, errors.New("page_size must be greater than 0")
		}
		if pageSize > h.config.MaxPageSize {
			return filter, errors.New("page_size exceeds configured maximum")
		}
		filter.PageSize = pageSize
	}
	if filter.Page > 1 && filter.Page-1 > math.MaxInt/filter.PageSize {
		return filter, errors.New("page is too large")
	}

	if raw, exists := c.GetQuery("status"); exists {
		switch raw {
		case model.AnalysisTaskStatusPending,
			model.AnalysisTaskStatusRunning,
			model.AnalysisTaskStatusSucceeded,
			model.AnalysisTaskStatusFailed:
			filter.Status = &raw
		default:
			return filter, errors.New("invalid analysis task status")
		}
	}
	if raw, exists := c.GetQuery("project_id"); exists {
		value, err := parsePositiveUintQuery(raw)
		if err != nil {
			return filter, errors.New("project_id must be a positive integer")
		}
		filter.ProjectID = &value
	}
	if raw, exists := c.GetQuery("diff_id"); exists {
		value, err := parsePositiveUintQuery(raw)
		if err != nil {
			return filter, errors.New("diff_id must be a positive integer")
		}
		filter.DiffID = &value
	}
	if raw, exists := c.GetQuery("error_code"); exists {
		raw = strings.TrimSpace(raw)
		if raw == "" || len(raw) > 64 {
			return filter, errors.New("error_code must be between 1 and 64 characters")
		}
		filter.ErrorCode = &raw
	}
	if raw, exists := c.GetQuery("degraded"); exists {
		switch raw {
		case "true":
			value := true
			filter.Degraded = &value
		case "false":
			value := false
			filter.Degraded = &value
		default:
			return filter, errors.New("degraded must be true or false")
		}
	}
	if raw, exists := c.GetQuery("created_from"); exists {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return filter, errors.New("created_from must use RFC3339")
		}
		filter.CreatedFrom = &value
	}
	if raw, exists := c.GetQuery("created_to"); exists {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return filter, errors.New("created_to must use RFC3339")
		}
		filter.CreatedTo = &value
	}
	if filter.CreatedFrom != nil && filter.CreatedTo != nil && filter.CreatedFrom.After(*filter.CreatedTo) {
		return filter, errors.New("created_from cannot be later than created_to")
	}
	return filter, nil
}

func (h *AnalysisTaskOpsHandler) parseWindowHours(c *gin.Context) (int, error) {
	raw, exists := c.GetQuery("window_hours")
	if !exists {
		return h.config.DefaultMetricsWindowHours, nil
	}
	windowHours, err := strconv.Atoi(raw)
	if err != nil || windowHours <= 0 {
		return 0, errors.New("window_hours must be greater than 0")
	}
	if windowHours > h.config.MaxMetricsWindowHours {
		return 0, errors.New("window_hours exceeds configured maximum")
	}
	return windowHours, nil
}

func (h *AnalysisTaskOpsHandler) writeOpsQueryError(c *gin.Context, ctx context.Context, err error, fallback string) {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		c.JSON(http.StatusGatewayTimeout, gin.H{"code": 1, "msg": "operations query timed out"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "msg": fallback})
}

func parsePositiveUintQuery(raw string) (uint, error) {
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || value == 0 || uint64(uint(value)) != value {
		return 0, errors.New("invalid positive integer")
	}
	return uint(value), nil
}
