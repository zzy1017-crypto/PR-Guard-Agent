package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"pr-guard-agent/internal/service"
)

type EmbeddingHandler struct {
	service *service.EmbeddingService
}

type embeddingTestRequest struct {
	Text string `json:"text"`
}

// NewEmbeddingHandler 创建一个新的EmbeddingHandler实例，使用提供的EmbeddingService进行初始化。
func NewEmbeddingHandler(service *service.EmbeddingService) *EmbeddingHandler {
	return &EmbeddingHandler{service: service}
}

// 绑定{text}JSON，执行测试Embedding并返回维度及是否Mock；用于开发联调，不是业务分析接口。
func (h *EmbeddingHandler) Test(c *gin.Context) {
	var req embeddingTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": "invalid json body"})
		return
	}

	result, err := h.service.Test(c.Request.Context(), req.Text)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": result,
	})
}
