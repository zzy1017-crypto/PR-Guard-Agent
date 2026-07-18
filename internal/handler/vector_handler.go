package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"pr-guard-agent/internal/service"
)

type VectorHandler struct {
	service *service.VectorService
}

func NewVectorHandler(service *service.VectorService) *VectorHandler {
	return &VectorHandler{service: service}
}

// 创建或确认Qdrant Collection
func (h *VectorHandler) InitCollection(c *gin.Context) {
	//调用VectorService的InitCollection方法执行初始化，并处理返回结果或错误。
	result, err := h.service.InitCollection(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": result,
	})
}

// 生成固定测试代码的向量并写测试point
func (h *VectorHandler) TestUpsert(c *gin.Context) {
	result, err := h.service.TestUpsert(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": result,
	})
}

// 查询相同测试内容的向量并返回TopK
func (h *VectorHandler) TestSearch(c *gin.Context) {
	topK, err := parseTopK(c.Query("top_k"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": err.Error()})
		return
	}

	result, err := h.service.TestSearch(c.Request.Context(), topK)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": result,
	})
}

// parseTopK 解析top_k参数，确保其为正整数，并在未提供或为0时返回默认值5，最大值为20。
func parseTopK(raw string) (uint64, error) {
	if raw == "" {
		return 0, nil
	}
	topK, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, err
	}
	return topK, nil
}
