package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"pr-guard-agent/internal/service"
)

type IndexHandler struct {
	indexService *service.IndexService
}

func NewIndexHandler(indexService *service.IndexService) *IndexHandler {
	return &IndexHandler{indexService: indexService}
}

func (h *IndexHandler) IndexProject(c *gin.Context) {
	handleIndexProject(c, func(projectID uint) (*service.IndexProjectResult, error) {
		return h.indexService.IndexProjectWithContext(c.Request.Context(), projectID)
	})
}

func IndexProject(c *gin.Context) {
	handleIndexProject(c, service.IndexProject)
}

func handleIndexProject(c *gin.Context, indexProject func(projectID uint) (*service.IndexProjectResult, error)) {
	projectID64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || projectID64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "1", "msg": "invalid project id"})
		return
	}

	result, err := indexProject(uint(projectID64))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrProjectNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"code": "1", "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": "0",
		"msg":  "project indexed",
		"data": result,
	})
}
