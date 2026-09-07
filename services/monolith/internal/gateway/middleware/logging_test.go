package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLogging_WritesOneLineCarryingRequestID(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))

	r := gin.New()
	r.Use(RequestID(), Logging(logger))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(RequestIDHeader, "test-req-99")
	r.ServeHTTP(httptest.NewRecorder(), req)

	line := logs.String()
	if !strings.Contains(line, "test-req-99") {
		t.Errorf("log line %q does not carry the request ID", line)
	}
	if !strings.Contains(line, `"status":200`) {
		t.Errorf("log line %q does not carry the response status", line)
	}
}
