package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"pr-guard-agent/internal/service"
)

func IndexProject(c *gin.Context) {
	projectID64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || projectID64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "1", "msg": "invalid project id"})
		return
	}

	result, err := service.IndexProject(uint(projectID64))
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
