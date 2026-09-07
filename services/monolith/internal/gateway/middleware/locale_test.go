package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"grindstats/libs/httpkit"
)

func TestLocale_ResolvesAndStoresOnContext(t *testing.T) {
	var stored string
	r := newRouter(Locale(), func(c *gin.Context) {
		v, _ := c.Get(httpkit.LocaleContextKey)
		stored, _ = v.(string)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "vi-VN")
	r.ServeHTTP(httptest.NewRecorder(), req)

	if stored != "vi-VN" {
		t.Errorf("stored locale = %q, want vi-VN", stored)
	}
}

func TestLocale_SetsContentLanguageHeader(t *testing.T) {
	r := newRouter(Locale(), func(c *gin.Context) {})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "vi-VN")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Language"); got != "vi-VN" {
		t.Errorf("Content-Language = %q, want vi-VN", got)
	}
}

func TestLocale_UnsupportedHeaderFallsBackToSource(t *testing.T) {
	r := newRouter(Locale(), func(c *gin.Context) {})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "de-DE")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get("Content-Language"); got != "en-US" {
		t.Errorf("Content-Language = %q, want en-US fallback", got)
	}
}
