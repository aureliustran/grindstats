package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
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
			id = generateRequestID()
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

func generateRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failing is effectively unheard of, but a request ID
		// must never block or fail the request that needs it traced.
		return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
