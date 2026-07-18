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

// 限制ZIP大小、校验名称和扩展名、读取内容并调用ProjectService.
func UploadProject(c *gin.Context) {
	// 设置请求体大小限制，防止上传过大的ZIP文件。
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadSize+1<<20)

	// 读取上传的ZIP文件，如果未提供文件或文件过大，则返回相应的错误信息。
	fileHeader, err := c.FormFile("file")
	if err != nil {
		if strings.Contains(err.Error(), "request body too large") {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"code": 1, "msg": "zip file size exceeds 20MB"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": "file is required"})
		return
	}

	// 获取项目名称参数，并进行相应的验证。如果参数无效，则返回400错误。
	projectName := strings.TrimSpace(c.PostForm("project_name"))
	if projectName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": "project_name is required"})
		return
	}

	// 检查上传的ZIP文件大小和扩展名，如果不符合要求，则返回相应的错误信息。
	if fileHeader.Size > maxUploadSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"code": 1, "msg": "zip file size exceeds 20MB"})
		return
	}
	// 检查文件扩展名是否为.zip，如果不是，则返回400错误。
	if strings.ToLower(filepath.Ext(fileHeader.Filename)) != ".zip" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": "only .zip file is allowed"})
		return
	}

	// 打开上传的ZIP文件，并读取其内容。如果读取失败或文件过大，则返回相应的错误信息。
	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": "open uploaded file failed"})
		return
	}
	defer file.Close()

	// 读取ZIP文件内容，并限制最大读取大小为20MB。如果读取失败或文件过大，则返回相应的错误信息。
	zipContent, err := io.ReadAll(io.LimitReader(file, maxUploadSize+1))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": "read uploaded file failed"})
		return
	}
	if len(zipContent) > maxUploadSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"code": 1, "msg": "zip file size exceeds 20MB"})
		return
	}

	// 调用ProjectService的UploadProject方法处理上传的项目ZIP文件，并返回结果或错误信息。
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

// 兼容接口，只对已有项目Go文件生成AST Chunk并写MySQL；正式完整索引还应使用/index
func GenerateASTChunks(c *gin.Context) {
	// 解析项目ID参数，如果解析失败则返回400错误。
	projectID64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || projectID64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": "invalid project id"})
		return
	}

	// 调用ChunkService的GenerateASTChunks方法生成AST代码块，并处理返回结果或错误。
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

// isUploadInputError 判断上传项目的错误是否属于输入错误类型，避免对外暴露内部错误。
func isUploadInputError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "project_name is required") ||
		strings.Contains(msg, "uploaded zip file is empty") ||
		strings.Contains(msg, "extract zip failed")
}
