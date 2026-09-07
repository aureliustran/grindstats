package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newRouter(h Handler) *gin.Engine {
	r := gin.New()
	h.RegisterRoutes(r)
	return r
}

func readyBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return body
}

func alwaysOK(context.Context) error { return nil }

func neverCalled(t *testing.T) Checker {
	return func(context.Context) error {
		t.Fatal("liveness must not invoke any dependency checker")
		return nil
	}
}

func TestLiveness_DoesNotInvokeAnyDependencyChecker(t *testing.T) {
	r := newRouter(Handler{
		Dependencies: []Dependency{
			{Name: "postgres", Required: true, Check: neverCalled(t)},
			{Name: "redis", Required: false, Check: neverCalled(t)},
		},
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := readyBody(t, rec)["status"]; got != "alive" {
		t.Errorf(`status field = %v, want "alive"`, got)
	}
}

func TestReadiness_AllDependenciesUpReturnsReady(t *testing.T) {
	r := newRouter(Handler{
		Dependencies: []Dependency{
			{Name: "postgres", Required: true, Check: alwaysOK},
			{Name: "redis", Required: false, Check: alwaysOK},
		},
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := readyBody(t, rec)["status"]; got != "ready" {
		t.Errorf(`status field = %v, want "ready"`, got)
	}
}

func TestReadiness_OptionalDependencyDownStaysReady(t *testing.T) {
	r := newRouter(Handler{
		Dependencies: []Dependency{
			{Name: "postgres", Required: true, Check: alwaysOK},
			{Name: "redis", Required: false, Check: func(context.Context) error {
				return errors.New("dial tcp 127.0.0.1:6379: connect: connection refused")
			}},
		},
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (NFR-02: fail open when only the optional dependency is down)", rec.Code)
	}
	if got := readyBody(t, rec)["status"]; got != "degraded" {
		t.Errorf(`status field = %v, want "degraded"`, got)
	}
}

func TestReadiness_RequiredDependencyDownReturns503WithEnvelope(t *testing.T) {
	r := newRouter(Handler{
		Dependencies: []Dependency{
			{Name: "postgres", Required: true, Check: func(context.Context) error {
				return errors.New("dial tcp 10.0.1.7:5432: connect: connection refused")
			}},
			{Name: "redis", Required: false, Check: alwaysOK},
		},
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if body.Error.Code != "SERVICE_UNAVAILABLE" {
		t.Errorf("code = %q, want SERVICE_UNAVAILABLE", body.Error.Code)
	}
	if body.Error.Message == "" {
		t.Error("message is empty")
	}
}

func TestReadiness_BodyNamesNoDependencyOnFailure(t *testing.T) {
	r := newRouter(Handler{
		Dependencies: []Dependency{
			{Name: "postgres", Required: true, Check: func(context.Context) error {
				return errors.New("dial tcp 10.0.1.7:5432: pgx: connection refused")
			}},
		},
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	body := strings.ToLower(rec.Body.String())
	for _, leaky := range []string{"postgres", "10.0.1.7", "pgx", "tcp"} {
		if strings.Contains(body, leaky) {
			t.Errorf("response body %q leaks dependency detail %q", rec.Body.String(), leaky)
		}
	}
}

func TestReadiness_BothDependenciesDownReturns503(t *testing.T) {
	r := newRouter(Handler{
		Dependencies: []Dependency{
			{Name: "postgres", Required: true, Check: func(context.Context) error { return errors.New("down") }},
			{Name: "redis", Required: false, Check: func(context.Context) error { return errors.New("down") }},
		},
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 (required failure takes priority over degraded)", rec.Code)
	}
}

func TestReadiness_HungDependencyIsAbandonedAtItsTimeout(t *testing.T) {
	r := newRouter(Handler{
		Dependencies: []Dependency{
			{
				Name:     "postgres",
				Required: true,
				Timeout:  50 * time.Millisecond,
				Check: func(ctx context.Context) error {
					<-ctx.Done()
					return ctx.Err()
				},
			},
		},
	})

	start := time.Now()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	elapsed := time.Since(start)

	if elapsed > time.Second {
		t.Errorf("readiness took %v, want it bounded by the 50ms check timeout", elapsed)
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", rec.Code)
	}
}

func TestReadiness_ChecksRunConcurrentlyNotSequentially(t *testing.T) {
	const perCheck = 100 * time.Millisecond
	slow := func(context.Context) error {
		time.Sleep(perCheck)
		return nil
	}

	r := newRouter(Handler{
		Dependencies: []Dependency{
			{Name: "a", Required: true, Check: slow},
			{Name: "b", Required: true, Check: slow},
			{Name: "c", Required: true, Check: slow},
		},
	})

	start := time.Now()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	elapsed := time.Since(start)

	// Sequential would take ~3*perCheck; concurrent takes ~1*perCheck.
	if elapsed >= 2*perCheck {
		t.Errorf("readiness took %v, want well under %v (checks should run concurrently)", elapsed, 2*perCheck)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestDependency_DefaultTimeoutAppliesWhenUnset(t *testing.T) {
	dep := Dependency{Name: "x", Check: alwaysOK}
	if dep.timeout() != defaultTimeout {
		t.Errorf("timeout() = %v, want default %v", dep.timeout(), defaultTimeout)
	}
}

