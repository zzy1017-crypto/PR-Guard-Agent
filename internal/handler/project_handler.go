package handler

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"pr-guard-agent/internal/database"
	"pr-guard-agent/internal/service"
)

const maxUploadSize = 20 << 20 //限制上传文件大小为20MB，保护后端安全

func UploadProject(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadSize+1<<20)

	//从multipart表单中获取名为"file"的上传文件
	fileHeader, err := c.FormFile("file")
	if err != nil {
		//处理上传文件错误，如果错误信息包含"request body too large"，则返回请求实体过大的响应，否则返回请求错误的响应
		if strings.Contains(err.Error(), "request body too large") {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"code": 1, "msg": "zip file size exceeds 20MB"})
			return
		}
		//处理其他上传文件错误，返回请求错误的响应
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": "file is required"})
		return
	}

	//从表单中获取名为"project_name"的项目名称，并去除前后空格
	projectName := strings.TrimSpace(c.PostForm("project_name"))
	if projectName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": "project_name is required"})
		return
	}

	//检查上传文件的大小和类型，如果文件大小超过20MB，则返回相应的错误响应
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

func isUploadInputError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "project_name is required") ||
		strings.Contains(msg, "uploaded zip file is empty") ||
		strings.Contains(msg, "extract zip failed")
}
