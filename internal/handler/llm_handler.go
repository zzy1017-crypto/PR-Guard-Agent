package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"pr-guard-agent/internal/service"
)

type LLMHandler struct {
	service *service.LLMService
}

func NewLLMHandler(service *service.LLMService) *LLMHandler {
	return &LLMHandler{service: service}
}

func (h *LLMHandler) RiskTest(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "msg": "llm service is not initialized"})
		return
	}

	result, err := h.service.TestRiskReport(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "risk report generated",
		"data": result,
	})
}
