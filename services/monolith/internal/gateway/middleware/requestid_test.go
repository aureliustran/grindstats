package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newRouter(mw gin.HandlerFunc, handler gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.Use(mw)
	r.GET("/", handler)
	return r
}

func TestRequestID_GeneratedWhenAbsent(t *testing.T) {
	var seen1, seen2 string
	r := newRouter(RequestID(), func(c *gin.Context) {
		seen1 = RequestIDFrom(c)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if seen1 == "" {
		t.Fatal("RequestIDFrom returned empty inside the handler")
	}
	if got := rec.Header().Get(RequestIDHeader); got != seen1 {
		t.Errorf("response header %q, want %q", got, seen1)
	}

	// A second, independent request must not reuse the first ID.
	r2 := newRouter(RequestID(), func(c *gin.Context) {
		seen2 = RequestIDFrom(c)
	})
	r2.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if seen1 == seen2 {
		t.Error("two requests generated the same request ID")
	}
}

func TestRequestID_EchoedWhenSupplied(t *testing.T) {
	r := newRouter(RequestID(), func(c *gin.Context) {})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(RequestIDHeader, "caller-supplied-42")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get(RequestIDHeader); got != "caller-supplied-42" {
		t.Errorf("response header %q, want %q", got, "caller-supplied-42")
	}
}

func TestRequestIDFrom_EmptyWithoutMiddleware(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)

	if got := RequestIDFrom(c); got != "" {
		t.Errorf("RequestIDFrom on bare context = %q, want empty", got)
	}
}
