package handler

import (
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"pr-guard-agent/internal/database"
	"pr-guard-agent/internal/service"
)

const maxUploadSize = 20 << 20

func UploadProject(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadSize+1<<20)

	fileHeader, err := c.FormFile("file")
	if err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"code": 1, "msg": "zip file size exceeds 20MB"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": "file is required"})
		return
	}

	projectName := strings.TrimSpace(c.PostForm("project_name"))
	if projectName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": "project_name is required"})
		return
	}

	if fileHeader.Size > maxUploadSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"code": 1, "msg": "zip file size exceeds 20MB"})
		return
	}
	if strings.ToLower(filepath.Ext(fileHeader.Filename)) != ".zip" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": "only .zip file is allowed"})
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": "open uploaded file failed"})
		return
	}
	defer file.Close()

	zipContent, err := io.ReadAll(io.LimitReader(file, maxUploadSize+1))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": "read uploaded file failed"})
		return
	}
	if len(zipContent) > maxUploadSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"code": 1, "msg": "zip file size exceeds 20MB"})
		return
	}

	projectService := service.NewProjectService(database.DB)
	result, err := projectService.UploadProject(projectName, zipContent)
	if err != nil {
		status := http.StatusInternalServerError
		if isUploadInputError(err) {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"code": 1, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "success",
		"data": result,
	})
}

func GenerateASTChunks(c *gin.Context) {
	projectID64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || projectID64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": "invalid project id"})
		return
	}

	chunkService := service.NewChunkService(database.DB)
	count, err := chunkService.GenerateASTChunks(uint(projectID64))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "ast chunks generated",
		"data": gin.H{
			"project_id":  uint(projectID64),
			"chunk_count": count,
		},
	})
}

func isUploadInputError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "project_name is required") ||
		strings.Contains(msg, "uploaded zip file is empty") ||
		strings.Contains(msg, "extract zip failed")
}
