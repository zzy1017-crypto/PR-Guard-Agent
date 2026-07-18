package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"pr-guard-agent/internal/service"
)

type RAGHandler struct {
	service *service.RAGService
}

func NewRAGHandler(service *service.RAGService) *RAGHandler {
	return &RAGHandler{service: service}
}

// 解析项目、diff和TopK，执行检索相关代码块并返回文件、符号、上下文。
func (h *RAGHandler) RetrieveRelatedChunks(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "msg": "rag service is not initialized"})
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

	topK, err := parseRetrieveTopK(c.Query("top_k"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": err.Error()})
		return
	}

	//调用RAGService的RetrieveRelatedChunks方法执行检索相关代码块，并处理返回结果或错误。
	result, err := h.service.RetrieveRelatedChunks(c.Request.Context(), uint(projectID64), uint(diffID64), topK)
	if err != nil {
		status := retrieveErrorStatus(err)
		c.JSON(status, gin.H{"code": 1, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "related chunks retrieved",
		"data": result,
	})
}

// parseRetrieveTopK 解析top_k参数，确保其为正整数，并在未提供或为0时返回默认值5，最大值为20。
func parseRetrieveTopK(raw string) (int, error) {
	if raw == "" {
		return 5, nil
	}

	topK, err := strconv.Atoi(raw)
	if err != nil || topK < 0 {
		return 0, errors.New("invalid top_k")
	}
	if topK == 0 {
		return 5, nil
	}
	if topK > 20 {
		return 20, nil
	}
	return topK, nil
}

// 不存在映射404，归属或空diff映射400，其他映射500；避免对外暴露内部错误。
func retrieveErrorStatus(err error) int {
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
