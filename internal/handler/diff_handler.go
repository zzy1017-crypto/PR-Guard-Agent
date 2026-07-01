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

func UploadDiff(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxDiffSize+1<<20)

	projectID64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || projectID64 == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "msg": "invalid project id"})
		return
	}

	diffText, err := readDiffText(c)
	if err != nil {
		status := http.StatusBadRequest
		if isDiffTooLargeError(err) {
			status = http.StatusRequestEntityTooLarge
		}
		c.JSON(status, gin.H{"code": 1, "msg": err.Error()})
		return
	}

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

func isDiffTooLargeError(err error) bool {
	return strings.Contains(err.Error(), "diff size exceeds 50MB") ||
		strings.Contains(err.Error(), "request body too large")
}

func isDiffInputError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "diff text is empty") ||
		strings.Contains(msg, "parse diff failed")
}

func errDiffInvalidFileExt() error {
	return errors.New("only .diff or .patch file is allowed")
}

func errDiffTooLarge() error {
	return errors.New("diff size exceeds 50MB")
}

func errDiffOpenFailed() error {
	return errors.New("open uploaded diff file failed")
}

func errDiffReadFailed() error {
	return errors.New("read uploaded diff file failed")
}

func errDiffEmpty() error {
	return errors.New("diff text is empty")
}
