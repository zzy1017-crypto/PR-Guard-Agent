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

// 运行内置订单场景的LLM测试，生成风险报告；用于Provider联调。
func (h *LLMHandler) RiskTest(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "msg": "llm service is not initialized"})
		return
	}

	//调用LLMService的TestRiskReport方法执行风险报告生成，并处理返回结果或错误。
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
