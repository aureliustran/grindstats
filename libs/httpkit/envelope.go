// Package httpkit is the one place that writes the API error envelope
// ({ "error": { "code", "message" } }, docs/backend.md §5), so every
// endpoint returns byte-identical shape and no handler hand-writes an error
// code string (docs/backend.md §5a).
package httpkit

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"grindstats/libs/auditmodel"
	"grindstats/libs/i18n"
)

type errorBody struct {
	Code    auditmodel.ErrorCode `json:"code"`
	Message string               `json:"message"`
}

type envelope struct {
	Error errorBody `json:"error"`
}

// LocaleContextKey is where the gateway's locale middleware stores the
// request's already-resolved locale, so Error need not re-parse
// Accept-Language on every call. Exported so that middleware — which lives
// in a higher-level package than httpkit — can write to the same key
// without httpkit importing anything above it.
const LocaleContextKey = "i18n.locale"

// Locale returns the resolved locale a middleware stored on c, falling back
// to resolving the raw Accept-Language header when nothing was stored (e.g.
// a unit test building a bare context without the full chain).
func Locale(c *gin.Context) string {
	if v, ok := c.Get(LocaleContextKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return i18n.ResolveLocale(c.GetHeader("Accept-Language"))
}

// Error writes the standard error envelope for code, with c.Status set from
// auditmodel.ErrorCodes and the message rendered from libs/i18n in the
// request's resolved locale (falling back to i18n.SourceLocale). It panics
// if code is not a declared error code — a hand-written or stale code is a
// programmer error to catch immediately, not a shape to serve to a caller.
func Error(c *gin.Context, code auditmodel.ErrorCode) {
	spec, ok := auditmodel.ErrorCodes[code]
	if !ok {
		panic("httpkit: undeclared error code " + string(code))
	}

	message := i18n.Render(string(code), Locale(c))

	c.AbortWithStatusJSON(spec.HTTPStatus, envelope{
		Error: errorBody{Code: code, Message: message},
	})
}

// InternalError writes the INTERNAL_ERROR envelope. It is the one place a
// recovery middleware should call after a panic, so that path never needs to
// know the code by name.
func InternalError(c *gin.Context) {
	Error(c, auditmodel.ErrInternalError)
}

// ServiceUnavailable writes the SERVICE_UNAVAILABLE envelope, used by the
// readiness probe when a required dependency is unreachable (GATE-001). The
// body never names which dependency failed; that detail belongs in the
// structured log, keyed by the request ID.
func ServiceUnavailable(c *gin.Context) {
	Error(c, auditmodel.ErrServiceUnavailable)
}

// StatusFor is a small convenience for callers that need the HTTP status a
// code maps to without writing the envelope (e.g. an assertion in a test).
func StatusFor(code auditmodel.ErrorCode) int {
	spec, ok := auditmodel.ErrorCodes[code]
	if !ok {
		return http.StatusInternalServerError
	}
	return spec.HTTPStatus
}
