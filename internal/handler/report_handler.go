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

func (h *ReportHandler) AnalyzeDiff(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "msg": "report service is not initialized"})
		return
	}

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
