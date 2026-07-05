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

func (h *VectorHandler) InitCollection(c *gin.Context) {
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
