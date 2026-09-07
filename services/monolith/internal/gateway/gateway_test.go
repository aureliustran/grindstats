package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// A route registered on a fresh Server passes through every stage of the
// chain New assembles. These assert the observable effect of each stage
// rather than inspecting gin's internal handler slice, since that is what a
// caller (or a later story's own handler) actually depends on.
func TestNew_ChainAppliesEveryStage(t *testing.T) {
	srv := New(Deps{AllowedOrigins: []string{"http://localhost:5173"}})
	srv.Engine.GET("/probe", func(c *gin.Context) { c.Status(http.StatusOK) })
	srv.Engine.GET("/panic", func(c *gin.Context) { panic("boom") })

	t.Run("request ID is generated and echoed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/probe", nil))
		if rec.Header().Get("X-Request-ID") == "" {
			t.Error("X-Request-ID missing from response")
		}
	})

	t.Run("locale sets Content-Language", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/probe", nil)
		req.Header.Set("Accept-Language", "vi-VN")
		rec := httptest.NewRecorder()
		srv.Engine.ServeHTTP(rec, req)
		if got := rec.Header().Get("Content-Language"); got != "vi-VN" {
			t.Errorf("Content-Language = %q, want vi-VN", got)
		}
	})

	t.Run("CORS reflects an allowed origin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/probe", nil)
		req.Header.Set("Origin", "http://localhost:5173")
		rec := httptest.NewRecorder()
		srv.Engine.ServeHTTP(rec, req)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
			t.Errorf("Access-Control-Allow-Origin = %q, want the allowed origin", got)
		}
	})

	t.Run("recovery turns a panic into the error envelope", func(t *testing.T) {
		rec := httptest.NewRecorder()
		srv.Engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/panic", nil))
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", rec.Code)
		}
	})
}

func TestNew_V1GroupExistsAndEmpty(t *testing.T) {
	srv := New(Deps{})

	rec := httptest.NewRecorder()
	srv.Engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil))

	// The group exists (created by New) but this story registers no routes
	// under it, so any path there 404s until AUTH-002 adds one.
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for an unregistered /api/v1 route", rec.Code)
	}
}

func TestNew_DefaultsLoggerWhenNil(t *testing.T) {
	// Must not panic when Deps.Logger is left zero-valued.
	srv := New(Deps{})
	srv.Engine.GET("/probe", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	srv.Engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/probe", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}
