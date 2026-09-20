package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestIDHeader is read from an incoming request and echoed on the
// response; a caller-supplied ID (e.g. from an upstream proxy) survives the
// round trip so a request can be traced across services.
const RequestIDHeader = "X-Request-ID"

const requestIDContextKey = "gateway.request_id"

// RequestID accepts an inbound X-Request-ID or generates one, stores it on
// the context for later middleware (Logging, Recovery) and echoes it on the
// response.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(RequestIDHeader)
		if id == "" {
			id = uuid.NewString()
		}
		c.Set(requestIDContextKey, id)
		c.Writer.Header().Set(RequestIDHeader, id)
		c.Next()
	}
}

// RequestIDFrom returns the request ID set by RequestID, or "" if that
// middleware never ran (e.g. a test constructing a bare context).
func RequestIDFrom(c *gin.Context) string {
	if v, ok := c.Get(requestIDContextKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
