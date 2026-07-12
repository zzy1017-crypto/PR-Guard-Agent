package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestRecoveryCatchesPanicAndReturnsRequestID(t *testing.T) {
	r := gin.New()
	r.Use(RequestID(), Recovery(zap.NewNop()))
	r.GET("/panic", func(*gin.Context) { panic("test panic") })

	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/panic", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", response.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["request_id"] == "" || body["request_id"] != response.Header().Get("X-Request-ID") {
		t.Fatalf("response request_id = %#v, header = %q", body["request_id"], response.Header().Get("X-Request-ID"))
	}
	if _, exists := body["stack"]; exists {
		t.Fatal("panic stack leaked to client")
	}
}
