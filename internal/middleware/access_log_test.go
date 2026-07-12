package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestAccessLogRecordsStatusAndLatency(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	r := gin.New()
	r.Use(RequestID(), AccessLog(logger))
	r.GET("/created", func(c *gin.Context) { c.Status(http.StatusCreated) })

	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/created", nil))

	entries := logs.FilterMessage("http_request").All()
	if len(entries) != 1 {
		t.Fatalf("http_request log count = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["status"] != int64(http.StatusCreated) {
		t.Fatalf("status field = %#v, want %d", fields["status"], http.StatusCreated)
	}
	if _, ok := fields["latency_ms"]; !ok {
		t.Fatal("latency_ms field is missing")
	}
}
