// Package httpkit is the one place that writes an API response envelope —
// success (§ "Success responses" below), error ({ "error": { "code",
// "message" } }, docs/backend.md §5), or paginated — so every endpoint
// returns byte-identical shape and no handler hand-writes an error code
// string (docs/backend.md §5a) or a bespoke success/list body.
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

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

// --- Success responses -----------------------------------------------------
//
// Every non-error response body is enveloped the same way a caller can rely
// on without inspecting the endpoint: a single resource or action result is
// { "data": ... }, and a list is { "data": [...], "pagination": {...} }.
// A handler never writes a bare gin.H{...} body — that reintroduces the
// per-endpoint shape drift the error envelope already exists to prevent.

type dataEnvelope[T any] struct {
	Data T `json:"data"`
}

// Pagination describes one page of a list response. TotalPages is derived
// from TotalItems and PageSize by NewPagination so callers never compute it
// (and risk an off-by-one) themselves.
type Pagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	TotalItems int `json:"total_items"`
	TotalPages int `json:"total_pages"`
}

// NewPagination computes TotalPages from totalItems and pageSize. A
// non-positive pageSize yields TotalPages 0 rather than dividing by zero —
// callers pass whatever the request validated, and a bad pageSize should
// have already failed validation before reaching here.
func NewPagination(page, pageSize, totalItems int) Pagination {
	totalPages := 0
	if pageSize > 0 {
		totalPages = (totalItems + pageSize - 1) / pageSize
	}
	return Pagination{
		Page:       page,
		PageSize:   pageSize,
		TotalItems: totalItems,
		TotalPages: totalPages,
	}
}

type listEnvelope[T any] struct {
	Data       []T        `json:"data"`
	Pagination Pagination `json:"pagination"`
}

// Data writes data enveloped as { "data": ... } with the given status code.
// Use the OK/Created shorthands for the common cases.
func Data[T any](c *gin.Context, status int, data T) {
	c.JSON(status, dataEnvelope[T]{Data: data})
}

// OK writes data enveloped as { "data": ... } with 200 OK — the shape for a
// successful read or update.
func OK[T any](c *gin.Context, data T) {
	Data(c, http.StatusOK, data)
}

// Created writes data enveloped as { "data": ... } with 201 Created — the
// shape for a successful resource creation.
func Created[T any](c *gin.Context, data T) {
	Data(c, http.StatusCreated, data)
}

// Paginated writes one page of items as { "data": [...], "pagination": {...} }
// with 200 OK. A nil items slice is written as [], never null — a client
// iterating an empty page should never need a nil check the non-empty case
// doesn't also require.
func Paginated[T any](c *gin.Context, items []T, page Pagination) {
	if items == nil {
		items = []T{}
	}
	c.JSON(http.StatusOK, listEnvelope[T]{Data: items, Pagination: page})
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

	c.AbortWithStatusJSON(spec.HTTPStatus, errorEnvelope{
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
