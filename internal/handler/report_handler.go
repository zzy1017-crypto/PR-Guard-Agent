package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"pr-guard-agent/internal/service"
)

type ReportHandler struct {
	service *service.ReportService
}

func NewReportHandler(service *service.ReportService) *ReportHandler {
	return &ReportHandler{service: service}
}

// AnalyzeDiff 解析项目、diff和TopK，执行分析并返回风险报告。
func (h *ReportHandler) AnalyzeDiff(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "msg": "report service is not initialized"})
		return
	}

	//解析项目ID、diffID和TopK参数，如果解析失败则返回400错误。
	projectID64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || projectID64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": "invalid project id"})
		return
	}

	diffID64, err := strconv.ParseUint(c.Param("diff_id"), 10, 64)
	if err != nil || diffID64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": "invalid diff id"})
		return
	}

	topK, err := parseAnalyzeTopK(c.Query("top_K"), c.Query("top_k"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": err.Error()})
		return
	}

	//调用ReportService的AnalyzeDiff方法执行分析，并处理返回结果或错误。
	result, err := h.service.AnalyzeDiff(c.Request.Context(), uint(projectID64), uint(diffID64), topK)
	if err != nil {
		status := analyzeErrorStatus(err)
		c.JSON(status, gin.H{"code": 1, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "risk report generated",
		"data": result,
	})
}

// parseAnalyzeTopK 解析top_K参数，确保其为正整数，并在未提供或为0时返回默认值5，最大值为20。
func parseAnalyzeTopK(primary string, fallback string) (int, error) {
	raw := primary
	if raw == "" {
		raw = fallback
	}
	if raw == "" {
		return 5, nil
	}

	topK, err := strconv.Atoi(raw)
	if err != nil || topK < 0 {
		return 0, errors.New("invalid top_K")
	}
	if topK == 0 {
		return 5, nil
	}
	if topK > 20 {
		return 20, nil
	}
	return topK, nil
}

// analyzeErrorStatus 根据错误类型返回相应的HTTP状态码，避免对外暴露内部错误。
func analyzeErrorStatus(err error) int {
	switch {
	case errors.Is(err, service.ErrProjectNotFound),
		errors.Is(err, service.ErrDiffNotFound):
		return http.StatusNotFound
	case errors.Is(err, service.ErrDiffProjectMismatch),
		errors.Is(err, service.ErrDiffTextEmpty):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
