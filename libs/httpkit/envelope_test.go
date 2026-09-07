package httpkit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"grindstats/libs/auditmodel"
	"grindstats/libs/i18n"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newTestContext(acceptLanguage string) (*gin.Context, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	if acceptLanguage != "" {
		c.Request.Header.Set("Accept-Language", acceptLanguage)
	}
	return c, rec
}

func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) envelope {
	t.Helper()
	var env envelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return env
}

func TestError_WritesStatusAndCodeFromRegistry(t *testing.T) {
	c, rec := newTestContext("")

	Error(c, auditmodel.ErrServiceUnavailable)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	env := decodeEnvelope(t, rec)
	if env.Error.Code != auditmodel.ErrServiceUnavailable {
		t.Errorf("code = %q, want %q", env.Error.Code, auditmodel.ErrServiceUnavailable)
	}
}

func TestError_MessageRendersInRequestLocale(t *testing.T) {
	c, rec := newTestContext("vi-VN")

	Error(c, auditmodel.ErrServiceUnavailable)

	env := decodeEnvelope(t, rec)
	want := i18n.Render(string(auditmodel.ErrServiceUnavailable), "vi-VN")
	if env.Error.Message != want {
		t.Errorf("message = %q, want %q", env.Error.Message, want)
	}
	if env.Error.Message == i18n.Render(string(auditmodel.ErrServiceUnavailable), "en-US") {
		t.Error("vi-VN request produced the en-US message")
	}
}

func TestError_CodeIsIdenticalAcrossLocales(t *testing.T) {
	// The three-planes rule at the envelope boundary: locale changes the
	// message, never the code a client branches on.
	viCtx, viRec := newTestContext("vi-VN")
	Error(viCtx, auditmodel.ErrServiceUnavailable)

	enCtx, enRec := newTestContext("en-US")
	Error(enCtx, auditmodel.ErrServiceUnavailable)

	viEnv := decodeEnvelope(t, viRec)
	enEnv := decodeEnvelope(t, enRec)

	if viEnv.Error.Code != enEnv.Error.Code {
		t.Errorf("code differs across locales: vi-VN=%q en-US=%q", viEnv.Error.Code, enEnv.Error.Code)
	}
	if viEnv.Error.Message == enEnv.Error.Message {
		t.Error("messages are identical across locales; catalogs should differ")
	}
}

func TestError_UndeclaredCodePanics(t *testing.T) {
	c, _ := newTestContext("")

	defer func() {
		if recover() == nil {
			t.Fatal("Error did not panic on an undeclared code")
		}
	}()

	Error(c, auditmodel.ErrorCode("NOT_A_REAL_CODE"))
}

func TestInternalError_WritesInternalErrorCode(t *testing.T) {
	c, rec := newTestContext("")

	InternalError(c)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	env := decodeEnvelope(t, rec)
	if env.Error.Code != auditmodel.ErrInternalError {
		t.Errorf("code = %q, want %q", env.Error.Code, auditmodel.ErrInternalError)
	}
}

func TestServiceUnavailable_NamesNoDependencyInBody(t *testing.T) {
	c, rec := newTestContext("")

	ServiceUnavailable(c)

	body := strings.ToLower(rec.Body.String())
	for _, leaky := range []string{"postgres", "redis", "pgx", "tcp", "dial"} {
		if strings.Contains(body, leaky) {
			t.Errorf("body %q leaks dependency detail %q", rec.Body.String(), leaky)
		}
	}
}

func TestStatusFor_KnownAndUnknownCodes(t *testing.T) {
	if got := StatusFor(auditmodel.ErrServiceUnavailable); got != http.StatusServiceUnavailable {
		t.Errorf("StatusFor(ServiceUnavailable) = %d, want %d", got, http.StatusServiceUnavailable)
	}
	if got := StatusFor(auditmodel.ErrorCode("NOT_A_REAL_CODE")); got != http.StatusInternalServerError {
		t.Errorf("StatusFor(unknown) = %d, want %d", got, http.StatusInternalServerError)
	}
}
