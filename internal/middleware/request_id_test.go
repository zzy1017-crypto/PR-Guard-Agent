package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"pr-guard-agent/pkg/requestid"
)

func TestRequestIDGeneratedWhenMissing(t *testing.T) {
	response := performRequestIDRequest(t, "")
	got := response.Header().Get(requestid.HeaderName)
	if !requestid.IsValid(got) || len(got) != 36 {
		t.Fatalf("generated request ID = %q, want valid UUID", got)
	}
}

func TestRequestIDReusesValidHeader(t *testing.T) {
	want := "client-request_123.test"
	response := performRequestIDRequest(t, want)
	if got := response.Header().Get(requestid.HeaderName); got != want {
		t.Fatalf("response request ID = %q, want %q", got, want)
	}
}

func TestRequestIDReplacesInvalidOrLongHeader(t *testing.T) {
	for _, incoming := range []string{"unsafe/request/id", strings.Repeat("a", requestid.MaxLength+1)} {
		t.Run(incoming[:min(len(incoming), 16)], func(t *testing.T) {
			response := performRequestIDRequest(t, incoming)
			got := response.Header().Get(requestid.HeaderName)
			if got == incoming || !requestid.IsValid(got) {
				t.Fatalf("replacement request ID = %q", got)
			}
		})
	}
}

func performRequestIDRequest(t *testing.T, incoming string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	r.GET("/test", func(c *gin.Context) {
		if FromGin(c) == "" || requestid.FromContext(c.Request.Context()) != FromGin(c) {
			t.Error("request ID was not propagated to both contexts")
		}
		c.Status(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	if incoming != "" {
		req.Header.Set(requestid.HeaderName, incoming)
	}
	response := httptest.NewRecorder()
	r.ServeHTTP(response, req)
	return response
}
