package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"pr-guard-agent/internal/service"
)

// IndexHandler 处理索引相关的HTTP请求。
type IndexHandler struct {
	indexService *service.IndexService
}

// NewIndexHandler 创建一个新的IndexHandler实例，使用提供的IndexService进行初始化。
func NewIndexHandler(indexService *service.IndexService) *IndexHandler {
	return &IndexHandler{indexService: indexService}
}

// 把请求Context传给索引过程，支持超时/取消传播
func (h *IndexHandler) IndexProject(c *gin.Context) {
	handleIndexProject(c, func(projectID uint) (*service.IndexProjectResult, error) {
		return h.indexService.IndexProjectWithContext(c.Request.Context(), projectID)
	})
}

// 兼容旧式包级调用，走包级函数，支持超时/取消传播
func IndexProject(c *gin.Context) {
	handleIndexProject(c, service.IndexProject)
}

// 统一解析项目ID、调用索引函数、映射项目不存在和内部错误；减少两种入口的重复代码。
func handleIndexProject(c *gin.Context, indexProject func(projectID uint) (*service.IndexProjectResult, error)) {
	//解析项目ID参数，如果解析失败则返回400错误。
	projectID64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || projectID64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": "1", "msg": "invalid project id"})
		return
	}

	//调用索引函数进行项目索引，并处理返回结果或错误。
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
