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

// 定义一个接口analysisTaskOpsService，包含ListTasks、GetMetrics和GetWorkers方法，用于分析任务运维相关的操作。
type analysisTaskOpsService interface {
	ListTasks(ctx context.Context, filter repository.TaskListFilter) (*service.TaskListResult, error)
	GetMetrics(ctx context.Context, windowHours int) (*service.TaskMetricsResult, error)
	GetWorkers(ctx context.Context) (*service.WorkerStatusResult, error)
}

// AnalysisTaskOpsHandler 处理分析任务运维相关的HTTP请求。
type AnalysisTaskOpsHandler struct {
	service analysisTaskOpsService
	config  config.OpsConfig
	logger  *zap.Logger
}

// 注入Ops Service、OpsConfig和Logger，创建一个新的AnalysisTaskOpsHandler实例。nil Logger降级为Nop
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

// 解析过滤/分页，为查询设置超时，记录失败日志并返回安全结果。
func (h *AnalysisTaskOpsHandler) ListTasks(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "msg": "operations service is not initialized"})
		return
	}
	//解析过滤器参数，如果无效则返回400错误。
	filter, err := h.parseTaskListFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": err.Error()})
		return
	}
	//设置查询超时，并调用service.ListTasks方法获取分析任务列表，如果查询失败则记录日志并返回500错误。
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

// 解析指标窗口时间，为查询设置超时并获取队列指标。
func (h *AnalysisTaskOpsHandler) GetMetrics(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "msg": "operations service is not initialized"})
		return
	}
	// 解析指标窗口时间参数，如果无效则返回400错误。
	windowHours, err := h.parseWindowHours(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": err.Error()})
		return
	}
	//设置查询超时，并调用service.GetMetrics方法获取分析任务指标，如果查询失败则记录日志并返回500错误。
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

// 在超时Context中获取进程内Worker快照。
func (h *AnalysisTaskOpsHandler) GetWorkers(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "msg": "operations service is not initialized"})
		return
	}
	//设置查询超时，并调用service.GetWorkers方法获取分析任务Worker快照，如果查询失败则记录日志并返回500错误。
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

// 校验页码、页大小、状态、项目/diff、错误码、degraded和创建时间范围参数，防止非法SQL参数和超大Offset；如果无效则返回400错误。
func (h *AnalysisTaskOpsHandler) parseTaskListFilter(c *gin.Context) (repository.TaskListFilter, error) {
	//初始化过滤器，设置默认页码和页大小。
	filter := repository.TaskListFilter{Page: 1, PageSize: h.config.DefaultPageSize}

	//解析分页参数，如果存在则进行验证，确保页码大于等于1，页大小大于0且不超过配置的最大值，并防止页码和页大小的乘积溢出。
	if raw, exists := c.GetQuery("page"); exists {
		page, err := strconv.Atoi(raw)
		if err != nil || page < 1 {
			return filter, errors.New("page must be greater than or equal to 1")
		}
		filter.Page = page
	}
	//解析页大小参数，如果存在则进行验证，确保页大小大于0且不超过配置的最大值，并防止页码和页大小的乘积溢出。
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
	//防止页码和页大小的乘积溢出，确保查询的Offset不会超过int的最大值。
	if filter.Page > 1 && filter.Page-1 > math.MaxInt/filter.PageSize {
		return filter, errors.New("page is too large")
	}

	//解析任务状态，并进行相应的验证。如果参数无效，则返回400错误。
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
	//解析项目ID，并进行相应的验证。如果参数无效，则返回400错误。
	if raw, exists := c.GetQuery("project_id"); exists {
		value, err := parsePositiveUintQuery(raw)
		if err != nil {
			return filter, errors.New("project_id must be a positive integer")
		}
		filter.ProjectID = &value
	}
	//解析Diff ID，并进行相应的验证。如果参数无效，则返回400错误。
	if raw, exists := c.GetQuery("diff_id"); exists {
		value, err := parsePositiveUintQuery(raw)
		if err != nil {
			return filter, errors.New("diff_id must be a positive integer")
		}
		filter.DiffID = &value
	}
	//解析错误码参数，并进行相应的验证。如果参数无效，则返回400错误。
	if raw, exists := c.GetQuery("error_code"); exists {
		raw = strings.TrimSpace(raw)
		if raw == "" || len(raw) > 64 {
			return filter, errors.New("error_code must be between 1 and 64 characters")
		}
		filter.ErrorCode = &raw
	}
	//解析degraded参数，并进行相应的验证。如果参数无效，则返回400错误。
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
	//解析创建时间范围参数，并进行相应的验证。如果参数无效，则返回400错误。
	if raw, exists := c.GetQuery("created_from"); exists {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return filter, errors.New("created_from must use RFC3339")
		}
		filter.CreatedFrom = &value
	}
	//如果created_to参数存在，则进行相应的验证，确保其格式正确，并且不早于created_from。如果参数无效，则返回400错误。
	if filter.CreatedFrom != nil && filter.CreatedTo != nil && filter.CreatedFrom.After(*filter.CreatedTo) {
		return filter, errors.New("created_from cannot be later than created_to")
	}
	return filter, nil
}

// 使用默认窗口并限制最大窗口
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

// 超时映射为504，其他错误映射为500，避免泄露敏感信息。
func (h *AnalysisTaskOpsHandler) writeOpsQueryError(c *gin.Context, ctx context.Context, err error, fallback string) {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		c.JSON(http.StatusGatewayTimeout, gin.H{"code": 1, "msg": "operations query timed out"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "msg": fallback})
}

// 安全解析正整数查询参数，如果无效则返回错误。
func parsePositiveUintQuery(raw string) (uint, error) {
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || value == 0 || uint64(uint(value)) != value {
		return 0, errors.New("invalid positive integer")
	}
	return uint(value), nil
}
