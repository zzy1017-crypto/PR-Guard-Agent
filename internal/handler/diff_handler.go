package handler

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"pr-guard-agent/internal/database"
	"pr-guard-agent/internal/service"
)

const maxDiffSize = 50 << 20

// UploadDiff 处理上传diff文件的HTTP请求。它支持通过文件上传或直接提交diff文本的方式，并对输入进行验证和限制。
func UploadDiff(c *gin.Context) {
	// 设置请求体大小限制，防止上传过大的diff文件。
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxDiffSize+1<<20)

	// 解析项目ID参数，并进行相应的验证。如果参数无效，则返回400错误。
	projectID64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || projectID64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": "invalid project id"})
		return
	}

	// 读取diff文本，可以通过文件上传或直接提交diff文本的方式。如果读取失败，则返回相应的错误信息。
	diffText, err := readDiffText(c)
	if err != nil {
		status := http.StatusBadRequest
		if isDiffTooLargeError(err) {
			status = http.StatusRequestEntityTooLarge
		}
		c.JSON(status, gin.H{"code": 1, "msg": err.Error()})
		return
	}

	// 调用DiffService的UploadDiff方法上传diff文本，并处理返回结果或错误。
	diffService := service.NewDiffService(database.DB)
	result, err := diffService.UploadDiff(uint(projectID64), diffText)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "project not found") {
			status = http.StatusNotFound
		} else if isDiffInputError(err) {
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

// 优先读取.diff/.patch文件，如果没有则读取diff_text参数。限制大小为50MB，空内容返回错误。
func readDiffText(c *gin.Context) (string, error) {
	fileHeader, fileErr := c.FormFile("file")
	if fileErr == nil {
		ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
		if ext != ".diff" && ext != ".patch" {
			return "", errDiffInvalidFileExt()
		}
		if fileHeader.Size > maxDiffSize {
			return "", errDiffTooLarge()
		}

		file, err := fileHeader.Open()
		if err != nil {
			return "", errDiffOpenFailed()
		}
		defer file.Close()

		content, err := io.ReadAll(io.LimitReader(file, maxDiffSize+1))
		if err != nil {
			return "", errDiffReadFailed()
		}
		if len(content) > maxDiffSize {
			return "", errDiffTooLarge()
		}
		if strings.TrimSpace(string(content)) == "" {
			return "", errDiffEmpty()
		}
		return string(content), nil
	}

	diffText := c.PostForm("diff_text")
	if strings.Contains(fileErr.Error(), "request body too large") {
		return "", errDiffTooLarge()
	}
	if len([]byte(diffText)) > maxDiffSize {
		return "", errDiffTooLarge()
	}
	if strings.TrimSpace(diffText) == "" {
		return "", errDiffEmpty()
	}
	return diffText, nil
}

// isDiffTooLargeError检查错误是否与diff大小超过限制相关。
func isDiffTooLargeError(err error) bool {
	return strings.Contains(err.Error(), "diff size exceeds 50MB") ||
		strings.Contains(err.Error(), "request body too large")
}

// isDiffInputError检查错误是否与diff输入无效相关。
func isDiffInputError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "diff text is empty") ||
		strings.Contains(msg, "parse diff failed")
}

// errDiffInvalidFileExt返回一个错误，表示只允许上传.diff或.patch文件。
func errDiffInvalidFileExt() error {
	return errors.New("only .diff or .patch file is allowed")
}

// errDiffTooLarge返回一个错误，表示diff大小超过50MB。
func errDiffTooLarge() error {
	return errors.New("diff size exceeds 50MB")
}

// errDiffOpenFailed返回一个错误，表示打开上传的diff文件失败。
func errDiffOpenFailed() error {
	return errors.New("open uploaded diff file failed")
}

// errDiffReadFailed返回一个错误，表示读取上传的diff文件失败。
func errDiffReadFailed() error {
	return errors.New("read uploaded diff file failed")
}

// errDiffEmpty返回一个错误，表示diff文本为空。
func errDiffEmpty() error {
	return errors.New("diff text is empty")
}
